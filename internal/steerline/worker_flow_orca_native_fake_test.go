package steerline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type fakeNativeOrcaState struct {
	Name, Base, Agent, AgentIncarnation, Comment, Path, Branch, Repo string
	CreatedWithAgent, FixedAgentTitle, FixedAgentCommand             string
	Created, Coordinator                                             bool
	FixedAgentTerminalCreated                                        bool
	LoseWorktreeOnce                                                 bool
	LoseFixedTerminalOnce                                            bool
	LoseCoordinatorOnce                                              bool
	LoseRunOnce                                                      bool
	Tasks                                                            []string
	Dispatches                                                       int
	LoseDispatchAt                                                   int
	LoseTaskAt                                                       int
	EscalationAt                                                     int
	ForceSourceChange                                                bool
	TranscriptAfterSwitch                                            bool
	ReportSourceChanged                                              bool
	Acked                                                            map[string]bool
	LoseAckOnce                                                      bool
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
}

func fakeNativeOrca(stateDir string, args []string) int {
	if len(args) == 0 {
		return fakeOrcaErr("invalid_argument", "missing verb")
	}
	if args[0] == "--version" {
		fmt.Println("1.4.210")
		return 0
	}
	path := filepath.Join(stateDir, "native-orca.json")
	var state fakeNativeOrcaState
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
	save := func() { data, _ := json.Marshal(state); _ = AtomicWrite(path, data) }
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
			return map[string]any{"handle": handle, "title": state.FixedAgentTitle, "incarnationId": incarnation, "worktreeId": worktreeID(), "worktreePath": filepath.ToSlash(state.Path), "branch": "refs/heads/" + state.Branch, "connected": true, "agentIdentity": state.Agent}
		}
		return map[string]any{"handle": handle, "incarnationId": "inc-coordinator", "worktreeId": "fake-repo::" + filepath.ToSlash(state.Repo), "worktreePath": filepath.ToSlash(state.Repo), "branch": "refs/heads/main", "connected": true}
	}
	if args[0] == "status" {
		return fakeOrcaOK(map[string]any{"runtime": map[string]any{"state": "ready"}})
	}
	if len(args) > 1 && args[0] == "repo" && args[1] == "list" {
		return fakeOrcaOK(map[string]any{"repos": []any{map[string]any{"id": "fake-repo", "path": filepath.ToSlash(cwd), "kind": "git"}}})
	}
	if len(args) > 1 && args[0] == "worktree" {
		switch args[1] {
		case "list":
			rows := []any{}
			if state.Created {
				rows = append(rows, worktree())
			}
			return fakeOrcaOK(map[string]any{"worktrees": rows, "truncated": false})
		case "create":
			if state.Created {
				return fakeOrcaErr("name_conflict", "duplicate worktree")
			}
			state.Name, state.Base, state.Agent, state.Comment = arg(args, "--name"), arg(args, "--base-branch"), arg(args, "--agent"), arg(args, "--comment")
			state.CreatedWithAgent = state.Agent
			state.Path, state.Branch = filepath.Join(stateDir, "worker"), "fake/"+state.Name
			if state.Name == "" || state.Base == "" || os.MkdirAll(state.Path, 0700) != nil {
				return fakeOrcaErr("invalid_argument", "bad worktree")
			}
			state.Created = true
			lost := state.LoseWorktreeOnce
			state.LoseWorktreeOnce = false
			save()
			if lost {
				return fakeOrcaErr("runtime_unavailable", "worktree and agent were created but response was lost")
			}
			return fakeOrcaOK(map[string]any{"worktree": worktree(), "agentTerminalHandle": "term-agent"})
		}
	}
	if len(args) > 1 && args[0] == "terminal" {
		switch args[1] {
		case "create":
			if strings.Contains(arg(args, "--worktree"), "worker") {
				if state.FixedAgentTerminalCreated || !state.Created || state.CreatedWithAgent != "" || arg(args, "--shell") != "powershell.exe" {
					return fakeOrcaErr("invalid_argument", "bad fixed agent terminal")
				}
				state.FixedAgentTerminalCreated = true
				state.Agent = "codex"
				state.FixedAgentTitle, state.FixedAgentCommand = arg(args, "--title"), arg(args, "--command")
				lost := state.LoseFixedTerminalOnce
				state.LoseFixedTerminalOnce = false
				save()
				if lost {
					return fakeOrcaErr("runtime_unavailable", "agent terminal created but response was lost")
				}
				return fakeOrcaOK(map[string]any{"terminal": terminal("term-agent")})
			}
			state.Coordinator = true
			state.CoordinatorMarker = strings.TrimPrefix(arg(args, "--command"), "echo ")
			lost := state.LoseCoordinatorOnce
			state.LoseCoordinatorOnce = false
			save()
			if lost {
				return fakeOrcaErr("runtime_unavailable", "coordinator was created but response was lost")
			}
			return fakeOrcaOK(map[string]any{"terminal": map[string]any{"handle": "term-coordinator"}})
		case "show":
			handle := arg(args, "--terminal")
			if handle == "term-agent" && state.Created && (state.CreatedWithAgent != "" || state.FixedAgentTerminalCreated) && !state.ClosedAgent || handle == "term-coordinator" && state.Coordinator && !state.ClosedCoordinator {
				return fakeOrcaOK(map[string]any{"terminal": terminal(handle)})
			}
			return fakeOrcaErr("terminal_not_found", "missing terminal")
		case "list":
			rows := []any{}
			if strings.Contains(arg(args, "--worktree"), "worker") && state.Created && (state.CreatedWithAgent != "" || state.FixedAgentTerminalCreated) && !state.ClosedAgent {
				rows = append(rows, terminal("term-agent"))
			}
			if strings.Contains(arg(args, "--worktree"), filepath.ToSlash(state.Repo)) && state.Coordinator && !state.ClosedCoordinator {
				rows = append(rows, terminal("term-coordinator"))
			}
			return fakeOrcaOK(map[string]any{"terminals": rows, "truncated": false})
		case "read":
			return fakeOrcaOK(map[string]any{"terminal": map[string]any{"handle": arg(args, "--terminal"), "tail": []string{state.CoordinatorMarker}}})
		case "close":
			handle := arg(args, "--terminal")
			if handle == "term-agent" && !state.ClosedAgent {
				state.ClosedAgent = true
			} else if handle == "term-coordinator" && !state.ClosedCoordinator {
				state.ClosedCoordinator = true
			} else {
				return fakeOrcaErr("terminal_not_found", "terminal already closed")
			}
			lost := state.LoseCloseOnce
			state.LoseCloseOnce = false
			save()
			if lost {
				return fakeOrcaErr("runtime_unavailable", "terminal closed but response was lost")
			}
			return fakeOrcaOK(map[string]any{"close": map[string]any{"handle": handle, "ptyKilled": true}})
		}
	}
	if len(args) > 1 && args[0] == "orchestration" {
		switch args[1] {
		case "run-create":
			state.RunObjective = arg(args, "--objective")
			lost := state.LoseRunOnce
			state.LoseRunOnce = false
			save()
			if lost {
				return fakeOrcaErr("runtime_unavailable", "Run was created but response was lost")
			}
			return fakeOrcaOK(map[string]any{"run": map[string]any{"id": "run-fake"}, "mutation": map[string]any{"requestId": "request-run"}})
		case "reply":
			if arg(args, "--id") != "msg-ask-1" || state.Dispatches != 1 {
				return fakeOrcaErr("invalid_argument", "wrong question")
			}
			state.Replies++
			state.ReplyBody = arg(args, "--body")
			lost := state.LoseReplyOnce
			state.LoseReplyOnce = false
			save()
			if lost {
				return fakeOrcaErr("runtime_unavailable", "reply was accepted but response was lost")
			}
			return fakeOrcaOK(map[string]any{"question": map[string]any{"message_id": "msg-ask-1", "dispatch_id": "ctx-1", "answer_body": arg(args, "--body")}, "mutation": map[string]any{"requestId": "request-reply-1"}})
		case "inbox":
			messages := []any{}
			if state.Replies > 0 {
				messages = append(messages, map[string]any{"id": "msg-reply-1", "run_id": "run-fake", "from_handle": "run:run-fake", "to_handle": "dispatch:ctx-1", "thread_id": "msg-ask-1", "body": state.ReplyBody, "type": "status"})
			}
			if state.Sends > 0 {
				messages = append(messages, map[string]any{"id": "msg-send-1", "run_id": "run-fake", "from_handle": "run:run-fake", "to_handle": fmt.Sprintf("dispatch:ctx-%d", state.EscalationAt), "subject": state.SendSubject, "body": state.SendBody, "type": "status"})
			}
			return fakeOrcaOK(map[string]any{"messages": messages, "count": len(messages)})
		case "send":
			if arg(args, "--to") != fmt.Sprintf("dispatch:ctx-%d", state.EscalationAt) || arg(args, "--run") != "run-fake" {
				return fakeOrcaErr("invalid_argument", "wrong active Dispatch")
			}
			state.Sends++
			state.SendSubject, state.SendBody = arg(args, "--subject"), arg(args, "--body")
			lost := state.LoseSendOnce
			state.LoseSendOnce = false
			save()
			if lost {
				return fakeOrcaErr("runtime_unavailable", "send was accepted but response was lost")
			}
			return fakeOrcaOK(map[string]any{"message": map[string]any{"id": fmt.Sprintf("msg-send-%d", state.Sends), "to_handle": arg(args, "--to")}, "mutation": map[string]any{"requestId": fmt.Sprintf("request-send-%d", state.Sends)}})
		case "run-list":
			return fakeOrcaOK(map[string]any{"runs": []any{map[string]any{"id": "run-fake", "objective": state.RunObjective, "coordinator_handle": "term-coordinator"}}})
		case "task-create":
			state.Tasks = append(state.Tasks, arg(args, "--spec"))
			save()
			n := len(state.Tasks)
			if n == state.LoseTaskAt {
				return fakeOrcaErr("runtime_unavailable", "Task was created but response was lost")
			}
			return fakeOrcaOK(map[string]any{"task": map[string]any{"id": fmt.Sprintf("task-%d", n)}, "mutation": map[string]any{"requestId": fmt.Sprintf("request-task-%d", n)}})
		case "task-list":
			rows := []any{}
			for i, spec := range state.Tasks {
				rows = append(rows, map[string]any{"id": fmt.Sprintf("task-%d", i+1), "run_id": "run-fake", "spec": spec, "created_by_terminal_handle": "term-coordinator"})
			}
			return fakeOrcaOK(map[string]any{"runId": "run-fake", "tasks": rows, "count": len(rows)})
		case "dispatch":
			if arg(args, "--to") != "term-agent" || arg(args, "--task") != fmt.Sprintf("task-%d", len(state.Tasks)) {
				return fakeOrcaErr("invalid_argument", "wrong target")
			}
			state.Dispatches++
			save()
			n := state.Dispatches
			if n == state.LoseDispatchAt {
				return fakeOrcaErr("runtime_unavailable", "Dispatch was injected but the response was lost")
			}
			return fakeOrcaOK(map[string]any{"dispatch": map[string]any{"id": fmt.Sprintf("ctx-%d", n), "task_id": fmt.Sprintf("task-%d", n), "run_id": "run-fake"}, "mutation": map[string]any{"requestId": fmt.Sprintf("request-dispatch-%d", n)}})
		case "dispatch-show":
			n, _ := strconv.Atoi(strings.TrimPrefix(arg(args, "--task"), "task-"))
			if n == 0 || n > state.Dispatches {
				return fakeOrcaErr("dispatch_not_found", "no dispatch")
			}
			return fakeOrcaOK(map[string]any{"dispatch": map[string]any{"id": fmt.Sprintf("ctx-%d", n), "task_id": fmt.Sprintf("task-%d", n), "run_id": "run-fake", "assignee_handle": "term-agent", "process_incarnation": "pty:inc-agent"}})
		case "check":
			if ack := arg(args, "--ack"); ack != "" {
				state.Acked[ack] = true
				lost := state.LoseAckOnce
				state.LoseAckOnce = false
				save()
				if lost {
					return fakeOrcaErr("runtime_unavailable", "Delivery was acknowledged but response was lost")
				}
				return fakeOrcaOK(map[string]any{"runId": "run-fake", "acknowledged": ack, "mutation": map[string]any{"requestId": "request-ack-" + ack}})
			}
			if state.Dispatches == 1 && state.Replies == 0 && !state.Acked["delivery-ask-1"] {
				payload, _ := json.Marshal(map[string]any{"taskId": "task-1", "dispatchId": "ctx-1", "question": "Which approach?", "options": []string{"Direct path"}})
				message := map[string]any{"id": "msg-ask-1", "from_handle": "dispatch:ctx-1", "type": "question", "body": "Which approach?", "payload": string(payload)}
				return fakeOrcaOK(map[string]any{"runId": "run-fake", "deliveryId": "delivery-ask-1", "messages": []any{message}})
			}
			if state.Dispatches == state.EscalationAt && state.Sends == 0 && !state.Acked[fmt.Sprintf("delivery-escalation-%d", state.EscalationAt)] {
				n := state.Dispatches
				message := map[string]any{"id": fmt.Sprintf("msg-escalation-%d", n), "from_handle": fmt.Sprintf("dispatch:ctx-%d", n), "type": "escalation", "subject": "Need more guidance", "body": "Send guidance to the current Dispatch"}
				return fakeOrcaOK(map[string]any{"runId": "run-fake", "deliveryId": fmt.Sprintf("delivery-escalation-%d", n), "messages": []any{message}})
			}
			if state.Dispatches > 0 && (state.Dispatches > 1 || state.Replies > 0) && (state.Dispatches != state.EscalationAt || state.Sends > 0) && !state.Acked[fmt.Sprintf("delivery-done-%d", state.Dispatches)] {
				n := state.Dispatches
				payload, _ := json.Marshal(map[string]any{"taskId": fmt.Sprintf("task-%d", n), "dispatchId": fmt.Sprintf("ctx-%d", n), "outcome": "succeeded"})
				message := map[string]any{"id": fmt.Sprintf("msg-%d", n), "from_handle": "term-agent", "type": "worker_done", "body": fmt.Sprintf("DONE %d", n), "payload": string(payload)}
				return fakeOrcaOK(map[string]any{"runId": "run-fake", "deliveryId": fmt.Sprintf("delivery-done-%d", n), "messages": []any{message}})
			}
			return fakeOrcaOK(map[string]any{"runId": "run-fake", "messages": []any{}})
		case "worker-read":
			n, _ := strconv.Atoi(strings.TrimPrefix(arg(args, "--dispatch"), "ctx-"))
			if arg(args, "--cursor") != "" && state.ForceSourceChange {
				state.ForceSourceChange = false
				state.TranscriptAfterSwitch = true
				state.ReportSourceChanged = true
				save()
				return fakeOrcaErr("source_changed", "worker-read source_changed; restart without cursor")
			}
			if state.TranscriptAfterSwitch {
				changed := state.ReportSourceChanged
				state.ReportSourceChanged = false
				save()
				return fakeOrcaOK(map[string]any{"dispatchId": arg(args, "--dispatch"), "source": "transcript", "nextCursor": fmt.Sprintf("transcript-cursor-%d", n), "contentComplete": true, "sourceChanged": changed, "transcript": map[string]any{"text": fmt.Sprintf("complete fake output %d", n)}})
			}
			return fakeOrcaOK(map[string]any{"dispatchId": arg(args, "--dispatch"), "source": "terminal", "cursor": fmt.Sprintf("cursor-%d", n), "contentComplete": false, "clipping": []string{"terminal_fallback"}, "fallbackReason": "session_not_reported", "terminal": map[string]any{"tail": []string{fmt.Sprintf("fake output %d", n)}}})
		case "worker-show":
			n, _ := strconv.Atoi(strings.TrimPrefix(arg(args, "--dispatch"), "ctx-"))
			return fakeOrcaOK(map[string]any{"dispatch": map[string]any{"id": fmt.Sprintf("ctx-%d", n), "runId": "run-fake", "taskId": fmt.Sprintf("task-%d", n), "status": "completed"}, "worker": map[string]any{"dispatchId": fmt.Sprintf("ctx-%d", n), "state": "unsupervised", "stage": "injected", "agentTerminalHandle": "term-agent"}, "terminal": terminal("term-agent")})
		}
	}
	return fakeOrcaErr("invalid_argument", "unhandled native Orca fixture command: "+strings.Join(args, " "))
}
