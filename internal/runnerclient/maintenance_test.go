package runnerclient

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPairedMaintenanceRequiresPublishedGitEvidence(t *testing.T) {
	published := `{"version":1,"git":{"command":"workspace-git --run-id R","read_only":true,"agent_calls":0,"resume_calls":0,"targets":[{"adapter":"orca-terminal","target_kind":"orca_environment","workflow":"execute/1","source":"original_authenticated_orca_git_status","workspace":"latest_managed_worktree_incarnation"}]}}`
	for _, tc := range []struct {
		name       string
		capability string
		want       bool
	}{
		{"no capability", "", false},
		{"old capability", `{"version":1,"git":{"command":"workspace-git --run-id R","read_only":true,"agent_calls":0,"resume_calls":0}}`, false},
		{"different workflow", strings.Replace(published, "execute/1", "development/2", 1), false},
		{"not read only", strings.Replace(published, "true", "false", 1), false},
		{"paired execute", published, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := Catalog{Capabilities: Capabilities{RunEvidence: json.RawMessage(tc.capability)}}
			if got := catalog.hasPairedMaintenanceGit(); got != tc.want {
				t.Fatalf("paired maintenance Git capability = %v, want %v", got, tc.want)
			}
		})
	}
}
