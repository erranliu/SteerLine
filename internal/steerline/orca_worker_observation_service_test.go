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

// The history/host source consumer remains here until #65. Preserve its original
// source replacement and committed attempt assertions while lifecycle moves out.
func TestCD64NativeSourceReplacementRetainsEvidence(t *testing.T) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{}, "")
	f.prepare(t)
	for i := 0; i < 8; i++ {
		f.poll(t)
	}
	execution := f.service.executionRecord(f.run)
	f.update(t, func(state *fakeNativeOrcaState) { state.EnvironmentMode = false; state.Replies = 1 })
	turn := nativeOrcaCurrentTurn(execution)
	if turn == nil || turn.Purpose != "business" || turn.DispatchID == "" {
		t.Fatal("source fixture lost its original business Dispatch")
	}
	require(t, f.service.readNativeOrcaOutput(context.Background(), f.run, execution, turn))
	require(t, f.service.commitWorker(f.run))
	f.update(t, func(state *fakeNativeOrcaState) { state.ForceSourceChange = true })
	require(t, f.service.readNativeOrcaOutput(context.Background(), f.run, execution, turn))
	require(t, f.service.commitWorker(f.run))
	attempt := f.service.workflowAttempt(f.run, f.run.AttemptID)
	snapshot, err := f.service.domainStore.Query(context.Background(), f.service.Config.identity)
	require(t, err)
	committed := snapshotAttempt(&snapshot, f.run.AttemptID)
	if committed == nil || committed.OutputSource != "transcript" || committed.OutputCursor == "" {
		t.Fatal("source replacement did not commit its original attempt evidence")
	}

	if attempt == nil || attempt.OutputSource != "transcript" || turn.OutputSource != "transcript" || !turn.OutputComplete || turn.OutputCursor == "" || !f.state(t).TranscriptAfterSwitch {
		t.Fatalf("source replacement lost native content, completeness or the committed attempt cursor: attempt=%+v turn=%+v", attempt, turn)
	}
}
