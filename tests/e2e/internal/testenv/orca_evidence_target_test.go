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

func TestOrcaEnvironmentLocalRepositoryRequiresExactSource(t *testing.T) {
	local := &orcaTarget{}
	remote := &orcaTarget{Environment: "paired"}
	const origin = "https://github.com/owner/project.git"
	for _, tc := range []struct {
		path, url string
		want      bool
	}{
		{"C:/isolated/source", origin, true},
		{"c:\\ISOLATED\\source\\", "https://github.com/owner/project", true},
		{"C:/user/project", origin, false},
		{"C:/isolated/source-other", origin, false},
		{"C:/isolated/source", "https://github.com/other/project.git", false},
	} {
		if got := local.matchesRepository(tc.path, tc.url, "C:/isolated/source", origin); got != tc.want {
			t.Fatalf("local source selection %q/%q = %v", tc.path, tc.url, got)
		}
	}
	if !remote.matchesRepository("D:/remote/project", origin, "C:/isolated/source", origin) {
		t.Fatal("remote source routing changed")
	}
}

func TestOrcaEnvironmentCleanupRequiresOriginalWorkspaceOwnership(t *testing.T) {
	source := orcaCleanupWorkspace{ID: "repo::C:/source", RepoID: "repo", Path: "C:/source", HostID: "local", IsMainWorktree: true}
	worker := orcaCleanupWorkspace{ID: "repo::C:/worker", RepoID: "repo", Path: "C:/worker", HostID: "local", Branch: "refs/heads/original"}
	owned := map[string]string{"C:/worker": "original"}
	got, err := ownedOrcaWorktrees("repo", "C:/source", []orcaCleanupWorkspace{source, worker}, owned)
	if err != nil || !reflect.DeepEqual(got, []string{worker.ID}) {
		t.Fatal("original workspace not selected", got, err)
	}
	for _, fault := range []string{"foreign-path", "changed-branch", "foreign-repo", "remote-host", "missing-source", "duplicate", "replaced-source"} {
		t.Run(fault, func(t *testing.T) {
			s, w := source, worker
			switch fault {
			case "foreign-path":
				w.Path = "C:/foreign"
			case "changed-branch":
				w.Branch = "refs/heads/foreign"
			case "foreign-repo":
				w.RepoID = "foreign"
			case "remote-host":
				w.HostID = "remote"
			case "replaced-source":
				s.Path = "C:/foreign"
			}
			trees := []orcaCleanupWorkspace{s, w}
			if fault == "missing-source" {
				trees = trees[1:]
			}
			if fault == "duplicate" {
				trees = append(trees, w)
			}
			if _, err := ownedOrcaWorktrees("repo", "C:/source", trees, owned); err == nil {
				t.Fatal("unsafe cleanup accepted")
			}
		})
	}
}
