package controlled

import (
	"bufio"
	"context"
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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
	"github.com/google/uuid"
)

func argument(args []string, key string) string {
	for i, arg := range args {
		if arg == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func Main(args []string) error {
	if len(args) > 0 && args[0] == "git-server" {
		return gitServer(argument(args, "--root"), argument(args, "--address-file"), argument(args, "--e2e-state"))
	}
	path := argument(args, "--e2e-state")
	if path == "" {
		path = os.Getenv("STEERLINE_E2E_CONTROLLED_STATE")
	}
	if path == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("helper requires absolute --e2e-state")
	}
	if len(args) > 0 && args[0] == "git" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		out, err := gitBytes(ctx, cwd, args[1:]...)
		if err == nil {
			_, err = os.Stdout.Write(out)
		}
		return err
	}
	if len(args) > 0 && (args[0] == "api" || args[0] == "pr") {
		return githubCommand(path, args)
	}
	for _, a := range args {
		if a == "--version" {
			fmt.Println(native.SupportedCodexVersion)
			return nil
		}
	}
	for _, a := range args {
		if a == "--help" {
			fmt.Println("app-server --listen --stdio --strict-config --sandbox --ask-for-approval on-request --config")
			return nil
		}
	}
	if len(args) > 0 && args[0] == "provider" {
		s, err := Read(path)
		if err != nil {
			return err
		}
		pr, err := ProviderPR(s, argument(args, "--pr-url"))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(pr)
	}
	mu := &sync.Mutex{}
	handle := func(raw []byte, send func(any) error) error {
		mu.Lock()
		defer mu.Unlock()
		return handleRPC(path, raw, send)
	}
	if argument(args, "--listen") != "" {
		u, err := url.Parse(argument(args, "--listen"))
		if err != nil || u.Scheme != "ws" {
			return fmt.Errorf("invalid WebSocket endpoint")
		}
		listener, err := net.Listen("tcp", u.Host)
		if err != nil {
			return err
		}
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return err
			}
			go func() {
				defer conn.Close()
				if err := websocket(conn, handle); err != nil && err != io.EOF {
					fmt.Fprintln(os.Stderr, err)
				}
			}()
		}
	}
	// Public history probes use the stdio app-server transport.
	for _, a := range args {
		if a == "app-server" {
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Buffer(make([]byte, 4096), 16<<20)
			for scanner.Scan() {
				if err := handle(scanner.Bytes(), func(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }); err != nil {
					return err
				}
			}
			return scanner.Err()
		}
	}
	return fmt.Errorf("unsupported controlled helper invocation")
}

func handleRPC(path string, raw []byte, send func(any) error) error {
	var message struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		return err
	}
	if len(message.ID) == 0 {
		return nil
	}
	response := map[string]any{"id": message.ID}
	s, err := Read(path)
	if os.IsNotExist(err) {
		s = State{}
	} else if err != nil {
		return err
	}
	switch message.Method {
	case "initialize":
		response["result"] = map[string]string{"userAgent": "controlled-e2e", "codexHome": filepath.Dir(path), "platformOs": "windows"}
	case "thread/start":
		var params struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return err
		}
		if params.Cwd == "" {
			return fmt.Errorf("missing assigned workspace")
		}
		if s.Session != "" {
			if s.Threads == nil {
				s.Threads = map[string]ThreadState{}
			}
			s.Threads[s.Session] = ThreadState{Cwd: s.Cwd, Turns: s.Turns}
		}
		s.Cwd, s.Session = params.Cwd, uuid.NewString()
		s.Sessions++
		s.Turns = nil
		if err := Write(path, s); err != nil {
			return err
		}
		response["result"] = map[string]any{"thread": map[string]any{"id": s.Session, "cwd": s.Cwd}}
	case "thread/resume", "thread/read":
		var params struct {
			ThreadID string `json:"threadId"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return err
		}
		thread := ThreadState{Cwd: s.Cwd, Turns: s.Turns}
		if s.Session == "" || params.ThreadID != s.Session {
			var ok bool
			thread, ok = s.Threads[params.ThreadID]
			if !ok {
				return fmt.Errorf("unknown controlled session")
			}
		}
		if message.Method == "thread/resume" && len(thread.Turns) == 0 {
			response["error"] = map[string]any{"code": -32600, "message": "no rollout found for thread id " + params.ThreadID}
			break
		}
		response["result"] = map[string]any{"thread": map[string]any{"id": params.ThreadID, "cwd": thread.Cwd, "turns": thread.Turns}}
	case "turn/start":
		var params struct {
			ThreadID string `json:"threadId"`
			Input    []struct {
				Text string `json:"text"`
			} `json:"input"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return err
		}
		if s.Session == "" || params.ThreadID != s.Session || len(params.Input) == 0 {
			return fmt.Errorf("invalid controlled dispatch")
		}
		steps := []string{"Implement the following Spec and Plan.", "Format the preceding implementation result as JSON.", "Push and create or reuse a PR using the current checked-out branch."}
		index := len(s.Turns)
		if index >= len(steps) || !strings.HasPrefix(params.Input[0].Text, steps[index]) {
			return fmt.Errorf("unexpected duplicate or out-of-order staged dispatch")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		final := "Controlled candidate implemented and committed locally; no publication performed."
		candidateSHA := ""
		switch index {
		case 0:
			result := s.ResultText
			if result == "" {
				result = "controlled E2E result\n"
			}
			if err := os.WriteFile(filepath.Join(s.Cwd, "result.txt"), []byte(result), 0600); err != nil {
				return err
			}
			if _, err := Git(ctx, s.Cwd, "add", "result.txt"); err != nil {
				return err
			}
			if _, err := Git(ctx, s.Cwd, "commit", "-m", "controlled worker candidate"); err != nil {
				return err
			}
			var err error
			candidateSHA, err = Git(ctx, s.Cwd, "rev-parse", "HEAD")
			if err != nil {
				return err
			}
		case 1:
			// Report only the already recorded execute result; do no business I/O.
			candidateSHA, _ = s.Turns[0]["fixture_candidate_sha"].(string)
			if candidateSHA == "" {
				return fmt.Errorf("source candidate missing")
			}
			raw, _ := json.Marshal(map[string]any{"outcome": "done", "summary": "Controlled source implementation completed"})
			final = string(raw)
		case 2:
			branch, err := Git(ctx, s.Cwd, "symbolic-ref", "--short", "HEAD")
			if err != nil {
				return err
			}
			if _, err := Git(ctx, s.Cwd, "push", "origin", branch); err != nil {
				return err
			}
			if err := githubCommand(path, []string{"pr", "create", "--repo", s.RepositoryName(), "--base", s.BranchName(), "--head", branch, "--draft", "--title", "controlled worker result", "--body", "worker-owned publication"}); err != nil {
				return err
			}
			published, err := Read(path)
			if err != nil {
				return err
			}
			s.PR, s.PRs = published.PR, published.PRs
			raw, _ := json.Marshal(map[string]any{"outcome": "done", "outputs": map[string]string{"pr_url": s.PR.URL}, "summary": "Controlled candidate pushed and PR created"})
			final = string(raw)
		}
		s.Dispatches++
		turnID := uuid.NewString()
		item := map[string]any{"type": "agentMessage", "id": "message-" + turnID, "phase": "final_answer", "text": final}
		turn := map[string]any{"id": turnID, "status": "completed", "completedAt": time.Now().Unix(), "items": []any{item}, "fixture_step": steps[index]}
		if candidateSHA != "" {
			turn["fixture_candidate_sha"] = candidateSHA
		}
		s.Turns = append(s.Turns, turn)
		if err := Write(path, s); err != nil {
			return err
		}
		response["result"] = map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}
		for _, v := range []any{response, map[string]any{"method": "turn/started", "params": map[string]any{"threadId": s.Session, "turn": map[string]any{"id": turnID}}}, map[string]any{"method": "item/completed", "params": map[string]any{"threadId": s.Session, "turnId": turnID, "item": item}}, map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": s.Session, "turn": turn}}} {
			if err := send(v); err != nil {
				return err
			}
		}
		return nil
	default:
		response["error"] = map[string]any{"code": -32601, "message": "unsupported controlled method: " + message.Method}
	}
	return send(response)
}

// Minimal text-frame transport, matching the actual app-server client.
func websocket(conn net.Conn, handle func([]byte, func(any) error) error) error {
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return err
	}
	if !strings.EqualFold(request.Header.Get("Upgrade"), "websocket") {
		return fmt.Errorf("WebSocket upgrade required")
	}
	accept := sha1.Sum([]byte(request.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if _, err = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "+base64.StdEncoding.EncodeToString(accept[:])+"\r\n\r\n"); err != nil {
		return err
	}
	send := func(v any) error {
		payload, err := json.Marshal(v)
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
			frame = append(frame, 127)
			size := make([]byte, 8)
			binary.BigEndian.PutUint64(size, uint64(len(payload)))
			frame = append(frame, size...)
		}
		_, err = conn.Write(append(frame, payload...))
		return err
	}
	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(reader, header); err != nil {
			return err
		}
		if header[0]&0xf == 8 {
			return nil
		}
		if header[0] != 0x81 || header[1]&0x80 == 0 {
			return fmt.Errorf("unsupported WebSocket frame")
		}
		size := uint64(header[1] & 0x7f)
		if size == 126 {
			var n uint16
			if err := binary.Read(reader, binary.BigEndian, &n); err != nil {
				return err
			}
			size = uint64(n)
		} else if size == 127 {
			if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
				return err
			}
		}
		if size > 16<<20 {
			return fmt.Errorf("oversized controlled request")
		}
		mask := make([]byte, 4)
		if _, err := io.ReadFull(reader, mask); err != nil {
			return err
		}
		payload := make([]byte, int(size))
		if _, err := io.ReadFull(reader, payload); err != nil {
			return err
		}
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
		if err := handle(payload, send); err != nil {
			return err
		}
	}
}
