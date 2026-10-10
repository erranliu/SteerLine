package orca_remote_worker

import (
	"context"
	"strings"
	"testing"
	"time"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func viewerFixture(t *testing.T, local bool, mutate func(string, map[string]any)) (Client, b.ExecutionRef) {
	t.Helper()
	c := Config{Agent: "claude", Settings: Settings{Target: "environment", ConnectionID: "connection", EnvironmentID: "environment", RepositorySelector: "id:repo"}}
	host, peer := "environment", "peer"
	if local {
		c.Settings = Settings{Target: "local"}
		host, peer = "local", "center"
	}
	i := b.ProcessIdentity{Kind: "orca_supervised", RuntimeID: peer, CoordinatorRuntimeID: "center", Terminal: "original", Incarnation: "inc", WorktreeID: "repo::/worker", ExecutionHost: host, OrchestrationRun: "run", Task: "task", Dispatch: "dispatch"}
	ref := b.ExecutionRef{Backend: b.BackendOrca, Protocol: b.ProtocolOrcaSupervised, Agent: b.AgentSpec{Name: c.Agent}, External: i, WorkspacePath: "/worker", WorkspaceBranch: "branch", Target: c.Target()}
	ref.Target.OrchestrationRun, ref.Target.ExternalTask, ref.Target.Dispatch = i.OrchestrationRun, i.Task, i.Dispatch
	ref.Target.ExecutionHost, ref.Target.WorktreeID = host, i.WorktreeID
	c.ExpectedRemoteRuntime = peer
	c.Call = func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		command := strings.Join(args[:2], " ")
		var result map[string]any
		runtime := "center"
		switch command {
		case "environment list":
			result = map[string]any{"environments": []any{map[string]any{"id": "environment", "runtimeId": peer, "pairedDeviceId": "connection"}}}
		case "orchestration worker-show":
			if flag(args, "--environment") != "" {
				t.Fatal("worker lookup routed away from coordinator")
			}
			result = map[string]any{"dispatch": map[string]any{"id": "dispatch", "runId": "run", "taskId": "task"}, "worker": map[string]any{"dispatchId": "dispatch", "worktreeId": i.WorktreeID, "agentTerminalHandle": i.Terminal, "runtimeEpoch": peer}, "terminal": map[string]any{"handle": i.Terminal, "incarnationId": i.Incarnation, "worktreeId": i.WorktreeID, "worktreePath": "/worker", "branch": "branch"}, "observation": map[string]any{"exactWorker": true}, "projection": map[string]any{"host": map[string]any{"id": host}, "liveness": map[string]any{"verdict": "live"}}}
			if !local {
				result["server"] = map[string]any{"environmentId": "environment"}
				result["remoteRuntimeEpoch"] = peer
			}
		case "terminal show", "terminal read":
			if local && flag(args, "--environment") != "" || !local && flag(args, "--environment") != "environment" {
				t.Fatalf("viewer used wrong host: %v", args)
			}
			runtime = peer
			result = map[string]any{"terminal": map[string]any{"handle": i.Terminal, "incarnationId": i.Incarnation, "worktreeId": i.WorktreeID, "worktreePath": "/worker", "branch": "branch", "agentIdentity": "claude", "connected": true, "orphaned": true, "source": "screen", "tail": []string{"original dialog"}}}
			if command == "terminal read" && !strings.Contains(strings.Join(args, " "), "--screen") {
				t.Fatal("viewer read stream as screen")
			}
		case "terminal switch":
			if flag(args, "--environment") != "" {
				t.Fatal("local focus redirected to remote host")
			}
			result = map[string]any{"focus": map[string]any{"handle": "original", "worktreeId": i.WorktreeID, "navigated": true}}
		default:
			t.Fatalf("viewer changed worker state: %v", args)
		}
		if mutate != nil {
			mutate(command, result)
		}
		return wire(runtime, result), nil
	}
	return New(c), ref
}

func TestOrcaViewerReadsOriginalLocalAndRemoteTerminal(t *testing.T) {
	for _, local := range []bool{false, true} {
		client, ref := viewerFixture(t, local, nil)
		frame, err := client.ReadViewerScreen(context.Background(), ref)
		if err != nil || !strings.Contains(frame, "original dialog") {
			t.Fatalf("original screen unavailable: %s / %v", frame, err)
		}
		if err := client.FocusViewerTerminal(context.Background(), "center", ref.External.Terminal, ref.External.WorktreeID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOrcaViewerRejectsChangedFactsAndUnknownScreen(t *testing.T) {
	for _, scenario := range []string{"pairing", "worker", "agent", "incarnation", "worktree", "disconnected", "read-handle", "old-host", "unknown-source", "after-read"} {
		t.Run(scenario, func(t *testing.T) {
			shows := 0
			client, ref := viewerFixture(t, false, func(command string, result map[string]any) {
				switch command {
				case "environment list":
					if scenario == "pairing" {
						result["environments"].([]any)[0].(map[string]any)["pairedDeviceId"] = "repaired"
					}
				case "orchestration worker-show":
					if scenario == "worker" {
						result["observation"].(map[string]any)["exactWorker"] = false
					}
				case "terminal show":
					shows++
					term := result["terminal"].(map[string]any)
					switch scenario {
					case "agent":
						term["agentIdentity"] = "codex"
					case "incarnation":
						term["incarnationId"] = "new"
					case "worktree":
						term["worktreeId"] = "other"
					case "disconnected":
						term["connected"] = false
					case "after-read":
						if shows > 1 {
							term["incarnationId"] = "new"
						}
					}
				case "terminal read":
					term := result["terminal"].(map[string]any)
					switch scenario {
					case "read-handle":
						term["handle"] = "other"
					case "old-host":
						delete(term, "source")
					case "unknown-source":
						term["source"] = "stream"
					}
				}
			})
			frame, err := client.ReadViewerScreen(context.Background(), ref)
			if err == nil || frame != "" {
				t.Fatalf("changed facts leaked a screen: %s / %v", frame, err)
			}
		})
	}
}

func TestOrcaViewerReportsScreenFallbackAndRequiresNavigationProof(t *testing.T) {
	client, ref := viewerFixture(t, false, func(command string, result map[string]any) {
		if command == "terminal read" {
			result["terminal"].(map[string]any)["source"] = "screen-unavailable"
		}
		if command == "terminal switch" {
			result["focus"].(map[string]any)["navigated"] = false
		}
	})
	frame, err := client.ReadViewerScreen(context.Background(), ref)
	if err != nil || !strings.Contains(frame, "屏幕暂不可用") {
		t.Fatalf("fallback hidden: %s / %v", frame, err)
	}
	if client.FocusViewerTerminal(context.Background(), "center", ref.External.Terminal, ref.External.WorktreeID) == nil {
		t.Fatal("false navigation reported as opened")
	}
}
