package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativePreflightRejectsSharedSQLiteBeforeValidator(t *testing.T) {
	config, _ := nativeTaskFixture(t)
	if err := os.MkdirAll(config.RunDir, 0700); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(config.RunDir, "steerline.db")
	if err := os.WriteFile(database, nil, 0600); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(t.TempDir(), "installed-wal")
	if err := os.WriteFile(sentinel, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(sentinel, database+"-wal"); err != nil {
		t.Fatal(err)
	}
	input := hostConfigForTest(config)
	called := false
	input.ValidateNativeStore = func(context.Context, Config) error { called = true; return nil }
	err := PreflightNativeStatePath(input)
	if err == nil || !strings.Contains(err.Error(), "must not be a hard link") || called {
		t.Fatalf("shared state reached store validator: err=%v called=%v", err, called)
	}
}

func TestNativeStatePreservesWorktreeMetadata(t *testing.T) {
	for _, populated := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "populated"}[populated], func(t *testing.T) {
			runDir := t.TempDir()
			trash := filepath.Join(runDir, "worktrees", ".worker-trash")
			if err := os.MkdirAll(trash, 0700); err != nil {
				t.Fatal(err)
			}
			evidence := filepath.Join(trash, "retained-worktree.txt")
			if populated {
				if err := os.WriteFile(evidence, []byte("retained native resource"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := ensureNativeStateFilesAllowed(runDir, []string{".worker-trash"}); err != nil {
				t.Fatalf("declared administrative directory blocked state access: %v", err)
			}
			if _, err := os.Stat(trash); err != nil {
				t.Fatal("state validation removed native trash", err)
			}
			if populated {
				content, err := os.ReadFile(evidence)
				if err != nil || string(content) != "retained native resource" {
					t.Fatal("state validation changed native trash", err)
				}
			}
		})
	}
}

func TestNativeStateMetadataExceptionIsScopedToRealWorktreeDirectory(t *testing.T) {
	for _, tc := range []struct {
		name, root, entry string
		file              bool
	}{
		{"unknown worktree directory", "worktrees", ".unknown", false},
		{"trash name in runs", "runs", ".worker-trash", false},
		{"trash file", "worktrees", ".worker-trash", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runDir := t.TempDir()
			root := filepath.Join(runDir, tc.root)
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			if tc.entry != ".worker-trash" {
				if err := os.Mkdir(filepath.Join(root, ".worker-trash"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			entry := filepath.Join(root, tc.entry)
			var err error
			if tc.file {
				err = os.WriteFile(entry, nil, 0600)
			} else {
				err = os.Mkdir(entry, 0700)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := ensureNativeStateFilesAllowed(runDir, []string{".worker-trash"}); err == nil {
				t.Fatal("trash exception accepted an unsupported state identity")
			}
		})
	}
}
