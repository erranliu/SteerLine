package runnerstatepath

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	httpfs "github.com/erranliu/SteerLine/internal/product/httpfs"
)

func TestRunnerSocketWorksForDeepApplicationHome(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), strings.Repeat("deep-home-", 12), "state", "SteerlineTest-b55e8cc42ae8")
	dir := Directory(runDir)
	if err := Validate(runDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("read-only validation created state: %v", err)
	}
	if err := Prepare(runDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := RemoveStopped(runDir); err != nil {
			t.Error(err)
		}
	})
	if err := httpfs.VerifyPrivateDirectory(dir); err != nil {
		t.Fatal(err)
	}
	// Use the actual pinned Runner Git-cache/socket shape. This binds a real
	// OS socket; a length assertion alone would miss OS-specific constraints.
	socket := filepath.Join(dir, "r", strings.Repeat("a", 26), ".git", "objects", ".sr-auth-12345678.sock")
	if len(socket) > 106 {
		t.Fatalf("socket too long (%d): %s", len(socket), socket)
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("home depth=%d, state=%s, socket bytes=%d", len(runDir), dir, len(socket))
}

func TestRunnerNamespaceIsolationAndCleanup(t *testing.T) {
	home := t.TempDir()
	first := filepath.Join(home, "one", "state", "same-project")
	second := filepath.Join(home, "two", "state", "same-project")
	third := filepath.Join(home, "one", "state", "other-project")
	if Directory(first) == Directory(second) || Directory(first) == Directory(third) {
		t.Fatal("different home or project shared Runner state")
	}
	if Directory(first) != Directory(filepath.Join(first, ".")) {
		t.Fatal("path cleaning changed namespace")
	}
	if runtime.GOOS == "windows" && Directory(first) != Directory(strings.ToUpper(first)) {
		t.Fatal("Windows path casing changed namespace")
	}
	if err := Validate("relative"); err == nil {
		t.Fatal("relative application directory accepted")
	}
	for _, run := range []string{first, second} {
		if err := Prepare(run); err != nil {
			t.Fatal(err)
		}
		run := run
		t.Cleanup(func() { _ = RemoveStopped(run) })
		if err := os.WriteFile(filepath.Join(Directory(run), "sentinel"), []byte(run), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveStopped(first); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Directory(first)); !os.IsNotExist(err) {
		t.Fatalf("owned directory not removed: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(Directory(second), "sentinel")); err != nil || string(raw) != second {
		t.Fatalf("cleanup changed another home's Runner data: %s %v", raw, err)
	}
}

func TestRunnerRejectsRedirectedStateBeforeCreateOrCleanup(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "state", "project")
	dir := Directory(runDir)
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	sentinel := filepath.Join(target, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, dir); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		if out, junctionErr := exec.Command("cmd", "/c", "mklink", "/J", dir, target).CombinedOutput(); junctionErr != nil {
			t.Fatalf("create directory junction: %v: %s", junctionErr, out)
		}
	}
	t.Cleanup(func() { _ = os.Remove(dir) })
	for _, action := range []func(string) error{Validate, Prepare, RemoveStopped} {
		if err := action(runDir); err == nil {
			t.Fatal("redirected Runner namespace accepted")
		}
	}
	if raw, err := os.ReadFile(sentinel); err != nil || string(raw) != "preserve" {
		t.Fatalf("redirect target changed: %s %v", raw, err)
	}
}
