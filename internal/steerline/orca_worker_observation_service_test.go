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
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal(t *testing.T) {
	testManagedLaunchRecovery(t, false)
}

func TestNativeOrcaFixedLunaLaunchReconcilesRenamedTerminalAfterRestart(t *testing.T) {
	testManagedLaunchRecovery(t, true)
}

func TestNativeOrcaCenterReusesWorkerAfterRestartAndSettlement(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("managed native Orca fixture uses Windows PowerShell")
	}
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push fixture base: %v: %s", err, out)
	}
	exe, err := os.Executable()
	require(t, err)
	stateDir := t.TempDir()
	statePath := filepath.Join(stateDir, "native-orca.json")
	require(t, os.WriteFile(statePath, []byte("{\"LoseWorktreeOnce\":true,\"LoseCoordinatorOnce\":true,\"LoseRunOnce\":true,\"EnvironmentMode\":true,\"BusinessActive\":true,\"FirstBusinessDispatch\":2}"), 0600))
	c.Host, c.Agent, c.Claimant = "orca", "codex", "operator"
	c.PublicationPolicy.Publisher = "worker"
	c.configDigest = digestValue(c)
	setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: exe, WindowsSandbox: "unelevated"}, &orcaworker.ProfileConfig{Executable: exe, Args: []string{"--fake", "orca-native", stateDir}, Shell: "powershell.exe", WaitTimeout: "2m"})
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
	require(t, err)
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	require(t, store.Transact(ctx, func(tx DomainTx) error {
		return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}))
	if _, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "808", RequestID: strings.Repeat("b", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true}); err != nil {
		t.Fatal(err)
	}
	s, err := NewService(c)
	require(t, err)
	workerCancel := enableTestLifecycleMessages(s)
	defer func() {
		stopTestLifecycleMessages(s, workerCancel)
		_ = s.Close()
	}()
	run := s.find("808")
	task := nativeTaskRecord(s.domain.Tasks, run.TaskID)
	if run == nil || task == nil {
		t.Fatal("claimed task was not reconstructed")
	}
	if err := s.prepareWorkerFlow(ctx, *task, run); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40 && !s.executionRecord(run).Environment.Ready; i++ {
		if s.executionRecord(run).Environment.Blocker != nil {
			(&environmentFixture{service: s, run: run}).recheck(t)
		} else if err := s.resumeWorkerFlow(ctx); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		s.drainWorkerMessages()
	}
	_, execution := s.workflowRecords(run)
	if !execution.Environment.Ready || run.Target.Dispatch != "ctx-2" || execution.TurnNumber != 0 {
		state := testOrcaState(t, execution)
		if state != nil {
			t.Logf("resource state: agent_bound=%v coordinator=%s relay_ignored=%v", state.AgentBound, state.Coordinator.Terminal, state.RelayIgnored)
		}
		for _, operation := range s.domain.Operations {
			t.Logf("operation key=%s kind=%s status=%s request=%s", operation.IdempotencyKey, operation.Kind, operation.Status, operation.Request)
		}
		t.Fatalf("readiness did not settle before business input: ready=%v blocker=%+v preparation=%+v error=%q dispatch=%s turn_number=%d", execution.Environment.Ready, execution.Environment.Blocker, execution.Environment.Preparation, deref(execution.Error), run.Target.Dispatch, execution.TurnNumber)
	}

	data, err := os.ReadFile(statePath)
	require(t, err)
	var fake fakeNativeOrcaState
	require(t, json.Unmarshal(data, &fake))
	fake.EnvironmentMode, fake.LoseAckOnce = false, true
	require(t, AtomicWrite(statePath, mustJSON(fake)))
	for i := 0; i < 40 && (run.Decision == nil || run.Decision.Status != "pending"); i++ {
		time.Sleep(100 * time.Millisecond)
		s.drainWorkerMessages()
	}
	workflow, execution := s.workflowRecords(run)
	if workflow.Phase != "waiting_decision" || execution.LastStoppedTurnID != "" || execution.TurnNumber != 1 ||
		run.Decision == nil || run.Decision.Status != "pending" || orcaQuestionID(run.Decision) != "msg-ask-1" {
		t.Fatalf("first Orca question did not stay on the original Dispatch: workflow=%+v execution=%+v decision=%+v", workflow, execution, run.Decision)
	}
	if execution.PendingDeliveryID != "" {
		require(t, s.acknowledgeLifecycleDelivery(run, execution.PendingDeliveryID))
	}
	if execution.PendingDeliveryID != "" {
		require(t, s.acknowledgeLifecycleDelivery(run, execution.PendingDeliveryID))
	}
	original := *run.Process
	session, _ := executionSessionFacts(execution)
	workspace := *run.WorktreePath

	stopTestLifecycleMessages(s, workerCancel)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewService(c)
	require(t, err)
	workerCancel = enableTestLifecycleMessages(s)
	run = s.find("808")
	workflow, execution = s.workflowRecords(run)
	currentSession, _ := executionSessionFacts(execution)
	if run == nil || run.Process == nil || run.Process.Terminal != original.Terminal || run.Process.Incarnation != original.Incarnation ||
		currentSession != session || *run.WorktreePath != workspace || run.Decision == nil || run.Decision.Status != "pending" || orcaQuestionID(run.Decision) != "msg-ask-1" {
		t.Fatalf("center restart lost the original Orca conversation: %+v / %+v", run, execution)
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	require(t, err)
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	require(t, err)
	recovery, err := lifecycle.VerifyManualRecovery(ctx, snapshot)
	require(t, err)
	if !recovery.ManagedProcess || recovery.ProcessAbsent || !recovery.SessionAvailable {
		t.Fatalf("original Orca session was not verified after restart: %+v", recovery)
	}
	s.markServiceRecoveryGate()
	require(t, s.commitWorker(run))
	require(t, s.resumeConfiguredWork(ctx))
	run = s.find("808")
	workflow, execution = s.workflowRecords(run)
	if !execution.RecoveryRequired || (execution.RecoveryState != "question" && execution.RecoveryState != "live") || run.Decision.Status != "pending" {
		t.Fatalf("Orca restart did not retain its manual input gate: state=%q decision=%+v error=%q", execution.RecoveryState, run.Decision, deref(run.Error))
	}
	data, err = os.ReadFile(statePath)
	require(t, err)
	require(t, json.Unmarshal(data, &fake))
	if fake.Dispatches != 2 || fake.Replies != 0 || fake.Sends != 0 {
		t.Fatalf("recovery sent input without a new decision: %+v", fake)
	}

	s.State.Service.Ready = true
	require(t, s.publishCheckpoint())
	command, created, err := SubmitDecisionAnswer(ctx, s.Config, AnswerOptions{RunID: "808", DecisionID: run.Decision.ID, Note: "Keep this session", By: "operator"})
	if err != nil || !created || command.Status != "pending" {
		t.Fatalf("native Orca answer was not submitted through the shared command path: %+v / %v", command, err)
	}
	require(t, s.processCommandsAndDecisions(ctx))
	require(t, s.applyDecisions(ctx))
	for i := 0; i < 40 && run.Decision.Status != "reply_accepted"; i++ {
		s.drainWorkerMessages()
		if err := s.reconcileSavedLifecycleQuestionAnswer(ctx, run); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if run.Decision.Status != "reply_accepted" || run.Target.Dispatch != "ctx-2" {
		ops := []string{}
		for _, op := range s.domain.Operations {
			if op.Kind == "reply_execution_question" || op.Status == "outcome_unknown" || op.Status == "in_flight" {
				ops = append(ops, op.Kind+":"+op.IdempotencyKey+":"+op.Status+":"+op.UnknownReason)
			}
		}
		t.Fatalf("answer did not reconcile against the original question and Dispatch: decision=%+v target=%+v ops=%v error=%s", run.Decision, run.Target, ops, deref(s.executionRecord(run).Error))
	}
	repeated, created, err := SubmitDecisionAnswer(ctx, s.Config, AnswerOptions{RunID: "808", DecisionID: run.Decision.ID, Note: "Keep this session", By: "operator"})
	if err != nil || created || repeated.CommandID != command.CommandID {
		t.Fatalf("repeated public answer was not idempotent: first=%+v repeated=%+v created=%v error=%v", command, repeated, created, err)
	}
	for i := 0; i < 50 && execution.LastStoppedTurnID == ""; i++ {
		s.drainWorkerMessages()
		time.Sleep(100 * time.Millisecond)
		workflow, execution = s.workflowRecords(run)
	}
	if execution.LastStoppedTurnID == "" || execution.CurrentTurnID != execution.LastStoppedTurnID || workflow.Phase != "waiting_decision" {
		t.Fatalf("original Orca Dispatch did not settle in the same session: workflow=%+v execution=%+v", workflow, execution)
	}
	if run.Process.Terminal != original.Terminal || run.Process.Incarnation != original.Incarnation || *run.WorktreePath != workspace {
		t.Fatalf("native Orca recovery replaced its fixed terminal or workspace: %+v", run)
	}
	data, err = os.ReadFile(statePath)
	require(t, err)
	require(t, json.Unmarshal(data, &fake))
	if fake.Dispatches != 2 || fake.Replies != 1 || fake.Sends != 0 {
		t.Fatalf("native lifecycle repeated a Dispatch or question reply: %+v", fake)
	}
}
func enableTestLifecycleMessages(s *Service) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	s.runContext = ctx
	s.workerMessages = make(chan workerMessage, 128)
	return cancel
}

func stopTestLifecycleMessages(s *Service, cancel context.CancelFunc) {
	if s != nil {
		for _, worker := range s.workers {
			if worker != nil && worker.cancelObservation != nil {
				worker.cancelObservation()
			}
		}
	}
	if cancel != nil {
		cancel()
	}
	if s != nil {
		s.forwarders.Wait()
	}
}
