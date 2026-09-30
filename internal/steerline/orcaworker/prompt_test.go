package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestStagePromptAcceptsMarkerAfterUncertainSendWithoutRetry(t *testing.T) {
	executionID, turnID, prompt := strings.Repeat("a", 32), strings.Repeat("b", 32), "fixed prompt"
	_, marker, err := ChunkCommand(executionID, turnID, 0, []byte(prompt))
	if err != nil {
		t.Fatal(err)
	}
	sends := 0
	call := func(_ context.Context, args ...string) (Result, error) {
		switch args[1] {
		case "show":
			return Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"terminal":{"handle":"terminal","incarnationId":"incarnation","connected":true,"status":"running"}}`)}, nil
		case "send":
			sends++
			return Result{}, errors.New("send receipt lost")
		case "read":
			if len(args) <= 5 {
				return Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"terminal":{"handle":"terminal","nextCursor":"0","source":"stream"}}`)}, nil
			}
			raw, err := json.Marshal(map[string]any{"terminal": map[string]any{"handle": "terminal", "tail": []string{marker}, "oldestCursor": "0", "nextCursor": "1", "source": "stream"}})
			return Result{RuntimeID: "runtime", Payload: raw}, err
		}
		return Result{}, errors.New("unexpected Orca call")
	}
	if err := StagePrompt(context.Background(), call, TerminalIdentity{Kind: "orca", Handle: "terminal", RuntimeID: "runtime", Incarnation: "incarnation"}, time.Second, executionID, turnID, prompt, nil); err != nil || sends != 1 {
		t.Fatalf("confirmed chunk was resent or rejected: %v, sends=%d", err, sends)
	}
}
