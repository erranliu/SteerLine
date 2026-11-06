//go:build e2e

package testenv

import (
	"context"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (p *Project) cleanupRemoteValidation(base string) {
	s := p.Scenario
	p.RemoteCleanupFailed = true
	ownedWorkspaces := map[string]string{}
	ownedReports := map[string]string{}
	defer func() {
		if !p.RemoteCleanupFailed && p.orcaEvidence != nil && p.orcaEvidence.ownedLocalSetup {
			if err := p.orcaEvidence.cleanupLocalRegistration(s, p, ownedWorkspaces, ownedReports); err != nil {
				p.RemoteCleanupFailed = true
				s.t.Error("own local Orca registration cleanup unverified:", err)
			}
		}
	}()
	p.StopService()
	// Failure/unverified records retain the exact refs instead of hiding an
	// deleting branches still held by Runner.
	_, err := os.Stat(filepath.Join(s.Output, "environment-proof.json"))
	if err != nil {
		p.RemoteCleanupFailed = true
		s.t.Error("retain original remote refs: environment result unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := p.Driver.CleanupOwnedRunner(ctx, s.Run.Helper); err != nil {
		p.RemoteCleanupFailed = true
		p.RunnerCleanupFailed = true
		s.t.Error("retain refs: original Runner cleanup unverified:", err)
		return
	}
	if !strings.HasPrefix(base, "codex/e2e/") || !strings.HasPrefix(p.BaseBranch, "codex/e2e/") {
		p.RemoteCleanupFailed = true
		s.t.Error("unowned validation refs")
		return
	}
	refs := map[string]bool{base: true, p.BaseBranch: true}
	reader, err := sqlite.OpenSQLiteReader(p.Database)
	if err != nil {
		p.RemoteCleanupFailed = true
		s.t.Error(err)
		return
	}
	defer reader.Close()
	activities, err := reader.Environments(ctx, p.ProjectID)
	if err != nil {
		p.RemoteCleanupFailed = true
		s.t.Error(err)
		return
	}
	for _, activity := range activities {
		for _, round := range activity.Rounds {
			if round.Run != nil && round.Run.Execution.Branch != "" {
				refs[round.Run.Execution.Branch] = true
				if round.Run.Execution.Workspace != "" {
					ownedWorkspaces[round.Run.Execution.Workspace] = round.Run.Execution.Branch
					ownedReports[round.Run.Execution.Workspace] = round.Run.RunID
				}
			}
		}
	}
	candidates, err := driver.ReadOriginalOperations(ctx, p.Database, p.ProjectID)
	if err != nil {
		p.RemoteCleanupFailed = true
		s.t.Error(err)
		return
	}
	for _, candidate := range candidates {
		if candidate.Project != p.ProjectID {
			p.RemoteCleanupFailed = true
			s.t.Error("candidate ref ownership differs")
			return
		}
		if candidate.Branch != "" {
			refs[candidate.Branch] = true
		}
		if candidate.Projection != nil && candidate.Projection.Execution.Workspace != "" && candidate.Projection.Execution.Branch != "" {
			ownedWorkspaces[candidate.Projection.Execution.Workspace] = candidate.Projection.Execution.Branch
			ownedReports[candidate.Projection.Execution.Workspace] = candidate.RunID
		}
	}
	for ref := range refs {
		if ref == "AIRefactor" || ref == "main" || ref == "master" || strings.HasPrefix(ref, "-") {
			p.RemoteCleanupFailed = true
			s.t.Error("unsafe owned ref", ref)
			return
		}
		probe := exec.CommandContext(ctx, "git", "-C", p.Repo, "ls-remote", "--exit-code", "origin", "refs/heads/"+ref)
		probe.Env = p.Driver.Environment(os.Environ())
		found, e := probe.CombinedOutput()
		if e != nil {
			if exit, ok := e.(*exec.ExitError); ok && exit.ExitCode() == 2 {
				continue
			}
			p.RemoteCleanupFailed = true
			s.t.Errorf("owned ref query: %v: %s", e, found)
			return
		}
		cmd := exec.CommandContext(ctx, "git", "-C", p.Repo, "push", "origin", "--delete", ref)
		cmd.Env = p.Driver.Environment(os.Environ())
		if output, err := cmd.CombinedOutput(); err != nil {
			p.RemoteCleanupFailed = true
			s.t.Errorf("owned remote ref cleanup: %v: %s", err, output)
			return
		}
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "remote-ref-cleanup.json"), map[string]any{"confirmed": true, "refs": refs}))
	p.RemoteCleanupFailed = false
}
