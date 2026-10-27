//go:build e2e

package testenv

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/testkit/remotetest"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
)

func RunRemoteDiscovery(t *testing.T) {
	s := NewScenario(t, remotetest.Discovery)
	if s.Run.Remote == nil || s.Run.Web == "" {
		t.Fatal("formal remote L4 input required")
	}
	p := s.NewProject("discovery")
	p.Driver.NativeHome = s.Run.AuthHome
	codex, err := driver.CodexExecutable("codex")
	s.Must(err)
	selection, err := driver.RunnerSelection(s.Run.Runner)
	s.Must(err)
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), map[string]any{"version": 3, "runner": selection, "default_execution_profile": "local-codex", "execution_profiles": []any{map[string]any{"id": "local-codex", "name": "Local catalog", "execution": map[string]any{"adapter": "codex-app-server", "target": map[string]string{"kind": "local"}, "settings": map[string]string{"executable": codex}}, "workflow": map[string]any{"id": "development", "version": 2}}, s.Run.Remote.Profile}}))
	// Keep the original discovery test's generated-package freeze boundary.
	s.Must(controlled.Write(filepath.Join(p.Root, "manifest.json"), map[string]any{"version": 1, "source_commit": s.Run.SourceCommit, "execution_digest": s.Run.Remote.ExecutionDigest, "runner": selection}))
	p.StartService()
	page, err := p.Driver.Tasks(s.Context)
	s.Must(err)
	p.ProjectID = page.ProjectID
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	s.Must(err)
	address := listener.Addr().String()
	s.Must(listener.Close())
	s.Must(controlled.Write(filepath.Join(p.Home, "web.json"), map[string]any{"version": 1, "address": address}))
	s.start(p.Command(s.Run.Web, "serve"), "discovery-web")
	s.Wait("discovery Web", func() (bool, error) {
		c, e := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if e == nil {
			c.Close()
		}
		return e == nil, nil
	})
	node, err := exec.LookPath("node")
	s.Must(err)
	browserDir := filepath.Join(s.Output, "browser")
	s.Must(os.MkdirAll(browserDir, 0700))
	cmd := p.Command(node, filepath.Join(s.Run.Root, "web", "node_modules", "@playwright", "test", "cli.js"), "test", "tests/execution-configs-live.spec.ts")
	cmd.Dir = filepath.Join(s.Run.Root, "web")
	cmd.Env = append(cmd.Env, "STEERLINE_CONFIG_SCOPE="+s.Root, "STEERLINE_CONFIG_FROZEN_INPUT="+filepath.Join(s.Run.Output, "remote-input.json"), "STEERLINE_CONFIG_WEB="+s.Run.Web, "STEERLINE_CONFIG_REMOTE="+s.Run.Remote.Profile.Execution.Target.Environment, "STEERLINE_CONFIG_PROFILE="+s.Run.Remote.Profile.ID, "STEERLINE_WEB_EVIDENCE="+browserDir)
	_, err = s.RunCommandFor(cmd, "discovery-browser", 12*time.Minute)
	s.Must(err)
	candidates, err := driver.ReadOriginalOperations(s.Context, p.Database, p.ProjectID)
	s.Must(err)
	if len(candidates) != 0 {
		t.Fatal("catalog query created candidate work")
	}
	// The browser saves the actual public API requests/results, including profile_id.
	paths, err := filepath.Glob(filepath.Join(browserDir, "*", "discovery.json"))
	s.Must(err)
	if len(paths) != 1 {
		t.Fatal("missing unique browser discovery evidence", paths)
	}
	raw, err := os.ReadFile(paths[0])
	s.Must(err)
	var methods any
	s.Must(json.Unmarshal(raw, &methods))
	s.Must(controlled.Write(filepath.Join(s.Output, "remote-discovery.json"), map[string]any{"execution_digest": s.Run.Remote.ExecutionDigest, "profile_id": s.Run.Remote.Profile.ID, "environment": s.Run.Remote.Profile.Execution.Target.Environment, "candidates": len(candidates), "browser": methods}))
	s.Must(s.FinishBody())
}
