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
	decision := service.decisionRecord(run)
	answer := DecisionAnswer{DecisionID: decision.DecisionID, RunID: run.RunID, Note: "continue", AnsweredBy: "operator", AnsweredAt: now(), ExpectedDecisionVersion: decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	decision.Answer, decision.AnsweredBy, decision.Status, decision.UpdatedAt = mustJSON(answer), answer.AnsweredBy, "answer_saved", now()
	decision.Version++
	require(t, service.save())
	workflow, execution = service.workflowRecords(run)
	decision = service.decisionRecord(run)
	expectedWorkflow, expectedExecution, expectedDecision := *workflow, *execution, *decision
	checkpoint, _, err := service.domainStore.LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	_, beforeRevision, err := service.domainStore.DatabaseVersion(ctx)
	require(t, err)

	committed, err := service.domainStore.commitWorkerFeedbackPhase(ctx, expectedWorkflow, expectedExecution, expectedDecision)
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
	if !bytes.Equal(checkpoint, afterCheckpoint) {
		t.Fatal("feedback phase commit rewrote the service checkpoint")
	}
	_, committedRevision, err := service.domainStore.DatabaseVersion(ctx)
	require(t, err)
	if committedRevision != beforeRevision+1 {
		t.Fatalf("feedback phase commit advanced database revision %d -> %d", beforeRevision, committedRevision)
	}

	_, err = service.domainStore.commitWorkerFeedbackPhase(ctx, expectedWorkflow, expectedExecution, expectedDecision)
	if !isNativeErrorCode(err, "stale_revision") {
		t.Fatalf("stale feedback transition was accepted: %v", err)
	}
	beforeOperations := len(service.State.Operations)
	if err = service.applyWorkerFlowDecision(ctx, run); !isNativeErrorCode(err, "stale_revision") {
		t.Fatalf("stale actor reached feedback delivery: %v", err)
	}
	if len(service.State.Operations) != beforeOperations || workflow.Phase != "waiting_pr_merge" {
		t.Fatal("stale feedback transition changed actor state or started an operation")
	}
	_, afterRevision, err := service.domainStore.DatabaseVersion(ctx)
	require(t, err)
	if afterRevision != committedRevision {
		t.Fatal("stale feedback transition changed the database")
	}
}
