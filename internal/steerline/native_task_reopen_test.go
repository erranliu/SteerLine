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

func TestReopenSettledSupervisedRunRetainsOldWorktreeAndHistory(t *testing.T) {
	for _, scenario := range []struct {
		name         string
		withDecision bool
		withPR       bool
		unknownPR    bool
	}{{name: "accepted_implement"}, {name: "pending_decision", withDecision: true}, {name: "unmerged_pr", withPR: true}, {name: "unknown_publication", withPR: true, unknownPR: true}} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			f := newFixture(t)
			f.c = supervisedConfigForFixture(t, f)
			c := f.c
			reader, err := OpenNativeTaskProject(ctx, &c)
			require(t, err)
			require(t, reader.Close())
			lock, err := LockRepository(c.Identity())
			require(t, err)
			defer lock.Close()
			store, err := OpenNativeTaskCommandStore(ctx, &c)
			require(t, err)
			defer store.Close()
			contract := nativeContract(t, c, f.commit, "settled-reopen")
			batch, err := json.Marshal(TaskBatchInput{SchemaVersion: taskBatchSchemaVersion, ProjectID: c.Identity(), Tasks: []TaskBatchTask{{LocalKey: "task", TaskID: 14, Title: "Restart settled run", Type: "task", Contract: contract}}, Dependencies: []TaskBatchEdge{}})
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
			runID, executionID, implementID := newID(), newID(), newID()
			currentID, currentStep := implementID, "implement"
			if !scenario.withDecision {
				currentID, currentStep = newID(), "verify"
			}
			if scenario.withPR {
				currentStep = "publish_pr"
			}
			target := targetFor(c)
			target.OrchestrationRun, target.ExternalTask, target.Dispatch = "orca-run-test", "task-test", "dispatch-test"
			target.WorktreeID, target.ExecutionHost = "repo-test::C:/remote/steerline-run", "env-home"
			target.CoordinatorTerminal = c.Orca.Supervised.CoordinatorTerminal
			process := ProcessIdentity{Kind: "orca_supervised", RuntimeID: "fake-remote-runtime", CoordinatorRuntimeID: "fake-center-runtime", Terminal: "terminal-test", Incarnation: "inc-test", OrchestrationRun: target.OrchestrationRun, Task: target.ExternalTask, Dispatch: target.Dispatch, ExecutionHost: target.ExecutionHost, WorktreeID: target.WorktreeID}
			boundAttemptID := implementID
			if scenario.withPR {
				boundAttemptID = currentID
			}
			ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: target, Agent: agentFor(c), RunID: runID, AttemptID: boundAttemptID, WorkspacePath: "C:/remote/steerline-run", WorkspaceBranch: "steerline-old", BaseCommit: f.commit, External: process, Generation: process.RuntimeID + ":" + process.Incarnation}
			require(t, (executionRecords{root: c.RunDir}).saveBinding(ref))
			profile := c.projectExecutionProfile().Freeze()
			workflow := WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: runID, TaskID: "14", WorkflowVersion: MinimalWorkflowVersion, ExecutionProfile: &profile, ProjectSteps: *c.ProjectSteps, Phase: "blocked", CreatedAt: now(), UpdatedAt: now()}
			if scenario.withDecision {
				workflow.PendingDecisions = 1
			}
			execution := ExecutionRecord{ProjectIdentity: c.Identity(), ExecutionID: executionID, WorkflowRunID: runID, Backend: "orca", Protocol: "orca_supervised", Target: target, Status: "blocked", CurrentStep: currentStep, CreatedAt: now(), UpdatedAt: now()}
			if scenario.withPR {
				workflow.Phase, workflow.SelectedResult = "waiting_pr_merge", executionID
				execution.Status, execution.CandidateRevision = "succeeded", f.commit
			}
			prepare := StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: newID(), ExecutionID: executionID, StepID: "prepare", AttemptNumber: 1, Status: "succeeded", CreatedAt: now(), UpdatedAt: now()}
			implement := StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: implementID, ExecutionID: executionID, StepID: "implement", AttemptNumber: 1, Status: "succeeded", ExternalBinding: target, CreatedAt: now(), UpdatedAt: now()}
			if scenario.withPR {
				implement.ExternalBinding = ExecutionTarget{}
			}
			attempts := []StepAttemptRecord{prepare, implement}
			if scenario.withDecision {
				attempts[1].Status = "waiting_decision"
			} else if scenario.withPR {
				attempts = append(attempts, StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: currentID, ExecutionID: executionID, StepID: "publish_pr", AttemptNumber: 1, Status: "succeeded", ExternalBinding: target, CreatedAt: now(), UpdatedAt: now()})
			} else {
				attempts = append(attempts, StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: currentID, ExecutionID: executionID, StepID: "verify", AttemptNumber: 1, Status: "planned", CreatedAt: now(), UpdatedAt: now()})
			}
			worktreePath, worktreeBranch, baseCommit := ref.WorkspacePath, ref.WorkspaceBranch, ref.BaseCommit
			run := &Run{TaskID: "14", DomainTaskID: "14", RunID: runID, ExecutionID: executionID, AttemptID: currentID, WorkflowVersion: MinimalWorkflowVersion, Host: "orca_supervised", Agent: "codex", Backend: "orca", Protocol: "orca_supervised", ExecutionStatus: "unknown", FinishTaskStatus: "blocked", Target: target, WorktreePath: &worktreePath, WorktreeBranch: &worktreeBranch, BaseCommit: &baseCommit}
			state := Runtime{SchemaVersion: 5, Revision: 1, Repository: Repository{Identity: c.Identity()}, Runs: []*Run{run}, CurrentTask: ptr("14"), SlotOccupied: true, WorkflowRuns: []WorkflowRunRecord{workflow}, Executions: []ExecutionRecord{execution}, Attempts: attempts}
			if scenario.withPR {
				prURL := f.server.URL + "/owner/repo/pulls/1"
				run.ExecutionStatus, run.FinishTaskStatus = "succeeded", "waiting_pr_merge"
				if !scenario.unknownPR {
					run.PR.URL = &prURL
				}
				state.CurrentTask, state.SlotOccupied = nil, false
			}
			var decision *DecisionRecord
			if scenario.withDecision {
				decision = &DecisionRecord{ProjectIdentity: c.Identity(), DecisionID: "decision-settled", Scope: "step", WorkflowRunID: runID, ExecutionID: executionID, AttemptID: implementID, QuestionID: "question-settled", Status: "pending", Version: 1, CreatedAt: now(), UpdatedAt: now()}
				state.Decisions = []DecisionRecord{*decision}
			}
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
				for _, attempt := range attempts {
					if e := tx.PutStepAttempt(attempt); e != nil {
						return e
					}
				}
				if decision != nil {
					if e := tx.PutDecision(*decision); e != nil {
						return e
					}
				}
				body, e := json.Marshal(state)
				if e != nil {
					return e
				}
				return tx.PutProjection(c.Identity(), 1, body)
			}))
			input := TaskReopenInput{TaskID: "14", WorkflowRunID: runID, RequestID: newID(), ExpectedVersion: 2, ConfirmUnfinishedReset: true, Reason: "original Dispatch completed; retain its worktree and start a separate run", RequestedBy: "operator"}
			t.Setenv("STEERLINE_FAKE_SUPERVISED_DISPATCH_STATE", "dispatched")
			if _, err := commands.ReopenNativeTask(ctx, c, input); err == nil {
				t.Fatal("live Dispatch was abandoned")
			}
			t.Setenv("STEERLINE_FAKE_SUPERVISED_DISPATCH_STATE", "completed")
			t.Setenv("STEERLINE_FAKE_SUPERVISED_SETTLED_NO_TERMINAL", "1")
			if scenario.unknownPR {
				if _, err := commands.ReopenNativeTask(ctx, c, input); !isNativeErrorCode(err, "pending_verification") {
					t.Fatalf("unknown publish side effect was reset: %v", err)
				}
				return
			}
			if scenario.withPR {
				mergedAt := time.Now().UTC()
				writeFake(f.db, fakeDB{PRHeadRevision: f.commit, PRState: "closed", PRMerged: true, PRMergedAt: &mergedAt, PRMergeCommit: f.commit})
				if _, err := commands.ReopenNativeTask(ctx, c, input); err == nil {
					t.Fatal("merged PR was reset")
				}
				writeFake(f.db, fakeDB{PRHeadRevision: f.commit})
			}
			t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKER_EPOCH", "replacement-remote-runtime")
			if _, err := commands.ReopenNativeTask(ctx, c, input); err == nil {
				t.Fatal("changed remote runtime was abandoned")
			}
			t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKER_EPOCH", "fake-remote-runtime")
			receipt, err := commands.ReopenNativeTask(ctx, c, input)
			require(t, err)
			if receipt.Status != "open" || receipt.AbandonedRunID != runID || receipt.TaskVersion != 3 {
				t.Fatalf("settled reopen receipt = %+v", receipt)
			}
			replayed, err := commands.ReopenNativeTask(ctx, c, input)
			require(t, err)
			if replayed != receipt {
				t.Fatalf("settled reopen was not idempotent: %+v %+v", receipt, replayed)
			}
			page, err = (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "14")
			require(t, err)
			if page.Tasks[0].Status != "open" || !page.Tasks[0].DependencyReady || len(page.Tasks[0].StartupBlockers) != 0 || page.Tasks[0].WorkflowRuns[0].Phase != "abandoned" {
				t.Fatalf("settled Run was not abandoned: %+v", page.Tasks[0])
			}
			projection, _, err := store.LoadProjection(ctx, c.Identity())
			require(t, err)
			var saved Runtime
			require(t, json.Unmarshal(projection, &saved))
			if len(saved.Runs) != 0 || saved.SlotOccupied || saved.CurrentTask != nil {
				t.Fatalf("old Run still reserves the task: %+v", saved)
			}
			if scenario.withDecision {
				if len(saved.Decisions) != 1 || saved.Decisions[0].Status != "superseded" || saved.WorkflowRuns[0].PendingDecisions != 0 {
					t.Fatalf("old pending question was not superseded: %+v", saved.Decisions)
				}
			}
			if _, err := (executionRecords{root: c.RunDir}).loadBinding(runID, boundAttemptID); err != nil {
				t.Fatalf("old worktree binding was discarded: %v", err)
			}
			if scenario.withPR && readFake(f.db).PRState != "" {
				t.Fatal("reset changed the old unmerged PR")
			}
			newRunID := newID()
			claimed, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "14", RequestID: newID(), ExpectedTaskVersion: receipt.TaskVersion, ExpectedAuthorityVersion: page.AuthorityVersion, RequestedBy: "operator", Explicit: true, WorkflowRunID: newRunID})
			require(t, err)
			if claimed.WorkflowRunID != newRunID {
				t.Fatalf("new claim reused the abandoned WorkflowRun: %+v", claimed)
			}
			newSnapshot, err := querySnapshot(ctx, store.db, c.Identity())
			require(t, err)
			for _, newExecution := range newSnapshot.Executions {
				if newExecution.WorkflowRunID == newRunID && (newExecution.ExecutionID == executionID || newExecution.Target.WorktreeID == target.WorktreeID || newExecution.Target.Dispatch != "" || newExecution.Target.OrchestrationRun != "") {
					t.Fatalf("new execution inherited the old remote identity: %+v", newExecution)
				}
			}
		})
	}
}

func TestReopenUnstartedSupervisedRunHasNoExternalWorkerToReplace(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.c = supervisedConfigForFixture(t, f)
	c := f.c
	reader, err := OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, reader.Close())
	lock, err := LockRepository(c.Identity())
	require(t, err)
	defer lock.Close()
	store, err := OpenNativeTaskCommandStore(ctx, &c)
	require(t, err)
	defer store.Close()
	contract := nativeContract(t, c, f.commit, "unstarted-reopen")
	batch, err := json.Marshal(TaskBatchInput{SchemaVersion: taskBatchSchemaVersion, ProjectID: c.Identity(), Tasks: []TaskBatchTask{{LocalKey: "task", TaskID: 14, Title: "Reset before start", Type: "task", Contract: contract}}, Dependencies: []TaskBatchEdge{}})
	require(t, err)
	commands := CommandService{Store: store, Project: c.Identity()}
	_, err = commands.CreateTaskBatch(ctx, c, batch, newID(), 1, "planner")
	require(t, err)
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "14")
	require(t, err)
	task := page.Tasks[0].TaskRecord
	task.Status, task.Claimant, task.Version, task.UpdatedAt = "in_progress", "operator", 2, now()
	runID, executionID, attemptID := newID(), newID(), newID()
	profile := c.projectExecutionProfile().Freeze()
	workflow := WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: runID, TaskID: "14", WorkflowVersion: MinimalWorkflowVersion, ExecutionProfile: &profile, ProjectSteps: *c.ProjectSteps, Phase: "queued", CreatedAt: now(), UpdatedAt: now()}
	execution := ExecutionRecord{ProjectIdentity: c.Identity(), ExecutionID: executionID, WorkflowRunID: runID, Backend: "orca", Protocol: "orca_supervised", Target: targetFor(c), Status: "pending", CurrentStep: "prepare", CreatedAt: now(), UpdatedAt: now()}
	attempt := StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: attemptID, ExecutionID: executionID, StepID: "prepare", AttemptNumber: 1, Status: "planned", CreatedAt: now(), UpdatedAt: now()}
	run := &Run{TaskID: "14", DomainTaskID: "14", RunID: runID, ExecutionID: executionID, AttemptID: attemptID, WorkflowVersion: MinimalWorkflowVersion, Host: "orca_supervised", Agent: "codex", Backend: "orca", Protocol: "orca_supervised", ExecutionStatus: "pending", FinishTaskStatus: "pending", Target: execution.Target}
	state := Runtime{SchemaVersion: 5, Revision: 1, Repository: Repository{Identity: c.Identity()}, Runs: []*Run{run}, CurrentTask: ptr("14"), SlotOccupied: true, WorkflowRuns: []WorkflowRunRecord{workflow}, Executions: []ExecutionRecord{execution}, Attempts: []StepAttemptRecord{attempt}}
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
		body, e := json.Marshal(state)
		if e != nil {
			return e
		}
		return tx.PutProjection(c.Identity(), 1, body)
	}))
	receipt, err := commands.ReopenNativeTask(ctx, c, TaskReopenInput{TaskID: "14", WorkflowRunID: runID, RequestID: newID(), ExpectedVersion: 2, ConfirmUnfinishedReset: true, Reason: "claimed task had no remote Dispatch or worktree", RequestedBy: "operator"})
	require(t, err)
	if receipt.Status != "open" || receipt.AbandonedRunID != runID {
		t.Fatalf("unstarted Run was not reset: %+v", receipt)
	}
}
