package codexworker

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// HistoryClient only uses the public app-server protocol. It never opens
// Codex's database or rollout files and never resumes a thread while reading it.
type HistoryClient struct {
	input      io.WriteCloser
	lines      chan []byte
	done       chan struct{}
	proc       executionbackend.StopHandle
	mu         sync.Mutex
	next       int
	err        error
	userAgent  string
	serverHome string
	platformOS string
}

func OpenHistory(ctx context.Context, command executionbackend.Command, dir, home string, start func(*exec.Cmd) (executionbackend.StopHandle, error)) (*HistoryClient, error) {
	cmd := exec.Command(command.Executable, append(append([]string{}, command.Args...), "app-server", "--stdio")...)
	cmd.Dir = dir
	if home != "" {
		cmd.Env = historyEnvironment(cmd.Environ(), home)
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	cmd.Stderr = io.Discard
	proc, err := start(cmd)
	if err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	c := &HistoryClient{input: input, lines: make(chan []byte, 8), done: make(chan struct{}), proc: proc}
	go func() {
		defer close(c.done)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 64<<10), 16<<20)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			select {
			case c.lines <- line:
			case <-ctx.Done():
				proc.Kill()
			}
		}
		c.err = scanner.Err()
		if waitErr := cmd.Wait(); c.err == nil {
			c.err = waitErr
		}
		proc.Close()
	}()
	initialized, initErr := c.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "steerline_conversation", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true}})
	if initErr != nil {
		c.Close()
		return nil, initErr
	}
	var metadata struct {
		UserAgent  string `json:"userAgent"`
		CodexHome  string `json:"codexHome"`
		PlatformOS string `json:"platformOs"`
	}
	if err = json.Unmarshal(initialized, &metadata); err != nil || metadata.UserAgent == "" || metadata.CodexHome == "" || metadata.PlatformOS == "" || home != "" && canonicalPath(metadata.CodexHome) != canonicalPath(home) {
		c.Close()
		return nil, errors.New("Codex app-server identity, version or home is unverified")
	}
	c.userAgent, c.serverHome, c.platformOS = metadata.UserAgent, metadata.CodexHome, metadata.PlatformOS
	if _, err = io.WriteString(input, "{\"method\":\"initialized\",\"params\":{}}\n"); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func CheckHistory(ctx context.Context, command executionbackend.Command, dir string, start func(*exec.Cmd) (executionbackend.StopHandle, error)) error {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client, err := OpenHistory(probeCtx, command, dir, "", start)
	if err != nil {
		return err
	}
	client.Close()
	return nil
}

func (c *HistoryClient) Close() {
	_ = c.input.Close()
	c.proc.Kill()
	// A killed child must not retain a blocked pipe writer while we wait.
	for {
		select {
		case <-c.lines:
		case <-c.done:
			return
		}
	}
}

func (c *HistoryClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	request, err := json.Marshal(map[string]any{"id": c.next, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if _, err = c.input.Write(append(request, '\n')); err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.done:
			return nil, fmt.Errorf("Codex app-server ended before %s: %v", method, c.err)
		case line := <-c.lines:
			var reply struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(line, &reply); err != nil {
				return nil, fmt.Errorf("invalid Codex app-server response: %w", err)
			}
			if reply.ID != c.next {
				continue
			}
			if reply.Error != nil {
				return nil, fmt.Errorf("Codex %s (%d): %s", method, reply.Error.Code, reply.Error.Message)
			}
			if len(reply.Result) == 0 {
				return nil, errors.New("Codex app-server omitted result")
			}
			return reply.Result, nil
		}
	}
}

type HistoryThread struct {
	ID    string        `json:"id"`
	Cwd   string        `json:"cwd"`
	Turns []HistoryTurn `json:"turns"`
}

type HistoryTurn struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	Items       []json.RawMessage `json:"items"`
	Error       json.RawMessage   `json:"error"`
	ItemsView   string            `json:"itemsView"`
	CompletedAt *int64            `json:"completedAt"`
}

type HistoryPage struct {
	Data            []HistoryTurn `json:"data"`
	NextCursor      *string       `json:"nextCursor"`
	BackwardsCursor *string       `json:"backwardsCursor"`
}

const MaxHistoryCursorBytes = 4096

func (c *HistoryClient) ReadMetadata(ctx context.Context, id, cwd string) error {
	raw, err := c.call(ctx, "thread/read", map[string]any{"threadId": id, "includeTurns": false})
	if err != nil {
		return err
	}
	var result struct {
		Thread HistoryThread `json:"thread"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if result.Thread.ID != id || canonicalPath(result.Thread.Cwd) != canonicalPath(cwd) {
		return errors.New("Codex history session/workspace identity mismatch")
	}
	return nil
}

func (c *HistoryClient) ReadPage(ctx context.Context, id, cursor string) (HistoryPage, error) {
	params := map[string]any{"threadId": id, "limit": 1, "sortDirection": "asc", "itemsView": "full"}
	if cursor != "" {
		params["cursor"] = cursor
	}
	raw, err := c.call(ctx, "thread/turns/list", params)
	if err != nil {
		return HistoryPage{}, err
	}
	var page HistoryPage
	err = json.Unmarshal(raw, &page)
	if len(page.Data) > 1 {
		return HistoryPage{}, errors.New("Codex ignored the one-turn page limit")
	}
	for _, turn := range page.Data {
		if turn.ItemsView != "full" {
			return HistoryPage{}, errors.New("Codex did not return full turn items")
		}
	}
	for _, token := range []*string{page.NextCursor, page.BackwardsCursor} {
		if token != nil && len(*token) > MaxHistoryCursorBytes {
			return HistoryPage{}, errors.New("Codex turn cursor exceeds the bounded source limit")
		}
	}
	return page, err
}

func MapHistoryTurn(base executionvalues.ConversationEvent, turn HistoryTurn, sequence uint64) ([]executionvalues.ConversationWrite, error) {
	if turn.ID == "" {
		return nil, errors.New("Codex history turn omitted native identity")
	}
	base.Version = executionvalues.ConversationVersion
	base.Source = "codex-app-server"
	base.SourceRole = executionvalues.ConversationSourceCanonical
	base.TurnID, base.TurnQuality = turn.ID, executionvalues.IdentityNative
	base.SessionQuality = executionvalues.IdentityNative
	base.ItemQuality = executionvalues.IdentityUnknown
	base.SourceEventQuality = executionvalues.IdentityDerived
	base.Phase = executionvalues.MessagePhaseUnknown
	base.ReceivedAt = time.Now().UTC()
	base.Evidence = map[string]string{"capture": "thread/turns/list", "items_view": "full", "identity_rule": "native-session-turn-item-and-snapshot-v1"}
	start := base
	start.Type, start.SourceEventID = executionvalues.ConversationTurnStarted, "turn:"+turn.ID+":started"
	start.FinalAvailability = executionvalues.FinalUnknown
	writes := []executionvalues.ConversationWrite{{Event: start}}
	terminal := turn.CompletedAt != nil && (turn.Status == "completed" || turn.Status == "failed" || turn.Status == "interrupted")
	finalSeen := false
	unknownPhase := false
	missingFinalContent := false
	for _, raw := range turn.Items {
		var item struct {
			ID     string  `json:"id"`
			Type   string  `json:"type"`
			Text   *string `json:"text"`
			Phase  string  `json:"phase"`
			Status string  `json:"status"`
			Output string  `json:"aggregatedOutput"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		if item.ID == "" {
			return nil, errors.New("Codex history item omitted native identity")
		}
		e := base
		e.ItemID, e.ItemQuality = item.ID, executionvalues.IdentityNative
		e.ItemKey = conversationItemKey(e, item.ID)
		e.Attributes = map[string]any{"item_type": item.Type}
		e.SourceEventID = "turn:" + turn.ID + ":item:" + item.ID + ":" + digestValue(struct {
			Raw      json.RawMessage
			Terminal bool
		}{raw, terminal})
		e.SourceRevision = int64(sequence)
		write := executionvalues.ConversationWrite{Event: e, Raw: append([]byte(nil), raw...), BodySource: "codex-app-server"}
		switch item.Type {
		case "agentMessage":
			e.Type = executionvalues.ConversationMessageSnapshot
			if item.Text != nil {
				write.Body, write.BodyMode = []byte(*item.Text), "snapshot"
			}
			if item.Phase == string(executionvalues.MessagePhaseCommentary) || item.Phase == string(executionvalues.MessagePhaseFinalAnswer) {
				e.Phase = executionvalues.MessagePhase(item.Phase)
			} else {
				unknownPhase = true
			}
			if terminal {
				e.Type = executionvalues.ConversationMessageComplete
				if item.Text != nil {
					write.BodyMode = "complete"
				}
			}
			if e.Phase == executionvalues.MessagePhaseFinalAnswer {
				if item.Text == nil {
					missingFinalContent = true
					e.FinalAvailability = executionvalues.FinalUnknown
				} else {
					finalSeen = true
					if terminal {
						e.FinalAvailability = executionvalues.FinalAvailable
					} else {
						e.FinalAvailability = executionvalues.FinalPending
					}
				}
			}
		case "commandExecution", "mcpToolCall", "dynamicToolCall", "fileChange", "webSearch":
			e.Type, e.Status = executionvalues.ConversationToolCompleted, item.Status
			if item.Status == "inProgress" {
				e.Type = executionvalues.ConversationToolStarted
			}
			if item.Output != "" {
				write.Body, write.BodyMode = []byte(item.Output), "snapshot"
				if item.Status != "inProgress" {
					write.BodyMode = "complete"
				}
			}
		case "userMessage":
			e.Type = executionvalues.ConversationObservation
			e.Attributes["role"] = "user"
		default:
			e.Type = executionvalues.ConversationUnknown
		}
		write.Event = e
		writes = append(writes, write)
	}
	if terminal {
		end := base
		end.SourceEventID = "turn:" + turn.ID + ":status:" + turn.Status
		end.Status = turn.Status
		switch turn.Status {
		case "completed":
			end.Type = executionvalues.ConversationTurnCompleted
		case "failed":
			end.Type = executionvalues.ConversationTurnFailed
		case "interrupted":
			end.Type = executionvalues.ConversationTurnCancelled
		}
		end.FinalAvailability = executionvalues.FinalAbsent
		if finalSeen {
			end.FinalAvailability = executionvalues.FinalAvailable
		} else if unknownPhase || missingFinalContent {
			end.FinalAvailability = executionvalues.FinalUnknown
		}
		end.SourceEventID += ":final:" + string(end.FinalAvailability)
		// Full persisted history is the evidence for absent, not a timeout or idle TUI.
		end.Evidence = cloneStringMap(base.Evidence)
		end.Evidence["final_reconciliation"] = "full-native-turn-snapshot"
		raw, _ := json.Marshal(map[string]any{"id": turn.ID, "status": turn.Status, "error": turn.Error})
		writes = append(writes, executionvalues.ConversationWrite{Event: end, Raw: raw})
	}
	for i := range writes {
		writes[i].Event.SourceSeq = sequence + uint64(i)
	}
	return writes, nil
}

func canonicalPath(s string) string {
	s = filepath.Clean(s)
	if runtime.GOOS == "windows" {
		return strings.ToLower(s)
	}
	return s
}

func historyEnvironment(env []string, home string) []string {
	out := []string{}
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "CODEX_HOME") {
			out = append(out, entry)
		}
	}
	return append(out, "CODEX_HOME="+home)
}

func digestValue(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func conversationItemKey(e executionvalues.ConversationEvent, id string) string {
	if id == "" {
		return ""
	}
	e.ItemID = id
	h := sha256.Sum256([]byte(e.ItemIdentity()))
	return hex.EncodeToString(h[:])
}

func cloneStringMap(v map[string]string) map[string]string {
	out := map[string]string{}
	for k, x := range v {
		out[k] = x
	}
	return out
}

func (c *HistoryClient) UserAgent() string { return c.userAgent }

func (c *HistoryClient) BootstrapSession(ctx context.Context, cwd, sandbox, instruction string) (string, error) {
	params := map[string]any{"cwd": cwd, "ephemeral": false}
	if sandbox != "" {
		params["sandbox"] = sandbox
	}
	raw, err := c.call(ctx, "thread/start", params)
	if err != nil {
		return "", err
	}
	var started struct {
		Thread HistoryThread `json:"thread"`
	}
	if err = json.Unmarshal(raw, &started); err != nil {
		return "", err
	}
	if !ValidNativeID(started.Thread.ID) {
		return "", errors.New("Codex session lacks native identity")
	}
	_, err = c.call(ctx, "thread/inject_items", map[string]any{"threadId": started.Thread.ID, "items": []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]string{"type": "input_text", "text": instruction}}}}})
	return started.Thread.ID, err
}

func ResumeCommand(command executionbackend.Command, sessionID string, args []string, prompt, cwd string) executionbackend.LaunchCommand {
	argv := append(append([]string{}, command.Args...), "resume", "--include-non-interactive", sessionID)
	argv = append(argv, args...)
	if prompt != "" {
		argv = append(argv, prompt)
	}
	return executionbackend.LaunchCommand{Executable: command.Executable, Args: argv, WorkingDirectory: cwd}
}

func ReaderCommand(command executionbackend.Command, profile string) executionbackend.Command {
	command.Args = append([]string(nil), command.Args...)
	if profile != "" {
		command.Args = append(command.Args, "--profile", profile)
	}
	return command
}

func EnvironmentHome() string {
	if path := os.Getenv("CODEX_HOME"); path != "" {
		return path
	}
	if path, err := os.UserHomeDir(); err == nil {
		return filepath.Join(path, ".codex")
	}
	return ""
}

type HistoryTurnFacts struct {
	Terminal      bool
	HasFinal      bool
	HasPhase      bool
	CompleteFinal bool
}

func InspectHistoryTurn(turn HistoryTurn) (HistoryTurnFacts, error) {
	facts := HistoryTurnFacts{Terminal: turn.CompletedAt != nil && (turn.Status == "completed" || turn.Status == "failed" || turn.Status == "interrupted")}
	for _, raw := range turn.Items {
		var item struct {
			Type  string  `json:"type"`
			Phase string  `json:"phase"`
			Text  *string `json:"text"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return facts, err
		}
		if item.Type == "agentMessage" {
			facts.HasPhase = facts.HasPhase || item.Phase == string(executionvalues.MessagePhaseCommentary) || item.Phase == string(executionvalues.MessagePhaseFinalAnswer)
			if item.Phase == string(executionvalues.MessagePhaseFinalAnswer) && item.Text != nil {
				facts.HasFinal = true
				facts.CompleteFinal = turn.CompletedAt != nil
			}
		}
	}
	return facts, nil
}
