package testprotocol

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

func FakeSupervisedOrca(args []string) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("1.4.209")
		return 0
	}
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "status":
		remote := nativeArg(args, "--environment") != ""
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
		if os.Getenv("STEERLINE_FAKE_REOPEN") == "1" && nativeArg(args, "--environment") == "env-home" && nativeArg(args, "--worktree") == "id:repo-test::worktree" {
			b, _ := json.Marshal(map[string]any{"ok": false, "error": map[string]any{"code": "selector_not_found", "message": "selector_not_found"}, "_meta": map[string]any{"runtimeId": "fake-remote-runtime"}})
			fmt.Println(string(b))
			return 1
		}
		if len(args) < 2 || args[1] != "show" {
			return nativeOrcaErr("invalid_argument", "unexpected worktree verb")
		}
		worktreeID := strings.TrimPrefix(nativeArg(args, "--worktree"), "id:")
		parts := strings.SplitN(worktreeID, "::", 2)
		if len(parts) != 2 || parts[0] != "repo-test" {
			return nativeOrcaErr("worktree_not_found", "unknown worktree")
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
		if len(args) < 2 || args[1] != "show" || nativeArg(args, "--repo") != "id:repo-test" || nativeArg(args, "--environment") != "env-home" {
			return nativeOrcaErr("repo_not_found", "unexpected repository selector")
		}
		return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"repo": map[string]any{"id": "repo-test", "kind": "git", "gitRemoteIdentity": map[string]any{"remoteUrl": os.Getenv("STEERLINE_FAKE_SUPERVISED_REMOTE_URL")}}})
	case "terminal":
		if len(args) < 2 {
			return nativeOrcaErr("invalid_argument", "missing terminal verb")
		}
		if args[1] == "show" && os.Getenv("STEERLINE_FAKE_ACCEPTANCE_TERMINAL") != "" {
			if nativeArg(args, "--environment") != "env-home" || nativeArg(args, "--terminal") != "terminal-acceptance" {
				return nativeOrcaErr("terminal_handle_stale", "wrong terminal identity")
			}
			return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"terminal": map[string]any{"handle": "terminal-acceptance", "incarnationId": os.Getenv("STEERLINE_FAKE_ACCEPTANCE_TERMINAL"), "worktreeId": "repo-test::worktree", "connected": false, "exitCause": map[string]any{"kind": "operator_close"}}})
		}
		if args[1] == "show" && os.Getenv("STEERLINE_FAKE_REOPEN") == "1" && nativeArg(args, "--environment") == "env-home" && nativeArg(args, "--terminal") == "terminal-reopen" {
			return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"terminal": map[string]any{"handle": "terminal-reopen", "incarnationId": "inc-reopen", "worktreeId": "repo-test::worktree", "connected": os.Getenv("STEERLINE_FAKE_REOPEN_TERMINAL_LIVE") == "1", "exitCause": map[string]any{"kind": "operator_close"}}})
		}
		if args[1] == "create" {
			workflow := strings.TrimPrefix(nativeArg(args, "--title"), "SteerLine coordinator ")
			if !strings.HasPrefix(nativeArg(args, "--title"), "SteerLine coordinator ") || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(workflow) || nativeArg(args, "--worktree") == "" {
				return nativeOrcaErr("invalid_argument", "per-Run coordinator identity missing")
			}
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_COORDINATOR_CREATE_LOSS") == "1" {
				return nativeOrcaErr("transport_unavailable", "coordinator create receipt lost")
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"terminal": map[string]any{"handle": "term-" + workflow, "connected": true}})
		}
		if args[1] == "wait" {
			if nativeArg(args, "--terminal") != "terminal-test" || nativeArg(args, "--for") != "tui-idle" || nativeArg(args, "--environment") != "env-home" {
				return nativeOrcaErr("invalid_argument", "prompt readiness wait did not target the exact remote terminal")
			}
			idle := os.Getenv("STEERLINE_FAKE_SUPERVISED_TUI_IDLE") != "0"
			return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"wait": map[string]any{"handle": "terminal-test", "condition": "tui-idle", "satisfied": idle, "status": "running"}})
		}
		if args[1] == "send" {
			if nativeArg(args, "--terminal") != "terminal-test" || nativeArg(args, "--environment") != "env-home" || !strings.Contains(strings.Join(args, " "), "--enter") {
				return nativeOrcaErr("invalid_argument", "prompt Enter did not target the exact remote terminal")
			}
			if marker := os.Getenv("STEERLINE_FAKE_SUPERVISED_ENTER_MARKER"); marker != "" {
				file, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					return nativeOrcaErr("internal", "prompt Enter marker could not be written")
				}
				_, err = file.WriteString("enter\n")
				_ = file.Close()
				if err != nil {
					return nativeOrcaErr("internal", "prompt Enter marker could not be written")
				}
			}
			return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"send": map[string]any{"handle": "terminal-test", "accepted": true}})
		}
		if args[1] != "show" || !strings.HasPrefix(nativeArg(args, "--terminal"), "term-") {
			return nativeOrcaErr("terminal_handle_stale", "coordinator terminal missing")
		}
		return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"terminal": map[string]any{"handle": nativeArg(args, "--terminal"), "connected": true, "orphaned": false}})
	case "orchestration":
		if len(args) < 2 {
			return nativeOrcaErr("invalid_argument", "missing orchestration verb")
		}
		switch args[1] {
		case "run-create":
			if nativeArg(args, "--from") == "term-coordinator" || !strings.HasPrefix(nativeArg(args, "--from"), "term-") || nativeArg(args, "--retry-request") != "" || nativeArg(args, "--objective") == "" {
				return nativeOrcaErr("invalid_argument", "missing Run identity")
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"run": map[string]any{"id": "orca-run-test", "objective": nativeArg(args, "--objective")}, "mutation": map[string]any{"requestId": "request-run-create"}})
		case "run-show":
			coordinator := os.Getenv("STEERLINE_FAKE_SUPERVISED_RUN_COORDINATOR")
			if coordinator == "" {
				coordinator = "term-coordinator"
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"run": map[string]any{"id": nativeArg(args, "--id"), "coordinator_handle": coordinator}})
		case "worker-start":
			if !strings.HasPrefix(nativeArg(args, "--from"), "term-") || nativeArg(args, "--on") != "env-home" || nativeArg(args, "--agent") != "codex" || nativeArg(args, "--model") != "gpt-6-luna" || nativeArg(args, "--effort") != "high" || nativeArg(args, "--retry-request") != "" {
				return nativeOrcaErr("invalid_argument", "launch identity/model was not fixed")
			}
			if marker := os.Getenv("STEERLINE_FAKE_SUPERVISED_START_MARKER"); marker != "" {
				file, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					return nativeOrcaErr("internal", "worker-start marker could not be written")
				}
				_, err = file.WriteString("attempt\n")
				_ = file.Close()
				if err != nil {
					return nativeOrcaErr("internal", "worker-start marker could not be written")
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
			if base := nativeArg(args, "--base-branch"); base != "" {
				if pinned := os.Getenv("STEERLINE_FAKE_SUPERVISED_HEAD"); pinned != "" && base != pinned {
					return nativeOrcaErr("base_mismatch", "fixed base did not match")
				}
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"runId": nativeArg(args, "--run"), "taskId": "task-test", "dispatchId": "dispatch-test", "state": "ready", "stage": "input_accepted", "server": map[string]any{"environmentId": "env-home"}, "launch": map[string]any{"effective": map[string]any{"agent": "codex", "model": "gpt-6-luna", "effort": "high"}}, "worker": map[string]any{"worktreeId": worktree, "effects": []any{map[string]any{"kind": "worktree", "action": "created_top_level", "id": worktree}}}, "mutation": map[string]any{"requestId": "request-worker-start"}})
		case "worker-show":
			if os.Getenv("STEERLINE_FAKE_REOPEN") == "1" {
				return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatch": map[string]any{"id": "dispatch-reopen", "runId": "run-reopen", "taskId": "task-reopen", "status": "completed"}, "worker": map[string]any{"dispatchId": "dispatch-reopen", "state": "succeeded", "worktreeId": "repo-test::worktree", "agentTerminalHandle": "terminal-reopen", "residualResources": []any{}}, "server": map[string]any{"environmentId": "env-home"}, "remoteRuntimeEpoch": "fake-remote-runtime", "observation": map[string]any{"status": "exited", "exactWorker": true}})
			}
			if os.Getenv("STEERLINE_FAKE_ACCEPTANCE_DISPATCH") != "" {
				return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatch": map[string]any{"id": "dispatch-acceptance", "runId": "run-acceptance", "taskId": "task-acceptance", "status": "failed"}, "worker": map[string]any{"dispatchId": "dispatch-acceptance", "state": os.Getenv("STEERLINE_FAKE_ACCEPTANCE_DISPATCH"), "worktreeId": "repo-test::worktree", "agentTerminalHandle": "terminal-acceptance", "residualResources": []any{}}, "server": map[string]any{"environmentId": "env-home"}})
			}
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_OBSERVE_UNAVAILABLE") != "" {
				return nativeOrcaErr("transport_unavailable", "execution host is temporarily unreachable")
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
				return nativeOrcaErr("identity_changed", "settled worker has no live terminal output")
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
				return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatchId": nativeArg(args, "--dispatch"), "source": "terminal", "cursor": "cursor-1", "nextCursor": "cursor-1", "contentComplete": true, "clipping": []string{}, "terminal": map[string]any{"status": "running", "tail": tail}, "status": map[string]any{"worker": "ready", "terminal": "running", "liveness": "live"}})
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatchId": nativeArg(args, "--dispatch"), "source": "transcript", "cursor": "cursor-1", "contentComplete": true, "clipping": []string{}, "status": map[string]any{"worker": "ready", "terminal": "running", "liveness": "live"}, "transcript": map[string]any{"messages": []any{}}})
		case "check":
			coordinator := os.Getenv("STEERLINE_FAKE_SUPERVISED_RUN_COORDINATOR")
			if coordinator == "" {
				coordinator = "term-coordinator"
			}
			if nativeArg(args, "--terminal") != coordinator {
				return nativeOrcaErr("consumer_fenced", "coordinator terminal is bound to another Run")
			}
			if payload := os.Getenv("STEERLINE_FAKE_SUPERVISED_SETTLED_STATUS"); payload != "" {
				if ack := nativeArg(args, "--ack"); ack != "" {
					return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"runId": nativeArg(args, "--run"), "acknowledged": ack, "mutation": map[string]any{"requestId": "request-settled-ack"}})
				}
				return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"runId": nativeArg(args, "--run"), "deliveryId": "delivery-settled", "messages": []any{map[string]any{"id": "status-settled", "type": "status", "from_handle": "dispatch:dispatch-test", "payload": payload}, map[string]any{"id": "done-settled", "type": "worker_done", "from_handle": "dispatch:dispatch-test", "payload": `{"taskId":"task-test","dispatchId":"dispatch-test","outcome":"succeeded"}`}}, "count": 2})
			}
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_DONE_WITHOUT_RESULT") == "1" {
				return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"runId": "orca-run-test", "deliveryId": "delivery-done-without-result", "messages": []any{map[string]any{"id": "done-without-result", "type": "worker_done", "from_handle": "dispatch:dispatch-test", "to_handle": "run:orca-run-test", "body": "worker reported success without the structured result", "payload": map[string]any{"taskId": "task-test", "dispatchId": "dispatch-test", "outcome": "succeeded"}}}, "count": 1})
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"runId": nativeArg(args, "--run"), "deliveryId": nil, "messages": []any{}, "count": 0, "timedOut": true, "connectionLost": false, "cancelled": false})
		case "worker-release":
			if marker := os.Getenv("STEERLINE_FAKE_SUPERVISED_RELEASE_MARKER"); marker != "" {
				file, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					return nativeOrcaErr("internal", "worker-release marker could not be written")
				}
				_, err = file.WriteString("attempt\n")
				_ = file.Close()
				if err != nil {
					return nativeOrcaErr("internal", "worker-release marker could not be written")
				}
			}
			state := os.Getenv("STEERLINE_FAKE_SUPERVISED_RELEASE_STATE")
			if state == "" {
				state = "released"
			}
			reason := os.Getenv("STEERLINE_FAKE_SUPERVISED_RELEASE_REASON")
			residual := []string{}
			if state != "released" && state != "already_released" {
				residual = append(residual, "dispatch:"+nativeArg(args, "--dispatch"))
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatchId": nativeArg(args, "--dispatch"), "state": state, "reason": reason, "processAction": "none", "residualResources": residual, "mutation": map[string]any{"requestId": "request-worker-release"}})
		case "reply":
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_DENY_REPLY") != "" {
				return nativeOrcaErr("unexpected_mutation", "reply was resent during read-only reconciliation")
			}
			coordinator := os.Getenv("STEERLINE_FAKE_SUPERVISED_RUN_COORDINATOR")
			if coordinator == "" {
				coordinator = "term-coordinator"
			}
			if nativeArg(args, "--run") != "orca-run-test" || nativeArg(args, "--from") != coordinator || nativeArg(args, "--id") == "" || nativeArg(args, "--body") == "" {
				return nativeOrcaErr("invalid_argument", "reply lost its original question or Run")
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"message": map[string]any{"from_handle": "run:orca-run-test", "to_handle": "dispatch:dispatch-test"}, "question": map[string]any{"message_id": nativeArg(args, "--id"), "run_id": "orca-run-test", "dispatch_id": "dispatch-test", "answer_body": nativeArg(args, "--body"), "status": "answered"}, "mutation": map[string]any{"requestId": "request-reply"}})
		case "inbox":
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"messages": []any{map[string]any{"id": "answer-message-test", "from_handle": "run:orca-run-test", "to_handle": "dispatch:dispatch-test", "thread_id": "relay-answer-test", "body": "left", "type": "status"}}})
		default:
			return nativeOrcaErr("unknown", "unknown supervised orchestration verb")
		}
	default:
		return nativeOrcaErr("unknown", "unknown supervised Orca test command")
	}
}
