package sqlite

import (
	"context"
	"errors"
	"fmt"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/query"
	"github.com/erranliu/SteerLine/internal/steerline/task"
)

type Factory struct{}

func (Factory) OpenWriter(path string) (persistence.Connection, error) {
	v, err := OpenSQLiteStore(path)
	if err != nil {
		return nil, storageFailure(err, persistence.StorageIO)
	}
	return v, nil
}
func (Factory) OpenReader(path string) (persistence.Connection, error) {
	v, err := OpenSQLiteReader(path)
	if err != nil {
		return nil, storageFailure(err, persistence.StorageIO)
	}
	return v, nil
}
func (s *SQLiteStore) BindAuthority(v task.ProjectTaskAuthority) {
	s.Reader = NewReader(s.db, &v)
}
func (s *SQLiteStore) ValidateStoredFormat(ctx context.Context) error {
	if err := validateRetiredWholeRunResultCommands(ctx, s.db); err != nil {
		return err
	}
	return validateNativeSQLiteRuntimeProjection(ctx, s.db)
}
func (s *SQLiteStore) ConversationCapabilityReports(ctx context.Context, backend execution.BackendKind, protocol execution.ExecutionProtocol, target string, limit int) ([]contracts.ConversationCapabilityReport, error) {
	return (query.ConversationService{Capabilities: s.Reader}).ReadCapabilities(ctx, backend, protocol, target, limit)
}

var _ persistence.Connection = (*SQLiteStore)(nil)

func (s *SQLiteStore) ExecutionBinding(ctx context.Context, run, attempt string) (execution.ExecutionRef, error) {
	if !execution.StableExecutionID(run) || !execution.StableExecutionID(attempt) {
		return execution.ExecutionRef{}, errors.New("invalid execution binding path identity")
	}
	var b []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM execution_bindings WHERE run_id=? AND attempt_id=? AND record_id='binding'`, run, attempt).Scan(&b)
	if err != nil {
		return execution.ExecutionRef{}, persistenceError(err)
	}
	var ref execution.ExecutionRef
	if err = decode(b, &ref); err != nil {
		return ref, fmt.Errorf("invalid execution binding; preserve it: %w", err)
	}
	if ref.Version != execution.ExecutionRecordVersion || ref.RunID != run || ref.AttemptID != attempt {
		return ref, errors.New("execution binding identity/version mismatch")
	}
	if err := execution.ValidateExecutionRef(ref); err != nil {
		return ref, fmt.Errorf("invalid execution binding; preserve it: %w", err)
	}
	return ref, nil
}

func (s *SQLiteStore) ExecutionOperation(ctx context.Context, run, attempt, id string) (execution.Operation, error) {
	if !execution.StableExecutionID(run) || !execution.StableExecutionID(attempt) || !execution.StableOperationID(id) {
		return execution.Operation{}, errors.New("invalid operation path identity")
	}
	var b []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM execution_operations WHERE run_id=? AND attempt_id=? AND record_id=?`, run, attempt, id).Scan(&b)
	if err != nil {
		return execution.Operation{}, persistenceError(err)
	}
	var op execution.Operation
	if err = decode(b, &op); err != nil {
		return op, fmt.Errorf("invalid operation record; preserve it: %w", err)
	}
	if op.Version != execution.ExecutionRecordVersion || op.RunID != run || op.AttemptID != attempt || op.ID != id {
		return op, errors.New("operation identity/version mismatch")
	}
	if err := execution.ValidateExecutionOperation(op); err != nil {
		return op, fmt.Errorf("invalid operation record; preserve it: %w", err)
	}
	return op, nil
}
