package refactoring

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/refactoring/model"
)

// Start checks have no project slot and cannot dispatch Runner work.
func (s *Service) checkStart(ctx, lifecycle context.Context, r model.Record) error {
	if s.StartContext != nil {
		ctx = s.StartContext(ctx)
	}
	err := s.freezeStart(ctx, r)
	if err == nil || lifecycle.Err() != nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = errors.New("启动核验超时，请检查项目和运行环境后重新发起")
	}
	ctx, cancel := context.WithTimeout(lifecycle, advanceTimeout)
	defer cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, readErr := s.Store.Refactoring(ctx, s.Project, r.ID)
	if readErr != nil || current.Phase != model.PhaseCheckingStart {
		return readErr
	}
	current.Phase = "blocked"
	current.Diagnostic = err.Error()
	return s.save(ctx, &current, nil)
}

func (s *Service) freezeStart(ctx context.Context, r model.Record) error {
	if s.Admit != nil {
		if err := s.Admit(ctx); err != nil {
			return err
		}
	}
	frozen, err := s.Prepare(ctx, r.Input)
	if err != nil {
		return err
	}
	save := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		current, err := s.Store.Refactoring(ctx, s.Project, r.ID)
		if err != nil || current.Phase != model.PhaseCheckingStart {
			return err
		}
		current.Frozen = frozen
		current.Phase = "first"
		if frozen.Environment != nil {
			return s.save(ctx, &current, nil, *frozen.Environment)
		}
		return s.save(ctx, &current, nil)
	}
	if s.Reserve != nil {
		return s.Reserve(ctx, frozen.Repository.BaseRevision, save)
	}
	return save()
}
