package steerline

import (
	"context"
	"path/filepath"
	"testing"
)

func TestResumingDecisionPublishKeepsAcceptedAnswerInSQLite(t *testing.T) {
	ctx := context.Background()
	db := openDomainTestStore(t)
	project, runID, decisionID := "repo:resuming-answer", newID(), newID()
	originalID, continuationID := newID(), newID()
	seedNativeDecision(t, db, project, runID, newID())
	request := DecisionRequest{Question: "Proceed?", Options: []DecisionOption{{ID: "yes", Label: "Yes"}}}
	answer := DecisionAnswer{DecisionID: decisionID, RunID: runID, OptionID: "yes", AnsweredBy: "tester", AnsweredAt: now()}
	if err := db.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutStepAttempt(StepAttemptRecord{ProjectIdentity: project, AttemptID: originalID, ExecutionID: runID, StepID: "implement", AttemptNumber: 1, Status: "succeeded"}); err != nil {
			return err
		}
		if err := tx.PutStepAttempt(StepAttemptRecord{ProjectIdentity: project, AttemptID: continuationID, ExecutionID: runID, StepID: "implement", AttemptNumber: 2, Status: "outcome_unknown"}); err != nil {
			return err
		}
		return tx.PutDecision(DecisionRecord{ProjectIdentity: project, DecisionID: decisionID, Scope: "execution", WorkflowRunID: runID, ExecutionID: runID, AttemptID: originalID, QuestionID: decisionID, Request: mustJSON(request), Answer: mustJSON(answer), Status: "answer_saved", Version: 3})
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.Query(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	decision := &DecisionState{ID: decisionID, AttemptID: originalID, Version: 3, Request: request, Status: "resuming", OptionID: "yes", AnsweredBy: "tester", AnsweredAt: &answer.AnsweredAt}
	run := &Run{TaskID: "1", DomainTaskID: "1", RunID: runID, ExecutionID: runID, AttemptID: continuationID, WorkflowVersion: MinimalWorkflowVersion, ExecutionStatus: "unknown", FinishTaskStatus: "blocked", Decision: decision}
	runtime := Runtime{SchemaVersion: 5, Repository: Repository{Identity: project, Path: "repo", Remote: "origin", Branch: "main"}, Service: ServiceState{InstanceID: "test-instance", Status: "running", Ready: true}, Runs: []*Run{run}, Tasks: snapshot.Tasks, WorkflowRuns: snapshot.WorkflowRuns, Executions: snapshot.Executions, Attempts: snapshot.Attempts, Decisions: snapshot.Decisions, Control: ControlState{DispatchMode: "manual", Revision: 1}}
	store := &Store{Path: filepath.Join(t.TempDir(), "runtime.json"), Domain: db, Project: project}
	if err := store.Publish(&runtime); err != nil {
		t.Fatal(err)
	}
	saved, err := db.Query(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range saved.Decisions {
		if record.DecisionID != decisionID {
			continue
		}
		var got DecisionAnswer
		if record.Status != "answer_saved" || decode(record.Answer, &got) != nil || !sameDecisionAnswer(got, answer) {
			t.Fatalf("resuming projection discarded its accepted answer: %+v", record)
		}
		return
	}
	t.Fatal("original Decision disappeared after resuming projection")
}
