package steerline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNativeOrcaPRBindingUsesProviderAndRetainsSession(t *testing.T) {
	ctx := context.Background()
	state, headBranch := "open", "codex/task-8"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/org/repo/pulls/8" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"html_url": server.URL + "/org/repo/pulls/8", "number": 8, "state": state, "merged": false,
			"base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "org/repo"}},
			"head": map[string]any{"ref": headBranch, "sha": strings.Repeat("a", 40), "repo": map[string]string{"full_name": "org/repo"}},
		})
	}))
	defer server.Close()
	url := server.URL + "/org/repo/pulls/8"
	config := PRConfig{Kind: "gitea", BaseURL: server.URL, Repository: "org/repo"}
	s := Service{Config: Config{PR: config}, prs: &PRClient{Config: config, Runner: Runner{Timeout: 5 * time.Second}}}
	workflow := WorkflowRunRecord{WorkflowRunID: "wf-8", WorkflowVersion: WorkerFlowVersion, Phase: "executing", Branch: "main"}
	execution := ExecutionRecord{ExecutionID: "exec-8", WorkflowRunID: workflow.WorkflowRunID, SessionID: "orca-session-8", Workspace: WorkspaceEvidence{Branch: "codex/task-8"}, CurrentTurnID: "turn-8", LastStoppedTurnID: "turn-8"}
	run := Run{}
	headBranch = "codex/other-task"
	if err := s.bindNativeOrcaPRCandidates(ctx, &run, &workflow, &execution, []string{url}); err == nil || workflow.PRURL != "" || run.PR.URL != nil {
		t.Fatalf("unowned PR was bound: workflow=%+v run=%+v error=%v", workflow, run, err)
	}
	headBranch = "codex/task-8"
	if err := s.bindNativeOrcaPRCandidates(ctx, &run, &workflow, &execution, []string{url}); err != nil || workflow.PRURL != url || workflow.Phase != "waiting_pr_merge" {
		t.Fatalf("provider-verified PR was not bound: workflow=%+v error=%v", workflow, err)
	}
	state = "closed"
	if err := s.bindNativeOrcaPRCandidates(ctx, &run, &workflow, &execution, []string{url}); err != nil || workflow.PRState != "closed_unmerged" || workflow.Phase != "waiting_decision" || execution.SessionID != "orca-session-8" || execution.CurrentTurnID != "turn-8" {
		t.Fatalf("unmerged PR did not return the original session to feedback: workflow=%+v execution=%+v error=%v", workflow, execution, err)
	}
}

func TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal(t *testing.T) {
	testManagedLaunchRecovery(t, false)
}

func TestNativeOrcaFixedLunaLaunchReconcilesRenamedTerminalAfterRestart(t *testing.T) {
	testManagedLaunchRecovery(t, true)
}

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
	if err := os.WriteFile(filepath.Join(stateDir, "native-orca.json"), []byte(`{"LoseWorktreeOnce":true,"LoseCoordinatorOnce":true,"LoseRunOnce":true,"LoseAckOnce":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	c.Host, c.Agent, c.Sandbox, c.Claimant = "orca", "opencode", "workspace-write", "operator"
	c.Orca = OrcaConfig{Executable: exe, Args: []string{"--fake", "orca-native", stateDir}, Shell: "cmd.exe", WaitTimeout: "2s"}
	c.configDigest = digestValue(c) // The production CLI obtains this from LoadConfig.
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
	task := nativeTaskRecord(s.domain.Tasks, "808")
	if run == nil || task == nil {
		t.Fatal("claimed task was not reconstructed")
	}
	if err := s.prepareWorkerFlow(ctx, *task, run); err != nil {
		t.Fatal(err)
	}
	if run.Process != nil || run.Target.Dispatch != "" {
		t.Fatal("lost worktree creation receipt was guessed before reconciliation")
	}
	for i := 0; i < 3; i++ {
		if err := s.resumeWorkerFlow(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if run.Process == nil || run.Process.Kind != "orca-native" || run.Target.Dispatch != "ctx-1" || run.SessionID != nil {
		_, execution := s.workflowRecords(run)
		t.Fatalf("first Orca Dispatch was not bound: error=%q process=%+v execution_process=%+v orca_state=%+v target=%+v", deref(run.Error), run.Process, execution.Process, execution.OrcaNative, run.Target)
	}
	if err := s.pollNativeOrcaWorker(ctx, run); err != nil {
		t.Fatal(err)
	}
	_, execution := s.workflowRecords(run)
	if execution.OrcaNative.DeliveryID != "delivery-ask-1" {
		t.Fatal("lost acknowledgement response did not retain the original Delivery")
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
	s.markServiceRecoveryGate()
	if err := s.commitWorker(run); err != nil {
		t.Fatal(err)
	}
	if err := s.resumeConfiguredWork(ctx); err != nil {
		t.Fatal(err)
	}
	if !execution.RecoveryRequired || execution.RecoveryState != "live" || run.Decision.Status != "pending" {
		t.Fatalf("Orca restart did not retain its manual input gate: state=%q decision=%+v", execution.RecoveryState, run.Decision)
	}
	noInputData, err := os.ReadFile(filepath.Join(stateDir, "native-orca.json"))
	if err != nil {
		t.Fatal(err)
	}
	var noInputFake fakeNativeOrcaState
	if err := json.Unmarshal(noInputData, &noInputFake); err != nil {
		t.Fatal(err)
	}
	if noInputFake.Dispatches != 1 || noInputFake.Replies != 0 || noInputFake.Sends != 0 {
		t.Fatalf("Orca recovery sent execution input without a new decision: %+v", noInputFake)
	}
	replyStatePath := filepath.Join(stateDir, "native-orca.json")
	replyData, err := os.ReadFile(replyStatePath)
	if err != nil {
		t.Fatal(err)
	}
	var replyFake fakeNativeOrcaState
	if err := json.Unmarshal(replyData, &replyFake); err != nil {
		t.Fatal(err)
	}
	replyFake.LoseReplyOnce = true
	replyData, _ = json.Marshal(replyFake)
	if err := AtomicWrite(replyStatePath, replyData); err != nil {
		t.Fatal(err)
	}
	s.State.Service.Ready = true
	if err := s.publishCheckpoint(); err != nil {
		t.Fatal(err)
	}
	command, created, err := SubmitDecisionAnswer(ctx, s.Config, AnswerOptions{RunID: "808", DecisionID: run.Decision.ID, Note: "Keep this session", By: "operator"})
	if err != nil || !created || command.Status != "pending" {
		t.Fatalf("native Orca answer was not submitted through the shared command path: %+v / %v", command, err)
	}
	if err := s.processCommandsAndDecisions(ctx); err != nil {
		t.Fatal(err)
	}
	if run.Decision.Status != "answer_saved" || run.Target.Dispatch != "ctx-1" {
		t.Fatalf("lost reply receipt was treated as confirmed: decision=%+v target=%+v", run.Decision, run.Target)
	}
	if err := s.applyDecisions(ctx); err != nil {
		t.Fatal(err)
	}
	if run.Target.Dispatch != "ctx-1" || run.Decision.Status != "reply_accepted" {
		t.Fatalf("reply did not target the original question/Dispatch: target=%+v decision=%+v", run.Target, run.Decision)
	}
	repeated, created, err := SubmitDecisionAnswer(ctx, s.Config, AnswerOptions{RunID: "808", DecisionID: run.Decision.ID, Note: "Keep this session", By: "operator"})
	if err != nil || created || repeated.CommandID != command.CommandID {
		t.Fatalf("repeated public answer was not idempotent: first=%+v repeated=%+v created=%v error=%v", command, repeated, created, err)
	}
	firstStatePath := filepath.Join(stateDir, "native-orca.json")
	firstData, err := os.ReadFile(firstStatePath)
	if err != nil {
		t.Fatal(err)
	}
	var firstFake fakeNativeOrcaState
	if err := json.Unmarshal(firstData, &firstFake); err != nil {
		t.Fatal(err)
	}
	firstFake.ForceSourceChange = true
	firstData, _ = json.Marshal(firstFake)
	if err := AtomicWrite(firstStatePath, firstData); err != nil {
		t.Fatal(err)
	}
	if err := s.pollNativeOrcaWorker(ctx, run); err != nil {
		t.Fatal(err)
	}
	workflow, execution = s.workflowRecords(run)
	if attempt := s.workflowAttempt(run, run.AttemptID); attempt == nil || attempt.OutputSource != "transcript" || execution.OrcaNative.Turns[0].OutputSource != "transcript" || !execution.OrcaNative.Turns[0].OutputComplete {
		t.Fatalf("source_changed was not retried from the new source: attempt=%+v turn=%+v", attempt, execution.OrcaNative.Turns[0])
	}
	if workflow.Phase != "waiting_decision" || execution.LastStoppedTurnID == "" || run.Decision == nil || run.Decision.Status != "pending" {
		t.Fatalf("first Orca settlement did not produce feedback Decision: workflow=%+v execution=%+v decision=%+v", workflow, execution, run.Decision)
	}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: "continue", Note: "Continue in the same session", AnsweredBy: "operator"}
	record := s.decisionRecord(run)
	record.Status, record.Version, record.AnsweredBy = "answer_saved", record.Version+1, answer.AnsweredBy
	answer.AnsweredAt = now()
	record.Answer = mustJSON(answer)
	s.markWorkerDecisionWrite(record.DecisionID)
	s.refreshRun(run)
	if err := s.commitWorker(run); err != nil {
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
	record = s.decisionRecord(run)
	record.Status, record.Version, record.AnsweredBy = "answer_saved", record.Version+1, answer.AnsweredBy
	answer.AnsweredAt = now()
	record.Answer = mustJSON(answer)
	s.markWorkerDecisionWrite(record.DecisionID)
	s.refreshRun(run)
	if err := s.commitWorker(run); err != nil {
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
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fake); err != nil {
		t.Fatal(err)
	}
	fake.EscalationAt = 4
	fake.LoseSendOnce = true
	data, _ = json.Marshal(fake)
	if err := AtomicWrite(statePath, data); err != nil {
		t.Fatal(err)
	}
	answer = DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: "continue", Note: "Start one more Dispatch", AnsweredBy: "operator"}
	record = s.decisionRecord(run)
	record.Status, record.Version, record.AnsweredBy = "answer_saved", record.Version+1, answer.AnsweredBy
	answer.AnsweredAt = now()
	record.Answer = mustJSON(answer)
	s.markWorkerDecisionWrite(record.DecisionID)
	s.refreshRun(run)
	if err := s.commitWorker(run); err != nil {
		t.Fatal(err)
	}
	if err := s.applyNativeOrcaDecision(ctx, run, workflow, execution, answer, strings.Repeat("f", 32)); err != nil {
		t.Fatal(err)
	}
	if err := s.pollNativeOrcaWorker(ctx, run); err != nil {
		t.Fatal(err)
	}
	workflow, execution = s.workflowRecords(run)
	if run.Target.Dispatch != "ctx-4" || run.Decision == nil || run.Decision.Status != "pending" || orcaQuestionID(run.Decision) != "" || execution.LastStoppedTurnID == execution.CurrentTurnID {
		t.Fatalf("active Orca escalation did not keep the current Dispatch: target=%+v decision=%+v execution=%+v", run.Target, run.Decision, execution)
	}
	command, created, err = SubmitDecisionAnswer(ctx, s.Config, AnswerOptions{RunID: "808", DecisionID: run.Decision.ID, Option: "continue", Note: "Continue current work", By: "operator"})
	if err != nil || !created {
		t.Fatalf("active send answer was not accepted: %+v / %v", command, err)
	}
	if err := s.processCommandsAndDecisions(ctx); err != nil {
		t.Fatal(err)
	}
	if run.Decision.Status != "answer_saved" || run.Target.Dispatch != "ctx-4" {
		t.Fatal("lost send receipt was treated as confirmed or retargeted")
	}
	if err := s.applyDecisions(ctx); err != nil {
		t.Fatal(err)
	}
	if run.Target.Dispatch != "ctx-4" || run.Decision.Status != "reply_accepted" {
		t.Fatal("active feedback did not stay on the original Dispatch")
	}
	if err := s.pollNativeOrcaWorker(ctx, run); err != nil {
		t.Fatal(err)
	}
	workflow, execution = s.workflowRecords(run)
	if execution.TurnNumber != 4 || execution.OrcaNative.Turns[3].DispatchID != "ctx-4" || workflow.Phase != "waiting_decision" {
		t.Fatal("active feedback caused an extra worker or lost settlement")
	}
	data, err = os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fake); err != nil {
		t.Fatal(err)
	}
	fake.LoseTaskAt = 5
	data, _ = json.Marshal(fake)
	if err := AtomicWrite(statePath, data); err != nil {
		t.Fatal(err)
	}
	answer = DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: "continue", Note: "Recover Task creation before Dispatch", AnsweredBy: "operator"}
	record = s.decisionRecord(run)
	record.Status, record.Version, record.AnsweredBy = "answer_saved", record.Version+1, answer.AnsweredBy
	answer.AnsweredAt = now()
	record.Answer = mustJSON(answer)
	s.markWorkerDecisionWrite(record.DecisionID)
	s.refreshRun(run)
	if err := s.commitWorker(run); err != nil {
		t.Fatal(err)
	}
	if err := s.applyNativeOrcaDecision(ctx, run, workflow, execution, answer, strings.Repeat("2", 32)); err != nil {
		t.Fatal(err)
	}
	if run.Target.Dispatch == "ctx-5" || execution.PendingTurnID == "" {
		t.Fatal("lost Task receipt was guessed rather than reconciled")
	}
	if err := s.resumeWorkerFlow(ctx); err != nil {
		t.Fatal(err)
	}
	workflow, execution = s.workflowRecords(run)
	if execution.TurnNumber != 5 || len(execution.OrcaNative.Turns) != 5 || execution.OrcaNative.Turns[4].DispatchID != "ctx-5" || workflow.Phase != "waiting_decision" {
		message := ""
		if execution.Error != nil {
			message = *execution.Error
		}
		t.Fatalf("lost Task receipt did not recover in original worker: workflow=%+v execution=%+v error=%s", workflow, execution, message)
	}
	data, err = os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fake); err != nil {
		t.Fatal(err)
	}
	fake.AgentIncarnation = "replaced-agent"
	data, _ = json.Marshal(fake)
	if err := AtomicWrite(statePath, data); err != nil {
		t.Fatal(err)
	}
	answer = DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: "continue", Note: "Do not replace the lost worker", AnsweredBy: "operator"}
	record = s.decisionRecord(run)
	record.Status, record.Version, record.AnsweredBy = "answer_saved", record.Version+1, answer.AnsweredBy
	answer.AnsweredAt = now()
	record.Answer = mustJSON(answer)
	s.markWorkerDecisionWrite(record.DecisionID)
	s.refreshRun(run)
	if err := s.commitWorker(run); err != nil {
		t.Fatal(err)
	}
	if err := s.applyNativeOrcaDecision(ctx, run, workflow, execution, answer, strings.Repeat("1", 32)); err != nil {
		t.Fatal(err)
	}
	if run.Decision.Status != "answer_saved" || run.Target.Dispatch != "ctx-5" || run.Error == nil || !strings.Contains(*run.Error, "original Orca session is unavailable") {
		t.Fatalf("replacement agent did not retain feedback and original binding: decision=%+v target=%+v error=%q", run.Decision, run.Target, deref(run.Error))
	}
	data, err = os.ReadFile(statePath)
	if err != nil || !strings.Contains(string(data), `"Dispatches":5`) || !strings.Contains(string(data), `"Replies":1`) || !strings.Contains(string(data), `"Sends":1`) || !strings.Contains(string(data), `"Tasks":[`) {
		t.Fatalf("fake Orca duplicated a Dispatch or reply during recovery: %s / %v", data, err)
	}
	// Reset this fake observation to the original terminal identity to exercise
	// terminal-close recovery independently of the replacement-agent case.
	if err := json.Unmarshal(data, &fake); err != nil {
		t.Fatal(err)
	}
	fake.AgentIncarnation = ""
	fake.LoseCloseOnce = true
	data, _ = json.Marshal(fake)
	if err := AtomicWrite(statePath, data); err != nil {
		t.Fatal(err)
	}
	if err := s.closeNativeOrcaTerminal(ctx, run, execution, *execution.Process, "agent"); err == nil {
		t.Fatal("lost terminal-close response was treated as confirmed")
	}
	if err := s.closeNativeOrcaTerminal(ctx, run, execution, *execution.Process, "agent"); err != nil {
		t.Fatal(err)
	}
	if err := s.closeNativeOrcaTerminal(ctx, run, execution, execution.OrcaNative.Coordinator, "coordinator"); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(statePath)
	if err != nil || json.Unmarshal(data, &fake) != nil || !fake.ClosedAgent || !fake.ClosedCoordinator {
		t.Fatalf("exact fake Orca terminals were not closed: %s / %v", data, err)
	}
}
