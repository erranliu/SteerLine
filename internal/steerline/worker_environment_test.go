package steerline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type environmentFixture struct {
	service   *Service
	config    Config
	statePath string
	run       *Run
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
	c.Host, c.Agent, c.Sandbox, c.WindowsSandbox, c.Model, c.ReasoningEffort, c.Claimant = "orca", "codex", "workspace-write", "unelevated", model, "", "operator"
	if model != "" {
		c.ReasoningEffort = "high"
	}
	c.Codex = Command{Executable: exe}
	c.Orca = OrcaConfig{Executable: exe, Args: []string{"--fake", "orca-native", stateDir}, Shell: "powershell.exe", WaitTimeout: "2m"}
	c.configDigest = digestValue(c)
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
	f := &environmentFixture{service: s, config: c, statePath: statePath, run: s.find("8154")}
	t.Cleanup(func() { _ = f.service.Close() })
	return f
}

func (f *environmentFixture) prepare(t *testing.T) {
	t.Helper()
	task := nativeTaskRecord(f.service.State.Tasks, "8154")
	require(t, f.service.prepareWorkerFlow(context.Background(), *task, f.run))
}
func (f *environmentFixture) poll(t *testing.T) {
	t.Helper()
	require(t, f.service.pollNativeOrcaWorker(context.Background(), f.run))
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
	state := f.state(t)
	change(&state)
	data, _ := json.Marshal(state)
	require(t, AtomicWrite(f.statePath, data))
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
	require(t, s.save())
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
			f.poll(t)
			wf, e := f.service.workflowRecords(f.run)
			state := f.state(t)
			if mode == "success" {
				if state.Dispatches != 2 || !e.Environment.Ready || e.TurnNumber != 0 || e.CurrentTurnID != "" || len(e.OrcaNative.Turns) != 2 || e.OrcaNative.Turns[0].Purpose != "readiness" {
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
			if !strings.Contains(state.FixedAgentCommand, "'never'") || !strings.Contains(state.FixedAgentCommand, "'windows.sandbox=unelevated'") || state.CreatedWithAgent != "" {
				t.Fatalf("managed permissions missing: %+v", state)
			}
			if strings.Contains(state.FixedAgentCommand, "'--model'") {
				t.Fatal("default model replaced")
			}
		})
	}
}

func TestWorkerEnvironmentER2HostWaitPreservesQuestionAndPR(t *testing.T) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{}, "chosen-model")
	f.prepare(t)
	f.poll(t)
	f.poll(t)
	wf, e := f.service.workflowRecords(f.run)
	turn := nativeOrcaCurrentTurn(e)
	question, err := WriteDecisionRequest(f.run, DecisionRequest{Question: "Business question", AllowFreeText: true, OrcaAsk: &orcaAskCall{RunID: e.Target.OrchestrationRun, TaskID: turn.TaskID, DispatchID: turn.DispatchID, MessageID: "original-question", Questions: []nativeAskQuestion{{ID: "original-question", Question: "Business question", AllowFreeText: true}}}})
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
	if f.state(t).Sends != 0 || len(e.Environment.DeferredFeedback) != 1 || f.run.Decision.Status != "pending" {
		t.Fatal("host-wait feedback was sent or blocked further center actions")
	}
	f.update(t, func(s *fakeNativeOrcaState) { s.HostWait = nil; s.InboxUnavailable = false })
	f.recheck(t)
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
	if *e.Process != original || turn.DispatchID != originalDispatch || f.state(t).Dispatches != 1 {
		t.Fatal("active original Dispatch was replaced")
	}
	f.update(t, func(s *fakeNativeOrcaState) { s.AgentIncarnation = "replacement" })
	f.poll(t)
	if e.Environment.Blocker == nil || e.Environment.Blocker.Fact.Reason != "identity_mismatch" {
		t.Fatal("replacement identity was accepted")
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
	require(t, f.service.Close())
	s, err := NewService(f.config)
	require(t, err)
	f.service, f.run = s, s.find("8154")
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
	if f.state(t).Dispatches != 2 || !e.Environment.Ready {
		t.Fatalf("explicit recheck failed to accept original complete facts: %+v", e)
	}
	business := nativeOrcaCurrentTurn(e)
	business.IntentAt = now().Add(-3 * time.Minute)
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
	if e.Environment.Blocker == nil || e.Environment.Blocker.ID == oldBlocker || e.Environment.Blocker.Fact.Reason != "tool_initialization_failed" || f.run.Decision == nil || f.run.Decision.Status != "pending" || f.state(t).Dispatches != 1 {
		t.Fatalf("old recheck cleared newer fault: %+v / %+v", e.Environment, f.run.Decision)
	}
}

func TestManagedOrcaFrozenResourceEvidenceIsVerifiedBeforeInput(t *testing.T) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{ReadinessMode: "hold"}, "")
	f.prepare(t)
	e := f.service.executionRecord(f.run)
	e.Environment.WriteRoots = append(e.Environment.WriteRoots, "C:/")
	if err := f.service.verifyNativeOrcaWorker(context.Background(), f.run, e); err == nil {
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
