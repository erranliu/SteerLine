package app

import (
	"os"
	"strings"
	"testing"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
)

func TestReadinessNativeLaunchKeepsDefaultModelAndPermissions(t *testing.T) {
	t.Setenv("STEERLINE_APP_VERSION_FIXTURE", "1")
	c := Config{Host: "orca", Agent: "codex", Repository: t.TempDir(), CommandTimeout: "2s"}
	setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: os.Args[0]}, &orcaworker.ProfileConfig{Executable: os.Args[0], Shell: "powershell.exe", WaitTimeout: "2m"})
	command, err := managedTerminalCommand(c.executionConfig(), "", nil)
	require(t, err)
	if !strings.Contains(command, "'approval_policy=on-request'") || !strings.Contains(command, "'approvals_reviewer=auto_review'") || !strings.Contains(command, "'windows.sandbox=unelevated'") || strings.Contains(command, "'--model'") {
		t.Fatalf("native readiness launch changed permissions or replaced default model: %s", command)
	}
}
