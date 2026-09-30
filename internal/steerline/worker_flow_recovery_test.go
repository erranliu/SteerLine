package steerline

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDeadWorkerRecoveryPreservesDecisionAndFreshAnswer(t *testing.T) {
	service, run := appServerAuditService(t)
	cmd := exec.Command(os.Args[0], "--fake", "sleep")
	owned, err := startProcess(cmd)
	require(t, err)
	identity := owned.Identity()
	owned.Close()
	_ = cmd.Wait()
	if !workerTreeAbsent(identity) {
		t.Fatal("isolated worker tree did not exit")
	}
	workflow, execution := service.workflowRecords(run)
	execution.Process = &identity
	execution.SessionID = workerFlowMockSession
	execution.CurrentTurnID = "00000000-0000-0000-0000-000000000001"
	execution.CurrentCommandID, execution.TurnNumber = newID(), 1
	execution.Status, workflow.Phase = "active", "executing"
	service.refreshRun(run)
	service.markServiceRecoveryGate()
	require(t, service.save())
	result := appServerRecoveryResult{runID: run.RunID, attemptID: run.AttemptID, execution: *execution, dead: true}
	require(t, service.applyAppServerRecovery(result))
	if run.Decision == nil || run.Decision.Status != "pending" || execution.RecoveryState != "interrupted" {
		t.Fatal("dead worker did not create a recovery decision")
	}
	decisionID, decisionVersion, count := run.Decision.ID, run.Decision.Version, len(service.State.Decisions)
	for i := 0; i < 2; i++ {
		require(t, service.applyAppServerRecovery(result))
		if run.Decision.ID != decisionID || run.Decision.Version != decisionVersion || len(service.State.Decisions) != count {
			t.Fatal("repeated recovery probe replaced the displayed decision")
		}
	}
	service.State.Service.Ready = true
	require(t, service.save())
	answer := DecisionAnswer{DecisionID: decisionID, RunID: run.RunID, OptionID: "continue", Note: "resume after inspecting files", AnsweredBy: "operator", ExpectedDecisionVersion: decisionVersion, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID, SourceAttemptID: run.AttemptID, ExpectedRecoveryVersion: execution.RecoveryVersion}
	commands := CommandService{Store: service.domainStore, Project: service.Config.identity}
	command, _, err := commands.AnswerDecisionWithID(context.Background(), newID(), decisionID, decisionVersion, decisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	record, _, err := commands.AcceptAnswer(context.Background(), command, answer)
	require(t, err)
	*service.decisionRecord(run) = record
	service.store.baseline["decision:"+record.DecisionID] = recordFingerprint(record)
	service.refreshRun(run)
	require(t, service.save())
	require(t, service.applyAppServerRecovery(result))
	if run.Decision.ID != decisionID || run.Decision.Status != "answer_saved" || len(service.State.Decisions) != count {
		t.Fatal("recovery probe superseded the fresh manual answer")
	}
	result.attemptID = newID() // An observer from a different generation is late.
	result.execution.Process = nil
	require(t, service.applyAppServerRecovery(result))
	if execution.Process == nil || *execution.Process != identity || run.Decision.ID != decisionID {
		t.Fatal("late recovery result changed the current Attempt")
	}
	_, err = service.startOperation("publish_local_worker", "local-publish:"+run.RunID+":"+execution.CurrentTurnID, map[string]string{"run_id": run.RunID})
	require(t, err)
	require(t, service.applyWorkerFlowDecision(context.Background(), run))
	if !execution.RecoveryRequired || execution.RecoveryState != "unverified" || len(service.State.Attempts) != 1 ||
		execution.Error == nil || !strings.Contains(*execution.Error, "external operation has an unknown outcome") {
		t.Fatal("WorkflowRun publication uncertainty was bypassed by manual recovery")
	}
}
