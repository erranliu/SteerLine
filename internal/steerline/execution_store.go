package steerline

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
)

// Capability evidence files stay with the center; durable bindings and receipts
// are loaded as typed records through the injected storage port.
func (r executionRecords) openReader() (persistence.Connection, error) {
	if r.persistence == nil {
		return nil, errors.New("persistence factory is required")
	}
	path := filepath.Join(r.root, "steerline.db")
	reader, err := r.persistence.OpenReader(path)
	if err != nil {
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			return nil, os.ErrNotExist
		}
	}
	return reader, err
}
func (r executionRecords) loadBinding(run, attempt string) (executionapi.ExecutionRef, error) {
	if !executionapi.StableExecutionID(run) || !executionapi.StableExecutionID(attempt) {
		return executionapi.ExecutionRef{}, errors.New("invalid execution binding path identity")
	}
	reader, err := r.openReader()
	if err != nil {
		return executionapi.ExecutionRef{}, err
	}
	defer reader.Close()
	value, err := reader.ExecutionBinding(context.Background(), run, attempt)
	if errors.Is(err, persistence.ErrNotFound) {
		err = os.ErrNotExist
	}
	return value, err
}
func (r executionRecords) loadOperation(run, attempt, id string) (executionapi.Operation, error) {
	if !executionapi.StableExecutionID(run) || !executionapi.StableExecutionID(attempt) || !executionapi.StableOperationID(id) {
		return executionapi.Operation{}, errors.New("invalid operation path identity")
	}
	reader, err := r.openReader()
	if err != nil {
		return executionapi.Operation{}, err
	}
	defer reader.Close()
	value, err := reader.ExecutionOperation(context.Background(), run, attempt, id)
	if errors.Is(err, persistence.ErrNotFound) {
		err = os.ErrNotExist
	}
	return value, err
}
