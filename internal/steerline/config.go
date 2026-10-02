package steerline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

type Skill struct {
	Path       string `json:"path"`
	Invocation string `json:"invocation"`
}
type Config struct {
	fixedProfileDigest      string
	requirementsEvaluation  *executionapi.RequirementsEvaluation
	executionConfigPath     string
	publicationRemoteURL    string
	selectedProfileID       string
	PublicationPolicy       executionapi.PublicationPolicy     `json:"publication_policy"`
	ExecutionPolicy         executionapi.ExecutionPolicy       `json:"execution_policy"`
	ExecutionRequirements   executionapi.ExecutionRequirements `json:"execution_requirements,omitempty"`
	policySource            string
	Repository              string                                    `json:"repository"`
	TaskAuthority           string                                    `json:"task_authority,omitempty"`
	Remote                  string                                    `json:"remote"`
	Branch                  string                                    `json:"branch"`
	RunDir                  string                                    `json:"run_dir"`
	WorktreeRoot            string                                    `json:"worktree_root"`
	Claimant                string                                    `json:"claimant"`
	Git                     Command                                   `json:"git"`
	ImplementationData      []executionapi.ImplementationDataEnvelope `json:"-"`
	ImplementationDisplay   []string                                  `json:"-"`
	WorkflowProtocol        string                                    `json:"workflow_protocol,omitempty"`
	ExecutableTypes         []string                                  `json:"executable_types"`
	TaskPoll                string                                    `json:"task_poll"`
	PRPoll                  string                                    `json:"pr_poll"`
	Heartbeat               string                                    `json:"heartbeat"`
	CommandTimeout          string                                    `json:"command_timeout"`
	ShutdownGrace           string                                    `json:"shutdown_grace"`
	Agent                   string                                    `json:"agent"`
	Host                    string                                    `json:"host"`
	ExecutionProfiles       []ExecutionProfile                        `json:"execution_profiles,omitempty"`
	DefaultExecutionProfile string                                    `json:"default_execution_profile,omitempty"`
	StallTimeout            string                                    `json:"stall_timeout"`
	PR                      publication.Config                        `json:"pr"`
	identity                string
	workerEnv               map[string]string
	configDigest            string
	globalExecution         bool
	profilePublication      bool
}

// LoadedConfigDigest identifies the project and global execution file bytes
// used to build this value. A console compares it with a running service before
// presenting execution settings as current.
func (c Config) LoadedConfigDigest() string { return c.configDigest }

// LegacyCompletionConfig is decoded only as a marker for rejecting old runtime
// projections; it is never accepted in Config or executed.

// trimBOM removes a leading UTF-8 byte-order mark. Windows tooling (notably
// PowerShell) prepends one when writing JSON, and encoding/json rejects it.
func trimBOM(b []byte) []byte         { return contracts.TrimBOM(b) }
func decode(data []byte, v any) error { return contracts.DecodeJSON(data, v) }

// SteerlineHome is the machine-level SteerLine home shared by every source
// worktree. STEERLINE_HOME overrides the default (C:\SteerlineRuntime on
// Windows). Binaries, configs and per-repo state live under it, so "what is this
// machine running" has one answer regardless of which checkout built it.
func SteerlineHome() string {
	if v := strings.TrimSpace(os.Getenv("STEERLINE_HOME")); v != "" {
		return filepath.Clean(v)
	}
	if runtime.GOOS == "windows" {
		return `C:\SteerlineRuntime`
	}
	if home, e := os.UserHomeDir(); e == nil {
		return filepath.Join(home, ".steerline")
	}
	return filepath.Join(os.TempDir(), "steerline")
}

func SteerlineBinDir() string           { return filepath.Join(SteerlineHome(), "bin") }
func SteerlineEtcDir() string           { return filepath.Join(SteerlineHome(), "etc") }
func GlobalExecutionConfigPath() string { return filepath.Join(SteerlineHome(), "execution.json") }
func SteerlineStateRoot() string        { return filepath.Join(SteerlineHome(), "state") }

// repoStateKey derives a worktree-independent key for a repository from its Git
// common dir, so every worktree and clone of the same repository maps to one
// state directory.
func repoStateKey(c Config) (string, error) {
	repository, e := workspace.ResolveRepository(context.Background(), workspaceGit(c), c.Repository)
	if e != nil {
		return "", e
	}
	common := repository.CommonDir
	if common == "" {
		return "", errors.New("git returned an empty common dir")
	}
	if resolved, e := filepath.EvalSymlinks(common); e == nil {
		common = resolved
	}
	name := filepath.Base(common)
	if strings.EqualFold(name, ".git") {
		name = filepath.Base(filepath.Dir(common))
	}
	name = sanitizeName(name)
	if name == "" {
		name = "repo"
	}
	return name + "-" + shortHash(canonical(common))[:12], nil
}

func sanitizeName(s string) string {
	return strings.Trim(regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(s, "-"), "-.")
}

// InstallExecutable copies a built binary into <STEERLINE_HOME>\bin so the
// service and console always run one stable binary regardless of which source
// worktree built it.
func InstallExecutable(src string) (string, error) {
	if resolved, e := filepath.EvalSymlinks(src); e == nil {
		src = resolved
	}
	info, e := os.Stat(src)
	if e != nil {
		return "", e
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", src)
	}
	name := "steerline"
	if runtime.GOOS == "windows" {
		name = "steerline.exe"
	}
	dir := SteerlineBinDir()
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	dst := filepath.Join(dir, name)
	if canonical(src) == canonical(dst) {
		return dst, nil
	}
	b, e := os.ReadFile(src)
	if e != nil {
		return "", e
	}
	if e := AtomicWrite(dst, b); e != nil {
		return "", fmt.Errorf("install %s: %w (stop a running service using it first)", dst, e)
	}
	return dst, nil
}

// InstallSelf installs the currently running executable.
func InstallSelf() (string, error) {
	exe, e := os.Executable()
	if e != nil {
		return "", e
	}
	return InstallExecutable(exe)
}

func LoadConfig(path string) (Config, error) {
	c := Config{Remote: "origin", TaskPoll: "10s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "30s", ShutdownGrace: "10s", Git: Command{Executable: "git"}, ExecutableTypes: []string{"task", "bug", "feature", "chore"}, StallTimeout: "5m"}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err = decode(b, &c); err != nil {
		return c, err
	}
	if c.TaskAuthority != "native" {
		return c, errors.New("task_authority must be explicitly set to native; Beads is not supported")
	}
	if c.WorkflowProtocol != WorkerFlowVersion {
		return c, errors.New("workflow_protocol must be worker-flow-v1; old workflow configurations are unsupported and their external state is preserved")
	}
	if err := c.ExecutionPolicy.Validate(); err != nil {
		return c, err
	}
	if err := c.ExecutionRequirements.Validate(); err != nil {
		return c, err
	}
	if err := validateProjectPublicationPolicy(c.PublicationPolicy); err != nil {
		return c, err
	}
	if err := publication.ValidateNonSecret(c.PR, c.Git); err != nil {
		return c, err
	}
	c.policySource, _ = filepath.Abs(path)
	c.executionConfigPath = GlobalExecutionConfigPath()
	if c.Host != "" || c.Agent != "" || len(c.ExecutionProfiles) != 0 || c.DefaultExecutionProfile != "" {
		return c, fmt.Errorf("execution profiles belong in global execution config %s", GlobalExecutionConfigPath())
	}
	globalBytes, err := os.ReadFile(GlobalExecutionConfigPath())
	if err != nil {
		return c, fmt.Errorf("load global execution config %s: %w", GlobalExecutionConfigPath(), err)
	}
	var global struct {
		Version           int                `json:"version"`
		ExecutionProfiles []ExecutionProfile `json:"execution_profiles"`
		DefaultProfile    string             `json:"default_execution_profile"`
	}
	if err := decode(globalBytes, &global); err != nil {
		return c, fmt.Errorf("global execution config: %w", err)
	}
	if global.Version != 2 {
		return c, errors.New("global execution config version must be 2; older development formats are unsupported and their state must be preserved")
	}
	if len(global.ExecutionProfiles) == 0 {
		return c, errors.New("global execution config requires at least one execution profile")
	}
	c.ExecutionProfiles = global.ExecutionProfiles
	c.DefaultExecutionProfile = global.DefaultProfile
	c.globalExecution = true
	c.configDigest = loadedConfigDigest(b, globalBytes)
	if err := validateExecutionProfiles(c); err != nil {
		return c, err
	}
	initial := global.ExecutionProfiles[0].effective()
	if global.DefaultProfile != "" {
		for _, profile := range global.ExecutionProfiles {
			if profile.ID == global.DefaultProfile {
				initial = profile.effective()
				break
			}
		}
	}
	c = c.ApplyExecutionProfile(initial)
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return c, err
	}
	if c.Repository == "" || c.Branch == "" {
		return c, errors.New("repository and branch are required")
	}
	if !filepath.IsAbs(c.Repository) {
		c.Repository = filepath.Join(base, c.Repository)
	}
	if c.RunDir == "" {
		// Derive a worktree-independent state dir under STEERLINE_HOME.
		key, e := repoStateKey(c)
		if e != nil {
			return c, fmt.Errorf("run_dir is required (cannot derive from repository identity: %w)", e)
		}
		c.RunDir = filepath.Join(SteerlineStateRoot(), key)
	} else if !filepath.IsAbs(c.RunDir) {
		c.RunDir = filepath.Join(base, c.RunDir)
	}
	err = preflightNativeStatePath(c)
	if err != nil {
		return c, err
	}
	if c.WorktreeRoot == "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	}
	if !filepath.IsAbs(c.WorktreeRoot) {
		c.WorktreeRoot = filepath.Join(base, c.WorktreeRoot)
	}
	if err := validateNativeStateLocation(c); err != nil {
		return c, err
	}
	for _, v := range []string{c.TaskPoll, c.PRPoll, c.Heartbeat, c.CommandTimeout, c.ShutdownGrace} {
		d, e := time.ParseDuration(v)
		if e != nil || d <= 0 {
			return c, fmt.Errorf("invalid positive duration %q", v)
		}
	}

	// Empty means "use the default" so programmatic Configs stay valid.
	if c.Host != "" && c.Host != "local" && c.Host != "orca" && c.Host != "orca_supervised" {
		return c, errors.New("host must be local, orca, or orca_supervised")
	}
	if c.StallTimeout != "" {
		if d, e := time.ParseDuration(c.StallTimeout); e != nil || d < 0 {
			return c, errors.New("stall_timeout must be zero or a positive duration")
		}
	}
	if len(c.ExecutableTypes) == 0 {
		return c, errors.New("executable_types is empty")
	}
	for _, s := range c.ExecutableTypes {
		if !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(s) || s == "epic" || s == "molecule" || s == "convoy" {
			return c, fmt.Errorf("invalid executable type %q", s)
		}
	}
	if err := c.ExecutionPolicy.Validate(); err != nil {
		return c, err
	}
	if err = validateExecutionProfiles(c); err != nil {
		return c, err
	}
	return c, nil
}

func validateWorkflowConfig(c *Config, _ []byte) error {
	if _, err := registrationFor(*c); err != nil {
		return err
	}
	if err := c.ExecutionRequirements.Validate(); err != nil {
		return err
	}
	if err := publication.CheckPolicy(c.PublicationPolicy, c.hostName(), c.agentName(), c.PR); err != nil {
		return err
	}
	if err := publication.ValidateNonSecret(c.PR, c.Git); err != nil {
		return err
	}
	if c.WorkflowProtocol != WorkerFlowVersion {
		return errors.New("only worker-flow-v1 is supported")
	}
	return validateImplementationProfileData(*c)
}

func duration(s string) time.Duration { d, _ := time.ParseDuration(s); return d }
func (c Config) Identity() string     { return c.identity }

// ResolveIdentity reads only Git metadata and does not run task-provider probes
// or create runtime directories. Control clients use it without constructing a
// Service or taking ownership of scheduler state.
func (c *Config) ResolveIdentity(ctx context.Context) error {
	// Readers and the service must identify the same native Git executable
	// before they compute a selectable execution profile's digest.
	if err := resolveCommand(&c.Git); err != nil {
		return err
	}
	repository, err := workspace.ResolveRepository(ctx, workspaceGit(*c), c.Repository)
	if err != nil {
		return err
	}
	identity := canonical(repository.CommonDir)
	if c.identity != "" && canonical(c.identity) != identity {
		return errors.New("configured repository identity changed after it was resolved")
	}
	c.identity = identity
	if c.selectedProfileID != "" {
		var selected ExecutionProfile
		found := false
		if c.globalExecution {
			for _, profile := range c.ExecutionProfiles {
				if profile.ID == c.selectedProfileID {
					selected, found = profile, true
					break
				}
			}
		} else if c.selectedProfileID == ProjectExecutionProfileID {
			selected, found = c.projectExecutionProfile(), true
		}
		if !found {
			return errors.New("selected execution profile is not available after project identity resolution")
		}
		*c = c.ApplyExecutionProfile(c.bindProjectPolicy(selected))
	}
	return nil
}

// hostName and agentName apply the documented defaults for
// programmatic Configs that do not go through LoadConfig.
func (c Config) hostName() string {
	if c.Host == "" {
		return "local"
	}
	return c.Host
}
func (c Config) agentName() string {
	if c.Agent == "" {
		return "codex"
	}
	return c.Agent
}
func (c Config) needsCodexExecutable() bool {
	settings := codexProfileSettings(c)
	return c.requiresLocalCodex() || c.managedWindowsOrca() || c.hostName() == "orca" && c.agentName() == "codex" && (settings.Model != "" || settings.ReasoningEffort != "" || settings.Profile != "")
}
func resolveCommand(c *Command) error {
	p, e := exec.LookPath(c.Executable)
	if e != nil {
		return e
	}
	ext := strings.ToLower(filepath.Ext(p))
	if ext == ".cmd" || ext == ".bat" || ext == ".ps1" {
		return fmt.Errorf("use a native executable (or node.exe with a script argument), not a shell shim: %s", p)
	}
	c.Executable = p
	return nil
}
func (c *Config) Preflight(ctx context.Context, checkSkills bool) error {
	if err := requireNativeTaskAuthority(*c); err != nil {
		return err
	}
	return c.preflightNative(ctx)
}

// preflightNative validates a native project without resolving or invoking a
// Beads executable. A check never initializes a database or adopts an existing
// state directory.
func (c *Config) preflightNative(ctx context.Context) error {
	if err := validateWorkflowConfig(c, nil); err != nil {
		return err
	}
	if err := requireNativeTaskAuthority(*c); err != nil {
		return err
	}
	if err := validateExecutionProfiles(*c); err != nil {
		return err
	}
	if c.WorkflowProtocol == WorkerFlowVersion && c.hostName() != "local" && c.hostName() != "orca" && c.hostName() != "orca_supervised" {
		return &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "this worker-flow host is not supported"}
	}
	codex := codexProfileSettings(*c)
	orca := orcaProfileSettings(*c)
	if c.WorkflowProtocol == WorkerFlowVersion && c.hostName() == "orca" && (codex.Profile != "" || (c.ExecutionPolicy.Filesystem.WriteScope != "workspace" && c.ExecutionPolicy.Filesystem.WriteScope != "host")) {
		return &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "Orca requires a supported project policy and no Codex profile"}
	}
	if c.hostName() == "local" && c.agentName() != "codex" {
		return errors.New("local_process currently supports only agent=codex")
	}
	commands := []*Command{&c.Git}
	for _, command := range commands {
		if err := resolveCommand(command); err != nil {
			return err
		}
	}
	if c.needsCodexExecutable() {
		if _, err := exec.LookPath(codex.Executable); err != nil {
			return err
		}
	}
	git := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	if c.hostName() == "orca" {
		if _, err := (Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}).Output(ctx, Command{Executable: orca.Executable, Args: orca.Args}, "status", "--json"); err != nil {
			return fmt.Errorf("orca host is not reachable: %w", err)
		}
	}
	repository, err := workspace.ResolveRepository(ctx, workspaceGit(*c), c.Repository)
	if err != nil {
		return err
	}
	c.Repository, c.identity = repository.Path, canonical(repository.CommonDir)
	git.Dir = c.Repository
	if c.hostName() == "orca" {
		if err = validateOrcaRepositoryRegistration(ctx, *c); err != nil {
			return err
		}
	}
	if c.Claimant == "" {
		c.Claimant = "steerline-" + shortHash(c.identity)
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
	if c.hostName() == "orca_supervised" {
		if err = validateImplementationProfileData(*c); err != nil {
			return err
		}
		if _, err = probeSupervisedOrcaTarget(ctx, *c); err != nil {
			return fmt.Errorf("supervised Orca target preflight: %w", err)
		}
	}
	if c.PR.Kind == "command" {
		if err = resolveCommand(&c.PR.Query); err != nil {
			return err
		}
		if c.PR.Close.Executable == "" {
			return errors.New("native command PR provider requires a close command for discarded task cleanup")
		}
		if err = resolveCommand(&c.PR.Close); err != nil {
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

func requireNativeTaskAuthority(c Config) error {
	if c.TaskAuthority != "native" {
		return errors.New("task_authority=native is required; Beads is not an active product task source")
	}
	return nil
}

func (c Config) requiresLocalCodex() bool {
	return c.hostName() == "local"
}

func inside(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

type Command = contracts.Command

type LegacyCompletionConfig = contracts.LegacyCompletionConfig

func (c Config) managedWindowsOrca() bool {
	return runtime.GOOS == "windows" && c.hostName() == "orca" && c.agentName() == "codex" && codexProfileSettings(c).WindowsSandbox != ""
}
