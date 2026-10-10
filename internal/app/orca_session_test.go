package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	orca "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestOrcaSessionViewerBindsOriginalHomeBeforeLookup(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	home := t.TempDir()
	request := orcaViewerRequest{Project: "project", Home: home, ConfigPath: filepath.Join(home, "missing-project.json")}
	raw, _ := json.Marshal(request)
	path := filepath.Join(home, "request.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if RunOrcaSessionViewer(context.Background(), path, &bytes.Buffer{}) == nil || HomeDir() != home {
		t.Fatal("viewer looked up a foreign home")
	}
}

func orcaSessionRecords(t *testing.T) (p.ExecutionSnapshot, api.NativeSessionTarget) {
	t.Helper()
	i := b.ProcessIdentity{Kind: "orca_supervised", RuntimeID: "peer", CoordinatorRuntimeID: "center", Terminal: "original", Incarnation: "inc", ExecutionHost: "environment", WorktreeID: "repo::/worker", OrchestrationRun: "run", Task: "task", Dispatch: "dispatch"}
	target := api.NativeSessionTarget{RunID: "run", ExecutionID: "execution", AttemptID: "attempt", SessionID: remote.SessionID(i), SessionGeneration: i.Incarnation}
	raw, err := remote.EncodeProfileConfig(remote.ProfileConfig{Target: "environment", Executable: "original-orca", Shell: "powershell.exe", WaitTimeout: "1m", ConnectionID: "connection", EnvironmentID: "environment", RepositorySelector: "id:repo", PollInterval: "1s", CapabilityReportVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := b.NewImplementationDataEnvelope(b.ImplementationDataEnvelope{CodecVersion: remote.ProfileCodecVersion, Purpose: b.ImplementationConfigPurpose, ImplementationKey: remote.ProfileCodecKey, ProjectID: "project", ProfileID: "frozen", AcceptedConfigVersion: "version"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	records := p.ExecutionSnapshot{ProjectIdentity: "project", WorkflowRuns: []m.WorkflowRunRecord{{ProjectIdentity: "project", WorkflowRunID: "run", ExecutionProfile: &m.FixedExecutionProfile{ID: "frozen", AcceptedConfigVersion: "version", Host: "orca_supervised", Agent: "claude", ImplementationData: []b.ImplementationDataEnvelope{envelope}}}}, Executions: []m.ExecutionRecord{{ProjectIdentity: "project", WorkflowRunID: "run", ExecutionID: "execution", CurrentAttemptID: "attempt", SessionID: target.SessionID, SessionGeneration: target.SessionGeneration, CurrentTurnID: "turn", Backend: string(b.BackendOrca), Protocol: string(b.ProtocolOrcaSupervised), Agent: "claude", Process: &i, Workspace: m.WorkspaceEvidence{Worktree: "/worker", Branch: "branch"}}}}
	return records, target
}

func TestOrcaSessionResolvesFrozenProfileAndRejectsStaleCandidate(t *testing.T) {
	records, target := orcaSessionRecords(t)
	spec, err := resolveOrcaSession(records, "project", target)
	if err != nil || spec.profile.Executable != "original-orca" || spec.ref.External.Terminal != "original" || spec.ref.Agent.Name != "claude" {
		t.Fatalf("lost frozen candidate: %+v / %v", spec, err)
	}
	for _, scenario := range []string{"project", "attempt", "session", "generation", "process", "profile", "empty-turn"} {
		t.Run(scenario, func(t *testing.T) {
			copy, stale := records, target
			copy.Executions = append([]m.ExecutionRecord(nil), records.Executions...)
			project := "project"
			switch scenario {
			case "project":
				project = "other"
			case "attempt":
				stale.AttemptID = "other"
			case "session":
				stale.SessionID = "other"
			case "generation":
				stale.SessionGeneration = "other"
			case "process":
				i := *copy.Executions[0].Process
				i.Incarnation = "new"
				copy.Executions[0].Process = &i
			case "profile":
				copy.WorkflowRuns = nil
			case "empty-turn":
				copy.Executions[0].CurrentTurnID = ""
			}
			if _, err := resolveOrcaSession(copy, project, stale); err == nil {
				t.Fatal("stale candidate opened")
			}
		})
	}
}

func TestOrcaSessionDisplayQuotesLiteralPaths(t *testing.T) {
	cmd := orcaDisplayCommand("C:/agent's dir/steerline.exe", "C:/a'b/request.json", "windows")
	if cmd != "& 'C:/agent''s dir/steerline.exe' orca-session-view -config 'C:/a''b/request.json'" {
		t.Fatal(cmd)
	}
	cmd = orcaDisplayCommand("/a'b/steerline", "/tmp/request.json", "linux")
	if !strings.HasPrefix(cmd, "'/a'\\''b/steerline'") || strings.Contains(cmd, "resume") {
		t.Fatal(cmd)
	}
}

func TestOrcaSessionReusesDisplayAndRejectsReplacedIncarnation(t *testing.T) {
	records, target := orcaSessionRecords(t)
	spec, _ := resolveOrcaSession(records, "project", target)
	path := filepath.Join(t.TempDir(), "request.json")
	saved := orcaViewerBinding{Runtime: "center", Terminal: orca.Terminal{Handle: "display", IncarnationID: "view-inc", WorktreeID: "repo::/center"}}
	raw, _ := json.Marshal(saved)
	if err := os.WriteFile(path+".terminal.json", raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []bool{false, true} {
		focus := 0
		client := remote.New(remote.Config{Call: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
			var result any
			switch args[1] {
			case "show":
				term := saved.Terminal
				term.Connected = true
				if changed {
					term.IncarnationID = "new"
				}
				result = map[string]any{"terminal": term}
			case "switch":
				focus++
				result = map[string]any{"focus": map[string]any{"handle": "display", "worktreeId": saved.Terminal.WorktreeID, "navigated": true}}
			default:
				t.Fatalf("reopen created another terminal: %v", args)
			}
			raw, _ := json.Marshal(map[string]any{"ok": true, "result": result, "_meta": map[string]string{"runtimeId": "center"}})
			return raw, nil
		}})
		err := openOrcaRemoteDisplay(context.Background(), Config{}, spec, client, path)
		if changed && (err == nil || focus != 0) || !changed && (err != nil || focus != 1) {
			t.Fatalf("wrong reuse decision: %v / %d", err, focus)
		}
	}
}

func TestOrcaSessionReopensClosedOrStoppedDisplayWithoutAgentLaunch(t *testing.T) {
	for _, closed := range []bool{false, true} {
		records, target := orcaSessionRecords(t)
		spec, _ := resolveOrcaSession(records, "project", target)
		dir := t.TempDir()
		path := filepath.Join(dir, "request.json")
		saved := orcaViewerBinding{Runtime: "center", Terminal: orca.Terminal{Handle: "old-display", IncarnationID: "old-inc", WorktreeID: "center-worktree"}}
		raw, _ := json.Marshal(saved)
		if err := os.WriteFile(path+".terminal.json", raw, 0600); err != nil {
			t.Fatal(err)
		}
		if !closed {
			if err := os.WriteFile(path+".stopped", []byte("stopped"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		creates, focus := 0, 0
		client := remote.New(remote.Config{Call: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
			var result any
			switch args[1] {
			case "show":
				if slices.Contains(args, "old-display") {
					if closed {
						return []byte(`{"ok":false,"error":{"message":"closed"},"_meta":{"runtimeId":"center"}}`), nil
					}
					term := saved.Terminal
					term.Connected = true
					result = map[string]any{"terminal": term}
				} else {
					result = map[string]any{"terminal": orca.Terminal{Handle: "new-display", IncarnationID: "new-inc", Connected: true, WorktreeID: "center-worktree", WorktreePath: dir}}
				}
			case "list":
				result = map[string]any{"terminals": []any{}, "truncated": false}
			case "create":
				creates++
				all := strings.Join(args, " ")
				if strings.Contains(all, "--agent") || strings.Contains(all, "resume") || strings.Contains(all, "--focus") || !strings.Contains(all, "orca-session-view -config") {
					t.Fatal("viewer launched agent or coupled focus with creation: ", args)
				}
				result = map[string]any{"terminal": map[string]any{"handle": "new-display"}}
			case "switch":
				focus++
				result = map[string]any{"focus": map[string]any{"handle": "new-display", "worktreeId": "center-worktree", "navigated": true}}
			default:
				t.Fatal("viewer changed native agent: ", args)
			}
			raw, _ := json.Marshal(map[string]any{"ok": true, "result": result, "_meta": map[string]string{"runtimeId": "center"}})
			return raw, nil
		}})
		if err := openOrcaRemoteDisplay(context.Background(), Config{Repository: dir, ProcessHost: NewProcessHost(time.Second)}, spec, client, path); err != nil || creates != 1 || focus != 1 {
			t.Fatalf("failed to reopen closed/stopped display: %v / %d / %d", err, creates, focus)
		}
	}
}
