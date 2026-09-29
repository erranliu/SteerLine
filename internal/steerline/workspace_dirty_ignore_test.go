package steerline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorkspaceDirtyIgnoreStatus(t *testing.T) {
	rule := WorkspaceDirtyIgnore{Paths: []string{"Test-Log/", "notes.txt"}, Extensions: []string{".meta"}}
	if err := rule.validate(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		status string
		dirty  bool
	}{
		{"clean", "", false},
		{"generated meta", "?? Assets/Activity.meta\x00 D Assets/Old.cs.meta\x00", false},
		{"ignored directory", "?? Test-Log/run.log\x00", false},
		{"source change", "?? Assets/New.cs\x00", true},
		{"rename leaves ignored scope", "R  Assets/New.cs\x00Assets/Old.cs.meta\x00", true},
		{"rename within ignored scope", "R  Assets/New.cs.meta\x00Assets/Old.cs.meta\x00", false},
		{"nonterminated status", "?? Assets/Activity.meta", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dirty, err := rule.dirtyFromPorcelain([]byte(test.status))
			if dirty != test.dirty || (err != nil) != (test.name == "nonterminated status") {
				t.Fatalf("dirty=%t err=%v", dirty, err)
			}
		})
	}
	for _, path := range []string{".", "../outside", "/root", "C:/outside", ".git/"} {
		if err := (WorkspaceDirtyIgnore{Paths: []string{path}}).validate(); err == nil {
			t.Errorf("accepted unsafe path %q", path)
		}
	}
}

func TestWorkspaceDirtyIgnoreUsesRealGitStatus(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "old.cs.meta"), []byte("tracked"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "base")
	if err := os.Remove(filepath.Join(repo, "old.cs.meta")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.cs.meta"), []byte("generated"), 0600); err != nil {
		t.Fatal(err)
	}
	c := Config{Git: Command{Executable: "git"}, CommandTimeout: "10s", WorkspaceDirtyIgnore: WorkspaceDirtyIgnore{Extensions: []string{".meta"}}}
	if dirty, err := inspectGitDirty(context.Background(), c, repo); err != nil || dirty {
		t.Fatalf("meta-only status should be effectively clean: dirty=%t err=%v", dirty, err)
	}
	if err := os.WriteFile(filepath.Join(repo, "source.cs"), []byte("code"), 0600); err != nil {
		t.Fatal(err)
	}
	if dirty, err := inspectGitDirty(context.Background(), c, repo); err != nil || !dirty {
		t.Fatalf("source changes must remain dirty: dirty=%t err=%v", dirty, err)
	}
	c.WorkspaceDirtyIgnore = WorkspaceDirtyIgnore{}
	if dirty, err := inspectGitDirty(context.Background(), c, repo); err != nil || !dirty {
		t.Fatalf("default policy must be strict: dirty=%t err=%v", dirty, err)
	}
}
