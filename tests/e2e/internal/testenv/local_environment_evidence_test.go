package testenv

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalEnvironmentEvidenceReadsOnlyOriginalWorkspace(t *testing.T) {
	root := t.TempDir()
	execution := runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}}
	if err := os.WriteFile(filepath.Join(root, "summary.json"), []byte(`{"passed":true,"source":"original"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cmd, err := localCodexEnvironmentEvidenceCommand(execution, root, `Get-Content -Raw -LiteralPath ./summary.json|ConvertFrom-Json`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Passed bool
		Source string
	}
	if json.Unmarshal(raw, &result) != nil || !result.Passed || result.Source != "original" {
		t.Fatal(string(raw))
	}
	cmd, err = localCodexEnvironmentEvidenceCommand(execution, root, `throw 'missing original evidence'`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cmd.Output(); err == nil {
		t.Fatal("failed original evidence passed")
	}
	if _, err = localCodexEnvironmentEvidenceCommand(execution, "relative", `@{passed=$true}`); err == nil {
		t.Fatal("caller relative workspace accepted")
	}
}

func TestLocalOrcaEnvironmentEvidenceRejectsDirectWorkspaceReads(t *testing.T) {
	for _, target := range []runnercontract.ExecutionTarget{{Kind: "local"}, {Kind: "orca_environment", Environment: "remote"}} {
		execution := runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: target}
		if _, err := localCodexEnvironmentEvidenceCommand(execution, t.TempDir(), `@{passed=$true}`); err == nil {
			t.Fatal("Orca evidence used the local Codex shortcut")
		}
	}
}
