package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestFindRunRejectsDuplicateRecoveryIdentityAcrossPages(t *testing.T) {
	calls := 0
	call := func(_ context.Context, args ...string) (json.RawMessage, error) {
		calls++
		if calls == 1 {
			return json.RawMessage(`{"runs":[{"id":"first","objective":"fixed","coordinator_handle":"terminal"}],"nextCursor":"page-two"}`), nil
		}
		if len(args) < 2 || args[len(args)-2] != "--cursor" || args[len(args)-1] != "page-two" {
			return nil, errors.New("recovery did not use the saved page cursor")
		}
		return json.RawMessage(`{"runs":[{"id":"second","objective":"fixed","coordinator_handle":"terminal"}]}`), nil
	}
	if _, err := FindRun(context.Background(), call, "fixed", "terminal"); err == nil {
		t.Fatal("ambiguous Orca Run identity was accepted")
	}
}

func TestFindTaskRejectsIncompleteInventory(t *testing.T) {
	call := func(context.Context, ...string) (json.RawMessage, error) {
		return json.RawMessage(`{"runId":"run","tasks":[{"id":"task","run_id":"run","spec":"fixed","created_by_terminal_handle":"terminal"}],"count":2}`), nil
	}
	if _, err := FindTask(context.Background(), call, "run", "terminal", "fixed"); err == nil {
		t.Fatal("truncated Orca Task inventory was accepted")
	}
}

func TestCreateWorktreePreservesUnknownReceipt(t *testing.T) {
	callCount := 0
	call := func(context.Context, ...string) (json.RawMessage, error) {
		callCount++
		if callCount == 2 {
			return nil, errors.New("transport lost its receipt")
		}
		return json.RawMessage(`{"worktrees":[]}`), nil
	}
	if _, err := CreateWorktree(context.Background(), call, "repo", "fixed", "base"); !errors.Is(err, ErrWorktreeOutcomeUnknown) {
		t.Fatalf("unverified worktree creation returned %v", err)
	}
	if callCount != 3 {
		t.Fatalf("expected one create and one read only recovery probe, got %d calls", callCount)
	}
}

func TestRecoveredDispatchRequiresExactInjectedIdentity(t *testing.T) {
	expected := DispatchIdentity{DispatchID: "dispatch", TaskID: "task", RunID: "run", RuntimeID: "runtime", Terminal: "terminal", Incarnation: "incarnation", WorktreeID: "worktree"}
	call := func(context.Context, ...string) (Result, error) {
		return Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"dispatch":{"id":"dispatch","taskId":"task","runId":"run"},"worker":{"dispatchId":"dispatch","stage":"injected","agentTerminalHandle":"terminal"},"terminal":{"handle":"terminal","incarnationId":"incarnation","worktreeId":"worktree"}}`)}, nil
	}
	if err := VerifyRecoveredDispatch(context.Background(), call, expected); err != nil {
		t.Fatal(err)
	}
	expected.Incarnation = "replacement"
	if err := VerifyRecoveredDispatch(context.Background(), call, expected); !errors.Is(err, ErrDispatchIdentity) {
		t.Fatalf("replaced worker terminal was accepted: %v", err)
	}
}

func TestListTerminalsRejectsMissingRuntime(t *testing.T) {
	call := func(context.Context, ...string) (Result, error) {
		return Result{Payload: json.RawMessage(`{"terminals":[{"handle":"terminal"}]}`)}, nil
	}
	if _, _, err := ListTerminals(context.Background(), call, "path:C:/repo"); err == nil {
		t.Fatal("terminal inventory without runtime identity was accepted")
	}
}
