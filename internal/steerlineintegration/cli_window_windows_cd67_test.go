//go:build windows

package steerlineintegration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsServiceWindowRejectsMissingConsole(t *testing.T) {
	f := newCLILifecycleFixture(t)
	config, configPath := f.c, f.path
	serviceExe := buildProductLifecycleCLI(t)
	cmd := exec.Command(serviceExe, "run", "-config", configPath, "-dispatch", "manual", "-console", "summary", "-service-window")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true}
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "requires a visible Windows console") {
		t.Fatalf("service-window accepted a hidden process: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(config.RunDir, "service")); !os.IsNotExist(err) {
		t.Fatalf("hidden service-window wrote startup state: %v", err)
	}
}
