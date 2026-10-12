package driver

import (
	"context"
	"encoding/json"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDebugReleaseCodexSandbox(t *testing.T) {
	root := os.Getenv("STEERLINE_DEBUG_SANDBOX_ROOT")
	if root == "" {
		t.Skip("run scripts/test-codex-sandbox.ps1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	if _, err := RunDebugSandbox(ctx, root, ""); err != nil {
		t.Fatal(err)
	}
}

func TestDebugSandboxStageCommands(t *testing.T) {
	for _, tc := range []struct {
		name, step, command string
		wantError           bool
	}{
		{"ordinary_probe", executionmodel.StepImplement, "powershell.exe -NoProfile -File ./sandbox-check.ps1", false},
		{"read_publication_script", executionmodel.StepImplement, "Get-Content ./publish.ps1", false},
		{"premature_script", executionmodel.StepImplement, "pwsh -Command 'powershell.exe -NoProfile -File ./publish.ps1'", true},
		{"premature_windows_script", executionmodel.StepImplement, `powershell.exe -File .\publish.ps1`, true},
		{"premature_git_push", executionmodel.StepImplement, "git -c credential.helper= push origin HEAD:refs/heads/codex/task", true},
		{"premature_pr", executionmodel.StepImplement, "gh pr create --repo erranliu/SteerlineTest", true},
		{"report_without_tools", executionmodel.StepReport, "", false},
		{"report_with_tools", executionmodel.StepReport, "git status --short", true},
		{"publication_without_tools", executionmodel.StepPublish, "", true},
		{"publication", executionmodel.StepPublish, "powershell.exe -NoProfile -File ./publish.ps1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			journal := filepath.Join(t.TempDir(), "appserver.jsonl")
			raw, err := json.Marshal(map[string]any{"method": "item/completed", "params": map[string]any{"item": map[string]any{"type": "agentMessage", "text": "I am moving to publication now."}}})
			if err != nil {
				t.Fatal(err)
			}
			if tc.command != "" {
				command, err := json.Marshal(map[string]any{"method": "item/started", "params": map[string]any{"item": map[string]any{"type": "commandExecution", "command": tc.command}}})
				if err != nil {
					t.Fatal(err)
				}
				raw = append(append(raw, '\n'), command...)
			}
			if err := os.WriteFile(journal, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if err := debugSandboxStageCommands(journal, tc.step); (err != nil) != tc.wantError {
				t.Fatalf("stage evidence error=%v, wantError=%v", err, tc.wantError)
			}
		})
	}
}
