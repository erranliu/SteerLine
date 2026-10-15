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
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
)

type ConsoleStep struct {
	Input, WaitFor string
	Timeout        time.Duration
}

func OriginalTerminalWorkingObserved(s Snapshot) bool {
	return len(s.Runner.Candidates) == 1 && s.Runner.Candidates[0].Projection != nil && s.Runner.Candidates[0].Projection.Execution.TurnStarted
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
	f.Orca = &controlled.TerminalFixture{RepositoryPath: p.Repo, Root: filepath.Join(p.Home, "native-worktrees"), Runtime: "owned-runtime", Handle: "owned-terminal", Incarnation: "original-incarnation", Environment: environment}
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
	rawConfig, err := os.ReadFile(filepath.Join(p.Home, "execution.json"))
	p.Scenario.Must(err)
	var config map[string]any
	p.Scenario.Must(json.Unmarshal(rawConfig, &config))
	kind := "orca_local"
	if environment != "" {
		kind = "orca_environment"
	}
	target := map[string]string{"kind": kind}
	if environment != "" {
		target["environment"] = environment
	}
	profile := map[string]any{"id": "controlled", "name": "Controlled single terminal", "execution": map[string]any{"adapter": "orca-terminal", "target": target, "settings": map[string]any{"transport": map[string]any{"executable": p.Scenario.Run.Helper, "args": []string{"orca-fixture"}, "wait_timeout": "1s"}, "agent_launch": map[string]any{"executable": "codex", "args": []string{}}}}, "workflow": map[string]any{"id": "development", "version": 1}}
	config["execution_profiles"] = []any{profile}
	p.Scenario.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), config))
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
	if len(snap.Runner.Candidates) != 1 || snap.Runner.Candidates[0].Projection == nil {
		p.Scenario.t.Fatal("fault needs one original public Run")
	}
	c := snap.Runner.Candidates[0]
	r := c.Projection
	var input struct {
		Execution struct {
			Settings struct {
				Executable string `json:"executable"`
			} `json:"settings"`
		} `json:"execution"`
	}
	p.Scenario.Must(json.Unmarshal(c.Submit.Input, &input))
	service, err := driver.ReadRunnerService(p.Scenario.Context, filepath.Join(p.Home, "execution.json"), c.StateDir)
	p.Scenario.Must(err)
	if service.Status != "ready" || r.Execution.Backend != "local" {
		p.Scenario.t.Fatal("original native fault owner unavailable")
	}
	p.Scenario.Must(native.DisconnectCandidate(native.ProcessFaultTarget{ParentPID: service.PID, ParentCreated: service.ProcessCreated, Executable: input.Execution.Settings.Executable, Workspace: r.Execution.Workspace, Session: r.Execution.SessionID}, p.Root))
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
// pre-accept stale_version/stale_recovery rejection can refresh this test's authorized answer;
// unknown command outcomes are never retried under a different request ID.
func (p *Project) AnswerRecovery(snap Snapshot) driver.CandidateCommand {
	original := snap
	var lastErr error
	for retry := 0; retry < 3; retry++ {
		if snap.Runtime == nil || len(snap.Runtime.Runs) != 1 || len(snap.Runner.Candidates) != 1 {
			p.Scenario.t.Fatal("missing public recovery binding")
		}
		r := snap.Runtime.Runs[0]
		d := r.Decision
		public := snap.Runner.Candidates[0].Projection
		if public == nil || !public.Controls.CanResume || d == nil || d.Request.InteractionKind != "runner_resume" {
			p.Scenario.t.Fatal("original Runner does not allow explicit resume")
		}
		if retry > 0 {
			before := original.Runner.Candidates[0]
			if before.ID != snap.Runner.Candidates[0].ID || before.RunID != public.RunID || before.Projection.Execution.AttemptID != public.Execution.AttemptID || before.Projection.Phase != public.Phase || original.Runtime.Runs[0].Decision.ID != d.ID {
				p.Scenario.t.Fatal("rejected resume refresh changed original recovery target")
			}
		}
		input := driver.CandidateCommand{Action: "answer_decision", RequestID: requestID(), RunID: r.RunID, ExecutionID: r.ExecutionID, DecisionID: d.ID, DecisionVersion: d.Version, RunRevision: r.ControlRevision, AttemptID: r.AttemptID, RecoveryVersion: r.RecoveryVersion, InstanceID: snap.Runtime.Service.InstanceID, Option: "resume"}
		receipt, err := p.Driver.Candidate(p.Scenario.Context, p.Scenario.Run.Helper, input)
		if err == nil {
			return input
		}
		if receipt.PreAcceptRejection != "stale_version" && receipt.PreAcceptRejection != "stale_recovery" {
			p.Scenario.Must(err)
		}
		lastErr = err
		p.Scenario.Must(controlled.Write(filepath.Join(p.Scenario.Output, "resume-preaccept-rejection-"+input.RequestID+".json"), receipt))
		snap, err = p.Snapshot()
		p.Scenario.Must(err)
	}
	p.Scenario.Must(lastErr)
	return driver.CandidateCommand{}
}

// FinishRecoveredFlow confirms each distinct product decision, never native questions.
func (p *Project) FinishRecoveredFlow(alreadyAnswered ...string) Snapshot {
	answered := map[string]uint64{}
	for _, id := range alreadyAnswered {
		answered[id] = 0
	}
	return p.WaitRecovery("recovered-await-merge", func(snap Snapshot) bool {
		if len(snap.Runner.Candidates) != 1 || snap.Runner.Candidates[0].Projection == nil {
			return false
		}
		run := snap.Runner.Candidates[0].Projection
		if run.Status == "pr_ready" {
			return true
		}
		if !run.Controls.CanResume || snap.Runtime == nil || len(snap.Runtime.Runs) != 1 {
			return false
		}
		d := snap.Runtime.Runs[0].Decision
		if d == nil || d.Request.InteractionKind != "runner_resume" {
			return false
		}
		if version, ok := answered[d.ID]; ok && (version == 0 || version == d.Version) {
			if version == 0 {
				answered[d.ID] = d.Version
			}
			return false
		}
		if len(answered) > 3 {
			p.Scenario.t.Fatal("bounded explicit recovery exhausted")
		}
		p.AnswerRecovery(snap)
		answered[d.ID] = d.Version
		return false
	})
}

func (p *Project) NativeManualCommit(text string) string {
	snap, err := p.Snapshot()
	p.Scenario.Must(err)
	if len(snap.Runner.Candidates) != 1 || snap.Runner.Candidates[0].Projection == nil {
		p.Scenario.t.Fatal("manual result needs one public Run")
	}
	worktree := snap.Runner.Candidates[0].Projection.Execution.Workspace
	relative, err := filepath.Rel(p.Root, worktree)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		p.Scenario.t.Fatal("manual result escaped owned fixture")
	}

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
	transcript, err := p.Scenario.RunPseudoConsoleSteps(cmd, name, "E2E a", ConsoleStep{Input: "u", WaitFor: "继续资格", Timeout: 45 * time.Second})
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
	if len(snap.Runner.Graph.Deliveries) != 1 || len(snap.Runner.Candidates) != 1 || snap.Runner.Candidates[0].Participation != "completed" {
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
