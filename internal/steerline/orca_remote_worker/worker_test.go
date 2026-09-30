package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func flag(args []string, key string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}
func wire(runtime string, result any) []byte {
	raw, _ := json.Marshal(map[string]any{"ok": true, "result": result, "_meta": map[string]string{"runtimeId": runtime}})
	return raw
}

func remoteTestConfig(t *testing.T, call func([]string) ([]byte, error)) Config {
	t.Helper()
	return Config{Settings: Settings{ConnectionID: "connection", EnvironmentID: "peer-env", RepositorySelector: "id:repo", CoordinatorTerminal: "coordinator"}, Agent: "codex", Model: "model", ReasoningEffort: "high", ExpectedRemoteRuntime: "peer", Origin: func(context.Context) ([]byte, error) { return []byte("https://forge/owner/repo.git"), nil }, Call: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("1.4.210"), nil
		}
		switch args[0] {
		case "status":
			runtime := "center"
			if flag(args, "--environment") != "" {
				runtime = "peer"
			}
			return wire(runtime, map[string]any{"runtime": map[string]any{"state": "ready", "reachable": true, "appVersion": "1.4.210", "runtimeId": runtime, "capabilities": []string{"orchestration.federation.v1", "orchestration.federation-control-mail.v1", "orchestration.federation-lifecycle-settlement.v1", "orchestration.worker-launch-preferences.v1", "orchestration.federation-structured-read.v1", "orchestration.federation-fleet-snapshot.v1", "orchestration.federation-release-archive.v1", "agent.launch.v2"}}}), nil
		case "environment":
			return wire("center", map[string]any{"environments": []any{map[string]string{"id": "peer-env", "runtimeId": "peer", "pairedDeviceId": "connection"}}}), nil
		case "repo":
			return wire("peer", map[string]any{"repo": map[string]any{"id": "repo", "kind": "git", "gitRemoteIdentity": map[string]string{"remoteUrl": "https://forge/owner/repo.git"}}}), nil
		case "terminal":
			return wire("center", map[string]any{"terminal": map[string]any{"handle": "coordinator", "connected": true}}), nil
		}
		return call(args)
	}}
}

func TestRemoteContinuationRequiresOriginalPeerAndTerminal(t *testing.T) {
	starts := 0
	previous := executionapi.ProcessIdentity{Kind: "orca_supervised", RuntimeID: "peer", CoordinatorRuntimeID: "center", Terminal: "agent", Incarnation: "inc", OrchestrationRun: "run", Task: "old", Dispatch: "old-dispatch", ExecutionHost: "peer-env", WorktreeID: "repo::/srv/worker"}
	config := remoteTestConfig(t, func(args []string) ([]byte, error) {
		if flag(args, "--environment") != "" {
			t.Fatal("center lifecycle routed to peer")
		}
		if args[1] == "worker-show" {
			return wire("center", map[string]any{"dispatch": map[string]string{"id": "old-dispatch", "runId": "run", "taskId": "old", "status": "completed"}, "worker": map[string]string{"dispatchId": "old-dispatch", "worktreeId": "repo::/srv/worker", "agentTerminalHandle": "agent", "state": "succeeded"}, "server": map[string]string{"environmentId": "peer-env"}, "projection": map[string]any{"host": map[string]string{"id": "peer-env"}}, "remoteRuntimeEpoch": "peer"}), nil
		}
		if args[1] != "worker-start" {
			t.Fatalf("unexpected platform call %v", args)
		}
		starts++
		if flag(args, "--terminal") != "agent" || flag(args, "--on") != "peer-env" || flag(args, "--task") != "next" || flag(args, "--model") != "" || flag(args, "--agent") != "" {
			t.Fatalf("continuation replaced its fixed agent: %v", args)
		}
		return wire("center", map[string]any{"runId": "run", "taskId": "next", "dispatchId": "next-dispatch", "state": "ready", "stage": "input_accepted", "server": map[string]string{"environmentId": "peer-env"}, "worker": map[string]string{"worktreeId": "repo::/srv/worker"}, "mutation": map[string]string{"requestId": "request"}}), nil
	})
	client := New(config)
	if _, err := client.StartTask(context.Background(), "run", "next", previous.WorktreeID, &previous); err != nil {
		t.Fatal(err)
	}
	previous.RuntimeID = "replaced-peer"
	if _, err := client.StartTask(context.Background(), "run", "next", previous.WorktreeID, &previous); err == nil {
		t.Fatal("changed peer was accepted")
	}
	if starts != 1 {
		t.Fatal("identity mismatch launched another worker")
	}
}

func TestRemoteWorkspaceKeepsPeerPathAndRuntime(t *testing.T) {
	config := remoteTestConfig(t, func(args []string) ([]byte, error) {
		if args[0] != "worktree" || flag(args, "--environment") != "peer-env" {
			t.Fatalf("workspace not routed to peer: %v", args)
		}
		return wire("peer", map[string]any{"worktree": map[string]string{"id": "repo::/srv/worker", "repoId": "repo", "path": "/srv/worker", "branch": "worker", "displayName": "worker", "baseRef": strings.Repeat("a", 40), "head": strings.Repeat("a", 40)}}), nil
	})
	client := New(config)
	pair := PreflightResult{RepositoryID: "repo", EnvironmentID: "peer-env", RemoteRuntime: "peer"}
	value, err := client.CreateWorktree(context.Background(), pair, "worker", strings.Repeat("a", 40))
	if err != nil || value.Path != "/srv/worker" {
		t.Fatalf("peer path translated using center OS: %+v %v", value, err)
	}
	pair.RemoteRuntime = "replaced-peer"
	if _, err := client.CreateWorktree(context.Background(), pair, "worker", strings.Repeat("a", 40)); err == nil {
		t.Fatal("workspace from replacement peer accepted")
	}
}

func TestRemoteReplyRejectsOversizeWithoutMutation(t *testing.T) {
	calls := 0
	config := remoteTestConfig(t, func([]string) ([]byte, error) { calls++; return nil, errors.New("unexpected mutation") })
	identity := executionapi.ProcessIdentity{Kind: "orca_supervised", RuntimeID: "peer", CoordinatorRuntimeID: "center", OrchestrationRun: "run", Task: "task", Dispatch: "dispatch", ExecutionHost: "peer-env", WorktreeID: "repo::/srv/worker"}
	ref := Ref{Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaSupervised, Target: executionapi.ExecutionTarget{ID: "orca-supervised:connection", ConnectionID: "connection", ExternalRuntime: "peer-env", OrchestrationRun: "run", ExternalTask: "task", Dispatch: "dispatch", ExecutionHost: "peer-env", WorktreeID: identity.WorktreeID}, External: identity}
	if _, err := New(config).Reply(context.Background(), ref, "question", strings.Repeat("\n", 4000)); err == nil {
		t.Fatal("oversize reply accepted")
	}
	if calls != 0 {
		t.Fatal("oversize reply reached platform")
	}
}
