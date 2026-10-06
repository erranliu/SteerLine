package app

import (
	"context"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestRequirementVersionParsers(t *testing.T) {
	for _, tc := range []struct{ id, output, version string }{
		{"git", "git version 2.51.0.windows.1", "2.51.0.windows.1"},
		{"go", "go version go1.26.4 windows/amd64", "1.26.4"},
		{"python", "Python 3.13.1", "3.13.1"},
		{"git", "codex-cli 0.159.2", ""}, {"go", "unknown", ""}, {"python", "Python version unknown", ""},
	} {
		matches := requirementToolPatterns[tc.id].FindStringSubmatch(tc.output)
		version := ""
		if len(matches) == 2 {
			version = matches[1]
		}
		if version != tc.version {
			t.Fatalf("%s parser: %q => %q", tc.id, tc.output, version)
		}
	}
}

func TestRequirementToolsUseWorkerEnvironment(t *testing.T) {
	c := appRequirementConfig(t)
	c.Repository = t.TempDir()
	c.PR.TokenEnv = "PATH"
	// A service PATH that is excluded from the actual worker must not prove a
	// target tool exists. No query may use the service PATH as a substitute.
	for _, id := range []string{"go", "python"} {
		e := queryHostRequirementTool(context.Background(), c.executionConfig(), id, c.ExecutionDirectory, c.ProcessHost)
		if e.Status != executionbackend.RequirementMissing {
			t.Fatalf("service environment proved %s after worker PATH removal: %+v", id, e)
		}
	}
}

func appRequirementConfig(t *testing.T) Config {
	t.Helper()
	c := bindUnitDirectory(t, Config{Host: "local", Agent: "codex", CommandTimeout: "1s"})
	setTestCodexProfile(t, &c, codexworker.ProfileConfig{Executable: "must-not-be-called"})
	return c
}
