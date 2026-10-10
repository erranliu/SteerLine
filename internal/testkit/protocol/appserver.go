package testprotocol

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
	"sync"
)

const AppServerSession = "00000000-0000-0000-0000-000000000008"

func appServerJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func MockAppServerStdio() {
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
			fmt.Fprintln(os.Stdout, string(appServerJSON(map[string]any{"id": message.ID, "result": map[string]string{"userAgent": "mock-app-server", "codexHome": os.TempDir(), "platformOs": "windows"}})))
		}
	}
}

type AppServerState struct {
	mu    sync.Mutex
	Cwd   string
	Turns []map[string]any
}

func MockAppServerMain() {
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
	state := &AppServerState{}
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go MockAppServerConnection(conn, state)
	}
}

func MockAppServerConnection(conn net.Conn, state *AppServerState) {
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
		payload := appServerJSON(value)
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
			// A newly created empty native thread has no persisted rollout.
			if message.Method == "thread/resume" && len(state.Turns) == 0 {
				state.mu.Unlock()
				response["error"] = map[string]any{"code": -32600, "message": "no rollout found for thread id " + AppServerSession}
				break
			}
			if state.Cwd == "" {
				state.Cwd = params.Cwd
			}
			Cwd := state.Cwd
			state.mu.Unlock()
			response["result"] = map[string]any{"thread": map[string]any{"id": AppServerSession, "cwd": Cwd}}
		case "thread/read":
			state.mu.Lock()
			response["result"] = map[string]any{"thread": map[string]any{"id": AppServerSession, "cwd": state.Cwd, "turns": append([]map[string]any(nil), state.Turns...)}}
			state.mu.Unlock()
		case "turn/steer":
			var params struct {
				ThreadID string           `json:"threadId"`
				Expected string           `json:"expectedTurnId"`
				ClientID string           `json:"clientUserMessageId"`
				Input    []map[string]any `json:"input"`
			}
			_ = json.Unmarshal(message.Params, &params)
			state.mu.Lock()
			if len(state.Turns) == 0 || params.ThreadID != AppServerSession || state.Turns[len(state.Turns)-1]["id"] != params.Expected || state.Turns[len(state.Turns)-1]["status"] != "inProgress" || params.ClientID == "" {
				state.mu.Unlock()
				response["error"] = map[string]any{"code": -32600, "message": "original turn is not active"}
				break
			}
			turn := state.Turns[len(state.Turns)-1]
			user := map[string]any{"type": "userMessage", "id": "answer", "clientId": params.ClientID, "content": params.Input}
			final := map[string]any{"type": "agentMessage", "id": "normal-final", "phase": "final_answer", "text": "done after answer"}
			turn["items"] = append(turn["items"].([]any), user, final)
			turn["status"], turn["completedAt"] = "completed", 1
			state.mu.Unlock()
			response["result"] = map[string]any{"turnId": params.Expected}
			if send(response) != nil || send(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": AppServerSession, "turnId": params.Expected, "item": user}}) != nil || send(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": AppServerSession, "turnId": params.Expected, "item": final}}) != nil || send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": AppServerSession, "turn": turn}}) != nil {
				return
			}
			continue
		case "turn/start":
			var params struct {
				Input []struct {
					Text string `json:"text"`
				} `json:"input"`
			}
			_ = json.Unmarshal(message.Params, &params)
			state.mu.Lock()
			turnID := fmt.Sprintf("00000000-0000-0000-0000-%012d", len(state.Turns)+1)
			text := "turn done"
			if len(params.Input) != 0 {
				text = params.Input[0].Text
			}
			item := map[string]any{"type": "agentMessage", "id": "message-" + turnID, "phase": "final_answer", "text": text}
			turn := map[string]any{"id": turnID, "status": "completed", "completedAt": 1, "items": []any{item}}
			if os.Getenv("STEERLINE_WORKER_FLOW_ASYNC") == "1" {
				item = map[string]any{"type": "agentMessage", "id": "async-question", "phase": "final_answer", "delivery": "async", "text": "Choose publisher", "questions": []any{map[string]any{"title": "Choose publisher", "options": []string{"worker", "host_service"}}}}
				turn = map[string]any{"id": turnID, "status": "inProgress", "items": []any{item}}
				state.Turns = append(state.Turns, turn)
				state.mu.Unlock()
				response["result"] = map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}
				if send(response) != nil || send(map[string]any{"method": "turn/started", "params": map[string]any{"threadId": AppServerSession, "turn": map[string]any{"id": turnID}}}) != nil || send(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": AppServerSession, "turnId": turnID, "item": item}}) != nil {
					return
				}
				continue
			}
			state.Turns = append(state.Turns, turn)
			state.mu.Unlock()
			response["result"] = map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}
			if send(response) != nil || send(map[string]any{"method": "turn/started", "params": map[string]any{"threadId": AppServerSession, "turn": map[string]any{"id": turnID}}}) != nil || send(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": AppServerSession, "turnId": turnID, "item": item}}) != nil || send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": AppServerSession, "turn": turn}}) != nil {
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
