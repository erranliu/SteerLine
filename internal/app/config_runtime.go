package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
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

func (c Config) WorkspaceGit() workspace.Git { return c.executionBase().WorkspaceGit(c.ProcessHost) }

func (c Config) ResolveCommand(command *executionbackend.Command) error {
	return execution.ResolveCommand(command)
}

func (c Config) TaskRepositoryTarget(ctx context.Context) (string, error) {
	return c.executionConfig().TaskRepositoryTarget(ctx, c.ProcessHost)
}

func (c *Config) ResolveIdentity(ctx context.Context) error {
	if c == nil {
		return errors.New("application configuration is required")
	}
	if err := execution.ResolveCommand(&c.Git); err != nil {
		return err
	}
	if c.PR.Kind == "command" {
		for _, command := range []*executionbackend.Command{&c.PR.Query, &c.PR.Close} {
			if command.Executable != "" {
				_ = execution.ResolveCommand(command)
			}
		}
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
	if c.activeProfile != nil {
		updated, profile, resolveErr := c.profileCatalog().SelectExecutionProfile(c.activeProfile.ID)
		if resolveErr != nil {
			return resolveErr
		}
		*c = c.withExecutionConfig(updated)
		c.activeProfile = &profile
	}
	return nil
}

func canonicalConfigPath(value string) string {
	return canonicalPath(value)
}

func (c Config) ValidateProfiles() error { return c.profileCatalog().Validate() }

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

func (c *Config) PrepareWorktreeRoot() error {
	if c == nil {
		return errors.New("application configuration is required")
	}
	if c.WorktreeRoot == "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	}
	return nil
}

func (c Config) Registration() (execution.ExecutionRegistration, error) {
	return c.executionConfig().Registration(c.ExecutionDirectory)
}

func (c Config) ValidateStoredExecutions(snapshot reconstruction.Snapshot) error {
	return c.executionConfig().ValidateStoredExecutions(snapshot, c.ExecutionDirectory)
}

func (c Config) EvaluateRequirements(ctx context.Context) (executionbackend.RequirementsEvaluation, error) {
	return c.executionConfig().EvaluateRequirements(ctx, c.ExecutionDirectory)
}

func (c Config) DecodeImplementationConfig(key string) ([]byte, error) {
	return c.executionConfig().DecodeImplementationConfig(key, c.ExecutionDirectory)
}

func (c Config) CheckFixedProfile(profile executionmodel.FixedExecutionProfile) error {
	return c.profileCatalog().CheckFixedProfile(profile)
}

func (c Config) ApplyFixedExecutionProfile(profile executionmodel.FixedExecutionProfile) Config {
	config := c.executionConfig().ApplyFixedExecutionProfile(profile, c.ExecutionDirectory)
	c = c.withExecutionConfig(config)
	for _, available := range c.AvailableExecutionProfiles() {
		if available.ID == profile.ID {
			c.activeProfile = &available
			break
		}
	}
	return c
}

func (c Config) AvailableExecutionProfiles() []execution.ExecutionProfile {
	return c.profileCatalog().AvailableExecutionProfiles()
}

func (c Config) DefaultProfileID() string { return c.profileCatalog().DefaultProfileID() }

func (c Config) ResolveExecutionProfile(id, digest string) (execution.Config, execution.ExecutionProfile, error) {
	return c.profileCatalog().ResolveExecutionProfile(id, digest)
}

func (c Config) ApplyExecutionProfile(profile execution.ExecutionProfile) Config {
	found := false
	for _, configured := range c.ExecutionProfiles {
		found = found || configured.ID == profile.ID
	}
	if !found {
		c.ExecutionProfiles = append(append([]execution.ExecutionProfile(nil), c.ExecutionProfiles...), profile)
	}
	catalog := c.profileCatalog()
	config, selected, err := catalog.SelectExecutionProfile(profile.ID)
	if err != nil {
		return c
	}
	c = c.withExecutionConfig(config)
	c.activeProfile = &selected
	return c
}

func (c Config) withExecutionConfig(config execution.Config) Config {
	c.Repository, c.Remote, c.Branch, c.RunDir, c.WorktreeRoot = config.Repository, config.Remote, config.Branch, config.RunDir, config.WorktreeRoot
	c.Git, c.WorkflowProtocol, c.CommandTimeout = config.Git, config.WorkflowProtocol, config.CommandTimeout
	c.PublicationPolicy, c.ExecutionPolicy, c.ExecutionRequirements = config.PublicationPolicy, config.ExecutionPolicy, config.ExecutionRequirements
	c.PR, c.Host, c.Agent = executionmodel.PRProviderConfig(config.PR), config.Host, config.Agent
	return c
}

func (c Config) executionBase() execution.Config {
	config := execution.Config{
		PublicationPolicy: c.PublicationPolicy, ExecutionPolicy: c.ExecutionPolicy, ExecutionRequirements: c.ExecutionRequirements,
		Repository: c.Repository, Remote: c.Remote, Branch: c.Branch, RunDir: c.RunDir, WorktreeRoot: c.WorktreeRoot,
		Git: c.Git, WorkflowProtocol: c.WorkflowProtocol, CommandTimeout: c.CommandTimeout, PR: executionmodel.PRProviderConfig(c.PR),
		Host: c.Host, Agent: c.Agent,
	}
	config.BindRepositoryIdentity(c.identity)
	config.BindAcceptedConfigVersion(c.configDigest)
	return config
}

func (c Config) executionConfig() execution.Config {
	base := c.executionBase()
	if c.activeProfile == nil {
		return base
	}
	selected, _, err := c.profileCatalog().SelectExecutionProfile(c.activeProfile.ID)
	if err != nil {
		return base
	}
	return selected
}

// ExecutionConfig returns the single selected frozen execution input for
// callers that need the lower-level read-only execution APIs.
func (c Config) ExecutionConfig() execution.Config { return c.executionConfig() }

func (c Config) profileCatalog() execution.ProfileCatalog {
	return execution.ProfileCatalog{Base: c.executionBase(), Directory: c.ExecutionDirectory, Profiles: append([]execution.ExecutionProfile(nil), c.ExecutionProfiles...), DefaultID: c.DefaultExecutionProfile, Global: c.globalExecution, PolicySource: c.projectPath}
}

func (c Config) AcceptedConfigVersion() string {
	if c.activeProfile != nil && c.activeProfile.AcceptedConfigVersion != "" {
		return c.activeProfile.AcceptedConfigVersion
	}
	return c.configDigest
}

func (c Config) runtimeDependencies() execution.RuntimeDependencies {
	return execution.RuntimeDependencies{
		Directory: c.ExecutionDirectory, ProcessHost: c.ProcessHost,
		Profiles: appProfilePort{config: c}, ConfigVerifier: appConfigVerifier{project: c.projectPath, global: c.executionPath, digest: c.configDigest},
	}
}

func (c Config) runtimeSettings() execution.RuntimeSettings {
	return execution.RuntimeSettings{Claimant: c.Claimant, Heartbeat: positiveDuration(c.Heartbeat, 5*time.Second), ShutdownGrace: positiveDuration(c.ShutdownGrace, 10*time.Second)}
}

type appProfilePort struct{ config Config }

func (p appProfilePort) DefaultProfileID() string { return p.config.DefaultProfileID() }
func (p appProfilePort) HasMultipleProfiles() bool {
	return len(p.config.AvailableExecutionProfiles()) > 1
}
func (p appProfilePort) ResolveExecutionProfile(id, digest string) (execution.Config, executionmodel.FixedExecutionProfile, error) {
	config, profile, err := p.config.ResolveExecutionProfile(id, digest)
	if err != nil {
		return execution.Config{}, executionmodel.FixedExecutionProfile{}, err
	}
	fixed := profile.Freeze(p.config.ExecutionDirectory, config.Identity())
	if fixed.Digest == "" {
		return execution.Config{}, executionmodel.FixedExecutionProfile{}, errors.New("selected execution profile could not be frozen")
	}
	return config, fixed, nil
}

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
