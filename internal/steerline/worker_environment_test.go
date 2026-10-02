package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/testprotocol"
)

type environmentFixture struct {
	service      *Service
	config       Config
	statePath    string
	run          *Run
	workerCancel context.CancelFunc
}

func newEnvironmentFixture(t *testing.T, state fakeNativeOrcaState, model string) *environmentFixture {
	t.Helper()
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("fixture base: %v: %s", err, out)
	}
	exe, err := os.Executable()
	require(t, err)
	state.EnvironmentMode, state.BusinessActive = true, true
	stateDir := t.TempDir()
	statePath := filepath.Join(stateDir, "native-orca.json")
	data, _ := json.Marshal(state)
	require(t, os.WriteFile(statePath, data, 0600))
	c.PublicationPolicy.Publisher = "worker"
	c.Host, c.Agent, c.Claimant = "orca", "codex", "operator"
	c.configDigest = digestValue(c)
	effort := ""
	if model != "" {
		effort = "high"
	}
	setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: exe, WindowsSandbox: "unelevated", Model: model, ReasoningEffort: effort}, &orcaworker.ProfileConfig{Executable: exe, Args: []string{"--fake", "orca-native", stateDir}, Shell: "powershell.exe", WaitTimeout: "2m"})
	commands := CommandService{Store: store, Project: c.Identity()}
	input := taskBatchFixture(t, c, commit, batchTasks(8154), nil)
	validation, err := commands.ValidateTaskBatch(ctx, c, input)
	require(t, err)
	if !validation.Valid {
		t.Fatalf("invalid fixture: %+v", validation)
	}
	_, err = commands.CreateTaskBatch(ctx, c, input, newID(), validation.AuthorityVersion, "operator")
	require(t, err)
	authority, err := store.nativeAuthority(ctx, c.Identity())
	require(t, err)
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	require(t, store.Transact(ctx, func(tx DomainTx) error {
		return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}))
	_, err = commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "8154", RequestID: newID(), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true})
	require(t, err)
	s, err := NewService(c)
	require(t, err)
	workerCancel := enableTestLifecycleMessages(s)
	f := &environmentFixture{service: s, config: c, statePath: statePath, run: s.find("8154"), workerCancel: workerCancel}
	t.Cleanup(func() { stopTestLifecycleMessages(f.service, f.workerCancel); _ = f.service.Close() })
	return f
}

func (f *environmentFixture) prepare(t *testing.T) {
	t.Helper()
	task := nativeTaskRecord(f.service.domain.Tasks, "8154")
	require(t, f.service.prepareWorkerFlow(context.Background(), *task, f.run))
}
func (f *environmentFixture) poll(t *testing.T) {
	t.Helper()
	require(t, driveTestLifecycleWorker(f.service, f.run))
}

func driveTestLifecycleWorker(service *Service, run *Run) error {
	if service == nil || run == nil {
		return errors.New("lifecycle test worker is missing")
	}
	if err := service.observeWorkerEnvironment(context.Background(), run); err != nil {
		return err
	}
	if service.workers[run.RunID] == nil {
		lifecycle, _, err := service.lifecycleForRun(run)
		if err != nil {
			return err
		}
		if err := service.resumeLifecycleObservation(context.Background(), run, lifecycle); err != nil {
			return err
		}
	}
	for i := 0; i < 20; i++ {
		service.drainWorkerMessages()
		_, execution := service.workflowRecords(run)
		if turn := nativeOrcaCurrentTurn(execution); turn != nil && turn.Purpose == "readiness" && !turn.Settled {
			if err := service.readNativeOrcaOutput(context.Background(), run, execution, turn); err != nil && execution.Environment.Blocker == nil {
				return err
			}
		}
		turn := nativeOrcaCurrentTurn(execution)
		if execution.Environment != nil && (execution.Environment.Ready || execution.Environment.Blocker != nil && (turn == nil || turn.Purpose != "readiness" || execution.Environment.Preparation.Verified || turn.Settled && turn.Outcome == "failed")) {
			return nil
		}
		select {
		case <-time.After(100 * time.Millisecond):
		}
	}
	service.drainWorkerMessages()
	return nil
}
func (f *environmentFixture) state(t *testing.T) fakeNativeOrcaState {
	t.Helper()
	data, err := os.ReadFile(f.statePath)
	require(t, err)
	var state fakeNativeOrcaState
	require(t, json.Unmarshal(data, &state))
	return state
}
func (f *environmentFixture) update(t *testing.T, change func(*fakeNativeOrcaState)) {
	t.Helper()
	require(t, testprotocol.UpdateNativeOrcaState(filepath.Dir(f.statePath), change))
}
func (f *environmentFixture) recheck(t *testing.T) {
	f.answer(t, "recheck_environment", "")
}

func (f *environmentFixture) answer(t *testing.T, option, note string) {
	t.Helper()
	decision := f.run.Decision
	if decision == nil {
		t.Fatal("no host decision")
	}
	s := f.service
	s.State.Service.Ready = true
	require(t, s.publishCheckpoint())
	answer := DecisionAnswer{DecisionID: decision.ID, RunID: f.run.RunID, OptionID: option, Note: note, AnsweredBy: "operator", ExpectedDecisionVersion: decision.Version, ExpectedRunRevision: f.run.ControlRevision, ServiceInstanceID: s.State.Service.InstanceID, SourceAttemptID: f.run.AttemptID, ExpectedRecoveryVersion: f.run.RecoveryVersion}
	commands := CommandService{Store: s.domainStore, Project: s.Config.identity}
	command, _, err := commands.AnswerDecisionWithID(context.Background(), newID(), decision.ID, decision.Version, decisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	require(t, s.processCommandsAndDecisions(context.Background()))
	saved, err := s.domainStore.CommandByIdempotency(context.Background(), s.Config.identity, command.IdempotencyKey)
	require(t, err)
	if saved.Status != "accepted" {
		t.Fatalf("host action was not accepted: %s / %s", saved.Status, saved.Result)
	}
}

func TestWorkerEnvironmentER1ProbeAndSettlementGate(t *testing.T) {
	for _, mode := range []string{"success", "tool_failure", "oral", "missing_receipt", "failed_settlement"} {
		t.Run(mode, func(t *testing.T) {
			f := newEnvironmentFixture(t, fakeNativeOrcaState{ReadinessMode: mode}, "")
			f.prepare(t)
			if execution := f.service.executionRecord(f.run); testOrcaState(t, execution) == nil {
				t.Fatalf("resource preparation did not bind the native worker: %+v", execution.Environment.Blocker)
			}
			f.poll(t)
			wf, e := f.service.workflowRecords(f.run)
			state := f.state(t)
			if mode == "success" {
				stateData := testOrcaState(t, e)
				if state.Dispatches != 2 || !e.Environment.Ready || e.TurnNumber != 0 || e.CurrentTurnID != "" || len(stateData.Turns) != 2 || stateData.Turns[0].Purpose != "readiness" {
					t.Fatalf("preparation leaked business state or failed: wf=%+v execution=%+v state=%+v", wf, e, state)
				}
				f.poll(t)
				if e.TurnNumber != 1 {
					t.Fatalf("business start not independently observed: %+v", e)
				}
			} else {
				if state.Dispatches != 1 || e.Environment.Ready || e.TurnNumber != 0 {
					t.Fatalf("failed readiness dispatched business: %+v / %+v", e, state)
				}
				if mode == "tool_failure" || mode == "failed_settlement" {
					if wf.Phase != "waiting_decision" || e.Environment.Blocker == nil {
						t.Fatalf("failure had no decision: %+v / %+v", wf, e)
					}
				}
			}
			if !strings.Contains(state.FixedAgentCommand, "'approval_policy=never'") || !strings.Contains(state.FixedAgentCommand, "'windows.sandbox=unelevated'") || state.CreatedWithAgent != "" {
				t.Fatalf("managed permissions missing: %+v", state)
			}
			if strings.Contains(state.FixedAgentCommand, "'--model'") {
				t.Fatal("default model replaced")
			}
		})
	}
}

func TestWorkerEnvironmentWaitsForOriginalAgentDetection(t *testing.T) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{HoldFixedAgentIdentity: true, ReadinessMode: "hold"}, "")
	f.prepare(t)
	_, execution := f.service.workflowRecords(f.run)
	env := execution.Environment
	if env.Blocker != nil || execution.Process == nil || env.OriginalTerminal == nil || *execution.Process != *env.OriginalTerminal || env.Ready {
		t.Fatalf("unidentified startup became a blocker or a ready agent: %+v", env)
	}
	deadline, nonce := env.Preparation.Deadline, env.Preparation.Nonce
	terminal := *env.OriginalTerminal
	var op *OperationRecord
	for i := range f.service.domain.Operations {
		candidate := &f.service.domain.Operations[i]
		var input executionapi.ResourceIntent
		if candidate.Kind == string(executionapi.LifecycleCreateResource) && json.Unmarshal(candidate.Request, &input) == nil && input.Purpose == "agent-terminal" {
			op = candidate
		}
	}
	if op == nil || op.Status != "accepted" || f.state(t).Dispatches != 0 {
		t.Fatal("startup receipt lost or input sent before agent identification")
	}
	// Another actor pass must only observe the original launch.
	require(t, f.service.advanceOrcaPreparation(context.Background(), f.run))
	if env.Blocker != nil || !env.Preparation.Deadline.Equal(deadline) || env.Preparation.Nonce != nonce || *env.OriginalTerminal != terminal {
		t.Fatal("waiting reset the preparation or replaced the original terminal")
	}
	f.update(t, func(state *fakeNativeOrcaState) { state.HoldFixedAgentIdentity = false })
	require(t, f.service.advanceOrcaPreparation(context.Background(), f.run))
	if execution.Process == nil || execution.Process.Terminal != terminal.Terminal || execution.Process.Incarnation != terminal.Incarnation || !testOrcaState(t, execution).AgentBound || f.state(t).Dispatches != 1 {
		t.Fatal("agent identification did not resume the same launch exactly once")
	}
}

func TestWorkerEnvironmentUnidentifiedAgentKeepsOriginalDeadline(t *testing.T) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{HoldFixedAgentIdentity: true}, "")
	f.prepare(t)
	execution := f.service.executionRecord(f.run)
	execution.Environment.Preparation.Deadline = now().Add(-time.Second)
	require(t, f.service.commitWorker(f.run))
	require(t, f.service.advanceOrcaPreparation(context.Background(), f.run))
	if execution.Environment.Blocker == nil || execution.Environment.Blocker.Fact.Reason != "preparation_unverified" || f.state(t).Dispatches != 0 {
		t.Fatal("unidentified startup bypassed its deadline or dispatched work")
	}
}

func TestWorkerEnvironmentER2HostWaitPreservesQuestionAndPR(t *testing.T) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{}, "chosen-model")
	f.prepare(t)
	f.poll(t)
	f.poll(t)
	wf, e := f.service.workflowRecords(f.run)
	turn := nativeOrcaCurrentTurn(e)
	fixed := workerSessionContext(f.run, e)
	fixed.TaskID, fixed.CurrentTurnID, fixed.Generation = turn.TaskID, turn.TurnID, nativeOrcaGeneration(e)
	message := orcaworker.Message{ID: "original-question", FromHandle: "dispatch:" + turn.DispatchID, Type: "question", Body: "Business question", Payload: mustJSON(map[string]any{"taskId": turn.TaskID, "dispatchId": turn.DispatchID, "question": "Business question"})}
	fact, matched, err := orcaworker.DecodeMessage(message, turn.TaskID, turn.DispatchID, e.Process.Terminal, *fixed)
	if err != nil || !matched {
		t.Fatalf("question fixture did not produce an implementation-owned route: %v", err)
	}
	question, err := WriteDecisionRequest(f.run, decisionRequestFromEventFact(fact))
	require(t, err)
	f.service.setRunDecision(f.run, question)
	require(t, f.service.commitWorker(f.run))
	f.update(t, func(s *fakeNativeOrcaState) {
		s.HostWait = json.RawMessage(`{"kind":"host_permission","source":"prompt-text"}`)
		s.InboxUnavailable = true
	})
	f.poll(t)
	blocker := e.Environment.Blocker
	if blocker == nil || blocker.Fact.Confidence != "suspected" || wf.Phase != "waiting_decision" || e.Environment.DeferredDecisionID != question.ID {
		t.Fatalf("host evidence or question lost: %+v", e.Environment)
	}
	count := len(f.service.State.Decisions)
	f.poll(t)
	if len(f.service.State.Decisions) != count {
		t.Fatal("duplicate host decision")
	}
	if f.state(t).Replies != 0 || f.state(t).Sends != 0 {
		t.Fatal("input was injected into host wait")
	}
	note := "  保留原问题，不要重派任务。  "
	f.answer(t, "", note)
	wf, e = f.service.workflowRecords(f.run)
	if f.state(t).Sends != 0 || len(e.Environment.DeferredFeedback) != 1 || f.run.Decision.Status != "pending" {
		t.Fatal("host-wait feedback was sent or blocked further center actions")
	}
	f.update(t, func(s *fakeNativeOrcaState) { s.HostWait = nil; s.InboxUnavailable = false })
	f.recheck(t)
	wf, e = f.service.workflowRecords(f.run)
	if state := f.state(t); state.Sends != 1 || state.SendBody != note || len(e.Environment.DeferredFeedback) != 0 || f.run.Decision == nil || orcaQuestionID(f.run.Decision) != "original-question" {
		t.Fatalf("deferred feedback lost original text or question: sends=%d body=%q queue=%+v decision=%+v event=%q turn=%+v", state.Sends, state.SendBody, e.Environment.DeferredFeedback, f.run.Decision, deref(e.LastEvent), nativeOrcaCurrentTurn(e))
	}
	// PR merge waiting keeps the current phase when a late host diagnostic arrives.
	wf.Phase = "waiting_pr_merge"
	e.Environment.Blocker = nil
	f.update(t, func(s *fakeNativeOrcaState) {
		s.HostWait = json.RawMessage(`{"kind":"host_permission","source":"prompt-text"}`)
	})
	require(t, f.service.commitWorker(f.run))
	f.poll(t)
	if wf.Phase != "waiting_pr_merge" {
		t.Fatal("host wait reopened PR merge waiting")
	}
}

func TestWorkerEnvironmentER3RecheckKeepsOriginalDispatch(t *testing.T) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{ReadinessMode: "hold"}, "")
	f.prepare(t)
	_, e := f.service.workflowRecords(f.run)
	if e.Process == nil || nativeOrcaCurrentTurn(e) == nil {
		t.Fatalf("readiness fixture has no original dispatch: execution=%+v error=%v environment=%+v", e, e.Error, e.Environment)
	}
	original := *e.Process
	turn := nativeOrcaCurrentTurn(e)
	originalDispatch := turn.DispatchID
	f.update(t, func(s *fakeNativeOrcaState) {
		s.HostWait = json.RawMessage(`{"kind":"approval","source":"structured"}`)
	})
	f.poll(t)
	f.recheck(t)
	f.recheck(t)
	if f.state(t).Dispatches != 1 {
		t.Fatal("repeated recheck duplicated probe")
	}
	f.update(t, func(s *fakeNativeOrcaState) { s.HostWait = nil })
	f.recheck(t)
	_, e = f.service.workflowRecords(f.run)
	turn = nativeOrcaCurrentTurn(e)
	if *e.Process != original || turn.DispatchID != originalDispatch || f.state(t).Dispatches != 1 {
		t.Fatal("active original Dispatch was replaced")
	}
	f.update(t, func(s *fakeNativeOrcaState) { s.AgentIncarnation = "replacement" })
	f.poll(t)
	_, e = f.service.workflowRecords(f.run)
	if e.Environment.Blocker == nil || e.Environment.Blocker.Fact.Reason != "identity_mismatch" {
		t.Fatal("replacement identity was accepted")
	}
}

func TestWorkerEnvironmentRestartRequiresConfirmationBeforeBusinessInput(t *testing.T) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{ReadinessMode: "hold"}, "")
	f.prepare(t)
	stopTestLifecycleMessages(f.service, f.workerCancel)
	require(t, f.service.Close())
	s, err := NewService(f.config)
	require(t, err)
	f.service, f.run = s, s.find("8154")
	f.workerCancel = enableTestLifecycleMessages(s)
	require(t, s.markServiceRecoveryGate())
	f.update(t, func(state *fakeNativeOrcaState) { state.ReadinessMode = "success" })
	require(t, s.resumeConfiguredWork(context.Background()))
	for i := 0; i < 3 && !s.executionRecord(f.run).Environment.Preparation.Verified; i++ {
		f.poll(t)
	}
	e := s.executionRecord(f.run)
	if !e.RecoveryRequired || !e.Environment.Preparation.Verified || f.state(t).Dispatches != 1 || e.TurnNumber != 0 {
		t.Fatalf("restart readiness sent business input or lost original facts: preparation=%+v error=%q turn=%+v", e.Environment.Preparation, deref(e.Error), nativeOrcaCurrentTurn(e))
	}
	if e.Environment.Blocker == nil || e.Environment.Blocker.Fact.Reason != "service_recovery_required" {
		t.Fatal("verified readiness did not retain an actionable recovery gate")
	}
	f.recheck(t)
	e = s.executionRecord(f.run)
	if e.RecoveryRequired || !e.Environment.Ready || f.state(t).Dispatches != 2 {
		t.Fatalf("explicit confirmation did not continue the original worker: %+v", e)
	}
}

func TestWorkerEnvironmentER4DeadlineRestartAndLostReceipt(t *testing.T) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{ReadinessMode: "hold", LoseDispatchAt: 1}, "")
	f.prepare(t)
	_, e := f.service.workflowRecords(f.run)
	deadline := e.Environment.Preparation.Deadline
	nonce := e.Environment.Preparation.Nonce
	// Exact original mutation can be reconciled; no second injection occurs.
	f.recheck(t)
	if f.state(t).Dispatches != 1 {
		t.Fatal("response loss duplicated dispatch")
	}
	e = f.service.executionRecord(f.run)
	if turn := nativeOrcaCurrentTurn(e); turn == nil || turn.DispatchID == "" {
		t.Fatal("reconciled Dispatch identity was not written back to its binding envelope")
	}
	stopTestLifecycleMessages(f.service, f.workerCancel)
	require(t, f.service.Close())
	s, err := NewService(f.config)
	require(t, err)
	f.service, f.run = s, s.find("8154")
	f.workerCancel = enableTestLifecycleMessages(s)
	_, e = s.workflowRecords(f.run)
	if e.Environment.Preparation.Nonce != nonce || !e.Environment.Preparation.Deadline.Equal(deadline) {
		t.Fatal("restart reset preparation identity or deadline")
	}
	deadline = time.Now().Add(-time.Second)
	e.Environment.Preparation.Deadline = deadline
	require(t, s.commitWorker(f.run))
	f.poll(t)
	if e.Environment.Blocker == nil || e.Environment.Blocker.Fact.Reason != "preparation_unverified" {
		t.Fatal("expired preparation has no actionable decision")
	}
	f.update(t, func(s *fakeNativeOrcaState) { s.ReadinessMode = "success" })
	f.poll(t)
	if f.state(t).Dispatches != 1 || !e.Environment.Preparation.Verified {
		t.Fatalf("late success auto-dispatched or failed to record history: execution=%+v preparation=%+v turn=%+v error=%s state=%+v", e, e.Environment.Preparation, nativeOrcaCurrentTurn(e), deref(e.Error), f.state(t))
	}
	f.recheck(t)
	_, e = s.workflowRecords(f.run)
	if f.state(t).Dispatches != 2 || !e.Environment.Ready {
		t.Fatalf("explicit recheck failed to accept original complete facts: %+v", e)
	}
	business := nativeOrcaCurrentTurn(e)
	business.IntentAt = now().Add(-3 * time.Minute)
	require(t, persistNativeOrcaTurn(e, business))
	require(t, s.commitWorker(f.run))
	require(t, s.observeWorkerEnvironment(context.Background(), f.run))
	if e.Environment.Blocker == nil || e.Environment.Blocker.Fact.Reason != "business_start_unverified" {
		t.Fatal("first business start deadline did not retain its original Dispatch")
	}
	f.poll(t)
	if f.state(t).Dispatches != 2 {
		t.Fatal("business dispatched more than once")
	}
	f.recheck(t)
	_, e = s.workflowRecords(f.run)
	if e.Environment.Blocker != nil || s.workflowRecord(f.run).Phase != "executing" || f.state(t).Dispatches != 2 {
		t.Fatal("verified business start did not reconcile without reinjection")
	}
}

func TestEnvironmentRecheckCannotClearNewFaultObservedDuringPoll(t *testing.T) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{ReadinessMode: "hold"}, "")
	f.prepare(t)
	f.update(t, func(state *fakeNativeOrcaState) {
		state.HostWait = json.RawMessage(`{"kind":"approval","source":"structured"}`)
	})
	require(t, f.service.observeWorkerEnvironment(context.Background(), f.run))
	e := f.service.executionRecord(f.run)
	oldBlocker := e.Environment.Blocker.ID
	f.update(t, func(state *fakeNativeOrcaState) { state.HostWait = nil; state.ReadinessMode = "tool_failure" })
	f.recheck(t)
	e = f.service.executionRecord(f.run)
	if e.Environment.Blocker == nil || e.Environment.Blocker.ID == oldBlocker || e.Environment.Blocker.Fact.Reason != "tool_initialization_failed" || f.run.Decision == nil || f.run.Decision.Status != "pending" || f.state(t).Dispatches != 1 {
		t.Fatalf("old recheck cleared newer fault: %+v / %+v", e.Environment, f.run.Decision)
	}
}

func TestManagedOrcaFrozenResourceEvidenceIsVerifiedBeforeInput(t *testing.T) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{ReadinessMode: "hold"}, "")
	f.prepare(t)
	e := f.service.executionRecord(f.run)
	e.Environment.WriteRoots = append(e.Environment.WriteRoots, "C:/")
	if err := f.service.verifyHostExecutionEvidence(context.Background(), f.run, e); err == nil {
		t.Fatal("changed writable roots were accepted")
	}
}

func testManagedLaunchRecovery(t *testing.T, renamed bool) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{LoseFixedTerminalOnce: !renamed, RenameFixedAgentAfterCreate: renamed}, "gpt-6-luna")
	f.prepare(t)
	if !renamed && f.run.Process != nil {
		t.Fatal("unknown terminal receipt guessed")
	}
	if renamed {
		require(t, f.service.Close())
		s, err := NewService(f.config)
		require(t, err)
		f.service, f.run = s, s.find("8154")
		require(t, s.resumeConfiguredWork(context.Background()))
	} else {
		f.recheck(t)
	}
	f.poll(t)
	_, e := f.service.workflowRecords(f.run)
	state := f.state(t)
	if !state.FixedAgentTerminalCreated || state.Dispatches != 2 || e.Process == nil || !strings.Contains(state.FixedAgentCommand, "'gpt-6-luna'") {
		t.Fatalf("original launch was not reconciled: %+v / %+v", state, e)
	}
	if state.CreatedWithAgent != "" || len(e.Environment.WriteRoots) != 5 {
		t.Fatalf("launch evidence missing: %+v", e.Environment)
	}
}
