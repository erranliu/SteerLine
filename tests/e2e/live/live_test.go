//go:build e2e

package live

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestSmokeLocalCodex(t *testing.T) {
	testenv.LiveDelivery(testenv.NewScenario(t, "smoke.local-codex"), "local-codex")
}
