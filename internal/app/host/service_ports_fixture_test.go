package host

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	execution "github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type testProcessHost struct{}

func mustTestDirectory() (*execution.ExecutionDirectory, error) { return newNeutralDirectory() }

func testConfigWithDirectory(c execution.Config) execution.Config {
	if c.ExecutionDirectory == nil {
		c = withNeutralProfile(c, neutralSettings{Executable: os.Args[0]})
	}
	c.ProcessHost = testProcessHost{}
	c.ProcessFacts = resourceprocess.NewServiceFacts()
	if c.WorktreeRoot == "" && c.RunDir != "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	}
	if c.StateRoot == "" {
		c.StateRoot = fixtureStateRoot()
	}
	if c.RepositoryStateKey == "" && c.Identity() != "" {
		c.RepositoryStateKey = fixtureStateKeyFromIdentity(c.Identity())
	}
	if c.ValidateNativeStore == nil {
		c.ValidateNativeStore = func(ctx context.Context, config execution.Config) error {
			reader, err := openHostFixtureNativeReader(ctx, &config)
			if err != nil {
				return err
			}
			return reader.Close()
		}
	}
	if c.EnsureNativeProject == nil {
		c.EnsureNativeProject = func(ctx context.Context, config execution.Config) error {
			store, err := openHostFixtureNativeProject(ctx, &config)
			if err != nil {
				return err
			}
			return store.Close()
		}
	}
	if c.ReadPersistedExecutionState == nil {
		c.ReadPersistedExecutionState = func(ctx context.Context, config execution.Config) (storageports.ExecutionStateSnapshot, error) {
			reader, err := openHostFixtureNativeReader(ctx, &config)
			if err != nil {
				return storageports.ExecutionStateSnapshot{}, err
			}
			defer reader.Close()
			return reader.ReadExecutionStateSnapshot(ctx, config.Identity())
		}
	}
	return c
}

func newTestService(c execution.Config) (*execution.Service, error) {
	return newNeutralService(testConfigWithDirectory(c))
}

// Only the generic bounded raw output test uses this fixture.
type localExecutionBackend struct{ config execution.Config }

func (testProcessHost) StartManaged(command *exec.Cmd) (executionbackend.StopHandle, error) {
	return resourceprocess.StartManaged(command)
}

func (testProcessHost) InspectProcess(identity *executionbackend.ProcessIdentity) string {
	return resourceprocess.InspectProcess(identity)
}

func (testProcessHost) NewProcessGeneration() string { return newID() }

func (testProcessHost) WriteExecutionFile(path string, value []byte) error {
	return AtomicWrite(path, value)
}

func (testProcessHost) OutputCommand(ctx context.Context, command executionbackend.LaunchCommand) ([]byte, error) {
	return (execution.Runner{Dir: command.WorkingDirectory, Timeout: 30 * time.Second, ProcessHost: testProcessHost{}}).Output(ctx, executionbackend.Command{Executable: command.Executable, Args: command.Args})
}

func inspectTestWorkspace(ctx context.Context, c execution.Config, ref executionbackend.ExecutionRef) (executionbackend.WorkspaceInspection, error) {
	if ref.Incomplete || ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || !taskmodel.ValidCommit(ref.BaseCommit) {
		return executionbackend.WorkspaceInspection{}, errors.New("workspace ref is incomplete")
	}
	if c.Identity() == "" {
		if err := c.ResolveIdentity(ctx); err != nil {
			return executionbackend.WorkspaceInspection{}, err
		}
	}
	repository, err := workspace.ResolveRepository(ctx, c.WorkspaceGit(), c.Repository)
	if err != nil {
		return executionbackend.WorkspaceInspection{}, err
	}
	inspection, err := workspace.Inspect(ctx, c.WorkspaceGit(), repository, &executionbackend.WorkspaceIntent{Path: ref.WorkspacePath, Branch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit})
	if err != nil {
		return executionbackend.WorkspaceInspection{}, err
	}
	return executionbackend.WorkspaceInspection{Repository: inspection.Repository, Worktree: inspection.Worktree, Branch: inspection.Branch, Base: inspection.Base, Head: inspection.Head, Dirty: inspection.Dirty, CheckedAt: inspection.CheckedAt}, nil
}

func testHomePath() string {
	if value := strings.TrimSpace(os.Getenv("STEERLINE_HOME")); value != "" {
		return filepath.Clean(value)
	}
	return filepath.Join(os.TempDir(), "steerline-test")
}

func fixtureHome() string { return testHomePath() }

func fixtureBinDir() string { return filepath.Join(testHomePath(), "bin") }

func fixtureStateRoot() string { return filepath.Join(testHomePath(), "state") }

func fixtureGlobalExecutionConfigPath() string {
	return filepath.Join(testHomePath(), "execution.json")
}

func fixtureRepositoryStateKey(c execution.Config) (string, error) {
	if c.Identity() == "" {
		if err := c.ResolveIdentity(context.Background()); err != nil {
			return "", err
		}
	}
	return fixtureStateKeyFromIdentity(c.Identity()), nil
}

func fixtureStateKeyFromIdentity(identity string) string {
	common := identity
	if resolved, err := filepath.EvalSymlinks(common); err == nil {
		common = resolved
	}
	name := filepath.Base(common)
	if strings.EqualFold(name, ".git") {
		name = filepath.Base(filepath.Dir(common))
	}
	name = strings.Trim(regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(name, "-"), "-.")
	if name == "" {
		name = "repo"
	}
	canonicalCommon := filepath.Clean(common)
	if runtime.GOOS == "windows" {
		canonicalCommon = strings.ToLower(canonicalCommon)
	}
	return name + "-" + shortHash(canonicalCommon)[:12]
}

func validateNativeStateLocation(c execution.Config) error {
	if c.RepositoryStateKey == "" || c.StateRoot == "" {
		return nativeErr("authority_mismatch", "application configuration lacks its fixed repository state location")
	}
	if err := c.ResolveIdentity(context.Background()); err != nil {
		return err
	}
	if c.RepositoryStateKey != fixtureStateKeyFromIdentity(c.Identity()) {
		return nativeErr("authority_mismatch", "repository state key differs from the resolved repository identity")
	}
	expected := filepath.Join(c.StateRoot, c.RepositoryStateKey)
	if canonical(c.RunDir) != canonical(expected) {
		return nativeErr("authority_mismatch", "native tasks require the repository's stable shared state directory; remove the custom run_dir setting")
	}
	return nil
}

func loadFixtureConfig(path string) (execution.Config, error) {
	c := execution.Config{Remote: "origin", TaskPoll: "10s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "30s", ShutdownGrace: "10s", Git: executionbackend.Command{Executable: "git"}, ExecutableTypes: []string{"task", "bug", "feature", "chore"}, StallTimeout: "5m"}
	projectPath, err := filepath.Abs(path)
	if err != nil {
		return c, err
	}
	projectBytes, err := os.ReadFile(projectPath)
	if err != nil {
		return c, err
	}
	if err := decode(projectBytes, &c); err != nil {
		return c, err
	}
	if c.TaskAuthority != "native" {
		return c, errors.New("task_authority must be explicitly set to native; Beads is not supported")
	}
	if c.WorkflowProtocol != executionmodel.Version {
		return c, errors.New("workflow_protocol must be worker-flow-v1")
	}
	if err := c.ExecutionPolicy.Validate(); err != nil {
		return c, err
	}
	if err := c.ExecutionRequirements.Validate(); err != nil {
		return c, err
	}
	if err := publication.ValidateNonSecret(c.PR, c.Git); err != nil {
		return c, err
	}
	if c.Host != "" || c.Agent != "" || len(c.ExecutionProfiles) != 0 || c.DefaultExecutionProfile != "" {
		return c, errors.New("execution profiles belong in global execution config")
	}
	globalPath := fixtureGlobalExecutionConfigPath()
	globalBytes, err := os.ReadFile(globalPath)
	if err != nil {
		return c, err
	}
	var global struct {
		Version           int                          `json:"version"`
		ExecutionProfiles []execution.ExecutionProfile `json:"execution_profiles"`
		DefaultProfile    string                       `json:"default_execution_profile"`
	}
	if err := decode(globalBytes, &global); err != nil {
		return c, err
	}
	if global.Version != 2 || len(global.ExecutionProfiles) == 0 {
		return c, errors.New("global execution config version or profiles are invalid")
	}
	directory, err := mustTestDirectory()
	if err != nil {
		return c, err
	}
	c.ExecutionDirectory, c.ProcessHost, c.StateRoot = directory, testProcessHost{}, fixtureStateRoot()
	c.ExecutionProfiles, c.DefaultExecutionProfile = global.ExecutionProfiles, global.DefaultProfile
	c.BindApplicationConfig(directory, projectPath, globalPath, operationDigest(append(append([]byte(nil), projectBytes...), globalBytes...)), true)
	base := filepath.Dir(projectPath)
	if c.Repository == "" || c.Branch == "" {
		return c, errors.New("repository and branch are required")
	}
	if !filepath.IsAbs(c.Repository) {
		c.Repository = filepath.Join(base, c.Repository)
	}
	key, err := fixtureRepositoryStateKey(c)
	if err != nil {
		return c, err
	}
	c.RepositoryStateKey = key
	if c.RunDir == "" {
		c.RunDir = filepath.Join(c.StateRoot, key)
	} else if !filepath.IsAbs(c.RunDir) {
		c.RunDir = filepath.Join(base, c.RunDir)
	}
	if c.WorktreeRoot == "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	} else if !filepath.IsAbs(c.WorktreeRoot) {
		c.WorktreeRoot = filepath.Join(base, c.WorktreeRoot)
	}
	if err := c.ValidateProfiles(); err != nil {
		return c, err
	}
	initial := global.ExecutionProfiles[0]
	if global.DefaultProfile != "" {
		for _, profile := range global.ExecutionProfiles {
			if profile.ID == global.DefaultProfile {
				initial = profile
			}
		}
	}
	c = c.ApplyExecutionProfile(initial)
	if err := validateNativeStateLocation(c); err != nil {
		return c, err
	}
	if err := preflightNativeStatePath(c); err != nil {
		return c, err
	}
	return c, nil
}

func testStartService(ctx context.Context, configPath string, options StartServiceOptions) (ServiceStartResult, error) {
	config, err := loadFixtureConfig(configPath)
	if err != nil {
		return ServiceStartResult{}, err
	}
	if options.ApplicationHome == "" {
		options.ApplicationHome = fixtureHome()
	}
	if options.ApplicationBin == "" {
		options.ApplicationBin = fixtureBinDir()
	}
	return StartService(ctx, config, configPath, options)
}

func testRunConfiguredService(ctx context.Context, config execution.Config, mode, startupID, console string) error {
	return RunConfiguredService(ctx, testConfigWithDirectory(config), mode, startupID, console, newHostFixtureService, testHostAdmission{}, nil)
}

func testPrepareServiceConfig(c execution.Config) (execution.Config, error) {
	if err := c.ValidateAdmission(); err != nil {
		return c, err
	}
	if err := validateNativeStateLocation(c); err != nil {
		return c, err
	}
	if err := preflightNativeStatePath(c); err != nil {
		return c, err
	}
	if err := c.PrepareWorktreeRoot(); err != nil {
		return c, err
	}
	return c, nil
}

type testHostAdmission struct{}

func (testHostAdmission) ValidateStateLocation(c execution.Config) error {
	return validateNativeStateLocation(c)
}

func (testHostAdmission) Preflight(_ context.Context, c *execution.Config, _ bool) error {
	return preflightNativeStatePath(*c)
}

func (testHostAdmission) InitializeNativeProject(ctx context.Context, c execution.Config) error {
	if c.EnsureNativeProject == nil {
		return nil
	}
	return c.EnsureNativeProject(ctx, c)
}
