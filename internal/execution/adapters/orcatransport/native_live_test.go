package orcatransport

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

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

type liveProbeHost struct{}

func (liveProbeHost) StartManaged(*exec.Cmd) (b.StopHandle, error) {
	return nil, errors.New("native probe owns an ordinary Orca terminal")
}
func (liveProbeHost) InspectProcess(*b.ProcessIdentity) string { return "unknown" }
func (liveProbeHost) NewProcessGeneration() string             { return "native-probe" }
func (liveProbeHost) WriteExecutionFile(string, []byte) error {
	return errors.New("native probe has no file channel")
}
func (liveProbeHost) OutputCommand(ctx context.Context, c b.LaunchCommand) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Executable, c.Args...)
	cmd.Dir = c.WorkingDirectory
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		blocked := false
		for _, b := range c.BlockedEnvironment {
			if strings.EqualFold(key, b) {
				blocked = true
			}
		}
		if !blocked {
			cmd.Env = append(cmd.Env, item)
		}
	}
	return cmd.Output()
}

func TestOrcaNativeStateLiveLifecycle(t *testing.T) {
	root := os.Getenv("STEERLINE_ORCA_STATE_PROBE_ROOT")
	if root == "" {
		t.Skip("native production lifecycle is opt-in through -Lifecycle")
	}
	root, err := filepath.Abs(root)
	if err != nil || !strings.HasPrefix(strings.ToLower(root), `c:\steerlinedebug\`) {
		t.Fatal("native lifecycle requires a generated debug package")
	}
	if _, err := os.Stat(filepath.Join(root, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	worktree, runtime, executable := os.Getenv("STEERLINE_ORCA_STATE_PROBE_WORKTREE"), os.Getenv("STEERLINE_ORCA_STATE_PROBE_RUNTIME"), os.Getenv("STEERLINE_ORCA_STATE_PROBE_CLI")
	if worktree == "" || runtime == "" || executable == "" {
		t.Fatal("missing fixed native probe target")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	repository := filepath.Join(root, "projects", "SteerlineTest")
	host := liveProbeHost{}
	creator := native.New(native.Config{Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
		return host.OutputCommand(ctx, b.LaunchCommand{Executable: executable, Args: args, WorkingDirectory: repository, Timeout: timeout})
	}})
	created, err := creator.MetaCall(ctx, "terminal", "create", "--worktree", worktree, "--title", "SteerLine production state probe", "--command", "codex", "--json")
	var result struct{ Terminal native.Terminal }
	if err != nil || created.RuntimeID != runtime || json.Unmarshal(created.Payload, &result) != nil || result.Terminal.Handle == "" {
		t.Fatalf("native ordinary terminal creation unverified: %v", err)
	}
	terminal := result.Terminal
	record := map[string]any{"inputs": 0, "results": 0, "cleaned_up": false, "runtime": runtime}
	defer func() {
		data, _ := json.MarshalIndent(record, "", "  ")
		if err := os.WriteFile(filepath.Join(root, "evidence", "native-production-lifecycle.json"), append(data, '\n'), 0600); err != nil {
			t.Error(err)
		}
	}()
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		proof, err := creator.CloseTerminalChecked(cleanup, terminal.Handle, runtime)
		record["cleaned_up"] = err == nil && proof.Disposition == "confirmed"
		if err != nil || proof.Disposition != "confirmed" {
			t.Error("probe cleanup unverified", err)
		}
	}()
	terminal, err = creator.ShowTerminal(ctx, terminal.Handle)
	if err != nil {
		t.Fatal(err)
	}
	var ready struct{ Wait struct{ Satisfied bool } }
	r, err := creator.MetaCall(ctx, "terminal", "wait", "--terminal", terminal.Handle, "--for", "tui-idle", "--timeout-ms", "60000", "--json")
	if err != nil || json.Unmarshal(r.Payload, &ready) != nil || !ready.Wait.Satisfied {
		t.Fatalf("native agent readiness unverified: %v", err)
	}
	p := Profile{Executable: executable, RepositorySelector: "id:" + strings.Split(terminal.WorktreeID, "::")[0], WaitTimeout: "40s"}
	process := b.ProcessIdentity{Kind: "orca_terminal", Terminal: terminal.Handle, RuntimeID: runtime, Incarnation: terminal.IncarnationID, WorktreeID: terminal.WorktreeID, ExecutionHost: "local"}
	s := b.LifecycleSnapshot{Ref: b.ExecutionRef{Backend: b.BackendOrca, Protocol: b.ProtocolOrcaTerminalV3, Target: p.Target(), Agent: b.AgentSpec{Name: "codex"}, RunID: strings.Repeat("a", 32), AttemptID: strings.Repeat("b", 32), External: process}, Session: b.SessionContext{ProjectID: "native-probe", ExecutionID: strings.Repeat("a", 32), AttemptID: strings.Repeat("b", 32), WorkflowRunID: strings.Repeat("a", 32), SessionID: SessionID(process)}, ConfigurationDigest: "native-probe", ResourceGeneration: "native-probe", Workspace: b.WorkspaceIntent{Path: terminal.WorktreePath, Branch: strings.TrimPrefix(terminal.Branch, "refs/heads/")}}
	raw, _ := json.Marshal(p)
	data, err := b.NewImplementationDataEnvelope(b.ImplementationDataEnvelope{CodecVersion: ProfileVersion, Purpose: b.ImplementationConfigPurpose, ImplementationKey: ProfileKey, ProjectID: "native-probe", ProfileID: "native", AcceptedConfigVersion: "native"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	port, err := NewLifecycle(b.LifecycleSetup{Snapshot: s, ProfileData: []b.ImplementationDataEnvelope{data}, ProfileID: "native", AcceptedConfigVersion: "native", ProcessHost: host, RepositoryPath: repository})
	if err != nil {
		t.Fatal(err)
	}
	l := port.(*Lifecycle)
	defer l.closeStateFeed()
	s.Implementation, err = l.envelope(s, State{Process: process})
	if err != nil {
		t.Fatal(err)
	}
	var proofs []NativeProof
	for i, command := range []string{strings.Repeat("c", 32), strings.Repeat("d", 32)} {
		input := b.TurnInput{CommandID: command, Prompt: "Read README.md and summarize its purpose in one sentence. Do not change any files, do not create commits, do not publish anything, and do not run project checks. Treat this as a read-only description task and keep the answer to one sentence."}
		s.PendingTurn = &input
		req := request(s, b.LifecyclePrepareSession, b.LifecycleInput{Turn: &input})
		req.Operation.ID += "-" + process.Terminal + "-" + command
		r, _, err := l.Execute(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		bind(&s, r)
		req = request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input})
		req.Operation.ID += "-" + process.Terminal + "-" + command
		r, observer, err := l.Execute(ctx, req)
		if err != nil || observer == nil {
			t.Fatalf("native original submission unverified: %v", err)
		}
		bind(&s, r)
		record["inputs"] = i + 1
		completed := false
		for !completed {
			select {
			case event, ok := <-observer.Events():
				if !ok {
					t.Fatal("native observation ended before complete reply")
				}
				if len(event.Implementation) > 0 {
					s.Implementation = event.Implementation
				}
				completed = event.Fact.Kind == b.EventCompleted && event.Fact.Completed
				event.Persisted <- nil
				event.Processed <- nil
			case err := <-observer.Done():
				if err != nil {
					t.Fatal(err)
				}
				if !completed {
					t.Fatal("native original result unavailable")
				}
			case <-ctx.Done():
				observer.Close(context.Background())
				t.Fatal(ctx.Err())
			}
		}
		observer.Close(context.Background())
		r, _, err = l.ObserveLifecycle(ctx, s)
		if err != nil || r.TurnEvidence == nil || r.TurnEvidence.NativeOutcome != "unknown" {
			t.Fatalf("native completed result provenance unverified: %v", err)
		}
		bind(&s, r)
		state, err := l.state(s)
		if err != nil {
			t.Fatal(err)
		}
		proof := turnFor(&state, command).Native
		proofs = append(proofs, proof)
		record["results"] = i + 1
	}
	record["proofs"] = proofs
}
