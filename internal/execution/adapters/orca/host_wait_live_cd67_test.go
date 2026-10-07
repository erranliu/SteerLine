package orcaworker_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestOrcaNativeHostWait(t *testing.T) {
	path := os.Getenv("STEERLINE_ORCA_HOST_WAIT_PROBE")
	if path == "" {
		t.Skip("set an exact isolated waiting Orca terminal to run acceptance")
	}
	raw, err := os.ReadFile(path)
	require(t, err)
	var probe struct {
		Workspace   string `json:"workspace"`
		Terminal    string `json:"terminal"`
		Runtime     string `json:"runtime"`
		Incarnation string `json:"incarnation"`
	}
	require(t, json.Unmarshal(raw, &probe))
	if !filepath.IsAbs(probe.Workspace) || probe.Terminal == "" || probe.Runtime == "" || probe.Incarnation == "" {
		t.Fatal("Orca host-wait probe lacks an exact bound terminal")
	}
	exe, err := exec.LookPath("orca")
	require(t, err)
	target := executionbackend.ExecutionTarget{ID: "orca:isolated-host-wait"}
	config := orcaworker.Config{Repository: probe.Workspace, Target: target, CommandTimeout: "15s",
		Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			command := exec.CommandContext(ctx, exe, args...)
			command.Dir = probe.Workspace
			return command.Output()
		},
	}
	identity := executionbackend.ProcessIdentity{Kind: "orca", Terminal: probe.Terminal, RuntimeID: probe.Runtime, Incarnation: probe.Incarnation, Executable: exe}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	term, err := orcaworker.New(config).InspectBoundTerminal(ctx, target, identity)
	require(t, err)
	if term.AgentWait == nil || term.AgentIdentity != "codex" {
		t.Fatalf("real Orca host wait was not reported by the bound Codex terminal: %+v", term)
	}
	var wait struct {
		Source string `json:"source"`
		Since  int64  `json:"since"`
	}
	require(t, json.Unmarshal(*term.AgentWait, &wait))
	if wait.Source == "" || wait.Since == 0 {
		t.Fatalf("real Orca wait lacks source/time: %+v", wait)
	}

	t.Logf("real Orca host wait: source=%s, reported_since=%d", wait.Source, wait.Since)
}
