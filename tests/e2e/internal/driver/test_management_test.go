package driver

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func TestTMFixtureProgramEmitsActualPassAndFailure(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join("..", "..", "..", "..", "debug-template", "projects", "SteerlineTest")
	for _, p := range []string{"test-marker.ps1", ".gitignore", "SteerlineFlow/testing/test-list.json"} {
		b, e := os.ReadFile(filepath.Join(source, p))
		if e != nil {
			t.Fatal(e)
		}
		target := filepath.Join(root, p)
		if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(target, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture"}} {
		c := exec.Command("git", args...)
		c.Dir = root
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatal(e, string(b))
		}
	}
	raw, _ := os.ReadFile(filepath.Join(root, "SteerlineFlow/testing/test-list.json"))
	var catalog testmanagement.Catalogue
	if e := json.Unmarshal(raw, &catalog); e != nil {
		t.Fatal(e)
	}
	var reportIDs []string
	for _, phase := range []string{"pass", "fail"} {
		input := "fixture-ok"
		if phase == "fail" {
			input = "fixture-fail"
		}
		artifacts := filepath.Join(root, ".steerrunner/artifacts")
		if e := os.MkdirAll(artifacts, 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(artifacts, "fixture-input.txt"), []byte(input), 0600); e != nil {
			t.Fatal(e)
		}
		c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "./test-marker.ps1", "-Out", ".steerrunner/artifacts/"+phase)
		c.Dir = root
		b, e := c.CombinedOutput()
		if phase == "pass" && e != nil || phase == "fail" && (c.ProcessState == nil || c.ProcessState.ExitCode() != 1) {
			t.Fatal(phase, e, string(b))
		}
		raw, e = os.ReadFile(filepath.Join(artifacts, phase, "managed-fixture.marker.json"))
		if e != nil {
			t.Fatal(e)
		}
		r, e := testmanagement.ParseReport(raw)
		if e != nil {
			t.Fatal(e)
		}
		if !r.BeforeClean || !r.AfterClean || r.BeforeCommit != r.AfterCommit || len(r.BeforeCommit) != 40 || r.Definitions["fixture.marker"] != testmanagement.Digest(catalog.Tests[0]) || r.Results[0].Status != map[string]string{"pass": "passed", "fail": "failed"}[phase] {
			t.Fatal(string(raw))
		}
		summary, e := os.ReadFile(filepath.Join(artifacts, phase, "summary.json"))
		if e != nil || !strings.Contains(string(summary), input) {
			t.Fatal(string(summary), e)
		}
		reportIDs = append(reportIDs, r.ID)
	}
	if reportIDs[0] == reportIDs[1] {
		t.Fatal("execution IDs reused")
	}
}
