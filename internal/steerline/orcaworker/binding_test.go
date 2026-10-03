package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
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

func TestVerifyAgentTerminalWaitsOnlyForOriginalAgentDetection(t *testing.T) {
	path := t.TempDir()
	term := Terminal{Handle: "agent", WorktreeID: "worktree", WorktreePath: path, Branch: "branch", IncarnationID: "inc", Connected: true}
	process := executionapi.ProcessIdentity{Terminal: term.Handle, RuntimeID: "runtime", Incarnation: term.IncarnationID, WorktreeID: term.WorktreeID}
	calls := 0
	call := func(context.Context, ...string) (Result, error) {
		calls++
		if calls == 2 {
			term.AgentIdentity = "codex"
		}
		raw, err := json.Marshal(map[string]any{"terminal": term})
		return Result{RuntimeID: "runtime", Payload: raw}, err
	}
	if err := VerifyAgentTerminal(context.Background(), call, process, path, "branch", "codex"); !errors.Is(err, ErrAgentStarting) || calls != 1 {
		t.Fatalf("original agent was not pending: calls=%d err=%v", calls, err)
	}
	if err := VerifyAgentTerminal(context.Background(), call, process, path, "branch", "codex"); err != nil || calls != 2 {
		t.Fatalf("original agent detection: calls=%d err=%v", calls, err)
	}
	for _, change := range []string{"agent", "incarnation", "worktree", "exited"} {
		t.Run(change, func(t *testing.T) {
			term.AgentIdentity, term.IncarnationID, term.WorktreeID, term.Status = "", "inc", "worktree", "running"
			switch change {
			case "agent":
				term.AgentIdentity = "claude"
			case "incarnation":
				term.IncarnationID = "replacement"
			case "worktree":
				term.WorktreeID = "other"
			case "exited":
				term.Status = "exited"
			}
			calls = 0
			if err := VerifyAgentTerminal(context.Background(), call, process, path, "branch", "codex"); err == nil || errors.Is(err, ErrAgentStarting) || calls != 1 {
				t.Fatalf("changed original terminal was retried: calls=%d err=%v", calls, err)
			}
		})
	}
}
