package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func fakeSupervisedEnvelope(runtime string, result any) int {
	data, err := json.Marshal(map[string]any{"ok": true, "result": result, "_meta": map[string]any{"runtimeId": runtime}})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println(string(data))
	return 0
}

func fakeSupervisedOrca(args []string) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("1.4.209")
		return 0
	}
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "status":
		remote := arg(args, "--environment") != ""
		version, runtimeID := "1.4.209", "fake-center-runtime"
		if remote {
			version, runtimeID = "1.4.210", "fake-remote-runtime"
			if override := os.Getenv("STEERLINE_FAKE_SUPERVISED_REMOTE_VERSION"); override != "" {
				version = override
			}
		}
		caps := []string{"orchestration.federation.v1", "orchestration.federation-control-mail.v1", "orchestration.federation-lifecycle-settlement.v1", "orchestration.worker-launch-preferences.v1", "orchestration.federation-structured-read.v1", "orchestration.federation-fleet-snapshot.v1", "orchestration.federation-release-archive.v1", "agent.launch.v2"}
		if remote && os.Getenv("STEERLINE_FAKE_SUPERVISED_MISSING_CAPABILITY") != "" {
			caps = append(caps[:0:0], caps[:len(caps)-1]...)
		}
		return fakeSupervisedEnvelope(runtimeID, map[string]any{"runtime": map[string]any{"state": "ready", "reachable": true, "runtimeId": runtimeID, "appVersion": version, "capabilities": caps}})
	case "worktree":
		if os.Getenv("STEERLINE_FAKE_REOPEN") == "1" && arg(args, "--environment") == "env-home" && arg(args, "--worktree") == "id:repo-test::worktree" {
			b, _ := json.Marshal(map[string]any{"ok": false, "error": map[string]any{"code": "selector_not_found", "message": "selector_not_found"}, "_meta": map[string]any{"runtimeId": "fake-remote-runtime"}})
			fmt.Println(string(b))
			return 1
		}
		if len(args) < 2 || args[1] != "show" {
			return fakeOrcaErr("invalid_argument", "unexpected worktree verb")
		}
		worktreeID := strings.TrimPrefix(arg(args, "--worktree"), "id:")
		parts := strings.SplitN(worktreeID, "::", 2)
		if len(parts) != 2 || parts[0] != "repo-test" {
			return fakeOrcaErr("worktree_not_found", "unknown worktree")
		}
		head := os.Getenv("STEERLINE_FAKE_SUPERVISED_HEAD")
		if head == "" {
			head = strings.Repeat("a", 40)
		}
		name := filepath.Base(parts[1])
		return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"worktree": map[string]any{"id": worktreeID, "repoId": "repo-test", "path": parts[1], "head": head, "branch": "refs/heads/erranliu/" + name}})
	case "environment":
		pairedID := os.Getenv("STEERLINE_FAKE_SUPERVISED_PAIRED_DEVICE_ID")
		if pairedID == "" {
			pairedID = "home-connection"
		}
		return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"environments": []any{map[string]any{"id": "env-home", "runtimeId": "fake-remote-runtime", "pairedDeviceId": pairedID}}})
	case "repo":
		if len(args) < 2 || args[1] != "show" || arg(args, "--repo") != "id:repo-test" || arg(args, "--environment") != "env-home" {
			return fakeOrcaErr("repo_not_found", "unexpected repository selector")
		}
		return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"repo": map[string]any{"id": "repo-test", "kind": "git", "gitRemoteIdentity": map[string]any{"remoteUrl": os.Getenv("STEERLINE_FAKE_SUPERVISED_REMOTE_URL")}}})
	case "terminal":
		if len(args) < 2 {
			return fakeOrcaErr("invalid_argument", "missing terminal verb")
		}
		if args[1] == "show" && os.Getenv("STEERLINE_FAKE_ACCEPTANCE_TERMINAL") != "" {
			if arg(args, "--environment") != "env-home" || arg(args, "--terminal") != "terminal-acceptance" {
				return fakeOrcaErr("terminal_handle_stale", "wrong terminal identity")
			}
			return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"terminal": map[string]any{"handle": "terminal-acceptance", "incarnationId": os.Getenv("STEERLINE_FAKE_ACCEPTANCE_TERMINAL"), "worktreeId": "repo-test::worktree", "connected": false, "exitCause": map[string]any{"kind": "operator_close"}}})
		}
		if args[1] == "show" && os.Getenv("STEERLINE_FAKE_REOPEN") == "1" && arg(args, "--environment") == "env-home" && arg(args, "--terminal") == "terminal-reopen" {
			return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"terminal": map[string]any{"handle": "terminal-reopen", "incarnationId": "inc-reopen", "worktreeId": "repo-test::worktree", "connected": os.Getenv("STEERLINE_FAKE_REOPEN_TERMINAL_LIVE") == "1", "exitCause": map[string]any{"kind": "operator_close"}}})
		}
		if args[1] == "create" {
			workflow := strings.TrimPrefix(arg(args, "--title"), "SteerLine coordinator ")
			if !strings.HasPrefix(arg(args, "--title"), "SteerLine coordinator ") || !stableExecutionID(workflow) || arg(args, "--worktree") == "" {
				return fakeOrcaErr("invalid_argument", "per-Run coordinator identity missing")
			}
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_COORDINATOR_CREATE_LOSS") == "1" {
				return fakeOrcaErr("transport_unavailable", "coordinator create receipt lost")
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"terminal": map[string]any{"handle": "term-" + workflow, "connected": true}})
		}
		if args[1] == "wait" {
			if arg(args, "--terminal") != "terminal-test" || arg(args, "--for") != "tui-idle" || arg(args, "--environment") != "env-home" {
				return fakeOrcaErr("invalid_argument", "prompt readiness wait did not target the exact remote terminal")
			}
			idle := os.Getenv("STEERLINE_FAKE_SUPERVISED_TUI_IDLE") != "0"
			return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"wait": map[string]any{"handle": "terminal-test", "condition": "tui-idle", "satisfied": idle, "status": "running"}})
		}
		if args[1] == "send" {
			if arg(args, "--terminal") != "terminal-test" || arg(args, "--environment") != "env-home" || !strings.Contains(strings.Join(args, " "), "--enter") {
				return fakeOrcaErr("invalid_argument", "prompt Enter did not target the exact remote terminal")
			}
			if marker := os.Getenv("STEERLINE_FAKE_SUPERVISED_ENTER_MARKER"); marker != "" {
				file, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					return fakeOrcaErr("internal", "prompt Enter marker could not be written")
				}
				_, err = file.WriteString("enter\n")
				_ = file.Close()
				if err != nil {
					return fakeOrcaErr("internal", "prompt Enter marker could not be written")
				}
			}
			return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"send": map[string]any{"handle": "terminal-test", "accepted": true}})
		}
		if args[1] != "show" || !strings.HasPrefix(arg(args, "--terminal"), "term-") {
			return fakeOrcaErr("terminal_handle_stale", "coordinator terminal missing")
		}
		return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"terminal": map[string]any{"handle": arg(args, "--terminal"), "connected": true, "orphaned": false}})
	case "orchestration":
		if len(args) < 2 {
			return fakeOrcaErr("invalid_argument", "missing orchestration verb")
		}
		switch args[1] {
		case "run-create":
			if arg(args, "--from") == "term-coordinator" || !strings.HasPrefix(arg(args, "--from"), "term-") || arg(args, "--retry-request") != "" || arg(args, "--objective") == "" {
				return fakeOrcaErr("invalid_argument", "missing Run identity")
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"run": map[string]any{"id": "orca-run-test", "objective": arg(args, "--objective")}, "mutation": map[string]any{"requestId": "request-run-create"}})
		case "run-show":
			coordinator := os.Getenv("STEERLINE_FAKE_SUPERVISED_RUN_COORDINATOR")
			if coordinator == "" {
				coordinator = "term-coordinator"
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"run": map[string]any{"id": arg(args, "--id"), "coordinator_handle": coordinator}})
		case "worker-start":
			if !strings.HasPrefix(arg(args, "--from"), "term-") || arg(args, "--on") != "env-home" || arg(args, "--agent") != "codex" || arg(args, "--model") != "gpt-6-luna" || arg(args, "--effort") != "high" || arg(args, "--retry-request") != "" {
				return fakeOrcaErr("invalid_argument", "launch identity/model was not fixed")
			}
			if marker := os.Getenv("STEERLINE_FAKE_SUPERVISED_START_MARKER"); marker != "" {
				file, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					return fakeOrcaErr("internal", "worker-start marker could not be written")
				}
				_, err = file.WriteString("attempt\n")
				_ = file.Close()
				if err != nil {
					return fakeOrcaErr("internal", "worker-start marker could not be written")
				}
			}
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_DROP_START_RESPONSE") == "1" {
				fmt.Fprintln(os.Stderr, "injected worker-start response loss after application")
				return 75
			}
			worktree := os.Getenv("STEERLINE_FAKE_SUPERVISED_WORKTREE_ID")
			if worktree == "" {
				worktree = "repo-test::C:/remote/steerline-run"
			}
			if base := arg(args, "--base-branch"); base != "" {
				if pinned := os.Getenv("STEERLINE_FAKE_SUPERVISED_HEAD"); pinned != "" && base != pinned {
					return fakeOrcaErr("base_mismatch", "fixed base did not match")
				}
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"runId": arg(args, "--run"), "taskId": "task-test", "dispatchId": "dispatch-test", "state": "ready", "stage": "input_accepted", "server": map[string]any{"environmentId": "env-home"}, "launch": map[string]any{"effective": map[string]any{"agent": "codex", "model": "gpt-6-luna", "effort": "high"}}, "worker": map[string]any{"worktreeId": worktree, "effects": []any{map[string]any{"kind": "worktree", "action": "created_top_level", "id": worktree}}}, "mutation": map[string]any{"requestId": "request-worker-start"}})
		case "worker-show":
			if os.Getenv("STEERLINE_FAKE_REOPEN") == "1" {
				return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatch": map[string]any{"id": "dispatch-reopen", "runId": "run-reopen", "taskId": "task-reopen", "status": "completed"}, "worker": map[string]any{"dispatchId": "dispatch-reopen", "state": "succeeded", "worktreeId": "repo-test::worktree", "agentTerminalHandle": "terminal-reopen", "residualResources": []any{}}, "server": map[string]any{"environmentId": "env-home"}, "remoteRuntimeEpoch": "fake-remote-runtime", "observation": map[string]any{"status": "exited", "exactWorker": true}})
			}
			if os.Getenv("STEERLINE_FAKE_ACCEPTANCE_DISPATCH") != "" {
				return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatch": map[string]any{"id": "dispatch-acceptance", "runId": "run-acceptance", "taskId": "task-acceptance", "status": "failed"}, "worker": map[string]any{"dispatchId": "dispatch-acceptance", "state": os.Getenv("STEERLINE_FAKE_ACCEPTANCE_DISPATCH"), "worktreeId": "repo-test::worktree", "agentTerminalHandle": "terminal-acceptance", "residualResources": []any{}}, "server": map[string]any{"environmentId": "env-home"}})
			}
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_OBSERVE_UNAVAILABLE") != "" {
				return fakeOrcaErr("transport_unavailable", "execution host is temporarily unreachable")
			}
			worktree := os.Getenv("STEERLINE_FAKE_SUPERVISED_WORKTREE_ID")
			if worktree == "" {
				worktree = "repo-test::C:/remote/steerline-run"
			}
			dispatchState := os.Getenv("STEERLINE_FAKE_SUPERVISED_DISPATCH_STATE")
			if dispatchState == "" {
				dispatchState = "dispatched"
			}
			worktreePath := strings.SplitN(worktree, "::", 2)[1]
			name := filepath.Base(worktreePath)
			runtimeEpoch := os.Getenv("STEERLINE_FAKE_SUPERVISED_WORKER_EPOCH")
			if runtimeEpoch == "" {
				runtimeEpoch = "fake-remote-runtime"
			}
			observation := map[string]any{"exactWorker": true}
			worker := map[string]any{"dispatchId": "dispatch-test", "worktreeId": worktree, "agentTerminalHandle": "terminal-test"}
			if dispatchState == "completed" {
				worker["state"], worker["stage"] = "succeeded", "settled"
			} else if dispatchState == "failed" {
				worker["state"], worker["stage"] = "failed", "settled"
			}
			switch os.Getenv("STEERLINE_FAKE_SUPERVISED_AGENT_WAIT") {
			case "present":
				observation["agentWait"] = map[string]any{"kind": "host_permission"}
			case "none":
				observation["agentWait"] = nil
			}
			var terminal any = map[string]any{"handle": "terminal-test", "incarnationId": "inc-test", "worktreeId": worktree, "worktreePath": worktreePath, "branch": "refs/heads/erranliu/" + name}
			liveness := "live"
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_SETTLED_NO_TERMINAL") == "1" && (dispatchState == "completed" || dispatchState == "failed") {
				terminal = nil
				liveness = "unverifiable"
				observation = map[string]any{"status": "identity_changed", "exactWorker": false}
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatch": map[string]any{"id": "dispatch-test", "runId": "orca-run-test", "taskId": "task-test", "status": dispatchState}, "worker": worker, "projection": map[string]any{"liveness": map[string]any{"verdict": liveness}, "host": map[string]any{"id": "env-home"}}, "server": map[string]any{"environmentId": "env-home"}, "remoteRuntimeEpoch": runtimeEpoch, "terminal": terminal, "observation": observation})
		case "worker-read":
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_DENY_WORKER_READ") == "1" {
				return fakeOrcaErr("identity_changed", "settled worker has no live terminal output")
			}
			state := os.Getenv("STEERLINE_FAKE_SUPERVISED_TUI_STATE")
			if state != "" || os.Getenv("STEERLINE_FAKE_SUPERVISED_ENTER_MARKER") != "" {
				tail := []string{"› [Pasted Content 123 chars]"}
				if state == "active" {
					tail = []string{"• Working (1s · esc to interrupt)"}
				} else if state == "not-ready" {
					tail = []string{"› Ask Codex to do anything"}
				} else if marker := os.Getenv("STEERLINE_FAKE_SUPERVISED_ENTER_MARKER"); marker != "" {
					if _, err := os.Stat(marker); err == nil {
						tail = []string{"• Working (1s · esc to interrupt)"}
					}
				}
				return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatchId": arg(args, "--dispatch"), "source": "terminal", "cursor": "cursor-1", "nextCursor": "cursor-1", "contentComplete": true, "clipping": []string{}, "terminal": map[string]any{"status": "running", "tail": tail}, "status": map[string]any{"worker": "ready", "terminal": "running", "liveness": "live"}})
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatchId": arg(args, "--dispatch"), "source": "transcript", "cursor": "cursor-1", "contentComplete": true, "clipping": []string{}, "status": map[string]any{"worker": "ready", "terminal": "running", "liveness": "live"}, "transcript": map[string]any{"messages": []any{}}})
		case "check":
			coordinator := os.Getenv("STEERLINE_FAKE_SUPERVISED_RUN_COORDINATOR")
			if coordinator == "" {
				coordinator = "term-coordinator"
			}
			if arg(args, "--terminal") != coordinator {
				return fakeOrcaErr("consumer_fenced", "coordinator terminal is bound to another Run")
			}
			if payload := os.Getenv("STEERLINE_FAKE_SUPERVISED_SETTLED_STATUS"); payload != "" {
				if ack := arg(args, "--ack"); ack != "" {
					return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"runId": arg(args, "--run"), "acknowledged": ack, "mutation": map[string]any{"requestId": "request-settled-ack"}})
				}
				return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"runId": arg(args, "--run"), "deliveryId": "delivery-settled", "messages": []any{map[string]any{"id": "status-settled", "type": "status", "from_handle": "dispatch:dispatch-test", "payload": payload}, map[string]any{"id": "done-settled", "type": "worker_done", "from_handle": "dispatch:dispatch-test", "payload": `{"taskId":"task-test","dispatchId":"dispatch-test","outcome":"succeeded"}`}}, "count": 2})
			}
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_DONE_WITHOUT_RESULT") == "1" {
				return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"runId": "orca-run-test", "deliveryId": "delivery-done-without-result", "messages": []any{map[string]any{"id": "done-without-result", "type": "worker_done", "from_handle": "dispatch:dispatch-test", "to_handle": "run:orca-run-test", "body": "worker reported success without the structured result", "payload": map[string]any{"taskId": "task-test", "dispatchId": "dispatch-test", "outcome": "succeeded"}}}, "count": 1})
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"runId": arg(args, "--run"), "deliveryId": nil, "messages": []any{}, "count": 0, "timedOut": true, "connectionLost": false, "cancelled": false})
		case "worker-release":
			if marker := os.Getenv("STEERLINE_FAKE_SUPERVISED_RELEASE_MARKER"); marker != "" {
				file, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					return fakeOrcaErr("internal", "worker-release marker could not be written")
				}
				_, err = file.WriteString("attempt\n")
				_ = file.Close()
				if err != nil {
					return fakeOrcaErr("internal", "worker-release marker could not be written")
				}
			}
			state := os.Getenv("STEERLINE_FAKE_SUPERVISED_RELEASE_STATE")
			if state == "" {
				state = "released"
			}
			reason := os.Getenv("STEERLINE_FAKE_SUPERVISED_RELEASE_REASON")
			residual := []string{}
			if state != "released" && state != "already_released" {
				residual = append(residual, "dispatch:"+arg(args, "--dispatch"))
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatchId": arg(args, "--dispatch"), "state": state, "reason": reason, "processAction": "none", "residualResources": residual, "mutation": map[string]any{"requestId": "request-worker-release"}})
		case "reply":
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_DENY_REPLY") != "" {
				return fakeOrcaErr("unexpected_mutation", "reply was resent during read-only reconciliation")
			}
			coordinator := os.Getenv("STEERLINE_FAKE_SUPERVISED_RUN_COORDINATOR")
			if coordinator == "" {
				coordinator = "term-coordinator"
			}
			if arg(args, "--run") != "orca-run-test" || arg(args, "--from") != coordinator || arg(args, "--id") == "" || arg(args, "--body") == "" {
				return fakeOrcaErr("invalid_argument", "reply lost its original question or Run")
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"message": map[string]any{"from_handle": "run:orca-run-test", "to_handle": "dispatch:dispatch-test"}, "question": map[string]any{"message_id": arg(args, "--id"), "run_id": "orca-run-test", "dispatch_id": "dispatch-test", "answer_body": arg(args, "--body"), "status": "answered"}, "mutation": map[string]any{"requestId": "request-reply"}})
		case "inbox":
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"messages": []any{map[string]any{"id": "answer-message-test", "from_handle": "run:orca-run-test", "to_handle": "dispatch:dispatch-test", "thread_id": "relay-answer-test", "body": "left", "type": "status"}}})
		default:
			return fakeOrcaErr("unknown", "unknown supervised orchestration verb")
		}
	default:
		return fakeOrcaErr("unknown", "unknown supervised Orca test command")
	}
}

func supervisedFixtureConfig(t *testing.T) Config {
	t.Helper()
	f := newFixture(t)
	return supervisedConfigForFixture(t, f)
}

func supervisedConfigForFixture(t *testing.T, f *fixture) Config {
	t.Helper()
	remoteBytes, err := gitRunner(f.c, f.c.Repository).Output(context.Background(), f.c.Git, "remote", "get-url", f.c.Remote)
	if err != nil {
		t.Fatal(err)
	}
	remoteURL := strings.TrimSpace(string(remoteBytes))
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_URL", remoteURL)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := f.c
	c.Host = "orca_supervised"
	c.Agent = "codex"
	c.Model = "gpt-6-luna"
	c.ReasoningEffort = "high"
	c.Orca = OrcaConfig{Executable: exe, Args: []string{"--fake", "orca-supervised"}, Shell: "powershell", WaitTimeout: "5s", Supervised: &SupervisedOrcaConfig{ConnectionID: "home-connection", EnvironmentID: "env-home", RepositorySelector: "id:repo-test", CoordinatorTerminal: "term-coordinator", ExpectedCodexVersion: "codex-cli 0.156.1", PollInterval: "2s", CapabilityReportVersion: 1}}
	return c
}

func TestSupervisedOrcaPreflightPinsRemoteIdentityAndVersionFamily(t *testing.T) {
	c := supervisedFixtureConfig(t)
	pair, err := probeSupervisedOrcaTarget(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if pair.CenterVersion != "1.4.209" || pair.RemoteVersion != "1.4.210" || pair.EnvironmentID != "env-home" || pair.RepositoryID != "repo-test" || pair.Coordinator != "term-coordinator" {
		t.Fatalf("preflight identity = %+v", pair)
	}
	if got := targetFor(c); got.ID != "orca-supervised:home-connection" || got.ExternalRuntime != "env-home" {
		t.Fatalf("target = %+v", got)
	}
}

func TestSupervisedOrcaPreflightRejectsVersionOrCapabilityDrift(t *testing.T) {
	c := supervisedFixtureConfig(t)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_VERSION", "1.5.0")
	if _, err := probeSupervisedOrcaTarget(context.Background(), c); err == nil || !strings.Contains(err.Error(), "version-family mismatch") {
		t.Fatalf("version mismatch error = %v", err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_VERSION", "1.4.210")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_MISSING_CAPABILITY", "1")
	if _, err := probeSupervisedOrcaTarget(context.Background(), c); err == nil || !strings.Contains(err.Error(), "agent.launch.v2") {
		t.Fatalf("missing capability error = %v", err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_MISSING_CAPABILITY", "")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_PAIRED_DEVICE_ID", "replacement-device")
	if _, err := probeSupervisedOrcaTarget(context.Background(), c); err == nil || !strings.Contains(err.Error(), "paired-device") {
		t.Fatalf("connection replacement error = %v", err)
	}
}

func TestSupervisedOrcaRequiresPinnedModelAndKeepsStopUnsupported(t *testing.T) {
	c := supervisedFixtureConfig(t)
	c.Model = ""
	if err := validateSupervisedOrcaConfig(&c); err == nil {
		t.Fatal("missing model unexpectedly accepted")
	}
	c.Model = "gpt-6-luna"
	c.Orca.Supervised.RemoteCancelAvailable = true
	if err := validateSupervisedOrcaConfig(&c); err == nil {
		t.Fatal("remote cancellation was enabled")
	}
	c.Orca.Supervised.RemoteCancelAvailable = false
	c.Orca.Supervised.ExpectedCodexVersion = ""
	if err := validateSupervisedOrcaConfig(&c); err != nil {
		t.Fatalf("supervised config unexpectedly requires an exact Codex CLI pin: %v", err)
	}
	report, err := (orcaSupervisedBackend{config: c}).Probe(context.Background(), targetFor(c), agentFor(c))
	if err != nil {
		t.Fatal(err)
	}
	if capabilityStatus(report, "stop") != executionapi.CapabilityUnsupported || capabilityStatus(report, "start") != executionapi.CapabilityNotTested {
		t.Fatalf("supervised capabilities = %+v", report.Capabilities)
	}
	records := executionRecords{root: t.TempDir()}
	if err = records.saveCapabilityReport(report); err != nil {
		t.Fatalf("capability report rejected an empty optional Codex CLI pin: %v", err)
	}
	if _, err = records.loadCapabilityReport(report); err != nil {
		t.Fatalf("load capability report without a Codex CLI pin: %v", err)
	}
	if _, err = executionStopperFor(Config{Host: "orca_supervised"}); err == nil || !isBackendError(err, executionapi.BackendUnsupported) {
		t.Fatalf("StopExecution error = %v", err)
	}
}

func TestSupervisedOrcaExecutionBindingRequiresExactDispatchIdentity(t *testing.T) {
	c := supervisedFixtureConfig(t)
	runID, attemptID := newID(), newID()
	ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaSupervised, Target: targetFor(c), Agent: agentFor(c), RunID: runID, AttemptID: attemptID, WorkspacePath: "C:/remote/worktree", WorkspaceBranch: "erranliu/remote", BaseCommit: strings.Repeat("a", 40), External: executionapi.ProcessIdentity{Kind: "orca_supervised", RuntimeID: "runtime-1", OrchestrationRun: "run-1", Task: "task-1", Dispatch: "dispatch-1", ExecutionHost: "host-1", WorktreeID: "repo-1::C:/remote/worktree"}}
	if err := validateExecutionRef(ref); err != nil {
		t.Fatalf("complete supervised binding rejected: %v", err)
	}
	ref.External.Dispatch = ""
	if err := validateExecutionRef(ref); err == nil {
		t.Fatal("supervised binding without its exact Dispatch was accepted")
	}
	backend, protocol, err := executionapi.ExecutionBackendKind("orca_supervised")
	if err != nil || backend != string(executionapi.BackendOrca) || protocol != string(executionapi.ProtocolOrcaSupervised) {
		t.Fatalf("backend resolution = %q/%q, %v", backend, protocol, err)
	}
}

func TestSupervisedOrcaClientPinsLaunchAndReadsExactDispatch(t *testing.T) {
	c := supervisedFixtureConfig(t)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKTREE_ID", "repo-test::C:/remote/spec03-test")
	workflowRunID := newID()
	client := remoteWorkerClient(c)
	coordinator, err := client.CreateCoordinator(context.Background(), workflowRunID, "fake-center-runtime")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := orcaworker.CreateRun(context.Background(), client.CenterCall, "remote worker fixture", coordinator, "fake-center-runtime")
	runID, requestID := receipt.ID, receipt.RequestID
	c.Orca.Supervised.CoordinatorTerminal = coordinator
	backend := orcaSupervisedBackend{config: c}
	if err != nil {
		t.Fatal(err)
	}
	if runID != "orca-run-test" {
		t.Fatalf("created Orca Run ID = %q", runID)
	}
	if requestID != "request-run-create" || coordinator != "term-"+workflowRunID {
		t.Fatalf("Orca Run-create request ID = %q", requestID)
	}
	started, err := remoteWorkerClient(c).StartTask(context.Background(), runID, "task-test", "repo-test::C:/remote/spec03-test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if started.TaskID != "task-test" || started.DispatchID != "dispatch-test" || started.WorktreeID != "repo-test::C:/remote/spec03-test" || started.PlatformRequestID != "request-worker-start" || started.State != "ready" || started.Stage != "input_accepted" {
		t.Fatalf("worker start receipt = %+v", started)
	}
	observed, err := backend.showWorker(context.Background(), started)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Identity.RuntimeID != "fake-remote-runtime" || observed.Identity.OrchestrationRun != runID || observed.Identity.Task != started.TaskID || observed.Identity.Dispatch != started.DispatchID || observed.Identity.WorktreeID != started.WorktreeID || observed.Liveness != "live" || observed.WorktreePath != "C:/remote/spec03-test" || observed.Branch != "erranliu/spec03-test" {
		t.Fatalf("worker observation = %+v", observed)
	}
	target := targetFor(c)
	target.OrchestrationRun, target.ExternalTask, target.Dispatch, target.WorktreeID, target.ExecutionHost = runID, started.TaskID, started.DispatchID, started.WorktreeID, "env-home"
	ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaSupervised, Target: target, Agent: agentFor(c), RunID: workflowRunID, AttemptID: newID(), External: observed.Identity}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKER_EPOCH", "replacement-remote-runtime")
	if _, err := backend.Observe(context.Background(), ref); err == nil || !isBackendError(err, executionapi.BackendIdentityMismatch) {
		t.Fatalf("replacement remote runtime was treated as the original Dispatch: %v", err)
	}
}

func TestSupervisedOrcaClientRejectsReturnedReplacementWorktree(t *testing.T) {
	c := supervisedFixtureConfig(t)
	_, err := remoteWorkerClient(c).StartTask(context.Background(), "orca-run-test", "task-test", "repo-test::C:/remote/expected", nil)
	err = remoteWorkerError(err)
	if err == nil || !isBackendError(err, executionapi.BackendIdentityMismatch) {
		t.Fatalf("replacement worktree error = %v", err)
	}
}

func TestSupervisedRunCoordinatorsAreDistinctAndFenced(t *testing.T) {
	c := supervisedFixtureConfig(t)
	client := remoteWorkerClient(c)
	a, b := newID(), newID()
	first, err := client.CreateCoordinator(context.Background(), a, "fake-center-runtime")
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.CreateCoordinator(context.Background(), b, "fake-center-runtime")
	if err != nil || first == second {
		t.Fatalf("WorkflowRuns shared a coordinator: %q / %q, %v", first, second, err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_RUN_COORDINATOR", first)
	ref := executionapi.ExecutionRef{RunID: a, Target: executionapi.ExecutionTarget{CoordinatorTerminal: first}, External: executionapi.ProcessIdentity{Kind: "orca_supervised", OrchestrationRun: "orca-run-test", CoordinatorRuntimeID: "fake-center-runtime"}}
	if _, err := client.Check(context.Background(), remoteWorkerRef(ref)); err != nil {
		t.Fatalf("exact coordinator was fenced: %v", err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_RUN_COORDINATOR", second)
	if _, err := client.Check(context.Background(), remoteWorkerRef(ref)); err == nil || !isBackendError(remoteWorkerError(err), executionapi.BackendIdentityMismatch) {
		t.Fatalf("Run rebinding was not rejected before inbox read: %v", err)
	}
}

func TestSettledSupervisedDispatchRetainsExactBindingWithoutLiveTerminal(t *testing.T) {
	c := supervisedFixtureConfig(t)
	b := orcaSupervisedBackend{config: c}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_DISPATCH_STATE", "completed")
	started := supervisedStartReceipt{RunID: "orca-run-test", TaskID: "task-test", DispatchID: "dispatch-test", WorktreeID: "repo-test::C:/remote/steerline-run", CenterRuntimeID: "fake-center-runtime"}
	observed, err := b.showWorker(context.Background(), started)
	if err != nil {
		t.Fatal(err)
	}
	ref := supervisedPromptTestRef(c, started, observed)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_SETTLED_NO_TERMINAL", "1")
	if _, err := b.showWorker(context.Background(), started); err == nil {
		t.Fatal("settled worker unexpectedly retained a live terminal observation")
	}
	if state, err := remoteWorkerClient(c).Settlement(context.Background(), remoteWorkerRef(ref), "succeeded"); err != nil || state != "succeeded" {
		t.Fatalf("durable settled Dispatch was rejected: %q, %v", state, err)
	}
	if _, err := remoteWorkerClient(c).Settlement(context.Background(), remoteWorkerRef(ref), "failed"); !isBackendError(remoteWorkerError(err), executionapi.BackendIdentityMismatch) {
		t.Fatalf("conflicting worker_done outcome was accepted: %v", err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKER_EPOCH", "replacement-remote-runtime")
	if _, err := remoteWorkerClient(c).Settlement(context.Background(), remoteWorkerRef(ref), "succeeded"); !isBackendError(remoteWorkerError(err), executionapi.BackendIdentityMismatch) {
		t.Fatalf("replacement remote runtime was accepted: %v", err)
	}
}

func supervisedPromptTestRef(c Config, started supervisedStartReceipt, observation supervisedWorkerObservation) executionapi.ExecutionRef {
	target := targetFor(c)
	target.OrchestrationRun = started.RunID
	target.ExternalTask = started.TaskID
	target.Dispatch = started.DispatchID
	target.ExecutionHost = observation.Identity.ExecutionHost
	target.WorktreeID = observation.Identity.WorktreeID
	target.Branch = observation.Branch
	target.BaseRevision = strings.Repeat("a", 40)
	target.CoordinatorTerminal = c.Orca.Supervised.CoordinatorTerminal
	return executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaSupervised, Target: target, Agent: executionapi.AgentSpec{Name: "codex"}, RunID: newID(), AttemptID: "attempt-test", WorkspacePath: observation.WorktreePath, WorkspaceBranch: observation.Branch, BaseCommit: target.BaseRevision, External: observation.Identity, Generation: observation.Identity.RuntimeID + ":" + observation.Identity.Incarnation}
}

func TestSupervisedReplyRejectsEscapedOversizeBeforeOrcaCall(t *testing.T) {
	c := supervisedFixtureConfig(t)
	started := supervisedStartReceipt{RunID: "orca-run-test", TaskID: "task-test", DispatchID: "dispatch-test"}
	worktreeID := "repo-test::C:/remote/steerline-run"
	observation := supervisedWorkerObservation{Identity: executionapi.ProcessIdentity{Kind: "orca_supervised", RuntimeID: "fake-remote-runtime", CoordinatorRuntimeID: "fake-center-runtime", Terminal: "terminal-test", Incarnation: "inc-test", OrchestrationRun: started.RunID, Task: started.TaskID, Dispatch: started.DispatchID, ExecutionHost: "env-home", WorktreeID: worktreeID}, WorktreePath: "C:/remote/steerline-run", Branch: "erranliu/steerline-run"}
	ref := supervisedPromptTestRef(c, started, observation)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_OBSERVE_UNAVAILABLE", "1")
	_, err := remoteWorkerClient(c).Reply(context.Background(), remoteWorkerRef(ref), "question-test", "x"+strings.Repeat("\n", 4000))
	if !isBackendError(remoteWorkerError(err), executionapi.BackendRejected) || !strings.Contains(err.Error(), "serialized message exceeds") {
		t.Fatalf("escaped oversized reply reached Orca observation or send: %v", err)
	}
	_, err = remoteWorkerClient(c).Reply(context.Background(), remoteWorkerRef(ref), "question-test", "yes")
	if !isBackendError(remoteWorkerError(err), executionapi.BackendUnreachable) {
		t.Fatalf("bounded reply was rejected before Orca observation: %v", err)
	}
}

func TestSupervisedPrepareKeepsPlatformWorktreeIdentitySeparateFromPath(t *testing.T) {
	const worktreeID = "repo-test::C:/remote/steerline-run"
	run := &Run{Protocol: string(executionapi.ProtocolOrcaSupervised), Target: executionapi.ExecutionTarget{WorktreeID: worktreeID}}
	execution := &ExecutionRecord{Protocol: run.Protocol, Target: run.Target}
	run.Target.WorktreeID = "incorrect-view-only-id"
	workspace := WorkspaceEvidence{Worktree: "C:/remote/steerline-run", Branch: "erranliu/steerline-run", Base: strings.Repeat("a", 40), Head: strings.Repeat("a", 40)}

	applyWorkflowWorkspaceEvidence(run, execution, workspace)

	if execution.Target.WorktreeID != worktreeID {
		t.Fatalf("supervised target lost the exact Orca worktree ID: got %q want %q", execution.Target.WorktreeID, worktreeID)
	}
	if run.Target.WorktreeID != worktreeID {
		t.Fatalf("supervised Run target was not synchronized with its Execution target: got %q want %q", run.Target.WorktreeID, worktreeID)
	}
	if execution.Workspace.Worktree != workspace.Worktree || execution.Target.Branch != workspace.Branch || execution.Target.BaseRevision != workspace.Base {
		t.Fatalf("supervised workspace evidence was not persisted separately: target=%+v workspace=%+v", execution.Target, execution.Workspace)
	}
}

func TestLoadConfigRequiresExplicitRemoteCancellationFalse(t *testing.T) {
	c := supervisedFixtureConfig(t)
	t.Setenv("STEERLINE_HOME", t.TempDir())
	c.RunDir, c.WorktreeRoot = "", ""
	profile := c.projectExecutionProfile()
	profile.ID, profile.Name = "remote", "Remote"
	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	var orca map[string]json.RawMessage
	if err = json.Unmarshal(fields["orca"], &orca); err != nil {
		t.Fatal(err)
	}
	var supervised map[string]json.RawMessage
	if err = json.Unmarshal(orca["supervised"], &supervised); err != nil {
		t.Fatal(err)
	}
	delete(supervised, "remote_cancel_available")
	orca["supervised"], _ = json.Marshal(supervised)
	fields["orca"], _ = json.Marshal(orca)
	withoutPolicy, _ := json.Marshal(map[string]any{"execution_profiles": []any{fields}, "default_execution_profile": "remote"})
	configPath := filepath.Join(t.TempDir(), "steerline.json")
	if err = os.WriteFile(configPath, projectConfigJSON(t, c), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(GlobalExecutionConfigPath(), withoutPolicy, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadConfig(configPath); err == nil || !strings.Contains(err.Error(), "explicitly set to false") {
		t.Fatalf("missing remote cancellation policy error = %v", err)
	}
	supervised["remote_cancel_available"] = json.RawMessage("true")
	orca["supervised"], _ = json.Marshal(supervised)
	fields["orca"], _ = json.Marshal(orca)
	withEnabledStop, _ := json.Marshal(map[string]any{"execution_profiles": []any{fields}, "default_execution_profile": "remote"})
	if err = os.WriteFile(GlobalExecutionConfigPath(), withEnabledStop, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadConfig(configPath); err == nil || !strings.Contains(err.Error(), "fixed unavailable") {
		t.Fatalf("enabled remote cancellation error = %v", err)
	}
}

func isBackendError(err error, want executionapi.BackendErrorCode) bool {
	var backendErr *executionapi.BackendError
	return errors.As(err, &backendErr) && backendErr.Code == want
}
