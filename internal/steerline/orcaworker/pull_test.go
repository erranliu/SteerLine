package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPullTurnUsesSavedExitWithoutReadingTerminal(t *testing.T) {
	dir := t.TempDir()
	events := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(events, nil, 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(TurnExit{Version: 1, TurnID: "turn", Cursor: 7, ExitCode: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "orca-exit.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	done := make(chan error, 1)
	PullTurn(context.Background(), func(context.Context, ...string) (Result, error) {
		called = true
		return Result{}, errors.New("unexpected terminal read")
	}, TerminalIdentity{}, "turn", events, done, func(string, []byte) error { return nil }, nil)
	if err := <-done; err != nil || called {
		t.Fatalf("saved exit was not authoritative: %v, terminal read=%v", err, called)
	}
}

func TestPullTurnSavesObservedExitBeforeReportingCompletion(t *testing.T) {
	dir := t.TempDir()
	events := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(events, nil, 0600); err != nil {
		t.Fatal(err)
	}
	cursor, err := json.Marshal(TurnCursor{Version: 1, Cursor: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "orca-cursor.json"), cursor, 0600); err != nil {
		t.Fatal(err)
	}
	call := func(_ context.Context, args ...string) (Result, error) {
		if args[1] == "show" {
			return Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"terminal":{"handle":"terminal","incarnationId":"incarnation","connected":true,"status":"running"}}`)}, nil
		}
		return Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"terminal":{"handle":"terminal","tail":["SLEND:turn:0"],"oldestCursor":"0","nextCursor":"1","source":"stream"}}`)}, nil
	}
	done := make(chan error, 1)
	PullTurn(context.Background(), call, TerminalIdentity{Kind: "orca", Handle: "terminal", RuntimeID: "runtime", Incarnation: "incarnation"}, "turn", events, done, func(path string, data []byte) error {
		return os.WriteFile(path, data, 0600)
	}, nil)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	marker, err := ReadTurnExit(events, "turn")
	if err != nil || marker == nil || marker.ExitCode != 0 || marker.Cursor != 0 {
		t.Fatalf("completed turn has no durable exit marker: %+v, %v", marker, err)
	}
}
