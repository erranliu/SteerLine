//go:build windows

package pathguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveProspectiveFollowsJunctionForMissingDescendant(t *testing.T) {
	root := t.TempDir()
	debugHome := filepath.Join(root, "debug home")
	installedHome := filepath.Join(root, "installed home")
	installedState := filepath.Join(installedHome, "state")
	if err := os.MkdirAll(debugHome, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(installedState, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(debugHome, "state")
	command := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", alias, installedState)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create test junction: %v: %s", err, output)
	}
	t.Cleanup(func() { _ = os.Remove(alias) })

	prospective := filepath.Join(alias, "repository-key", "steerline.db")
	resolved, err := ResolveProspective(prospective)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(installedState, "repository-key", "steerline.db")
	if !strings.EqualFold(resolved, want) {
		t.Fatalf("resolved path=%q want junction target path %q", resolved, want)
	}
	if Within(debugHome, resolved) {
		t.Fatalf("junction target was incorrectly contained in debug home: %q", resolved)
	}
}
