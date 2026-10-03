package steerline

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func waitingDecisionService(t *testing.T) (*Service, *Run) {
	t.Helper()
	service, run := neutralAuditService(t)
	_, execution := service.workflowRecords(run)
	execution.Status, execution.LastTurnOutcome = "idle", "succeeded"
	service.State.Service.Ready = true
	require(t, service.waitWorkerFlow(run, "await input"))
	return service, run
}

func TestRuntimeViewCannotWriteBusinessState(t *testing.T) {
	service, run := waitingDecisionService(t)
	_, execution := service.workflowRecords(run)
	originalIntent := *execution.WorkspaceIntent
	revision := run.ControlRevision
	view := service.State.Runs[0]
	if view == run {
		t.Fatal("public Run aliases the working actor projection")
	}
	view.FinishTaskStatus, view.ExecutionStatus, view.Decision.Status = "completed", "failed", "superseded"
	view.WorkspaceIntent.Path = "view-only-path"
	view.PR.URL = ptr("view-only-pr")
	service.State.Executions[0].WorkspaceIntent.Path = "record-view-only"
	service.State.Decisions[0].Status = "superseded"
	if run.Decision.Status != "pending" || *execution.WorkspaceIntent != originalIntent {
		t.Fatal("published view changed worker input before a commit")
	}
	execution.LastEvent = ptr("new diagnostic")
	require(t, service.publishCheckpoint())
	if service.executionRecord(run).LastEvent != nil && *service.executionRecord(run).LastEvent == "new diagnostic" {
		t.Fatal("checkpoint publication committed a worker candidate")
	}
	if run.FinishTaskStatus != "waiting_decision" || run.ExecutionStatus != "succeeded" || run.Decision.Status != "pending" || run.PR.URL != nil || *run.WorkspaceIntent != originalIntent || run.ControlRevision != revision {
		t.Fatalf("view became authority or diagnostic changed revision: %+v", run)
	}
	payload, _, err := testSQLite(service).LoadProjection(context.Background(), service.Config.Identity())
	require(t, err)
	for _, key := range []string{`"runs"`, `"executions"`, `"workflow_runs"`, `"dispatch_mode"`, `"slot_occupied"`} {
		if bytes.Contains(payload, []byte(key)) {
			t.Fatalf("checkpoint duplicates business state %s", key)
		}
	}
	require(t, service.store.Heartbeat())
	loaded, err := LoadDatabaseRuntime(service.Config)
	require(t, err)
	if loaded.Runs[0].ControlRevision != revision || loaded.Runs[0].LastEvent != nil && *loaded.Runs[0].LastEvent == "new diagnostic" {
		t.Fatal("coherent runtime read lost domain observation or heartbeat changed revision")
	}
}

func TestActorProjectionReusesPrivatePointersWithoutPublishingCandidates(t *testing.T) {
	service, run := waitingDecisionService(t)
	public := service.State.Runs[0]
	execution := service.executionRecord(run)
	execution.LastEvent = ptr("uncommitted actor diagnostic")
	require(t, service.projectActor())
	if service.find(run.TaskID) != run || run.LastEvent == nil || *run.LastEvent != "uncommitted actor diagnostic" {
		t.Fatal("actor projection replaced its original Run pointer or ignored working records")
	}
	if public.LastEvent != nil && *public.LastEvent == "uncommitted actor diagnostic" {
		t.Fatal("private projection changed the public committed sample")
	}
	require(t, service.projectState())
	if service.find(run.TaskID) != run || service.State.Runs[0] == run || service.State.Runs[0] == public ||
		service.State.Runs[0].LastEvent != nil && *service.State.Runs[0].LastEvent == "uncommitted actor diagnostic" {
		t.Fatal("refresh failed to preserve private identity and independently rebuild the public sample")
	}
}

func TestAnswerChecksDomainRevisionBeforeCheckpointPublication(t *testing.T) {
	service, run := waitingDecisionService(t)
	ctx := context.Background()
	commands := CommandService{Store: service.commandStore, Project: service.Config.Identity()}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Note: "continue", AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	command, _, err := commands.AnswerDecision(ctx, answer.DecisionID, answer.ExpectedDecisionVersion, newID(), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	before, _, err := testSQLite(service).LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	execution := *service.executionRecord(run)
	execution.PendingCommandID = newID()
	require(t, testSQLite(service).transactSQLite(ctx, func(tx *sqliteTx) error { return tx.PutExecution(execution) }))
	after, _, err := testSQLite(service).LoadProjection(ctx, service.Config.Identity())
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

func TestPublicationPreservesExternallyAcceptedDecision(t *testing.T) {
	service, run := waitingDecisionService(t)
	ctx := context.Background()
	commands := CommandService{Store: service.commandStore, Project: service.Config.Identity()}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Note: "keep this answer", AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	command, _, err := commands.AnswerDecision(ctx, answer.DecisionID, answer.ExpectedDecisionVersion, newID(), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	_, _, err = commands.AcceptAnswer(ctx, command, answer)
	require(t, err)
	service.executionRecord(run).LastEvent = ptr("actor diagnostic after external answer")
	require(t, service.publishCheckpoint())
	loaded, err := LoadDatabaseRuntime(service.Config)
	require(t, err)
	if loaded.Runs[0].Decision.Status != "answer_saved" || loaded.Runs[0].Decision.Note != answer.Note {
		t.Fatal("publication overwrote committed answer with actor's stale copy")
	}
}

func TestWorkerCommitRejectsConflictingDecisionAtomically(t *testing.T) {
	service, run := waitingDecisionService(t)
	ctx := context.Background()
	commands := CommandService{Store: service.commandStore, Project: service.Config.Identity()}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Note: "committed answer", AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	command, _, err := commands.AnswerDecision(ctx, answer.DecisionID, answer.ExpectedDecisionVersion, newID(), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	_, _, err = commands.AcceptAnswer(ctx, command, answer)
	require(t, err)
	before, revision, err := testSQLite(service).LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	service.setDecisionStatus(run, "superseded")
	service.executionRecord(run).LastEvent = ptr("must roll back")
	if err = service.commitWorker(run); err == nil || !strings.Contains(err.Error(), "stale_revision") {
		t.Fatalf("conflicting publication was not rejected: %v", err)
	}
	after, nextRevision, err := testSQLite(service).LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	loaded, err := LoadDatabaseRuntime(service.Config)
	require(t, err)
	if !bytes.Equal(before, after) || revision != nextRevision || loaded.Runs[0].Decision.Status != "answer_saved" || loaded.Runs[0].Decision.Note != answer.Note || loaded.Runs[0].LastEvent != nil && *loaded.Runs[0].LastEvent == "must roll back" {
		t.Fatal("conflicting publication changed committed state")
	}
}

func TestObservationReportsMissingAuthorityAsCorruption(t *testing.T) {
	service, _ := waitingDecisionService(t)
	_, err := testSQLite(service).db.Exec(`DELETE FROM project_task_authorities WHERE project_identity=?`, service.Config.Identity())
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
}

func TestWorkerCommitPreservesExternallyAcceptedDecision(t *testing.T) {
	service, run := waitingDecisionService(t)
	ctx := context.Background()
	commands := CommandService{Store: service.commandStore, Project: service.Config.Identity()}
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
