package codexworker

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestAppServerLaunchBindsNativeExecutable(t *testing.T) {
	path, err := NativeExecutable(os.Args[0])
	if err != nil || !isNativeCodexExecutable(path) {
		t.Fatalf("Go test executable was not recognized as a direct process: %q %v", path, err)
	}
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("Codex CLI is not installed")
	}
	native, err := NativeExecutable("codex")
	if err != nil || !isNativeCodexExecutable(native) {
		t.Fatalf("configured Codex command did not bind its native app-server: %q %v", native, err)
	}
	configuredVersion, configuredErr := exec.Command("codex", "--version").Output()
	nativeVersion, nativeErr := exec.Command(native, "--version").Output()
	if configuredErr != nil || nativeErr != nil || strings.TrimSpace(string(configuredVersion)) != strings.TrimSpace(string(nativeVersion)) {
		t.Fatalf("configured and native Codex versions differ: %q %q %v %v", configuredVersion, nativeVersion, configuredErr, nativeErr)
	}
}
