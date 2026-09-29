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
	"time"
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
	if c.ProjectSteps != nil {
		steps := *c.ProjectSteps
		steps.Implement.ExecutionSite = "target"
		steps.Verify.ExecutionSite = "target"
		steps.Review.ExecutionSite = "target"
		steps.PublishPR.ExecutionSite = "target"
		c.ProjectSteps = &steps
	}
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

func TestSupervisedHostWaitAndLaunchFailureRequireManualCheck(t *testing.T) {
	c := supervisedFixtureConfig(t)
	b := orcaSupervisedBackend{config: c}
	expected := supervisedStartReceipt{RunID: "orca-run-test", TaskID: "task-test", DispatchID: "dispatch-test", WorktreeID: "repo-test::C:/remote/steerline-run", CenterRuntimeID: "fake-center-runtime"}
	for _, tc := range []struct{ raw, want string }{{"present", "present"}, {"none", "none"}, {"", "unknown"}} {
		t.Setenv("STEERLINE_FAKE_SUPERVISED_AGENT_WAIT", tc.raw)
		observed, err := b.showWorker(context.Background(), expected)
		if err != nil || observed.AgentWaitState != tc.want {
			t.Fatalf("agentWait %q = %+v, %v; want %q", tc.raw, observed, err, tc.want)
		}
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_AGENT_WAIT", "present")
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: targetFor(c), Agent: AgentSpec{Name: "codex"}, RunID: newID(), External: ProcessIdentity{Kind: "orca_supervised", RuntimeID: "fake-remote-runtime", CoordinatorRuntimeID: "fake-center-runtime", Terminal: "terminal-test", Incarnation: "inc-test", OrchestrationRun: "orca-run-test", Task: "task-test", Dispatch: "dispatch-test", ExecutionHost: "env-home", WorktreeID: expected.WorktreeID}}
	ref.Target.CoordinatorTerminal = "term-coordinator"
	ref.Target.OrchestrationRun, ref.Target.ExternalTask, ref.Target.Dispatch = ref.External.OrchestrationRun, ref.External.Task, ref.External.Dispatch
	ref.Target.ExecutionHost, ref.Target.WorktreeID = ref.External.ExecutionHost, ref.External.WorktreeID
	if observed, err := b.Observe(context.Background(), ref); err != nil || observed.Diagnostic != "host_interaction_pending" {
		t.Fatalf("host wait observation = %+v, %v", observed, err)
	}
	batch := OutputBatch{Data: mustJSON(map[string]any{"transcript": map[string]any{"messages": []any{map[string]any{"role": "tool", "blocks": []any{map[string]any{"type": "tool-result", "output": "orchestrator_helper_launch_canceled: ShellExecuteExW failed to launch setup helper: 1223"}}}}}}), Source: "transcript", NextCursor: "host-cursor", ObservedAt: now()}
	activity := supervisedOutputActivity(batch, "local-run", "local-task")
	if activity.Kind != "error" || activity.Status != "manual_host_check" || !strings.Contains(activity.Detail, "will not treat this as a business question") {
		t.Fatalf("host command launch activity = %+v", activity)
	}
	observerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker := &Worker{Events: make(chan WorkerEvent, 16), Done: make(chan error, 1), observationDone: make(chan struct{})}
	input := MinimalStepInput{WorkflowRunID: "local-run", TaskID: "local-task", AttemptID: "local-attempt"}
	go b.observeWorker(observerCtx, worker, ref, StepAttemptRecord{AttemptID: input.AttemptID, Status: "dispatched"}, input, filepath.Join(t.TempDir(), "result.json"), false)
	seenPending, seenCleared := false, false
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !seenCleared {
		select {
		case event := <-worker.Events:
			if event.Activity.Status == "manual_host_check" {
				seenPending = true
				t.Setenv("STEERLINE_FAKE_SUPERVISED_AGENT_WAIT", "none")
			}
			if event.Activity.Title == "Remote host interaction cleared" {
				seenCleared = true
				if event.Activity.ItemID != "host-wait:dispatch-test" || event.Activity.Status != "completed" {
					t.Fatalf("host wait clear activity = %+v", event.Activity)
				}
			}
			event.Processed <- nil
		case <-deadline.C:
			t.Fatal("observer did not publish and clear the exact host wait")
		}
	}
	if !seenPending {
		t.Fatal("observer cleared a host wait that it never reported")
	}
	cancel()
	<-worker.observationDone
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
	if capabilityStatus(report, "stop") != CapabilityUnsupported || capabilityStatus(report, "start") != CapabilityNotTested {
		t.Fatalf("supervised capabilities = %+v", report.Capabilities)
	}
	records := executionRecords{root: t.TempDir()}
	if err = records.saveCapabilityReport(report); err != nil {
		t.Fatalf("capability report rejected an empty optional Codex CLI pin: %v", err)
	}
	if _, err = records.loadCapabilityReport(report); err != nil {
		t.Fatalf("load capability report without a Codex CLI pin: %v", err)
	}
	if _, err = (orcaSupervisedBackend{}).StopExecution(context.Background(), ExecutionRef{}, Operation{}, nil); err == nil || !isBackendError(err, BackendUnsupported) {
		t.Fatalf("StopExecution error = %v", err)
	}
}

func TestSupervisedOrcaExecutionBindingRequiresExactDispatchIdentity(t *testing.T) {
	c := supervisedFixtureConfig(t)
	runID, attemptID := newID(), newID()
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: targetFor(c), Agent: agentFor(c), RunID: runID, AttemptID: attemptID, WorkspacePath: "C:/remote/worktree", WorkspaceBranch: "erranliu/remote", BaseCommit: strings.Repeat("a", 40), External: ProcessIdentity{Kind: "orca_supervised", RuntimeID: "runtime-1", OrchestrationRun: "run-1", Task: "task-1", Dispatch: "dispatch-1", ExecutionHost: "host-1", WorktreeID: "repo-1::C:/remote/worktree"}}
	if err := validateExecutionRef(ref); err != nil {
		t.Fatalf("complete supervised binding rejected: %v", err)
	}
	ref.External.Dispatch = ""
	if err := validateExecutionRef(ref); err == nil {
		t.Fatal("supervised binding without its exact Dispatch was accepted")
	}
	backend, protocol, err := ExecutionBackendKind("orca_supervised")
	if err != nil || backend != string(BackendOrca) || protocol != string(ProtocolOrcaSupervised) {
		t.Fatalf("backend resolution = %q/%q, %v", backend, protocol, err)
	}
}

func TestSupervisedOrcaClientPinsLaunchAndReadsExactDispatch(t *testing.T) {
	c := supervisedFixtureConfig(t)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKTREE_ID", "repo-test::C:/remote/spec03-test")
	backend := orcaSupervisedBackend{config: c}
	workflowRunID := newID()
	runID, requestID, coordinator, err := backend.createOrchestrationRun(context.Background(), workflowRunID, "spec03 fixture")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "orca-run-test" {
		t.Fatalf("created Orca Run ID = %q", runID)
	}
	if requestID != "request-run-create" || coordinator != "term-"+workflowRunID {
		t.Fatalf("Orca Run-create request ID = %q", requestID)
	}
	base := strings.Repeat("a", 40)
	started, err := backend.startWorker(context.Background(), runID, newID(), "prepare", "verify workspace", "new-top-level", "spec03-test", "id:repo-test", base)
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
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: target, Agent: agentFor(c), RunID: workflowRunID, AttemptID: newID(), External: observed.Identity}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKER_EPOCH", "replacement-remote-runtime")
	if _, err := backend.Observe(context.Background(), ref); err == nil || !isBackendError(err, BackendIdentityMismatch) {
		t.Fatalf("replacement remote runtime was treated as the original Dispatch: %v", err)
	}
}

func TestSupervisedOrcaClientRejectsReturnedReplacementWorktree(t *testing.T) {
	c := supervisedFixtureConfig(t)
	backend := orcaSupervisedBackend{config: c}
	_, err := backend.startWorker(context.Background(), "orca-run-test", newID(), "verify", "read only", "id:repo-test::C:/remote/expected", "", "", "")
	if err == nil || !isBackendError(err, BackendIdentityMismatch) {
		t.Fatalf("replacement worktree error = %v", err)
	}
}

func TestSupervisedRunCoordinatorsAreDistinctAndFenced(t *testing.T) {
	c := supervisedFixtureConfig(t)
	backend := orcaSupervisedBackend{config: c}
	a, b := newID(), newID()
	first, err := backend.createRunCoordinator(context.Background(), a, "fake-center-runtime")
	if err != nil {
		t.Fatal(err)
	}
	second, err := backend.createRunCoordinator(context.Background(), b, "fake-center-runtime")
	if err != nil || first == second {
		t.Fatalf("WorkflowRuns shared a coordinator: %q / %q, %v", first, second, err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_RUN_COORDINATOR", first)
	ref := ExecutionRef{RunID: a, Target: ExecutionTarget{CoordinatorTerminal: first}, External: ProcessIdentity{Kind: "orca_supervised", OrchestrationRun: "orca-run-test", CoordinatorRuntimeID: "fake-center-runtime"}}
	if _, err := backend.checkDelivery(context.Background(), ref); err != nil {
		t.Fatalf("exact coordinator was fenced: %v", err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_RUN_COORDINATOR", second)
	if _, err := backend.checkDelivery(context.Background(), ref); err == nil || !isBackendError(err, BackendIdentityMismatch) {
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
	if state, err := b.settledWorker(context.Background(), ref, "succeeded"); err != nil || state != "succeeded" {
		t.Fatalf("durable settled Dispatch was rejected: %q, %v", state, err)
	}
	if receipt, err := b.SettleResources(context.Background(), ref, "succeeded"); err != nil || receipt.Disposition != "released" {
		t.Fatalf("exact settled worker could not be released: %+v, %v", receipt, err)
	}
	if _, err := b.settledWorker(context.Background(), ref, "failed"); !isBackendError(err, BackendIdentityMismatch) {
		t.Fatalf("conflicting worker_done outcome was accepted: %v", err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKER_EPOCH", "replacement-remote-runtime")
	if _, err := b.settledWorker(context.Background(), ref, "succeeded"); !isBackendError(err, BackendIdentityMismatch) {
		t.Fatalf("replacement remote runtime was accepted: %v", err)
	}
}

func TestSettledSupervisedObserverReplaysOriginalResultBeforeOutput(t *testing.T) {
	c := supervisedFixtureConfig(t)
	b := orcaSupervisedBackend{config: c}
	started := supervisedStartReceipt{RunID: "orca-run-test", TaskID: "task-test", DispatchID: "dispatch-test", WorktreeID: "repo-test::C:/remote/steerline-run", CenterRuntimeID: "fake-center-runtime"}
	observed, err := b.showWorker(context.Background(), started)
	if err != nil {
		t.Fatal(err)
	}
	ref := supervisedPromptTestRef(c, started, observed)
	input := MinimalStepInput{ProjectIdentity: c.identity, TaskID: "business-task", WorkflowRunID: ref.RunID, ExecutionID: newID(), AttemptID: ref.AttemptID, StepID: "implement"}
	attempt := StepAttemptRecord{AttemptID: input.AttemptID, StepID: input.StepID, Status: "running"}
	envelope := MinimalStepResultEnvelope{SchemaVersion: 1, ProjectIdentity: input.ProjectIdentity, TaskID: input.TaskID, WorkflowRunID: input.WorkflowRunID, ExecutionID: input.ExecutionID, AttemptID: input.AttemptID, Result: StepGateResult{StepID: input.StepID, ResultID: input.AttemptID, Succeeded: true}}
	payload := mustJSON(map[string]any{"taskId": started.TaskID, "dispatchId": started.DispatchID, "remote_codex_version": "codex-cli 0.156.1", "step_result": envelope})
	t.Setenv("STEERLINE_FAKE_SUPERVISED_SETTLED_STATUS", string(payload))
	t.Setenv("STEERLINE_FAKE_SUPERVISED_DENY_WORKER_READ", "1")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_DISPATCH_STATE", "completed")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_SETTLED_NO_TERMINAL", "1")
	resultPath := filepath.Join(t.TempDir(), "result.json")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	worker := &Worker{Events: make(chan WorkerEvent, 8), Done: make(chan error, 1), observationDone: make(chan struct{})}
	go b.observeWorker(ctx, worker, ref, attempt, input, resultPath, true)
	for {
		select {
		case event := <-worker.Events:
			if event.RemoteDeliveryID != "delivery-settled" {
				t.Fatalf("unexpected settled Delivery: %+v", event)
			}
			event.Processed <- nil
		case err := <-worker.Done:
			if err != nil || worker.RemoteOutcome != "succeeded" {
				t.Fatalf("settled result was not recovered: outcome=%q error=%v", worker.RemoteOutcome, err)
			}
			if _, err := os.Stat(resultPath); err != nil {
				t.Fatalf("settled result was not saved: %v", err)
			}
			return
		case <-ctx.Done():
			t.Fatal("settled result replay timed out")
		}
	}
}

func TestSupervisedResultDeliveryReplayUsesPersistedJSONContent(t *testing.T) {
	attempt := StepAttemptRecord{AttemptID: "attempt-1", StepID: "publish_pr"}
	input := MinimalStepInput{ProjectIdentity: "repo", TaskID: "business-task", WorkflowRunID: "workflow-1", ExecutionID: "execution-1", AttemptID: attempt.AttemptID, StepID: attempt.StepID}
	identity := ProcessIdentity{Task: "orca-task", Dispatch: "dispatch-1"}
	backend := orcaSupervisedBackend{config: Config{Orca: OrcaConfig{Supervised: &SupervisedOrcaConfig{}}}}
	resultPath := filepath.Join(t.TempDir(), "result.json")
	envelope := MinimalStepResultEnvelope{SchemaVersion: 1, ProjectIdentity: input.ProjectIdentity, TaskID: input.TaskID, WorkflowRunID: input.WorkflowRunID, ExecutionID: input.ExecutionID, AttemptID: input.AttemptID, Result: StepGateResult{StepID: attempt.StepID, ResultID: attempt.AttemptID, Succeeded: true, PRURL: "https://example.test/pr/1"}}
	version := "codex-cli 0.157.1"
	message := func() OrcaInboxMessage {
		return OrcaInboxMessage{Type: "status", FromHandle: "dispatch:" + identity.Dispatch, Payload: mustJSON(map[string]any{"taskId": identity.Task, "dispatchId": identity.Dispatch, "remote_codex_version": version, "step_result": envelope})}
	}
	if found, err := backend.stepResultFromMessages([]OrcaInboxMessage{message()}, identity, attempt, input, resultPath); err != nil || !found {
		t.Fatalf("first Delivery result = %t, %v", found, err)
	}
	if found, err := backend.stepResultFromMessages([]OrcaInboxMessage{message()}, identity, attempt, input, resultPath); err != nil || !found {
		t.Fatalf("same unacknowledged Delivery replay was not idempotent: %t, %v", found, err)
	}
	backend.config.Orca.Supervised.ExpectedCodexVersion = "codex-cli 0.157.1"
	version = "codex-cli 0.157.2"
	if found, err := backend.stepResultFromMessages([]OrcaInboxMessage{message()}, identity, attempt, input, resultPath); err == nil || !isBackendError(err, BackendIdentityMismatch) || found {
		t.Fatalf("explicit Codex CLI version pin did not reject a different version: %t, %v", found, err)
	}
	backend.config.Orca.Supervised.ExpectedCodexVersion = ""
	version = ""
	if found, err := backend.stepResultFromMessages([]OrcaInboxMessage{message()}, identity, attempt, input, resultPath); err == nil || !isBackendError(err, BackendIdentityMismatch) || found {
		t.Fatalf("missing observed Codex CLI version was accepted: %t, %v", found, err)
	}
	version = "codex-cli 0.157.1"
	backend.config.Orca.Supervised.ExpectedCodexVersion = "codex-cli 0.157.1"
	envelope.Result.PRURL = "https://example.test/pr/2"
	if found, err := backend.stepResultFromMessages([]OrcaInboxMessage{message()}, identity, attempt, input, resultPath); err == nil || !isBackendError(err, BackendProtocolInvalid) || found {
		t.Fatalf("different content under the same result ID was accepted: %t, %v", found, err)
	}
}

func TestSupervisedResultCannotCompleteAnotherAttempt(t *testing.T) {
	identity := ProcessIdentity{Task: "task-current", Dispatch: "dispatch-current"}
	attempt := StepAttemptRecord{AttemptID: "attempt-current", StepID: "verify"}
	expected := MinimalStepInput{ProjectIdentity: "repo", TaskID: "business-task", WorkflowRunID: "workflow-current", ExecutionID: "execution-current", AttemptID: attempt.AttemptID, StepID: attempt.StepID}
	backend := orcaSupervisedBackend{config: Config{Orca: OrcaConfig{Supervised: &SupervisedOrcaConfig{}}}}
	base := MinimalStepResultEnvelope{SchemaVersion: 1, ProjectIdentity: expected.ProjectIdentity, TaskID: expected.TaskID, WorkflowRunID: expected.WorkflowRunID, ExecutionID: expected.ExecutionID, AttemptID: expected.AttemptID, Result: StepGateResult{StepID: attempt.StepID, ResultID: attempt.AttemptID}}
	message := func(sender, task, dispatch string, envelope MinimalStepResultEnvelope) OrcaInboxMessage {
		return OrcaInboxMessage{Type: "status", FromHandle: sender, Payload: mustJSON(map[string]any{"taskId": task, "dispatchId": dispatch, "remote_codex_version": "codex-cli 0.157.1", "step_result": envelope})}
	}
	for _, test := range []struct {
		name      string
		message   OrcaInboxMessage
		wantErr   bool
		wantFound bool
	}{
		{name: "exact current Dispatch", message: message("dispatch:dispatch-current", identity.Task, identity.Dispatch, base), wantFound: true},
		{name: "wrong result ID under current Dispatch", message: func() OrcaInboxMessage {
			wrong := base
			wrong.Result.ResultID = "result-other"
			return message("dispatch:dispatch-current", identity.Task, identity.Dispatch, wrong)
		}(), wantErr: true},
		{name: "old Attempt under current Dispatch", message: func() OrcaInboxMessage {
			old := base
			old.AttemptID = "attempt-old"
			return message("dispatch:dispatch-current", identity.Task, identity.Dispatch, old)
		}(), wantErr: true},
		{name: "other Task under current Dispatch", message: message("dispatch:dispatch-current", "task-other", identity.Dispatch, base), wantErr: true},
		{name: "late old Dispatch", message: message("dispatch:dispatch-old", "task-old", "dispatch-old", base)},
		{name: "foreign Dispatch spoofing current payload", message: message("dispatch:dispatch-old", identity.Task, identity.Dispatch, base)},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "result.json")
			found, err := backend.stepResultFromMessages([]OrcaInboxMessage{test.message}, identity, attempt, expected, path)
			if found != test.wantFound || test.wantErr && !isBackendError(err, BackendIdentityMismatch) || !test.wantErr && err != nil {
				t.Fatalf("foreign result changed the current Attempt: found=%t error=%v", found, err)
			}
			if _, statErr := os.Stat(path); test.wantFound && statErr != nil || !test.wantFound && !os.IsNotExist(statErr) {
				t.Fatalf("result file did not match the fixed sender gate: %v", statErr)
			}
		})
	}
}

func TestSupervisedWorkerDoneWithoutStructuredResultBlocksCompletion(t *testing.T) {
	c := supervisedFixtureConfig(t)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_DONE_WITHOUT_RESULT", "1")
	b := orcaSupervisedBackend{config: c}
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: targetFor(c), Agent: AgentSpec{Name: "codex"}, RunID: newID(), AttemptID: "attempt-current", External: ProcessIdentity{Kind: "orca_supervised", RuntimeID: "fake-remote-runtime", CoordinatorRuntimeID: "fake-center-runtime", Terminal: "terminal-test", Incarnation: "inc-test", OrchestrationRun: "orca-run-test", Task: "task-test", Dispatch: "dispatch-test", ExecutionHost: "env-home", WorktreeID: "repo-test::C:/remote/steerline-run"}}
	ref.Target.CoordinatorTerminal = "term-coordinator"
	ref.Target.OrchestrationRun, ref.Target.ExternalTask, ref.Target.Dispatch = ref.External.OrchestrationRun, ref.External.Task, ref.External.Dispatch
	ref.Target.ExecutionHost, ref.Target.WorktreeID = ref.External.ExecutionHost, ref.External.WorktreeID
	input := MinimalStepInput{ProjectIdentity: "repo", TaskID: "business-task", WorkflowRunID: ref.RunID, ExecutionID: "execution-current", AttemptID: ref.AttemptID, StepID: "verify"}
	attempt := StepAttemptRecord{AttemptID: ref.AttemptID, StepID: input.StepID, Status: "running"}
	path := filepath.Join(t.TempDir(), "missing-result.json")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	worker := &Worker{Events: make(chan WorkerEvent, 8), Done: make(chan error, 1), observationDone: make(chan struct{})}
	go b.observeWorker(ctx, worker, ref, attempt, input, path, false)
	for {
		select {
		case event := <-worker.Events:
			event.Processed <- nil
		case err := <-worker.Done:
			if !isBackendError(err, BackendProtocolInvalid) || !strings.Contains(err.Error(), "no matching structured step result") {
				t.Fatalf("bare worker_done completed the step: %v", err)
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatalf("missing structured result acquired a file: %v", statErr)
			}
			return
		case <-ctx.Done():
			t.Fatal("observer never rejected the bare worker_done")
		}
	}
}

func TestSupervisedTurnRequiresTranscriptOrActiveTerminalEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		data   string
		want   bool
	}{
		{"input-only", "transcript", `{"transcript":{"messages":[{"role":"user"}]}}`, false},
		{"terminal-tail", "terminal", `{"transcript":{"messages":[{"role":"assistant"}]}}`, false},
		{"terminal-idle", "terminal", `{"terminal":{"tail":["› Ask Codex to do anything"]}}`, false},
		{"terminal-working", "terminal", `{"terminal":{"tail":["• Working (1s · esc to interrupt)"]}}`, true},
		{"terminal-reconnecting", "terminal", `{"terminal":{"tail":["• Reconnecting... 4/5", "└ Request timed out"]}}`, true},
		{"assistant-turn", "transcript", `{"transcript":{"messages":[{"role":"user"},{"role":"assistant"}]}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := supervisedTranscriptTurnStarted(OutputBatch{Source: tc.source, Data: []byte(tc.data)}); got != tc.want {
				t.Fatalf("turn evidence = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestSupervisedPromptHandshakeDoesNotEnterAnActiveTerminal(t *testing.T) {
	c := supervisedFixtureConfig(t)
	marker := filepath.Join(t.TempDir(), "prompt-enter.txt")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_TUI_STATE", "active")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_ENTER_MARKER", marker)
	worktreeID := "repo-test::C:/remote/steerline-run"
	started := supervisedStartReceipt{RunID: "orca-run-test", TaskID: "task-test", DispatchID: "dispatch-test", WorktreeID: worktreeID, CenterRuntimeID: "fake-center-runtime"}
	observation := supervisedWorkerObservation{Identity: ProcessIdentity{Kind: "orca_supervised", RuntimeID: "fake-remote-runtime", CoordinatorRuntimeID: "fake-center-runtime", Terminal: "terminal-test", Incarnation: "inc-test", OrchestrationRun: started.RunID, Task: started.TaskID, Dispatch: started.DispatchID, ExecutionHost: "env-home", WorktreeID: worktreeID}, WorktreePath: "C:/remote/steerline-run", Branch: "erranliu/steerline-run", Liveness: "live", DispatchState: "dispatched"}
	ref := supervisedPromptTestRef(c, started, observation)

	startedTurn, err := (orcaSupervisedBackend{config: c}).submitAcceptedPrompt(context.Background(), started, observation, ref, "prepare")
	if err != nil {
		t.Fatal(err)
	}
	if !startedTurn {
		t.Fatal("existing active turn was not recognized")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("active worker terminal received Enter: %v", err)
	}
}

func TestSupervisedPromptHandshakeLeavesUnreadyTerminalUntouched(t *testing.T) {
	c := supervisedFixtureConfig(t)
	marker := filepath.Join(t.TempDir(), "prompt-enter.txt")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_TUI_STATE", "not-ready")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_ENTER_MARKER", marker)
	worktreeID := "repo-test::C:/remote/steerline-run"
	started := supervisedStartReceipt{RunID: "orca-run-test", TaskID: "task-test", DispatchID: "dispatch-test", WorktreeID: worktreeID, CenterRuntimeID: "fake-center-runtime"}
	observation := supervisedWorkerObservation{Identity: ProcessIdentity{Kind: "orca_supervised", RuntimeID: "fake-remote-runtime", CoordinatorRuntimeID: "fake-center-runtime", Terminal: "terminal-test", Incarnation: "inc-test", OrchestrationRun: started.RunID, Task: started.TaskID, Dispatch: started.DispatchID, ExecutionHost: "env-home", WorktreeID: worktreeID}, WorktreePath: "C:/remote/steerline-run", Branch: "erranliu/steerline-run", Liveness: "live", DispatchState: "dispatched"}
	ref := supervisedPromptTestRef(c, started, observation)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := (orcaSupervisedBackend{config: c}).submitAcceptedPrompt(ctx, started, observation, ref, "prepare"); err == nil || !isBackendError(err, BackendOutcomeUnknown) {
		t.Fatalf("TUI without staged prompt was treated as ready: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("unready worker terminal received Enter: %v", err)
	}
}

func supervisedPromptTestRef(c Config, started supervisedStartReceipt, observation supervisedWorkerObservation) ExecutionRef {
	target := targetFor(c)
	target.OrchestrationRun = started.RunID
	target.ExternalTask = started.TaskID
	target.Dispatch = started.DispatchID
	target.ExecutionHost = observation.Identity.ExecutionHost
	target.WorktreeID = observation.Identity.WorktreeID
	target.Branch = observation.Branch
	target.BaseRevision = strings.Repeat("a", 40)
	target.CoordinatorTerminal = c.Orca.Supervised.CoordinatorTerminal
	return ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: target, Agent: AgentSpec{Name: "codex"}, RunID: newID(), AttemptID: "attempt-test", WorkspacePath: observation.WorktreePath, WorkspaceBranch: observation.Branch, BaseCommit: target.BaseRevision, External: observation.Identity, Generation: observation.Identity.RuntimeID + ":" + observation.Identity.Incarnation}
}

func TestSupervisedReplyRejectsEscapedOversizeBeforeOrcaCall(t *testing.T) {
	c := supervisedFixtureConfig(t)
	started := supervisedStartReceipt{RunID: "orca-run-test", TaskID: "task-test", DispatchID: "dispatch-test"}
	worktreeID := "repo-test::C:/remote/steerline-run"
	observation := supervisedWorkerObservation{Identity: ProcessIdentity{Kind: "orca_supervised", RuntimeID: "fake-remote-runtime", CoordinatorRuntimeID: "fake-center-runtime", Terminal: "terminal-test", Incarnation: "inc-test", OrchestrationRun: started.RunID, Task: started.TaskID, Dispatch: started.DispatchID, ExecutionHost: "env-home", WorktreeID: worktreeID}, WorktreePath: "C:/remote/steerline-run", Branch: "erranliu/steerline-run"}
	ref := supervisedPromptTestRef(c, started, observation)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_OBSERVE_UNAVAILABLE", "1")
	_, err := (orcaSupervisedBackend{config: c}).replyQuestion(context.Background(), ref, "question-test", "x"+strings.Repeat("\n", 4000))
	if !isBackendError(err, BackendRejected) || !strings.Contains(err.Error(), "serialized message exceeds") {
		t.Fatalf("escaped oversized reply reached Orca observation or send: %v", err)
	}
	_, err = (orcaSupervisedBackend{config: c}).replyQuestion(context.Background(), ref, "question-test", "yes")
	if !isBackendError(err, BackendUnreachable) {
		t.Fatalf("bounded reply was rejected before Orca observation: %v", err)
	}
}

func TestSupervisedPrepareKeepsPlatformWorktreeIdentitySeparateFromPath(t *testing.T) {
	const worktreeID = "repo-test::C:/remote/steerline-run"
	run := &Run{Protocol: string(ProtocolOrcaSupervised), Target: ExecutionTarget{WorktreeID: worktreeID}}
	execution := &ExecutionRecord{Target: run.Target}
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

func TestSupervisedWorkspaceRestoresMissingTargetFromPrepareBinding(t *testing.T) {
	fixed := ExecutionTarget{ID: "supervised:test", ConnectionID: "connection-test", ExternalRuntime: "environment-test"}
	bound := fixed
	bound.WorktreeID = "repo-test::C:/remote/steerline-run"
	bound.OrchestrationRun, bound.ExternalTask, bound.Dispatch = "run-test", "task-test", "dispatch-test"
	run := &Run{ExecutionID: "execution-test", Target: fixed}
	attempts := []StepAttemptRecord{{ExecutionID: run.ExecutionID, StepID: "prepare", ExternalBinding: bound}}
	records := executionRecords{root: t.TempDir()}

	if err := restoreSupervisedTarget(run, attempts, fixed, records); err != nil {
		t.Fatal(err)
	}
	if run.Target != bound {
		t.Fatalf("fixed prepare binding was not restored: got %+v want %+v", run.Target, bound)
	}
	stepTarget := bound
	stepTarget.ExternalTask, stepTarget.Dispatch = "task-implement", "dispatch-implement"
	stepRun := &Run{ExecutionID: run.ExecutionID, Target: stepTarget}
	if err := restoreSupervisedTarget(stepRun, attempts, fixed, records); err != nil {
		t.Fatal(err)
	}
	if stepRun.Target.ExternalTask != "task-implement" || stepRun.Target.Dispatch != "dispatch-implement" {
		t.Fatalf("later step Dispatch identity was replaced with the prepare Dispatch: %+v", stepRun.Target)
	}

	run.Target.WorktreeID = "repo-test::C:/remote/other"
	if err := restoreSupervisedTarget(run, attempts, fixed, records); err == nil || !isNativeErrorCode(err, "authority_mismatch") {
		t.Fatalf("conflicting runtime worktree identity was not rejected: %v", err)
	}

	workflowID, attemptID := newID(), newID()
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: bound, Agent: AgentSpec{Name: "codex"}, RunID: workflowID, AttemptID: attemptID, WorkspacePath: "C:/remote/steerline-run", WorkspaceBranch: "erranliu/steerline-run", BaseCommit: strings.Repeat("a", 40), External: ProcessIdentity{Kind: "orca_supervised", RuntimeID: "remote-runtime", CoordinatorRuntimeID: "center-runtime", Terminal: "terminal-test", Incarnation: "inc-test", OrchestrationRun: bound.OrchestrationRun, Task: bound.ExternalTask, Dispatch: bound.Dispatch, ExecutionHost: "environment-test", WorktreeID: bound.WorktreeID}}
	if err := records.saveBinding(ref); err != nil {
		t.Fatal(err)
	}
	missingRuntimeBinding := StepAttemptRecord{ExecutionID: "execution-test", AttemptID: attemptID, StepID: "prepare"}
	restoredRun := &Run{RunID: workflowID, ExecutionID: "execution-test", Protocol: string(ProtocolOrcaSupervised), Target: fixed}
	if err := restoreSupervisedTarget(restoredRun, []StepAttemptRecord{missingRuntimeBinding}, fixed, records); err != nil {
		t.Fatal(err)
	}
	if restoredRun.Target != bound {
		t.Fatalf("exact durable binding was not restored from its record: got %+v want %+v", restoredRun.Target, bound)
	}
	workspace := WorkspaceEvidence{Worktree: "C:/remote/steerline-run", Branch: "erranliu/steerline-run", Base: strings.Repeat("a", 40), Head: strings.Repeat("a", 40)}
	execution := ExecutionRecord{Target: fixed}
	applyWorkflowWorkspaceEvidence(restoredRun, &execution, workspace)
	if execution.Target.WorktreeID != bound.WorktreeID || execution.Target.OrchestrationRun != bound.OrchestrationRun || execution.Workspace.Worktree != workspace.Worktree {
		t.Fatalf("restored exact platform target was not persisted with workspace evidence: target=%+v workspace=%+v", execution.Target, execution.Workspace)
	}
}

func TestDecodeSupervisedStartInputRestoresNativeFixedStepInput(t *testing.T) {
	step := MinimalStepInput{SchemaVersion: 1, ProjectIdentity: `C:\repo\.git`, TaskID: "native-task", WorkflowRunID: "workflow", ExecutionID: "execution", AttemptID: "attempt", StepID: "implement", InputRevision: strings.Repeat("a", 40), PrerequisiteResultIDs: []string{"prepare-result"}}
	native := NativeTaskWorkerContext{SchemaVersion: NativeTaskInputSchema, ProjectID: step.ProjectIdentity, TaskID: step.TaskID, WorkflowRunID: step.WorkflowRunID, ExecutionID: step.ExecutionID, AttemptID: step.AttemptID, CurrentStep: step.StepID, Contract: TaskContract{ContractID: "contract", Digest: "digest"}, ContractDigest: "digest", StepInput: step}
	request := mustJSON(map[string]any{"input": native})
	decoded, err := decodeSupervisedStartInput(request)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ProjectIdentity != step.ProjectIdentity || decoded.TaskID != step.TaskID || decoded.WorkflowRunID != step.WorkflowRunID || decoded.ExecutionID != step.ExecutionID || decoded.AttemptID != step.AttemptID || decoded.StepID != step.StepID || decoded.InputRevision != step.InputRevision || !sameStrings(decoded.PrerequisiteResultIDs, step.PrerequisiteResultIDs) {
		t.Fatalf("native start request did not restore the exact Step Input: got %+v want %+v", decoded, step)
	}

	native.StepInput.StepID = "verify"
	if _, err = decodeSupervisedStartInput(mustJSON(map[string]any{"input": native})); err == nil || !isBackendError(err, BackendIdentityMismatch) {
		t.Fatalf("mismatched native current_step was accepted during recovery: %v", err)
	}
}

func TestLoadConfigRequiresExplicitRemoteCancellationFalse(t *testing.T) {
	c := supervisedFixtureConfig(t)
	encoded, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &config); err != nil {
		t.Fatal(err)
	}
	config["workflow_protocol"] = json.RawMessage(`"worker-flow-v1"`)
	var orca map[string]json.RawMessage
	if err = json.Unmarshal(config["orca"], &orca); err != nil {
		t.Fatal(err)
	}
	var supervised map[string]json.RawMessage
	if err = json.Unmarshal(orca["supervised"], &supervised); err != nil {
		t.Fatal(err)
	}
	delete(supervised, "remote_cancel_available")
	orca["supervised"], _ = json.Marshal(supervised)
	config["orca"], _ = json.Marshal(orca)
	withoutPolicy, _ := json.Marshal(config)
	configPath := filepath.Join(t.TempDir(), "steerline.json")
	if err = os.WriteFile(configPath, withoutPolicy, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadConfig(configPath); err == nil || !strings.Contains(err.Error(), "explicitly set to false") {
		t.Fatalf("missing remote cancellation policy error = %v", err)
	}
	supervised["remote_cancel_available"] = json.RawMessage("true")
	orca["supervised"], _ = json.Marshal(supervised)
	config["orca"], _ = json.Marshal(orca)
	withEnabledStop, _ := json.Marshal(config)
	if err = os.WriteFile(configPath, withEnabledStop, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadConfig(configPath); err == nil || !strings.Contains(err.Error(), "fixed unavailable") {
		t.Fatalf("enabled remote cancellation error = %v", err)
	}
}

func isBackendError(err error, want BackendErrorCode) bool {
	var backendErr *BackendError
	return errors.As(err, &backendErr) && backendErr.Code == want
}
