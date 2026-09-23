package steerline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSteerlineHomeAndDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STEERLINE_HOME", home)
	if got := SteerlineHome(); got != filepath.Clean(home) {
		t.Fatalf("home=%q want %q", got, home)
	}
	if SteerlineBinDir() != filepath.Join(home, "bin") ||
		SteerlineEtcDir() != filepath.Join(home, "etc") ||
		SteerlineStateRoot() != filepath.Join(home, "state") {
		t.Fatal("derived directories are wrong")
	}
}

func TestInstallExecutableCopiesAndOverwrites(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	write := func(name, content string) string {
		p := filepath.Join(t.TempDir(), name)
		if e := os.WriteFile(p, []byte(content), 0700); e != nil {
			t.Fatal(e)
		}
		return p
	}
	dst, e := InstallExecutable(write("one.bin", "binary-one"))
	if e != nil {
		t.Fatal(e)
	}
	if b, _ := os.ReadFile(dst); string(b) != "binary-one" {
		t.Fatalf("installed content %q", b)
	}
	// Installing the same source again is a no-op.
	if _, e := InstallExecutable(dst); e != nil {
		t.Fatalf("reinstall: %v", e)
	}
	dst2, e := InstallExecutable(write("two.bin", "binary-two"))
	if e != nil {
		t.Fatal(e)
	}
	if dst2 != dst {
		t.Fatalf("destination changed: %s vs %s", dst2, dst)
	}
	if b, _ := os.ReadFile(dst); string(b) != "binary-two" {
		t.Fatalf("overwrite failed: %q", b)
	}
}

// TestDerivedRunDirIsStableAcrossWorktrees is the core promise: any worktree of
// the same repository resolves to the same state directory.
func TestDerivedRunDirIsStableAcrossWorktrees(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	f := newFixture(t)

	base := f.c
	base.RunDir = ""
	base.WorktreeRoot = ""
	load := func(repo string) Config {
		c := base
		c.Repository = repo
		b, e := json.Marshal(c)
		if e != nil {
			t.Fatal(e)
		}
		p := filepath.Join(t.TempDir(), "config.json")
		if e := os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
		loaded, e := LoadConfig(p)
		if e != nil {
			t.Fatal(e)
		}
		return loaded
	}

	fromMain := load(f.c.Repository)
	if !strings.Contains(fromMain.RunDir, string(filepath.Separator)+"state"+string(filepath.Separator)) {
		t.Fatalf("run_dir was not derived under state: %s", fromMain.RunDir)
	}
	if fromMain.WorktreeRoot != filepath.Join(fromMain.RunDir, "worktrees") {
		t.Fatalf("worktree_root was not derived: %s", fromMain.WorktreeRoot)
	}

	linked := filepath.Join(t.TempDir(), "linked", "repo")
	if e := os.MkdirAll(filepath.Dir(linked), 0700); e != nil {
		t.Fatal(e)
	}
	fixtureGit(t, f.c.Repository, "worktree", "add", "--no-track", "-b", "wt-derived", linked)

	fromLinked := load(linked)
	if fromLinked.RunDir != fromMain.RunDir {
		t.Fatalf("state dir drifted across worktrees:\n  main   %s\n  linked %s", fromMain.RunDir, fromLinked.RunDir)
	}
}

// TestExplicitRunDirStillWins keeps existing configs working unchanged.
func TestExplicitRunDirStillWins(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	f := newFixture(t)
	b, e := json.Marshal(f.c)
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "config.json")
	if e := os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	loaded, e := LoadConfig(p)
	if e != nil {
		t.Fatal(e)
	}
	if loaded.RunDir != f.c.RunDir {
		t.Fatalf("explicit run_dir overridden: %s want %s", loaded.RunDir, f.c.RunDir)
	}
}
