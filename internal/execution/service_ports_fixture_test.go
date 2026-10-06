package execution

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
)

type testProcessHost struct{}

func mustTestDirectory() (*ExecutionDirectory, error) { return newNeutralDirectory() }

func testConfigWithDirectory(c Config) Config {
	host := executionbackend.ManagedProcessHost(testProcessHost{})
	originalIdentity := c.identity
	if c.identity == "" && c.Repository != "" {
		if c.Git.Executable == "" {
			c.Git.Executable = "git"
		}
		repository, err := workspace.ResolveRepository(context.Background(), workspace.Git{Command: executionbackend.LaunchCommand{Executable: c.Git.Executable, Args: append([]string(nil), c.Git.Args...)}, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second), Start: host.StartManaged}, c.Repository)
		if err == nil {
			c.Repository = repository.Path
			c.BindRepositoryIdentity(canonical(repository.CommonDir))
		}
	}
	if originalIdentity != c.identity && c.selectedProfileID != "" {
		if _, ok := neutralFixtureCatalogs.Load(c.fixedProfileDigest); ok {
			_, catalog := neutralFixtureRuntime(c)
			catalog.Base = c
			if selected, _, err := catalog.ResolveExecutionProfile(c.selectedProfileID, ""); err == nil {
				rememberNeutralCatalog(selected, catalog)
				c = selected
			}
		}
	}
	if c.WorktreeRoot == "" && c.RunDir != "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	}
	if c.selectedProfileID == "" {
		c = withNeutralProfile(c, neutralSettings{Executable: os.Args[0]})
	}
	return c
}

func newTestService(c Config) (*Service, error) { return newNeutralService(testConfigWithDirectory(c)) }

// Only the generic bounded raw output test uses this fixture.
type localExecutionBackend struct{ config Config }

func (testProcessHost) StartManaged(command *exec.Cmd) (executionbackend.StopHandle, error) {
	return resourceprocess.StartManaged(command)
}

func (testProcessHost) InspectProcess(identity *executionbackend.ProcessIdentity) string {
	return resourceprocess.InspectProcess(identity)
}

func (testProcessHost) NewProcessGeneration() string { return newID() }

func (testProcessHost) WriteExecutionFile(path string, value []byte) error {
	return fixtureAtomicWrite(path, value)
}

func (testProcessHost) OutputCommand(ctx context.Context, command executionbackend.LaunchCommand) ([]byte, error) {
	return (Runner{Dir: command.WorkingDirectory, Timeout: 30 * time.Second, ProcessHost: testProcessHost{}}).Output(ctx, executionbackend.Command{Executable: command.Executable, Args: command.Args})
}

func inspectTestWorkspace(ctx context.Context, c Config, ref executionbackend.ExecutionRef) (executionbackend.WorkspaceInspection, error) {
	if ref.Incomplete || ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || !commitHash.MatchString(ref.BaseCommit) {
		return executionbackend.WorkspaceInspection{}, errors.New("workspace ref is incomplete")
	}
	if c.Identity() == "" {
		return executionbackend.WorkspaceInspection{}, errors.New("app-resolved repository identity is required")
	}
	deps, _ := neutralFixtureRuntime(c)
	repository, err := workspaceRepository(c, deps.Directory)
	if err != nil {
		return executionbackend.WorkspaceInspection{}, err
	}
	inspection, err := workspace.Inspect(ctx, workspaceGit(c, testProcessHost{}), repository, &executionbackend.WorkspaceIntent{Path: ref.WorkspacePath, Branch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit})
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

func fixtureRepositoryStateKey(c Config) (string, error) {
	repository, err := workspace.ResolveRepository(context.Background(), workspaceGit(testConfigWithDirectory(c), testProcessHost{}), c.Repository)
	if err != nil {
		return "", err
	}
	return fixtureStateKey(repository.CommonDir), nil
}

func loadFixtureConfig(path string) (Config, error) {
	type projectInput struct {
		PublicationPolicy     executionbackend.PublicationPolicy     `json:"publication_policy"`
		ExecutionPolicy       executionbackend.ExecutionPolicy       `json:"execution_policy"`
		ExecutionRequirements executionbackend.ExecutionRequirements `json:"execution_requirements,omitempty"`
		Repository            string                                 `json:"repository"`
		TaskAuthority         string                                 `json:"task_authority,omitempty"`
		Remote                string                                 `json:"remote"`
		Branch                string                                 `json:"branch"`
		RunDir                string                                 `json:"run_dir"`
		WorktreeRoot          string                                 `json:"worktree_root"`
		Git                   executionbackend.Command               `json:"git"`
		WorkflowProtocol      string                                 `json:"workflow_protocol,omitempty"`
		CommandTimeout        string                                 `json:"command_timeout"`
		Host                  string                                 `json:"host"`
		Agent                 string                                 `json:"agent"`
		ExecutionProfiles     []ExecutionProfile                     `json:"execution_profiles,omitempty"`
		DefaultProfile        string                                 `json:"default_execution_profile,omitempty"`
		PR                    executionmodel.PRProviderConfig        `json:"pr"`
	}
	c := Config{}
	projectPath, err := filepath.Abs(path)
	if err != nil {
		return c, err
	}
	projectBytes, err := os.ReadFile(projectPath)
	if err != nil {
		return c, err
	}
	var input projectInput
	if err := decode(projectBytes, &input); err != nil {
		return c, err
	}
	if input.TaskAuthority != "native" {
		return c, errors.New("task_authority must be explicitly set to native; Beads is not supported")
	}
	if input.Remote == "" {
		input.Remote = "origin"
	}
	if input.CommandTimeout == "" {
		input.CommandTimeout = "30s"
	}
	if input.Git.Executable == "" {
		input.Git.Executable = "git"
	}
	if input.WorkflowProtocol != executionmodel.Version {
		return c, errors.New("workflow_protocol must be worker-flow-v1")
	}
	if err := input.ExecutionPolicy.Validate(); err != nil {
		return c, err
	}
	if err := input.ExecutionRequirements.Validate(); err != nil {
		return c, err
	}
	if err := publication.ValidateNonSecret(publication.Config(input.PR), input.Git); err != nil {
		return c, err
	}
	if input.Host != "" || input.Agent != "" || len(input.ExecutionProfiles) != 0 || input.DefaultProfile != "" {
		return c, errors.New("execution profiles belong in global execution config")
	}
	globalPath := fixtureGlobalExecutionConfigPath()
	globalBytes, err := os.ReadFile(globalPath)
	if err != nil {
		return c, err
	}
	var global struct {
		Version           int                `json:"version"`
		ExecutionProfiles []ExecutionProfile `json:"execution_profiles"`
		DefaultProfile    string             `json:"default_execution_profile"`
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
	c = Config{PublicationPolicy: input.PublicationPolicy, ExecutionPolicy: input.ExecutionPolicy, ExecutionRequirements: input.ExecutionRequirements,
		Repository: input.Repository, Remote: input.Remote, Branch: input.Branch, RunDir: input.RunDir, WorktreeRoot: input.WorktreeRoot,
		Git: input.Git, WorkflowProtocol: input.WorkflowProtocol, CommandTimeout: input.CommandTimeout, PR: input.PR}
	c.BindAcceptedConfigVersion(digestValue([]string{string(projectBytes), string(globalBytes)}))
	base := filepath.Dir(projectPath)
	if c.Repository == "" || c.Branch == "" {
		return c, errors.New("repository and branch are required")
	}
	if !filepath.IsAbs(c.Repository) {
		c.Repository = filepath.Join(base, c.Repository)
	}
	repository, err := workspace.ResolveRepository(context.Background(), workspaceGit(c, testProcessHost{}), c.Repository)
	if err != nil {
		return c, err
	}
	c.Repository = repository.Path
	c.BindRepositoryIdentity(canonical(repository.CommonDir))
	key := fixtureStateKey(repository.CommonDir)
	stateRoot := fixtureStateRoot()
	if c.RunDir == "" {
		c.RunDir = filepath.Join(stateRoot, key)
	} else if !filepath.IsAbs(c.RunDir) {
		c.RunDir = filepath.Join(base, c.RunDir)
	}
	if c.WorktreeRoot == "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	} else if !filepath.IsAbs(c.WorktreeRoot) {
		c.WorktreeRoot = filepath.Join(base, c.WorktreeRoot)
	}
	catalog := ProfileCatalog{Base: c, Directory: directory, Profiles: global.ExecutionProfiles, DefaultID: global.DefaultProfile, Global: true, PolicySource: projectPath}
	if err := catalog.Validate(); err != nil {
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
	selected, _, err := catalog.ResolveExecutionProfile(initial.ID, "")
	if err != nil {
		return c, err
	}
	c = selected
	rememberNeutralCatalog(c, catalog)
	if err := c.ValidateAdmission(directory); err != nil {
		return c, err
	}
	return c, nil
}
