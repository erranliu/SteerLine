package native

import (
	"encoding/json"
	"testing"
)

func TestOrcaInventoryPreservesOwnershipAndRejectsIncompleteEvidence(t *testing.T) {
	raw, err := OrcaPayload([]byte(`{"ok":true,"result":{"worktrees":[{"id":"worktree","repoId":"repo","path":"C:/owned"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	worktrees, err := OrcaWorktrees(raw)
	if err != nil || len(worktrees) != 1 || worktrees[0] != (OrcaWorktree{ID: "worktree", RepoID: "repo", Path: "C:/owned"}) {
		t.Fatalf("ownership changed: %+v / %v", worktrees, err)
	}
	if _, err := OrcaWorktrees(json.RawMessage(`{"worktrees":[],"truncated":true}`)); err == nil {
		t.Fatal("truncated cleanup inventory accepted")
	}
	if _, err := OrcaPayload([]byte(`{"ok":false,"error":{"message":"rejected"}}`)); err == nil {
		t.Fatal("native rejection accepted")
	}
}
