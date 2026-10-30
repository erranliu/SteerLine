package testenv

import (
	"reflect"
	"testing"
)

func TestOrcaEnvironmentEvidenceSharesOperationsAndBindsDestination(t *testing.T) {
	operations := [][]string{{"repo", "list"}, {"terminal", "create", "--worktree", "id:repo::workspace", "--command", "fixed evidence"}, {"terminal", "read", "--terminal", "original-handle"}, {"terminal", "close", "--terminal", "original-handle"}}
	for _, operation := range operations {
		local := &orcaTarget{}
		remote := &orcaTarget{Environment: "fixed-target"}
		if got := local.commandArgs(operation...); !reflect.DeepEqual(got, append(append([]string{}, operation...), "--json")) {
			t.Fatal("local Orca changed evidence operation", got)
		}
		want := append(append([]string{}, operation...), "--environment", "fixed-target", "--json")
		if got := remote.commandArgs(operation...); !reflect.DeepEqual(got, want) {
			t.Fatal("remote Orca changed evidence operation or target", got)
		}
	}
}
