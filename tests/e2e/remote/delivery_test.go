//go:build e2e

package remote

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestDeliveryRemoteOrcaCodex(t *testing.T) {
	t.Skip("retired single-turn remote delivery fixture: current development flow requires staged original final provenance unavailable from supervised adapter")
	testenv.RemoteDelivery(testenv.NewScenario(t, "delivery.orca-remote-codex"))
}
