//go:build e2e

package testenv

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOrcaEnvironmentRegistrationCleanupEffectsAndRefusals(t *testing.T) {
	for _, fault := range []string{"none", "remote-omitted", "foreign-workspace", "changed-setup", "nonempty-terminal", "local-omitted", "delete-failed", "final-null", "final-missing", "final-truncated", "initial-null", "worktrees-null", "worktrees-truncated", "retained-workspace", "retained-setup", "delayed-workspace", "delayed-files", "retained-files", "files-failed"} {
		t.Run(fault, func(t *testing.T) {
			r := &orcaTarget{Repository: "repo", Root: "C:/source", ownedLocalSetup: true}
			removed, deleted, inventories, setups := 0, 0, 0, 0
			ctx, cancel := context.WithTimeout(context.Background(), 650*time.Millisecond)
			defer cancel()
			fileChecks := 0
			err := r.removeLocalRegistration(ctx, "C:/source", map[string]string{"C:/worker": "original"}, func(args ...string) (json.RawMessage, error) {
				switch strings.Join(args[:2], " ") {
				case "project setups":
					setups++
					if fault == "initial-null" && setups == 1 || fault == "final-null" && setups > 1 {
						return json.RawMessage(`{"setups":null}`), nil
					}
					if setups > 1 && fault == "final-missing" {
						return json.RawMessage(`{}`), nil
					}
					if setups > 1 && fault == "final-truncated" {
						return json.RawMessage(`{"setups":[],"truncated":true}`), nil
					}
					if setups > 1 && fault != "retained-setup" {
						return json.RawMessage(`{"setups":[]}`), nil
					}
					path := "C:/source"
					if fault == "changed-setup" {
						path = "C:/foreign"
					}
					return json.Marshal(map[string]any{"setups": []any{map[string]string{"id": "repo", "repoId": "repo", "hostId": "local", "path": path}}})
				case "worktree list":
					inventories++
					if fault == "worktrees-null" {
						return json.RawMessage(`{"worktrees":null,"totalCount":0,"truncated":false}`), nil
					}
					rows := []any{map[string]any{"id": "repo::C:/source", "repoId": "repo", "path": "C:/source", "hostId": "local", "isMainWorktree": true}}
					if inventories == 1 || fault == "retained-workspace" || fault == "delayed-workspace" && inventories < 3 {
						path := "C:/worker"
						if fault == "foreign-workspace" {
							path = "C:/foreign"
						}
						rows = append(rows, map[string]any{"id": "repo::" + path, "repoId": "repo", "path": path, "hostId": "local", "branch": "refs/heads/original"})
					}
					return json.Marshal(map[string]any{"worktrees": rows, "totalCount": len(rows), "truncated": fault == "worktrees-truncated"})
				case "terminal list":
					rows := []any{}
					omitted := []string{}
					if fault == "nonempty-terminal" {
						rows = append(rows, map[string]string{"handle": "foreign-terminal"})
					}
					if fault == "local-omitted" {
						omitted = []string{"local"}
					}
					if fault == "remote-omitted" {
						omitted = []string{"runtime:unrelated"}
					}
					return json.Marshal(map[string]any{"terminals": rows, "totalCount": len(rows), "truncated": false, "hostScope": map[string]any{"hostIds": []string{"local"}, "omittedHostIds": omitted}})
				case "worktree rm":
					if args[3] != "id:repo::C:/worker" || removed != 0 {
						t.Fatal("unexpected or repeated workspace removal", args)
					}
					removed++
					return json.RawMessage(`{}`), nil
				case "project setup-delete":
					if removed != 1 || args[3] != "repo" || deleted != 0 {
						t.Fatal("metadata removal before owned workspace settlement", args)
					}
					deleted++
					if fault == "delete-failed" {
						return nil, errors.New("original deletion failed")
					}
					return json.RawMessage(`{}`), nil
				default:
					t.Fatal("unexpected operation", args)
					return nil, nil
				}
			}, func() (bool, error) {
				fileChecks++
				if fault == "files-failed" {
					return false, errors.New("filesystem read failed")
				}
				return fault != "retained-files" && (fault != "delayed-files" || fileChecks >= 2), nil
			})
			pass := fault == "none" || fault == "remote-omitted" || fault == "delayed-workspace" || fault == "delayed-files"
			if pass && (err != nil || removed != 1 || deleted != 1) {
				t.Fatal("normal owned cleanup failed", err, removed, deleted)
			}
			if (fault == "retained-files" || fault == "retained-workspace" || fault == "files-failed") && deleted != 0 {
				t.Fatal("setup deleted before settlement")
			}
			if !pass && err == nil {
				t.Fatal("incomplete cleanup accepted")
			}
			if fault == "foreign-workspace" || fault == "changed-setup" || fault == "nonempty-terminal" || fault == "local-omitted" || fault == "initial-null" || fault == "worktrees-null" || fault == "worktrees-truncated" {
				if removed != 0 || deleted != 0 {
					t.Fatal("unsafe mutation before validation")
				}
			}
		})
	}
}

func TestOrcaEnvironmentLocalDeletionRequiresFilesAndGitSettlement(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", root}, args...)...)
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatalf("git %v: %v: %s", args, e, b)
		}
	}
	git("init")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "fixture")
	worker := filepath.Join(t.TempDir(), "worker")
	git("worktree", "add", "--detach", worker, "HEAD")
	owned := map[string]string{worker: "original"}
	check := func(want bool) {
		t.Helper()
		got, err := localWorktreeFilesSettled(context.Background(), root, owned)
		if err != nil || got != want {
			t.Fatalf("settled=%v err=%v want=%v", got, err, want)
		}
	}
	check(false)
	if err := os.RemoveAll(worker); err != nil {
		t.Fatal(err)
	}
	check(false) // Files gone alone cannot confirm the Git registration is gone.
	git("worktree", "prune")
	check(true)
}
