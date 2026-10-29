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

func desktopOpenFixture() {
	marker := os.Args[len(os.Args)-1]
	if slices.Contains(os.Args, "--child") {
		time.Sleep(250 * time.Millisecond)
		if os.WriteFile(marker, []byte("desktop survived"), 0600) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if os.Getenv("STEERLINE_DESKTOP_TEST_ROUTE") != "" {
		os.Exit(2)
	}
	cmd := exec.Command(os.Args[0], "--desktop-open-fixture", "--child", marker)
	hideDesktopLauncher(cmd)
	if cmd.Start() != nil {
		os.Exit(1)
	}
	if cmd.Process.Release() != nil {
		os.Exit(1)
	}
	os.Exit(0)
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
	_, err := OutputDesktopOpen(context.Background(), b.LaunchCommand{Executable: os.Args[0], Args: []string{"--desktop-open-fixture", marker}, Timeout: 5 * time.Second}, environment)
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
