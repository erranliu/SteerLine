package steerline

import (
	"context"
	"testing"
)

func TestOrcaRestartPreflightRetainsOriginalPolicyRoots(t *testing.T) {
	for _, fullAccess := range []bool{false, true} {
		name := "workspace"
		if fullAccess {
			name = "full_access"
		}
		t.Run(name, func(t *testing.T) {
			f := newEnvironmentFixtureWithPolicy(t, fakeNativeOrcaState{ReadinessMode: "hold"}, "gpt-6-luna", fullAccess)
			f.prepare(t)
			e := f.service.executionRecord(f.run)
			if e.Process == nil || e.PolicyResources == nil || e.Environment == nil {
				t.Fatalf("original managed resource receipt missing: %s", deref(e.Error))
			}
			if fullAccess != (len(e.PolicyResources.WriteRoots) == 0) {
				t.Fatal("fixture did not bind the selected policy")
			}
			require(t, preflightWorkerFlowState(context.Background(), f.config))
			original := append([]string(nil), e.Environment.WriteRoots...)
			for _, roots := range [][]string{{"C:/unexpected"}, nil} {
				if fullAccess && len(roots) == 0 {
					continue
				}
				e.Environment.WriteRoots = roots
				require(t, f.service.commitWorker(f.run))
				if err := preflightWorkerFlowState(context.Background(), f.config); err == nil {
					t.Fatal("changed original root evidence accepted on restart")
				}
			}
			e.Environment.WriteRoots = original
			require(t, f.service.commitWorker(f.run))
			require(t, preflightWorkerFlowState(context.Background(), f.config))
		})
	}
}
