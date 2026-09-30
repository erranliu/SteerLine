package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestReadWorkerOutputResetsChangedSourceCursorOnce(t *testing.T) {
	calls := 0
	call := func(_ context.Context, args ...string) (Result, error) {
		calls++
		if calls == 1 {
			if args[len(args)-2] != "--cursor" || args[len(args)-1] != "old" {
				t.Fatal("first read lost the fixed cursor")
			}
			return Result{}, errors.New("source_changed")
		}
		if args[len(args)-2] == "--cursor" {
			t.Fatal("changed source retry reused the old cursor")
		}
		return Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"dispatchId":"dispatch","source":"terminal","cursor":"new","nextCursor":"next","contentComplete":true,"terminal":{"tail":["line"]}}`)}, nil
	}
	output, reset, err := ReadWorkerOutput(context.Background(), call, "dispatch", "old", "runtime")
	if err != nil || !reset || calls != 2 || output.NextCursor != "next" {
		t.Fatalf("changed output source was not reconciled: %+v, %v, %v, %d", output, reset, err, calls)
	}
}

func TestVerifySettledDispatchRejectsReplacedTerminal(t *testing.T) {
	call := func(context.Context, ...string) (Result, error) {
		return Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"dispatch":{"id":"dispatch","runId":"run","taskId":"task","status":"completed"},"worker":{"dispatchId":"dispatch","agentTerminalHandle":"terminal"},"terminal":{"handle":"terminal","incarnationId":"replacement","worktreeId":"worktree"}}`)}, nil
	}
	_, err := VerifySettledDispatch(context.Background(), call, DispatchIdentity{DispatchID: "dispatch", RunID: "run", TaskID: "task", RuntimeID: "runtime", Terminal: "terminal", Incarnation: "original", WorktreeID: "worktree"})
	if err == nil {
		t.Fatal("settled Dispatch on a replaced terminal was accepted")
	}
}
