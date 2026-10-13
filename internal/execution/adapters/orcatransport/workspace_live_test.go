package orcatransport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	"github.com/erranliu/SteerLine/internal/execution/agentlaunch"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

// A small opt-in probe of the changed startup path, not a delivery/PR scenario.
func TestOrcaWorkspaceStartupLive(t *testing.T) {
	root := os.Getenv("STEERLINE_ORCA_WORKSPACE_PROBE_ROOT")
	if root == "" {
		t.Skip("requires a generated debug package")
	}
	root, err := filepath.Abs(root)
	if err != nil || !strings.HasPrefix(strings.ToLower(root), `c:\steerlinedebug\`) {
		t.Fatal("probe must use an independent debug package")
	}
	if _, err := os.Stat(filepath.Join(root, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Profiles []struct {
			ID      string              `json:"id"`
			Launch  *agentlaunch.Config `json:"agent_launch"`
			Configs []struct {
				Payload json.RawMessage `json:"payload"`
			} `json:"implementation_configs"`
		} `json:"execution_profiles"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "home", "execution.json"))
	if err != nil || json.Unmarshal(raw, &catalog) != nil || len(catalog.Profiles) != 1 || len(catalog.Profiles[0].Configs) != 1 || catalog.Profiles[0].Launch == nil {
		t.Fatal("missing generated Orca launch profile", err)
	}
	profile := catalog.Profiles[0]
	p, err := DecodeProfile(profile.Configs[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	launch, err := profile.Launch.Command("codex")
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Join(root, "projects", "SteerlineTest")
	base, err := exec.Command("git", "-C", repository, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	host := liveProbeHost{}
	client := native.New(native.Config{Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
		if p.Environment != "" {
			args = append(args, "--environment", p.Environment)
		}
		return host.OutputCommand(ctx, b.LaunchCommand{Executable: p.Executable, Args: args, WorkingDirectory: repository, Timeout: timeout})
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if _, err := client.MetaCall(ctx, "repo", "add", "--path", repository, "--json"); err != nil {
		t.Fatal(err)
	}
	for _, recovery := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "saved-receipt-recovery"}[recovery], func(t *testing.T) {
			id := make([]byte, 16)
			if _, err := rand.Read(id); err != nil {
				t.Fatal(err)
			}
			runID := hex.EncodeToString(id)
			s := b.LifecycleSnapshot{Ref: b.ExecutionRef{Backend: b.BackendOrca, Protocol: b.ProtocolOrcaTerminalV3, Target: p.Target(), Agent: b.AgentSpec{Name: "codex"}, RunID: runID, AttemptID: runID}, Session: b.SessionContext{ProjectID: repository, TaskID: "startup-probe", ExecutionID: runID, AttemptID: runID, WorkflowRunID: runID}, ConfigurationDigest: "startup-probe", ResourceGeneration: runID, WorkspacePlan: b.WorkspaceIntent{BaseCommit: strings.TrimSpace(string(base))}}
			data, err := b.NewImplementationDataEnvelope(b.ImplementationDataEnvelope{CodecVersion: ProfileVersion, Purpose: b.ImplementationConfigPurpose, ImplementationKey: ProfileKey, ProjectID: repository, ProfileID: profile.ID, AcceptedConfigVersion: "startup-probe"}, profile.Configs[0].Payload)
			if err != nil {
				t.Fatal(err)
			}
			setup := b.LifecycleSetup{Snapshot: s, ProfileData: []b.ImplementationDataEnvelope{data}, ProfileID: profile.ID, AcceptedConfigVersion: "startup-probe", ProcessHost: host, RepositoryPath: repository, RunDirectory: root, AgentLaunchCommand: launch}
			port, err := NewLifecycle(setup)
			if err != nil {
				t.Fatal(err)
			}
			l := port.(*Lifecycle)
			defer l.closeStateFeed()
			injected := false
			calls := workspaceCalls(l, func(args []string) ([]byte, bool) {
				if recovery && !injected && args[0] == "terminal" && args[1] == "show" {
					injected = true
					return workspaceQueryError("probe_query_unavailable"), true
				}
				return nil, false
			})
			req := request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan})
			req.Operation.ID += "-" + runID
			receipt, _, startErr := l.Execute(ctx, req)
			var creation workspaceCreation
			if json.Unmarshal(receipt.Operation.Receipt, &creation) != nil {
				t.Fatal("creation response was not retained", startErr)
			}
			record := map[string]any{"recovery": recovery, "worktree": creation.Worktree, "runtime": creation.RuntimeID, "passed": false, "cleaned_up": false}
			defer func() {
				cleanup, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cleanupCancel()
				handle := creation.Terminal.Handle
				if handle != "" {
					proof, closeErr := client.CloseTerminalChecked(cleanup, handle, creation.RuntimeID)
					if closeErr != nil || proof.Disposition != "confirmed" {
						t.Error("terminal cleanup unverified", closeErr)
						return
					}
				}
				_, removeErr := client.MetaCall(cleanup, "worktree", "rm", "--worktree", "id:"+creation.Worktree.ID, "--force", "--json")
				record["cleaned_up"] = removeErr == nil
				if removeErr != nil {
					t.Error("worktree cleanup unverified", removeErr)
				}
				record["calls"] = *calls
				raw, _ := json.MarshalIndent(record, "", "  ")
				if err := os.WriteFile(filepath.Join(root, "evidence", "orca-startup-"+t.Name()[strings.LastIndex(t.Name(), "/")+1:]+".json"), raw, 0600); err != nil {
					t.Error(err)
				}
			}()
			if recovery {
				if startErr == nil || !injected {
					t.Fatal("query fault was not exercised")
				}
				checkpoint := filepath.Join(root, "evidence", "startup-creation-receipt.json")
				wire, _ := json.Marshal(receipt)
				if err := os.WriteFile(checkpoint, wire, 0600); err != nil {
					t.Fatal(err)
				}
				req.Operation.Receipt, err = os.ReadFile(checkpoint)
				if err != nil {
					t.Fatal(err)
				}
				port, err = NewLifecycle(setup) // Restore only the persisted operation receipt.
				if err != nil {
					t.Fatal(err)
				}
				l = port.(*Lifecycle)
				defer l.closeStateFeed()
				restoredCalls := workspaceCalls(l, nil)
				receipt, _, startErr = l.Reconcile(ctx, req)
				assertWorkspaceReadonly(t, *restoredCalls)
			}
			if startErr != nil || receipt.Process == nil || receipt.Disposition != b.OperationConfirmed {
				t.Fatal("native workspace binding failed", startErr)
			}
			bind(&s, receipt)
			record["workspace_binding_verified"] = true
			// Trust only this probe checkout through session argv, never the user's
			// global config. Resource preparation has not sent an agent command yet.
			setup.Snapshot = s
			setup.AgentLaunchCommand = launch
			for _, path := range []string{filepath.FromSlash(creation.Worktree.Path), repository} {
				trustedPath, _ := json.Marshal(path)
				trust := "projects." + string(trustedPath) + ".trust_level=\"trusted\""
				setup.AgentLaunchCommand += " '-c' '" + strings.ReplaceAll(trust, "'", "''") + "'"
			}
			port, err = NewLifecycle(setup)
			if err != nil {
				t.Fatal(err)
			}
			l = port.(*Lifecycle)
			defer l.closeStateFeed()
			launchCalls := workspaceCalls(l, nil)
			candidate, err := l.Next(ctx, s)
			if err != nil || candidate.Action != b.LifecycleCreateResource {
				t.Fatal(candidate, err)
			}
			launchReq := request(s, candidate.Action, candidate.Input)
			launchReq.Operation.ID += "-" + runID
			launched, _, err := l.Execute(ctx, launchReq)
			if err != nil {
				t.Fatal("native startup was not accepted", err)
			}
			bind(&s, launched)
			record["agent_start_commands"] = 1
			startupCtx, startupCancel := context.WithTimeout(ctx, 45*time.Second)
			defer startupCancel()
			for launched.Disposition != b.OperationConfirmed {
				if err := waitWorkspaceTerminal(startupCtx); err != nil {
					t.Fatal(err)
				}
				launchReq.Snapshot = s
				launched, _, err = l.Reconcile(startupCtx, launchReq)
				if err != nil {
					t.Fatal(err)
				}
				bind(&s, launched)
			}
			if _, err := l.ready(startupCtx, s); err != nil {
				t.Fatal("native main-agent readiness failed", err)
			}
			creates := 0
			for _, call := range *calls {
				if call[0] == "worktree" && call[1] == "create" {
					creates++
				}
			}
			if creates != 1 {
				t.Fatal("native workspace was recreated")
			}
			record["process"], record["passed"] = receipt.Process, true
			sends := 0
			for _, call := range *launchCalls {
				if call[0] == "terminal" && call[1] == "send" {
					sends++
				}
			}
			if sends != 1 {
				t.Fatal("agent startup was repeated", sends)
			}
			record["agent_start_commands"] = sends
			if recovery && receipt.Process.Incarnation != creation.Terminal.IncarnationID {
				t.Fatal(errors.New("recovery replaced the original PTY"))
			}
		})
	}
}
