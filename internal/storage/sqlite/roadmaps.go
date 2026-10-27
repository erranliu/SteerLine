package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"

	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
)

func readRoadmap(ctx context.Context, db schemaQuery, project, id string) (roadmap.Record, error) {
	var raw []byte
	var rev uint64
	var stored roadmap.Stored
	e := db.QueryRowContext(ctx, `SELECT revision,payload FROM roadmaps WHERE project_identity=? AND roadmap_id=?`, project, id).Scan(&rev, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return roadmap.Record{}, roadmap.ErrNotFound
	}
	if e != nil {
		return roadmap.Record{}, e
	}
	if e = json.Unmarshal(raw, &stored); e != nil {
		return roadmap.Record{}, e
	}
	r := stored.Value()
	if r.Project != project || r.ID != id || r.Revision != rev {
		return r, errors.New("roadmap identity/revision mismatch")
	}
	return r, nil
}
func (s *SQLiteStore) Roadmap(ctx context.Context, project, id string) (roadmap.Record, error) {
	return readRoadmap(ctx, s.db, project, id)
}
func (s Reader) Roadmaps(ctx context.Context, project string) ([]roadmap.Record, error) {
	return readRoadmaps(ctx, s.db, project)
}
func (s *SQLiteStore) Roadmaps(ctx context.Context, project string) ([]roadmap.Record, error) {
	return readRoadmaps(ctx, s.db, project)
}
func readRoadmaps(ctx context.Context, db schemaQuery, project string) ([]roadmap.Record, error) {
	rows, e := db.QueryContext(ctx, `SELECT payload FROM roadmaps WHERE project_identity=? ORDER BY roadmap_id`, project)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []roadmap.Record{}
	for rows.Next() {
		var raw []byte
		var stored roadmap.Stored
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &stored); e != nil {
			return nil, e
		}
		r := stored.Value()
		if r.Project != project {
			return nil, errors.New("roadmap project differs")
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *SQLiteStore) SaveRoadmap(ctx context.Context, r roadmap.Record, expected uint64) error {
	if s.readOnly || r.Project == "" || !roadmap.IDPattern.MatchString(r.ID) || r.Revision != expected+1 {
		return errors.New("invalid roadmap write")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	old, readErr := readRoadmap(ctx, tx, r.Project, r.ID)
	if expected == 0 {
		if !errors.Is(readErr, roadmap.ErrNotFound) {
			return errors.New("roadmap already exists or cannot be read")
		}
	} else {
		if readErr != nil {
			return readErr
		}
		if old.Revision != expected {
			return errors.New("stale_revision: roadmap changed")
		}
		if old.Purpose != r.Purpose || old.ParentID != r.ParentID || old.RepositoryTree != r.RepositoryTree ||
			old.CodeCommit != "" && old.CodeCommit != r.CodeCommit || !reflect.DeepEqual(old.Materials, r.Materials) {
			return errors.New("immutable roadmap generation context changed")
		}
		stopChanged := old.StopRequest != nil && !reflect.DeepEqual(old.StopRequest, r.StopRequest)
		stopRetried := old.StopState == "rejected" && r.StopState == "unknown" && r.StopRequest != nil && old.StopRequest != nil && r.StopRequest.RequestID != old.StopRequest.RequestID
		if !reflect.DeepEqual(old.Source, r.Source) || !reflect.DeepEqual(old.Existing, r.Existing) || old.Profile != r.Profile || old.Runner != r.Runner || old.Submission == "started" && r.Submission != "started" || old.ConfirmationRequest != "" && old.ConfirmationRequest != r.ConfirmationRequest || old.ConfirmedDigest != "" && old.ConfirmedDigest != r.ConfirmedDigest || old.StopID != "" && old.StopID != r.StopID || stopChanged && !stopRetried || old.AdmissionRequest != "" && (old.AdmissionRequest != r.AdmissionRequest || !bytes.Equal(old.AdmissionBatch, r.AdmissionBatch)) {
			return errors.New("immutable roadmap association changed")
		}
		if old.Publication != nil {
			if r.Publication == nil || old.Publication.RequestID != r.Publication.RequestID || old.Publication.ReviewDigest != r.Publication.ReviewDigest || old.Publication.Branch != r.Publication.Branch || old.Publication.Base != r.Publication.Base || old.Publication.Merge != r.Publication.Merge || old.Publication.Commit != "" && old.Publication.Commit != r.Publication.Commit || old.Publication.URL != "" && old.Publication.URL != r.Publication.URL || old.Publication.MergedCommit != "" && old.Publication.MergedCommit != r.Publication.MergedCommit {
				return errors.New("immutable publication binding changed")
			}
		}
		if old.RequestID != r.RequestID || old.Source.Digest != r.Source.Digest || old.GraphDigest != r.GraphDigest || roadmap.ValueDigest(old.Request) != roadmap.ValueDigest(r.Request) || old.RunID != "" && old.RunID != r.RunID || old.InputDigest != "" && old.InputDigest != r.InputDigest || old.Cancelled && !r.Cancelled || len(old.Result) > 0 && string(old.Result) != string(r.Result) {
			return errors.New("immutable roadmap input/result changed")
		}
	}
	raw, e := json.Marshal(roadmap.StoreValue(r))
	if e != nil {
		return e
	}
	if expected == 0 {
		_, e = tx.ExecContext(ctx, `INSERT INTO roadmaps VALUES(?,?,?,?)`, r.Project, r.ID, r.Revision, raw)
	} else {
		var result sql.Result
		result, e = tx.ExecContext(ctx, `UPDATE roadmaps SET revision=?,payload=? WHERE project_identity=? AND roadmap_id=? AND revision=?`, r.Revision, raw, r.Project, r.ID, expected)
		if e == nil {
			e = requireIdentityWrite(result, nil, "stale_revision: roadmap changed")
		}
	}
	if e != nil {
		return e
	}
	publicationRequest := ""
	if r.Publication != nil {
		publicationRequest = r.Publication.RequestID
	}
	for _, id := range []string{r.RequestID, r.ConfirmationRequest, r.StopID, r.AdmissionRequest, publicationRequest} {
		if id == "" {
			continue
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO roadmap_requests VALUES(?,?,?) ON CONFLICT DO NOTHING`, r.Project, id, r.ID)
		if e != nil {
			return e
		}
		var bound string
		if e = tx.QueryRowContext(ctx, `SELECT roadmap_id FROM roadmap_requests WHERE project_identity=? AND request_id=?`, r.Project, id).Scan(&bound); e != nil {
			return e
		}
		if bound != r.ID {
			return errors.New("roadmap request conflict")
		}
	}
	return tx.Commit()
}
func (s *SQLiteStore) RoadmapRequest(ctx context.Context, project, id string) (roadmap.Record, error) {
	var bound string
	e := s.db.QueryRowContext(ctx, `SELECT roadmap_id FROM roadmap_requests WHERE project_identity=? AND request_id=?`, project, id).Scan(&bound)
	if errors.Is(e, sql.ErrNoRows) {
		return roadmap.Record{}, roadmap.ErrNotFound
	}
	if e != nil {
		return roadmap.Record{}, e
	}
	return s.Roadmap(ctx, project, bound)
}
func (s *SQLiteStore) RoadmapMapping(ctx context.Context, project, id string) (map[string]string, error) {
	out := map[string]string{}
	rows, e := s.db.QueryContext(ctx, `SELECT local_key,task_id FROM roadmap_task_sources WHERE project_identity=? AND roadmap_id=?`, project, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var key, id string
		if e = rows.Scan(&key, &id); e != nil {
			return nil, e
		}
		out[key] = id
	}
	return out, rows.Err()
}
func (t *sqliteTx) AllocateTaskNumbers(ctx context.Context, project string, count int) ([]int64, error) {
	if count < 1 {
		return nil, errors.New("empty task allocation")
	}
	var repository string
	if e := t.tx.QueryRowContext(ctx, `SELECT repository FROM project_task_authorities WHERE project_identity=?`, project).Scan(&repository); e != nil {
		return nil, e
	}
	var next int64
	if e := t.tx.QueryRowContext(ctx, `SELECT next_id FROM task_number_sequences WHERE repository=?`, repository).Scan(&next); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	rows, e := t.tx.QueryContext(ctx, `SELECT task_id FROM tasks WHERE project_identity IN (SELECT project_identity FROM project_task_authorities WHERE repository=?)`, repository)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		n, e := strconv.ParseInt(id, 10, 64)
		if e != nil || n < 0 {
			rows.Close()
			return nil, errors.New("existing Task number is invalid")
		}
		if n >= next {
			if n == math.MaxInt64 {
				rows.Close()
				return nil, errors.New("Task numbers exhausted")
			}
			next = n + 1
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if next > math.MaxInt64-int64(count) {
		return nil, errors.New("Task numbers exhausted")
	}
	out := make([]int64, count)
	for i := range out {
		out[i] = next + int64(i)
	}
	_, e = t.tx.ExecContext(ctx, `INSERT INTO task_number_sequences VALUES(?,?) ON CONFLICT(repository) DO UPDATE SET next_id=excluded.next_id`, repository, next+int64(count))
	return out, e
}
func (t *sqliteTx) PutRoadmapTaskSource(ctx context.Context, project, id, key, task, request string) error {
	if project == "" || id == "" || key == "" || task == "" || request == "" {
		return errors.New("invalid roadmap source")
	}
	_, e := t.tx.ExecContext(ctx, `INSERT INTO roadmap_task_sources VALUES(?,?,?,?,?)`, project, id, key, task, request)
	return e
}
