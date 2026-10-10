package execution

import (
	"context"
	"errors"
	"path/filepath"

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type taskDiscardSignal struct {
	taskID, requestID   string
	expectedVersion     uint64
	reason, requestedBy string
	response            chan taskDiscardResponse
}

type taskDiscardResponse struct {
	receipt taskmodel.TaskDiscardReceipt
	err     error
}

func completeDiscardTask(request *taskDiscardSignal, receipt taskmodel.TaskDiscardReceipt, err error) {
	if request == nil || request.response == nil {
		return
	}
	select {
	case request.response <- taskDiscardResponse{receipt: receipt, err: err}:
	default:
	}
}

// DiscardNativeTask routes the existing SQLite-backed discard command through
// the configured service actor, which already owns the repository lock and
// command store. The request keeps its normal request ID/version/idempotency
// semantics; it does not open a second writer or bypass the lock.
func (s *Service) DiscardNativeTask(ctx context.Context, config Config, taskID, requestID string, expectedVersion uint64, reason, requestedBy string) (taskmodel.TaskDiscardReceipt, error) {
	if s == nil {
		return taskmodel.TaskDiscardReceipt{}, nativeErr("service_not_ready", "configured service is unavailable for task discard")
	}
	if config.Identity() != s.Config.Identity() || filepath.Clean(config.RunDir) != filepath.Clean(s.Config.RunDir) {
		return taskmodel.TaskDiscardReceipt{}, nativeErr("authority_mismatch", "discard caller differs from the configured service project or run_dir")
	}
	if err := ctx.Err(); err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	response := make(chan taskDiscardResponse, 1)
	request := &taskDiscardSignal{taskID: taskID, requestID: requestID, expectedVersion: expectedVersion,
		reason: reason, requestedBy: requestedBy, response: response}

	// Hold the read lock through enqueue so shutdown cannot retire the signal
	// queue before this request is either accepted or rejected.
	s.signalMu.RLock()
	queue, actorContext := s.signalQueue, s.runContext
	if queue == nil || actorContext == nil || actorContext.Err() != nil {
		s.signalMu.RUnlock()
		return taskmodel.TaskDiscardReceipt{}, nativeErr("service_not_ready", "configured service is not accepting task commands")
	}
	select {
	case queue <- Signal{discard: request}:
		s.signalMu.RUnlock()
	case <-ctx.Done():
		s.signalMu.RUnlock()
		return taskmodel.TaskDiscardReceipt{}, ctx.Err()
	case <-actorContext.Done():
		s.signalMu.RUnlock()
		return taskmodel.TaskDiscardReceipt{}, nativeErr("service_not_ready", "configured service stopped before accepting the discard")
	}
	select {
	case result := <-response:
		return result.receipt, result.err
	case <-ctx.Done():
		return taskmodel.TaskDiscardReceipt{}, ctx.Err()
	case <-actorContext.Done():
		return taskmodel.TaskDiscardReceipt{}, nativeErr("service_not_ready", "configured service stopped while the original discard was being reconciled")
	}
}

func (s *Service) handleDiscardTask(request *taskDiscardSignal) error {
	if request == nil {
		return nil
	}
	_, actorContext := s.signalTransport()
	if actorContext == nil || actorContext.Err() != nil || !s.State.Service.Ready {
		completeDiscardTask(request, taskmodel.TaskDiscardReceipt{}, nativeErr("service_not_ready", "configured service is not ready for task discard"))
		return nil
	}
	receipt, discardErr := s.commandService(s.Config.Identity()).DiscardNativeTask(actorContext, s.Config,
		request.taskID, request.requestID, request.expectedVersion, request.reason, request.requestedBy)
	refreshErr := s.reconcile(actorContext)
	if refreshErr != nil {
		s.setError(refreshErr)
	}
	checkpointErr := s.publishCheckpoint()
	completeDiscardTask(request, receipt, errors.Join(discardErr, refreshErr, checkpointErr))
	if refreshErr != nil {
		return refreshErr
	}
	return checkpointErr
}
