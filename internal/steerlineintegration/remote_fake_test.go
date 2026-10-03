package steerlineintegration

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/testprotocol"
)

func fakeIntegrationCommand(args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "native-orca":
		if len(args) < 2 {
			return 2
		}
		return testprotocol.FakeNativeOrca(args[1], args[2:])
	case "publication-git":
		return fakePublicationGit(args[1:])
	case "codex":
		return fakeCodexCommand(args[1:])
	default:
		return 2
	}
}

func fakePublicationGit(args []string) int {
	if len(args) == 0 {
		return 2
	}
	if len(args) >= 4 && args[0] == "git" && args[1] == "remote" && args[2] == "get-url" {
		if delay := os.Getenv("STEERLINE_CLI_PREFLIGHT_DELAY"); delay != "" {
			if d, err := time.ParseDuration(delay); err == nil {
				time.Sleep(d)
			}
		}
		fmt.Println(os.Getenv("STEERLINE_TEST_REMOTE_URL"))
		return 0
	}
	gitArgs := append([]string(nil), args...)
	if gitArgs[0] == "git" {
		gitArgs = gitArgs[1:]
	}
	if len(gitArgs) > 0 && gitArgs[0] == "fetch" {
		remote := os.Getenv("STEERLINE_FAKE_GIT_FETCH_REMOTE")
		for i := 1; i < len(gitArgs); i++ {
			if gitArgs[i] == "origin" && remote != "" {
				gitArgs[i] = remote
				break
			}
		}
	}
	command := exec.Command("git", gitArgs...)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func fakeCodexCommand(args []string) int {
	for _, arg := range args {
		if arg == "--version" {
			fmt.Println(codexworker.PolicyCodexVersion)
			return 0
		}
	}
	if slicesContains(args, "--help") {
		fmt.Println("--strict-config --sandbox --ask-for-approval never --config")
		return 0
	}
	if slicesContains(args, "--stdio") {
		return fakeCodexStdio()
	}
	if slicesContains(args, "--listen") {
		return fakeCodexWebSocket(args)
	}
	return 2
}

func slicesContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func fakeCodexStdio() int {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ThreadID  string `json:"threadId"`
				Cursor    string `json:"cursor"`
				Limit     int    `json:"limit"`
				ItemsView string `json:"itemsView"`
			} `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || len(request.ID) == 0 {
			continue
		}
		if request.Method == "initialized" {
			continue
		}
		response := map[string]any{"id": request.ID}
		if request.Method == "initialize" {
			response["result"] = map[string]string{"userAgent": codexworker.PolicyCodexVersion, "codexHome": codexworker.EnvironmentHome(), "platformOs": "windows"}
		} else {
			var history struct {
				Cwd   string           `json:"cwd"`
				Turns []map[string]any `json:"turns"`
			}
			err := withIntegrationHistoryFile(func(path string) error {
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				return json.Unmarshal(raw, &history)
			})
			if err != nil || request.Params.ThreadID != integrationAppServerSession {
				response["error"] = map[string]any{"code": -32602, "message": "original history session is unavailable"}
			} else if request.Method == "thread/read" {
				response["result"] = map[string]any{"thread": map[string]any{"id": integrationAppServerSession, "cwd": history.Cwd}}
			} else if request.Method == "thread/turns/list" && request.Params.Limit == 1 && request.Params.ItemsView == "full" {
				index := 0
				if request.Params.Cursor != "" {
					if _, err := fmt.Sscanf(request.Params.Cursor, "turn:%d", &index); err != nil {
						return 27
					}
				}
				if index < 0 || index > len(history.Turns) {
					return 28
				}
				data := []map[string]any{}
				var next, previous any
				if index < len(history.Turns) {
					turn := history.Turns[index]
					turn["itemsView"] = "full"
					data = append(data, turn)
					previous = fmt.Sprintf("turn:%d", index)
					if index+1 < len(history.Turns) {
						next = fmt.Sprintf("turn:%d", index+1)
					}
				}
				response["result"] = map[string]any{"data": data, "nextCursor": next, "backwardsCursor": previous}
			} else {
				response["error"] = map[string]any{"code": -32601, "message": "unsupported history request"}
			}
		}
		if json.NewEncoder(os.Stdout).Encode(response) != nil {
			return 29
		}
	}
	return 0
}

func withIntegrationHistoryFile(call func(string) error) error {
	path := os.Getenv("STEERLINE_INTEGRATION_HISTORY_FILE")
	if path == "" {
		return fmt.Errorf("history fixture path is not configured")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_ = lock.Close()
			defer os.Remove(path + ".lock")
			return call(path)
		}
		if !os.IsExist(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

const integrationAppServerSession = "00000000-0000-0000-0000-000000000901"

type integrationAppServerState struct {
	mu    sync.Mutex
	turns []map[string]any
}

func fakeCodexWebSocket(args []string) int {
	endpoint := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--listen" {
			endpoint = args[i+1]
			break
		}
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "ws" || u.Host == "" {
		return 21
	}
	listener, err := net.Listen("tcp", u.Host)
	if err != nil {
		return 22
	}
	state := &integrationAppServerState{}
	for {
		connection, err := listener.Accept()
		if err != nil {
			return 0
		}
		go integrationAppServerConnection(connection, state)
	}
}

func integrationAppServerConnection(connection net.Conn, state *integrationAppServerState) {
	defer connection.Close()
	reader := bufio.NewReader(connection)
	request, err := http.ReadRequest(reader)
	if err != nil || request.Header.Get("Upgrade") != "websocket" {
		return
	}
	accept := sha1.Sum([]byte(request.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if _, err := io.WriteString(connection, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "+base64.StdEncoding.EncodeToString(accept[:])+"\r\n\r\n"); err != nil {
		return
	}
	send := func(value any) error {
		payload, err := json.Marshal(value)
		if err != nil {
			return err
		}
		frame := []byte{0x81}
		switch {
		case len(payload) < 126:
			frame = append(frame, byte(len(payload)))
		case len(payload) <= 65535:
			frame = append(frame, 126, byte(len(payload)>>8), byte(len(payload)))
		default:
			frame = append(frame, 127, 0, 0, 0, 0, byte(len(payload)>>24), byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload)))
		}
		_, err = connection.Write(append(frame, payload...))
		return err
	}
	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(reader, header); err != nil || header[0]&0x0f != 1 || header[1]&0x80 == 0 {
			return
		}
		size := uint64(header[1] & 0x7f)
		if size == 126 {
			var short uint16
			if binary.Read(reader, binary.BigEndian, &short) != nil {
				return
			}
			size = uint64(short)
		} else if size == 127 && binary.Read(reader, binary.BigEndian, &size) != nil {
			return
		}
		if size > 16<<20 {
			return
		}
		mask := make([]byte, 4)
		if _, err := io.ReadFull(reader, mask); err != nil {
			return
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return
		}
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(payload, &message) != nil || len(message.ID) == 0 {
			continue
		}
		response := map[string]any{"id": message.ID}
		switch message.Method {
		case "initialize":
			response["result"] = map[string]string{"userAgent": codexworker.PolicyCodexVersion, "codexHome": os.TempDir(), "platformOs": "windows"}
		case "thread/start", "thread/resume":
			cwd, _ := os.Getwd()
			response["result"] = map[string]any{"thread": map[string]any{"id": integrationAppServerSession, "cwd": cwd}}
		case "thread/read":
			state.mu.Lock()
			turns := append([]map[string]any(nil), state.turns...)
			state.mu.Unlock()
			response["result"] = map[string]any{"thread": map[string]any{"id": integrationAppServerSession, "turns": turns}}
		case "turn/start":
			var params struct {
				Input []struct {
					Text string `json:"text"`
				} `json:"input"`
			}
			_ = json.Unmarshal(message.Params, &params)
			state.mu.Lock()
			turnID := fmt.Sprintf("00000000-0000-0000-0000-%012d", len(state.turns)+1)
			text := "completed integration turn"
			if len(params.Input) != 0 {
				text = "completed: " + params.Input[0].Text
			}
			item := map[string]any{"type": "agentMessage", "id": "message-" + turnID, "phase": "final_answer", "text": text}
			turn := map[string]any{"id": turnID, "status": "completed", "completedAt": len(state.turns) + 1, "items": []any{item}}
			state.turns = append(state.turns, turn)
			cwd, _ := os.Getwd()
			if err := withIntegrationHistoryFile(func(path string) error {
				data, err := json.Marshal(map[string]any{"cwd": cwd, "turns": state.turns})
				if err != nil {
					return err
				}
				return AtomicWrite(path, data)
			}); err != nil {
				state.mu.Unlock()
				return
			}
			state.mu.Unlock()
			response["result"] = map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}
			if send(response) != nil || send(map[string]any{"method": "turn/started", "params": map[string]any{"threadId": integrationAppServerSession, "turn": map[string]any{"id": turnID}}}) != nil ||
				send(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": integrationAppServerSession, "turnId": turnID, "item": item}}) != nil ||
				send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": integrationAppServerSession, "turn": turn}}) != nil {
				return
			}
			continue
		case "turn/interrupt":
			response["result"] = map[string]any{"accepted": true}
		default:
			response["error"] = map[string]any{"code": -32601, "message": "unsupported integration app-server method"}
		}
		if send(response) != nil {
			return
		}
	}
}

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
	if len(os.Args) > 1 && os.Args[1] == "--fake-supervised" {
		os.Exit(testprotocol.FakeSupervisedOrca(os.Args[2:]))
	}
	if os.Getenv("STEERLINE_LIFECYCLE_EXIT_EARLY") == "1" {
		os.Exit(23)
	}
	if len(os.Args) > 2 && os.Args[1] == "--fake-remote" {
		os.Exit(fakeRemoteOrca(os.Args[2], os.Args[3:]))
	}
	for i, arg := range os.Args {
		if arg == "--fake" {
			os.Exit(fakeIntegrationCommand(os.Args[i+1:]))
		}
	}
	joined := strings.Join(os.Args[1:], " ")
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(codexworker.PolicyCodexVersion)
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Println("--strict-config --sandbox --ask-for-approval never --config")
		os.Exit(0)
	}
	if strings.Contains(joined, "app-server") && !strings.HasPrefix(os.Args[1], "-test") {
		if strings.Contains(joined, "--help") {
			fmt.Println("app-server --listen")
			os.Exit(0)
		}
		if strings.Contains(joined, "--stdio") {
			os.Exit(fakeCodexStdio())
		}
		os.Exit(fakeCodexWebSocket(os.Args[1:]))
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
