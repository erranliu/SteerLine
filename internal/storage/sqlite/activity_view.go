package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (s Reader) OpenActivityView(ctx context.Context, project, candidate, run, attempt string, after int64, limit int, recent bool) (p.ActivityBinding, p.ActivityRecords, error) {
	binding := p.ActivityBinding{ProjectIdentity: project}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return binding, nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			_ = tx.Rollback()
		}
	}()
	c, err := readRunnerCandidate(ctx, tx, project, candidate)
	if err != nil {
		return binding, nil, err
	}
	if c.RunID != run || attempt == "" {
		return binding, nil, errors.New("activity differs from original candidate Run")
	}
	// Past Attempts need an original public event as proof, in this same sample.
	known := c.Projection != nil && c.Projection.Execution.AttemptID == attempt
	if !known {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM runner_events WHERE project_identity=? AND candidate_id=? AND json_extract(payload,'$.attempt_id')=?`, project, candidate, attempt).Scan(&count); err != nil {
			return binding, nil, err
		}
		known = count > 0
	}
	if !known {
		return binding, nil, errors.New("activity Attempt is not recorded by Runner")
	}
	binding.ExecutionID, binding.ExecutionWorkflowRunID = run, candidate
	binding.AttemptID, binding.AttemptExecutionID = attempt, run
	limit = max(1, min(limit, 1000))
	query := `SELECT seq,payload FROM runner_events WHERE project_identity=? AND candidate_id=? AND seq>? AND json_extract(payload,'$.attempt_id')=? ORDER BY seq ASC LIMIT ?`
	if recent {
		// Keep the same latest five changes and ten technical facts. Rendering
		// and projected byte budgets belong to product/query.
		query = `WITH ranked AS (SELECT seq,payload, row_number() OVER (PARTITION BY (json_extract(payload,'$.transition') IS NOT NULL) ORDER BY seq DESC) n FROM runner_events WHERE project_identity=? AND candidate_id=? AND seq>? AND json_extract(payload,'$.attempt_id')=?) SELECT seq,payload FROM ranked WHERE n<=CASE WHEN json_extract(payload,'$.transition') IS NOT NULL THEN 5 ELSE 10 END ORDER BY seq DESC LIMIT ?`
	}
	rows, err := tx.QueryContext(ctx, query, project, candidate, after, attempt, limit)
	if err != nil {
		return binding, nil, err
	}
	transferred = true
	return binding, &activityRecords{tx: tx, rows: rows, run: run, task: c.TaskID}, nil
}

type activityRecords struct {
	tx         *sql.Tx
	rows       *sql.Rows
	run, task  string
	closed     bool
	closeError error
}

func (r *activityRecords) Next() (*p.ActivityRecord, error) {
	if r.closed {
		return nil, errors.New("activity read stream is closed")
	}
	if !r.rows.Next() {
		return nil, r.rows.Err()
	}
	var seq int64
	var raw []byte
	if err := r.rows.Scan(&seq, &raw); err != nil {
		return nil, err
	}
	var event runnercontract.Event
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, err
	}
	if event.RunID != r.run {
		return nil, errors.New("Runner event Run identity differs")
	}
	return &p.ActivityRecord{EventID: seq, TaskID: r.task, Event: event}, nil
}

func (r *activityRecords) Close() error {
	if !r.closed {
		r.closed = true
		r.closeError = errors.Join(r.rows.Close(), r.tx.Commit())
	}
	return r.closeError
}
