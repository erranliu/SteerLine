package testprotocol

// Package testprotocol contains executable protocol fixtures shared by unit and outer integration tests.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

func sha256Hex(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

type NativeOrcaState struct {
	EnvironmentMode                                                  bool
	EndReadinessOnBusiness                                           bool
	ReadinessMode                                                    string
	HostWait                                                         json.RawMessage
	InboxUnavailable                                                 bool
	BusinessActive                                                   bool
	ProbeCommand, ProbeNonce, ProbeDigest                            string
	Name, Base, Agent, AgentIncarnation, Comment, Path, Branch, Repo string
	CreatedWithAgent, FixedAgentTitle, FixedAgentCommand             string
	FixedAgentCurrentTitle                                           string
	Created, Coordinator                                             bool
	FixedAgentTerminalCreated                                        bool
	RenameFixedAgentAfterCreate, HoldFixedAgentIdentity              bool
	LoseWorktreeOnce                                                 bool
	LoseFixedTerminalOnce                                            bool
	LoseCoordinatorOnce                                              bool
	LoseRunOnce                                                      bool
	Tasks                                                            []string
	Dispatches                                                       int
	FirstBusinessDispatch                                            int
	LoseDispatchAt                                                   int
	LoseTaskAt                                                       int
	EscalationAt                                                     int
	ForceSourceChange                                                bool
	TranscriptAfterSwitch                                            bool
	ReportSourceChanged                                              bool
	Acked                                                            map[string]bool
	LoseAckOnce                                                      bool
	LoseQuestionAckOnce                                              bool
	QuestionAckLossArmed                                             bool
	LoseCloseOnce                                                    bool
	ClosedAgent                                                      bool
	ClosedCoordinator                                                bool
	Replies                                                          int
	ReplyBody                                                        string
	LoseReplyOnce                                                    bool
	Sends                                                            int
	SendSubject                                                      string
	SendBody                                                         string
	LoseSendOnce                                                     bool
	RunObjective                                                     string
	CoordinatorMarker                                                string
	RelayMode, RelayDone, RelayHideDoneOnce                          bool
	PendingPaste                                                     bool
	PasteSubmissions                                                 int
}

func FakeNativeOrca(stateDir string, args []string) int {
	if len(args) == 0 {
		return nativeOrcaErr("invalid_argument", "missing verb")
	}
	unlock, lockErr := lockNativeOrcaState(stateDir)
	if lockErr != nil {
		return nativeOrcaErr("runtime_unavailable", lockErr.Error())
	}
	defer unlock()
	if args[0] == "--version" {
		fmt.Println("1.4.210")
		return 0
	}
	path := filepath.Join(stateDir, "native-orca.json")
	var state NativeOrcaState
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	cwd, _ := os.Getwd()
	if state.Repo == "" {
		state.Repo = cwd
	}
	if state.Acked == nil {
		state.Acked = map[string]bool{}
	}
	save := func() { data, _ := json.Marshal(state); _ = nativeAtomicWrite(path, data) }
	save()
	firstBusinessDispatch := state.FirstBusinessDispatch
	if firstBusinessDispatch == 0 {
		firstBusinessDispatch = 1
	}
	worktreeID := func() string { return "fake-repo::" + filepath.ToSlash(state.Path) }
	worktree := func() map[string]any {
		return map[string]any{"id": worktreeID(), "repoId": "fake-repo", "path": filepath.ToSlash(state.Path), "head": state.Base, "baseRef": state.Base, "branch": "refs/heads/" + state.Branch, "displayName": state.Name, "comment": state.Comment, "createdWithAgent": state.CreatedWithAgent}
	}
	terminal := func(handle string) map[string]any {
		if handle == "term-agent" {
			incarnation := state.AgentIncarnation
			if incarnation == "" {
				incarnation = "inc-agent"
			}
			title, agent := state.FixedAgentTitle, state.Agent
			if state.FixedAgentCurrentTitle != "" {
				title = state.FixedAgentCurrentTitle
			}
			if state.HoldFixedAgentIdentity {
				agent = ""
			}
			return map[string]any{"handle": handle, "title": title, "incarnationId": incarnation, "worktreeId": worktreeID(), "worktreePath": filepath.ToSlash(state.Path), "branch": "refs/heads/" + state.Branch, "connected": true, "agentIdentity": agent, "agentWait": state.HostWait}
		}
		return map[string]any{"handle": handle, "incarnationId": "inc-coordinator", "worktreeId": "fake-repo::" + filepath.ToSlash(state.Repo), "worktreePath": filepath.ToSlash(state.Repo), "branch": "refs/heads/main", "connected": true}
	}
	if args[0] == "status" {
		return nativeOrcaOK(map[string]any{"runtime": map[string]any{"state": "ready"}})
	}
	if len(args) > 1 && args[0] == "repo" && args[1] == "list" {
		return nativeOrcaOK(map[string]any{"repos": []any{map[string]any{"id": "fake-repo", "path": filepath.ToSlash(cwd), "kind": "git"}}})
	}
	if len(args) > 1 && args[0] == "worktree" {
		switch args[1] {
		case "list":
			rows := []any{}
			if state.Created {
				rows = append(rows, worktree())
			}
			return nativeOrcaOK(map[string]any{"worktrees": rows, "truncated": false})
		case "create":
			if state.Created {
				return nativeOrcaErr("name_conflict", "duplicate worktree")
			}
			state.Name, state.Base, state.Agent, state.Comment = nativeArg(args, "--name"), nativeArg(args, "--base-branch"), nativeArg(args, "--agent"), nativeArg(args, "--comment")
			state.CreatedWithAgent = state.Agent
			state.Path, state.Branch = filepath.Join(stateDir, "worker"), "fake/"+state.Name
			if state.Name == "" || state.Base == "" || os.MkdirAll(filepath.Dir(state.Path), 0700) != nil {
				return nativeOrcaErr("invalid_argument", "bad worktree")
			}
			if out, err := exec.Command("git", "-C", state.Repo, "worktree", "add", "-b", state.Branch, state.Path, state.Base).CombinedOutput(); err != nil {
				return nativeOrcaErr("invalid_argument", "cannot create test linked worktree: "+string(out))
			}
			state.Created = true
			lost := state.LoseWorktreeOnce
			state.LoseWorktreeOnce = false
			save()
			if lost {
				return nativeOrcaErr("runtime_unavailable", "worktree and agent were created but response was lost")
			}
			return nativeOrcaOK(map[string]any{"worktree": worktree(), "agentTerminalHandle": "term-agent"})
		}
	}
	if len(args) > 1 && args[0] == "terminal" {
		switch args[1] {
		case "create":
			if strings.Contains(nativeArg(args, "--worktree"), "worker") {
				if state.FixedAgentTerminalCreated || !state.Created || state.CreatedWithAgent != "" || nativeArg(args, "--shell") != "powershell.exe" {
					return nativeOrcaErr("invalid_argument", "bad fixed agent terminal")
				}
				state.FixedAgentTerminalCreated = true
				state.Agent = "codex"
				state.FixedAgentTitle, state.FixedAgentCommand = nativeArg(args, "--title"), nativeArg(args, "--command")
				created := terminal("term-agent")
				if state.RenameFixedAgentAfterCreate {
					state.FixedAgentCurrentTitle = state.Name
				}
				lost := state.LoseFixedTerminalOnce
				state.LoseFixedTerminalOnce = false
				save()
				if lost {
					return nativeOrcaErr("runtime_unavailable", "agent terminal created but response was lost")
				}
				return nativeOrcaOK(map[string]any{"terminal": created})
			}
			state.Coordinator = true
			state.CoordinatorMarker = strings.TrimPrefix(nativeArg(args, "--command"), "echo ")
			lost := state.LoseCoordinatorOnce
			state.LoseCoordinatorOnce = false
			save()
			if lost {
				return nativeOrcaErr("runtime_unavailable", "coordinator was created but response was lost")
			}
			return nativeOrcaOK(map[string]any{"terminal": map[string]any{"handle": "term-coordinator"}})
		case "show":
			handle := nativeArg(args, "--terminal")
			if handle == "term-agent" && state.Created && (state.CreatedWithAgent != "" || state.FixedAgentTerminalCreated) && !state.ClosedAgent || handle == "term-coordinator" && state.Coordinator && !state.ClosedCoordinator {
				return nativeOrcaOK(map[string]any{"terminal": terminal(handle)})
			}
			return nativeOrcaErr("terminal_not_found", "missing terminal")
		case "list":
			rows := []any{}
			if strings.Contains(nativeArg(args, "--worktree"), "worker") && state.Created && (state.CreatedWithAgent != "" || state.FixedAgentTerminalCreated) && !state.ClosedAgent {
				rows = append(rows, terminal("term-agent"))
			}
			if strings.Contains(nativeArg(args, "--worktree"), filepath.ToSlash(state.Repo)) && state.Coordinator && !state.ClosedCoordinator {
				rows = append(rows, terminal("term-coordinator"))
			}
			return nativeOrcaOK(map[string]any{"terminals": rows, "truncated": false})
		case "read":
			if (state.EnvironmentMode || state.FirstBusinessDispatch > 1) && nativeArg(args, "--terminal") == "term-agent" {
				return nativeOrcaOK(map[string]any{"terminal": map[string]any{"handle": "term-agent", "source": "screen", "tail": []string{"› Ask Codex to do anything", "? for shortcuts"}}})
			}
			if nativeArg(args, "--terminal") == "term-agent" && state.PendingPaste {
				return nativeOrcaOK(map[string]any{"terminal": map[string]any{"handle": "term-agent", "source": "screen", "tail": []string{"› [Pasted Content 6859 chars]"}}})
			}
			return nativeOrcaOK(map[string]any{"terminal": map[string]any{"handle": nativeArg(args, "--terminal"), "tail": []string{state.CoordinatorMarker}}})
		case "send":
			if nativeArg(args, "--terminal") != "term-agent" || !state.PendingPaste || !slices.Contains(args, "--enter") {
				return nativeOrcaErr("invalid_argument", "unexpected prompt submission")
			}
			state.PendingPaste = false
			state.PasteSubmissions++
			save()
			return nativeOrcaOK(map[string]any{"send": map[string]any{"handle": "term-agent", "accepted": true, "bytesWritten": 1}})
		case "close":
			handle := nativeArg(args, "--terminal")
			if handle == "term-agent" && !state.ClosedAgent {
				state.ClosedAgent = true
			} else if handle == "term-coordinator" && !state.ClosedCoordinator {
				state.ClosedCoordinator = true
			} else {
				return nativeOrcaErr("terminal_not_found", "terminal already closed")
			}
			lost := state.LoseCloseOnce
			state.LoseCloseOnce = false
			save()
			if lost {
				return nativeOrcaErr("runtime_unavailable", "terminal closed but response was lost")
			}
			return nativeOrcaOK(map[string]any{"close": map[string]any{"handle": handle, "ptyKilled": true}})
		}
	}
	if len(args) > 1 && args[0] == "orchestration" {
		switch args[1] {
		case "run-create":
			state.RunObjective = nativeArg(args, "--objective")
			lost := state.LoseRunOnce
			state.LoseRunOnce = false
			save()
			if lost {
				return nativeOrcaErr("runtime_unavailable", "Run was created but response was lost")
			}
			return nativeOrcaOK(map[string]any{"run": map[string]any{"id": "run-fake"}, "mutation": map[string]any{"requestId": "request-run"}})
		case "reply":
			if nativeArg(args, "--id") != "msg-ask-1" || state.Dispatches != firstBusinessDispatch {
				return nativeOrcaErr("invalid_argument", "wrong question")
			}
			state.Replies++
			state.ReplyBody = nativeArg(args, "--body")
			lost := state.LoseReplyOnce
			state.LoseReplyOnce = false
			save()
			if lost {
				return nativeOrcaErr("runtime_unavailable", "reply was accepted but response was lost")
			}
			return nativeOrcaOK(map[string]any{"question": map[string]any{"message_id": "msg-ask-1", "dispatch_id": fmt.Sprintf("ctx-%d", firstBusinessDispatch), "answer_body": nativeArg(args, "--body")}, "mutation": map[string]any{"requestId": "request-reply-1"}})
		case "inbox":
			feedbackDispatch := state.EscalationAt
			if state.EnvironmentMode {
				feedbackDispatch = state.Dispatches
			}
			messages := []any{}
			if state.Replies > 0 {
				messages = append(messages, map[string]any{"id": "msg-reply-1", "run_id": "run-fake", "from_handle": "run:run-fake", "to_handle": fmt.Sprintf("dispatch:ctx-%d", firstBusinessDispatch), "thread_id": "msg-ask-1", "body": state.ReplyBody, "type": "status"})
			}
			if state.Sends > 0 {
				messages = append(messages, map[string]any{"id": "msg-send-1", "run_id": "run-fake", "from_handle": "run:run-fake", "to_handle": fmt.Sprintf("dispatch:ctx-%d", feedbackDispatch), "subject": state.SendSubject, "body": state.SendBody, "type": "status"})
			}
			return nativeOrcaOK(map[string]any{"messages": messages, "count": len(messages)})
		case "send":
			if state.RelayMode && nativeArg(args, "--type") == "worker_done" {
				if nativeArg(args, "--from") != "term-agent" || nativeArg(args, "--run") != "run-fake" || nativeArg(args, "--to") != "run:run-fake" || nativeArg(args, "--task-id") != "task-1" || nativeArg(args, "--dispatch-id") != "ctx-1" || nativeArg(args, "--outcome") != "failed" || nativeArg(args, "--dispatch-capability") != "dcap_fake" {
					return nativeOrcaErr("dispatch_capability_invalid", "relay did not present the fixed Dispatch capability")
				}
				state.RelayDone = true
				save()
				return nativeOrcaOK(map[string]any{"message": map[string]any{"id": "msg-relay-1"}, "mutation": map[string]any{"requestId": "request-relay-1"}})
			}
			feedbackDispatch := state.EscalationAt
			if state.EnvironmentMode {
				feedbackDispatch = state.Dispatches
			}
			if nativeArg(args, "--to") != fmt.Sprintf("dispatch:ctx-%d", feedbackDispatch) || nativeArg(args, "--run") != "run-fake" {
				return nativeOrcaErr("invalid_argument", "wrong active Dispatch")
			}
			state.Sends++
			state.SendSubject, state.SendBody = nativeArg(args, "--subject"), nativeArg(args, "--body")
			lost := state.LoseSendOnce
			state.LoseSendOnce = false
			save()
			if lost {
				return nativeOrcaErr("runtime_unavailable", "send was accepted but response was lost")
			}
			return nativeOrcaOK(map[string]any{"message": map[string]any{"id": fmt.Sprintf("msg-send-%d", state.Sends), "to_handle": nativeArg(args, "--to")}, "mutation": map[string]any{"requestId": fmt.Sprintf("request-send-%d", state.Sends)}})
		case "run-list":
			return nativeOrcaOK(map[string]any{"runs": []any{map[string]any{"id": "run-fake", "objective": state.RunObjective, "coordinator_handle": "term-coordinator"}}})
		case "task-create":
			spec := nativeArg(args, "--spec")
			state.Tasks = append(state.Tasks, spec)
			if state.EnvironmentMode && state.EndReadinessOnBusiness && !strings.HasPrefix(spec, "Readiness preparation only.") {
				state.EnvironmentMode = false
				state.BusinessActive = false
			}
			save()
			n := len(state.Tasks)
			if n == state.LoseTaskAt {
				return nativeOrcaErr("runtime_unavailable", "Task was created but response was lost")
			}
			return nativeOrcaOK(map[string]any{"task": map[string]any{"id": fmt.Sprintf("task-%d", n)}, "mutation": map[string]any{"requestId": fmt.Sprintf("request-task-%d", n)}})
		case "task-list":
			rows := []any{}
			for i, spec := range state.Tasks {
				rows = append(rows, map[string]any{"id": fmt.Sprintf("task-%d", i+1), "run_id": "run-fake", "spec": spec, "created_by_terminal_handle": "term-coordinator"})
			}
			return nativeOrcaOK(map[string]any{"runId": "run-fake", "tasks": rows, "count": len(rows)})
		case "dispatch":
			if nativeArg(args, "--to") != "term-agent" || nativeArg(args, "--task") != fmt.Sprintf("task-%d", len(state.Tasks)) {
				return nativeOrcaErr("invalid_argument", "wrong target")
			}
			state.Dispatches++
			if state.EnvironmentMode && strings.HasPrefix(state.Tasks[len(state.Tasks)-1], "Readiness preparation only.") {
				_, rest, _ := strings.Cut(state.Tasks[len(state.Tasks)-1], "command once in the original worktree: ")
				state.ProbeCommand = strings.SplitN(rest, "\n", 2)[0]
				_, probeFile, _ := strings.Cut(state.ProbeCommand, "-File ")
				probeDir := filepath.Dir(filepath.Join(state.Path, strings.Trim(probeFile, "'")))
				data, err := os.ReadFile(filepath.Join(probeDir, "input.json"))
				if err != nil {
					return nativeOrcaErr("invalid_argument", err.Error())
				}
				var input map[string]any
				_ = json.Unmarshal(data, &input)
				state.ProbeNonce = input["nonce"].(string)
				script, _ := os.ReadFile(filepath.Join(probeDir, "probe.ps1"))
				state.ProbeDigest = sha256Hex(script)
				input["input_sha256"], input["git_head"] = sha256Hex(data), state.Base
				input["read"], input["write"], input["delete"] = true, true, true
				if state.ReadinessMode != "tool_failure" && state.ReadinessMode != "missing_receipt" {
					receipt, _ := json.Marshal(input)
					_ = os.WriteFile(filepath.Join(probeDir, "receipt.json"), receipt, 0600)
				}
			}
			save()
			n := state.Dispatches
			if n == state.LoseDispatchAt {
				return nativeOrcaErr("runtime_unavailable", "Dispatch was injected but the response was lost")
			}
			result := map[string]any{"dispatch": map[string]any{"id": fmt.Sprintf("ctx-%d", n), "task_id": fmt.Sprintf("task-%d", n), "run_id": "run-fake"}, "mutation": map[string]any{"requestId": fmt.Sprintf("request-dispatch-%d", n)}}
			if state.PendingPaste {
				result["prompt"] = map[string]any{"provider": "codex", "processIncarnation": "inc-agent", "stages": []string{"input_accepted"}}
			}
			return nativeOrcaOK(result)
		case "dispatch-show":
			n, _ := strconv.Atoi(strings.TrimPrefix(nativeArg(args, "--task"), "task-"))
			if n == 0 || n > state.Dispatches {
				return nativeOrcaErr("dispatch_not_found", "no dispatch")
			}
			return nativeOrcaOK(map[string]any{"dispatch": map[string]any{"id": fmt.Sprintf("ctx-%d", n), "task_id": fmt.Sprintf("task-%d", n), "run_id": "run-fake", "assignee_handle": "term-agent", "process_incarnation": "pty:inc-agent"}})
		case "check":
			if ack := nativeArg(args, "--ack"); ack != "" {
				state.Acked[ack] = true
				lost := state.LoseAckOnce
				state.LoseAckOnce = false
				save()
				if lost {
					return nativeOrcaErr("runtime_unavailable", "Delivery was acknowledged but response was lost")
				}
				return nativeOrcaOK(map[string]any{"runId": "run-fake", "acknowledged": ack, "mutation": map[string]any{"requestId": "request-ack-" + ack}})
			}
			if state.EnvironmentMode {
				if state.InboxUnavailable {
					return nativeOrcaErr("runtime_unavailable", "inbox observation unavailable")
				}
				n := state.Dispatches
				if n == 0 {
					return nativeOrcaOK(map[string]any{"runId": "run-fake", "messages": []any{}})
				}
				readiness := strings.HasPrefix(state.Tasks[n-1], "Readiness preparation only.")
				id := fmt.Sprintf("env-delivery-%d", n)
				if state.Acked[id] || readiness && (state.ReadinessMode == "hold" || state.ReadinessMode == "tool_failure") {
					return nativeOrcaOK(map[string]any{"runId": "run-fake", "messages": []any{}})
				}
				kind, outcome := "worker_done", "succeeded"
				if !readiness && state.BusinessActive {
					kind = "status"
				}
				if readiness && state.ReadinessMode == "failed_settlement" {
					outcome = "failed"
				}
				payload, _ := json.Marshal(map[string]any{"taskId": fmt.Sprintf("task-%d", n), "dispatchId": fmt.Sprintf("ctx-%d", n), "outcome": outcome})
				message := map[string]any{"id": fmt.Sprintf("env-message-%d", n), "from_handle": "term-agent", "type": kind, "body": "environment fixture observation", "payload": string(payload)}
				return nativeOrcaOK(map[string]any{"runId": "run-fake", "deliveryId": id, "messages": []any{message}})
			}
			if state.RelayMode {
				if state.RelayDone && state.RelayHideDoneOnce {
					state.RelayHideDoneOnce = false
					save()
					return nativeOrcaOK(map[string]any{"runId": "run-fake", "messages": []any{}})
				}
				if state.RelayDone && !state.Acked["delivery-relay-1"] {
					payload, _ := json.Marshal(map[string]any{"taskId": "task-1", "dispatchId": "ctx-1", "outcome": "failed"})
					message := map[string]any{"id": "msg-relay-1", "from_handle": "term-agent", "type": "worker_done", "body": "Worker failed before publication", "payload": string(payload)}
					return nativeOrcaOK(map[string]any{"runId": "run-fake", "deliveryId": "delivery-relay-1", "messages": []any{message}})
				}
				return nativeOrcaOK(map[string]any{"runId": "run-fake", "messages": []any{}})
			}
			if state.Dispatches == firstBusinessDispatch && state.Replies == 0 && !state.Acked["delivery-ask-1"] {
				if state.LoseQuestionAckOnce && !state.QuestionAckLossArmed {
					state.QuestionAckLossArmed, state.LoseAckOnce = true, true
					save()
				}
				payload, _ := json.Marshal(map[string]any{"taskId": fmt.Sprintf("task-%d", firstBusinessDispatch), "dispatchId": fmt.Sprintf("ctx-%d", firstBusinessDispatch), "question": "Which approach?", "options": []string{"Direct path"}})
				message := map[string]any{"id": "msg-ask-1", "from_handle": fmt.Sprintf("dispatch:ctx-%d", firstBusinessDispatch), "type": "question", "body": "Which approach?", "payload": string(payload)}
				return nativeOrcaOK(map[string]any{"runId": "run-fake", "deliveryId": "delivery-ask-1", "messages": []any{message}})
			}
			if state.Dispatches == state.EscalationAt && state.Sends == 0 && !state.Acked[fmt.Sprintf("delivery-escalation-%d", state.EscalationAt)] {
				n := state.Dispatches
				message := map[string]any{"id": fmt.Sprintf("msg-escalation-%d", n), "from_handle": fmt.Sprintf("dispatch:ctx-%d", n), "type": "escalation", "subject": "Need more guidance", "body": "Send guidance to the current Dispatch"}
				return nativeOrcaOK(map[string]any{"runId": "run-fake", "deliveryId": fmt.Sprintf("delivery-escalation-%d", n), "messages": []any{message}})
			}
			if state.Dispatches > 0 && (state.Dispatches > firstBusinessDispatch || state.Replies > 0) && (state.Dispatches != state.EscalationAt || state.Sends > 0) && !state.Acked[fmt.Sprintf("delivery-done-%d", state.Dispatches)] {
				n := state.Dispatches
				payload, _ := json.Marshal(map[string]any{"taskId": fmt.Sprintf("task-%d", n), "dispatchId": fmt.Sprintf("ctx-%d", n), "outcome": "succeeded"})
				message := map[string]any{"id": fmt.Sprintf("msg-%d", n), "from_handle": "term-agent", "type": "worker_done", "body": fmt.Sprintf("DONE %d", n), "payload": string(payload)}
				return nativeOrcaOK(map[string]any{"runId": "run-fake", "deliveryId": fmt.Sprintf("delivery-done-%d", n), "messages": []any{message}})
			}
			return nativeOrcaOK(map[string]any{"runId": "run-fake", "messages": []any{}})
		case "worker-read":
			n, _ := strconv.Atoi(strings.TrimPrefix(nativeArg(args, "--dispatch"), "ctx-"))
			if state.EnvironmentMode && n > 0 && strings.HasPrefix(state.Tasks[n-1], "Readiness preparation only.") {
				invocation, _ := json.Marshal(map[string]any{"cmd": state.ProbeCommand})
				blocks := []any{map[string]any{"id": "call", "role": "assistant", "source": "transcript", "blocks": []any{map[string]any{"type": "tool-call", "name": "exec_command", "callId": "probe-call", "input": string(invocation)}}}}
				result, _ := json.Marshal(map[string]any{"exit_code": 0, "output": "SLREADINESS:" + state.ProbeNonce + ":" + state.ProbeDigest})
				resultText := string(result)
				if state.ReadinessMode == "tool_failure" {
					resultText = "helper_unknown_error: setup refresh had errors"
				}
				if state.ReadinessMode == "oral" {
					blocks = []any{map[string]any{"role": "assistant", "source": "transcript", "blocks": []any{map[string]any{"type": "text", "text": "OK"}}}}
				} else {
					blocks = append(blocks, map[string]any{"id": "result", "role": "tool", "source": "transcript", "blocks": []any{map[string]any{"type": "tool-result", "output": resultText}}})
				}
				return nativeOrcaOK(map[string]any{"dispatchId": nativeArg(args, "--dispatch"), "source": "transcript", "provider": "codex", "sourceIdentity": "probe-source", "sourceExact": true, "contentComplete": true, "nextCursor": "probe-cursor", "transcript": map[string]any{"messages": blocks}})
			}
			if nativeArg(args, "--cursor") != "" && state.ForceSourceChange {
				state.ForceSourceChange = false
				state.TranscriptAfterSwitch = true
				state.ReportSourceChanged = true
				save()
				return nativeOrcaErr("source_changed", "worker-read source_changed; restart without cursor")
			}
			if state.TranscriptAfterSwitch {
				changed := state.ReportSourceChanged
				state.ReportSourceChanged = false
				save()
				return nativeOrcaOK(map[string]any{"dispatchId": nativeArg(args, "--dispatch"), "source": "transcript", "nextCursor": fmt.Sprintf("transcript-cursor-%d", n), "contentComplete": true, "sourceChanged": changed, "transcript": map[string]any{"text": fmt.Sprintf("complete fake output %d", n)}})
			}
			return nativeOrcaOK(map[string]any{"dispatchId": nativeArg(args, "--dispatch"), "source": "terminal", "cursor": fmt.Sprintf("cursor-%d", n), "contentComplete": false, "clipping": []string{"terminal_fallback"}, "fallbackReason": "session_not_reported", "terminal": map[string]any{"tail": []string{fmt.Sprintf("fake output %d", n)}}})
		case "worker-show":
			n, _ := strconv.Atoi(strings.TrimPrefix(nativeArg(args, "--dispatch"), "ctx-"))
			status := "completed"
			if !state.EnvironmentMode && (n == firstBusinessDispatch && state.Replies == 0 || state.EscalationAt > 0 && n == state.EscalationAt && state.Sends == 0) {
				status = "dispatched"
			}
			if state.EnvironmentMode && n > 0 {
				if strings.HasPrefix(state.Tasks[n-1], "Readiness preparation only.") {
					if state.ReadinessMode == "hold" || state.ReadinessMode == "tool_failure" {
						status = "dispatched"
					}
					if state.ReadinessMode == "failed_settlement" {
						status = "failed"
					}
				} else if state.BusinessActive {
					status = "dispatched"
				}
			}
			if state.RelayMode {
				status = "dispatched"
				if state.RelayDone {
					status = "failed"
				}
			}
			return nativeOrcaOK(map[string]any{"dispatch": map[string]any{"id": fmt.Sprintf("ctx-%d", n), "runId": "run-fake", "taskId": fmt.Sprintf("task-%d", n), "status": status}, "worker": map[string]any{"dispatchId": fmt.Sprintf("ctx-%d", n), "state": "unsupervised", "stage": "injected", "agentTerminalHandle": "term-agent"}, "terminal": terminal("term-agent")})
		}
	}
	return nativeOrcaErr("invalid_argument", "unhandled native Orca fixture command: "+strings.Join(args, " "))
}

func lockNativeOrcaState(stateDir string) (func(), error) {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(stateDir, "native-orca.lock")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out waiting for native Orca fixture state lock")
}

func nativeArg(args []string, key string) string {
	for i, value := range args {
		if value == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func nativeAtomicWrite(path string, value []byte) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, value, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func nativeOrcaOK(result map[string]any) int {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "result": result, "_meta": map[string]any{"runtimeId": "fake-runtime"}})
	return 0
}

func nativeOrcaErr(code, message string) int {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": false, "error": map[string]any{"code": code, "message": message}})
	return 1
}

func ReadNativeOrcaState(stateDir string) (NativeOrcaState, error) {
	var state NativeOrcaState
	raw, err := os.ReadFile(filepath.Join(stateDir, "native-orca.json"))
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(raw, &state)
	return state, err
}

func WriteNativeOrcaState(stateDir string, state NativeOrcaState) error {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return nativeAtomicWrite(filepath.Join(stateDir, "native-orca.json"), raw)
}
