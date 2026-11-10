//go:build e2e

package runtime

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestMainAgentAdvisory(t *testing.T) {
	testenv.MainAgentAdvisory(testenv.NewScenario(t, "runtime.main-agent-advisory"))
}
