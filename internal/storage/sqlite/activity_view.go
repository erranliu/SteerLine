package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// ReadActivityView pins identity records and bounded events to one WAL snapshot.
func (s Reader) ReadActivityView(ctx context.Context, project, workflow, execution, attempt string, after int64, limit, payloadBudget int) (storageports.ActivityBinding, []storageports.DomainEvent, error) {
	if s.db == nil {
		return storageports.ActivityBinding{}, nil, errors.New("activity view reader is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return storageports.ActivityBinding{}, nil, err
	}
	defer tx.Rollback()
	binding, events, err := readActivityView(ctx, tx, project, workflow, execution, attempt, after, limit, payloadBudget)
	if err != nil {
		return storageports.ActivityBinding{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return storageports.ActivityBinding{}, nil, err
	}
	return binding, events, nil
}

func readActivityView(ctx context.Context, db graphSnapshotQuery, project, workflow, execution, attempt string, after int64, limit, payloadBudget int) (storageports.ActivityBinding, []storageports.DomainEvent, error) {
	binding, err := readActivityBinding(ctx, db, project, execution, attempt)
	if err != nil {
		return storageports.ActivityBinding{}, nil, err
	}
	events, err := readBoundEvents(ctx, db, project, workflow, execution, attempt, after, limit, payloadBudget)
	if err != nil {
		return storageports.ActivityBinding{}, nil, err
	}
	return binding, events, nil
}

func readActivityBinding(ctx context.Context, db CommandRowQuerier, project, execution, attempt string) (storageports.ActivityBinding, error) {
	binding := storageports.ActivityBinding{ProjectIdentity: project}
	var payload []byte
	err := db.QueryRowContext(ctx, `SELECT payload FROM executions WHERE project_identity=? AND execution_id=?`, project, execution).Scan(&payload)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return storageports.ActivityBinding{}, err
	}
	if err == nil {
		var record executionmodel.ExecutionRecord
		if err = json.Unmarshal(payload, &record); err != nil {
			return storageports.ActivityBinding{}, err
		}
		if record.ProjectIdentity != project || record.ExecutionID != execution {
			return storageports.ActivityBinding{}, errors.New("activity execution row and payload identity differ")
		}
		binding.ExecutionID = record.ExecutionID
		binding.ExecutionWorkflowRunID = record.WorkflowRunID
	}
	payload = nil
	err = db.QueryRowContext(ctx, `SELECT payload FROM step_attempts WHERE project_identity=? AND execution_id=? AND attempt_id=?`, project, execution, attempt).Scan(&payload)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return storageports.ActivityBinding{}, err
	}
	if err == nil {
		var record executionmodel.StepAttemptRecord
		if err = json.Unmarshal(payload, &record); err != nil {
			return storageports.ActivityBinding{}, err
		}
		if record.ProjectIdentity != project || record.ExecutionID != execution || record.AttemptID != attempt {
			return storageports.ActivityBinding{}, errors.New("activity attempt row and payload identity differ")
		}
		binding.AttemptID = record.AttemptID
		binding.AttemptExecutionID = record.ExecutionID
	}
	return binding, nil
}
