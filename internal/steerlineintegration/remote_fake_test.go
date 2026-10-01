package steerlineintegration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type remoteFixtureTask struct {
	ID, Spec, Dispatch, Outcome string
	Answered                    bool
}
type remoteFixtureState struct {
	Origin, Base, Name, Path, Branch, Coordinator, Objective, PRURL           string
	Tasks                                                                     []remoteFixtureTask
	Acked                                                                     map[string]bool
	Calls                                                                     []string
	Question, Finish, LoseStart, LoseReply, LoseAck, Offline, ChangedIdentity bool
	Starts, Replies, Releases, CoordinatorCloses                              int
}

func fakeRemoteOrca(dir string, args []string) int {
	path := filepath.Join(dir, "remote.json")
	var state remoteFixtureState
	raw, _ := os.ReadFile(path)
	_ = json.Unmarshal(raw, &state)
	if state.Acked == nil {
		state.Acked = map[string]bool{}
	}
	save := func() { data, _ := json.Marshal(state); _ = AtomicWrite(path, data) }
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("1.4.210")
		return 0
	}
	if len(args) == 0 {
		return 2
	}
	state.Calls = append(state.Calls, strings.Join(args, " "))
	save()
	if state.Offline && (arg(args, "--environment") != "" || args[0] == "orchestration") {
		return fakeOrcaErr("transport_unavailable", "remote unavailable")
	}
	remote := arg(args, "--environment") != ""
	runtimeID := "center"
	if remote {
		runtimeID = "peer"
	}
	worktreeID := "repo-test::" + state.Path
	worktree := func() map[string]any {
		return map[string]any{"id": worktreeID, "repoId": "repo-test", "path": state.Path, "head": state.Base, "baseRef": state.Base, "branch": "refs/heads/" + state.Branch, "displayName": state.Name}
	}
	result := map[string]any{}
	mutation := func() { result["mutation"] = map[string]any{"requestId": fmt.Sprintf("request-%d", len(state.Calls))} }
	switch args[0] {
	case "status":
		caps := []string{"orchestration.federation.v1", "orchestration.federation-control-mail.v1", "orchestration.federation-lifecycle-settlement.v1", "orchestration.worker-launch-preferences.v1", "orchestration.federation-structured-read.v1", "orchestration.federation-fleet-snapshot.v1", "orchestration.federation-release-archive.v1", "agent.launch.v2"}
		result["runtime"] = map[string]any{"state": "ready", "reachable": true, "runtimeId": runtimeID, "appVersion": "1.4.210", "capabilities": caps}
	case "environment":
		result["environments"] = []any{map[string]any{"id": "env-home", "pairedDeviceId": "home-connection", "runtimeId": "peer"}}
	case "repo":
		result["repo"] = map[string]any{"id": "repo-test", "kind": "git", "gitRemoteIdentity": map[string]any{"remoteUrl": state.Origin}}
	case "worktree":
		if !remote {
			return fakeOrcaErr("wrong_host", "remote worktree called on center")
		}
		switch args[1] {
		case "list":
			rows := []any{}
			if state.Name != "" {
				rows = append(rows, worktree())
			}
			result["worktrees"] = rows
		case "create":
			if state.Name != "" {
				return fakeOrcaErr("duplicate", "duplicate worktree")
			}
			state.Name, state.Base = arg(args, "--name"), arg(args, "--base-branch")
			state.Path = "/srv/steerline/" + state.Name
			state.Branch = "remote/" + state.Name
			worktreeID = "repo-test::" + state.Path
			result["worktree"] = worktree()
		case "show":
			result["worktree"] = worktree()
		}
	case "terminal":
		if remote {
			return fakeOrcaErr("wrong_surface", "remote worker must use Dispatch protocol")
		}
		switch args[1] {
		case "create":
			if state.Coordinator != "" {
				return fakeOrcaErr("duplicate", "duplicate coordinator")
			}
			state.Coordinator = "term-remote-coordinator"
			result["terminal"] = map[string]any{"handle": state.Coordinator}
		case "show":
			result["terminal"] = map[string]any{"handle": arg(args, "--terminal"), "connected": true, "orphaned": false, "incarnationId": "inc-coordinator"}
		case "close":
			state.CoordinatorCloses++
			result["close"] = map[string]any{"handle": state.Coordinator, "ptyKilled": true}
		case "list":
			result["terminals"] = []any{map[string]any{"handle": state.Coordinator, "title": arg(args, "--title"), "connected": true}}
		default:
			return fakeOrcaErr("wrong_surface", "worker must not use terminal shell commands")
		}
	case "orchestration":
		if remote {
			return fakeOrcaErr("wrong_host", "Run and inbox must remain on center")
		}
		var task *remoteFixtureTask
		for i := range state.Tasks {
			if state.Tasks[i].ID == arg(args, "--task") || state.Tasks[i].Dispatch == arg(args, "--dispatch") {
				task = &state.Tasks[i]
				break
			}
		}
		switch args[1] {
		case "run-create":
			state.Objective = arg(args, "--objective")
			result["run"] = map[string]any{"id": "remote-run", "objective": state.Objective}
			mutation()
		case "run-current":
			result["run"] = map[string]any{"id": "remote-run", "objective": state.Objective}
		case "run-show":
			result["run"] = map[string]any{"id": "remote-run", "coordinator_handle": state.Coordinator}
		case "task-create":
			id := fmt.Sprintf("task-%d", len(state.Tasks)+1)
			state.Tasks = append(state.Tasks, remoteFixtureTask{ID: id, Spec: arg(args, "--spec")})
			result["task"] = map[string]any{"id": id}
			mutation()
		case "task-list":
			rows := []any{}
			for _, row := range state.Tasks {
				rows = append(rows, map[string]any{"id": row.ID, "run_id": "remote-run", "spec": row.Spec, "created_by_terminal_handle": state.Coordinator})
			}
			result = map[string]any{"runId": "remote-run", "tasks": rows, "count": len(rows)}
		case "worker-start":
			if task == nil || task.Dispatch != "" || arg(args, "--on") != "env-home" || arg(args, "--worktree") != "id:"+worktreeID {
				return fakeOrcaErr("identity", "wrong or repeated remote start")
			}
			if state.Starts == 0 {
				if arg(args, "--agent") != "codex" || arg(args, "--model") != "gpt-6-luna" || arg(args, "--effort") != "high" {
					return fakeOrcaErr("launch", "fixed model missing")
				}
			} else {
				if arg(args, "--terminal") != "remote-agent" || arg(args, "--model") != "" || arg(args, "--effort") != "" {
					return fakeOrcaErr("replacement", "original session was not reused")
				}
			}
			state.Starts++
			task.Dispatch = fmt.Sprintf("dispatch-%d", state.Starts)
			mutation()
			result["runId"], result["taskId"], result["dispatchId"], result["state"], result["stage"] = "remote-run", task.ID, task.Dispatch, "ready", "input_accepted"
			result["server"] = map[string]any{"environmentId": "env-home"}
			result["worker"] = map[string]any{"worktreeId": worktreeID}
			result["launch"] = map[string]any{"effective": map[string]any{"agent": "codex", "model": "gpt-6-luna", "effort": "high"}}
			if state.LoseStart {
				state.LoseStart = false
				save()
				return fakeOrcaErr("transport_unavailable", "start applied but receipt lost")
			}
		case "dispatch-show":
			if task == nil {
				return fakeOrcaErr("absent", "Task has no Dispatch")
			}
			result["dispatch"] = map[string]any{"id": task.Dispatch, "task_id": task.ID, "run_id": "remote-run", "assignee_handle": "remote-agent", "process_incarnation": "peer:inc-agent"}
		case "worker-show":
			if task == nil {
				return fakeOrcaErr("absent", "Dispatch absent")
			}
			status, workerState := "dispatched", "ready"
			if task.Outcome != "" {
				status, workerState = "completed", task.Outcome
			}
			inc := "inc-agent"
			if state.ChangedIdentity {
				inc = "replacement"
			}
			result["dispatch"] = map[string]any{"id": task.Dispatch, "runId": "remote-run", "taskId": task.ID, "status": status}
			result["worker"] = map[string]any{"dispatchId": task.Dispatch, "worktreeId": worktreeID, "agentTerminalHandle": "remote-agent", "state": workerState}
			result["projection"] = map[string]any{"host": map[string]any{"id": "env-home"}, "liveness": map[string]any{"verdict": "live"}}
			result["server"] = map[string]any{"environmentId": "env-home"}
			result["remoteRuntimeEpoch"] = "peer"
			result["observation"] = map[string]any{"exactWorker": true}
			result["terminal"] = map[string]any{"handle": "remote-agent", "incarnationId": inc, "worktreeId": worktreeID, "worktreePath": state.Path, "branch": "refs/heads/" + state.Branch}
		case "worker-read":
			result = map[string]any{"dispatchId": arg(args, "--dispatch"), "source": "transcript", "cursor": "cursor", "nextCursor": "cursor", "contentComplete": true, "transcript": map[string]any{"messages": []any{}}}
		case "check":
			if arg(args, "--terminal") != state.Coordinator || arg(args, "--run") != "remote-run" {
				return fakeOrcaErr("identity", "wrong coordinator/Run")
			}
			if ack := arg(args, "--ack"); ack != "" {
				state.Acked[ack] = true
				result["acknowledged"] = ack
				mutation()
				if state.LoseAck {
					state.LoseAck = false
					save()
					return fakeOrcaErr("transport_unavailable", "ack applied but receipt lost")
				}
				break
			}
			result["runId"] = "remote-run"
			messages := []any{}
			if len(state.Tasks) > 0 {
				task = &state.Tasks[len(state.Tasks)-1]
			}
			if task != nil && task.Dispatch != "" {
				kind, body, id := "status", "Working", task.Dispatch+"-active"
				payload := map[string]any{"taskId": task.ID, "dispatchId": task.Dispatch}
				if state.Question && !task.Answered {
					kind, body, id = "question", "Which path?", task.Dispatch+"-question"
					payload["question"] = "Which path?"
				} else if state.Finish {
					kind, body, id = "worker_done", "Done. "+state.PRURL, task.Dispatch+"-done"
					payload["outcome"] = "succeeded"
					task.Outcome = "succeeded"
				}
				delivery := "delivery-" + id
				if !state.Acked[delivery] {
					messages = append(messages, map[string]any{"id": id, "type": kind, "body": body, "from_handle": "dispatch:" + task.Dispatch, "to_handle": "run:remote-run", "payload": payload})
					result["deliveryId"] = delivery
				}
			}
			result["messages"] = messages
		case "reply":
			for i := range state.Tasks {
				if strings.HasPrefix(arg(args, "--id"), state.Tasks[i].Dispatch+"-") {
					task = &state.Tasks[i]
				}
			}
			if task == nil || task.Answered {
				return fakeOrcaErr("duplicate", "answer was repeated")
			}
			task.Answered = true
			state.Replies++
			mutation()
			result["question"] = map[string]any{"message_id": arg(args, "--id"), "run_id": "remote-run", "dispatch_id": task.Dispatch, "answer_body": arg(args, "--body"), "status": "answered"}
			result["message"] = map[string]any{"from_handle": "run:remote-run", "to_handle": "dispatch:" + task.Dispatch}
			if state.LoseReply {
				state.LoseReply = false
				save()
				return fakeOrcaErr("transport_unavailable", "reply applied but receipt lost")
			}
		case "inbox":
			rows := []any{}
			for _, task := range state.Tasks {
				if task.Answered {
					rows = append(rows, map[string]any{"id": "answer-" + task.ID, "thread_id": task.Dispatch + "-question", "from_handle": "run:remote-run", "to_handle": "dispatch:" + task.Dispatch, "type": "status", "body": "left"})
				}
			}
			result["messages"] = rows
		case "worker-release":
			if task == nil || task.Outcome == "" {
				return fakeOrcaErr("not_settled", "cannot release active worker")
			}
			state.Releases++
			result = map[string]any{"dispatchId": task.Dispatch, "state": "released", "residualResources": []any{}}
			mutation()
		default:
			return fakeOrcaErr("unsupported", args[1])
		}
	default:
		return fakeOrcaErr("unsupported", args[0])
	}
	save()
	return fakeSupervisedEnvelope(runtimeID, result)
}

func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == "--fake-remote" {
		os.Exit(fakeRemoteOrca(os.Args[2], os.Args[3:]))
	}
	os.Exit(m.Run())
}
func arg(args []string, key string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}
func fakeOrcaErr(code, diagnostic string) int {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": false, "error": map[string]any{"code": code, "diagnostic": diagnostic}})
	return 1
}
func fakeSupervisedEnvelope(runtime string, result any) int {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "result": result, "_meta": map[string]string{"runtimeId": runtime}})
	return 0
}
