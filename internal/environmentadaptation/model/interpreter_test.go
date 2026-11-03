package model

import (
	"fmt"
	"testing"
)

func TestManifestDeclaresWindowsPowerShellOrBash(t *testing.T) {
	for _, tc := range []struct {
		script, interpreter string
		valid               bool
	}{
		{"ps1", "powershell.exe", true},
		{"sh", "bash", true},
		{"ps1", "pwsh", false},
		{"ps1", "bash", false},
		{"sh", "powershell.exe", false},
		{"ps1", "", false},
	} {
		t.Run(tc.script+"-"+tc.interpreter, func(t *testing.T) {
			doc := fmt.Sprintf("```steerline-environment\n{\"version\":5,\"script\":%q,\"interpreter\":%q,\"dependencies\":[]}\n```", ScriptPath(tc.script), tc.interpreter)
			_, err := ParseManifest([]byte(doc))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
