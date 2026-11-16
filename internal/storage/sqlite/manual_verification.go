package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	mv "github.com/erranliu/SteerLine/internal/manualverification"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func readManualVerification(ctx context.Context, db schemaQuery, project, id string) (mv.Record, error) {
	var raw []byte
	var r mv.Record
	err := db.QueryRowContext(ctx, `SELECT payload FROM manual_verifications WHERE project_identity=? AND candidate_id=?`, project, id).Scan(&raw)
	if err == nil {
		err = json.Unmarshal(raw, &r)
	}
	return r, err
}

func (s *SQLiteStore) ManualVerification(ctx context.Context, project, id string) (mv.Record, error) {
	return readManualVerification(ctx, s.db, project, id)
}

func (s *SQLiteStore) ManualVerifications(ctx context.Context, project string) ([]mv.Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM manual_verifications WHERE project_identity=? ORDER BY candidate_id`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []mv.Record{}
	for rows.Next() {
		var r mv.Record
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

func writeManualVerification(ctx context.Context, tx *sql.Tx, r mv.Record) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO manual_verifications(project_identity,candidate_id,revision,payload) VALUES(?,?,?,?) ON CONFLICT(project_identity,candidate_id) DO UPDATE SET revision=excluded.revision,payload=excluded.payload`, r.Project, r.Candidate, r.Revision, raw)
	return err
}

func (s *SQLiteStore) ObserveManualPR(ctx context.Context, project string, observation ports.RunnerPRObservation, observationErr error) error {
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		return tx.observeManualPR(ctx, project, observation, observationErr)
	})
	if errors.Is(err, errAtomicReplay) {
		return nil
	}
	return err
}

func (tx *sqliteTx) observeManualPR(ctx context.Context, project string, observation ports.RunnerPRObservation, observationErr error) error {
	c, err := readRunnerCandidate(ctx, tx.tx, project, observation.CandidateID)
	if err != nil {
		return err
	}
	if len(c.ManualChecks) == 0 || c.RunID != observation.RunID {
		return errAtomicReplay
	}
	target, err := ports.BindRunnerPR(c, c.Contract.Branch)
	if err != nil {
		return errAtomicReplay
	}
	r, err := readManualVerification(ctx, tx.tx, project, c.ID)
	missing := errors.Is(err, sql.ErrNoRows)
	if err != nil && !missing {
		return err
	}
	if missing {
		first := c.Projection.PR
		if !first.InitialDraftVerified || !taskmodel.ValidCommit(first.HeadRevision) {
			return errAtomicReplay
		}
		var definition mv.Snapshot
		if err := json.Unmarshal(c.ManualChecks, &definition); err != nil {
			return err
		}
		r = mv.Record{Project: project, Candidate: c.ID, Task: c.TaskID, PR: target.URL, Head: first.HeadRevision, Draft: true, Definition: definition, Attempts: []mv.Attempt{}, Results: []mv.Result{}}
	}
	before := r
	fact := observation.Fact
	if observation.CheckedAt.Before(r.ObservedAt) && !observation.CheckedAt.IsZero() {
		return errAtomicReplay
	}
	if observationErr != nil {
		r.Diagnostic = observationErr.Error()
	} else if target.Matches(fact) && (!r.Merged || fact.State == "merged" && fact.Merged) {
		r.Diagnostic = ""
		if !r.Merged && taskmodel.ValidCommit(fact.HeadRevision) {
			if r.Head != fact.HeadRevision {
				r.Closure = nil
			}
			r.Head = fact.HeadRevision
		}
		if fact.Draft != nil {
			r.Draft = *fact.Draft
		} else if fact.State == "open" {
			r.Diagnostic = "PR 平台未返回草稿状态"
		}
		if fact.State == "merged" && fact.Merged && taskmodel.ValidCommit(fact.HeadRevision) {
			r.Merged = true
			r.Draft = false
		}
		r.Archived = !r.Merged && fact.State == "closed_unmerged"
		r.ObservedAt = observation.CheckedAt
	}
	if !r.Merged && (c.Participation != "active" || c.StopRequested) {
		r.Archived = true
	}
	if !missing && before.Head == r.Head && before.Draft == r.Draft && before.Merged == r.Merged && before.Archived == r.Archived && before.Diagnostic == r.Diagnostic {
		return errAtomicReplay
	}
	r.Revision++
	return writeManualVerification(ctx, tx.tx, r)
}

func (s *SQLiteStore) UpdateManualVerification(ctx context.Context, project, id string, mutate func(*mv.Record) error) error {
	return s.transactSQLite(ctx, func(tx *sqliteTx) error {
		r, err := readManualVerification(ctx, tx.tx, project, id)
		if err != nil {
			return err
		}
		if err := mutate(&r); err != nil {
			return err
		}
		r.Revision++
		return writeManualVerification(ctx, tx.tx, r)
	})
}

func (s *SQLiteStore) ApplyManualVerification(ctx context.Context, command ports.CommandRecord, mutate func(*mv.Record) error) error {
	return s.transactSQLite(ctx, func(tx *sqliteTx) error {
		var original []byte
		err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM commands WHERE project_identity=? AND command_id=? AND kind=?`, command.ProjectIdentity, command.CommandID, command.Kind).Scan(&original)
		if err == nil {
			if string(original) != string(command.Payload) {
				return errors.New("请求 ID 已用于其他操作")
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		r, err := readManualVerification(ctx, tx.tx, command.ProjectIdentity, command.TargetID)
		if err != nil {
			return err
		}
		if r.Revision != command.ExpectedVersion {
			return errors.New("核验记录已更新，请刷新后重试")
		}
		c, err := readRunnerCandidate(ctx, tx.tx, r.Project, r.Candidate)
		if err != nil {
			return err
		}
		if !r.Merged && (c.Participation != "active" || c.StopRequested) {
			r.Archived = true
		}
		if err := mutate(&r); err != nil {
			return err
		}
		r.Revision++
		if err := writeManualVerification(ctx, tx.tx, r); err != nil {
			return err
		}
		at := now().Format("2006-01-02T15:04:05.999999999Z07:00")
		_, err = tx.tx.ExecContext(ctx, `INSERT INTO commands(project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, command.ProjectIdentity, command.CommandID, command.Kind, command.TargetID, command.ExpectedVersion, command.CommandID, command.RequestedBy, command.Payload, "applied", at, at)
		return err
	})
}

func (s *SQLiteStore) ManualVerificationReceipt(ctx context.Context, project, id string) ([]byte, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM commands WHERE project_identity=? AND command_id=? AND kind='manual_verification'`, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return raw, err
}
