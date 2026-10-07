package native

import (
	"encoding/json"
	"testing"
)

func TestWorkerConfigsPreserveNativeModelAndChainSettings(t *testing.T) {
	for _, orca := range []map[string]any{nil, {"executable": "isolated-orca", "wait_timeout": "4m"}} {
		configs := WorkerImplementationConfigs("isolated-codex", orca)
		codex := configs[0]["payload"].(map[string]any)
		if codex["executable"] != "isolated-codex" || codex["model"] != Model || codex["reasoning_effort"] != Effort || codex["windows_sandbox"] != nil || codex["args"] != nil {
			t.Fatal("native model/permission configuration changed")
		}
		if orca == nil {
			if len(configs) != 1 || codex["args"] != nil {
				t.Fatal("local profile changed")
			}
		} else if len(configs) != 2 || configs[1]["payload"].(map[string]any)["executable"] != "isolated-orca" || configs[1]["payload"].(map[string]any)["wait_timeout"] != "4m" {
			t.Fatal("Orca profile changed")
		}
	}
}

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
