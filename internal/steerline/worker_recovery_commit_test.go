package steerline

import (
	"context"
	"testing"
)

func TestWorkerRecoveryAttemptCommitsAtomically(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "checkpoint_failure"}[fail], func(t *testing.T) {
			service, run := waitingDecisionService(t)
			execution := service.executionRecord(run)
			execution.RecoveryRequired, execution.RecoveryState = true, "interrupted"
			require(t, service.commitWorker(run))
			oldID := run.AttemptID
			old := service.workflowAttempt(run, oldID)
			old.Status, old.UpdatedAt = "stopped", now()
			next := *old
			next.AttemptID, next.AttemptNumber, next.Status = newID(), old.AttemptNumber+1, "planned"
			next.CreatedAt, next.UpdatedAt = now(), now()
			service.domain.Attempts = append(service.domain.Attempts, next)
			execution.CurrentAttemptID = next.AttemptID
			execution.RecoveryRequired, execution.RecoveryState = false, "resuming"
			service.setRunDecision(run, nil)
			service.refreshRun(run)
			if fail {
				_, err := service.domainStore.db.Exec("CREATE TRIGGER reject_recovery_checkpoint BEFORE UPDATE ON projection BEGIN SELECT RAISE(FAIL, 'recovery checkpoint failure'); END")
				require(t, err)
			}
			err := service.commitWorker(run)
			if (err != nil) != fail {
				t.Fatalf("unexpected recovery commitment result: %v", err)
			}
			snapshot, err := service.domainStore.Query(context.Background(), service.Config.identity)
			require(t, err)
			current := snapshot.Executions[0]
			if fail {
				if len(snapshot.Attempts) != 1 || current.CurrentAttemptID != oldID || !current.RecoveryRequired || run.AttemptID != oldID {
					t.Fatal("failed recovery exposed a successor or retired its original Attempt")
				}
			} else if len(snapshot.Attempts) != 2 || current.CurrentAttemptID != next.AttemptID || current.RecoveryRequired || snapshotAttempt(&snapshot, oldID).Status != "stopped" {
				t.Fatal("recovery did not atomically retire and select its worker generation")
			}
		})
	}
}
