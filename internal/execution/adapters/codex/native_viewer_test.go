package codexworker

import (
	"os"
	"strings"
	"testing"
)

func TestNativeViewerAttachesWithoutPromptOrPolicyOverrides(t *testing.T) {
	binding := AppServerBinding{SessionID: "00000000-0000-0000-0000-000000000001", Endpoint: "ws://127.0.0.1:12345", Generation: "original"}
	workspace := t.TempDir()
	command, err := NativeViewerCommand(os.Args[0], workspace, binding)
	if err != nil {
		t.Fatal(err)
	}
	if len(command.Args) != 6 || command.Args[1] != "--remote" || command.Args[2] != binding.Endpoint || command.Args[4] != workspace || command.Args[5] != binding.SessionID || strings.Contains(strings.Join(command.Args, " "), "sandbox") {
		t.Fatalf("viewer starts input or overrides worker settings: %+v", command)
	}
	for _, endpoint := range []string{"", "ws://outside.example:12345", "ws://127.0.0.1:0"} {
		binding.Endpoint = endpoint
		if _, err := NativeViewerCommand(os.Args[0], workspace, binding); err == nil {
			t.Fatalf("invalid endpoint accepted: %q", endpoint)
		}
	}
}
