package app

import (
	"context"
	"errors"
	"os"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (c *Config) bindApplicationFiles(projectPath, executionPath, digest string, global bool) {
	c.projectPath, c.executionPath, c.configDigest, c.globalExecution = projectPath, executionPath, digest, global
}

func (c Config) Identity() string           { return c.identity }
func (c Config) LoadedConfigDigest() string { return c.configDigest }

func (c Config) RequireNativeTaskAuthority() error {
	if c.TaskAuthority != "native" {
		return errors.New("task_authority must be explicitly set to native; Beads is not supported")
	}
	return nil
}

func (c Config) ValidateTaskCommandConfiguration() error {
	if c.Repository == "" || c.Remote == "" || c.Branch == "" {
		return errors.New("repository, remote and branch are required")
	}
	return nil
}

func (c Config) WorkspaceGit() workspace.Git {
	return workspace.Git{Command: executionbackend.LaunchCommand{Executable: c.Git.Executable, Args: append([]string(nil), c.Git.Args...)}, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second), Start: c.ProcessHost.StartManaged}
}

func (c *Config) ResolveIdentity(ctx context.Context) error {
	if c == nil {
		return errors.New("application configuration is required")
	}
	ctx = c.queryContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := resolveCommand(&c.Git); err != nil {
		return err
	}
	if c.PR.Kind == "command" {
		for _, command := range []*executionbackend.Command{&c.PR.Query} {
			if command.Executable != "" {
				_ = resolveCommand(command)
			}
		}
	}
	key := c.identityQueryKey()
	if q := c.startup; q != nil && !q.closed && q.identityKey == key && (c.identity == "" || c.identity == q.identity) {
		c.Repository, c.identity = q.repository, q.identity
		return nil
	}
	repository, err := workspace.ResolveRepository(ctx, c.WorkspaceGit(), c.Repository)
	if err != nil {
		return err
	}
	identity := canonicalConfigPath(repository.CommonDir)
	if c.identity != "" && canonicalConfigPath(c.identity) != identity {
		return errors.New("configured repository identity changed after it was resolved")
	}
	c.identity, c.Repository = identity, repository.Path
	if q := c.startup; q != nil && !q.closed {
		q.identityKey, q.repository, q.identity = c.identityQueryKey(), c.Repository, identity
	}

	return nil
}

func canonicalConfigPath(value string) string {
	return canonicalPath(value)
}

func (c Config) ValidateProfiles() error { return c.RunnerExecution.Validate() }

func (c Config) ValidateAdmission() error {
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return err
	}
	if err := c.ValidateTaskCommandConfiguration(); err != nil {
		return err
	}
	if err := c.ValidateProfiles(); err != nil {
		return err
	}
	if err := c.ResolveIdentity(context.Background()); err != nil {
		return err
	}
	if err := ValidateApplicationPaths(c); err != nil {
		return err
	}
	return nil
}

func (c Config) DefaultProfileID() string { return c.RunnerExecution.DefaultProfile }

type appConfigVerifier struct{ project, global, digest string }

func (v appConfigVerifier) VerifyCurrentConfig() error {
	if v.project == "" || v.global == "" {
		return nil
	}
	project, err := os.ReadFile(v.project)
	if err != nil {
		return err
	}
	global, err := os.ReadFile(v.global)
	if err != nil {
		return err
	}
	if loadedConfigDigest(project, global) != v.digest {
		return &taskmodel.Error{Code: "stale_profile", Message: "project or execution configuration changed during claim evaluation"}
	}
	return nil
}
