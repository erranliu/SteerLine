package orcaworker

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestNativeWorktreeReceiptRequiresAgentHandle(t *testing.T) {
	raw, err := json.Marshal(map[string]any{"worktree": Worktree{ID: "worktree", RepoID: "repo", Path: filepath.Join(t.TempDir(), "worktree"), Branch: "branch", Head: "base", DisplayName: "name", BaseRef: "base"}})
	if err != nil {
		t.Fatal(err)
	}
	result := Result{Payload: raw}
	_, err = ParseNativeWorktreeReceipt(result, "name", "repo", "base")
	if !errors.Is(err, ErrAgentHandleOmitted) {
		t.Fatalf("native worktree without agent receipt was accepted: %v", err)
	}
}

func TestFixedAgentTerminalReceiptRequiresOriginalWorktree(t *testing.T) {
	result := Result{Payload: json.RawMessage(`{"terminal":{"handle":"terminal","incarnationId":"incarnation","worktreeId":"other","title":"fixed"}}`)}
	if _, err := ParseFixedAgentTerminalReceipt(result, "worktree", "fixed"); err == nil {
		t.Fatal("agent terminal on another worktree was accepted")
	}
}
