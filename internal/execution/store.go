package execution

import (
	"context"
	"errors"
	"fmt"
	"sync"

	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// Store serializes publication with heartbeat. SQLite is the only durable
// runtime store.
type Store struct {
	mu      sync.Mutex
	Domain  persistence.WorkerMetadata
	Project string
	failure error
}

func (s *Store) commitWorker(request persistence.WorkerCommit) (persistence.WorkerCommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		return persistence.WorkerCommitResult{}, s.failure
	}
	if s.Domain == nil {
		return persistence.WorkerCommitResult{}, errors.New("worker commitment requires a durable store")
	}
	result, err := s.Domain.CommitWorker(context.Background(), request)
	var conflict *taskmodel.Error
	if err != nil && !errors.As(err, &conflict) {
		s.failure = fmt.Errorf("critical worker persistence: %w", err)
		return result, s.failure
	}
	return result, err
}

func (s *Store) Publish(checkpoint *contracts.ServiceCheckpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		return s.failure
	}
	if checkpoint == nil {
		return errors.New("service checkpoint is required")
	}
	value := *checkpoint
	value.UpdatedAt, value.HeartbeatAt = now(), now()
	if s.Domain != nil {
		committed, err := s.Domain.SaveCheckpoint(context.Background(), value)
		if err != nil {
			s.failure = fmt.Errorf("critical checkpoint persistence: %w", err)
			return s.failure
		}
		value = committed
	}
	*checkpoint = value
	return nil
}

func (s *Store) Heartbeat() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil || s.Domain == nil {
		return s.failure
	}
	if err := s.Domain.Heartbeat(context.Background(), s.Project); err != nil {
		s.failure = fmt.Errorf("database projection heartbeat failed: %w", err)
		return s.failure
	}
	return nil
}

func (s *Store) Err() error { s.mu.Lock(); defer s.mu.Unlock(); return s.failure }
