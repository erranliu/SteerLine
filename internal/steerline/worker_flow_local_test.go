package steerline

import (
	"bufio"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const workerFlowMockSession = "00000000-0000-0000-0000-000000000008"

func TestWorkerFlowPromptIncludesFixedWorkflowRunID(t *testing.T) {
	task := TaskRecord{TaskID: "102", Title: "Native branch A"}
	contract := TaskContract{ContractID: "fixed", Branch: "main", SpecRef: FileReference{Path: "docs/spec/task.spec.md"}, PlanRef: FileReference{Path: "docs/spec/task.plan.md"}}
	runID := strings.Repeat("a", 32)
	for _, localPublish := range []bool{false, true} {
		prompt, err := workerFlowPrompt(task, contract, runID, localPublish)
		if err != nil || !strings.Contains(prompt, "Fixed SteerLine workflow_run_id: "+runID) || strings.Contains(prompt, "Orca Run, Task and Dispatch IDs are separate") == localPublish {
			t.Fatalf("worker prompt omitted its fixed center identity: %q / %v", prompt, err)
		}
	}
	if _, err := workerFlowPrompt(task, contract, "", false); err == nil {
		t.Fatal("worker prompt accepted a missing workflow run ID")
	}
}

func TestWorkerTurnFindsPRBeforeFinalMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	events := `{"type":"thread.started","thread_id":"00000000-0000-0000-0000-000000000008"}
{"type":"turn.started"}
{"type":"item.completed","item":{"type":"agent_message","text":"Created https://git.example/o/r/pulls/8."}}
{"type":"item.completed","item":{"type":"agent_message","text":"Work is ready."}}
{"type":"turn.completed"}
`
	if err := os.WriteFile(path, []byte(events), 0600); err != nil {
		t.Fatal(err)
	}
	outcome, err := readWorkerTurnOutcome(path)
	if err != nil || !outcome.Completed || outcome.LastMessage != "Work is ready." || len(outcome.PRCandidates) != 1 || outcome.PRCandidates[0] != "https://git.example/o/r/pulls/8" {
		t.Fatalf("lost an earlier PR clue: outcome=%+v err=%v", outcome, err)
	}
}

func TestWorkerFlowMockCodexHelper(t *testing.T) {
	args := strings.Join(os.Args, " ")
	if os.Getenv("STEERLINE_WORKER_FLOW_MOCK") != "1" && !strings.Contains(args, " wf-orca-mock ") {
		return
	}
	if strings.Contains(args, " --version") {
		fmt.Fprintln(os.Stdout, "codex-cli mock-1")
		os.Exit(0)
	}
	if strings.Contains(args, " app-server ") && strings.Contains(args, " --help") {
		fmt.Fprintln(os.Stdout, "Codex app-server --listen ADDRESS")
		os.Exit(0)
	}
	if strings.Contains(args, " app-server ") && strings.Contains(args, " --stdio") {
		mockAppServerStdio()
		os.Exit(0)
	}
	if strings.Contains(args, " app-server ") && strings.Contains(args, " --listen ") {
		mockAppServerMain()
		os.Exit(0)
	}
	if strings.Contains(args, " resume ") && !strings.Contains(args, " resume "+workerFlowMockSession+" ") {
		os.Exit(2)
	}
	if !strings.Contains(args, " resume ") && strings.Contains(args, workerFlowMockSession) {
		os.Exit(3)
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(4)
	}
	received := string(prompt)
	if strings.Contains(args, " wf-orca-mock ") {
		withoutPipelineNewline := strings.TrimSuffix(strings.ReplaceAll(string(prompt), "\r\n", "\n"), "\n")
		sum := sha256.Sum256([]byte(withoutPipelineNewline))
		start, end := prompt[:min(96, len(prompt))], prompt[max(0, len(prompt)-96):]
		received = fmt.Sprintf("received_sha256:%s bytes:%d prefix_b64:%s suffix_b64:%s", hex.EncodeToString(sum[:]), len(prompt), base64.StdEncoding.EncodeToString(start), base64.StdEncoding.EncodeToString(end))
	}
	echo, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]string{"id": "received-prompt", "type": "agent_message", "text": received}})
	_, _ = fmt.Fprintln(os.Stdout, `{"type":"thread.started","thread_id":"`+workerFlowMockSession+`"}`)
	_, _ = fmt.Fprintln(os.Stdout, `{"type":"turn.started"}`)
	_, _ = fmt.Fprintln(os.Stdout, string(echo))
	_, _ = fmt.Fprintln(os.Stdout, `{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"turn done"}}`)
	_, _ = fmt.Fprintln(os.Stdout, `{"type":"turn.completed"}`)
	os.Exit(0)
}

func mockAppServerStdio() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &message) != nil || len(message.ID) == 0 {
			continue
		}
		if message.Method == "initialize" {
			fmt.Fprintln(os.Stdout, string(mustJSON(map[string]any{"id": message.ID, "result": map[string]string{"userAgent": "mock-app-server", "codexHome": os.TempDir(), "platformOs": "windows"}})))
		}
	}
}

type mockAppServerState struct {
	mu    sync.Mutex
	cwd   string
	turns []map[string]any
}

func mockAppServerMain() {
	endpoint := ""
	for i, arg := range os.Args {
		if arg == "--listen" && i+1 < len(os.Args) {
			endpoint = os.Args[i+1]
		}
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "ws" || u.Host == "" {
		os.Exit(21)
	}
	listener, err := net.Listen("tcp", u.Host)
	if err != nil {
		os.Exit(22)
	}
	state := &mockAppServerState{}
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go mockAppServerConnection(conn, state)
	}
}

func mockAppServerConnection(conn net.Conn, state *mockAppServerState) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil || request.Header.Get("Upgrade") != "websocket" {
		return
	}
	accept := sha1.Sum([]byte(request.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if _, err := io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "+base64.StdEncoding.EncodeToString(accept[:])+"\r\n\r\n"); err != nil {
		return
	}
	send := func(value any) error {
		payload := mustJSON(value)
		frame := []byte{0x81}
		switch {
		case len(payload) < 126:
			frame = append(frame, byte(len(payload)))
		case len(payload) <= 65535:
			frame = append(frame, 126, byte(len(payload)>>8), byte(len(payload)))
		default:
			frame = append(frame, 127, 0, 0, 0, 0, byte(len(payload)>>24), byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload)))
		}
		_, err := conn.Write(append(frame, payload...))
		return err
	}
	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(reader, header); err != nil {
			return
		}
		if header[0]&0xf != 1 || header[1]&0x80 == 0 {
			return
		}
		size := uint64(header[1] & 0x7f)
		if size == 126 {
			var value uint16
			if binary.Read(reader, binary.BigEndian, &value) != nil {
				return
			}
			size = uint64(value)
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
		if json.Unmarshal(payload, &message) != nil {
			return
		}
		if len(message.ID) == 0 {
			continue
		}
		response := map[string]any{"id": message.ID}
		switch message.Method {
		case "initialize":
			response["result"] = map[string]string{"userAgent": "mock-app-server", "codexHome": os.TempDir(), "platformOs": "windows"}
		case "thread/start", "thread/resume":
			var params struct {
				Cwd string `json:"cwd"`
			}
			_ = json.Unmarshal(message.Params, &params)
			state.mu.Lock()
			if state.cwd == "" {
				state.cwd = params.Cwd
			}
			cwd := state.cwd
			state.mu.Unlock()
			response["result"] = map[string]any{"thread": map[string]any{"id": workerFlowMockSession, "cwd": cwd}}
		case "thread/read":
			state.mu.Lock()
			response["result"] = map[string]any{"thread": map[string]any{"id": workerFlowMockSession, "cwd": state.cwd, "turns": append([]map[string]any(nil), state.turns...)}}
			state.mu.Unlock()
		case "turn/start":
			var params struct {
				Input []struct {
					Text string `json:"text"`
				} `json:"input"`
			}
			_ = json.Unmarshal(message.Params, &params)
			state.mu.Lock()
			turnID := fmt.Sprintf("00000000-0000-0000-0000-%012d", len(state.turns)+1)
			text := "turn done"
			if len(params.Input) != 0 {
				text = params.Input[0].Text
			}
			item := map[string]any{"type": "agentMessage", "id": "message-" + turnID, "phase": "final_answer", "text": text}
			turn := map[string]any{"id": turnID, "status": "completed", "completedAt": 1, "items": []any{item}}
			state.turns = append(state.turns, turn)
			state.mu.Unlock()
			response["result"] = map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}
			if send(response) != nil || send(map[string]any{"method": "turn/started", "params": map[string]any{"threadId": workerFlowMockSession, "turn": map[string]any{"id": turnID}}}) != nil || send(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": workerFlowMockSession, "turnId": turnID, "item": item}}) != nil || send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": workerFlowMockSession, "turn": turn}}) != nil {
				return
			}
			continue
		default:
			response["error"] = map[string]any{"code": -32601, "message": "unsupported mock method"}
		}
		if send(response) != nil {
			return
		}
	}
}
