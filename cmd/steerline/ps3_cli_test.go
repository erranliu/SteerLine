package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPS3RetiredCommandsAndGraphFlagsFailBeforeOpeningProject(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-config.json")
	original := os.Args
	defer func() { os.Args = original }()
	for _, command := range []string{"tasks", "run", "serve-app", "stop-app", "start-service", "stop-service", "console", "projects", "certify-backend"} {
		os.Args = []string{"steerline", command, "-config", missing}
		if err := run(); err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Fatalf("%s did not retire before config I/O: %v", command, err)
		}
	}
	for _, args := range [][]string{
		{"export", "-config", missing},
		{"graph", "-config", missing, "-task-id", "1"},
		{"graph", "-config", missing, "-status", "open"},
		{"graph", "-config", missing, "-direction", "both"},
		{"graph", "-config", missing, "-depth", "1"},
		{"graph", "-config", missing, "-output", filepath.Join(t.TempDir(), "graph.json")},
	} {
		err := runNativeTaskCommand(args)
		if err == nil || !(strings.Contains(err.Error(), "unsupported task command") || strings.Contains(err.Error(), "does not accept") || strings.Contains(err.Error(), "flag provided but not defined")) {
			t.Fatalf("%v reached config I/O: %v", args, err)
		}
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("retired command created project config")
	}
}
