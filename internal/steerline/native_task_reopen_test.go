package steerline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReopenNativeTaskRequiresStoppedWorkerAndPreservesAbandonedHistory(t *testing.T) {
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
	contract := nativeContract(t, c, commit, "reopen-14")
	batch, err := json.Marshal(TaskBatchInput{SchemaVersion: taskBatchSchemaVersion, ProjectID: c.Identity(), Tasks: []TaskBatchTask{{LocalKey: "task", TaskID: 14, Title: "Restart me", Type: "task", Contract: contract}}, Dependencies: []TaskBatchEdge{}})
	require(t, err)
	commands := CommandService{Store: store, Project: c.Identity()}
	_, err = commands.CreateTaskBatch(ctx, c, batch, newID(), 1, "planner")
	require(t, err)
	authority, err := store.nativeAuthority(ctx, c.Identity())
	require(t, err)
	unpaused := false
	_, err = commands.SetNativeDispatchState(ctx, NativeDispatchUpdate{RequestID: newID(), ExpectedAuthorityVersion: authority.AuthorityVersion, Paused: &unpaused, RequestedBy: "operator"})
	require(t, err)
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "14")
	require(t, err)
	task := page.Tasks[0].TaskRecord
	task.Status, task.Claimant, task.Version, task.UpdatedAt = "in_progress", "operator", 2, now()
	runID, executionID, attemptID := newID(), newID(), newID()
	proc := ProcessIdentity{Kind: "orca", Terminal: "bound-reopen", RuntimeID: "fake-runtime", Incarnation: "inc-bound-reopen", Executable: c.Orca.Executable}
	profile := ExecutionProfile{ID: "orca-test", Name: "Orca test", Host: "orca", Agent: "codex", Orca: c.Orca, Codex: Command{Executable: "codex"}, Sandbox: "danger-full-access", ProjectSteps: &ProjectSteps{}}
	workflow := WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: runID, TaskID: "14", WorkflowVersion: MinimalWorkflowVersion, ConfigDigest: c.configDigest, ExecutionProfile: ptr(profile.Freeze()), Phase: "blocked", Repository: c.Repository, Branch: c.Branch, CreatedAt: now(), UpdatedAt: now()}
	execution := ExecutionRecord{ProjectIdentity: c.Identity(), ExecutionID: executionID, WorkflowRunID: runID, Backend: "orca", Protocol: "orca_terminal_v2", Process: &proc, Status: "blocked", CurrentStep: "implement", CreatedAt: now(), UpdatedAt: now()}
	attempt := StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: attemptID, ExecutionID: executionID, StepID: "implement", AttemptNumber: 1, Status: "outcome_unknown", CreatedAt: now(), UpdatedAt: now()}
	worktree := filepath.Join(c.RunDir, "worktrees", runID)
	run := &Run{TaskID: "14", DomainTaskID: "14", RunID: runID, ExecutionID: executionID, AttemptID: attemptID, WorkflowVersion: MinimalWorkflowVersion, ExecutionStatus: "unknown", FinishTaskStatus: "blocked", Process: &proc, WorktreePath: &worktree}
	runtimeState := Runtime{SchemaVersion: 5, Revision: 1, Repository: Repository{Identity: c.Identity()}, Runs: []*Run{run}, CurrentTask: ptr("14"), SlotOccupied: true, WorkflowRuns: []WorkflowRunRecord{workflow}}
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
	input := TaskReopenInput{TaskID: "14", WorkflowRunID: runID, RequestID: newID(), ExpectedVersion: 2, ConfirmedProcessStopped: true, Reason: "original terminal absent and no worker process remains", RequestedBy: "operator"}
	require(t, os.MkdirAll(worktree, 0700))
	if _, err := commands.ReopenNativeTask(ctx, c, input); err == nil {
		t.Fatal("worktree with previous progress was reopened before cleanup")
	}
	require(t, os.Remove(worktree))
	require(t, os.MkdirAll(stateDir, 0700))
	require(t, writeFakeOrcaState(stateDir, fakeOrcaState{Handle: proc.Terminal, Incarnation: proc.Incarnation}))
	t.Setenv("STEERLINE_FAKE_ORCA_ORPHANED", "1")
	if _, err := commands.ReopenNativeTask(ctx, c, input); err == nil {
		t.Fatal("live orphaned Orca terminal was reopened")
	}
	t.Setenv("STEERLINE_FAKE_ORCA_ORPHANED", "0")
	require(t, os.Remove(fakeOrcaStatePath(stateDir, proc.Terminal)))
	t.Setenv("STEERLINE_FAKE_ORCA_SHOW_UNREACHABLE", "1")
	if _, err := commands.ReopenNativeTask(ctx, c, input); err == nil {
		t.Fatal("unreachable Orca was treated as proof that the worker stopped")
	}
	t.Setenv("STEERLINE_FAKE_ORCA_SHOW_UNREACHABLE", "0")
	receipt, err := commands.ReopenNativeTask(ctx, c, input)
	require(t, err)
	if receipt.Status != "open" || receipt.TaskVersion != 3 || receipt.AbandonedRunID != runID {
		t.Fatalf("unexpected reopen receipt: %+v", receipt)
	}
	replayed, err := commands.ReopenNativeTask(ctx, c, input)
	require(t, err)
	if replayed != receipt {
		t.Fatalf("idempotent reopen changed its result: %+v %+v", replayed, receipt)
	}
	page, err = (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "14")
	require(t, err)
	if page.Tasks[0].Status != "open" || !page.Tasks[0].DependencyReady || len(page.Tasks[0].StartupBlockers) != 0 || len(page.Tasks[0].WorkflowRuns) != 1 || page.Tasks[0].WorkflowRuns[0].Phase != "abandoned" {
		t.Fatalf("task is not ready with abandoned history: %+v", page.Tasks[0])
	}
	projection, _, err := store.LoadProjection(ctx, c.Identity())
	require(t, err)
	var saved Runtime
	require(t, json.Unmarshal(projection, &saved))
	if len(saved.Runs) != 0 || saved.CurrentTask != nil || saved.SlotOccupied {
		t.Fatalf("old run still reserves the execution slot: %+v", saved)
	}
	// The old attempt is terminal, and a new workflow for the same Task ID can
	// now be created without deleting the previous execution history.
	require(t, store.Transact(ctx, func(d DomainTx) error {
		newRun := WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: newID(), TaskID: "14", WorkflowVersion: MinimalWorkflowVersion, Phase: "queued", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		return d.PutWorkflowRun(newRun)
	}))
}

func TestSupervisedReopenRequiresExactCompletedWorkerStoppedTerminalAndRemovedWorktree(t *testing.T) {
	executable, err := os.Executable()
	require(t, err)
	config := Config{Repository: t.TempDir(), CommandTimeout: "5s", Orca: OrcaConfig{Executable: executable, Args: []string{"--fake", "orca-supervised"}, Supervised: &SupervisedOrcaConfig{EnvironmentID: "env-home"}}}
	identity := ProcessIdentity{Kind: "orca_supervised", CoordinatorRuntimeID: "fake-center-runtime", RuntimeID: "fake-remote-runtime", OrchestrationRun: "run-reopen", Task: "task-reopen", Dispatch: "dispatch-reopen", ExecutionHost: "env-home", WorktreeID: "repo-test::worktree", Terminal: "terminal-reopen", Incarnation: "inc-reopen"}
	t.Setenv("STEERLINE_FAKE_REOPEN", "1")
	t.Setenv("STEERLINE_FAKE_REOPEN_TERMINAL_LIVE", "1")
	if err := verifyStoppedSupervisedReopen(context.Background(), config, identity); err == nil || !strings.Contains(err.Error(), "cleanup_pending") {
		t.Fatalf("live terminal was reopened: %v", err)
	}
	t.Setenv("STEERLINE_FAKE_REOPEN_TERMINAL_LIVE", "0")
	require(t, verifyStoppedSupervisedReopen(context.Background(), config, identity))
	identity.RuntimeID = "replacement-runtime"
	if err := verifyStoppedSupervisedReopen(context.Background(), config, identity); err == nil || !strings.Contains(err.Error(), "cleanup_pending") {
		t.Fatalf("changed runtime was reopened: %v", err)
	}
}
