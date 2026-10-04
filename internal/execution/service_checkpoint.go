package execution

import (
	"context"
)

func (s *Service) publishCheckpoint() error {
	if err := s.projectState(); err != nil {
		return err
	}
	checkpoint := runtimeCheckpoint(s.State)
	if err := s.store.Publish(&checkpoint); err != nil {
		return err
	}
	s.State.Revision, s.State.UpdatedAt, s.State.HeartbeatAt = checkpoint.Revision, checkpoint.UpdatedAt, checkpoint.HeartbeatAt
	return s.refreshNativeSnapshot(context.Background())
}
