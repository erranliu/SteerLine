package steerline

import (
	"bytes"
	"context"
	"testing"
)

func TestWorkerFeedbackPhaseCommitIsDurableAndRejectsStaleActor(t *testing.T) {
	service, run := waitingDecisionService(t)
	t.Cleanup(func() { _ = service.Close() })
	ctx := context.Background()
	workflow, execution := service.workflowRecords(run)
	workflow.Phase, workflow.PRURL, workflow.PRState = "waiting_pr_merge", "https://git.example/org/repo/pulls/8", "open"
	require(t, service.commitWorker(run))
	decision := service.decisionRecord(run)
	answer := DecisionAnswer{DecisionID: decision.DecisionID, RunID: run.RunID, Note: "continue", AnsweredBy: "operator", AnsweredAt: now(), ExpectedDecisionVersion: decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	commands := CommandService{Store: service.domainStore, Project: service.Config.identity}
	command, _, err := commands.AnswerDecisionWithID(ctx, newID(), decision.DecisionID, decision.Version, decisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	_, _, err = commands.AcceptAnswer(ctx, command, answer)
	require(t, err)
	require(t, service.refreshNativeSnapshot(ctx))
	workflow, execution = service.workflowRecords(run)
	decision = service.decisionRecord(run)
	expectedWorkflow, expectedExecution, expectedDecision := *workflow, *execution, *decision
	checkpoint, _, err := service.domainStore.LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	_, beforeRevision, err := service.domainStore.DatabaseVersion(ctx)
	require(t, err)
	baseConfig := service.baseConfig
	service.baseConfig.Host = "orca"
	beforeOperations := len(service.domain.Operations)
	// Later default changes must not retarget a previously frozen execution.
	fixedConfig, fixedErr := service.executionConfigForRun(run)
	service.baseConfig = baseConfig
	if fixedErr != nil || fixedConfig.Host != expectedWorkflow.ExecutionProfile.Host {
		t.Fatalf("mutable defaults changed frozen feedback target: %v", fixedErr)
	}
	_, rejectedRevision, err := service.domainStore.DatabaseVersion(ctx)
	require(t, err)
	if rejectedRevision != beforeRevision || workflow.Phase != "waiting_pr_merge" || len(service.domain.Operations) != beforeOperations {
		t.Fatal("configuration drift changed the workflow or created an operation")
	}

	nextWorkflow := expectedWorkflow
	require(t, BeginWorkerFeedbackDelivery(&nextWorkflow, expectedExecution, expectedDecision, now()))
	attempt := *service.workflowAttempt(run, run.AttemptID)
	request := workerCommit{Project: service.Config.Identity(), TaskID: run.DomainTaskID, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, ControlRevision: expectedWorkflow.ControlRevision,
		Workflow: &recordWrite[WorkflowRunRecord]{Before: &expectedWorkflow, After: nextWorkflow}, Execution: &recordWrite[ExecutionRecord]{Before: &expectedExecution, After: expectedExecution}, Attempt: &recordWrite[StepAttemptRecord]{Before: &attempt, After: attempt}, Decisions: []recordWrite[DecisionRecord]{{Before: &expectedDecision, After: expectedDecision}}, Checkpoint: runtimeCheckpoint(service.State)}
	result, err := service.domainStore.commitWorker(ctx, request)
	committed := nextWorkflow
	committed.ControlRevision = result.ControlRevision
	require(t, err)
	if committed.Phase != "waiting_decision" || committed.ControlRevision != expectedWorkflow.ControlRevision+1 {
		t.Fatalf("feedback phase was not committed once: %+v", committed)
	}
	loaded, err := LoadDatabaseRuntime(service.Config)
	require(t, err)
	if len(loaded.WorkflowRuns) != 1 || loaded.WorkflowRuns[0].Phase != "waiting_decision" || loaded.Runs[0].Decision.Status != "answer_saved" {
		t.Fatalf("coherent read missed the committed feedback phase: %+v", loaded.Runs)
	}
	afterCheckpoint, _, err := service.domainStore.LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	if bytes.Equal(checkpoint, afterCheckpoint) {
		t.Fatal("feedback phase did not publish its checkpoint atomically")
	}
	_, committedRevision, err := service.domainStore.DatabaseVersion(ctx)
	require(t, err)
	if committedRevision != beforeRevision+1 {
		t.Fatalf("feedback phase commit advanced database revision %d -> %d", beforeRevision, committedRevision)
	}

	_, err = service.domainStore.commitWorker(ctx, request)
	if !isNativeErrorCode(err, "stale_revision") {
		t.Fatalf("stale feedback transition was accepted: %v", err)
	}
	if err = service.applyWorkerFlowDecision(ctx, run); !isNativeErrorCode(err, "stale_revision") {
		t.Fatalf("stale actor reached feedback delivery: %v", err)
	}
	if len(service.domain.Operations) != beforeOperations || service.workflowRecord(run).Phase != "waiting_pr_merge" {
		t.Fatal("stale feedback transition changed actor state or started an operation")
	}
	_, afterRevision, err := service.domainStore.DatabaseVersion(ctx)
	require(t, err)
	if afterRevision != committedRevision {
		t.Fatal("stale feedback transition changed the database")
	}
}
