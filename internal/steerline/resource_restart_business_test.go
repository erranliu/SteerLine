package steerline

import (
	"context"
	"testing"
)

func TestRestartPreflightRetainsOriginalPolicyRoots(t *testing.T) {
	for _, fullAccess := range []bool{false, true} {
		name := "workspace"
		if fullAccess {
			name = "full_access"
		}
		t.Run(name, func(t *testing.T) {
			policy := workspacePolicyFixture()
			if fullAccess {
				policy.Filesystem.WriteScope, policy.Filesystem.GitMetadataWrite = "host", false
			}
			s, run, _ := neutralPreparationServiceWithPolicy(t, "success", policy)
			e := s.executionRecord(run)
			if e.Process == nil || e.PolicyResources == nil || e.Environment == nil {
				t.Fatalf("original managed resource receipt missing: %s", deref(e.Error))
			}
			if fullAccess != (len(e.PolicyResources.WriteRoots) == 0) {
				t.Fatal("fixture did not bind the selected policy")
			}
			require(t, preflightWorkerFlowState(context.Background(), s.Config))
			original := append([]string(nil), e.Environment.WriteRoots...)
			for _, roots := range [][]string{{"C:/unexpected"}, nil} {
				if fullAccess && len(roots) == 0 {
					continue
				}
				e.Environment.WriteRoots = roots
				require(t, s.commitWorker(run))
				if err := preflightWorkerFlowState(context.Background(), s.Config); err == nil {
					t.Fatal("changed original root evidence accepted on restart")
				}
			}
			e.Environment.WriteRoots = original
			require(t, s.commitWorker(run))
			require(t, preflightWorkerFlowState(context.Background(), s.Config))
		})
	}
}
