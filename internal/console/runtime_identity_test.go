package console

import (
	"strings"
	"testing"
)

func TestConsoleDiagnosticsShowRuntimeIdentity(t *testing.T) {
	ref := testProject("runtime")
	ref.ExecutablePath = `C:\SteerLine Debug\bin\steerline.exe`
	ref.ApplicationHome = `C:\SteerLine Debug`
	ref.ConfigPath = `C:\SteerLine Debug\etc\runtime.json`
	ref.SourceCommit = strings.Repeat("a", 40)
	ref.SourceDirty = "true"
	state := projectState{ref: ref}
	lines := strings.Join(renderEmptySelection(state), "\n")
	for _, expected := range []string{ref.ExecutablePath, ref.ApplicationHome, ref.ConfigPath, ref.SourceCommit, "dirty=true"} {
		if !strings.Contains(lines, expected) {
			t.Fatalf("console diagnostic did not show %q:\n%s", expected, lines)
		}
	}
}
