package steerline

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// codexHistoryClient only uses the public app-server protocol. It never opens
// Codex's database or rollout files and never resumes a thread while reading it.
type codexHistoryClient struct {
	input      io.WriteCloser
	lines      chan []byte
	done       chan struct{}
	proc       process
	mu         sync.Mutex
	next       int
	err        error
	userAgent  string
	serverHome string
	platformOS string
}

func openCodexHistory(ctx context.Context, command Command, dir, home string) (*codexHistoryClient, error) {
	cmd := exec.Command(command.Executable, append(append([]string{}, command.Args...), "app-server", "--stdio")...)
	cmd.Dir = dir
	if home != "" {
		cmd.Env = overrideEnv(cmd.Environ(), map[string]string{"CODEX_HOME": home})
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
	proc, err := startProcess(cmd)
	if err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	c := &codexHistoryClient{input: input, lines: make(chan []byte, 8), done: make(chan struct{}), proc: proc}
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
	if err = json.Unmarshal(initialized, &metadata); err != nil || metadata.UserAgent == "" || metadata.CodexHome == "" || metadata.PlatformOS == "" || home != "" && canonical(metadata.CodexHome) != canonical(home) {
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

func (c *codexHistoryClient) Close() {
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

func (c *codexHistoryClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
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

type codexHistoryThread struct {
	ID    string             `json:"id"`
	Cwd   string             `json:"cwd"`
	Turns []codexHistoryTurn `json:"turns"`
}
type codexHistoryTurn struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	Items       []json.RawMessage `json:"items"`
	Error       json.RawMessage   `json:"error"`
	ItemsView   string            `json:"itemsView"`
	CompletedAt *int64            `json:"completedAt"`
}
type codexHistoryPage struct {
	Data            []codexHistoryTurn `json:"data"`
	NextCursor      *string            `json:"nextCursor"`
	BackwardsCursor *string            `json:"backwardsCursor"`
}

const maxCodexHistoryCursorBytes = 4096

func (c *codexHistoryClient) readMetadata(ctx context.Context, id, cwd string) error {
	raw, err := c.call(ctx, "thread/read", map[string]any{"threadId": id, "includeTurns": false})
	if err != nil {
		return err
	}
	var result struct {
		Thread codexHistoryThread `json:"thread"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if result.Thread.ID != id || canonical(result.Thread.Cwd) != canonical(cwd) {
		return errors.New("Codex history session/workspace identity mismatch")
	}
	return nil
}

func (c *codexHistoryClient) readPage(ctx context.Context, id, cursor string) (codexHistoryPage, error) {
	params := map[string]any{"threadId": id, "limit": 1, "sortDirection": "asc", "itemsView": "full"}
	if cursor != "" {
		params["cursor"] = cursor
	}
	raw, err := c.call(ctx, "thread/turns/list", params)
	if err != nil {
		return codexHistoryPage{}, err
	}
	var page codexHistoryPage
	err = json.Unmarshal(raw, &page)
	if len(page.Data) > 1 {
		return codexHistoryPage{}, errors.New("Codex ignored the one-turn page limit")
	}
	for _, turn := range page.Data {
		if turn.ItemsView != "full" {
			return codexHistoryPage{}, errors.New("Codex did not return full turn items")
		}
	}
	for _, token := range []*string{page.NextCursor, page.BackwardsCursor} {
		if token != nil && len(*token) > maxCodexHistoryCursorBytes {
			return codexHistoryPage{}, errors.New("Codex turn cursor exceeds the bounded source limit")
		}
	}
	return page, err
}

func mapCodexHistoryTurn(base ConversationEvent, turn codexHistoryTurn, sequence uint64) ([]ConversationWrite, error) {
	if turn.ID == "" {
		return nil, errors.New("Codex history turn omitted native identity")
	}
	base.Version = ConversationVersion
	base.Source = "codex-app-server"
	base.TurnID, base.TurnQuality = turn.ID, IdentityNative
	base.SessionQuality = IdentityNative
	base.ItemQuality = IdentityUnknown
	base.SourceEventQuality = IdentityDerived
	base.Phase = MessagePhaseUnknown
	base.ReceivedAt = time.Now().UTC()
	base.Evidence = map[string]string{"capture": "thread/turns/list", "items_view": "full", "identity_rule": "native-session-turn-item-and-snapshot-v1"}
	start := base
	start.Type, start.SourceEventID = ConversationTurnStarted, "turn:"+turn.ID+":started"
	start.FinalAvailability = FinalUnknown
	writes := []ConversationWrite{{Event: start}}
	terminal := turn.CompletedAt != nil && (turn.Status == "completed" || turn.Status == "failed" || turn.Status == "interrupted")
	finalSeen := false
	unknownPhase := false
	for _, raw := range turn.Items {
		var item struct {
			ID     string `json:"id"`
			Type   string `json:"type"`
			Text   string `json:"text"`
			Phase  string `json:"phase"`
			Status string `json:"status"`
			Output string `json:"aggregatedOutput"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		if item.ID == "" {
			return nil, errors.New("Codex history item omitted native identity")
		}
		e := base
		e.ItemID, e.ItemQuality = item.ID, IdentityNative
		e.ItemKey = conversationItemKey(e, item.ID)
		e.Attributes = map[string]any{"item_type": item.Type}
		e.SourceEventID = "turn:" + turn.ID + ":item:" + item.ID + ":" + digestValue(struct {
			Raw      json.RawMessage
			Terminal bool
		}{raw, terminal})
		e.SourceRevision = int64(sequence)
		write := ConversationWrite{Event: e, Raw: append([]byte(nil), raw...), BodySource: "codex-app-server"}
		switch item.Type {
		case "agentMessage":
			e.Type = ConversationMessageSnapshot
			write.Body, write.BodyMode = []byte(item.Text), "snapshot"
			if item.Phase == string(MessagePhaseCommentary) || item.Phase == string(MessagePhaseFinalAnswer) {
				e.Phase = MessagePhase(item.Phase)
			} else {
				unknownPhase = true
			}
			if terminal {
				e.Type = ConversationMessageComplete
				write.BodyMode = "complete"
			}
			if e.Phase == MessagePhaseFinalAnswer {
				finalSeen = true
				if terminal {
					e.FinalAvailability = FinalAvailable
				} else {
					e.FinalAvailability = FinalPending
				}
			}
		case "commandExecution", "mcpToolCall", "dynamicToolCall", "fileChange", "webSearch":
			e.Type, e.Status = ConversationToolCompleted, item.Status
			if item.Status == "inProgress" {
				e.Type = ConversationToolStarted
			}
			if item.Output != "" {
				write.Body, write.BodyMode = []byte(item.Output), "snapshot"
				if item.Status != "inProgress" {
					write.BodyMode = "complete"
				}
			}
		case "userMessage":
			e.Type = ConversationObservation
			e.Attributes["role"] = "user"
		default:
			e.Type = ConversationUnknown
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
			end.Type = ConversationTurnCompleted
		case "failed":
			end.Type = ConversationTurnFailed
		case "interrupted":
			end.Type = ConversationTurnCancelled
		}
		end.FinalAvailability = FinalAbsent
		if finalSeen {
			end.FinalAvailability = FinalAvailable
		} else if unknownPhase {
			end.FinalAvailability = FinalUnknown
		}
		end.SourceEventID += ":final:" + string(end.FinalAvailability)
		// Full persisted history is the evidence for absent, not a timeout or idle TUI.
		end.Evidence = cloneStringMap(base.Evidence)
		end.Evidence["final_reconciliation"] = "full-native-turn-snapshot"
		raw, _ := json.Marshal(map[string]any{"id": turn.ID, "status": turn.Status, "error": turn.Error})
		writes = append(writes, ConversationWrite{Event: end, Raw: raw})
	}
	for i := range writes {
		writes[i].Event.SourceSeq = sequence + uint64(i)
	}
	return writes, nil
}
