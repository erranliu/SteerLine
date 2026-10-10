package native

import (
	"encoding/json"
	"testing"
)

func TestWorkerConfigsPreserveNativeModelAndChainSettings(t *testing.T) {
	local := WorkerImplementationConfigs("isolated-codex", nil)
	codex := local[0]["payload"].(map[string]any)
	if len(local) != 1 || codex["executable"] != "isolated-codex" || codex["model"] != Model || codex["reasoning_effort"] != Effort || codex["windows_sandbox"] != nil || codex["args"] != nil {
		t.Fatal("local model/permission profile changed")
	}
	orca := OrcaWorkerProfile("isolated-orca", "C:/owned", "30s")
	configs := WorkerImplementationConfigs("isolated-codex", orca)
	if len(configs) != 1 || configs[0]["payload"].(map[string]any)["executable"] != "isolated-orca" || orca["model"] != nil || orca["reasoning_effort"] != nil || orca["repository_selector"] != "path:C:/owned" || orca["windows_sandbox"] != nil {
		t.Fatal("Orca native launch inherited the client Codex configuration")
	}
}

func TestOrcaSmokeProfileUsesExplicitEmptyArgs(t *testing.T) {
	profile := OrcaWorkerProfile("isolated-orca", "C:/owned", "30s")
	raw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["args"]) != "[]" {
		t.Fatalf("Orca smoke omitted its regression-triggering args: %s", raw)
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
