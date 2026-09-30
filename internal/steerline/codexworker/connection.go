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
	Answers   map[string][]string
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
