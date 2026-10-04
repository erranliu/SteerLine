//go:build e2e

package candidates

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestCandidateLocal(t *testing.T) {
	testenv.CandidateAcceptance(testenv.NewScenario(t, "candidates.local"))
}
func TestCandidateRemote(t *testing.T) {
	testenv.CandidateAcceptance(testenv.NewScenario(t, "candidates.remote"))
}
func TestCandidateMixed(t *testing.T) {
	testenv.CandidateAcceptance(testenv.NewScenario(t, "candidates.mixed"))
}
