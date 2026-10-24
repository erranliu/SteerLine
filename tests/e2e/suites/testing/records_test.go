//go:build e2e

package testing

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestRecords(t *testing.T) { testenv.TestRecords(testenv.NewScenario(t, "testing.records")) }

func TestRuntimeProgress(t *testing.T) {
	t.Parallel()
	testenv.RuntimeProgress(testenv.NewScenario(t, "testing.runtime-progress"))
}
