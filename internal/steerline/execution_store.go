package steerline

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
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

func (r executionRecords) loadBinding(run, attempt string) (executionbackend.ExecutionRef, error) {
	if !executionbackend.StableExecutionID(run) || !executionbackend.StableExecutionID(attempt) {
		return executionbackend.ExecutionRef{}, errors.New("invalid execution binding path identity")
	}
	reader, err := r.openReader()
	if err != nil {
		return executionbackend.ExecutionRef{}, err
	}
	defer reader.Close()
	value, err := reader.ExecutionBinding(context.Background(), run, attempt)
	if errors.Is(err, storageports.ErrNotFound) {
		err = os.ErrNotExist
	}
	return value, err
}

func (r executionRecords) loadOperation(run, attempt, id string) (executionbackend.Operation, error) {
	if !executionbackend.StableExecutionID(run) || !executionbackend.StableExecutionID(attempt) || !executionbackend.StableOperationID(id) {
		return executionbackend.Operation{}, errors.New("invalid operation path identity")
	}
	reader, err := r.openReader()
	if err != nil {
		return executionbackend.Operation{}, err
	}
	defer reader.Close()
	value, err := reader.ExecutionOperation(context.Background(), run, attempt, id)
	if errors.Is(err, storageports.ErrNotFound) {
		err = os.ErrNotExist
	}
	return value, err
}
