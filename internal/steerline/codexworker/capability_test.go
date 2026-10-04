package codexworker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestProbeRejectsVersionMismatchBeforeManagedStart(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	outputs := 0
	started := false
	err = ProbeCapabilities(context.Background(), ProbeConfig{Executable: executable, Repository: t.TempDir(), RunDir: t.TempDir()}, ProbePort{
		Output: func(context.Context, string, []string) ([]byte, error) {
			outputs++
			if outputs == 1 {
				return []byte("configured version"), nil
			}
			return []byte("different native version"), nil
		},
		CheckHistory: func(context.Context) error { return nil },
		StartManaged: func(*exec.Cmd) (func(), error) { started = true; return func() {}, nil },
	})
	var unsupported *ProbeError
	if !errors.As(err, &unsupported) || started || outputs != 2 {
		t.Fatalf("mismatched Codex binaries reached managed start: %v, started=%v, probes=%d", err, started, outputs)
	}
}

func TestProbePreservesHistoryFailureBeforeManagedStart(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("history initialization fixture failed")
	started := false
	err = ProbeCapabilities(context.Background(), ProbeConfig{Executable: executable}, ProbePort{
		Output: func(_ context.Context, _ string, args []string) ([]byte, error) {
			if args[len(args)-1] == "--version" {
				return []byte("codex-cli 0.159.2"), nil
			}
			return []byte("app-server --listen"), nil
		},
		CheckHistory: func(context.Context) error { return cause },
		StartManaged: func(*exec.Cmd) (func(), error) {
			started = true
			return func() {}, nil
		},
	})
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), cause.Error()) || started {
		t.Fatalf("history failure lost its cause or started a worker: %v, started=%v", err, started)
	}
}
