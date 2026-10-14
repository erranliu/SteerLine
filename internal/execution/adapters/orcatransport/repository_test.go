package orcatransport

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestWorkspaceRepositoryFollowsProject(t *testing.T) {
	for _, tc := range []struct {
		name, environment, project, remote, selector string
	}{
		{"local", "", `C:\Projects\merge-h-308-code`, "", `path:C:\Projects\merge-h-308-code`},
		{"remote-windows", "desktop", `C:\Projects\merge-h-308-code\`, `D:\Projects\merge-h-308-code\`, "id:repo"},
		{"remote-unix", "desktop", "C:/Projects/merge-h-308-code", "/srv/merge-h-308-code/", "id:repo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, f, s := fixtureOn(t, tc.environment)
			l.setup.RepositoryPath = tc.project
			f.repoInventory = map[string]any{"repos": []any{
				map[string]any{"id": "wrong-project", "kind": "git", "path": "/srv/SteerlineTest", "displayName": "merge-h-308-code"},
				map[string]any{"id": "folder", "kind": "folder", "path": tc.remote},
				map[string]any{"id": "repo", "kind": "git", "path": tc.remote, "displayName": "renamed in Orca"},
			}}
			receipt, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err != nil || receipt.Workspace == nil {
				t.Fatalf("project workspace not prepared: %+v %v", receipt, err)
			}
			creates, lists := 0, 0
			for _, call := range f.calls {
				if call[0] == "repo" && call[1] == "list" {
					lists++
				}
				if call[0] == "worktree" && call[1] == "create" {
					creates++
					if call[3] != tc.selector {
						t.Fatalf("wrong project selector: %v", call)
					}
				}
			}
			if creates != 1 || tc.environment == "" && lists != 0 || tc.environment != "" && lists != 1 {
				t.Fatalf("unexpected discovery/create count: %d/%d", lists, creates)
			}
			// Recovery uses the exact original receipt even if the inventory changes.
			before := len(f.calls)
			f.repoInventory = map[string]any{"repos": []any{}}
			q := request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan})
			q.Operation.Receipt, _ = json.Marshal(receipt)
			if _, _, err = l.Reconcile(context.Background(), q); err != nil {
				t.Fatal("original creation could not be reconciled", err)
			}
			for _, call := range f.calls[before:] {
				if call[0] == "repo" || call[1] == "create" {
					t.Fatal("recovery rediscovered or recreated workspace", call)
				}
			}
		})
	}
}

func TestRemoteRepositoryDiscoveryRejectsBeforeCreation(t *testing.T) {
	matching := map[string]any{"id": "repo", "kind": "git", "path": "/srv/merge-h-308-code"}
	for _, tc := range []struct {
		name, message string
		inventory     any
	}{
		{"missing", "no Orca Git repository", map[string]any{"repos": []any{}}},
		{"ambiguous", "multiple Orca Git repositories", map[string]any{"repos": []any{matching, map[string]any{"id": "other", "kind": "git", "path": `D:\Projects\merge-h-308-code`}}}},
		{"folder-only", "no Orca Git repository", map[string]any{"repos": []any{map[string]any{"id": "folder", "kind": "folder", "path": "/srv/merge-h-308-code"}}}},
		{"truncated", "inventory is incomplete", map[string]any{"repos": []any{matching}, "truncated": true}},
		{"malformed", "inventory is incomplete", "invalid inventory"},
		{"missing-id", "no repository identity", map[string]any{"repos": []any{map[string]any{"kind": "git", "path": "/srv/merge-h-308-code"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, f, s := fixtureOn(t, "desktop")
			l.setup.RepositoryPath = "C:/Projects/merge-h-308-code"
			f.repoInventory = tc.inventory
			receipt, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err == nil || !strings.Contains(err.Error(), tc.message) || receipt.Disposition != b.OperationRejected {
				t.Fatalf("discovery failure must be explicit and rejected: %+v %v", receipt, err)
			}
			for _, call := range f.calls {
				if call[0] != "repo" || call[1] != "list" {
					t.Fatal("failed discovery mutated resources", call)
				}
			}
		})
	}
}

func TestRemoteRepositoryCreationRetainsReceiptOnMismatch(t *testing.T) {
	l, f, s := fixtureOn(t, "desktop")
	l.setup.RepositoryPath = "C:/Projects/merge-h-308-code"
	f.repoInventory = map[string]any{"repos": []any{map[string]any{"id": "different-repo", "kind": "git", "path": "/srv/merge-h-308-code"}}}
	receipt, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err == nil || !strings.Contains(err.Error(), "changed the resolved repository/runtime") || receipt.Disposition != b.OperationUnknown || len(receipt.Operation.Receipt) == 0 {
		t.Fatalf("creation mismatch lost its original receipt: %+v %v", receipt, err)
	}
	if len(f.calls) != 2 || f.calls[0][0] != "repo" || f.calls[1][0] != "worktree" {
		t.Fatal("creation mismatch continued into terminal operations", f.calls)
	}
	q := request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan})
	q.Operation.Receipt, _ = json.Marshal(receipt)
	if _, _, err = l.Reconcile(context.Background(), q); err == nil || !strings.Contains(err.Error(), "changed the resolved repository/runtime") {
		t.Fatal("recovery accepted the mismatched creation", err)
	}
	if len(f.calls) != 2 {
		t.Fatal("mismatched recovery rediscovered resources", f.calls)
	}
}
