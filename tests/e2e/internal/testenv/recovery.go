package testenv

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/erranliu/SteerLine/tests/e2e/internal/evidence"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
)

type ConsoleStep struct{ Input, WaitFor string }

func OriginalTerminalWorkingObserved(s Snapshot) bool {
	return len(s.Domain.Executions) == 1 && evidence.OrcaOriginalWorkingObserved(s.Domain.Executions[0])
}

func (p *Project) ConfigureRecovery(f controlled.RecoveryFixture) {
	s, err := controlled.Read(p.Provider)
	p.Scenario.Must(err)
	s.Recovery = &f
	if s.Cwd == "" {
		s.Cwd = p.Repo
	}
	p.Scenario.Must(controlled.Write(p.Provider, s))
}

func (p *Project) UseControlledTerminal(environment string, f controlled.RecoveryFixture) {
	f.Orca = &controlled.TerminalFixture{Root: filepath.Join(p.Home, "native-worktrees"), Runtime: "owned-runtime", Handle: "owned-terminal", Incarnation: "original-incarnation", Environment: environment}
	p.ConfigureRecovery(f)
	pairing, closeState, err := controlled.OpenTerminalState(p.Provider)
	p.Scenario.Must(err)
	p.Scenario.t.Cleanup(closeState)
	raw, err := json.Marshal(pairing)
	p.Scenario.Must(err)
	p.Scenario.Must(controlled.Write(filepath.Join(p.Home, "orca-state-fixture.json"), map[string]string{"pairing": "orca://pair?code=" + base64.RawURLEncoding.EncodeToString(raw)}))
	p.Scenario.Must(controlled.Write(filepath.Join(p.Home, "orca-runtime.json"), map[string]string{"runtimeId": f.Orca.Runtime}))
	if environment != "" {
		store := map[string]any{"environments": []any{map[string]any{"id": environment, "preferredEndpointId": "fixture", "endpoints": []any{map[string]any{"id": "fixture", "endpoint": pairing["endpoint"], "deviceToken": pairing["deviceToken"], "publicKeyB64": pairing["publicKeyB64"]}}}}}
		p.Scenario.Must(controlled.Write(filepath.Join(p.Home, "orca-environments.json"), store))
	}
	payload := map[string]any{"executable": p.Scenario.Run.Helper, "args": []string{"orca-fixture"}, "wait_timeout": "1s", "environment": environment}
	profile := map[string]any{"id": "controlled", "name": "Controlled single terminal", "host": "orca", "agent": "codex", "implementation_configs": []any{map[string]any{"implementation_key": "orca.terminal-profile", "codec_version": 3, "payload": payload}}}
	p.Scenario.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), map[string]any{"version": 2, "execution_profiles": []any{profile}, "default_execution_profile": "controlled"}))
}

// CrashService terminates the exact owned test Job, without a product stop command.
func (p *Project) CrashService() {
	if p.Service == nil {
		p.Scenario.t.Fatal("no owned service to crash")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p.Scenario.Must(p.Service.Stop(ctx))
	p.Service = nil
}

func (p *Project) DisconnectWorker(snap Snapshot) {
	if len(snap.Domain.Executions) != 1 {
		p.Scenario.t.Fatal("fault needs one owned execution")
	}
	p.Scenario.Must(native.DisconnectCandidate(snap.Domain.Executions[0], p.Root))
}

func (p *Project) ReleaseNativeRecovery() {
	_, err := p.Driver.RunHelper(p.Scenario.Context, p.Scenario.Run.Helper, "release-recovery")
	p.Scenario.Must(err)
}

func (p *Project) WaitRecovery(label string, predicate func(Snapshot) bool) Snapshot {
	var result Snapshot
	lastError := ""
	p.Scenario.Wait(label, func() (bool, error) {
		var err error
		result, err = p.Snapshot()
		if err != nil {
			return false, err
		}
		if len(result.Domain.Executions) == 1 && result.Domain.Executions[0].Error != nil && *result.Domain.Executions[0].Error != lastError {
			lastError = *result.Domain.Executions[0].Error
			p.Record(strings.ReplaceAll(label, " ", "-")+"-latest", result)
		}
		return predicate(result), nil
	})
	p.Record(strings.ReplaceAll(label, " ", "-"), result)
	return result
}

// AnswerRecovery exercises formal optimistic concurrency. Only an explicit
// pre-accept stale_recovery rejection can refresh this test's authorized answer;
// unknown command outcomes are never retried under a different request ID.
func (p *Project) AnswerRecovery(snap Snapshot) driver.CandidateCommand {
	if snap.Runtime == nil || len(snap.Runtime.Runs) != 1 || snap.Runtime.Runs[0].Decision == nil {
		p.Scenario.t.Fatal("missing formal recovery decision")
	}
	original := snap.Runtime.Runs[0]
	for attempt := 0; attempt < 3; attempt++ {
		if snap.Runtime == nil || len(snap.Runtime.Runs) != 1 {
			p.Scenario.t.Fatal("recovery refresh lost original execution")
		}
		r := snap.Runtime.Runs[0]
		d := r.Decision
		if r.RunID != original.RunID || r.ExecutionID != original.ExecutionID || r.AttemptID != original.AttemptID || d == nil || d.Status != "pending" || d.Request.QuestionRoute != nil || len(d.Request.Options) == 0 || d.Request.Options[0].ID != "continue" || r.RecoveryState == "unverified" {
			p.Scenario.t.Fatal("original recovery is not qualified for continuation")
		}
		in := driver.CandidateCommand{Action: "answer_decision", RequestID: requestID(), RunID: r.RunID, ExecutionID: r.ExecutionID, DecisionID: d.ID, DecisionVersion: d.Version, RunRevision: r.ControlRevision, AttemptID: r.AttemptID, RecoveryVersion: r.RecoveryVersion, InstanceID: snap.Runtime.Service.InstanceID, Option: "continue", Note: "Continue this original session and verify existing work against its original Spec/Plan. Preserve correct changes; finish only the current authorized stage."}
		_, err := p.Driver.Candidate(p.Scenario.Context, p.Scenario.Run.Helper, in)
		if err == nil {
			return in
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		rejected, readErr := p.SnapshotContext(ctx)
		cancel()
		if readErr == nil {
			p.RecordRecoveryBranch("recovery-answer-rejected", rejected, map[string]any{"error": err.Error(), "request_id": in.RequestID, "refresh_attempt": attempt})
		}
		if attempt == 2 || !strings.Contains(err.Error(), "stale_recovery: source Attempt or recovery version changed") {
			p.Scenario.Must(err)
		}
		snap = p.WaitRecovery("refresh-original-recovery-consent", func(value Snapshot) bool {
			if value.Runtime == nil || len(value.Runtime.Runs) != 1 {
				return false
			}
			current := value.Runtime.Runs[0]
			decision := current.Decision
			return current.RunID == original.RunID && current.ExecutionID == original.ExecutionID && current.AttemptID == original.AttemptID && current.RecoveryState != "unverified" && decision != nil && decision.Status == "pending" && decision.Request.QuestionRoute == nil && len(decision.Request.Options) > 0 && decision.Request.Options[0].ID == "continue"
		})
	}
	p.Scenario.t.Fatal("recovery consent refresh exhausted")
	return driver.CandidateCommand{}
}

// FinishRecoveredFlow confirms each distinct product decision, never native questions.
func (p *Project) FinishRecoveredFlow(alreadyAnswered ...string) Snapshot {
	answered := map[string]bool{}
	for _, id := range alreadyAnswered {
		answered[id] = true
	}
	return p.WaitRecovery("recovered-await-merge", func(snap Snapshot) bool {
		if len(snap.Domain.WorkflowRuns) != 1 {
			return false
		}
		if snap.Domain.WorkflowRuns[0].Status == "waiting_external" {
			return true
		}
		if snap.Runtime != nil && len(snap.Runtime.Runs) == 1 {
			r := snap.Runtime.Runs[0]
			d := r.Decision
			if d != nil && d.Status == "pending" && d.Request.QuestionRoute == nil && len(d.Request.Options) > 0 && d.Request.Options[0].ID == "continue" && r.RecoveryState != "unverified" && !answered[d.ID] {
				p.AnswerRecovery(snap)
				answered[d.ID] = true
			}
		}
		return false
	})
}

func (p *Project) NativeManualCommit(text string) string {
	snap, err := p.Snapshot()
	p.Scenario.Must(err)
	if len(snap.Domain.Executions) != 1 {
		p.Scenario.t.Fatal("manual result needs one execution")
	}
	worktree := snap.Domain.Executions[0].Workspace.Worktree
	p.Scenario.Must(os.WriteFile(filepath.Join(worktree, "result.txt"), []byte(text), 0600))
	p.git(worktree, "add", "result.txt")
	p.git(worktree, "commit", "-m", "native manual continuation result")
	return p.git(worktree, "rev-parse", "HEAD")
}

func (p *Project) RecordRecoveryBranch(name string, snap Snapshot, extra map[string]any) {
	path := filepath.Join(p.Scenario.Output, "branches", p.Name+"-"+name)
	p.Scenario.Must(os.MkdirAll(path, 0700))
	value := map[string]any{"branch": name, "expected": "original identity, explicit inputs only", "actual": "assertions passed", "snapshot": snap, "cleanup": "owned by scenario final cleanup"}
	for k, v := range extra {
		value[k] = v
	}
	p.Scenario.Must(controlled.Write(filepath.Join(path, "result.json"), value))
}

func (p *Project) AssertRecoveryConsole() {
	before, err := p.Snapshot()
	p.Scenario.Must(err)
	cmd := p.Driver.Command(p.Scenario.Context, "console", "-config", p.Config)
	name := p.Name + "-recovery-console"
	if before.Helper.Recovery != nil && before.Helper.Recovery.MissingThread {
		name += "-error"
	}
	transcript, err := p.Scenario.RunPseudoConsoleSteps(cmd, name, "E2E a", ConsoleStep{Input: "u", WaitFor: "继续资格"})
	p.Scenario.Must(err)
	if strings.Contains(string(transcript), "panic:") {
		p.Scenario.t.Fatal("recovery console panicked")
	}
	after, err := p.Snapshot()
	p.Scenario.Must(err)
	first, _ := json.Marshal(before.Domain)
	second, _ := json.Marshal(after.Domain)
	if string(first) != string(second) {
		p.Scenario.t.Fatal("read-only u changed business state")
	}
	p.RecordRecoveryBranch(name, after, map[string]any{"transcript": name + ".log"})
}

func (p *Project) AssertOneDelivery(snap Snapshot) {
	if len(snap.Domain.Deliveries) != 1 || snap.Domain.WorkflowRuns[0].Status != "completed" {
		p.Scenario.t.Fatal("recovery did not produce one merged delivery")
	}
	for _, task := range snap.Page.Tasks {
		if task.TaskID == p.Tasks["b"].ID && (!task.DependencyReady || len(task.WorkflowRuns) > 0) {
			p.Scenario.t.Fatal("recovery duplicated or failed to release successor")
		}
	}
}

func (p *Project) ConfigureMissingNativeThread() {
	s, err := controlled.Read(p.Provider)
	p.Scenario.Must(err)
	if s.Recovery == nil {
		p.Scenario.t.Fatal("no native recovery fixture")
	}
	s.Recovery.MissingThread = true
	p.Scenario.Must(controlled.Write(p.Provider, s))
}
