package testenv

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	feedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
)

func TestManagement(s *Scenario) {
	s.t.Helper()
	cmd := exec.CommandContext(s.Context, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(s.Run.Root, "scripts", "test-test-management.ps1"), "-SteerRunnerExe", s.Run.Runner, "-Out", s.Output)
	cmd.Dir = s.Run.Root
	output, err := cmd.CombinedOutput()
	s.Must(os.WriteFile(filepath.Join(s.Output, "live-driver.log"), output, 0600))
	s.externalCleanupError = "test management package closure unverified; preserve its evidence"
	if raw, e := os.ReadFile(filepath.Join(s.Output, "debug-package.txt")); e == nil {
		s.register("debug-package", string(raw), 0)
	}
	raw, readErr := os.ReadFile(filepath.Join(s.Output, "proof.json"))
	var proof feedback.TestManagementProof
	if readErr == nil && json.Unmarshal(raw, &proof) == nil && proof.Stopped && proof.Cleanup {
		s.externalCleanupError = ""
	}
	if err != nil {
		s.Must(fmt.Errorf("testing live: %w: %s", err, output))
	}
	s.Must(readErr)
	s.Must(proof.Validate())
	s.TestingProof = &proof
}
