//go:build e2e

package runtime

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestProjectMaintenance(t *testing.T) {
	testenv.ProjectMaintenance(testenv.NewScenario(t, "runtime.project-maintenance"))
}
