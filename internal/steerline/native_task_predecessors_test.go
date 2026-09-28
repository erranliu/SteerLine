package steerline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddNativeTaskPredecessorsIsAtomicIdempotentAndBlocksClaim(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	commands := CommandService{Store: store, Project: c.Identity()}
	batch := taskBatchFixture(t, c, commit, batchTasks(101, 102, 103), nil)
	_, err := commands.CreateTaskBatch(ctx, c, batch, newID(), 1, "planner")
	require(t, err)
	input := TaskPredecessorAddInput{TaskID: "102", PredecessorIDs: []string{"103", "101"}, RequestID: newID(), ExpectedTaskVersion: 1, ExpectedGraphRevision: 1, Reason: "task 102 consumes tasks 101 and 103", RequestedBy: "planner"}
	receipt, err := commands.AddNativeTaskPredecessors(ctx, c, input)
	require(t, err)
	if receipt.GraphRevision != 2 || receipt.TaskVersion != 2 || receipt.Status != "open" {
		t.Fatalf("unexpected addition receipt: %+v", receipt)
	}
	replayed, err := commands.AddNativeTaskPredecessors(ctx, c, input)
	require(t, err)
	if replayed.GraphRevision != receipt.GraphRevision || !sameStrings(replayed.PredecessorIDs, receipt.PredecessorIDs) {
		t.Fatalf("addition was not idempotent: %+v / %+v", replayed, receipt)
	}
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "102")
	require(t, err)
	if page.Tasks[0].DependencyReady || len(page.Tasks[0].UnmetPredecessors) != 2 || page.Tasks[0].UnmetPredecessors[0].TaskID != "101" {
		t.Fatalf("new predecessor did not block the existing task: %+v", page.Tasks[0])
	}
	bad := input
	bad.RequestID, bad.ExpectedGraphRevision, bad.ExpectedTaskVersion = newID(), 2, 2
	if _, err := commands.AddNativeTaskPredecessors(ctx, c, bad); err == nil || !strings.Contains(err.Error(), "duplicate_dependency") {
		t.Fatalf("duplicate edge accepted: %v", err)
	}
	bad.TaskID, bad.PredecessorIDs, bad.RequestID, bad.ExpectedTaskVersion = "101", []string{"102"}, newID(), 1
	if _, err := commands.AddNativeTaskPredecessors(ctx, c, bad); err == nil || !strings.Contains(err.Error(), "dependency_cycle") {
		t.Fatalf("cycle accepted: %v", err)
	}
	bad.TaskID, bad.PredecessorIDs, bad.RequestID = "103", []string{"101"}, newID()
	bad.ExpectedGraphRevision = 1
	if _, err := commands.AddNativeTaskPredecessors(ctx, c, bad); err == nil || !strings.Contains(err.Error(), "stale_version") {
		t.Fatalf("stale graph revision accepted: %v", err)
	}
	graph, err := (QueryService{Store: store, Project: c.Identity()}).TaskGraph(ctx, "", 100)
	require(t, err)
	if graph.GraphRevision != 2 || len(graph.Edges) != 2 {
		t.Fatalf("invalid additions wrote partial graph: %+v", graph)
	}
	reopened, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	require(t, err)
	defer reopened.Close()
	page, err = (QueryService{Store: reopened, Project: c.Identity()}).ShowNativeTask(ctx, "102")
	require(t, err)
	if page.GraphRevision != 2 || page.Tasks[0].DependencyReady || page.Tasks[0].Version != 2 {
		t.Fatalf("reopened database lost new dependency: %+v", page)
	}
}

func TestNativePredecessorAdditionRejectsDeliveredAndDiscardedSuccessor(t *testing.T) {
	graph := taskGraphSnapshot{Authority: ProjectTaskAuthority{ProjectIdentity: "fixture"}, ByID: map[string]TaskRecord{"1": {TaskID: "1", Status: "open"}, "2": {TaskID: "2", Status: "closed"}}, Deliveries: map[string]TaskDeliveryRecord{"2": {TaskID: "2"}}}
	if err := validateNativePredecessorAddition(graph, "2", []string{"1"}); err == nil || !strings.Contains(err.Error(), "already_merged") {
		t.Fatalf("delivered successor accepted: %v", err)
	}
	graph.ByID["2"] = TaskRecord{TaskID: "2", Status: "discarded"}
	delete(graph.Deliveries, "2")
	if err := validateNativePredecessorAddition(graph, "2", []string{"1"}); err == nil || !strings.Contains(err.Error(), "task_discarded") {
		t.Fatalf("discarded successor accepted: %v", err)
	}
}

func TestAddNativeTaskPredecessorsReopensOnlySettledExecution(t *testing.T) {
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	stateDir := filepath.Join(t.TempDir(), "orca")
	orca := orcaTestConfig(t, stateDir, "local")
	c.Host, c.Agent, c.Orca = "orca", "codex", orca.Orca
	c.configDigest = strings.Repeat("a", 64)
	reader, err := OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, reader.Close())
	lock, err := LockRepository(c.Identity())
	require(t, err)
	defer lock.Close()
	store, err := OpenNativeTaskCommandStore(ctx, &c)
	require(t, err)
	defer store.Close()
	batch := taskBatchFixture(t, c, commit, batchTasks(201, 202), nil)
	commands := CommandService{Store: store, Project: c.Identity()}
	_, err = commands.CreateTaskBatch(ctx, c, batch, newID(), 1, "planner")
	require(t, err)
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "202")
	require(t, err)
	task := page.Tasks[0].TaskRecord
	task.Status, task.Claimant, task.Version, task.UpdatedAt = "in_progress", "operator", 2, now()
	runID, executionID, attemptID := newID(), newID(), newID()
	proc := ProcessIdentity{Kind: "orca", Terminal: "bound-add-predecessor", RuntimeID: "fake-runtime", Incarnation: "inc-bound-add", Executable: c.Orca.Executable}
	profile := ExecutionProfile{ID: "orca-test", Name: "Orca test", Host: "orca", Agent: "codex", Orca: c.Orca, Codex: Command{Executable: "codex"}, Sandbox: "danger-full-access", ProjectSteps: &ProjectSteps{}}
	workflow := WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: runID, TaskID: "202", WorkflowVersion: MinimalWorkflowVersion, ConfigDigest: c.configDigest, ExecutionProfile: ptr(profile.Freeze()), Phase: "blocked", Repository: c.Repository, Branch: c.Branch, CreatedAt: now(), UpdatedAt: now()}
	execution := ExecutionRecord{ProjectIdentity: c.Identity(), ExecutionID: executionID, WorkflowRunID: runID, Backend: "orca", Protocol: "orca_terminal_v2", Process: &proc, Status: "blocked", CurrentStep: "implement", CreatedAt: now(), UpdatedAt: now()}
	attempt := StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: attemptID, ExecutionID: executionID, StepID: "implement", AttemptNumber: 1, Status: "outcome_unknown", CreatedAt: now(), UpdatedAt: now()}
	worktree := filepath.Join(c.RunDir, "worktrees", runID)
	run := &Run{TaskID: "202", DomainTaskID: "202", RunID: runID, ExecutionID: executionID, AttemptID: attemptID, WorkflowVersion: MinimalWorkflowVersion, ExecutionStatus: "unknown", FinishTaskStatus: "blocked", Process: &proc, WorktreePath: &worktree}
	runtimeState := Runtime{SchemaVersion: 5, Revision: 1, Repository: Repository{Identity: c.Identity()}, Runs: []*Run{run}, CurrentTask: ptr("202"), SlotOccupied: true, WorkflowRuns: []WorkflowRunRecord{workflow}}
	require(t, store.Transact(ctx, func(d DomainTx) error {
		tx := d.(*sqliteTx)
		if e := tx.UpdateNativeTaskCAS(task, 1); e != nil {
			return e
		}
		if e := tx.PutWorkflowRun(workflow); e != nil {
			return e
		}
		if e := tx.PutExecution(execution); e != nil {
			return e
		}
		if e := tx.PutStepAttempt(attempt); e != nil {
			return e
		}
		body, e := json.Marshal(runtimeState)
		if e != nil {
			return e
		}
		return tx.PutProjection(c.Identity(), 1, body)
	}))
	if err := store.Transact(ctx, func(d DomainTx) error {
		return d.PutTaskDependency(TaskDependencyRecord{ProjectIdentity: c.Identity(), PredecessorID: "201", SuccessorID: "202"})
	}); err == nil || !strings.Contains(err.Error(), "immutable_task") {
		t.Fatalf("direct edge insertion into a claimed task was accepted: %v", err)
	}
	input := TaskPredecessorAddInput{TaskID: "202", PredecessorIDs: []string{"201"}, RequestID: newID(), ExpectedTaskVersion: 2, ExpectedGraphRevision: 1, WorkflowRunID: runID, ConfirmedProcessStopped: true, Reason: "old worker exited and the new dependency changes its base", RequestedBy: "operator"}
	require(t, os.MkdirAll(stateDir, 0700))
	require(t, writeFakeOrcaState(stateDir, fakeOrcaState{Handle: proc.Terminal, Incarnation: proc.Incarnation}))
	t.Setenv("STEERLINE_FAKE_ORCA_ORPHANED", "1")
	if _, err := commands.AddNativeTaskPredecessors(ctx, c, input); err == nil {
		t.Fatal("unsettled Orca terminal was reset")
	}
	t.Setenv("STEERLINE_FAKE_ORCA_ORPHANED", "0")
	require(t, os.Remove(fakeOrcaStatePath(stateDir, proc.Terminal)))
	run.PR.URL = ptr("https://forge.invalid/org/repo/pulls/1")
	runtimeState.Revision = 2
	require(t, store.Transact(ctx, func(d DomainTx) error {
		body, e := json.Marshal(runtimeState)
		if e != nil {
			return e
		}
		return d.PutProjection(c.Identity(), 2, body)
	}))
	if _, err := commands.AddNativeTaskPredecessors(ctx, c, input); err == nil {
		t.Fatal("published PR was reset")
	}
	run.PR.URL = nil
	runtimeState.Revision = 3
	require(t, store.Transact(ctx, func(d DomainTx) error {
		body, e := json.Marshal(runtimeState)
		if e != nil {
			return e
		}
		return d.PutProjection(c.Identity(), 3, body)
	}))
	receipt, err := commands.AddNativeTaskPredecessors(ctx, c, input)
	require(t, err)
	if receipt.Status != "open" || receipt.TaskVersion != 3 || receipt.GraphRevision != 2 || receipt.AbandonedRunID != runID {
		t.Fatalf("unexpected reset receipt: %+v", receipt)
	}
	replayed, err := commands.AddNativeTaskPredecessors(ctx, c, input)
	require(t, err)
	if replayed.GraphRevision != receipt.GraphRevision || replayed.AbandonedRunID != runID {
		t.Fatalf("reset did not replay: %+v / %+v", replayed, receipt)
	}
	page, err = (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "202")
	require(t, err)
	if page.Tasks[0].Status != "open" || page.Tasks[0].DependencyReady || len(page.Tasks[0].WorkflowRuns) != 1 || page.Tasks[0].WorkflowRuns[0].Phase != "abandoned" {
		t.Fatalf("old run was not preserved while new predecessor blocked claim: %+v", page.Tasks[0])
	}
}
