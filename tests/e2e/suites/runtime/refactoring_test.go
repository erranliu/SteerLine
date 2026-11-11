//go:build e2e

package runtime

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestRefactoringMaintenance(t *testing.T) {
	t.Parallel()
	testenv.RefactoringMaintenance(testenv.NewScenario(t, "runtime.refactoring-maintenance"))
}
