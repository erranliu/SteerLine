package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func provisioningCommand(executable, home string, timeout time.Duration) (*exec.Cmd, error) {
	user := os.Getenv("USERNAME")
	if user == "" {
		return nil, errors.New("USERNAME is required to bind provisioning to the original Windows user")
	}
	// Bind the target user explicitly: UAC may use a different administrator.
	// ComposeCommandLine quotes Windows argv; single quotes protect the result
	// as one literal PowerShell string, including spaces, apostrophes and `$`.
	args := windows.ComposeCommandLine([]string{"sandbox", "setup", "--elevated", "--user", user, "--codex-home", home})
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
try {
    $p = Start-Process -FilePath %s -ArgumentList %s -Verb RunAs -WindowStyle Hidden -PassThru
    if (-not $p.WaitForExit(%d)) { Write-Error 'Native elevated setup timed out; do not start another setup while it is still running.'; exit 124 }
    exit $p.ExitCode
} catch {
    Write-Error ('Native elevation/setup failed or was cancelled: ' + $_.Exception.Message)
    exit 1
}`, powershellLiteral(executable), powershellLiteral(args), timeout.Milliseconds())
	// This host process stays unelevated. Only the pinned native Codex command
	// is elevated through Windows UAC; no model, Go build or test runs as admin.
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	return exec.Command(powershell, "-NoProfile", "-NonInteractive", "-Command", script), nil
}

func powershellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
