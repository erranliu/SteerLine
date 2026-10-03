package driver

import (
	"strings"
	"testing"
)

func TestProjectEnvironmentRejectsInheritedRuntimeAndTestSwitches(t *testing.T) {
	got := strings.Join(Environment([]string{"Path=tools", "STEERLINE_HOME=production", "STEERLINE_REAL_CODEX=1", "STEERLINE_WORKER_FLOW_MOCK=1", "CODEX_HOME=real", "TEMP=old", "GOFLAGS=-tags=e2e"}, "isolated", "state"), "\n")
	for _, bad := range []string{"production", "REAL_CODEX", "WORKER_FLOW_MOCK", "CODEX_HOME=real", "TEMP=old", "-tags=e2e"} {
		if strings.Contains(got, bad) {
			t.Fatalf("inherited %s: %s", bad, got)
		}
	}
	for _, want := range []string{"Path=tools", "STEERLINE_HOME=isolated", "STEERLINE_E2E_CONTROLLED_STATE=state"} {
		if !strings.Contains(got, want) {
			t.Fatal(got)
		}
	}
}
