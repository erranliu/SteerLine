package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"

	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

func readGeneration(ctx context.Context, db schemaQuery, project, id string) (testmodel.Generation, error) {
	var r testmodel.Generation
	var raw []byte
	var revision uint64
	err := db.QueryRowContext(ctx, `SELECT revision,payload FROM test_generations WHERE project_identity=? AND generation_id=?`, project, id).Scan(&revision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return r, testmodel.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(raw, &r); err != nil {
		return r, err
	}
	if r.Project != project || r.ID != id || r.Revision != revision || revision == 0 {
		return r, errors.New("generation association mismatch")
	}
	return r, nil
}
func (s *SQLiteStore) Generation(ctx context.Context, project, id string) (testmodel.Generation, error) {
	return readGeneration(ctx, s.db, project, id)
}
func (s *SQLiteStore) Generations(ctx context.Context, project string) ([]testmodel.Generation, error) {
	return readGenerations(ctx, s.db, project)
}
func (s Reader) Generations(ctx context.Context, project string) ([]testmodel.Generation, error) {
	return readGenerations(ctx, s.db, project)
}
func readGenerations(ctx context.Context, db schemaQuery, project string) ([]testmodel.Generation, error) {
	rows, err := db.QueryContext(ctx, `SELECT payload FROM test_generations WHERE project_identity=? ORDER BY generation_id`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []testmodel.Generation{}
	for rows.Next() {
		var raw []byte
		var r testmodel.Generation
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		if r.Project != project {
			return nil, errors.New("generation project mismatch")
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
func (s *SQLiteStore) SaveGeneration(ctx context.Context, r testmodel.Generation, expected uint64) error {
	if s.readOnly || r.Project == "" || r.ID == "" || r.Revision != expected+1 || r.Request.RequestID == "" {
		return errors.New("invalid generation write")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old, readErr := readGeneration(ctx, tx, r.Project, r.ID)
	if expected == 0 {
		if !errors.Is(readErr, testmodel.ErrNotFound) {
			return errors.New("generation already exists or cannot be read")
		}
	} else {
		if readErr != nil {
			return readErr
		}
		if old.Revision != expected {
			return errors.New("stale_revision: generation changed")
		}
		if old.Submission != r.Submission && !(old.Submission == "pending" && r.Submission == "started") {
			return errors.New("generation submission cannot move backwards")
		}
		if old.Inputs != r.Inputs || old.PlanText != r.PlanText || len(old.Result) > 0 && !bytes.Equal(old.Result, r.Result) {
			return errors.New("immutable generation input/result changed")
		}
		if old.Command != r.Command || !reflect.DeepEqual(old.Request, r.Request) || !reflect.DeepEqual(old.Runner, r.Runner) || old.Target != r.Target || !reflect.DeepEqual(old.Sources, r.Sources) || !reflect.DeepEqual(old.Catalogue, r.Catalogue) || old.RunID != "" && old.RunID != r.RunID || old.InputDigest != "" && old.InputDigest != r.InputDigest || old.Cancelled && !r.Cancelled || old.StopID != "" && old.StopID != r.StopID || old.StopRequest != nil && !reflect.DeepEqual(old.StopRequest, r.StopRequest) {
			return errors.New("immutable generation association changed")
		}
	}
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if expected == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO test_generations VALUES(?,?,?,?)`, r.Project, r.ID, r.Revision, body)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE test_generations SET revision=?,payload=? WHERE project_identity=? AND generation_id=? AND revision=?`, r.Revision, body, r.Project, r.ID, expected)
		if err == nil {
			var n int64
			n, err = result.RowsAffected()
			if err == nil && n != 1 {
				err = errors.New("stale_revision: generation changed")
			}
		}
	}
	if err != nil {
		return err
	}
	for _, id := range []string{r.ID, r.StopID} {
		if id == "" {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO test_generation_requests VALUES(?,?,?) ON CONFLICT DO NOTHING`, r.Project, id, r.ID); err != nil {
			return err
		}
		var key string
		if err = tx.QueryRowContext(ctx, `SELECT generation_id FROM test_generation_requests WHERE project_identity=? AND request_id=?`, r.Project, id).Scan(&key); err != nil {
			return err
		}
		if key != r.ID {
			return errors.New("generation request conflict")
		}
	}

	return tx.Commit()
}
