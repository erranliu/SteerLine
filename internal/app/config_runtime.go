package app

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/product/httpfs"
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

func (c Config) verifyCurrentConfig(ctx context.Context) error {
	if c.projectPath == "" || c.executionPath == "" {
		return nil
	}
	project, err := readConfiguration(c.projectPath)
	if err != nil {
		return err
	}
	raw, err := readConfiguration(c.executionPath)
	if err != nil {
		return taskError("stale_profile", err.Error())
	}
	startup := c.RunnerExecution.startupSettings()
	if value, _ := ctx.Value(profileContextKey{}).(profileContextValue); value.source != nil {
		if value.err != nil {
			return taskError("stale_profile", value.err.Error())
		}
		if value.snapshot == nil || value.snapshot.digest != executionconfig.Digest(raw) {
			return taskError("stale_profile", "执行方案在受理期间已变化，请刷新后重试")
		}
	} else {
		var global RunnerExecutionConfig
		if err := decodeStrictJSON(raw, &global); err != nil {
			return taskError("stale_profile", err.Error())
		}
		startup = global.startupSettings()
	}
	if loadedConfigDigest(project, startup) != c.configDigest {
		return &taskmodel.Error{Code: "stale_profile", Message: "project or execution configuration changed during claim evaluation"}
	}
	return nil
}

func (c Config) reserveCurrentProfiles(ctx context.Context, reserve func() error) error {
	if c.projectPath == "" || c.executionPath == "" {
		return reserve()
	}
	return httpfs.WithConfigurationLock(filepath.Dir(c.executionPath), func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.verifyCurrentConfig(ctx); err != nil {
			return err
		}
		return reserve()
	})
}
