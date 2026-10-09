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
	"github.com/erranliu/SteerLine/internal/execution/hostport"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type testProcessHost struct{}

func mustTestDirectory() (*execution.ExecutionDirectory, error) { return newNeutralDirectory() }

func testConfigWithDirectory(c execution.Config) execution.Config {
	if _, ok := hostFixtureCatalogs.Load(c.FixedProfileDigest()); !ok {
		c = withNeutralProfile(c, neutralSettings{Executable: os.Args[0]})
	}
	if c.WorktreeRoot == "" && c.RunDir != "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	}
	_ = resolveHostTestIdentity(context.Background(), &c)
	return c
}

func resolveHostTestIdentity(ctx context.Context, c *execution.Config) error {
	if c == nil || c.Identity() != "" {
		return nil
	}
	processHost := testProcessHost{}
	command := c.Git
	if command.Executable == "" {
		command.Executable = "git"
	}
	repository, err := workspace.ResolveRepository(ctx, workspace.Git{Command: executionbackend.LaunchCommand{Executable: command.Executable, Args: append([]string(nil), command.Args...)}, Timeout: 30 * time.Second, Start: processHost.StartManaged}, c.Repository)
	if err != nil {
		return err
	}
	c.Repository = repository.Path
	c.BindRepositoryIdentity(canonical(repository.CommonDir))
	return nil
}

func hostConfigForTest(c execution.Config) Config {
	identity := c.Identity()
	if identity == "" {
		_ = resolveHostTestIdentity(context.Background(), &c)
		identity = c.Identity()
	}
	h := Config{
		Repository: c.Repository, RepositoryIdentity: identity, Remote: c.Remote, Branch: c.Branch, RunDir: c.RunDir,
		Claimant: "operator", ConfigDigest: c.AcceptedConfigVersion(), TaskAuthority: "native",
		WorkflowProtocol: c.WorkflowProtocol, TaskPoll: "10s", PRPoll: "30s", Heartbeat: "5s",
		CommandTimeout: c.CommandTimeout, ShutdownGrace: "10s", ProcessFacts: resourceprocess.NewServiceFacts(),
		ControlValidation: fixtureControlValidation{},
	}
	h.RuntimeReader = func(ctx context.Context, input Config) (*reconstruction.Runtime, error) {
		if err := input.RequireNativeTaskAuthority(); err != nil {
			return nil, err
		}
		resolved, err := executionConfigForHostTest(c, input)
		if err != nil {
			return nil, err
		}
		return readFixtureDatabaseRuntime(ctx, resolved)
	}
	h.ValidateNativeStore = func(ctx context.Context, input Config) error {
		resolved, err := executionConfigForHostTest(c, input)
		if err != nil {
			return err
		}
		reader, err := openHostFixtureNativeReader(ctx, &resolved)
		if err != nil {
			return err
		}
		return reader.Close()
	}
	h.EnsureRunDirOutsideRepo = func(_ context.Context, input Config) error {
		if input.Repository == "" || input.RunDir == "" || inside(input.Repository, input.RunDir) {
			return errors.New("run directory must be outside the repository")
		}
		return nil
	}
	return h
}

func executionConfigForHostTest(base execution.Config, input Config) (execution.Config, error) {
	base.Repository, base.Remote, base.Branch, base.RunDir = input.Repository, input.Remote, input.Branch, input.RunDir
	processHost := testProcessHost{}
	repository, err := workspace.ResolveRepository(context.Background(), workspace.Git{Command: executionbackend.LaunchCommand{Executable: base.Git.Executable, Args: append([]string(nil), base.Git.Args...)}, Timeout: 30 * time.Second, Start: processHost.StartManaged}, base.Repository)
	if err != nil {
		return base, err
	}
	base.Repository = repository.Path
	base.BindRepositoryIdentity(canonical(repository.CommonDir))
	if base.Identity() != input.Identity() {
		return base, errors.New("host test identity differs from execution fixture")
	}
	return base, nil
}

type fixtureControlValidation struct{}

func (fixtureControlValidation) ValidAction(value string) bool {
	return execution.ValidControlAction(value)
}
func (fixtureControlValidation) DecodePayload(value productapi.ControlRequest) (productapi.ControlPayload, error) {
	return execution.DecodeControlPayload(value)
}
func (fixtureControlValidation) ValidateRequest(value productapi.ControlRequest, instance, identity string, revision uint64, at time.Time) (string, error) {
	return execution.ValidateControlRequest(value, instance, identity, revision, at)
}
func (fixtureControlValidation) RequestDigest(value productapi.ControlRequest) string {
	return execution.RequestDigest(value)
}
func (fixtureControlValidation) TerminalStatus(value string) bool {
	return execution.TerminalControlStatus(value)
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
		if err := resolveHostTestIdentity(ctx, &c); err != nil {
			return executionbackend.WorkspaceInspection{}, err
		}
	}
	processHost := testProcessHost{}
	repository, err := workspace.ResolveRepository(ctx, c.WorkspaceGit(processHost), c.Repository)
	if err != nil {
		return executionbackend.WorkspaceInspection{}, err
	}
	inspection, err := workspace.Inspect(ctx, c.WorkspaceGit(processHost), repository, &executionbackend.WorkspaceIntent{Path: ref.WorkspacePath, Branch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit})
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
		if err := resolveHostTestIdentity(context.Background(), &c); err != nil {
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
	if err := resolveHostTestIdentity(context.Background(), &c); err != nil {
		return err
	}
	expected := filepath.Join(fixtureStateRoot(), fixtureStateKeyFromIdentity(c.Identity()))
	if canonical(c.RunDir) != canonical(expected) {
		return nativeErr("authority_mismatch", "native tasks require the repository's stable shared state directory; remove the custom run_dir setting")
	}
	return nil
}

func loadFixtureConfig(path string) (execution.Config, error) {
	type projectInput struct {
		PublicationPolicy executionbackend.PublicationPolicy `json:"publication_policy"`

		Repository              string                          `json:"repository"`
		TaskAuthority           string                          `json:"task_authority,omitempty"`
		Remote                  string                          `json:"remote"`
		Branch                  string                          `json:"branch"`
		RunDir                  string                          `json:"run_dir"`
		WorktreeRoot            string                          `json:"worktree_root"`
		Claimant                string                          `json:"claimant"`
		Git                     executionbackend.Command        `json:"git"`
		WorkflowProtocol        string                          `json:"workflow_protocol,omitempty"`
		ExecutableTypes         []string                        `json:"executable_types"`
		TaskPoll                string                          `json:"task_poll"`
		PRPoll                  string                          `json:"pr_poll"`
		Heartbeat               string                          `json:"heartbeat"`
		CommandTimeout          string                          `json:"command_timeout"`
		ShutdownGrace           string                          `json:"shutdown_grace"`
		Host                    string                          `json:"host"`
		Agent                   string                          `json:"agent"`
		ExecutionProfiles       []execution.ExecutionProfile    `json:"execution_profiles,omitempty"`
		DefaultExecutionProfile string                          `json:"default_execution_profile,omitempty"`
		StallTimeout            string                          `json:"stall_timeout"`
		PR                      executionmodel.PRProviderConfig `json:"pr"`
	}
	c := execution.Config{}
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
	if input.WorkflowProtocol != executionmodel.Version {
		return c, errors.New("workflow_protocol must be worker-flow-v2")
	}

	if err := publication.ValidateNonSecret(publication.Config(input.PR), input.Git); err != nil {
		return c, err
	}
	if input.Host != "" || input.Agent != "" || len(input.ExecutionProfiles) != 0 || input.DefaultExecutionProfile != "" {
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
	if input.Remote == "" {
		input.Remote = "origin"
	}
	if input.Git.Executable == "" {
		input.Git.Executable = "git"
	}
	if input.CommandTimeout == "" {
		input.CommandTimeout = "30s"
	}
	c = execution.Config{PublicationPolicy: input.PublicationPolicy,
		Repository: input.Repository, Remote: input.Remote, Branch: input.Branch, RunDir: input.RunDir, WorktreeRoot: input.WorktreeRoot,
		Git: input.Git, WorkflowProtocol: input.WorkflowProtocol, CommandTimeout: input.CommandTimeout, PR: input.PR}
	c.BindAcceptedConfigVersion(operationDigest(append(append([]byte(nil), projectBytes...), globalBytes...)))
	base := filepath.Dir(projectPath)
	if c.Repository == "" || c.Branch == "" {
		return c, errors.New("repository and branch are required")
	}
	if !filepath.IsAbs(c.Repository) {
		c.Repository = filepath.Join(base, c.Repository)
	}
	repository, err := workspace.ResolveRepository(context.Background(), c.WorkspaceGit(testProcessHost{}), c.Repository)
	if err != nil {
		return c, err
	}
	c.Repository = repository.Path
	c.BindRepositoryIdentity(canonical(repository.CommonDir))
	key := fixtureStateKeyFromIdentity(c.Identity())
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
	catalog := execution.ProfileCatalog{Base: c, Directory: directory, Profiles: global.ExecutionProfiles, DefaultID: global.DefaultProfile, Global: true}
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
	c, _, err = catalog.ResolveExecutionProfile(initial.ID, "")
	if err != nil {
		return c, err
	}
	rememberHostFixtureCatalog(c, catalog)
	if err := c.ValidateAdmission(directory); err != nil {
		return c, err
	}
	if err := validateNativeStateLocation(c); err != nil {
		return c, err
	}
	if err := PreflightNativeStatePath(hostConfigForTest(c)); err != nil {
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
	return StartService(ctx, hostConfigForTest(config), configPath, options)
}

func testRunConfiguredService(ctx context.Context, config execution.Config, mode, startupID, console string) error {
	config = testConfigWithDirectory(config)
	admission := testHostAdmission{config: config}
	factory := func(input Config, port hostport.HostPort, id string) (ServiceLifetime, error) {
		executionConfig, err := executionConfigForHostTest(config, input)
		if err != nil {
			return nil, err
		}
		service, err := newHostFixtureService(executionConfig, port)
		if err != nil {
			return nil, err
		}
		service.SetStartupMetadata(id)
		return fixtureServiceLifetime{service: service, config: input, mode: mode}, nil
	}
	return RunConfiguredService(ctx, hostConfigForTest(config), mode, startupID, console, factory, admission, nil)
}

func testPrepareServiceConfig(c execution.Config) (execution.Config, error) {
	dependencies := hostFixtureRuntime(c)
	if err := c.ValidateAdmission(dependencies.Directory); err != nil {
		return c, err
	}
	if err := validateNativeStateLocation(c); err != nil {
		return c, err
	}
	if err := PreflightNativeStatePath(hostConfigForTest(c)); err != nil {
		return c, err
	}
	if err := c.PrepareWorktreeRoot(); err != nil {
		return c, err
	}
	return c, nil
}

type testHostAdmission struct{ config execution.Config }

func (a testHostAdmission) ValidateStateLocation(c Config) error {
	if err := validateNativeStateLocation(a.config); err != nil {
		return err
	}
	return PreflightNativeStatePath(c)
}

func (a testHostAdmission) Preflight(_ context.Context, c Config, _ bool) (Config, error) {
	if err := PreflightNativeStatePath(hostConfigForTest(a.config)); err != nil {
		return c, err
	}
	return c, PreflightNativeStatePath(c)
}

func (a testHostAdmission) InitializeNativeProject(ctx context.Context, c Config) error {
	resolved, err := executionConfigForHostTest(a.config, c)
	if err != nil {
		return err
	}
	store, err := openHostFixtureNativeProject(ctx, &resolved)
	if err != nil {
		return err
	}
	return store.Close()
}

type fixtureServiceLifetime struct {
	service *execution.Service
	config  Config
	mode    string
}

func (l fixtureServiceLifetime) Run(ctx context.Context) error {
	return RunService(ctx, l.service, l.config, l.mode)
}
func (l fixtureServiceLifetime) Close() error { return l.service.Close() }

var _ ApplicationAdmission = testHostAdmission{}
