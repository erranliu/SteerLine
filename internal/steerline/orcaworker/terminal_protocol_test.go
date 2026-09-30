package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestReadOutputRejectsGapFromOriginalTerminal(t *testing.T) {
	call := func(_ context.Context, args ...string) (Result, error) {
		if len(args) > 1 && args[1] == "show" {
			return Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"terminal":{"handle":"terminal","incarnationId":"incarnation","connected":true,"status":"running"}}`)}, nil
		}
		return Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"terminal":{"handle":"terminal","tail":["line"],"oldestCursor":"2","nextCursor":"3","source":"stream"}}`)}, nil
	}
	_, err := ReadOutput(context.Background(), call, TerminalIdentity{Kind: "orca", Handle: "terminal", RuntimeID: "runtime", Incarnation: "incarnation"}, 0)
	var platform *PlatformError
	if !errors.As(err, &platform) || platform.Code != "protocol_invalid" {
		t.Fatalf("retention gap was accepted: %v", err)
	}
}

func TestSendCommandKeepsUncertainReceiptUnknown(t *testing.T) {
	sends := 0
	call := func(_ context.Context, args ...string) (Result, error) {
		if len(args) > 1 && args[1] == "show" {
			return Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"terminal":{"handle":"terminal","incarnationId":"incarnation","connected":true,"status":"running"}}`)}, nil
		}
		sends++
		return Result{}, errors.New("transport lost its receipt")
	}
	_, err := SendCommand(context.Background(), call, TerminalIdentity{Kind: "orca", Handle: "terminal", RuntimeID: "runtime", Incarnation: "incarnation"}, "fixed command")
	var platform *PlatformError
	if !errors.As(err, &platform) || platform.Code != "outcome_unknown" || sends != 1 {
		t.Fatalf("uncertain send was retried or treated as confirmed: %v (%d sends)", err, sends)
	}
}
