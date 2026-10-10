//go:build e2e

package live

import (
	"testing"

	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
)

func TestSmokeLocalCodex(t *testing.T) {
	testenv.LiveDelivery(testenv.NewScenario(t, "smoke.local-codex"), "local-codex", false)
}
func TestDeliveryLocalCodex(t *testing.T) {
	testenv.LiveDelivery(testenv.NewScenario(t, "delivery.local-codex"), "local-codex", false)
}
func TestRecoveryLocalCodex(t *testing.T) {
	testenv.LiveDelivery(testenv.NewScenario(t, "recovery.local-codex"), "local-codex", true)
}
