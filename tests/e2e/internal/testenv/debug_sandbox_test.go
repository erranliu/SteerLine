package testenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDebugSandboxFailurePreservesOriginalPackage(t *testing.T) {
	for _, complete := range []bool{true, false} {
		s := &Scenario{debugSandboxRoot: t.TempDir(), Output: t.TempDir(), debugSandboxResult: map[string]any{"passed": false, "cleanup_complete": complete}}
		packageRoot := filepath.Join(s.debugSandboxRoot, "generated")
		evidence := filepath.Join(packageRoot, "evidence")
		if err := os.MkdirAll(evidence, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(evidence, "sandbox-service.log"), []byte("original failure"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := s.cleanupDebugSandbox(); (err == nil) != complete {
			t.Fatalf("cleanup complete=%v: %v", complete, err)
		}
		if _, err := os.Stat(packageRoot); err != nil {
			t.Fatal("original failure package lost", err)
		}
		if b, err := os.ReadFile(filepath.Join(s.Output, "debug-evidence", "sandbox-service.log")); err != nil || string(b) != "original failure" {
			t.Fatalf("failure evidence lost: %q %v", b, err)
		}
	}
}
