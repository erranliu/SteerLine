package steerline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAuditInvalidGatesDoNotMutateExecution(t *testing.T) {
	base, head := strings.Repeat("a", 40), strings.Repeat("b", 40)
	workspace := WorkspaceEvidence{Repository: "repo", Worktree: "tree", Branch: "branch", Base: base, Head: head, VerifiedAt: now()}
	for _, name := range []string{"missing_workspace_receipt", "stale_review_changes", "unverified_review", "empty_candidate_publication", "early_publication"} {
		t.Run(name, func(t *testing.T) {
			wf := WorkflowRunRecord{WorkflowVersion: MinimalWorkflowVersion, Definition: DefaultMinimalWorkflowDefinition(true), Phase: "executing"}
			execution := ExecutionRecord{Status: "active", CurrentStep: "review", Workspace: workspace, CandidateRevision: head, VerifiedRevision: head}
			result := StepGateResult{StepID: "review", Succeeded: true, ResultID: "result", Revision: head, Workspace: workspace, ReviewVerdict: "changes_requested"}
			switch name {
			case "missing_workspace_receipt":
				execution.CurrentStep = "implement"
				result.StepID = "implement"
				result.InputRevision = head
				result.Workspace.VerifiedAt = execution.CreatedAt
			case "stale_review_changes":
				result.Revision = base
			case "unverified_review":
				execution.VerifiedRevision = ""
			case "empty_candidate_publication":
				execution.CurrentStep = "publish_pr"
				execution.CandidateRevision = ""
				execution.VerifiedRevision = ""
				execution.Workspace.Head = ""
				result = StepGateResult{StepID: "publish_pr", Succeeded: true, PRURL: "https://example.invalid/pr/1"}
			case "early_publication":
				result.PRURL = "https://example.invalid/pr/1"
			}
			beforeExecution, beforeWorkflow := execution, wf
			if _, err := AdvanceMinimalExecution(&wf, &execution, result, []string{"unit"}); err == nil {
				t.Fatal("invalid gate was accepted")
			}
			if !reflect.DeepEqual(beforeExecution, execution) || !reflect.DeepEqual(beforeWorkflow, wf) {
				t.Fatal("rejected gate mutated domain state")
			}
		})
	}
}

func TestAuditPRHeadCheckedBeforeExternalTaskClosure(t *testing.T) {
	f := newFixture(t, task("t-1"))
	db := readFake(f.db)
	db.Tasks[0].Status = "in_progress"
	db.Tasks[0].Assignee = f.c.Claimant
	writeFake(f.db, db)
	f.pr("1", "merged") // Fake platform omits head evidence: completion must refuse.
	s := f.service(t)
	defer s.Close()
	project, runID, executionID := f.c.identity, newID(), newID()
	head := strings.Repeat("a", 40)
	s.State.Runs = []*Run{{TaskID: "t-1", DomainTaskID: "1", RunID: runID, ExecutionID: executionID, WorkflowVersion: MinimalWorkflowVersion, Claimant: f.c.Claimant, ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_pr_merge", PR: PRState{URL: ptr(f.server.URL + "/owner/repo/pulls/1")}}}
	s.State.Tasks = []TaskRecord{{ProjectIdentity: project, TaskID: "1", Status: "in_progress", Version: 1}}
	s.State.WorkflowRuns = []WorkflowRunRecord{{ProjectIdentity: project, TaskID: "1", WorkflowRunID: runID, WorkflowVersion: MinimalWorkflowVersion, Definition: DefaultMinimalWorkflowDefinition(true), Phase: "waiting_pr_merge", SelectedResult: executionID}}
	s.State.Executions = []ExecutionRecord{{ProjectIdentity: project, ExecutionID: executionID, WorkflowRunID: runID, Status: "succeeded", CurrentStep: "waiting_pr_merge", CandidateRevision: head, VerifiedRevision: head, ReviewedRevision: head}}
	require(t, s.pollPRs(context.Background()))
	if readFake(f.db).Tasks[0].Status != "in_progress" {
		t.Fatal("task was closed before checking PR head")
	}
	if s.State.Runs[0].FinishTaskStatus != "blocked" {
		t.Fatal("mismatched publication did not block the workflow")
	}
}

func TestAuditManualRecoveryCannotBypassMinimalGates(t *testing.T) {
	run := &Run{RunID: newID(), WorkflowVersion: MinimalWorkflowVersion, ExecutionStatus: "failed", FinishTaskStatus: "blocked"}
	s := &Service{State: Runtime{Runs: []*Run{run}}}
	s.State.Control.DispatchMode = "manual"
	if err := s.startRetry(context.Background(), run.RunID); err == nil || !strings.Contains(err.Error(), "minimal-v1") {
		t.Fatalf("retry bypass: %v", err)
	}
	if err := s.acceptResultBytes(context.Background(), run.RunID, newID(), "audit", "confirmed", []byte("{}"), operationDigest([]byte("{}"))); err == nil || !strings.Contains(err.Error(), "minimal-v1") {
		t.Fatalf("accept-result bypass: %v", err)
	}
	if err := s.markStoppedCore(run.RunID, "audit", "confirmed"); err == nil || !strings.Contains(err.Error(), "minimal-v1") {
		t.Fatalf("mark-stopped bypass: %v", err)
	}
}

func TestAuditProjectionFailureRetainsDetachedCommittedSnapshot(t *testing.T) {
	db := openDomainTestStore(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.json")
	require(t, os.Mkdir(path, 0700)) // export fails while SQLite commit succeeds
	state := Runtime{SchemaVersion: 5, Repository: Repository{Identity: "repo"}, Runs: []*Run{{TaskID: "task", DomainTaskID: "1", RunID: newID(), Title: "committed", ExecutionStatus: "succeeded", FinishTaskStatus: "completed"}}}
	store := Store{Path: path, Domain: db, Project: "repo"}
	require(t, store.Publish(&state))
	if store.ProjectionErr() == nil {
		t.Fatal("test did not induce an export failure")
	}
	state.Runs[0].Title = "uncommitted"
	state.Tasks[0].Status = "discarded"
	require(t, store.Heartbeat())
	data, _, err := db.LoadProjection(context.Background(), "repo")
	require(t, err)
	var committed Runtime
	require(t, json.Unmarshal(data, &committed))
	if committed.Runs[0].Title != "committed" || committed.Tasks[0].Status == "discarded" {
		t.Fatal("heartbeat published scheduler mutations without a domain transaction")
	}
}

func TestAuditCorruptFileProjectionRebuildsFromDatabase(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	s.State.Control.DispatchPaused = true
	require(t, s.save())
	require(t, s.Close())
	require(t, os.WriteFile(filepath.Join(f.c.RunDir, f.c.RuntimeFile), []byte(`{"schema_version":5,`), 0600))
	restarted, err := NewService(f.c)
	require(t, err)
	defer restarted.Close()
	if !restarted.State.Control.DispatchPaused {
		t.Fatal("recovery discarded committed state")
	}
	require(t, restarted.save())
	if _, err = LoadRuntime(filepath.Join(f.c.RunDir, f.c.RuntimeFile)); err != nil {
		t.Fatal(err)
	}
}

func TestAuditAnswerFileCannotAuthorizeResume(t *testing.T) {
	run := decisionRun(t)
	decision, err := WriteDecisionRequest(run, DecisionRequest{Question: "choose", Options: []DecisionOption{{ID: "yes", Label: "Yes"}}})
	require(t, err)
	run.Decision = decision
	run.ExecutionStatus = "succeeded"
	run.FinishTaskStatus = "waiting_decision"
	answerPath := filepath.Join(filepath.Dir(*run.ResultPath), "decision.answer.json")
	require(t, os.WriteFile(answerPath, []byte(`{"decision_id":"`+decision.ID+`","run_id":"`+run.RunID+`","option_id":"yes","answered_by":"outside-client"}`), 0600))
	s := Service{State: Runtime{SchemaVersion: 5, Runs: []*Run{run}}}
	require(t, s.applyDecisions(context.Background()))
	if run.Decision.Status != "pending" || s.worker != nil {
		t.Fatal("a file bypassed command arbitration")
	}
}

func TestAuditInvalidAnswerDoesNotReserveDecisionVersion(t *testing.T) {
	db := openDomainTestStore(t)
	ctx := context.Background()
	project := "repo:answer-validation"
	request := DecisionRequest{Question: "choose", Options: []DecisionOption{{ID: "yes", Label: "Yes"}}}
	run := &Run{TaskID: "1", RunID: "wf", DomainTaskID: "1", ExecutionID: "exec", AttemptID: "attempt", ControlRevision: 5, Host: "orca", Backend: string(BackendOrca), Protocol: string(ProtocolOrcaTerminalV2), WorkflowVersion: "legacy-v1", ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision", Decision: &DecisionState{ID: "decision", AttemptID: "attempt", Version: 1, Status: "pending", Request: request}}
	projection := Runtime{SchemaVersion: 5, Revision: 1, Repository: Repository{Identity: project}, Service: ServiceState{InstanceID: "service", Status: "running", Ready: true}, Runs: []*Run{run}}
	require(t, db.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutTask(TaskRecord{ProjectIdentity: project, TaskID: "1", Status: "open"}); err != nil {
			return err
		}
		if err := tx.PutWorkflowRun(WorkflowRunRecord{ProjectIdentity: project, TaskID: "1", WorkflowRunID: "wf", Phase: "waiting_decision"}); err != nil {
			return err
		}
		if err := tx.PutExecution(ExecutionRecord{ProjectIdentity: project, ExecutionID: "exec", WorkflowRunID: "wf", Status: "waiting_decision"}); err != nil {
			return err
		}
		if err := tx.PutStepAttempt(StepAttemptRecord{ProjectIdentity: project, AttemptID: "attempt", ExecutionID: "exec", StepID: "implement", AttemptNumber: 1, Status: "waiting_decision"}); err != nil {
			return err
		}
		if err := tx.PutDecision(DecisionRecord{ProjectIdentity: project, DecisionID: "decision", WorkflowRunID: "wf", ExecutionID: "exec", AttemptID: "attempt", Scope: "execution", QuestionID: "q", Status: "pending", Version: 1, Request: mustJSON(request)}); err != nil {
			return err
		}
		return tx.PutProjection(project, projection.Revision, mustJSON(projection))
	}))
	commands := CommandService{Store: db, Project: project}
	answer := DecisionAnswer{DecisionID: "decision", RunID: "wf", OptionID: "invalid", AnsweredBy: "alice"}
	if _, _, err := commands.AnswerDecision(ctx, "decision", 1, "invalid", "alice", mustJSON(answer)); err == nil {
		t.Fatal("invalid option was queued")
	}
	answer.OptionID = "yes"
	if _, created, err := commands.AnswerDecision(ctx, "decision", 1, "valid", "alice", mustJSON(answer)); err != nil || !created {
		t.Fatalf("invalid request consumed version reservation: %v", err)
	}
	snapshot, err := db.Query(ctx, project)
	if err != nil || len(snapshot.Decisions) != 1 || snapshot.Decisions[0].Status != "pending" || snapshot.Decisions[0].Version != 1 {
		t.Fatalf("invalid answer changed the authoritative decision: %+v err=%v", snapshot.Decisions, err)
	}
}

func TestAuditSingleExecutionAlsoSerializesDifferentSteps(t *testing.T) {
	db := openDomainTestStore(t)
	ctx := context.Background()
	project := "repo:active"
	require(t, db.Transact(ctx, func(tx DomainTx) error {
		for _, id := range []string{"1", "2"} {
			if err := tx.PutTask(TaskRecord{ProjectIdentity: project, TaskID: id, Status: "open"}); err != nil {
				return err
			}
		}
		if err := tx.PutWorkflowRun(WorkflowRunRecord{ProjectIdentity: project, TaskID: "1", WorkflowRunID: "wf", Phase: "executing"}); err != nil {
			return err
		}
		if err := tx.PutExecution(ExecutionRecord{ProjectIdentity: project, ExecutionID: "exec", WorkflowRunID: "wf", Status: "active"}); err != nil {
			return err
		}
		return tx.PutStepAttempt(StepAttemptRecord{ProjectIdentity: project, AttemptID: "implement", ExecutionID: "exec", StepID: "implement", AttemptNumber: 1, Status: "running"})
	}))
	if err := db.Transact(ctx, func(tx DomainTx) error {
		return tx.PutStepAttempt(StepAttemptRecord{ProjectIdentity: project, AttemptID: "verify", ExecutionID: "exec", StepID: "verify", AttemptNumber: 1, Status: "planned"})
	}); err == nil {
		t.Fatal("concurrent steps in the same workspace were allowed")
	}
	if err := db.Transact(ctx, func(tx DomainTx) error {
		return tx.PutWorkflowRun(WorkflowRunRecord{ProjectIdentity: project, TaskID: "1", WorkflowRunID: "another", Phase: "queued"})
	}); err == nil {
		t.Fatal("second active workflow was allowed for the same task")
	}
}

func TestAuditRejectedVerificationIsFailedAndCannotPublish(t *testing.T) {
	f := newFixture(t, task("t-19"))
	exe, err := os.Executable()
	require(t, err)
	git, err := exec.LookPath("git")
	require(t, err)
	contract := func(step string) StepContract {
		return StepContract{Provider: "command", Entry: "builtin:" + step, Version: "test-v1", InputSchema: MinimalStepInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "local", Timeout: 10 * time.Second, Command: []string{exe, "--fake", "minimal-step", f.db, git}}
	}
	f.c.WorkflowDefinition = ptr(DefaultMinimalWorkflowDefinition(true))
	f.c.ProjectSteps = &ProjectSteps{Implement: contract("implement"), Verify: contract("verify"), Review: contract("review"), PublishPR: contract("publish_pr")}
	f.c.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	s := f.service(t)
	defer s.Close()
	ctx := context.Background()
	require(t, s.dispatch(ctx))
	finishWorker(t, s)
	db := readFake(f.db)
	db.Behavior = "minimal-no-checks"
	writeFake(f.db, db)
	require(t, s.resumeMinimalSteps(ctx))
	for ev := range s.worker.Events {
		require(t, s.event(ev))
	}
	if err = s.finished(<-s.worker.Done); err == nil {
		t.Fatal("verification without check receipts was accepted")
	}
	run := s.State.Runs[0]
	if s.minimalAttempt(run, run.AttemptID).Status != "failed" || run.FinishTaskStatus != "blocked" {
		t.Fatal("rejected gate was reported as succeeded")
	}
	for _, attempt := range s.State.Attempts {
		if attempt.StepID == "publish_pr" {
			t.Fatal("invalid verification generated a publication intent")
		}
	}
}

func TestAuditEventsAreDurableWithoutDisplayJournal(t *testing.T) {
	db := openDomainTestStore(t)
	dir := t.TempDir()
	run := &Run{TaskID: "task", DomainTaskID: "1", RunID: newID(), AttemptID: "attempt-one", ExecutionStatus: "running", FinishTaskStatus: "pending"}
	s := Service{Config: Config{identity: "repo"}, State: Runtime{SchemaVersion: 5, Repository: Repository{Identity: "repo"}, Runs: []*Run{run}}, store: &Store{Path: filepath.Join(dir, "runtime.json"), Domain: db, Project: "repo"}, domainStore: db, active: run}
	s.note(run, "steer", "gate", "pending", "gate pending", "", nil)
	require(t, s.event(WorkerEvent{Activity: ActivityEvent{Source: "codex", Kind: "message", Detail: "worker message"}}))
	events, err := db.Events(context.Background(), "repo", run.ExecutionID, "attempt-one", 0, 10)
	require(t, err)
	if len(events) != 2 {
		t.Fatalf("canonical events depended on display journal: %d", len(events))
	}
	if events[1].AttemptID != "attempt-one" || events[1].Type != "activity.message" {
		t.Fatalf("worker event binding lost: %+v", events[1])
	}
}

func TestAuditDecisionRequiresApplicationEvidence(t *testing.T) {
	db := openDomainTestStore(t)
	dir := t.TempDir()
	project := "repo:proof"
	run := &Run{RunID: newID(), TaskID: "task", DomainTaskID: "1", ExecutionID: "execution", AttemptID: "attempt", WorkflowVersion: MinimalWorkflowVersion, ExecutionStatus: "running", FinishTaskStatus: "pending", StepResultPath: ptr(filepath.Join(dir, "result.json")), Decision: &DecisionState{ID: "decision", AttemptID: "attempt", Status: "reply_accepted", Version: 2, Request: DecisionRequest{Question: "choose", AllowFreeText: true}, Note: "yes", AnsweredBy: "alice"}}
	wf := WorkflowRunRecord{ProjectIdentity: project, TaskID: "1", WorkflowRunID: run.RunID, WorkflowVersion: MinimalWorkflowVersion, Definition: DefaultMinimalWorkflowDefinition(true), Phase: "executing"}
	execution := ExecutionRecord{ProjectIdentity: project, ExecutionID: "execution", WorkflowRunID: run.RunID, Status: "active", CurrentStep: "implement"}
	attempt := StepAttemptRecord{ProjectIdentity: project, AttemptID: "attempt", ExecutionID: "execution", StepID: "implement", AttemptNumber: 1, Status: "running"}
	s := Service{Config: Config{identity: project}, State: Runtime{SchemaVersion: 5, Repository: Repository{Identity: project}, Runs: []*Run{run}, Tasks: []TaskRecord{{ProjectIdentity: project, TaskID: "1", Status: "in_progress"}}, WorkflowRuns: []WorkflowRunRecord{wf}, Executions: []ExecutionRecord{execution}, Attempts: []StepAttemptRecord{attempt}}, store: &Store{Path: filepath.Join(dir, "runtime.json"), Domain: db, Project: project}, domainStore: db, active: run}
	result := MinimalStepResultEnvelope{SchemaVersion: 1, ProjectIdentity: project, TaskID: "1", WorkflowRunID: run.RunID, ExecutionID: "execution", AttemptID: "attempt", Result: StepGateResult{StepID: "implement", Succeeded: true, ResultID: "result"}}
	require(t, os.WriteFile(*run.StepResultPath, mustJSON(result), 0600))
	if err := s.finishMinimalStep(nil); err == nil || !strings.Contains(err.Error(), "application evidence") {
		t.Fatalf("missing application proof accepted: %v", err)
	}
	if run.Decision.Status != "failed" || run.Decision.AppliedAt != nil || run.ExecutionStatus != "failed" || run.FinishTaskStatus != "blocked" {
		t.Fatalf("missing application evidence was not recorded as failed/blocked: decision=%+v execution=%s finish=%s", run.Decision, run.ExecutionStatus, run.FinishTaskStatus)
	}
}

func TestAuditCommandOnlyAndOrcaDoNotRequireLocalCodex(t *testing.T) {
	if (Config{Host: "orca"}).requiresLocalCodex() {
		t.Fatal("Orca requires an unused local Codex")
	}
	c := Config{Host: "local", WorkflowDefinition: ptr(DefaultMinimalWorkflowDefinition(false)), ProjectSteps: &ProjectSteps{Implement: StepContract{Provider: "command"}, Verify: StepContract{Provider: "command"}, PublishPR: StepContract{Provider: "command"}}}
	if c.requiresLocalCodex() {
		t.Fatal("command-only workflow requires an unused local Codex")
	}
}

func TestAuditTaskNumberCannotBeRebound(t *testing.T) {
	db := openDomainTestStore(t)
	ctx := context.Background()
	task := TaskRecord{ProjectIdentity: "repo", TaskID: "17", Status: "open", ExternalProvider: "beads", ExternalProject: "source", ExternalID: "task-a"}
	require(t, db.Transact(ctx, func(tx DomainTx) error { return tx.PutTask(task) }))
	task.ExternalID = "task-b"
	if err := db.Transact(ctx, func(tx DomainTx) error { return tx.PutTask(task) }); err == nil {
		t.Fatal("same task number was rebound to a different external task")
	}
}

func TestAuditEntryMustExistAtPinnedBase(t *testing.T) {
	f := newFixture(t)
	require(t, os.WriteFile(filepath.Join(f.c.Repository, "uncommitted-step.py"), []byte("print('not in base')"), 0600))
	out, err := gitRunner(f.c, f.c.Repository).Output(context.Background(), f.c.Git, "rev-parse", "HEAD")
	require(t, err)
	contract := StepContract{Entry: "builtin:command"}
	workflow := MinimalWorkflowConfig{Definition: DefaultMinimalWorkflowDefinition(false), Steps: ProjectSteps{Implement: contract, Verify: contract, PublishPR: contract}}
	workflow.Steps.Verify.Entry = "uncommitted-step.py"
	if err = f.c.validateMinimalEntriesAtBase(context.Background(), workflow, strings.TrimSpace(string(out))); err == nil {
		t.Fatal("uncommitted entry passed pre-claim validation")
	}
}

func TestAuditStepInputIncludesFailureBodiesAndApplicationSchema(t *testing.T) {
	run := &Run{RunID: "wf", DomainTaskID: "1"}
	execution := &ExecutionRecord{ExecutionID: "exec"}
	result := mustJSON(StepGateResult{StepID: "verify", ResultID: "failed-check", Checks: []CheckEvidence{{Name: "unit", Status: "failed", Revision: strings.Repeat("a", 40)}}})
	input := buildMinimalStepInput("repo", Task{}, run, &WorkflowRunRecord{}, execution, &StepAttemptRecord{PrerequisiteIDs: []string{"failed-check"}}, StepAttemptRecord{ExecutionID: "exec", ResultID: "failed-check", Result: result})
	if len(input.PrerequisiteResults) != 1 || string(input.PrerequisiteResults[0]) != string(result) {
		t.Fatal("repair input omitted failure details")
	}
	var schema map[string]any
	require(t, json.Unmarshal(MinimalStepResultSchema, &schema))
	properties := schema["properties"].(map[string]any)["result"].(map[string]any)["properties"].(map[string]any)
	if properties["applied_decision_id"] == nil || properties["applied_decision_attempt_id"] == nil || properties["applied_decision_execution_id"] == nil || properties["applied_answer_sha256"] == nil || properties["workspace"].(map[string]any)["properties"] == nil {
		t.Fatal("result schema omits application or workspace evidence")
	}
}

func TestMinimalDecisionApplicationEvidenceBindsOriginalAttemptExecutionAndAnswer(t *testing.T) {
	run := &Run{RunID: "workflow", ExecutionID: "execution", Decision: &DecisionState{ID: "decision", AttemptID: "decision-attempt", Status: "reply_accepted", Request: DecisionRequest{Question: "choose", Options: []DecisionOption{{ID: "a", Label: "A"}}}, OptionID: "a", Note: "keep", AnsweredBy: "alice"}}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: run.Decision.OptionID, Note: run.Decision.Note, AnsweredBy: run.Decision.AnsweredBy}
	result := StepGateResult{AppliedDecisionID: run.Decision.ID, AppliedDecisionAttemptID: run.Decision.AttemptID, AppliedDecisionExecutionID: run.ExecutionID, AppliedAnswerSHA256: decisionAnswerDigest(answer)}
	if !minimalDecisionApplicationMatches(run, result) {
		t.Fatal("strictly bound application evidence was rejected")
	}
	result.AppliedDecisionAttemptID = "late-attempt"
	if minimalDecisionApplicationMatches(run, result) {
		t.Fatal("late attempt evidence unlocked the pending decision")
	}
}

func TestAuditExistingOperationCannotBeDispatchedAgain(t *testing.T) {
	payload := map[string]string{"attempt_id": "attempt"}
	data := mustJSON(payload)
	for _, status := range []string{"in_flight", "accepted", "confirmed", "outcome_unknown", "rejected"} {
		s := Service{State: Runtime{Operations: []OperationRecord{{OperationID: "original", IdempotencyKey: "same-key", Kind: "start", Status: status, Request: data, PayloadDigest: operationDigest(data)}}}}
		if _, err := s.startOperation("start", "same-key", payload); err == nil {
			t.Fatalf("%s operation was resent", status)
		}
		if _, err := s.minimalOperation(nil, nil, "start", "same-key", payload); err == nil {
			t.Fatalf("%s step operation was resent", status)
		}
		if len(s.State.Operations) != 1 || s.State.Operations[0].OperationID != "original" {
			t.Fatal("retry invented another operation identity")
		}
	}
}
