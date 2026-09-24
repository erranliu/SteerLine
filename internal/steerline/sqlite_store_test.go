package steerline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func openDomainTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSQLiteAllowsParallelExecutionsInOneWorkflow(t *testing.T) {
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
		for _, id := range []string{"exec-a", "exec-b"} {
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
	snapshot, err := s.Query(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.WorkflowRuns) != 1 || len(snapshot.Executions) != 2 || len(snapshot.Attempts) != 2 {
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
		return tx.PutDecision(DecisionRecord{ProjectIdentity: project, DecisionID: "decision", Scope: "execution", WorkflowRunID: "wf", ExecutionID: "exec", AttemptID: "attempt", QuestionID: "q1", Status: "pending", Version: 1})
	})
	if err != nil {
		t.Fatal(err)
	}
	commands := CommandService{Store: s, Project: project}
	first, created, err := commands.AnswerDecision(ctx, "decision", 1, "answer-key-a", "alice", []byte(`{"choice":"yes"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first answer was not enqueued")
	}
	retry, created, err := commands.AnswerDecision(ctx, "decision", 1, "answer-key-a", "alice", []byte(`{"choice":"yes"}`))
	if err != nil {
		t.Fatal(err)
	}
	if created || retry.CommandID != first.CommandID {
		t.Fatalf("idempotent retry changed command: %+v %+v", first, retry)
	}
	if _, _, err = commands.AnswerDecision(ctx, "decision", 1, "answer-key-b", "bob", []byte(`{"choice":"no"}`)); err == nil || !strings.Contains(err.Error(), "already has an accepted answer") {
		t.Fatalf("second answer should lose version arbitration, got %v", err)
	}
	pending, err := commands.Pending(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].CommandID != first.CommandID {
		t.Fatalf("unexpected pending commands: %+v", pending)
	}
	snapshot, err := s.Query(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Decisions[0].Status != "pending" {
		t.Fatal("command client wrote business state instead of enqueueing an answer")
	}
}

func TestMinimalWorkflowRevisionGatesAndRepairLimit(t *testing.T) {
	run := &WorkflowRunRecord{WorkflowVersion: MinimalWorkflowVersion, Definition: WorkflowDefinition{ID: MinimalWorkflowVersion, Version: MinimalWorkflowVersion, ReviewRequired: true, MaxRepairRounds: 1}, Phase: "executing"}
	base := strings.Repeat("a", 40)
	prepared := WorkspaceEvidence{Repository: "repo", Worktree: "tree", Branch: "codex/test", Base: base, Head: base, VerifiedAt: time.Now().UTC()}
	exec := &ExecutionRecord{Status: "active", CurrentStep: "prepare", Workspace: WorkspaceEvidence{Repository: "repo", Worktree: "tree", Branch: "codex/test", Base: base}}
	if step, err := AdvanceMinimalExecution(run, exec, StepGateResult{StepID: "prepare", Succeeded: true, Workspace: prepared}, nil); err != nil || step != "implement" {
		t.Fatalf("prepare gate: %q %v", step, err)
	}
	exec.CurrentStep = "implement"
	commit := strings.Repeat("b", 40)
	verified := time.Now().UTC()
	implement := StepGateResult{StepID: "implement", Succeeded: true, InputRevision: base, Revision: commit, Workspace: WorkspaceEvidence{Repository: "repo", Worktree: "tree", Branch: "codex/test", Base: exec.Workspace.Base, Head: commit, VerifiedAt: verified}}
	if step, err := AdvanceMinimalExecution(run, exec, implement, nil); err != nil || step != "verify" {
		t.Fatalf("implement gate: %q %v", step, err)
	}
	exec.CurrentStep = "verify"
	verify := StepGateResult{StepID: "verify", Succeeded: true, Revision: commit, Workspace: implement.Workspace, ResultID: "verify-1", Checks: []CheckEvidence{{Name: "unit", Status: "failed", Revision: commit}}}
	if step, err := AdvanceMinimalExecution(run, exec, verify, []string{"unit"}); err != nil || step != "implement" || exec.RepairRounds != 1 {
		t.Fatalf("failed deterministic check did not enter repair: step=%q err=%v rounds=%d", step, err, exec.RepairRounds)
	}
	verify.Checks[0].Status = "passed"
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

func TestMinimalDecisionAnswerResumesTheSameStepAsANewAttempt(t *testing.T) {
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
	answer, err := json.Marshal(DecisionAnswer{DecisionID: decision.ID, RunID: s.State.Runs[0].RunID, OptionID: "keep", AnsweredBy: "tester"})
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
	if len(queued) != 1 || queued[0].Status != "accepted" {
		t.Fatalf("decision answer command was not accepted: %+v", queued)
	}
	if err = s.applyDecisions(ctx); err != nil {
		t.Fatal(err)
	}
	if s.worker == nil || s.State.Runs[0].Decision.Status != "applied" {
		t.Fatalf("minimal step was not resumed: worker=%v decision=%+v run_error=%s", s.worker != nil, s.State.Runs[0].Decision, deref(s.State.Runs[0].Error))
	}
	var resumed, prior *StepAttemptRecord
	for i := range s.State.Attempts {
		attempt := &s.State.Attempts[i]
		if attempt.StepID == "implement" && attempt.AttemptNumber == 1 {
			prior = attempt
		}
		if attempt.StepID == "implement" && attempt.AttemptNumber == 2 {
			resumed = attempt
		}
	}
	wantPrerequisites := []string{}
	if prior != nil {
		wantPrerequisites = append(wantPrerequisites, prior.PrerequisiteIDs...)
		wantPrerequisites = append(wantPrerequisites, prior.ResultID)
	}
	if resumed == nil || prior == nil || resumed.Status != "running" || !sameStrings(resumed.PrerequisiteIDs, wantPrerequisites) {
		t.Fatalf("resumed step attempt=%+v, all attempts=%+v", resumed, s.State.Attempts)
	}
	finishWorker(t, s)
	if s.State.Executions[0].CurrentStep != "verify" || s.State.Executions[0].CandidateRevision == "" {
		t.Fatalf("decision continuation did not finish implement: %+v", s.State.Executions[0])
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

func TestQueuedDecisionAnswerIsAppliedByTheCenterWriter(t *testing.T) {
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
	decision := &DecisionState{ID: "decision-1", Version: 1, Request: DecisionRequest{Question: "Choose", Options: []DecisionOption{{ID: "yes", Label: "Yes"}}}, Status: "pending", RequestPath: filepath.Join(dir, "runs", runID, "decision.request.json"), AnswerPath: filepath.Join(dir, "runs", runID, "decision.answer.json")}
	run := &Run{TaskID: "beads-1", Title: "question", RunID: runID, AttemptID: runID, Claimant: "center", Host: "local", Backend: "local_process", Protocol: "local_process", BeadsStatus: "in_progress", ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision", Decision: decision, LogPath: &logPath}
	runtime := Runtime{SchemaVersion: 5, Repository: Repository{Identity: project, Path: "repo", Remote: "origin", Branch: "main"}, Service: ServiceState{Claimant: "center"}, Runs: []*Run{run}, Control: ControlState{DispatchMode: "auto", Revision: 1}}
	service := &Service{Config: Config{RunDir: dir, RuntimeFile: "runtime.json", Claimant: "center", identity: project}, State: runtime, domainStore: db, store: &Store{Path: filepath.Join(dir, "runtime.json"), Domain: db, Project: project}}
	if err = service.save(); err != nil {
		t.Fatal(err)
	}
	answer := DecisionAnswer{DecisionID: decision.ID, RunID: runID, OptionID: "yes", AnsweredBy: "alice"}
	payload, _ := json.Marshal(answer)
	commandService := CommandService{Store: db, Project: project}
	_, created, err := commandService.AnswerDecision(ctx, decision.ID, 1, "answer:decision-1:v1", "alice", payload)
	if err != nil || !created {
		t.Fatalf("enqueue answer: created=%t err=%v", created, err)
	}
	if err = service.processPendingCommands(ctx); err != nil {
		t.Fatal(err)
	}
	if run.Decision.Status != "answer_saved" || run.Decision.Version != 2 || run.Decision.AnsweredBy != "alice" {
		t.Fatalf("center did not apply the queued answer: %+v", run.Decision)
	}
	saved, err := ReadDecisionAnswer(run)
	if err != nil || saved.OptionID != "yes" {
		t.Fatalf("answer file not materialized: %+v %v", saved, err)
	}
	commands, err := db.Commands(ctx, project, "accepted", 10)
	if err != nil || len(commands) != 1 || commands[0].Kind != "answer_decision" {
		t.Fatalf("answer command was not acknowledged: %+v %v", commands, err)
	}
	snapshot, err := db.Query(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Decisions) != 1 || snapshot.Decisions[0].Status != "answer_saved" || snapshot.Decisions[0].Version != 2 {
		t.Fatalf("decision write did not commit with the runtime projection: %+v", snapshot.Decisions)
	}
}
