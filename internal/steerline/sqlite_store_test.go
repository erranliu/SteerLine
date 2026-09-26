package steerline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNativeRuntimeProjectionDoesNotShadowCenterOwnedControlCommands(t *testing.T) {
	runtime := Runtime{Repository: Repository{Identity: "native-project"}, Control: ControlState{Operation: &ControlOperation{RequestID: strings.Repeat("a", 32), Action: "set_dispatch_mode", Phase: "applying", RequestDigest: strings.Repeat("b", 64)}}}
	if operations := runtimeOperations(runtime, true); len(operations) != 0 {
		t.Fatalf("native center control was projected as a second operation: %+v", operations)
	}
	if operations := runtimeOperations(runtime, false); len(operations) != 1 || operations[0].Kind != "set_dispatch_mode" {
		t.Fatalf("Beads-era control projection behavior changed: %+v", operations)
	}
	runtime.Control.Operation.Action = "stop_service"
	if operations := runtimeOperations(runtime, true); len(operations) != 1 || operations[0].Kind != "stop_service" {
		t.Fatalf("service stop operation was omitted from the native runtime projection: %+v", operations)
	}
}

func openDomainTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestConversationSchemaUpgradesCurrentCenterDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steerline.db")
	raw, err := sql.Open("sqlite", path)
	require(t, err)
	tx, err := raw.Begin()
	require(t, err)
	_, err = tx.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`)
	require(t, err)
	for _, stmt := range append(append([]string{}, domainExecutionSchema...), nativeTaskSchema...) {
		_, err = tx.Exec(stmt)
		require(t, err)
	}
	_, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(3,?)`, time.Now().UTC().Format(time.RFC3339Nano))
	require(t, err)
	require(t, tx.Commit())
	require(t, raw.Close())
	require(t, validateExistingDatabase(path))
	reader, err := OpenSQLiteReader(path)
	require(t, err)
	if err = validateDatabaseSchema(context.Background(), reader.db); err == nil {
		t.Fatal("schema 3 was exposed as the current native task schema before upgrade")
	}
	require(t, reader.Close())
	store, err := OpenSQLiteStore(path)
	require(t, err)
	defer store.Close()
	var version int
	require(t, store.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version))
	if version != domainSchemaVersion {
		t.Fatalf("conversation upgrade wrote schema %d", version)
	}
	for _, name := range []string{"conversation_events", "conversation_bodies", "conversation_source_cursors", "conversation_native_source_idx"} {
		var count int
		require(t, store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name=?`, name).Scan(&count))
		if count != 1 {
			t.Fatalf("conversation upgrade lacks %s", name)
		}
	}
	require(t, validateDatabaseSchema(context.Background(), store.db))
}

func TestSQLiteRejectsParallelExecutionsInOneWorkflow(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	project := "repo:test"
	err := s.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutTask(TaskRecord{ProjectIdentity: project, TaskID: "17", Status: "in_progress", Version: 1}); err != nil {
			return err
		}
		if err := tx.PutWorkflowRun(WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "wf-1", TaskID: "17", WorkflowVersion: MinimalWorkflowVersion, Phase: "executing"}); err != nil {
			return err
		}
		for _, id := range []string{"exec-a"} {
			if err := tx.PutExecution(ExecutionRecord{ProjectIdentity: project, ExecutionID: id, WorkflowRunID: "wf-1", Agent: id, Status: "active", CurrentStep: "implement", Target: ExecutionTarget{WorktreeID: "worktree-" + id, Branch: "branch-" + id, Dispatch: "dispatch-" + id}}); err != nil {
				return err
			}
			if err := tx.PutStepAttempt(StepAttemptRecord{ProjectIdentity: project, AttemptID: "attempt-" + id, ExecutionID: id, StepID: "implement", AttemptNumber: 1, Status: "running"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Transact(ctx, func(tx DomainTx) error {
		return tx.PutExecution(ExecutionRecord{ProjectIdentity: project, ExecutionID: "exec-b", WorkflowRunID: "wf-1", Status: "active", CurrentStep: "verify"})
	})
	if err == nil {
		t.Fatal("second active execution was accepted")
	}
	snapshot, err := s.Query(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.WorkflowRuns) != 1 || len(snapshot.Executions) != 1 || len(snapshot.Attempts) != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestSQLiteTransactionRollsBackAllDomainWrites(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	project := "repo:rollback"
	err := s.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutTask(TaskRecord{ProjectIdentity: project, TaskID: "task-a", Status: "open", Version: 1}); err != nil {
			return err
		}
		return tx.PutWorkflowRun(WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "wf-bad", TaskID: "missing-task", Phase: "queued"})
	})
	if err == nil {
		t.Fatal("expected FK violation")
	}
	snapshot, queryErr := s.Query(ctx, project)
	if queryErr != nil {
		t.Fatal(queryErr)
	}
	if len(snapshot.Tasks) != 0 || len(snapshot.WorkflowRuns) != 0 {
		t.Fatalf("partial state survived rollback: %+v", snapshot)
	}
}

func TestCommandIdempotencyAndDecisionVersionArbitration(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	project := "repo:commands"
	request := DecisionRequest{Question: "choose", AllowFreeText: true}
	run := &Run{TaskID: "task", RunID: "wf", DomainTaskID: "task", ExecutionID: "exec", AttemptID: "attempt", ControlRevision: 5, Host: "orca", Backend: string(BackendOrca), Protocol: string(ProtocolOrcaTerminalV2), WorkflowVersion: "legacy-v1", ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision", Decision: &DecisionState{ID: "decision", AttemptID: "attempt", Version: 1, Status: "pending", Request: request}}
	projection := Runtime{SchemaVersion: 5, Revision: 1, Repository: Repository{Identity: project}, Service: ServiceState{InstanceID: "service", Status: "running", Ready: true}, Runs: []*Run{run}}
	err := s.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutTask(TaskRecord{ProjectIdentity: project, TaskID: "task", Status: "in_progress", Version: 1}); err != nil {
			return err
		}
		if err := tx.PutWorkflowRun(WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "wf", TaskID: "task", Phase: "executing"}); err != nil {
			return err
		}
		if err := tx.PutExecution(ExecutionRecord{ProjectIdentity: project, ExecutionID: "exec", WorkflowRunID: "wf", Status: "waiting_decision", CurrentStep: "implement"}); err != nil {
			return err
		}
		if err := tx.PutStepAttempt(StepAttemptRecord{ProjectIdentity: project, AttemptID: "attempt", ExecutionID: "exec", StepID: "implement", AttemptNumber: 1, Status: "waiting_decision"}); err != nil {
			return err
		}
		if err := tx.PutDecision(DecisionRecord{ProjectIdentity: project, DecisionID: "decision", Scope: "execution", WorkflowRunID: "wf", ExecutionID: "exec", AttemptID: "attempt", QuestionID: "q1", Status: "pending", Version: 1, Request: mustJSON(request)}); err != nil {
			return err
		}
		return tx.PutProjection(project, projection.Revision, mustJSON(projection))
	})
	if err != nil {
		t.Fatal(err)
	}
	commands := CommandService{Store: s, Project: project}
	answer := DecisionAnswer{DecisionID: "decision", RunID: "wf", Note: "yes", AnsweredBy: "alice", ExpectedDecisionVersion: 1, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: projection.Service.InstanceID}
	first, created, err := commands.AnswerDecision(ctx, "decision", 1, "answer-key-a", "alice", mustJSON(answer))
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first answer was not enqueued")
	}
	retry, created, err := commands.AnswerDecision(ctx, "decision", 1, "answer-key-a", "alice", mustJSON(answer))
	if err != nil {
		t.Fatal(err)
	}
	if created || retry.CommandID != first.CommandID {
		t.Fatalf("idempotent retry changed command: %+v %+v", first, retry)
	}
	var savedAnswer DecisionAnswer
	require(t, decode(first.Payload, &savedAnswer))
	if _, _, err = commands.AcceptAnswer(ctx, first, savedAnswer); err != nil {
		t.Fatalf("first answer was not accepted against the current service projection: %v", err)
	}
	if _, _, err = commands.AnswerDecision(ctx, "decision", 1, "answer-key-b", "bob", mustJSON(DecisionAnswer{DecisionID: "decision", RunID: "wf", Note: "no", AnsweredBy: "bob", ExpectedDecisionVersion: 1, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: projection.Service.InstanceID})); !errors.Is(err, ErrAnswerConflict) {
		t.Fatalf("second accepted answer should lose first-wins arbitration, got %v", err)
	}
	pending, err := commands.Pending(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("unexpected pending commands: %+v", pending)
	}
	snapshot, err := s.Query(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Decisions[0].Status != "answer_saved" || snapshot.Decisions[0].Version != 2 {
		t.Fatalf("first accepted command did not persist its business fact: %+v", snapshot.Decisions[0])
	}
}

func TestMinimalWorkflowRevisionGatesAndRepairLimit(t *testing.T) {
	run := &WorkflowRunRecord{WorkflowVersion: MinimalWorkflowVersion, Definition: WorkflowDefinition{ID: MinimalWorkflowVersion, Version: MinimalWorkflowVersion, ReviewRequired: true, MaxRepairRounds: 1}, Phase: "executing"}
	base := strings.Repeat("a", 40)
	prepared := WorkspaceEvidence{Repository: "repo", Worktree: "tree", Branch: "codex/test", Base: base, Head: base, VerifiedAt: time.Now().UTC()}
	exec := &ExecutionRecord{Status: "active", Protocol: string(ProtocolOrcaSupervised), CurrentStep: "prepare", Workspace: WorkspaceEvidence{Repository: "repo", Worktree: "tree", Branch: "codex/test", Base: base}}
	if step, err := AdvanceMinimalExecution(run, exec, StepGateResult{StepID: "prepare", Succeeded: true, Workspace: prepared}, nil); err != nil || step != "implement" {
		t.Fatalf("prepare gate: %q %v", step, err)
	}
	exec.CurrentStep = "implement"
	commit := strings.Repeat("b", 40)
	verified := time.Now().UTC()
	implement := StepGateResult{StepID: "implement", Succeeded: true, InputRevision: base, Revision: commit, Workspace: WorkspaceEvidence{Repository: "repo", Worktree: "tree", Branch: "codex/test", Base: exec.Workspace.Base, Head: commit, VerifiedAt: verified}}
	for _, test := range []struct {
		name       string
		mutate     func(*StepGateResult)
		wantErrSub string
	}{
		{name: "input revision", mutate: func(v *StepGateResult) { v.InputRevision = strings.Repeat("c", 40) }, wantErrSub: "implement input revision"},
		{name: "repository", mutate: func(v *StepGateResult) { v.Workspace.Repository = "other-repo" }, wantErrSub: "implement workspace repository"},
		{name: "worktree", mutate: func(v *StepGateResult) { v.Workspace.Worktree = "other-tree" }, wantErrSub: "implement worktree"},
		{name: "branch", mutate: func(v *StepGateResult) { v.Workspace.Branch = "other-branch" }, wantErrSub: "implement branch"},
		{name: "base", mutate: func(v *StepGateResult) { v.Workspace.Base = strings.Repeat("c", 40) }, wantErrSub: "implement base revision"},
	} {
		t.Run(test.name, func(t *testing.T) {
			badRun, badExec, badResult := *run, *exec, implement
			test.mutate(&badResult)
			if _, err := AdvanceMinimalExecution(&badRun, &badExec, badResult, nil); err == nil || !strings.Contains(err.Error(), test.wantErrSub) {
				t.Fatalf("mismatched implement %s receipt was not rejected precisely: %v", test.name, err)
			}
		})
	}
	if step, err := AdvanceMinimalExecution(run, exec, implement, nil); err != nil || step != "verify" {
		t.Fatalf("implement gate: %q %v", step, err)
	}
	exec.CurrentStep = "verify"
	failedExit := 1
	failedPassed := false
	verify := StepGateResult{StepID: "verify", Succeeded: true, Revision: commit, Workspace: implement.Workspace, ResultID: "verify-1", Checks: []CheckEvidence{{Name: "unit", Status: "failed", Revision: commit, Command: "go test ./...", ScriptVersion: "fixture/v1", ExitCode: &failedExit, Passed: &failedPassed, EvidenceRef: "fixture:unit", ActualCommit: commit}}}
	badVerify := verify
	badVerify.Checks = append([]CheckEvidence(nil), verify.Checks...)
	badVerify.Checks[0].EvidenceRef = ""
	if _, err := AdvanceMinimalExecution(run, exec, badVerify, []string{"unit"}); err == nil {
		t.Fatal("verify passed without a script evidence reference")
	}
	badVerify = verify
	badVerify.Checks = append([]CheckEvidence(nil), verify.Checks...)
	badVerify.Checks[0].ActualCommit = base
	if _, err := AdvanceMinimalExecution(run, exec, badVerify, []string{"unit"}); err == nil {
		t.Fatal("verify passed with check evidence from another commit")
	}
	badVerify = verify
	badVerify.Checks = append([]CheckEvidence(nil), verify.Checks...)
	badVerify.Checks[0].Status = "passed"
	if _, err := AdvanceMinimalExecution(run, exec, badVerify, []string{"unit"}); err == nil {
		t.Fatal("verify passed with a failed exit code")
	}
	if step, err := AdvanceMinimalExecution(run, exec, verify, []string{"unit"}); err != nil || step != "implement" || exec.RepairRounds != 1 {
		t.Fatalf("failed deterministic check did not enter repair: step=%q err=%v rounds=%d", step, err, exec.RepairRounds)
	}
	verify.Checks[0].Status = "passed"
	passedExit := 0
	passed := true
	verify.Checks[0].ExitCode = &passedExit
	verify.Checks[0].Passed = &passed
	exec.CurrentStep = "verify"
	if step, err := AdvanceMinimalExecution(run, exec, verify, []string{"unit"}); err != nil || step != "review" {
		t.Fatalf("verify gate: %q %v", step, err)
	}
	exec.CurrentStep = "review"
	if step, err := AdvanceMinimalExecution(run, exec, StepGateResult{StepID: "review", Succeeded: true, Revision: strings.Repeat("c", 40), ReviewVerdict: "passed", ResultID: "review-bad", Workspace: implement.Workspace}, nil); err == nil || step != "" {
		t.Fatal("review of a different revision must be rejected")
	}
	if step, err := AdvanceMinimalExecution(run, exec, StepGateResult{StepID: "review", Succeeded: true, Revision: commit, ReviewVerdict: "passed", ResultID: "review-1", Workspace: implement.Workspace}, nil); err != nil || step != "publish_pr" {
		t.Fatalf("review gate: %q %v", step, err)
	}
	exec.CurrentStep = "publish_pr"
	if _, err := AdvanceMinimalExecution(run, exec, StepGateResult{StepID: "publish_pr", Succeeded: true, Revision: commit, Workspace: implement.Workspace, PublishedHead: strings.Repeat("c", 40), PRURL: "https://example.test/pr/1"}, nil); err == nil {
		t.Fatal("published head mismatch passed")
	}
	if step, err := AdvanceMinimalExecution(run, exec, StepGateResult{StepID: "publish_pr", Succeeded: true, Revision: commit, Workspace: implement.Workspace, PublishedHead: commit, PRURL: "https://example.test/pr/1"}, nil); err != nil || step != "waiting_pr_merge" {
		t.Fatalf("publish gate: %q %v", step, err)
	}
}

func TestMinimalWorkflowDispatchesEachStepAndPersistsPRGate(t *testing.T) {
	f := newFixture(t, task("t-17"))
	exe, err := os.Executable()
	require(t, err)
	git, err := exec.LookPath("git")
	require(t, err)
	contract := func(step string) StepContract {
		return StepContract{Provider: "command", Entry: "builtin:" + step, Version: "test-v1", InputSchema: MinimalStepInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "local", Timeout: 10 * time.Second, Command: []string{exe, "--fake", "minimal-step", f.db, git}}
	}
	f.c.Host = "local"
	f.c.WorkflowDefinition = ptr(DefaultMinimalWorkflowDefinition(true))
	f.c.ProjectSteps = &ProjectSteps{Implement: contract("implement"), Verify: contract("verify"), Review: contract("review"), PublishPR: contract("publish_pr")}
	f.c.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	f.c.PR.Kind = "command"
	f.c.PR.Query = Command{Executable: exe, Args: []string{"--fake", "pr-query", f.db, "{pr_url}", "{repository}"}}
	ctx := context.Background()
	f.c.Remote = "web"
	preflightErr := f.c.Preflight(ctx, true)
	f.c.Remote = "origin"
	if preflightErr != nil {
		t.Fatal(preflightErr)
	}
	s := f.service(t)
	t.Cleanup(func() { _ = s.Close() })
	if err = s.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if s.worker == nil {
		t.Fatalf("dispatch returned without a step worker: service=%+v runs=%+v workflow_runs=%+v attempts=%+v", s.State.Service, s.State.Runs, s.State.WorkflowRuns, s.State.Attempts)
	}
	finishWorker(t, s)
	if s.State.Executions[0].CurrentStep != "verify" {
		t.Fatalf("implement completion did not persist verify intent: %+v", s.State.Executions[0])
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = f.service(t)
	if err = s.resumeMinimalSteps(ctx); err != nil {
		t.Fatal(err)
	}
	for step := 0; step < 3; step++ {
		finishWorker(t, s)
		if s.State.Runs[0].FinishTaskStatus == "waiting_pr_merge" {
			break
		}
		if err = s.resumeMinimalSteps(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.State.Runs[0].FinishTaskStatus; got != "waiting_pr_merge" {
		t.Fatalf("finish_task_status=%s, want waiting_pr_merge", got)
	}
	if len(s.State.WorkflowRuns) != 1 || s.State.WorkflowRuns[0].Phase != "waiting_pr_merge" || s.State.WorkflowRuns[0].SelectedResult != s.State.Executions[0].ExecutionID {
		t.Fatalf("workflow gate was not persisted: %+v", s.State.WorkflowRuns)
	}
	if len(s.State.Attempts) != 5 {
		t.Fatalf("step attempts=%d, want prepare plus four configured steps", len(s.State.Attempts))
	}
	for _, attempt := range s.State.Attempts {
		if attempt.Status != "succeeded" {
			t.Fatalf("attempt %s/%s status=%s", attempt.StepID, attempt.AttemptID, attempt.Status)
		}
	}
	if s.State.Tasks[0].Status != "in_progress" || s.State.Executions[0].Status != "succeeded" || s.State.Executions[0].CurrentStep != "waiting_pr_merge" {
		t.Fatalf("PR publication must wait for merge without closing task: task=%+v execution=%+v", s.State.Tasks[0], s.State.Executions[0])
	}
}

func TestMinimalLocalDecisionAnswerIsRejectedWithoutContinuation(t *testing.T) {
	f := newFixture(t, task("t-18"))
	db := readFake(f.db)
	db.Behavior = "minimal-decision"
	writeFake(f.db, db)
	exe, err := os.Executable()
	require(t, err)
	git, err := exec.LookPath("git")
	require(t, err)
	contract := func(step string) StepContract {
		return StepContract{Provider: "command", Entry: "builtin:" + step, Version: "test-v1", InputSchema: MinimalStepInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "local", Timeout: 10 * time.Second, Command: []string{exe, "--fake", "minimal-step", f.db, git}}
	}
	f.c.Host = "local"
	f.c.WorkflowDefinition = ptr(DefaultMinimalWorkflowDefinition(true))
	f.c.ProjectSteps = &ProjectSteps{Implement: contract("implement"), Verify: contract("verify"), Review: contract("review"), PublishPR: contract("publish_pr")}
	f.c.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	s := f.service(t)
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	if err = s.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	finishWorker(t, s) // prepare and start implement
	if got := s.State.Runs[0].FinishTaskStatus; got != "waiting_decision" {
		t.Fatalf("finish_task_status=%s, want waiting_decision", got)
	}
	decision := s.State.Runs[0].Decision
	if decision == nil || decision.Status != "pending" || len(s.State.Decisions) != 1 {
		t.Fatalf("decision was not persisted: run=%+v records=%+v", decision, s.State.Decisions)
	}
	answer, err := json.Marshal(DecisionAnswer{DecisionID: decision.ID, RunID: s.State.Runs[0].RunID, OptionID: "keep", AnsweredBy: "tester", ExpectedDecisionVersion: decision.Version, ExpectedRunRevision: s.State.Runs[0].ControlRevision, ServiceInstanceID: s.State.Service.InstanceID})
	require(t, err)
	commands := CommandService{Store: s.domainStore, Project: s.Config.identity}
	if _, _, err = commands.AnswerDecision(ctx, decision.ID, decision.Version, "answer-once", "tester", answer); err != nil {
		t.Fatal(err)
	}
	if err = s.processPendingCommands(ctx); err != nil {
		t.Fatal(err)
	}
	queued, err := s.domainStore.Commands(ctx, s.Config.identity, "", 100)
	require(t, err)
	if len(queued) != 1 || queued[0].Status != "rejected" || !strings.Contains(string(queued[0].Result), "local continuation is unsupported") {
		t.Fatalf("unsupported local decision continuation was not rejected with its reason: %+v", queued)
	}
	if s.worker != nil || s.State.Runs[0].Decision.Status != "pending" || s.State.Runs[0].Decision.AnsweredBy != "" {
		t.Fatalf("unsupported continuation mutated the decision or started a worker: worker=%v decision=%+v", s.worker != nil, s.State.Runs[0].Decision)
	}
	attemptCount := len(s.State.Attempts)
	decision.Status, decision.OptionID, decision.AnsweredBy = "answer_saved", "keep", "tester"
	decision.AnsweredAt = ptr(now())
	if err = s.save(); err != nil {
		t.Fatal(err)
	}
	if err = s.applyDecisions(ctx); err != nil {
		t.Fatal(err)
	}
	if s.worker != nil || decision.Status != "answer_saved" || len(s.State.Attempts) != attemptCount || s.State.Executions[0].CurrentStep != "implement" {
		t.Fatalf("saved answer bypassed unsupported-continuation gate: worker=%v decision=%+v attempts=%d/%d execution=%+v", s.worker != nil, decision, len(s.State.Attempts), attemptCount, s.State.Executions[0])
	}
}

func TestRuntimeSchemasV1ThroughV4AreRejectedWithoutMutation(t *testing.T) {
	for version := 1; version <= 4; version++ {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			runDir := t.TempDir()
			path := filepath.Join(runDir, "runtime.json")
			source, err := json.Marshal(Runtime{SchemaVersion: version})
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, source, 0600); err != nil {
				t.Fatal(err)
			}
			if err = preflightExistingState(runDir, "runtime.json"); err == nil || !strings.Contains(err.Error(), "unsupported runtime schema") {
				t.Fatalf("v%d was not rejected by startup preflight: %v", version, err)
			}
			if _, err = os.Stat(filepath.Join(runDir, "steerline.db")); !os.IsNotExist(err) {
				t.Fatalf("preflight created or modified the database path: %v", err)
			}
			if _, err = LoadRuntime(path); err == nil || !strings.Contains(err.Error(), "unsupported runtime schema") {
				t.Fatalf("v%d was accepted: %v", version, err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(source, after) {
				t.Fatalf("v%d snapshot was changed while rejecting it: %v", version, err)
			}
		})
	}
}

func TestPreflightRejectsUnknownSQLiteSchemaWithoutMutation(t *testing.T) {
	runDir := t.TempDir()
	path := filepath.Join(runDir, "steerline.db")
	db, err := sql.Open("sqlite", path)
	require(t, err)
	if _, err = db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(99,'unknown')`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = preflightExistingState(runDir, "runtime.json"); err == nil || !strings.Contains(err.Error(), "unsupported SQLite schema 99") {
		t.Fatalf("unknown database schema was accepted: %v", err)
	}
	reader, err := OpenSQLiteReader(path)
	require(t, err)
	defer reader.Close()
	var version, tableCount int
	require(t, reader.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version))
	require(t, reader.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tableCount))
	if version != 99 || tableCount != 1 {
		t.Fatalf("preflight modified unknown database: version=%d tables=%d", version, tableCount)
	}
}

func TestPlannedTaskIdentityNeedsVerifiedExplicitMapping(t *testing.T) {
	repository := t.TempDir()
	runDir := t.TempDir()
	planned := filepath.Join(repository, "docs", "roadmap", "task-ids")
	if err := os.MkdirAll(planned, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planned, "5.json"), []byte(`{"id":5,"name":"planned"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveDomainTaskIdentity(repository, runDir, "repo:map", "beads-project", "5"); err == nil {
		t.Fatal("planned numeric identity was guessed from the source ID")
	}
	mapFile := filepath.Join(runDir, "task-identity-map.json")
	identityMap := TaskIdentityMap{Schema: taskIdentityMapSchema, ProjectIdentity: "repo:map", Mappings: []TaskIdentityMapping{{TaskID: "5", SourceProvider: "beads", SourceProject: "beads-project", SourceTaskID: "5"}}}
	data, _ := json.Marshal(identityMap)
	mapFile = filepath.Join(runDir, "task-identity-map.json")
	if err := os.WriteFile(mapFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	if id, digest, err := resolveDomainTaskIdentity(repository, runDir, "repo:map", "beads-project", "5"); err != nil || id != "5" || digest == "" {
		t.Fatalf("mapped task identity=%q digest=%q err=%v", id, digest, err)
	}
	identityMap.Mappings[0].TaskID = "05"
	data, _ = json.Marshal(identityMap)
	_ = os.WriteFile(mapFile, data, 0600)
	if _, _, _, err := parseTaskIdentityMap(mapFile, "repo:map"); err == nil {
		t.Fatal("leading zero in numeric ID should be rejected")
	}
}

func TestQueryEventCursorAndLargeBodyPaging(t *testing.T) {
	ctx := context.Background()
	store := openDomainTestStore(t)
	project := "repo:event-query"
	body := make([]byte, (1<<20)+12345)
	for i := range body {
		body[i] = byte(i % 251)
	}
	var firstID int64
	err := store.Transact(ctx, func(tx DomainTx) error {
		var err error
		firstID, err = tx.AppendEvent(DomainEvent{ProjectIdentity: project, ExecutionID: "exec", AttemptID: "attempt", ExternalSource: "test", ExternalID: "1", Type: "conversation.message", Payload: body})
		if err != nil {
			return err
		}
		_, err = tx.AppendEvent(DomainEvent{ProjectIdentity: project, ExecutionID: "exec", AttemptID: "attempt", ExternalSource: "test", ExternalID: "2", Type: "observation", Payload: []byte("small")})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	query := QueryService{Store: store, Project: project}
	page, err := query.ReadEvents(ctx, EventFilter{ExecutionID: "exec", AttemptID: "attempt"}, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Events[0].EventID != firstID || !page.Events[0].PayloadOmitted || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("large event page was not bounded: %+v", page)
	}
	firstBody, err := query.ReadEventBody(ctx, firstID, 0, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	if firstBody.Complete || firstBody.TotalBytes != int64(len(body)) || firstBody.Digest != operationDigest(body) {
		t.Fatalf("bad first body page: %+v", firstBody)
	}
	secondBody, err := query.ReadEventBody(ctx, firstID, firstBody.NextOffset, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	thirdBody, err := query.ReadEventBody(ctx, firstID, secondBody.NextOffset, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	fourthBody, err := query.ReadEventBody(ctx, firstID, thirdBody.NextOffset, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	fifthBody, err := query.ReadEventBody(ctx, firstID, fourthBody.NextOffset, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	joined := append(append(append(append(append([]byte{}, firstBody.Data...), secondBody.Data...), thirdBody.Data...), fourthBody.Data...), fifthBody.Data...)
	if !fifthBody.Complete || operationDigest(joined) != operationDigest(body) {
		t.Fatal("event body pages did not reconstruct the original payload")
	}
	secondPage, err := query.ReadEvents(ctx, EventFilter{ExecutionID: "exec", AttemptID: "attempt"}, page.NextCursor, 1)
	if err != nil || len(secondPage.Events) != 1 || string(secondPage.Events[0].Payload) != "small" {
		t.Fatalf("second event page: %+v %v", secondPage, err)
	}
	if _, err = query.ReadEvents(ctx, EventFilter{ExecutionID: "other"}, page.NextCursor, 1); err == nil {
		t.Fatal("cursor was reused with a different filter")
	} else if cursorErr, ok := err.(*CursorError); !ok || cursorErr.Code != "cursor_invalidated" {
		t.Fatalf("bad cursor error: %v", err)
	}
}

func TestOperationIdempotencyKeepsOneDurableIntent(t *testing.T) {
	ctx := context.Background()
	store := openDomainTestStore(t)
	project := "repo:ops"
	request := []byte(`{"attempt_id":"attempt-1"}`)
	digest := operationDigest(request)
	base := OperationRecord{ProjectIdentity: project, OperationID: "operation-1", IdempotencyKey: "start:attempt-1", Kind: "start_execution", Status: "pending", PayloadDigest: digest, Request: request}
	err := store.Transact(ctx, func(tx DomainTx) error {
		_, created, err := tx.PutOperation(base)
		if err == nil && !created {
			t.Fatal("initial operation was not created")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := base
	updated.Status = "in_flight"
	err = store.Transact(ctx, func(tx DomainTx) error {
		_, created, err := tx.PutOperation(updated)
		if err == nil && created {
			t.Fatal("retry created a second operation")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	conflict := base
	conflict.Request = []byte(`{"attempt_id":"other"}`)
	conflict.PayloadDigest = operationDigest(conflict.Request)
	err = store.Transact(ctx, func(tx DomainTx) error { _, _, err := tx.PutOperation(conflict); return err })
	if err == nil {
		t.Fatal("same idempotency key with a different payload was accepted")
	}
	var status string
	if err = store.db.QueryRowContext(ctx, `SELECT status FROM operations WHERE project_identity=? AND operation_id=?`, project, base.OperationID).Scan(&status); err != nil || status != "in_flight" {
		t.Fatalf("operation status was not retained: %s %v", status, err)
	}
}

func TestQueuedLocalDecisionAnswerIsRejectedWithoutAContinuationCapability(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	project := "repo:decision-queue"
	db, err := OpenSQLiteStore(filepath.Join(dir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runID := strings.Repeat("d", 32)
	logPath := filepath.Join(dir, "runs", runID, "events.jsonl")
	decision := &DecisionState{ID: "decision-1", AttemptID: runID, Version: 1, Request: DecisionRequest{Question: "Choose", Options: []DecisionOption{{ID: "yes", Label: "Yes"}}}, Status: "pending"}
	run := &Run{TaskID: "beads-1", Title: "question", RunID: runID, AttemptID: runID, Claimant: "center", Host: "local", Backend: string(BackendLocal), Protocol: "local_process", BeadsStatus: "in_progress", ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision", Decision: decision, LogPath: &logPath}
	runtime := Runtime{SchemaVersion: 5, Repository: Repository{Identity: project, Path: "repo", Remote: "origin", Branch: "main"}, Service: ServiceState{InstanceID: "service-instance", Claimant: "center"}, Runs: []*Run{run}, Control: ControlState{DispatchMode: "auto", Revision: 1}}
	service := &Service{Config: Config{RunDir: dir, RuntimeFile: "runtime.json", Claimant: "center", identity: project}, State: runtime, domainStore: db, store: &Store{Path: filepath.Join(dir, "runtime.json"), Domain: db, Project: project}}
	if err = service.save(); err != nil {
		t.Fatal(err)
	}
	answer := DecisionAnswer{DecisionID: decision.ID, RunID: runID, OptionID: "yes", AnsweredBy: "alice", ExpectedDecisionVersion: decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: runtime.Service.InstanceID}
	payload, _ := json.Marshal(answer)
	commandService := CommandService{Store: db, Project: project}
	_, created, err := commandService.AnswerDecision(ctx, decision.ID, 1, "answer:decision-1:v1", "alice", payload)
	if err != nil || !created {
		t.Fatalf("enqueue answer: created=%t err=%v", created, err)
	}
	if err = service.processPendingCommands(ctx); err != nil {
		t.Fatal(err)
	}
	if run.Decision.Status != "pending" || run.Decision.Version != 1 || run.Decision.AnsweredBy != "" {
		t.Fatalf("unsupported local continuation accepted an answer: %+v", run.Decision)
	}
	commands, err := db.Commands(ctx, project, "rejected", 10)
	if err != nil || len(commands) != 1 || commands[0].Kind != "answer_decision" || !strings.Contains(string(commands[0].Result), "unsupported") {
		t.Fatalf("unsupported answer command was not rejected with its reason: %+v %v", commands, err)
	}
	snapshot, err := db.Query(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Decisions) != 1 || snapshot.Decisions[0].Status != "pending" || snapshot.Decisions[0].Version != 1 {
		t.Fatalf("unsupported continuation changed authoritative decision state: %+v", snapshot.Decisions)
	}
}

func TestDecisionAnswerFirstWinsIdempotencyAndConflictAreTransactional(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	project := "repo:decision-first-wins"
	db, err := OpenSQLiteStore(filepath.Join(dir, "steerline.db"))
	require(t, err)
	defer db.Close()
	runID := strings.Repeat("a", 32)
	decisionID := strings.Repeat("b", 32)
	decision := &DecisionState{ID: decisionID, AttemptID: runID, Version: 1, Request: DecisionRequest{Question: "Choose", Options: []DecisionOption{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}}}, Status: "pending"}
	run := &Run{TaskID: "task-1", RunID: runID, AttemptID: runID, ControlRevision: 4, Claimant: "alice", Host: "orca", Backend: string(BackendOrca), Protocol: string(ProtocolOrcaTerminalV2), BeadsStatus: "in_progress", ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision", Decision: decision}
	runtimeState := Runtime{SchemaVersion: 5, Repository: Repository{Identity: project, Path: "repo", Remote: "origin", Branch: "main"}, Service: ServiceState{InstanceID: "service-instance", Status: "running", Ready: true, Claimant: "alice"}, Runs: []*Run{run}, Control: ControlState{DispatchMode: "manual", Revision: 2}}
	store := &Store{Path: filepath.Join(dir, "runtime.json"), Domain: db, Project: project}
	require(t, store.Publish(&runtimeState))
	db2, err := OpenSQLiteStore(filepath.Join(dir, "steerline.db"))
	require(t, err)
	defer db2.Close()
	commands1 := CommandService{Store: db, Project: project}
	commands2 := CommandService{Store: db2, Project: project}
	answer := DecisionAnswer{DecisionID: decisionID, RunID: runID, OptionID: "yes", Note: "same", AnsweredBy: "alice", ExpectedDecisionVersion: 1, ExpectedRunRevision: 4, ServiceInstanceID: "service-instance"}
	answerFromSecondClient := answer
	answerFromSecondClient.AnsweredBy = "bob"
	type submitResult struct {
		command CommandRecord
		created bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan submitResult, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		command, created, submitErr := commands1.AnswerDecisionWithID(ctx, strings.Repeat("c", 32), decisionID, 1, decisionAnswerKey(answer), "alice", mustJSON(answer))
		results <- submitResult{command: command, created: created, err: submitErr}
	}()
	go func() {
		defer wait.Done()
		<-start
		command, created, submitErr := commands2.AnswerDecisionWithID(ctx, strings.Repeat("d", 32), decisionID, 1, decisionAnswerKey(answerFromSecondClient), "bob", mustJSON(answerFromSecondClient))
		results <- submitResult{command: command, created: created, err: submitErr}
	}()
	close(start)
	wait.Wait()
	firstResult, secondResult := <-results, <-results
	if firstResult.err != nil || secondResult.err != nil {
		t.Fatalf("concurrent answer submissions failed: first=%v second=%v", firstResult.err, secondResult.err)
	}
	if firstResult.command.CommandID != secondResult.command.CommandID || firstResult.created == secondResult.created {
		t.Fatalf("two clients did not converge on one first-wins command: first=%+v second=%+v", firstResult, secondResult)
	}
	first := firstResult.command
	if !firstResult.created {
		first = secondResult.command
	}
	var acceptedAnswer DecisionAnswer
	require(t, decode(first.Payload, &acceptedAnswer))
	first.Status = "in_flight"
	require(t, commands1.SetStatus(ctx, first, "in_flight", nil))
	var accepted DecisionRecord
	require(t, db.Transact(ctx, func(tx DomainTx) error {
		var acceptErr error
		accepted, _, acceptErr = tx.AcceptDecisionAnswer(first, acceptedAnswer)
		return acceptErr
	}))
	if accepted.Status != "answer_saved" || accepted.Version != 2 {
		t.Fatalf("answer acceptance fact was not atomically recorded: %+v", accepted)
	}
	repeat, created, err := commands1.AnswerDecisionWithID(ctx, strings.Repeat("e", 32), decisionID, 1, decisionAnswerKey(answer), "alice", mustJSON(answer))
	require(t, err)
	if created || repeat.CommandID != first.CommandID || repeat.Status != "accepted" {
		t.Fatalf("same accepted answer did not return its original result: %+v created=%v", repeat, created)
	}
	different := answer
	different.OptionID, different.Note = "no", "different"
	if _, created, err = commands1.AnswerDecisionWithID(ctx, strings.Repeat("f", 32), decisionID, 1, decisionAnswerKey(different), "alice", mustJSON(different)); !errors.Is(err, ErrAnswerConflict) || created {
		t.Fatalf("different answer did not return answer_conflict: created=%v err=%v", created, err)
	}
	projected, err := db.Query(ctx, project)
	require(t, err)
	if len(projected.Decisions) != 1 || projected.Decisions[0].Status != "answer_saved" || projected.Decisions[0].Version != 2 {
		t.Fatalf("conflict overwrote the first accepted answer: %+v", projected.Decisions)
	}
}

type decisionChildResult struct {
	CommandID string `json:"command_id"`
	Created   bool   `json:"created"`
	Error     string `json:"error,omitempty"`
}

func TestDecisionAnswerCommandClientProcess(t *testing.T) {
	if os.Getenv("STEERLINE_PR5_ANSWER_CHILD") != "1" {
		return
	}
	db, err := OpenSQLiteStore(os.Getenv("STEERLINE_PR5_ANSWER_DB"))
	if err != nil {
		t.Fatal(err)
	}
	project, decisionID, commandID, requester := os.Getenv("STEERLINE_PR5_ANSWER_PROJECT"), os.Getenv("STEERLINE_PR5_ANSWER_DECISION"), os.Getenv("STEERLINE_PR5_ANSWER_COMMAND"), os.Getenv("STEERLINE_PR5_ANSWER_BY")
	var answer DecisionAnswer
	if err = json.Unmarshal([]byte(os.Getenv("STEERLINE_PR5_ANSWER_PAYLOAD")), &answer); err != nil {
		t.Fatal(err)
	}
	command, created, submitErr := (CommandService{Store: db, Project: project}).AnswerDecisionWithID(context.Background(), commandID, decisionID, answer.ExpectedDecisionVersion, decisionAnswerKey(answer), requester, mustJSON(answer))
	result := decisionChildResult{CommandID: command.CommandID, Created: created}
	if submitErr != nil {
		result.Error = submitErr.Error()
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(os.Getenv("STEERLINE_PR5_ANSWER_OUTPUT"), data, 0600); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
}

func TestDecisionAnswerSeparateClientProcessesUseOneFirstWinsCommand(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	project := "repo:decision-process-race"
	dbPath := filepath.Join(dir, "steerline.db")
	db, err := OpenSQLiteStore(dbPath)
	require(t, err)
	runID, decisionID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	decision := &DecisionState{ID: decisionID, AttemptID: runID, Version: 1, Request: DecisionRequest{Question: "Choose", Options: []DecisionOption{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}}}, Status: "pending"}
	run := &Run{TaskID: "task", RunID: runID, AttemptID: runID, ControlRevision: 3, Claimant: "service", Host: "orca", Backend: string(BackendOrca), Protocol: string(ProtocolOrcaTerminalV2), ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision", Decision: decision}
	runtimeState := Runtime{SchemaVersion: 5, Repository: Repository{Identity: project, Path: "repo", Remote: "origin", Branch: "main"}, Service: ServiceState{InstanceID: "instance", Status: "running", Ready: true, Claimant: "service"}, Runs: []*Run{run}, Control: ControlState{DispatchMode: "manual", Revision: 1}}
	require(t, (&Store{Path: filepath.Join(dir, "runtime.json"), Domain: db, Project: project}).Publish(&runtimeState))
	_ = db.Close()
	by := []string{"alice", "bob"}
	commands := []string{strings.Repeat("3", 32), strings.Repeat("4", 32)}
	outputPaths := []string{filepath.Join(dir, "client-a.json"), filepath.Join(dir, "client-b.json")}
	children := make([]*exec.Cmd, 2)
	var childOutput [2]bytes.Buffer
	for i := range children {
		answer := DecisionAnswer{DecisionID: decisionID, RunID: runID, OptionID: "yes", Note: "same", AnsweredBy: by[i], ExpectedDecisionVersion: 1, ExpectedRunRevision: 3, ServiceInstanceID: "instance"}
		payload, _ := json.Marshal(answer)
		executable, execErr := os.Executable()
		require(t, execErr)
		children[i] = exec.Command(executable, "-test.run=^TestDecisionAnswerCommandClientProcess$")
		children[i].Stdout, children[i].Stderr = &childOutput[i], &childOutput[i]
		children[i].Env = append(os.Environ(), "STEERLINE_PR5_ANSWER_CHILD=1", "STEERLINE_PR5_ANSWER_DB="+dbPath, "STEERLINE_PR5_ANSWER_PROJECT="+project, "STEERLINE_PR5_ANSWER_DECISION="+decisionID, "STEERLINE_PR5_ANSWER_COMMAND="+commands[i], "STEERLINE_PR5_ANSWER_BY="+by[i], "STEERLINE_PR5_ANSWER_PAYLOAD="+string(payload), "STEERLINE_PR5_ANSWER_OUTPUT="+outputPaths[i])
		require(t, children[i].Start())
	}
	for i, child := range children {
		if waitErr := child.Wait(); waitErr != nil {
			t.Fatalf("answer client process %d failed: %v %s", i, waitErr, childOutput[i].String())
		}
	}
	var results [2]decisionChildResult
	for i, path := range outputPaths {
		data, readErr := os.ReadFile(path)
		require(t, readErr)
		require(t, json.Unmarshal(data, &results[i]))
		if results[i].Error != "" {
			t.Fatalf("answer client process %d returned an error: %s", i, results[i].Error)
		}
	}
	created := 0
	for _, result := range results {
		if result.Created {
			created++
		}
	}
	if created != 1 || results[0].CommandID == "" || results[0].CommandID != results[1].CommandID {
		t.Fatalf("two OS client processes did not converge on one first-wins command: %+v", results)
	}
	store, err := OpenSQLiteStore(dbPath)
	require(t, err)
	defer store.Close()
	commandsPending, err := store.Commands(ctx, project, "pending", 10)
	require(t, err)
	if len(commandsPending) != 1 || commandsPending[0].CommandID != results[0].CommandID {
		t.Fatalf("command history contains duplicate answer writes: %+v", commandsPending)
	}
}
