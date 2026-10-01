package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestBindAgentPendingDetectionDoesNotAcceptAnotherAgentOrWorkspace(t *testing.T) {
	worktree := Worktree{ID: "worktree", Path: t.TempDir(), Branch: "branch"}
	term := Terminal{Handle: "original", WorktreeID: worktree.ID, WorktreePath: worktree.Path, Branch: "refs/heads/branch", IncarnationID: "incarnation", Connected: true}
	call := func(_ context.Context, args ...string) (Result, error) {
		if len(args) != 5 || args[0] != "terminal" || args[1] != "show" || args[3] != term.Handle {
			t.Fatalf("binding attempted another operation: %v", args)
		}
		raw, err := json.Marshal(map[string]any{"terminal": term})
		return Result{RuntimeID: "runtime", Payload: raw}, err
	}
	if _, err := BindAgent(context.Background(), call, worktree, term.Handle, "codex", "orca"); !errors.Is(err, ErrAgentStarting) {
		t.Fatalf("unidentified original agent was not pending: %v", err)
	}
	term.AgentIdentity = "claude"
	if _, err := BindAgent(context.Background(), call, worktree, term.Handle, "codex", "orca"); err == nil || errors.Is(err, ErrAgentStarting) {
		t.Fatal("wrong agent accepted or treated as startup")
	}
	term.AgentIdentity = ""
	term.WorktreePath = filepath.Join(worktree.Path, "other")
	if _, err := BindAgent(context.Background(), call, worktree, term.Handle, "codex", "orca"); err == nil || errors.Is(err, ErrAgentStarting) {
		t.Fatal("wrong workspace treated as startup")
	}
	term.WorktreePath, term.AgentIdentity = worktree.Path, "codex"
	identity, err := BindAgent(context.Background(), call, worktree, term.Handle, "codex", "orca")
	if err != nil || identity.Terminal != term.Handle || identity.Incarnation != term.IncarnationID {
		t.Fatalf("identified original terminal could not bind: %+v / %v", identity, err)
	}
}
