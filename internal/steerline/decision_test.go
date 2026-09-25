package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decisionRun(t *testing.T) *Run {
	t.Helper()
	dir := t.TempDir()
	return &Run{
		TaskID:           "t-1",
		RunID:            "0123456789abcdef0123456789abcdef",
		Claimant:         "c",
		Host:             "orca",
		FinishTaskStatus: "waiting_decision",
		ResultPath:       ptr(filepath.Join(dir, "result.json")),
		WorktreePath:     ptr(filepath.Join(dir, "wt")),
	}
}

func TestDecisionRequestAndAnswerRoundTrip(t *testing.T) {
	run := decisionRun(t)
	st, e := WriteDecisionRequest(run, DecisionRequest{
		Question:    "pick",
		Options:     []DecisionOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		Recommended: "a",
	})
	if e != nil {
		t.Fatal(e)
	}
	run.Decision = st
	if entries, e := os.ReadDir(filepath.Dir(*run.ResultPath)); e != nil || len(entries) != 0 {
		t.Fatalf("decision request wrote an answer/request sidecar: entries=%v err=%v", entries, e)
	}
	if _, e := ReadDecisionAnswer(run); e == nil {
		t.Fatal("pending decision unexpectedly had an answer")
	}
	run.Decision.Status, run.Decision.OptionID, run.Decision.AnsweredBy = "answer_saved", "b", "alice"
	a, e := ReadDecisionAnswer(run)
	if e != nil {
		t.Fatal(e)
	}
	if a.OptionID != "b" || a.AnsweredBy != "alice" || a.DecisionID != st.ID {
		t.Fatalf("bad answer: %+v", a)
	}
}

func TestDecisionAnswerRejectsBadInput(t *testing.T) {
	run := decisionRun(t)
	st, e := WriteDecisionRequest(run, DecisionRequest{Question: "q", Options: []DecisionOption{{ID: "a", Label: "A"}}})
	if e != nil {
		t.Fatal(e)
	}
	run.Decision = st
	cases := map[string]DecisionAnswer{
		"missing option": {AnsweredBy: "x"},
		"unknown option": {OptionID: "z", AnsweredBy: "x"},
		"missing by":     {OptionID: "a"},
	}
	for name, a := range cases {
		a.DecisionID, a.RunID = st.ID, run.RunID
		if e := validateDecisionAnswer(&st.Request, &a); e == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestDecisionReplyEligibilityRejectsLocalContinuationAndMissingFixedBinding(t *testing.T) {
	localAttempt := newID()
	local := &Run{RunID: newID(), AttemptID: localAttempt, Backend: string(BackendLocal), Protocol: string(ProtocolLocalProcess), Decision: &DecisionState{ID: newID(), AttemptID: localAttempt, Status: "pending"}}
	err := DecisionReplyEligibility(context.Background(), Config{}, local)
	var backendErr *BackendError
	if !errors.As(err, &backendErr) || backendErr.Code != BackendUnsupported {
		t.Fatalf("local continuation did not report unsupported: %v", err)
	}
	orca := &Run{RunID: newID(), AttemptID: newID(), Backend: string(BackendOrca), Protocol: string(ProtocolOrcaTerminalV2), Decision: &DecisionState{ID: newID(), AttemptID: newID(), Status: "pending"}}
	err = DecisionReplyEligibility(context.Background(), Config{RunDir: t.TempDir()}, orca)
	if !errors.As(err, &backendErr) || backendErr.Code != BackendIdentityMismatch || !strings.Contains(err.Error(), "different execution attempt") {
		t.Fatalf("old-attempt decision was eligible for a new execution: %v", err)
	}
	orca.Decision.AttemptID = orca.AttemptID
	err = DecisionReplyEligibility(context.Background(), Config{RunDir: t.TempDir()}, orca)
	if !errors.As(err, &backendErr) || backendErr.Code != BackendIdentityMismatch {
		t.Fatalf("unbound Orca decision did not fail closed: %v", err)
	}
}

func TestDecisionReplyEligibilityAllowsExactSupervisedBinding(t *testing.T) {
	c := Config{Host: "orca_supervised", RunDir: t.TempDir(), Orca: OrcaConfig{Supervised: &SupervisedOrcaConfig{ConnectionID: "connection-home", EnvironmentID: "environment-home"}}}
	target := ExecutionTarget{ID: "orca-supervised:connection-home", ConnectionID: "connection-home", ExternalRuntime: "environment-home", WorktreeID: "repo::C:/probe", Branch: "probe", BaseRevision: strings.Repeat("a", 40), OrchestrationRun: "run-probe", ExternalTask: "task-probe", Dispatch: "dispatch-probe", ExecutionHost: "remote-host"}
	process := ProcessIdentity{Kind: "orca_supervised", RuntimeID: "remote-runtime", CoordinatorRuntimeID: "center-runtime", OrchestrationRun: target.OrchestrationRun, Task: target.ExternalTask, Dispatch: target.Dispatch, ExecutionHost: target.ExecutionHost, WorktreeID: target.WorktreeID}
	path, branch, base := "C:/probe", target.Branch, target.BaseRevision
	run := &Run{RunID: newID(), AttemptID: newID(), Backend: string(BackendOrca), Protocol: string(ProtocolOrcaSupervised), Agent: "codex", Target: target, Process: &process, WorktreePath: &path, WorktreeBranch: &branch, BaseCommit: &base}
	run.Decision = &DecisionState{ID: newID(), AttemptID: run.AttemptID, Status: "pending"}
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: target, Agent: AgentSpec{Name: "codex"}, RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: path, WorkspaceBranch: branch, BaseCommit: base, External: process}
	if err := (executionRecords{root: c.RunDir}).saveBinding(ref); err != nil {
		t.Fatal(err)
	}
	if err := DecisionReplyEligibility(context.Background(), c, run); err != nil {
		t.Fatalf("exact supervised decision was rejected: %v", err)
	}
	run.Target.Dispatch = "different-dispatch"
	if err := DecisionReplyEligibility(context.Background(), c, run); err == nil {
		t.Fatal("supervised decision with a changed Dispatch was accepted")
	}
}

func TestDecisionRequestValidation(t *testing.T) {
	cases := map[string]DecisionRequest{
		"empty":           {},
		"no options":      {Question: "q"},
		"empty option id": {Question: "q", Options: []DecisionOption{{ID: "", Label: "x"}}},
		"duplicate id":    {Question: "q", Options: []DecisionOption{{ID: "a", Label: "x"}, {ID: "a", Label: "y"}}},
		"bad recommend":   {Question: "q", Options: []DecisionOption{{ID: "a", Label: "x"}}, Recommended: "b"},
	}
	for name, req := range cases {
		c := req
		if e := validateDecisionRequest(&c); e == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestFreeTextDecisionRequiresNote(t *testing.T) {
	request := DecisionRequest{Question: "explain", Options: []DecisionOption{{ID: "a", Label: "A"}}, AllowFreeText: true}
	if e := validateDecisionRequest(&request); e != nil {
		t.Fatal(e)
	}
	custom := DecisionAnswer{DecisionID: "d", RunID: "r", AnsweredBy: "x"}
	if e := validateDecisionAnswer(&request, &custom); e == nil {
		t.Fatal("expected an error without a note")
	}
	custom.Note = "because"
	if e := validateDecisionAnswer(&request, &custom); e != nil {
		t.Fatal(e)
	}
	request.Options = nil
	free := DecisionRequest{Question: "explain", AllowFreeText: true}
	if e := validateDecisionRequest(&free); e != nil {
		t.Fatal(e)
	}
	if e := validateDecisionAnswer(&free, &custom); e != nil {
		t.Fatal(e)
	}
	long := strings.Repeat("界", 4001)
	custom.Note = long
	if e := validateDecisionAnswer(&free, &custom); e == nil || !strings.Contains(e.Error(), "4000 character") {
		t.Fatalf("oversized answer was accepted or silently clipped: %v", e)
	}
}

func TestDecisionRequestRejectsOversizedEvidenceWithoutTruncating(t *testing.T) {
	req := DecisionRequest{Question: strings.Repeat("界", 2001), Options: []DecisionOption{{ID: "a", Label: "A"}}}
	if err := validateDecisionRequest(&req); err == nil || len([]rune(req.Question)) != 2001 {
		t.Fatalf("oversized question was not rejected intact: len=%d err=%v", len([]rune(req.Question)), err)
	}
	req = DecisionRequest{Question: "Q", Context: strings.Repeat("x", 8001), AllowFreeText: true}
	if err := validateDecisionRequest(&req); err == nil || len(req.Context) != 8001 {
		t.Fatalf("oversized context was not rejected intact: len=%d err=%v", len(req.Context), err)
	}
}

func TestAnswerDecisionRejectsV3RuntimeWithoutMutatingIt(t *testing.T) {
	dir := t.TempDir()
	run := decisionRun(t)
	st, e := WriteDecisionRequest(run, DecisionRequest{Question: "q", Options: []DecisionOption{{ID: "a", Label: "A"}}})
	if e != nil {
		t.Fatal(e)
	}
	run.Decision = st
	b, e := json.Marshal(&Runtime{SchemaVersion: 3, Runs: []*Run{run}})
	if e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "runtime.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	c := Config{RunDir: dir, RuntimeFile: "runtime.json"}
	if e := AnswerDecision(c, AnswerOptions{RunID: run.TaskID, Option: "a", By: "bob"}); e == nil || !strings.Contains(e.Error(), "unsupported runtime schema 3") {
		t.Fatalf("v3 answer was accepted: %v", e)
	}
	if _, e = os.Stat(filepath.Join(dir, "runs", run.RunID, "decision.answer.json")); !os.IsNotExist(e) {
		t.Fatalf("rejected old runtime was mutated: %v", e)
	}
	if _, e = LoadRuntime(filepath.Join(dir, "runtime.json")); e == nil {
		t.Fatal("v3 runtime was readable")
	}
}

func TestNewServiceRefusesOldDecisionSidecarWithoutImportingAnswer(t *testing.T) {
	f := newFixture(t, task("t-1"))
	s := f.service(t)
	runID := newID()
	requestPath := filepath.Join(f.c.RunDir, "runs", runID, "decision.request.json")
	answerPath := filepath.Join(f.c.RunDir, "runs", runID, "decision.answer.json")
	if err := os.MkdirAll(filepath.Dir(requestPath), 0700); err != nil {
		t.Fatal(err)
	}
	answerBytes := []byte(`{"decision_id":"old","run_id":"old","option_id":"yes","answered_by":"legacy"}`)
	if err := os.WriteFile(answerPath, answerBytes, 0600); err != nil {
		t.Fatal(err)
	}
	run := &Run{TaskID: "t-1", Title: "legacy sidecar", RunID: runID, AttemptID: runID, Claimant: f.c.Claimant, Host: "local", Backend: string(BackendLocal), Protocol: string(ProtocolLocalProcess), ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision", ResultPath: ptr(filepath.Join(f.c.RunDir, "runs", runID, "result.json")), Decision: &DecisionState{ID: newID(), AttemptID: runID, Version: 1, Status: "pending", Request: DecisionRequest{Question: "choose", Options: []DecisionOption{{ID: "yes", Label: "Yes"}}}, RequestPath: requestPath, AnswerPath: answerPath}}
	s.State.Runs = append(s.State.Runs, run)
	require(t, s.save())
	require(t, s.Close())
	if _, err := NewService(f.c); err == nil || !strings.Contains(err.Error(), "removed sidecar-answer format") {
		t.Fatalf("legacy answer sidecar state was not explicitly refused: %v", err)
	}
	stored, err := os.ReadFile(answerPath)
	require(t, err)
	if string(stored) != string(answerBytes) {
		t.Fatal("legacy answer file was imported, rewritten or deleted")
	}
}

// TestOrcaDecisionResumesRun verifies that terminal delivery without a
// decision-bound application receipt cannot advance the task to PR waiting.
func TestOrcaDecisionResumesRun(t *testing.T) {
	f := newFixture(t, task("t-1"))
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	stateDir := filepath.Join(filepath.Dir(f.c.RunDir), "orca-state")
	if e := os.MkdirAll(stateDir, 0700); e != nil {
		t.Fatal(e)
	}
	f.c.Host = "orca"
	f.c.Agent = "codex"
	f.c.Orca = OrcaConfig{Executable: exe, Args: []string{"--fake", "orca", stateDir}, Shell: "cmd.exe", WaitTimeout: "5s"}
	t.Setenv("STEERLINE_FAKE_ORCA_PR_BASE", f.server.URL)
	t.Setenv("STEERLINE_FAKE_ORCA_DECISION", "1")

	ctx := context.Background()
	s := f.service(t)
	t.Cleanup(func() {
		if s.worker != nil {
			s.shutdown()
		}
		_ = s.Close()
	})
	require(t, s.dispatch(ctx))
	if s.worker == nil {
		t.Fatal("orca worker was not started")
	}
	finishWorker(t, s)

	run := s.State.Runs[0]
	if run.FinishTaskStatus != "waiting_decision" || run.Decision == nil {
		t.Fatalf("run did not pause for a decision: status=%s execution=%s error=%s", run.FinishTaskStatus, run.ExecutionStatus, deref(run.Error))
	}
	if fileExists(filepath.Join(filepath.Dir(*run.ResultPath), "decision.request.json")) {
		t.Fatal("decision request was written outside the SQLite authority")
	}
	if run.PR.URL != nil {
		t.Fatalf("decision run must not bind a PR: %v", *run.PR.URL)
	}

	s.State.Service.Status, s.State.Service.Ready = "running", true
	require(t, s.save())
	_, _, e = (CommandService{Store: s.domainStore, Project: s.Config.identity}).AnswerDecision(ctx, run.Decision.ID, run.Decision.Version, "orca-answer", "tester", mustJSON(DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: "a", Note: "keep compat", AnsweredBy: "tester", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: s.State.Service.InstanceID}))
	require(t, e)
	if run.Decision.Status != "pending" {
		t.Fatalf("answer must not mutate run state: %s", run.Decision.Status)
	}
	beforeReply, err := readFakeOrcaState(stateDir, run.Process.Terminal)
	require(t, err)
	require(t, s.processPendingCommands(ctx))
	if run.Decision.Status != "answer_saved" || s.worker != nil {
		t.Fatalf("answer was not durably accepted separately from continuation: decision=%+v worker=%v", run.Decision, s.worker != nil)
	}
	require(t, s.Close())
	s = f.service(t)
	restartedRun := s.State.Runs[0]
	if restartedRun.Decision == nil || restartedRun.Decision.Status != "answer_saved" || restartedRun.Decision.OptionID != "a" {
		t.Fatalf("service restart did not hydrate the accepted answer: %+v", restartedRun.Decision)
	}
	require(t, s.applyDecisions(ctx))
	run = s.State.Runs[0]
	if run.Decision.Status != "reply_accepted" {
		t.Fatalf("restarted service did not continue the accepted decision: %+v", run.Decision)
	}
	if s.worker == nil {
		t.Fatal("decision did not resume a worker")
	}
	afterReply, err := readFakeOrcaState(stateDir, run.Process.Terminal)
	require(t, err)
	if afterReply.Sends != beforeReply.Sends+1 {
		t.Fatalf("restart did not issue exactly one continuation: before=%d after=%d", beforeReply.Sends, afterReply.Sends)
	}
	finishWorker(t, s)

	run = s.State.Runs[0]
	if run.FinishTaskStatus != "blocked" || run.Decision.Status != "failed" || run.PR.URL != nil || run.Error == nil || !strings.Contains(*run.Error, "not verified") {
		t.Fatalf("terminal send without application evidence was accepted: %+v", run)
	}
	act, e := os.ReadFile(filepath.Join(f.c.RunDir, "runs", run.RunID, "activity.jsonl"))
	require(t, e)
	for _, want := range []string{"decision", "answered", "reply_accepted", "decision application was not verified"} {
		if !strings.Contains(string(act), want) {
			t.Fatalf("activity journal missing %q: %s", want, act)
		}
	}
}

func TestOrcaLostReplyReceiptKeepsOriginalExecutionUnknown(t *testing.T) {
	f := newFixture(t, task("t-1"))
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(filepath.Dir(f.c.RunDir), "orca-lost-reply")
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	f.c.Host, f.c.Agent = "orca", "codex"
	f.c.Orca = OrcaConfig{Executable: exe, Args: []string{"--fake", "orca", stateDir}, Shell: "cmd.exe", WaitTimeout: "5s"}
	t.Setenv("STEERLINE_FAKE_ORCA_PR_BASE", f.server.URL)
	t.Setenv("STEERLINE_FAKE_ORCA_DECISION", "1")
	s := f.service(t)
	ctx := context.Background()
	require(t, s.dispatch(ctx))
	finishWorker(t, s)
	run := s.State.Runs[0]
	if run.Decision == nil || run.Process == nil || run.FinishTaskStatus != "waiting_decision" {
		t.Fatalf("initial decision not retained: %+v", run)
	}
	before, err := readFakeOrcaState(stateDir, run.Process.Terminal)
	require(t, err)
	s.State.Service.Status, s.State.Service.Ready = "running", true
	require(t, s.save())
	_, _, err = (CommandService{Store: s.domainStore, Project: s.Config.identity}).AnswerDecision(ctx, run.Decision.ID, run.Decision.Version, "lost-reply-answer", "tester", mustJSON(DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: "a", Note: "fixed answer", AnsweredBy: "tester", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: s.State.Service.InstanceID}))
	require(t, err)
	t.Setenv("STEERLINE_FAKE_ORCA_SEND_RESPONSE_LOST", "1")
	require(t, s.processCommandsAndDecisions(ctx))
	if run.ExecutionStatus != "unknown" || run.FinishTaskStatus != "blocked" || !s.State.SlotOccupied {
		t.Fatalf("lost reply response released original execution: %+v slot=%v", run, s.State.SlotOccupied)
	}
	op, err := (executionRecords{root: f.c.RunDir}).loadOperation(run.RunID, run.AttemptID, "reply-"+run.Decision.ID)
	if err != nil || op.Status != OperationUnknown {
		t.Fatalf("lost reply result was not persisted unknown: %#v %v", op, err)
	}
	after, err := readFakeOrcaState(stateDir, run.Process.Terminal)
	require(t, err)
	if after.Sends != before.Sends+1 {
		t.Fatalf("reply was not sent exactly once: before=%d after=%d", before.Sends, after.Sends)
	}
	run.Decision.Note = "changed answer at the same continuation path"
	if err := s.resumeAfterDecision(ctx, run); err == nil || !strings.Contains(err.Error(), "conflicts with its fixed answer") {
		t.Fatalf("reply digest did not bind answer content: %v", err)
	}
	t.Setenv("STEERLINE_FAKE_ORCA_SEND_RESPONSE_LOST", "")
	restarted := f.service(t)
	_ = restarted.applyDecisions(ctx)
	afterRestart, err := readFakeOrcaState(stateDir, run.Process.Terminal)
	require(t, err)
	if afterRestart.Sends != after.Sends || !restarted.State.SlotOccupied {
		t.Fatalf("restart resent uncertain reply or released slot: before=%d after=%d slot=%v", after.Sends, afterRestart.Sends, restarted.State.SlotOccupied)
	}
}

func TestDecisionApplicationEvidenceBindsOriginalDecisionAndAnswer(t *testing.T) {
	run := &Run{RunID: "run", ExecutionID: "execution", Decision: &DecisionState{ID: "decision", AttemptID: "attempt", Status: "reply_accepted", Request: DecisionRequest{Question: "choose", Options: []DecisionOption{{ID: "a", Label: "A"}}}, OptionID: "a", Note: "keep", AnsweredBy: "alice"}}
	answer, err := ReadDecisionAnswer(run)
	require(t, err)
	ref := ExecutionRef{RunID: run.RunID, AttemptID: run.Decision.AttemptID, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: ExecutionTarget{ID: "target"}, WorkspacePath: "worktree", WorkspaceBranch: "branch", BaseCommit: strings.Repeat("a", 40)}
	result := Result{TaskID: "task", RunID: run.RunID, Outcome: "blocked", DecisionApplication: &DecisionApplicationEvidence{DecisionID: run.Decision.ID, AttemptID: run.Decision.AttemptID, ExecutionID: run.ExecutionID, ExecutionRefHash: digestValue(ref), AnswerHash: decisionAnswerDigest(answer)}}
	if err = validateDecisionApplication(run, result, ref); err != nil {
		t.Fatal(err)
	}
	result.DecisionApplication.AnswerHash = digestValue("different answer")
	if err = validateDecisionApplication(run, result, ref); err == nil {
		t.Fatal("evidence for a different answer was accepted")
	}
}
