package testenv

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"os/exec"
	"path/filepath"
)

// Only the sealed original local workspace is read; this never opens an Orca
// terminal, runs L2 again, or substitutes the caller's repository for the Run.
func localCodexEnvironmentEvidenceCommand(execution runnercontract.ExecutionInput, workspace, script string) (*exec.Cmd, error) {
	if execution.Adapter != "codex-app-server" || execution.Target.Kind != "local" || execution.Target.Environment != "" {
		return nil, errors.New("direct workspace evidence is only allowed for local Codex")
	}
	if !filepath.IsAbs(workspace) {
		return nil, errors.New("original local Run workspace must be absolute")
	}
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference='Stop';[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false);$PSDefaultParameterValues=@{'Get-Content:Encoding'='UTF8'};$value=& {\n"+script+"\n};$value|ConvertTo-Json -Depth 12 -Compress")
	command.Dir = workspace
	return command, nil
}
