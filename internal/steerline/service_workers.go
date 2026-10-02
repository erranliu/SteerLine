package steerline

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// workerMessage is delivered to the single service state machine. Worker
// observers may run concurrently, but only that state machine changes runs.
type workerMessage struct {
	runID    string
	worker   *Worker
	event    *WorkerEvent
	done     bool
	err      error
	launch   *appServerLaunchMessage
	reply    *lifecycleReplyMessage
	recovery *appServerRecoveryResult
}

type appServerLaunchMessage struct {
	phase    string
	request  executionapi.LifecycleRequest
	receipt  executionapi.LifecycleReceipt
	observer executionapi.LifecycleObserver
	worker   *Worker
	err      error
	ack      chan appServerLaunchAck
}

type appServerLaunchAck struct {
	Proceed bool
	Request executionapi.LifecycleRequest
}

type lifecycleReplyMessage struct {
	request executionapi.LifecycleRequest
	receipt executionapi.LifecycleReceipt
	err     error
}

func (s *Service) registerWorker(run *Run, worker *Worker) {
	if run == nil || worker == nil {
		return
	}
	if s.workers == nil {
		s.workers = make(map[string]*Worker)
	}
	s.workers[run.RunID] = worker
	if owned, ok := worker.proc.(executionapi.OwnedProcessHandle); ok && owned.OwnedProcess() {
		if s.ownedProcesses == nil {
			s.ownedProcesses = make(map[string]executionapi.StopHandle)
		}
		s.ownedProcesses[run.RunID] = owned
	}
	if s.workerMessages != nil {
		s.forwardWorker(s.runContext, run.RunID, worker)
	}
}

func (s *Service) forwardWorker(ctx context.Context, runID string, worker *Worker) {
	s.forwarders.Add(1)
	go func() {
		defer s.forwarders.Done()
		for {
			var value WorkerEvent
			var ok bool
			select {
			case <-ctx.Done():
				return
			case value, ok = <-worker.Events:
				if !ok {
					goto finishedEvents
				}
			}
			if ctx.Err() != nil {
				rejectUndeliveredWorkerEvent(value, ctx.Err())
				return
			}
			select {
			case s.workerMessages <- workerMessage{runID: runID, worker: worker, event: &value}:
			case <-ctx.Done():
				rejectUndeliveredWorkerEvent(value, ctx.Err())
				return
			}
		}
	finishedEvents:
		if ctx.Err() != nil {
			return
		}
		var err error
		select {
		case err = <-worker.Done:
		case <-ctx.Done():
			return
		}
		message := workerMessage{runID: runID, worker: worker, done: true, err: err}
		select {
		case s.workerMessages <- message:
		case <-ctx.Done():
			select {
			case s.workerMessages <- message:
			default:
				s.forwardMu.Lock()
				s.forwardedDone = append(s.forwardedDone, message)
				s.forwardMu.Unlock()
			}
		}
	}()
}

func rejectUndeliveredWorkerEvent(event WorkerEvent, err error) {
	ackWorkerEventPersistence(event, err)
	if event.Processed != nil {
		select {
		case event.Processed <- err:
		default:
		}
		close(event.Processed)
	}
}

func (s *Service) drainWorkerMessages() {
	for s.workerMessages != nil {
		select {
		case message := <-s.workerMessages:
			if err := s.handleWorkerMessage(message); err != nil {
				s.setError(err)
			}
		default:
			goto forwarded
		}
	}
forwarded:
	s.forwardMu.Lock()
	pending := s.forwardedDone
	s.forwardedDone = nil
	s.forwardMu.Unlock()
	for _, message := range pending {
		if err := s.handleWorkerMessage(message); err != nil {
			s.setError(err)
		}
	}
}

func (s *Service) handleWorkerMessage(message workerMessage) error {
	if message.recovery != nil {
		delete(s.appServerRecovering, message.runID)
		return s.applyAppServerRecovery(*message.recovery)
	}
	if message.launch != nil {
		return s.handleAppServerLaunchMessage(message.runID, message.launch)
	}
	if message.reply != nil {
		return s.handleLifecycleFeedbackMessage(message.runID, message.reply)
	}
	if s.workers[message.runID] != message.worker {
		if message.event != nil {
			rejectUndeliveredWorkerEvent(*message.event, ErrWorkerTurnStale)
		}
		return nil // A late observer message cannot change a replacement attempt.
	}
	var run *Run
	for _, candidate := range s.runs {
		if candidate != nil && candidate.RunID == message.runID {
			run = candidate
			break
		}
	}
	if run == nil || run.RunID != message.runID {
		return errors.New("worker message has no fixed workflow run")
	}
	if message.event != nil {
		if message.event.RunID != "" && message.event.RunID != message.runID {
			return errors.New("worker event belongs to another workflow run")
		}
		return s.eventForRun(run, *message.event)
	}
	if !message.done {
		return nil
	}
	delete(s.workers, message.runID)
	return s.finishWorkerInLoop(run, message.worker, message.err)
}
