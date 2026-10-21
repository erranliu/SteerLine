package testenv

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func ProjectMaintenance(s *Scenario) {
	s.t.Helper()
	cmd := exec.CommandContext(s.Context, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(s.Run.Root, "scripts", "test-project-maintenance.ps1"), "-SteerRunnerExe", s.Run.Runner, "-Out", s.Output)
	cmd.Dir = s.Run.Root
	b, e := cmd.CombinedOutput()
	s.Must(os.WriteFile(filepath.Join(s.Output, "controlled-driver.log"), b, 0600))
	s.externalCleanupError = "maintenance debug package closure unverified; preserve evidence"
	if raw, err := os.ReadFile(filepath.Join(s.Output, "debug-package.txt")); err == nil {
		s.register("debug-package", string(raw), 0)
	}
	var proof struct {
		Passed           bool `json:"passed"`
		Stopped          bool `json:"stopped"`
		ResourcesSettled bool `json:"resources_settled"`
	}
	raw, readErr := os.ReadFile(filepath.Join(s.Output, "proof.json"))
	if readErr == nil && json.Unmarshal(raw, &proof) == nil && proof.Stopped && proof.ResourcesSettled {
		s.externalCleanupError = ""
	}
	if e != nil {
		s.Must(fmt.Errorf("maintenance controlled: %w: %s", e, b))
	}
	s.Must(readErr)
	if !proof.Passed || !proof.Stopped || !proof.ResourcesSettled {
		s.t.Fatal("maintenance proof incomplete")
	}
}
