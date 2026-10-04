package steerline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

func Preflight(ctx context.Context, c *execution.Config, checkSkills bool) error {
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return err
	}
	return preflightNative(ctx, c)
}

// preflightNative validates a native project without resolving or invoking a
// Beads executable. A check never initializes a database or adopts an existing
// state directory.
func preflightNative(ctx context.Context, c *execution.Config) error {
	if err := c.ValidateTaskCommandConfiguration(); err != nil {
		return err
	}
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return err
	}
	if err := c.ValidateProfiles(); err != nil {
		return err
	}
	entry, err := c.Registration()
	if err != nil {
		return err
	}
	if entry.ValidateConfig != nil {
		if err := entry.ValidateConfig(ctx, *c); err != nil {
			return err
		}
	}
	commands := []*executionbackend.Command{&c.Git}
	for _, command := range commands {
		if err := execution.ResolveCommand(command); err != nil {
			return err
		}
	}
	git := execution.Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout), ProcessHost: c.ProcessHost}
	repository, err := workspace.ResolveRepository(ctx, c.WorkspaceGit(), c.Repository)
	if err != nil {
		return err
	}
	c.Repository = repository.Path

	if err := c.ResolveIdentity(ctx); err != nil {
		return err
	}
	git.Dir = c.Repository
	if entry.Preflight != nil {
		if err := entry.Preflight(ctx, *c); err != nil {
			return err
		}
	}
	if c.Claimant == "" {
		c.Claimant = "steerline-" + shortHash(c.Identity())
	}
	if _, err = git.Output(ctx, c.Git, "check-ref-format", "refs/heads/"+c.Branch); err != nil {
		return err
	}
	remote, err := git.Output(ctx, c.Git, "remote", "get-url", c.Remote)
	if err != nil {
		return err
	}
	if err = c.PR.ValidateRemote(strings.TrimSpace(string(remote))); err != nil {
		return err
	}
	if c.PR.Kind == "command" {
		if err = execution.ResolveCommand(&c.PR.Query); err != nil {
			return err
		}
		if c.PR.Close.Executable == "" {
			return errors.New("native command PR provider requires a close command for discarded task cleanup")
		}
		if err = execution.ResolveCommand(&c.PR.Close); err != nil {
			return err
		}
	}
	if err = ensureRunDirOutsideRepository(*c); err != nil {
		return err
	}
	if _, err = os.Stat(filepath.Join(c.RunDir, "steerline.db")); err == nil {
		reader, openErr := openBoundNativeStore(ctx, *c, filepath.Join(c.RunDir, "steerline.db"))
		if openErr != nil {
			return openErr
		}
		if closeErr := reader.Close(); closeErr != nil {
			return closeErr
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if err = ensureNativeStateClean(c.RunDir, c.Repository); err != nil {
		return err
	}
	if c.PR.Kind == "gitea" {
		if err = (publication.NewClient(c.PR, git, git.Timeout)).Check(ctx); err != nil {
			return err
		}
	}
	return nil
}
