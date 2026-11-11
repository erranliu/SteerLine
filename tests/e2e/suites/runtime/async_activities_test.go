//go:build e2e

package runtime

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestAsyncActivitiesAndReadingPage(t *testing.T) {
	t.Parallel()
	testenv.AsyncActivities(testenv.NewScenario(t, "runtime.async-activities"))
}
