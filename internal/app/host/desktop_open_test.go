package host

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestDesktopOpenHelper(t *testing.T) {
	if !slices.Contains(os.Args, "-test.run=^TestDesktopOpenHelper$") {
		t.Skip("subprocess helper")
	}
	marker := os.Args[len(os.Args)-1]
	if slices.Contains(os.Args, "--child") {
		time.Sleep(250 * time.Millisecond)
		if err := os.WriteFile(marker, []byte("desktop survived"), 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if os.Getenv("STEERLINE_DESKTOP_TEST_ROUTE") != "" {
		t.Fatal("desktop launcher inherited remote routing")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestDesktopOpenHelper$", "--", "--child", marker)
	hideDesktopLauncher(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Process.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestDesktopOpenPreservesDesktopAfterLauncherExit(t *testing.T) {
	t.Setenv("STEERLINE_DESKTOP_TEST_ROUTE", "foreign")
	marker := t.TempDir() + string(os.PathSeparator) + "desktop.txt"
	environment := []string{}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "STEERLINE_DESKTOP_TEST_ROUTE=") {
			environment = append(environment, entry)
		}
	}
	_, err := OutputDesktopOpen(context.Background(), b.LaunchCommand{Executable: os.Args[0], Args: []string{"-test.run=^TestDesktopOpenHelper$", "--", marker}, Timeout: 5 * time.Second}, environment)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(marker); err == nil && string(raw) == "desktop survived" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("desktop child was killed with its launcher")
}
