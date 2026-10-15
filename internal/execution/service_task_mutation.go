package execution

import "context"

// taskMutationSignal serializes the existing task application's transactions
// with execution commits. It adds no task rules or alternate writer.
type taskMutationSignal struct {
	run  func(context.Context) error
	done chan error
}

func (s *Service) ExecuteTaskMutation(ctx context.Context, run func(context.Context) error) error {
	response := make(chan error, 1)
	request := &taskMutationSignal{run: run, done: response}
	s.signalMu.RLock()
	queue, actor := s.signalQueue, s.runContext
	if queue == nil || actor == nil || actor.Err() != nil {
		s.signalMu.RUnlock()
		return nativeErr("service_not_ready", "project actor is not accepting task commands")
	}
	select {
	case queue <- Signal{mutation: request}:
		s.signalMu.RUnlock()
	case <-ctx.Done():
		s.signalMu.RUnlock()
		return ctx.Err()
	case <-actor.Done():
		s.signalMu.RUnlock()
		return nativeErr("service_not_ready", "project actor stopped")
	}
	select {
	case err := <-response:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-actor.Done():
		return nativeErr("outcome_unknown", "original task command requires read-only reconciliation")
	}
}

func (s *Service) handleTaskMutation(request *taskMutationSignal) error {
	_, ctx := s.signalTransport()
	if ctx == nil || ctx.Err() != nil || !s.State.Service.Ready {
		request.done <- nativeErr("service_not_ready", "project actor is stopping")
		return nil
	}
	err := request.run(ctx)
	// Publication failures must retain the original committed command receipt.
	if refresh := s.refreshNativeSnapshot(ctx); refresh != nil {
		request.done <- refresh
		return refresh
	}
	if checkpoint := s.publishCheckpoint(); checkpoint != nil {
		request.done <- checkpoint
		return checkpoint
	}
	request.done <- err
	return nil
}
