package execution

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (r executionRecords) loadBinding(run, attempt string) (executionbackend.ExecutionRef, error) {
	if !executionbackend.StableExecutionID(run) || !executionbackend.StableExecutionID(attempt) {
		return executionbackend.ExecutionRef{}, errors.New("invalid execution binding path identity")
	}
	if r.reader == nil {
		if _, err := os.Stat(filepath.Join(r.root, "steerline.db")); os.IsNotExist(err) {
			return executionbackend.ExecutionRef{}, os.ErrNotExist
		}
		return executionbackend.ExecutionRef{}, errors.New("execution record reader is required")
	}
	value, err := r.reader.ExecutionBinding(context.Background(), run, attempt)
	if errors.Is(err, storageports.ErrNotFound) {
		err = os.ErrNotExist
	}
	return value, err
}

func (r executionRecords) loadOperation(run, attempt, id string) (executionbackend.Operation, error) {
	if !executionbackend.StableExecutionID(run) || !executionbackend.StableExecutionID(attempt) || !executionbackend.StableOperationID(id) {
		return executionbackend.Operation{}, errors.New("invalid operation path identity")
	}
	if r.reader == nil {
		if _, err := os.Stat(filepath.Join(r.root, "steerline.db")); os.IsNotExist(err) {
			return executionbackend.Operation{}, os.ErrNotExist
		}
		return executionbackend.Operation{}, errors.New("execution record reader is required")
	}
	value, err := r.reader.ExecutionOperation(context.Background(), run, attempt, id)
	if errors.Is(err, storageports.ErrNotFound) {
		err = os.ErrNotExist
	}
	return value, err
}
