package codexworker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestProbeRejectsVersionMismatchBeforeDetachedStart(t *testing.T) {
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
		HistoryAvailable: func(context.Context) bool { return true },
		StartDetached:    func(*exec.Cmd) error { started = true; return nil },
	})
	var unsupported *ProbeError
	if !errors.As(err, &unsupported) || started || outputs != 2 {
		t.Fatalf("mismatched Codex binaries reached detached start: %v, started=%v, probes=%d", err, started, outputs)
	}
}
