//go:build windows

package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeStateRejectsMetadataJunction(t *testing.T) {
	for _, outside := range []bool{false, true} {
		t.Run(map[bool]string{false: "inside", true: "outside"}[outside], func(t *testing.T) {
			runDir := t.TempDir()
			root := filepath.Join(runDir, "worktrees")
			target := filepath.Join(root, strings.Repeat("a", 32))
			if outside {
				target = t.TempDir()
			}
			if err := os.MkdirAll(target, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(root, ".worker-trash")
			if output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
				t.Fatalf("create fixture junction: %v: %s", err, output)
			}
			t.Cleanup(func() { _ = os.Remove(link) })
			if err := ensureNativeStateFilesAllowed(runDir, []string{".worker-trash"}); err == nil {
				t.Fatal("administrative junction bypassed state containment/type validation")
			}
		})
	}
}
