package execution

import (
	"context"
	"errors"
	"fmt"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type workerServiceOwner struct {
	port     executionbackend.LifecyclePort
	snapshot executionbackend.LifecycleSnapshot
}

// Shutdown delegates resource semantics to the fixed worker. The service only
// drains durable messages and closes its observation; it never interrupts a
// native turn or guesses whether an external execution survives.
func (s *Service) shutdown() error {
	// Final persistence/ack is drained by the actor, not a canceled run queue.
	if s.runContext != nil && s.runContext.Err() != nil {
		s.forwarders.Wait()
		pendingQueue := s.signalQueue
		s.signalQueue = nil
		s.drainWorkerMessagesFrom(pendingQueue)
	} else {
		s.signalQueue = nil
	}
	for _, messages := range s.lifecycleMessages {
		for _, message := range messages {
			if message.event != nil {
				rejectUndeliveredWorkerEvent(*message.event, context.Canceled)
			}
		}
	}
	s.lifecycleMessages, s.lifecycleEffects = nil, nil
	s.signalQueue = nil
	var failures []error
	for _, run := range s.runs {
		if run == nil {
			continue
		}
		worker, owned := s.workers[run.RunID], s.ownedProcesses[run.RunID]
		if worker == nil && owned == nil {
			continue
		}
		port, fixed, err := s.lifecycleForRun(run)
		if err == nil {
			var snapshotErr error
			snapshot, snapshotErr := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
			if snapshotErr != nil {
				err = snapshotErr
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), s.runtimeSettings.ShutdownGrace)
				_, err = port.ShutdownLifecycle(ctx, snapshot, owned)
				cancel()
			}
		}
		if err != nil && owned != nil {
			// An already accepted owner can still release its original resources
			// if later profile/receipt inspection fails. Never discard that handle.
			if owner, ok := s.workerServiceOwners[run.RunID]; ok && (owner.snapshot.Ref.External.Kind == "" || owner.snapshot.Ref.External == owned.Identity()) {
				ctx, cancel := context.WithTimeout(context.Background(), s.runtimeSettings.ShutdownGrace)
				_, cleanupErr := owner.port.ShutdownLifecycle(ctx, owner.snapshot, owned)
				cancel()
				if cleanupErr == nil {
					delete(s.ownedProcesses, run.RunID)
				} else {
					err = errors.Join(err, cleanupErr)
				}
			}
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("worker service stop %s: %w", run.RunID, err))
		}
		if worker != nil {
			ctx, cancel := context.WithTimeout(context.Background(), s.runtimeSettings.ShutdownGrace)
			detachErr := worker.detachLocalObservation(ctx)
			cancel()
			if detachErr != nil {
				failures = append(failures, fmt.Errorf("worker observation detach %s: %w", run.RunID, detachErr))
			}
		}
		delete(s.workers, run.RunID)
		if err == nil {
			delete(s.ownedProcesses, run.RunID)
			delete(s.workerServiceOwners, run.RunID)
		}
	}
	return errors.Join(failures...)
}
