package integration

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

	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/testkit/protocol"
)

func fakeIntegrationCommand(args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "terminal-orca":
		return fakeTerminalOrca(args[1], args[2:])
	case "staged-pr":
		raw, err := os.ReadFile(os.Getenv("STEERLINE_STAGE_PROVIDER"))
		if err != nil {
			return 2
		}
		fmt.Print(string(raw))
		return 0
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
		fmt.Println("--strict-config --sandbox --ask-for-approval on-request --config")
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
				ThreadID     string `json:"threadId"`
				Cursor       string `json:"cursor"`
				Limit        int    `json:"limit"`
				ItemsView    string `json:"itemsView"`
				IncludeTurns bool   `json:"includeTurns"`
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
				thread := map[string]any{"id": integrationAppServerSession, "cwd": history.Cwd}
				if request.Params.IncludeTurns {
					thread["turns"] = history.Turns
				}
				response["result"] = map[string]any{"thread": thread}
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
	if path := os.Getenv("STEERLINE_INTEGRATION_HISTORY_FILE"); path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			var saved struct {
				Turns []map[string]any `json:"turns"`
			}
			if json.Unmarshal(raw, &saved) == nil {
				state.turns = saved.Turns
			}
		}
	}
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
			cwd, _ := os.Getwd()
			response["result"] = map[string]any{"thread": map[string]any{"id": integrationAppServerSession, "cwd": cwd, "turns": turns}}
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
				if strings.HasPrefix(params.Input[0].Text, "Format the preceding implementation result as JSON.") {
					text = `{"outcome":"needs_input","summary":"Original fixture execution completed","question":"Continue this original candidate?"}`
				}
			}
			if os.Getenv("STEERLINE_STAGE_PROVIDER") != "" && len(params.Input) != 0 {
				cwd, _ := os.Getwd()
				staged, err := testprotocol.StagedBusiness(cwd, params.Input[0].Text, os.Getenv("STEERLINE_STAGE_PROVIDER"), os.Getenv("STEERLINE_FAKE_GIT_FETCH_REMOTE"))
				if err != nil {
					state.mu.Unlock()
					return
				}
				text = staged
			}
			item := map[string]any{"type": "agentMessage", "id": "message-" + turnID, "phase": "final_answer", "text": text}
			turn := map[string]any{"id": turnID, "status": "completed", "completedAt": len(state.turns) + 1, "items": []any{item}}
			hold := os.Getenv("STEERLINE_INTEGRATION_HOLD_FIRST_TURN") == "1" && len(state.turns) == 0
			if hold {
				turn = map[string]any{"id": turnID, "status": "inProgress", "items": []any{}}
			}
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
			if hold {
				if send(response) != nil || send(map[string]any{"method": "turn/started", "params": map[string]any{"threadId": integrationAppServerSession, "turn": map[string]any{"id": turnID}}}) != nil {
					return
				}
				continue
			}
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

func TestMain(m *testing.M) {

	if os.Getenv("STEERLINE_LIFECYCLE_EXIT_EARLY") == "1" {
		os.Exit(23)
	}
	// The process-port output probe needs no project, CLI build or profiles.
	if os.Getenv("STEERLINE_TM06_RESOURCE_HELPER") == "1" {
		os.Exit(m.Run())
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
		fmt.Println("--strict-config --sandbox --ask-for-approval on-request --config")
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
	// Direct go test runs must not write fixture profiles into the real runtime.
	fixtureHome, err := os.MkdirTemp("", "steerline-integration-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = os.Setenv("STEERLINE_HOME", fixtureHome); err != nil {
		_ = os.RemoveAll(fixtureHome)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cleanup, err := prepareIntegrationCLI()
	if err != nil {
		_ = os.RemoveAll(fixtureHome)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	_ = os.RemoveAll(fixtureHome)
	os.Exit(code)
}
func arg(args []string, key string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}
