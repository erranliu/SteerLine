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
	execution.PolicyResources = policyResourceReceiptFixture(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	execution.Process = &identity
	setTestCodexBinding(t, execution, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	execution.CurrentTurnID = "00000000-0000-0000-0000-000000000001"
	execution.CurrentCommandID, execution.TurnNumber = newID(), 1
	execution.Status, workflow.Phase = "active", "executing"
	service.refreshRun(run)
	service.markServiceRecoveryGate()
	require(t, service.commitWorker(run))
	result := appServerRecoveryResult{runID: run.RunID, attemptID: run.AttemptID, execution: *execution, dead: true}
	require(t, service.applyAppServerRecovery(result))
	if run.Decision == nil || run.Decision.Status != "pending" || execution.RecoveryState != "interrupted" {
		t.Fatal("dead worker did not create a recovery decision")
	}
	decisionID, decisionVersion, count := run.Decision.ID, run.Decision.Version, len(service.domain.Decisions)
	for i := 0; i < 2; i++ {
		require(t, service.applyAppServerRecovery(result))
		if run.Decision.ID != decisionID || run.Decision.Version != decisionVersion || len(service.domain.Decisions) != count {
			t.Fatal("repeated recovery probe replaced the displayed decision")
		}
	}
	service.State.Service.Ready = true
	require(t, service.commitWorker(run))
	answer := DecisionAnswer{DecisionID: decisionID, RunID: run.RunID, OptionID: "continue", Note: "resume after inspecting files", AnsweredBy: "operator", ExpectedDecisionVersion: decisionVersion, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID, SourceAttemptID: run.AttemptID, ExpectedRecoveryVersion: execution.RecoveryVersion}
	commands := CommandService{Store: service.domainStore, Project: service.Config.identity}
	command, _, err := commands.AnswerDecisionWithID(context.Background(), newID(), decisionID, decisionVersion, decisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	record, _, err := commands.AcceptAnswer(context.Background(), command, answer)
	require(t, err)
	*service.decisionRecord(run) = record
	require(t, service.refreshNativeSnapshot(context.Background()))
	execution = service.executionRecord(run)
	service.refreshRun(run)
	require(t, service.commitWorker(run))
	require(t, service.applyAppServerRecovery(result))
	if run.Decision.ID != decisionID || run.Decision.Status != "answer_saved" || len(service.domain.Decisions) != count {
		t.Fatal("recovery probe superseded the fresh manual answer")
	}
	result.attemptID = newID() // An observer from a different generation is late.
	result.execution.Process = nil
	require(t, service.applyAppServerRecovery(result))
	if execution.Process == nil || *execution.Process != identity || run.Decision.ID != decisionID {
		t.Fatal("late recovery result changed the current Attempt")
	}
	_, err = service.startWorkerOperation(run, "publish_local_worker", "local-publish:"+run.RunID+":"+execution.CurrentTurnID, map[string]string{"run_id": run.RunID})
	require(t, err)
	require(t, service.applyWorkerFlowDecision(context.Background(), run))
	if !execution.RecoveryRequired || execution.RecoveryState != "unverified" || len(service.domain.Attempts) != 1 ||
		execution.Error == nil || !strings.Contains(*execution.Error, "external operation has an unknown outcome") {
		t.Fatal("WorkflowRun publication uncertainty was bypassed by manual recovery")
	}
}
