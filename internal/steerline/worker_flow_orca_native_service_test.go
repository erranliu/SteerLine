package steerline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativeOrcaCenterReusesWorkerAfterRestartAndSettlement(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Orca shell fixture uses cmd.exe")
	}
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push fixture base: %v: %s", err, out)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	c.Host, c.Agent, c.Sandbox, c.Claimant = "orca", "codex", "workspace-write", "operator"
	c.Orca = OrcaConfig{Executable: exe, Args: []string{"--fake", "orca-native", stateDir}, Shell: "cmd.exe", WaitTimeout: "2s"}
	commands := CommandService{Store: store, Project: c.Identity()}
	input := taskBatchFixture(t, c, commit, batchTasks(808), nil)
	validation, err := commands.ValidateTaskBatch(ctx, c, input)
	if err != nil || !validation.Valid {
		t.Fatalf("validate: %+v / %v", validation, err)
	}
	if _, err := commands.CreateTaskBatch(ctx, c, input, strings.Repeat("a", 32), validation.AuthorityVersion, "operator"); err != nil {
		t.Fatal(err)
	}
	authority, err := store.nativeAuthority(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err := store.Transact(ctx, func(tx DomainTx) error {
		return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "808", RequestID: strings.Repeat("b", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true}); err != nil {
		t.Fatal(err)
	}
	s, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	run := s.find("808")
	task := nativeTaskRecord(s.State.Tasks, "808")
	if run == nil || task == nil {
		t.Fatal("claimed task was not reconstructed")
	}
	if err := s.prepareWorkerFlow(ctx, *task, run); err != nil {
		t.Fatal(err)
	}
	if run.Process == nil || run.Process.Kind != "orca-native" || run.Target.Dispatch != "ctx-1" || run.SessionID != nil {
		_, execution := s.workflowRecords(run)
		t.Fatalf("first Orca Dispatch was not bound: error=%q process=%+v execution_process=%+v orca_state=%+v target=%+v", deref(run.Error), run.Process, execution.Process, execution.OrcaNative, run.Target)
	}
	if err := s.pollNativeOrcaWorker(ctx, run); err != nil {
		t.Fatal(err)
	}
	workflow, execution := s.workflowRecords(run)
	if workflow.Phase != "waiting_decision" || execution.LastStoppedTurnID != "" || execution.TurnNumber != 1 || run.Decision == nil || run.Decision.Status != "pending" || orcaQuestionID(run.Decision) != "msg-ask-1" {
		t.Fatalf("first Orca ask did not retain its live Dispatch: workflow=%+v execution=%+v decision=%+v", workflow, execution, run.Decision)
	}
	original := *run.Process
	session := execution.SessionID
	workspace := *run.WorktreePath
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	run = s.find("808")
	workflow, execution = s.workflowRecords(run)
	if run == nil || run.Process == nil || *run.Process != original || execution.SessionID != session || *run.WorktreePath != workspace || run.Decision == nil || run.Decision.Status != "pending" || orcaQuestionID(run.Decision) != "msg-ask-1" {
		t.Fatalf("center restart lost the original Orca conversation: %+v / %+v", run, execution)
	}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Note: "Keep this session", AnsweredBy: "operator"}
	if err := normalizeNativeDecisionAnswer(&run.Decision.Request, &answer); err != nil {
		t.Fatal(err)
	}
	run.Decision.Status, run.Decision.OptionID, run.Decision.Note, run.Decision.AnsweredBy = "answer_saved", answer.OptionID, answer.Note, answer.AnsweredBy
	run.Decision.Answers = answer.Answers
	answered := now()
	run.Decision.AnsweredAt = &answered
	run.Decision.Version++
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	if err := s.applyNativeOrcaDecision(ctx, run, workflow, execution, answer, strings.Repeat("c", 32)); err != nil {
		t.Fatal(err)
	}
	if run.Target.Dispatch != "ctx-1" || run.Decision.Status != "reply_accepted" {
		t.Fatalf("reply did not target the original question/Dispatch: target=%+v decision=%+v", run.Target, run.Decision)
	}
	if err := s.pollNativeOrcaWorker(ctx, run); err != nil {
		t.Fatal(err)
	}
	workflow, execution = s.workflowRecords(run)
	if workflow.Phase != "waiting_decision" || execution.LastStoppedTurnID == "" || run.Decision == nil || run.Decision.Status != "pending" {
		t.Fatalf("first Orca settlement did not produce feedback Decision: workflow=%+v execution=%+v decision=%+v", workflow, execution, run.Decision)
	}
	answer = DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: "continue", Note: "Continue in the same session", AnsweredBy: "operator"}
	run.Decision.Status, run.Decision.OptionID, run.Decision.Note, run.Decision.AnsweredBy = "answer_saved", answer.OptionID, answer.Note, answer.AnsweredBy
	answered = now()
	run.Decision.AnsweredAt = &answered
	run.Decision.Version++
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	if err := s.applyNativeOrcaDecision(ctx, run, workflow, execution, answer, strings.Repeat("d", 32)); err != nil {
		t.Fatal(err)
	}
	if run.Target.Dispatch != "ctx-2" || run.Process.Terminal != original.Terminal || run.Process.Incarnation != original.Incarnation || *run.WorktreePath != workspace {
		t.Fatalf("feedback replaced the worker rather than adding a new Dispatch: %+v", run)
	}
	if err := s.pollNativeOrcaWorker(ctx, run); err != nil {
		t.Fatal(err)
	}
	workflow, execution = s.workflowRecords(run)
	if execution.TurnNumber != 2 || execution.SessionID != session || len(execution.OrcaNative.Turns) != 2 || execution.OrcaNative.Turns[0].DispatchID != "ctx-1" || execution.OrcaNative.Turns[1].DispatchID != "ctx-2" || workflow.Phase != "waiting_decision" {
		t.Fatalf("second Orca turn did not settle in the original conversation: workflow=%+v execution=%+v", workflow, execution)
	}
	statePath := filepath.Join(stateDir, "native-orca.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var fake fakeNativeOrcaState
	if err := json.Unmarshal(data, &fake); err != nil {
		t.Fatal(err)
	}
	fake.LoseDispatchAt = 3
	data, _ = json.Marshal(fake)
	if err := AtomicWrite(statePath, data); err != nil {
		t.Fatal(err)
	}
	answer = DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: "continue", Note: "Recover the original Dispatch after its receipt is lost", AnsweredBy: "operator"}
	run.Decision.Status, run.Decision.OptionID, run.Decision.Note, run.Decision.AnsweredBy = "answer_saved", answer.OptionID, answer.Note, answer.AnsweredBy
	answered = now()
	run.Decision.AnsweredAt = &answered
	run.Decision.Version++
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	if err := s.applyNativeOrcaDecision(ctx, run, workflow, execution, answer, strings.Repeat("e", 32)); err != nil {
		t.Fatal(err)
	}
	if run.Target.Dispatch == "ctx-3" || execution.PendingTurnID == "" {
		t.Fatal("lost Dispatch receipt was guessed rather than retained for reconciliation")
	}
	if err := s.resumeWorkerFlow(ctx); err != nil {
		t.Fatal(err)
	}
	workflow, execution = s.workflowRecords(run)
	if execution.TurnNumber != 3 || len(execution.OrcaNative.Turns) != 3 || execution.OrcaNative.Turns[2].DispatchID != "ctx-3" || workflow.Phase != "waiting_decision" {
		t.Fatalf("lost Dispatch receipt was not reconciled from the original agent: workflow=%+v execution=%+v", workflow, execution)
	}
	data, err = os.ReadFile(statePath)
	if err != nil || !strings.Contains(string(data), `"Dispatches":3`) || !strings.Contains(string(data), `"Replies":1`) {
		t.Fatalf("fake Orca duplicated a Dispatch or reply during recovery: %s / %v", data, err)
	}
}
