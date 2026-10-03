package steerlineapp

import (
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"os"
	"strings"
	"testing"
)

func TestReadinessNativeLaunchKeepsDefaultModelAndPermissions(t *testing.T) {
	t.Setenv("STEERLINE_APP_VERSION_FIXTURE", "1")
	c := Config{Host: "orca", Agent: "codex", Repository: t.TempDir(), CommandTimeout: "2s", ExecutionPolicy: workspacePolicyFixture()}
	setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: os.Args[0], WindowsSandbox: "unelevated"}, &orcaworker.ProfileConfig{Executable: os.Args[0], Shell: "powershell.exe", WaitTimeout: "2m"})
	command, err := managedTerminalCommand(c, "", nil)
	require(t, err)
	if !strings.Contains(command, "'approval_policy=never'") || !strings.Contains(command, "'windows.sandbox=unelevated'") || strings.Contains(command, "'--model'") {
		t.Fatalf("native readiness launch changed permissions or replaced default model: %s", command)
	}
}
