//go:build e2e

package recovery

import (
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"github.com/google/uuid"
)

func qualified(s testenv.Snapshot) bool {
	if s.Runtime == nil || len(s.Runtime.Runs) != 1 || len(s.Domain.Executions) != 1 || s.Runtime.Runs[0].Decision == nil {
		return false
	}
	d, e := s.Runtime.Runs[0].Decision, s.Domain.Executions[0]
	if d.Status != "pending" || len(d.Request.Options) == 0 || d.Request.Options[0].ID != "continue" || e.RecoveryState == "unverified" {
		return false
	}
	for _, record := range s.Domain.Decisions {
		if record.DecisionID == d.ID {
			return record.Status == "pending" && record.Version == d.Version && record.RecoveryVersion == e.RecoveryVersion && record.RecoveryState == e.RecoveryState
		}
	}
	return false
}

func TestObservationReconnect(t *testing.T) {
	s := testenv.NewScenario(t, "recovery.observation-reconnect")
	p := s.NewProject("codex-drop")
	p.ConfigureRecovery(controlled.RecoveryFixture{DropStep: "implement.execute"})
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	open := p.WaitPhase("waiting_pr_merge")
	if open.Helper.Sessions != 1 || open.Helper.Dispatches != 3 || len(open.Domain.Attempts) != 1 || len(open.Domain.WorkflowRuns[0].StageResults) != 3 {
		t.Fatal("observation reconnect resent or lost original result")
	}
	p.RecordRecoveryBranch("completed-after-disconnect", open, map[string]any{"sends": open.Helper.Dispatches})
	p.StopService()
}

func TestFailureResults(t *testing.T) {
	s := testenv.NewScenario(t, "recovery.failure-results")
	f := s.NewProject("failed-report")
	f.ConfigureRecovery(controlled.RecoveryFixture{FailStep: "implement.report", NoFinalStep: "implement.report"})
	f.CreateTasks()
	f.StartService()
	f.Claim("a")
	failure := f.WaitRecovery("failed-report-qualified", qualified)
	if failure.Helper.Dispatches != 2 || len(failure.Domain.WorkflowRuns[0].StageResults) != 2 || failure.Domain.WorkflowRuns[0].StageResults[1].Succeeded {
		t.Fatal("failed report without final did not retain failure")
	}
	// Inspect through the public HTTP recovery route with this owned service live.
	f.AssertRecoveryHTTP(failure)
	f.StopService()
	f.ConfigureMissingNativeThread()
	f.StartService()
	missing := f.WaitRecovery("missing-thread-unverified", func(s testenv.Snapshot) bool {
		return s.Runtime != nil && len(s.Runtime.Runs) == 1 && len(s.Domain.Executions) == 1 &&
			s.Domain.Executions[0].RecoveryState == "unverified" && s.Runtime.Runs[0].Decision != nil &&
			s.Runtime.Runs[0].Decision.Status == "pending" && len(s.Runtime.Runs[0].Decision.Request.Options) == 0
	})
	f.AssertRecoveryHTTP(missing)
	s.Must(s.FinishBody())
	f.StopService()
}

func TestMissingSuccessfulResult(t *testing.T) {
	s := testenv.NewScenario(t, "recovery.missing-successful-result")
	g := s.NewProject("missing-success-final")
	g.ConfigureRecovery(controlled.RecoveryFixture{NoFinalStep: "implement.report"})
	g.CreateTasks()
	g.StartService()
	g.Claim("a")
	gap := g.WaitRecovery("success-final-gap", func(s testenv.Snapshot) bool {
		return len(s.Domain.WorkflowRuns) == 1 && s.Domain.WorkflowRuns[0].Status == "reconciling"
	})
	if gap.Helper.Dispatches != 2 || len(gap.Domain.WorkflowRuns[0].StageResults) != 1 {
		t.Fatal("success gap became new input or false success")
	}
	g.RecordRecoveryBranch("success-final-gap", gap, nil)
	g.StopService()
}

func TestTerminalLocalReconnect(t *testing.T) {
	testTerminalReconnect(t, "recovery.terminal-local-reconnect", "")
}
func TestTerminalEnvironmentReconnect(t *testing.T) {
	testTerminalReconnect(t, "recovery.terminal-environment-reconnect", "owned-environment")
}

func testTerminalReconnect(t *testing.T, id, environment string) {
	s := testenv.NewScenario(t, id)

	name := "orca-local"
	if environment != "" {
		name = "orca-environment"
	}
	o := s.NewProject(name)
	o.UseControlledTerminal(environment, controlled.RecoveryFixture{HoldStep: "implement.execute"})
	o.CreateTasks()
	o.StartService()
	o.Claim("a")
	original := o.WaitRecovery("original-terminal-active", func(s testenv.Snapshot) bool {
		return len(s.Domain.Executions) == 1 && s.Helper.Recovery.Orca.Sends == 1 && testenv.OriginalTerminalWorkingObserved(s)
	})
	o.CrashService()
	o.ReleaseNativeRecovery()
	o.StartService()
	resumed := o.FinishRecoveredFlow()
	first, last := original.Domain.Executions[0], resumed.Domain.Executions[0]
	if first.ExecutionID != last.ExecutionID || first.CurrentAttemptID != last.CurrentAttemptID || first.SessionID != last.SessionID || *first.Process != *last.Process || resumed.Helper.Recovery.Orca.Creates != 1 || resumed.Helper.Recovery.Orca.Sends != 3 || len(resumed.Domain.WorkflowRuns[0].StageResults) != 3 {
		t.Fatal("single terminal recovery replaced or duplicated original input")
	}
	o.StopService()
	state, err := controlled.Read(o.Provider)
	s.Must(err)
	state.Recovery.Orca.Lost = true
	s.Must(controlled.Write(o.Provider, state))
	o.StartService()
	saved := o.WaitPhase("waiting_pr_merge")
	if saved.Helper.Recovery.Orca.Sends != 3 || len(saved.Domain.WorkflowRuns[0].StageResults) != 3 {
		t.Fatal("saved results required the lost original terminal or added input")
	}
	o.RecordRecoveryBranch("original-terminal-reconnect", resumed, map[string]any{"environment": environment, "sends": resumed.Helper.Recovery.Orca.Sends})
	o.StopService()
}

func testCodexResume(t *testing.T, id string, changed bool) {
	s := testenv.NewScenario(t, id)
	name := "same-report-source"
	if changed {
		name = "manual-result"
	}
	p := s.NewProject(name)
	failedStep := "implement.report"
	if changed {
		failedStep = "implement.execute"
	}
	p.ConfigureRecovery(controlled.RecoveryFixture{FailStep: failedStep, NoFinalStep: failedStep})
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	before := p.WaitRecovery("failed-original-report", qualified)
	p.DisconnectWorker(before)
	p.CrashService()
	candidate := ""
	if changed {
		candidate = p.NativeManualCommit("controlled E2E result\nmanual native result\n")
	}
	p.StartService()
	gate := p.WaitRecovery("original-session-restore-ready", qualified)
	answer := p.AnswerRecovery(gate)
	after := p.FinishRecoveredFlow(answer.DecisionID)
	first, last := before.Domain.Executions[0], after.Domain.Executions[0]
	if first.ExecutionID != last.ExecutionID || first.SessionID != last.SessionID || first.Workspace.Worktree != last.Workspace.Worktree || first.CurrentAttemptID == last.CurrentAttemptID || len(after.Domain.Attempts) != 2 || after.Helper.Sessions != 1 {
		t.Fatal("resume replaced original thread or failed to advance Attempt")
	}
	results := after.Domain.WorkflowRuns[0].StageResults
	failedIndex := 1
	if changed {
		failedIndex = 0
	}
	if results[failedIndex].Succeeded || results[failedIndex].Failure == "" {
		t.Fatal("old native failure overwritten")
	}
	want := 5
	if changed {
		want = 4
		if results[1].StepID != "implement.execute" || after.Helper.PR.HeadRevision == candidate {
			t.Fatal("changed manual work skipped contract verification")
		}
	}
	if after.Helper.Dispatches != want {
		t.Fatalf("business sends=%d expected %d", after.Helper.Dispatches, want)
	}
	p.Merge()
	closed := p.WaitPhase("completed")
	p.AssertOneDelivery(closed)
	p.RecordRecoveryBranch("same-thread-recovery", closed, map[string]any{"changed": changed, "sends": want})
	p.StopService()
}
func TestCodexResume(t *testing.T)       { testCodexResume(t, "recovery.codex-resume", true) }
func TestCodexReportResume(t *testing.T) { testCodexResume(t, "recovery.codex-report-resume", false) }

func TestChangedReportSourceReverification(t *testing.T) {
	s := testenv.NewScenario(t, "recovery.report-source-changed")
	p := s.NewProject("changed-report-source")
	p.ConfigureRecovery(controlled.RecoveryFixture{FailStep: "implement.report", NoFinalStep: "implement.report"})
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	before := p.WaitRecovery("old-report-failed", qualified)
	manual := p.NativeManualCommit("controlled E2E result\nmanual native result\n")
	answer := p.AnswerRecovery(before)
	open := p.FinishRecoveredFlow(answer.DecisionID)
	results := open.Domain.WorkflowRuns[0].StageResults
	inputs := open.Domain.WorkflowRuns[0].StageInputs
	if open.Helper.Dispatches != 5 || len(results) != 5 || results[1].Succeeded || results[2].StepID != "implement.execute" || open.Helper.PR.HeadRevision == manual || inputs[3].SourceCommandID != results[2].CommandID || open.Domain.Executions[0].CurrentAttemptID != before.Domain.Executions[0].CurrentAttemptID || open.Helper.Sessions != 1 {
		t.Fatal("changed report source skipped implementation or replaced the live original resource")
	}
	p.Merge()
	done := p.WaitPhase("completed")
	p.AssertOneDelivery(done)
	p.RecordRecoveryBranch("changed-report-source-reverified", done, map[string]any{"sends": 5, "manual_head": manual})
	p.StopService()
}

func TestRecoveryAnswerReplay(t *testing.T) {
	s := testenv.NewScenario(t, "recovery.answer-replay")
	p := s.NewProject("answer-replay")
	p.ConfigureRecovery(controlled.RecoveryFixture{FailStep: "implement.execute"})
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	gate := p.WaitRecovery("original-failure-qualified", qualified)
	p.ConfigureRecovery(controlled.RecoveryFixture{HoldStep: "implement.execute"})
	answer := p.AnswerRecovery(gate)
	p.WaitRecovery("authorized-original-input-accepted", func(snap testenv.Snapshot) bool {
		return snap.Helper.Dispatches == 2 && snap.Domain.Executions[0].CurrentTurnID != snap.Domain.Executions[0].LastStoppedTurnID
	})
	_, replayErr := p.Driver.Candidate(s.Context, s.Run.Helper, answer)
	stale := answer
	stale.RequestID = strings.ReplaceAll(uuid.NewString(), "-", "")
	stale.Note = "Different input using an expired recovery decision"
	_, staleErr := p.Driver.Candidate(s.Context, s.Run.Helper, stale)
	if staleErr == nil {
		t.Fatal("expired decision accepted different input")
	}
	after, err := p.Snapshot()
	s.Must(err)
	if after.Helper.Dispatches != 2 || after.Helper.Sessions != 1 || len(after.Domain.Attempts) != 1 || after.Domain.Executions[0].SessionID != gate.Domain.Executions[0].SessionID {
		t.Fatal("replayed or expired answer added input or replaced original resource")
	}
	p.RecordRecoveryBranch("duplicate-and-expired-answer", after, map[string]any{"replay_rejected": replayErr != nil, "expired_rejected": true, "sends": 2})
	p.StopService()
}

func TestResumeGates(t *testing.T) {
	s := testenv.NewScenario(t, "recovery.resume-gates")
	p := s.NewProject("missing-thread")
	p.ConfigureRecovery(controlled.RecoveryFixture{FailStep: "implement.execute"})
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	original := p.WaitRecovery("original-failed-turn", qualified)
	p.CrashService()
	p.ConfigureMissingNativeThread()
	p.StartService()
	blocked := p.WaitRecovery("missing-thread-blocked", func(s testenv.Snapshot) bool {
		return len(s.Domain.Executions) == 1 && s.Domain.Executions[0].RecoveryState == "unverified" && s.Runtime.Runs[0].Decision != nil && len(s.Runtime.Runs[0].Decision.Request.Options) == 0
	})
	if blocked.Helper.Sessions != original.Helper.Sessions || blocked.Helper.Dispatches != original.Helper.Dispatches {
		t.Fatal("missing original thread recreated or resent input")
	}
	p.RecordRecoveryBranch("missing-thread", blocked, nil)
	p.StopService()
}

func testTerminalUnknownSend(t *testing.T, id, environment string) {
	s := testenv.NewScenario(t, id)

	name := "unknown-send-local"
	if environment != "" {
		name = "unknown-send-environment"
	}
	o := s.NewProject(name)
	o.UseControlledTerminal(environment, controlled.RecoveryFixture{HoldStep: "implement.execute"})
	state, err := controlled.Read(o.Provider)
	s.Must(err)
	state.Recovery.Orca.LostSend = true
	s.Must(controlled.Write(o.Provider, state))
	o.CreateTasks()
	o.StartService()
	o.ClaimExpectedStatus("a", "indeterminate")
	before := o.WaitRecovery("original-send-unknown", func(s testenv.Snapshot) bool {
		return s.Helper.Recovery.Orca.Sends == 1 && len(s.Domain.Executions) == 1
	})
	o.CrashService()
	o.ReleaseNativeRecovery()
	o.StartService()
	after := o.WaitRecovery("unknown-send-remains-unverified", func(s testenv.Snapshot) bool {
		return len(s.Domain.Executions) == 1 && s.Domain.Executions[0].RecoveryState == "unverified" && s.Runtime.Runs[0].Decision != nil && len(s.Runtime.Runs[0].Decision.Request.Options) == 0
	})
	if after.Helper.Recovery.Orca.Sends != 1 || after.Helper.Recovery.Orca.Creates != 1 || after.Domain.Executions[0].SessionID != before.Domain.Executions[0].SessionID || len(after.Domain.WorkflowRuns[0].StageResults) != 0 {
		t.Fatal("unknown original send was repeated or its unbound reply was adopted")
	}
	o.RecordRecoveryBranch("unknown-send-refused", after, map[string]any{"environment": environment})
	o.StopService()
}
func TestTerminalLocalUnknownSend(t *testing.T) {
	testTerminalUnknownSend(t, "recovery.terminal-local-unknown-send", "")
}
func TestTerminalEnvironmentUnknownSend(t *testing.T) {
	testTerminalUnknownSend(t, "recovery.terminal-environment-unknown-send", "owned-environment")
}

func TestTerminalResourceGates(t *testing.T) {
	testTerminalResourceGate(t, "recovery.terminal-resource-gates", "lost")
}
func TestTerminalTargetGate(t *testing.T) {
	testTerminalResourceGate(t, "recovery.terminal-target-gate", "target")
}
func TestTerminalIncarnationGate(t *testing.T) {
	testTerminalResourceGate(t, "recovery.terminal-incarnation-gate", "incarnation")
}
func TestTerminalGapGate(t *testing.T) {
	testTerminalResourceGate(t, "recovery.terminal-gap-gate", "gap")
}
func testTerminalResourceGate(t *testing.T, id, fault string) {
	s := testenv.NewScenario(t, id)
	o := s.NewProject("lost-terminal")
	o.UseControlledTerminal("", controlled.RecoveryFixture{HoldStep: "implement.execute"})
	o.CreateTasks()
	o.StartService()
	o.Claim("a")
	o.WaitRecovery("active-before-terminal-loss", func(s testenv.Snapshot) bool {
		return s.Helper.Recovery.Orca.Sends == 1 && len(s.Domain.Executions) == 1
	})
	{
		previous, err := o.Snapshot()
		s.Must(err)
		previousDecision := ""
		if previous.Runtime.Runs[0].Decision != nil {
			previousDecision = previous.Runtime.Runs[0].Decision.ID
		}
		o.CrashService()
		state, err := controlled.Read(o.Provider)
		s.Must(err)
		state.Recovery.Orca.WrongTarget = fault == "target"
		state.Recovery.Orca.Incarnation = "original-incarnation"
		if fault == "incarnation" {
			state.Recovery.Orca.Incarnation = "replacement"
		}
		state.Recovery.Orca.Gap = fault == "gap"
		state.Recovery.Orca.Lost = fault == "lost"
		s.Must(controlled.Write(o.Provider, state))
		o.StartService()
		lost := o.WaitRecovery("original-resource-"+fault+"-blocked", func(s testenv.Snapshot) bool {
			if fault == "gap" {
				if len(s.Domain.Executions) != 1 || s.Domain.Executions[0].Error == nil || !strings.Contains(*s.Domain.Executions[0].Error, "subscription is unverified") || len(s.Domain.WorkflowRuns[0].StageResults) != 0 {
					return false
				}
				d := s.Runtime.Runs[0].Decision
				return d == nil || len(d.Request.Options) == 0
			}
			if len(s.Domain.Executions) != 1 || s.Domain.Executions[0].RecoveryState != "unverified" || s.Domain.Executions[0].Error == nil || s.Runtime.Runs[0].Decision == nil {
				return false
			}
			d := s.Runtime.Runs[0].Decision
			for _, record := range s.Domain.Decisions {
				if record.DecisionID == d.ID {
					return d.ID != previousDecision && record.Status == "pending" && record.RecoveryVersion == s.Domain.Executions[0].RecoveryVersion && len(d.Request.Options) == 0
				}
			}
			return false
		})
		if lost.Helper.Recovery.Orca.Creates != 1 || lost.Helper.Recovery.Orca.Sends != 1 {
			t.Fatal("unverified original resource replaced or resent")
		}
		o.RecordRecoveryBranch(fault, lost, nil)
	}
	o.StopService()
}
