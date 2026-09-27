package steerline

import (
	"context"
	"errors"
)

// workerMessage is delivered to the single service state machine. Worker
// observers may run concurrently, but only that state machine changes runs.
type workerMessage struct {
	runID  string
	worker *Worker
	event  *WorkerEvent
	done   bool
	err    error
}

func (s *Service) registerWorker(run *Run, worker *Worker) {
	if run == nil || worker == nil {
		return
	}
	if s.workers == nil {
		s.workers = make(map[string]*Worker)
	}
	s.workers[run.RunID] = worker
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

func (s *Service) selectWorker(run *Run) error {
	if run == nil {
		return errors.New("worker run is missing")
	}
	worker := s.workers[run.RunID]
	if worker == nil {
		return errors.New("worker identity is no longer active")
	}
	config, err := s.executionConfigForRun(run)
	if err != nil {
		return err
	}
	s.Config, s.active, s.worker = config, run, worker
	return nil
}

func (s *Service) parkWorker() {
	s.worker, s.active = nil, nil
	s.Config = s.projectExecutionConfig()
}

func (s *Service) handleWorkerMessage(message workerMessage) error {
	if s.workers[message.runID] != message.worker {
		return nil // A late observer message cannot change a replacement attempt.
	}
	var run *Run
	for _, candidate := range s.State.Runs {
		if candidate != nil && candidate.RunID == message.runID {
			run = candidate
			break
		}
	}
	if err := s.selectWorker(run); err != nil {
		return err
	}
	defer s.parkWorker()
	if message.event != nil {
		if message.event.RunID != "" && message.event.RunID != message.runID {
			return errors.New("worker event belongs to another workflow run")
		}
		return s.event(*message.event)
	}
	if !message.done {
		return nil
	}
	delete(s.workers, message.runID)
	return s.finishWorkerInLoop(message.err)
}
