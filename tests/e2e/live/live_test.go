//go:build e2e

package live

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestSmokeLocalCodex(t *testing.T) {
	testenv.LiveDelivery(testenv.NewScenario(t, "smoke.local-codex"), "local-codex")
}

func TestWorkflowLocalCodexAudited(t *testing.T) {
	testenv.LiveDelivery(testenv.NewScenario(t, "workflow.local-codex-audited"), "local-codex")
}

func TestManagementLocalCodex(t *testing.T) {
	testenv.TestManagement(testenv.NewScenario(t, "testing.management-local-codex"))
}
func TestManualVerificationLocalCodex(t *testing.T) {
	testenv.ManualVerificationLive(testenv.NewScenario(t, testenv.ManualVerificationLiveID))
}

func TestRefactoringLocalCodex(t *testing.T) {
	testenv.RefactoringLive(testenv.NewScenario(t, "refactoring.local-codex"))
}
