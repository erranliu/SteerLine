//go:build windows && e2e

package testenv

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPseudoConsoleCapturesInteractiveChild(t *testing.T) {
	output := filepath.Join(t.TempDir(), "evidence")
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	scenario := &Scenario{Output: output, Context: context.Background(), t: t}
	t.Cleanup(func() {
		for _, file := range scenario.files {
			_ = file.Close()
		}
	})
	command := exec.Command("cmd.exe", "/d", "/c", "echo STEERLINE_PTY_READY & pause")
	transcript, err := scenario.RunPseudoConsole(command, "pseudo-console", "STEERLINE_PTY_READY")
	if err != nil {
		t.Fatalf("pseudo console child failed: %v\n%s", err, transcript)
	}
	if !strings.Contains(string(transcript), "STEERLINE_PTY_READY") {
		t.Fatalf("pseudo console output omitted marker: %q", transcript)
	}
}
