//go:build e2e

package remote

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestDeliveryRemoteOrcaCodex(t *testing.T) {
	testenv.RemoteDelivery(testenv.NewScenario(t, "delivery.orca-remote-codex"))
}
