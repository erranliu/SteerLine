package runnerclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeSessionViewerCLIOriginalTarget(t *testing.T) {
	exe, _ := os.Executable()
	call := filepath.Join(t.TempDir(), "call")
	t.Setenv("STEERLINE_RUNNER_CLI_CALL", call)
	target := NativeSessionTarget{RunID: "run", AttemptID: "attempt", SessionID: "session", Generation: "generation"}
	for _, mode := range []string{"native", "native-wrong", "native-broken"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", mode)
			err := (Client{Executable: exe, StateDir: t.TempDir()}).OpenNativeSession(context.Background(), target)
			if mode == "native" && err != nil {
				t.Fatal(err)
			}
			if mode != "native" {
				var f *Failure
				if !errors.As(err, &f) || !f.Unknown {
					t.Fatal("viewer outcome guessed", err)
				}
			}
			raw, _ := os.ReadFile(call)
			for _, value := range []string{"open-native-session\n", "--attempt-id\nattempt\n", "--session-id\nsession\n", "--generation\ngeneration\n"} {
				if !strings.Contains(string(raw), value) {
					t.Fatal("lost target identity", string(raw))
				}
			}
			if strings.Contains(string(raw), "--input\n") || strings.Contains(string(raw), "ws://") {
				t.Fatal("viewer sent input or private endpoint")
			}
		})
	}
}
