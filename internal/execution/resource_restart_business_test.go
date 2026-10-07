package execution

import (
	"context"
	"testing"
)

func TestRestartPreflightRetainsOriginalPolicyRoots(t *testing.T) {
	t.Run("workspace", func(t *testing.T) {
		s, run, _ := neutralPreparationService(t, "success")
		e := s.executionRecord(run)
		if e.Process == nil || e.PolicyResources == nil || e.Environment == nil {
			t.Fatalf("original managed resource receipt missing: %s", deref(e.Error))
		}
		if len(e.PolicyResources.WriteRoots) == 0 {
			t.Fatal("fixture did not bind the selected policy")
		}
		require(t, checkFixtureStoredExecutions(context.Background(), s.Config))
		original := append([]string(nil), e.Environment.WriteRoots...)
		for _, roots := range [][]string{{"C:/unexpected"}, nil} {
			e.Environment.WriteRoots = roots
			require(t, s.commitWorker(run))
			if err := checkFixtureStoredExecutions(context.Background(), s.Config); err == nil {
				t.Fatal("changed original root evidence accepted on restart")
			}
		}
		e.Environment.WriteRoots = original
		require(t, s.commitWorker(run))
		require(t, checkFixtureStoredExecutions(context.Background(), s.Config))
	})
}
