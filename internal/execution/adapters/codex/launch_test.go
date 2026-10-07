package codexworker

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestLaunchServerFixesWindowsSandboxBeforeStarting(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows launch contract")
	}
	called := false
	sentinel := errors.New("captured launch")
	config := LaunchConfig{Executable: os.Args[0], Workspace: t.TempDir(), ServerDir: t.TempDir(), Sandbox: "workspace-write", WindowsSandbox: "unelevated"}
	_, _, _, err := LaunchServer(config, func(cmd *exec.Cmd) (int, error) {
		called = true
		if !strings.Contains(strings.Join(cmd.Args, " "), `--config windows.sandbox=unelevated`) {
			t.Fatalf("missing fixed mode: %v", cmd.Args)
		}
		return 0, sentinel
	}, func(env []string) []string { return env })
	if !called || !errors.Is(err, sentinel) {
		t.Fatalf("launch not observed: %v", err)
	}
	config.WindowsSandbox = "elevated"
	called = false
	_, _, _, err = LaunchServer(config, func(cmd *exec.Cmd) (int, error) { called = true; return 0, sentinel }, func(env []string) []string { return env })
	if called || err == nil {
		t.Fatal("unsupported mode reached process start")
	}
}
