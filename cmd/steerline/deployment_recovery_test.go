package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeploymentRecoveryArguments(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	previous := os.Args
	defer func() { os.Args = previous }()
	for _, args := range [][]string{
		{"prepare-deployment", "--no-open"},
		{"prepare-deployment", "--goal", "reset"},
		{"prepare-deployment", "--goal", "update", "--runner-exe", "relative"},
		{"verify-stop", "--goal", "update"},
		{"recover-deployment", "--no-open"},
		{"recover-deployment", "--detached"},
		{"recover-deployment", "--runner-exe", "relative-runner", "unexpected"},
	} {
		os.Args = append([]string{"steerline"}, args...)
		if err := run(); err == nil {
			t.Fatal("invalid recovery options accepted", args)
		}
	}
	os.Args = []string{"steerline", "recover-deployment", "--runner-exe", filepath.Join(t.TempDir(), "runner.exe"), "--json"}
	if err := run(); err == nil || !strings.Contains(err.Error(), "exactly exited original host") {
		t.Fatal("recovery command bypassed original host admission", err)
	}
}
