//go:build e2e

package testenv

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	process "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

type progressBrowser struct {
	Process        *process.Process
	Target, Output string
}

func startProgressBrowser(s *Scenario, p *Project) *progressBrowser {
	web := os.Getenv("STEERLINE_PROGRESS_WEB")
	if web == "" {
		if os.Getenv("STEERLINE_PROGRESS_REMOTE_PROFILE") != "" {
			s.t.Fatal("remote progress acceptance requires a real browser")
		}
		return nil
	}
	if !filepath.IsAbs(web) {
		s.t.Fatal("Web executable must be absolute")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	s.Must(err)
	address := listener.Addr().String()
	s.Must(listener.Close())
	s.Must(controlled.Write(filepath.Join(p.Home, "web.json"), map[string]any{"version": 1, "address": address}))
	s.start(p.Command(web, "serve"), "progress-web")
	s.Wait("real Web listener", func() (bool, error) {
		c, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			c.Close()
		}
		return err == nil, err
	})
	node, err := exec.LookPath("node")
	s.Must(err)
	cli := filepath.Join(s.Run.Root, "web", "node_modules", "@playwright", "test", "cli.js")
	if _, err := os.Stat(cli); err != nil {
		s.Must(err)
	}
	b := &progressBrowser{Target: filepath.Join(s.Output, "browser-target.json"), Output: filepath.Join(s.Output, "browser")}
	s.Must(os.MkdirAll(b.Output, 0700))
	cmd := p.Command(node, cli, "test", "tests/project-progress-live.spec.ts")
	cmd.Dir = filepath.Join(s.Run.Root, "web")
	cmd.Env = append(cmd.Env, "STEERLINE_PROGRESS_WEB="+web, "STEERLINE_PROGRESS_HOME="+p.Home, "STEERLINE_PROGRESS_TARGET="+b.Target, "STEERLINE_WEB_EVIDENCE="+b.Output)
	b.Process = s.start(cmd, "progress-browser")
	s.Wait("one authorized Edge page before dispatch", func() (bool, error) {
		select {
		case <-b.Process.Done:
			s.t.Fatal("browser exited before dispatch; see progress-browser.log")
		default:
		}
		_, err := os.Stat(filepath.Join(b.Output, "ready.json"))
		return err == nil, nil
	})
	return b
}

func (b *progressBrowser) Bind(s *Scenario, project, task, candidate string) {
	s.Must(controlled.Write(b.Target, map[string]string{"project": project, "task": task, "candidate": candidate}))
}

func (b *progressBrowser) Verify(s *Scenario, runner map[uint64]map[string]bool) {
	ctx, cancel := context.WithTimeout(s.Context, 60*time.Second)
	defer cancel()
	s.Must(b.Process.Wait(ctx))
	raw, err := os.ReadFile(filepath.Join(b.Output, "observations.json"))
	s.Must(err)
	var proof struct {
		Passed  bool `json:"passed"`
		Reloads int  `json:"document_requests_during_observation"`
		States  []struct {
			Version uint64  `json:"version"`
			State   [3]bool `json:"state"`
		} `json:"states"`
	}
	s.Must(json.Unmarshal(raw, &proof))
	if !proof.Passed || proof.Reloads != 0 || len(proof.States) != 6 {
		s.t.Fatal("continuous browser proof incomplete")
	}
	keys := []string{"demo.progress.roundtrip", "demo.progress.retry", "demo.progress.cleanup"}
	for _, row := range proof.States {
		fields, ok := runner[row.Version]
		if !ok {
			s.t.Fatal("browser version lacks original Runner snapshot", row.Version)
		}
		for i, key := range keys {
			if row.State[i] != fields["test."+key] {
				s.t.Fatal(fmt.Sprintf("browser/Runner differ at version %d: %s", row.Version, key))
			}
		}
	}
}
