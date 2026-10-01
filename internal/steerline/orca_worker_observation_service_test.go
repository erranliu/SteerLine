package steerline

import (
	"context"
	"testing"
)

func TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal(t *testing.T) {
	testManagedLaunchRecovery(t, false)
}

func TestNativeOrcaFixedLunaLaunchReconcilesRenamedTerminalAfterRestart(t *testing.T) {
	testManagedLaunchRecovery(t, true)
}

func enableTestLifecycleMessages(s *Service) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	s.runContext = ctx
	s.workerMessages = make(chan workerMessage, 128)
	return cancel
}

func stopTestLifecycleMessages(s *Service, cancel context.CancelFunc) {
	if s != nil {
		for _, worker := range s.workers {
			if worker != nil && worker.cancelObservation != nil {
				worker.cancelObservation()
			}
		}
	}
	if cancel != nil {
		cancel()
	}
	if s != nil {
		s.forwarders.Wait()
	}
}
