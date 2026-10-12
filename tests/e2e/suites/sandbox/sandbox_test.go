//go:build e2e

package sandbox

import (
	"testing"

	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
)

func TestCodexSandboxGit(t *testing.T) {
	testenv.DebugSandbox(testenv.NewScenario(t, "runtime.sandbox-git.local-codex"))
}
