//go:build e2e

package runtime

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestContinuationSuggestionAdvisory(t *testing.T) {
	testenv.ContinuationSuggestionAdvisory(testenv.NewScenario(t, "runtime.main-agent-advisory"))
}
