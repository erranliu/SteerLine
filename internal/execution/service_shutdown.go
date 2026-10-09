package execution

import (
	"context"
	"errors"
	"fmt"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func (s *Service) shutdown() {
	defer func() {
		for runID, owned := range s.ownedProcesses {
			s.interruptOwnedLocalTurn(runID, owned)
			owned.Close()
			delete(s.ownedProcesses, runID)
		}
	}()
	_, actorContext := s.signalTransport()
	if actorContext != nil && actorContext.Err() != nil {
		s.forwarders.Wait()
		s.drainWorkerMessages()
	}
	for _, messages := range s.lifecycleMessages {
		for _, message := range messages {
			if message.event != nil {
				rejectUndeliveredWorkerEvent(*message.event, context.Canceled)
			}
		}
	}
	s.lifecycleMessages, s.lifecycleEffects = nil, nil

	queue := s.clearSignalTransport()
	s.drainSignalQueue(queue)
	for len(s.workers) != 0 {
		var runID string
		for id := range s.workers {
			runID = id
			break
		}
		var run *reconstructiontypes.RunView
		for _, candidate := range s.runs {
			if candidate != nil && candidate.RunID == runID {
				run = candidate
				break
			}
		}
		if run == nil {
			s.setError(errors.New("active worker has no fixed workflow run"))
			return
		}
		_, err := s.executionConfigForRun(run)
		if err != nil {
			s.setError(err)
			return
		}
		stoppingWorker := s.workers[runID]
		ctx, cancel := context.WithTimeout(context.Background(), s.runtimeSettings.ShutdownGrace)
		err = stoppingWorker.detachLocalObservation(ctx)
		cancel()
		if err != nil {
			s.setError(fmt.Errorf("original worker observation could not be detached: %w", err))
			return
		}
		if s.workers[runID] == stoppingWorker {
			delete(s.workers, runID)
		}
	}
}
