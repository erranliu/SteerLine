package model_test

import (
	"fmt"
	"testing"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
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
			doc := fmt.Sprintf("```steerline-environment\n{\"version\":1,\"script\":\"scripts/verify-environment.%s\",\"interpreter\":%q,\"validation_base\":\"validation\",\"dependencies\":[]}\n```", tc.script, tc.interpreter)
			_, err := env.ParseManifest([]byte(doc))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
