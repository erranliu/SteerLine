package execution

import (
	"context"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"testing"
)

func TestThreeLayerAtomicCommit(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "commit_and_replay"
		if fail {
			name = "checkpoint_rollback"
		}
		t.Run(name, func(t *testing.T) {
			s, run, port := workerStageFixture(t)
			before, err := readServiceSnapshot(context.Background(), s.domainStore, s.Config.identity)
			require(t, err)
			evidence := workerStageEvidence(s, run, "original implementation result", true)
			if fail {
				_, err = testSQLite(s).db.Exec(`CREATE TRIGGER reject_three_layer_checkpoint BEFORE UPDATE ON projection BEGIN SELECT RAISE(FAIL,'checkpoint rollback'); END`)
				require(t, err)
			}
			fixed, err := s.workerRecords(run)
			require(t, err)
			acceptErr := s.acceptStageTurn(context.Background(), run, port, fixed, evidence)
			after, err := readServiceSnapshot(context.Background(), s.domainStore, s.Config.identity)
			require(t, err)
			if fail {
				if acceptErr == nil {
					t.Fatal("failed transaction accepted result")
				}
				if recordFingerprint(before.WorkflowRuns) != recordFingerprint(after.WorkflowRuns) || recordFingerprint(before.Executions) != recordFingerprint(after.Executions) || recordFingerprint(before.Operations) != recordFingerprint(after.Operations) {
					t.Fatal("failed combined transition persisted half its writes")
				}
				return
			}
			require(t, acceptErr)
			w, e := s.workflowRecords(run)
			if len(after.WorkflowRuns[0].StageResults) != 1 || len(after.WorkflowRuns[0].StageInputs) != 2 || after.WorkflowRuns[0].StepID != executionmodel.StepReport || after.Executions[0].PendingCommandID == "" || e.PendingCommandID != w.StageInputs[1].CommandID {
				t.Fatal("result, business successor and next intent were not saved together")
			}
			fingerprint := recordFingerprint(after)
			require(t, s.acceptStageTurn(context.Background(), run, port, fixed, evidence))
			replay, err := readServiceSnapshot(context.Background(), s.domainStore, s.Config.identity)
			require(t, err)
			if recordFingerprint(replay) != fingerprint {
				t.Fatal("same original result created duplicate writes")
			}
		})
	}
}
