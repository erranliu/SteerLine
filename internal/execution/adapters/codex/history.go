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
	stderr, err := cmd.StderrPipe()
	if err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	proc, err := start(cmd)
	if err != nil {
		input.Close()
		output.Close()
		stderr.Close()
		return nil, err
	}
	diagnostic := make(chan string, 1)
	go func() {
		raw, _ := io.ReadAll(io.LimitReader(stderr, 8192))
		_, _ = io.Copy(io.Discard, stderr)
		diagnostic <- strings.TrimSpace(string(raw))
	}()
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
		if detail := <-diagnostic; detail != "" {
			return nil, fmt.Errorf("%w: %s", initErr, detail)
		}
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
	// Let the public reader close its native stores after stdin EOF. It never
	// owns a model turn; forced termination is only the bounded fallback.
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-c.lines:
		case <-c.done:
			return
		case <-timer.C:
			c.proc.Kill()
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

// ReadThread reads the original durable thread without resuming or starting it.
func (c *HistoryClient) ReadThread(ctx context.Context, id, cwd string) (HistoryThread, error) {
	raw, err := c.call(ctx, "thread/read", map[string]any{"threadId": id, "includeTurns": true})
	if err != nil {
		return HistoryThread{}, err
	}
	var result struct {
		Thread HistoryThread `json:"thread"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return HistoryThread{}, err
	}
	return verifiedHistoryThread(result.Thread, id, cwd)
}

func verifiedHistoryThread(thread HistoryThread, id, cwd string) (HistoryThread, error) {
	if thread.ID != id || !sameWorkspace(thread.Cwd, cwd) || len(thread.Turns) > 4096 {
		return HistoryThread{}, errors.New("Codex recovery thread/workspace or bounded history is unverified")
	}
	return thread, nil
}

func historyThreadIdle(thread HistoryThread, stoppedTurn string) bool {
	if len(thread.Turns) == 0 {
		return false
	}
	for _, turn := range thread.Turns {
		// A killed owned tree can leave its exact turn without completion timing,
		// even after native history labels it interrupted. Other native/manual
		// turns still require terminal proof; this does not adopt an old result.
		if turn.ID != "" && turn.ID == stoppedTurn && (turn.Status == "inProgress" || turn.Status == "completed" || turn.Status == "failed" || turn.Status == "interrupted") {
			continue
		}
		if turn.ID == "" || turn.CompletedAt == nil || (turn.Status != "completed" && turn.Status != "failed" && turn.Status != "interrupted") {
			return false
		}
	}
	return true
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
	base.Version, base.Source, base.SourceRole = executionvalues.ConversationVersion, "codex-app-server", executionvalues.ConversationSourceCanonical
	base.TurnID, base.TurnQuality, base.SessionQuality = turn.ID, executionvalues.IdentityNative, executionvalues.IdentityNative
	base.ItemQuality, base.SourceEventQuality = executionvalues.IdentityUnknown, executionvalues.IdentityDerived
	base.Phase, base.ReceivedAt = executionvalues.MessagePhaseUnknown, time.Now().UTC()
	base.Evidence = map[string]string{"capture": "thread/turns/list", "identity_rule": "native-session-turn-item-and-final-v1"}
	start := base
	start.Type, start.SourceEventID, start.FinalAvailability = executionvalues.ConversationTurnStarted, "turn:"+turn.ID+":started", executionvalues.FinalUnknown
	start.SourceSeq = sequence
	writes := []executionvalues.ConversationWrite{{Event: start}}
	terminal := turn.CompletedAt != nil && (turn.Status == "completed" || turn.Status == "failed" || turn.Status == "interrupted")
	if !terminal {
		return writes, nil
	}
	finalSeen, unknownPhase, missingFinal := false, false, false
	for _, raw := range turn.Items {
		var item struct {
			ID       string  `json:"id"`
			Type     string  `json:"type"`
			Text     *string `json:"text"`
			Phase    string  `json:"phase"`
			Delivery string  `json:"delivery"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		if item.Type != "agentMessage" || item.Delivery == "async" {
			continue
		}
		if item.Phase != string(executionvalues.MessagePhaseFinalAnswer) {
			unknownPhase = unknownPhase || item.Phase != string(executionvalues.MessagePhaseCommentary)
			continue
		}
		if item.ID == "" {
			return nil, errors.New("Codex final omitted native item identity")
		}
		if item.Text == nil {
			missingFinal = true
			continue
		}
		event := base
		event.Type, event.Phase, event.FinalAvailability = executionvalues.ConversationMessageComplete, executionvalues.MessagePhaseFinalAnswer, executionvalues.FinalAvailable
		event.ItemID, event.ItemQuality = item.ID, executionvalues.IdentityNative
		event.ItemKey = conversationItemKey(event, item.ID)
		event.SourceEventID = "turn:" + turn.ID + ":final:" + item.ID + ":" + digestValue(*item.Text)
		writes = append(writes, executionvalues.ConversationWrite{Event: event, Body: []byte(*item.Text), BodyMode: "complete", BodySource: "codex-app-server"})
		finalSeen = true
	}
	end := base
	end.Status, end.FinalAvailability = turn.Status, executionvalues.FinalAbsent
	if len(turn.Error) > 0 && string(turn.Error) != "null" {
		end.Diagnostic = clip(string(turn.Error), 2000)
	}
	switch turn.Status {
	case "completed":
		end.Type = executionvalues.ConversationTurnCompleted
	case "failed":
		end.Type = executionvalues.ConversationTurnFailed
	case "interrupted":
		end.Type = executionvalues.ConversationTurnCancelled
	}
	if finalSeen {
		end.FinalAvailability = executionvalues.FinalAvailable
	} else if unknownPhase || missingFinal {
		end.FinalAvailability = executionvalues.FinalUnknown
	}
	end.SourceEventID = "turn:" + turn.ID + ":status:" + turn.Status + ":final:" + string(end.FinalAvailability)
	writes = append(writes, executionvalues.ConversationWrite{Event: end})
	if missingFinal || !finalSeen {
		gap := base
		gap.Type, gap.Status, gap.FinalAvailability = executionvalues.ConversationGap, "final_unavailable", end.FinalAvailability
		gap.SourceEventID = "turn:" + turn.ID + ":final-unavailable:" + string(end.FinalAvailability)
		gap.Diagnostic = "The native completed turn has no verifiable complete final reply; inspect its original session."
		writes = append(writes, executionvalues.ConversationWrite{Event: gap})
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
			Type     string  `json:"type"`
			Phase    string  `json:"phase"`
			Delivery string  `json:"delivery"`
			Text     *string `json:"text"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return facts, err
		}
		if item.Type == "agentMessage" && item.Delivery != "async" {
			facts.HasPhase = facts.HasPhase || item.Phase == string(executionvalues.MessagePhaseCommentary) || item.Phase == string(executionvalues.MessagePhaseFinalAnswer)
			if item.Phase == string(executionvalues.MessagePhaseFinalAnswer) && item.Text != nil {
				facts.HasFinal = true
				facts.CompleteFinal = turn.CompletedAt != nil
			}
		}
	}
	return facts, nil
}
