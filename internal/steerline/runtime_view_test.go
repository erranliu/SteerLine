package steerline

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func waitingDecisionService(t *testing.T) (*Service, *Run) {
	t.Helper()
	service, run := appServerAuditService(t)
	_, execution := service.workflowRecords(run)
	execution.Status, execution.LastTurnOutcome = "idle", "succeeded"
	service.State.Service.Ready = true
	require(t, service.waitWorkerFlow(run, "await input"))
	return service, run
}

func TestRuntimeViewCannotWriteBusinessState(t *testing.T) {
	service, run := waitingDecisionService(t)
	workflow, execution := service.workflowRecords(run)
	originalIntent := *execution.WorkspaceIntent
	revision := run.ControlRevision
	run.FinishTaskStatus, run.ExecutionStatus, run.Decision.Status = "completed", "failed", "superseded"
	run.WorkspaceIntent.Path = "view-only-path"
	run.PR.URL = ptr("view-only-pr")
	execution.LastEvent = ptr("new diagnostic")
	require(t, service.save())
	if workflow != service.workflowRecord(run) || execution != service.executionRecord(run) {
		t.Fatal("publication replaced actor domain records")
	}
	if run.FinishTaskStatus != "waiting_decision" || run.ExecutionStatus != "succeeded" || run.Decision.Status != "pending" || run.PR.URL != nil || *run.WorkspaceIntent != originalIntent || run.ControlRevision != revision {
		t.Fatalf("view became authority or diagnostic changed revision: %+v", run)
	}
	payload, _, err := service.domainStore.LoadProjection(context.Background(), service.Config.Identity())
	require(t, err)
	for _, key := range []string{`"runs"`, `"executions"`, `"workflow_runs"`, `"dispatch_mode"`, `"slot_occupied"`} {
		if bytes.Contains(payload, []byte(key)) {
			t.Fatalf("checkpoint duplicates business state %s", key)
		}
	}
	require(t, service.store.Heartbeat())
	loaded, err := LoadDatabaseRuntime(service.Config)
	require(t, err)
	if loaded.Runs[0].ControlRevision != revision || loaded.Runs[0].LastEvent == nil || *loaded.Runs[0].LastEvent != "new diagnostic" {
		t.Fatal("coherent runtime read lost domain observation or heartbeat changed revision")
	}
}

func TestAnswerChecksDomainRevisionBeforeCheckpointPublication(t *testing.T) {
	service, run := waitingDecisionService(t)
	ctx := context.Background()
	commands := CommandService{Store: service.domainStore, Project: service.Config.Identity()}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Note: "continue", AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	command, _, err := commands.AnswerDecision(ctx, answer.DecisionID, answer.ExpectedDecisionVersion, newID(), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	before, _, err := service.domainStore.LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	execution := *service.executionRecord(run)
	execution.PendingCommandID = newID()
	require(t, service.domainStore.Transact(ctx, func(tx DomainTx) error { return tx.PutExecution(execution) }))
	after, _, err := service.domainStore.LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	if !bytes.Equal(before, after) {
		t.Fatal("domain update rewrote checkpoint")
	}
	loaded, err := LoadDatabaseRuntime(service.Config)
	require(t, err)
	if loaded.Runs[0].ControlRevision <= answer.ExpectedRunRevision {
		t.Fatal("read ignored durable control revision")
	}
	_, _, err = commands.AcceptAnswer(ctx, command, answer)
	if err == nil || !strings.Contains(err.Error(), "stale_revision") {
		t.Fatalf("answer accepted against stale domain state: %v", err)
	}
	snapshot, err := service.domainStore.Query(ctx, service.Config.Identity())
	require(t, err)
	if snapshot.Decisions[0].Status != "pending" || len(snapshot.Decisions[0].Answer) != 0 {
		t.Fatal("rejected answer changed decision")
	}
}

func TestWorkerCommitPreservesExternallyAcceptedDecision(t *testing.T) {
	service, run := waitingDecisionService(t)
	ctx := context.Background()
	commands := CommandService{Store: service.domainStore, Project: service.Config.Identity()}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Note: "keep this answer", AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	command, _, err := commands.AnswerDecision(ctx, answer.DecisionID, answer.ExpectedDecisionVersion, newID(), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	_, _, err = commands.AcceptAnswer(ctx, command, answer)
	require(t, err)
	service.executionRecord(run).LastEvent = ptr("actor diagnostic after external answer")
	if err := service.commitWorker(run); !isNativeErrorCode(err, "stale_revision") {
		t.Fatalf("stale worker actor did not stop before overwriting the answer: %v", err)
	}
	loaded, err := LoadDatabaseRuntime(service.Config)
	require(t, err)
	if loaded.Runs[0].Decision.Status != "answer_saved" || loaded.Runs[0].Decision.Note != answer.Note {
		t.Fatal("worker commit overwrote the externally saved answer")
	}
}

func TestWorkerCommitRejectsConflictingDecisionAtomically(t *testing.T) {
	service, run := waitingDecisionService(t)
	ctx := context.Background()
	commands := CommandService{Store: service.domainStore, Project: service.Config.Identity()}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Note: "committed answer", AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	command, _, err := commands.AnswerDecision(ctx, answer.DecisionID, answer.ExpectedDecisionVersion, newID(), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	_, _, err = commands.AcceptAnswer(ctx, command, answer)
	require(t, err)
	before, revision, err := service.domainStore.LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	service.setDecisionStatus(run, "superseded")
	service.executionRecord(run).LastEvent = ptr("must roll back")
	if err = service.commitWorker(run); !isNativeErrorCode(err, "stale_revision") {
		t.Fatalf("conflicting worker commit was not rejected: %v", err)
	}
	after, nextRevision, err := service.domainStore.LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	loaded, err := LoadDatabaseRuntime(service.Config)
	require(t, err)
	if !bytes.Equal(before, after) || revision != nextRevision || loaded.Runs[0].Decision.Status != "answer_saved" || loaded.Runs[0].Decision.Note != answer.Note || loaded.Runs[0].LastEvent != nil && *loaded.Runs[0].LastEvent == "must roll back" {
		t.Fatal("conflicting worker commit changed committed state")
	}
}

func TestObservationReportsMissingAuthorityAsCorruption(t *testing.T) {
	service, _ := waitingDecisionService(t)
	_, err := service.domainStore.db.Exec(`DELETE FROM project_task_authorities WHERE project_identity=?`, service.Config.Identity())
	require(t, err)
	value, err := loadObservedRuntime(service.Config)
	if err == nil || value != nil {
		t.Fatalf("missing authority was treated as absent checkpoint: %+v / %v", value, err)
	}
}

func TestRuntimeViewRejectsInvalidWorkerAttempts(t *testing.T) {
	service, run := waitingDecisionService(t)
	for _, count := range []int{0, 2} {
		state := service.State
		state.Attempts = nil
		for i := 0; i < count; i++ {
			state.Attempts = append(state.Attempts, StepAttemptRecord{ExecutionID: run.ExecutionID, StepID: "worker", AttemptID: newID(), AttemptNumber: i + 1})
		}
		if err := projectRuntime(&state); err == nil {
			t.Fatalf("accepted %d worker attempts: %v", count, err)
		}
	}
	state := service.State
	old := state.Attempts[0]
	old.Status = "stopped"
	next := old
	next.AttemptID, next.AttemptNumber, next.Status = newID(), old.AttemptNumber+1, "planned"
	state.Attempts = []StepAttemptRecord{old, next}
	for i := range state.Executions {
		if state.Executions[i].ExecutionID == run.ExecutionID {
			state.Executions[i].CurrentAttemptID = next.AttemptID
			state.Executions[i].DecisionID = ""
		}
	}
	if err := projectRuntime(&state); err != nil || state.Runs[0].AttemptID != next.AttemptID {
		t.Fatalf("valid successor attempt was not projected: %v", err)
	}
}
