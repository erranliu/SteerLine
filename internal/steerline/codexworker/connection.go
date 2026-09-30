package codexworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func ConnectAppServer(ctx context.Context, endpoint string) (*codexapp.Client, error) {
	deadline := time.Now().Add(10 * time.Second)
	var err error
	for time.Now().Before(deadline) {
		var client *codexapp.Client
		client, err = codexapp.Open(ctx, endpoint)
		if err == nil {
			return client, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("app-server did not become ready at its fixed endpoint: %w", err)
}

type AskReply struct {
	Endpoint  string
	Workspace string
	ThreadID  string
	RequestID json.RawMessage
	executionapi.Answer
}

type ThreadReceipt struct {
	Thread struct {
		ID  string `json:"id"`
		Cwd string `json:"cwd"`
	} `json:"thread"`
}

func StartThread(ctx context.Context, client *codexapp.Client, params map[string]any, workspace string) (ThreadReceipt, error) {
	var receipt ThreadReceipt
	if err := client.Call(ctx, "thread/start", params, &receipt); err != nil {
		return ThreadReceipt{}, err
	}
	if !nativeID.MatchString(receipt.Thread.ID) || !sameWorkspace(receipt.Thread.Cwd, workspace) {
		return ThreadReceipt{}, errors.New("app-server created a different thread/workspace")
	}
	return receipt, nil
}

func ResumeThread(ctx context.Context, client *codexapp.Client, threadID, workspace string) error {
	var receipt ThreadReceipt
	if err := client.Call(ctx, "thread/resume", map[string]any{"threadId": threadID, "cwd": workspace, "excludeTurns": true}, &receipt); err != nil {
		return err
	}
	if receipt.Thread.ID != threadID || !sameWorkspace(receipt.Thread.Cwd, workspace) {
		return errors.New("app-server resumed a different thread/workspace")
	}
	return nil
}

type TurnReceipt struct {
	Turn struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"turn"`
}

func StartTurn(ctx context.Context, client *codexapp.Client, params map[string]any) (TurnReceipt, error) {
	var receipt TurnReceipt
	if err := client.Call(ctx, "turn/start", params, &receipt); err != nil {
		return TurnReceipt{}, err
	}
	if !nativeID.MatchString(receipt.Turn.ID) || receipt.Turn.Status == "" {
		return TurnReceipt{}, errors.New("app-server omitted native turn identity/status")
	}
	return receipt, nil
}

// SendAskReply verifies the original thread and workspace before replying to
// the exact native request ID. Retrying an uncertain result remains the
// service writer's responsibility.
func SendAskReply(ctx context.Context, reply AskReply) error {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client, err := codexapp.Open(probeCtx, reply.Endpoint)
	if err != nil {
		return err
	}
	defer client.Close()
	var resumed struct {
		Thread struct {
			ID  string `json:"id"`
			Cwd string `json:"cwd"`
		} `json:"thread"`
	}
	if err := client.Call(probeCtx, "thread/resume", map[string]any{"threadId": reply.ThreadID, "cwd": reply.Workspace, "excludeTurns": true}, &resumed); err != nil {
		return err
	}
	if resumed.Thread.ID != reply.ThreadID || !sameWorkspace(resumed.Thread.Cwd, reply.Workspace) {
		return errors.New("original Codex question thread/workspace changed before reply")
	}
	return client.ReplyRequest(reply.RequestID, codexapp.AskToolResponse(reply.Answers))
}

func sameWorkspace(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.ToLower(a) == strings.ToLower(b)
	}
	return a == b
}

type TurnSnapshot struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	Items       []json.RawMessage `json:"items"`
	CompletedAt *int64            `json:"completedAt"`
}

// ReadTurnSnapshot verifies a native turn in the original thread and
// workspace before recovery decides whether to attach or settle it.
func ReadTurnSnapshot(ctx context.Context, client *codexapp.Client, threadID, workspace, turnID string) (TurnSnapshot, error) {
	var resumed struct {
		Thread struct {
			ID  string `json:"id"`
			Cwd string `json:"cwd"`
		} `json:"thread"`
	}
	if err := client.Call(ctx, "thread/resume", map[string]any{"threadId": threadID, "cwd": workspace, "excludeTurns": true}, &resumed); err != nil {
		return TurnSnapshot{}, err
	}
	if resumed.Thread.ID != threadID || !sameWorkspace(resumed.Thread.Cwd, workspace) {
		return TurnSnapshot{}, errors.New("resumed app-server thread/workspace changed")
	}
	var read struct {
		Thread struct {
			ID    string            `json:"id"`
			Cwd   string            `json:"cwd"`
			Turns []json.RawMessage `json:"turns"`
		} `json:"thread"`
	}
	if err := client.Call(ctx, "thread/read", map[string]any{"threadId": threadID, "includeTurns": true}, &read); err != nil {
		return TurnSnapshot{}, err
	}
	if read.Thread.ID != threadID || !sameWorkspace(read.Thread.Cwd, workspace) {
		return TurnSnapshot{}, errors.New("app-server turn snapshot changed thread/workspace")
	}
	for _, raw := range read.Thread.Turns {
		var candidate struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &candidate) != nil || candidate.ID != turnID {
			continue
		}
		var turn TurnSnapshot
		if json.Unmarshal(raw, &turn) != nil {
			return TurnSnapshot{}, errors.New("native turn snapshot is invalid")
		}
		return turn, nil
	}
	return TurnSnapshot{}, errors.New("original native turn is absent from its fixed thread")
}
