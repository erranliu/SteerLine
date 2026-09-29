package steerline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type Command struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
}
type Skill struct {
	Path       string `json:"path"`
	Invocation string `json:"invocation"`
}
type Config struct {
	Repository              string             `json:"repository"`
	TaskAuthority           string             `json:"task_authority,omitempty"`
	Remote                  string             `json:"remote"`
	Branch                  string             `json:"branch"`
	RunDir                  string             `json:"run_dir"`
	WorktreeRoot            string             `json:"worktree_root"`
	RuntimeFile             string             `json:"runtime_file"`
	Claimant                string             `json:"claimant"`
	Git                     Command            `json:"git"`
	Codex                   Command            `json:"codex"`
	WorkflowProtocol        string             `json:"workflow_protocol,omitempty"`
	ExecutableTypes         []string           `json:"executable_types"`
	TaskPoll                string             `json:"task_poll"`
	PRPoll                  string             `json:"pr_poll"`
	Heartbeat               string             `json:"heartbeat"`
	CommandTimeout          string             `json:"command_timeout"`
	ShutdownGrace           string             `json:"shutdown_grace"`
	Model                   string             `json:"model"`
	ReasoningEffort         string             `json:"reasoning_effort"`
	Sandbox                 string             `json:"sandbox"`
	Profile                 string             `json:"profile"`
	Agent                   string             `json:"agent"`
	Host                    string             `json:"host"`
	ExecutionProfiles       []ExecutionProfile `json:"execution_profiles,omitempty"`
	DefaultExecutionProfile string             `json:"default_execution_profile,omitempty"`
	StallTimeout            string             `json:"stall_timeout"`
	Orca                    OrcaConfig         `json:"orca"`
	PR                      PRConfig           `json:"pr"`
	identity                string
	workerEnv               map[string]string
	configDigest            string
}

// LoadedConfigDigest identifies the exact config file bytes used to build this
// value. A console can compare it with a running service before presenting its
// execution settings as the service's current settings.
func (c Config) LoadedConfigDigest() string { return c.configDigest }

type OrcaConfig struct {
	Executable   string                `json:"executable"`
	Args         []string              `json:"args"`
	Shell        string                `json:"shell"`
	WaitTimeout  string                `json:"wait_timeout"`
	KeepTerminal bool                  `json:"keep_terminal"`
	Activate     bool                  `json:"activate"`
	Supervised   *SupervisedOrcaConfig `json:"supervised,omitempty"`
}

type SupervisedOrcaConfig struct {
	ConnectionID            string `json:"connection_id"`
	EnvironmentID           string `json:"environment_id"`
	RepositorySelector      string `json:"repository_selector"`
	CoordinatorTerminal     string `json:"coordinator_terminal"`
	ExpectedCodexVersion    string `json:"expected_codex_version,omitempty"` // Optional exact pin; empty accepts and records the observed CLI version.
	PollInterval            string `json:"poll_interval"`
	CapabilityReportVersion int    `json:"capability_report_version"`
	RemoteCancelAvailable   bool   `json:"remote_cancel_available"`
}

// LegacyCompletionConfig is decoded only as a marker for rejecting old runtime
// projections; it is never accepted in Config or executed.
type LegacyCompletionConfig struct {
	Command Command `json:"command"`
	Timeout string  `json:"timeout"`
}

type PRConfig struct {
	Kind       string  `json:"kind"`
	BaseURL    string  `json:"base_url"`
	Repository string  `json:"repository"`
	TokenEnv   string  `json:"token_env"`
	Query      Command `json:"query"`
	Close      Command `json:"close,omitempty"`
}

// trimBOM removes a leading UTF-8 byte-order mark. Windows tooling (notably
// PowerShell) prepends one when writing JSON, and encoding/json rejects it.
func trimBOM(b []byte) []byte {
	return bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
}
func decode(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(trimBOM(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("expected exactly one JSON value")
	}
	return nil
}

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

func SteerlineBinDir() string    { return filepath.Join(SteerlineHome(), "bin") }
func SteerlineEtcDir() string    { return filepath.Join(SteerlineHome(), "etc") }
func SteerlineStateRoot() string { return filepath.Join(SteerlineHome(), "state") }

// repoStateKey derives a worktree-independent key for a repository from its Git
// common dir, so every worktree and clone of the same repository maps to one
// state directory.
func repoStateKey(c Config) (string, error) {
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	out, e := r.Output(context.Background(), c.Git, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil {
		return "", e
	}
	common := strings.TrimSpace(string(out))
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
	c := Config{Remote: "origin", RuntimeFile: "runtime.json", TaskPoll: "10s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "30s", ShutdownGrace: "10s", Sandbox: "workspace-write", Git: Command{Executable: "git"}, Codex: Command{Executable: "codex"}, ExecutableTypes: []string{"task", "bug", "feature", "chore"}, Agent: "codex", Host: "orca", StallTimeout: "5m", Orca: OrcaConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "15m"}}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	digest := sha256.Sum256(b)
	c.configDigest = hex.EncodeToString(digest[:])
	if err = decode(b, &c); err != nil {
		return c, err
	}
	if c.TaskAuthority != "native" {
		return c, errors.New("task_authority must be explicitly set to native; Beads is not supported")
	}
	if c.WorkflowProtocol != WorkerFlowVersion {
		return c, errors.New("workflow_protocol must be worker-flow-v1; old workflow configurations are unsupported and their external state is preserved")
	}
	var configFields map[string]json.RawMessage
	_ = json.Unmarshal(b, &configFields)
	if _, explicit := configFields["host"]; !explicit {
		c.Host = "local"
	}
	if c.hostName() == "local" && c.agentName() == "codex" && c.ReasoningEffort == "" {
		c.ReasoningEffort = "high"
	}
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
	if filepath.Base(c.RuntimeFile) != c.RuntimeFile || c.RuntimeFile == "." {
		return c, errors.New("runtime_file must be a filename inside run_dir")
	}
	switch c.RuntimeFile {
	case "steerline.db", "steerline.db-wal", "steerline.db-shm", "service", "execution", "runs", "worktrees":
		return c, errors.New("runtime_file conflicts with native authority or execution state")
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
	if c.Sandbox != "read-only" && c.Sandbox != "workspace-write" && c.Sandbox != "danger-full-access" {
		return c, errors.New("sandbox must be read-only, workspace-write or explicitly configured danger-full-access")
	}
	if c.ReasoningEffort != "" {
		switch c.ReasoningEffort {
		case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		default:
			return c, fmt.Errorf("invalid reasoning_effort %q", c.ReasoningEffort)
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
	if c.Host == "orca" {
		if c.agentName() != "codex" && (c.Model != "" || c.Profile != "" || c.ReasoningEffort != "") {
			return c, errors.New("orca model, profile and reasoning_effort overrides require agent=codex")
		}
		if c.Orca.Executable == "" || c.Orca.Shell == "" {
			return c, errors.New("orca.executable and orca.shell are required")
		}
		if d, e := time.ParseDuration(c.Orca.WaitTimeout); e != nil || d <= 0 {
			return c, errors.New("orca.wait_timeout must be a positive duration")
		}
	}
	if c.Host == "orca_supervised" {
		var orcaFields map[string]json.RawMessage
		var supervisedFields map[string]json.RawMessage
		if json.Unmarshal(configFields["orca"], &orcaFields) != nil || json.Unmarshal(orcaFields["supervised"], &supervisedFields) != nil {
			return c, errors.New("orca.supervised must be an explicit object")
		}
		if _, ok := supervisedFields["remote_cancel_available"]; !ok {
			return c, errors.New("orca.supervised.remote_cancel_available must be explicitly set to false")
		}
		if err = validateSupervisedOrcaConfig(&c); err != nil {
			return c, err
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
	if err = validateWorkflowConfig(&c, b); err != nil {
		return c, err
	}
	if err = validateExecutionProfiles(c); err != nil {
		return c, err
	}
	return c, nil
}

func validateSupervisedOrcaConfig(c *Config) error {
	if c == nil || c.agentName() != "codex" {
		return errors.New("orca_supervised currently requires agent=codex")
	}
	if c.Model == "" || c.ReasoningEffort == "" {
		return errors.New("orca_supervised requires explicit model and reasoning_effort; the remote Orca default is never inherited")
	}
	if c.Orca.Supervised == nil {
		return errors.New("orca_supervised requires an explicit orca.supervised configuration")
	}
	supervised := c.Orca.Supervised
	if strings.TrimSpace(supervised.ConnectionID) == "" || strings.TrimSpace(supervised.EnvironmentID) == "" || strings.TrimSpace(supervised.RepositorySelector) == "" || strings.TrimSpace(supervised.CoordinatorTerminal) == "" {
		return errors.New("orca.supervised requires fixed connection_id, environment_id, repository_selector, and coordinator_terminal")
	}
	if supervised.RemoteCancelAvailable {
		return errors.New("orca_supervised remote cancellation is fixed unavailable")
	}
	if strings.TrimSpace(supervised.PollInterval) == "" {
		return errors.New("orca.supervised.poll_interval must be explicit")
	}
	if d, e := time.ParseDuration(supervised.PollInterval); e != nil || d <= 0 {
		return errors.New("orca.supervised.poll_interval must be a positive duration")
	}
	if supervised.CapabilityReportVersion != 1 {
		return errors.New("orca.supervised.capability_report_version must explicitly pin version 1")
	}
	if c.Orca.Executable == "" || c.Orca.Shell == "" {
		return errors.New("orca.executable and orca.shell are required")
	}
	if d, e := time.ParseDuration(c.Orca.WaitTimeout); e != nil || d <= 0 {
		return errors.New("orca.wait_timeout must be a positive duration")
	}
	return nil
}

func validateWorkflowConfig(c *Config, _ []byte) error {
	if c.WorkflowProtocol != WorkerFlowVersion {
		return errors.New("only worker-flow-v1 is supported")
	}
	return nil
}
func duration(s string) time.Duration { d, _ := time.ParseDuration(s); return d }
func (c Config) Identity() string     { return c.identity }

// ResolveIdentity reads only Git metadata and does not run task-provider probes
// or create runtime directories. Control clients use it without constructing a
// Service or taking ownership of scheduler state.
func (c *Config) ResolveIdentity(ctx context.Context) error {
	timeout := duration(c.CommandTimeout)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	r := Runner{Dir: c.Repository, Timeout: timeout}
	common, err := r.Output(ctx, c.Git, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	path := strings.TrimSpace(string(common))
	if path == "" {
		return errors.New("git returned an empty common dir")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	identity := canonical(resolved)
	if c.identity != "" && canonical(c.identity) != identity {
		return errors.New("configured repository identity changed after it was resolved")
	}
	c.identity = identity
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
	return c.requiresLocalCodex() || c.hostName() == "orca" && c.agentName() == "codex" && (c.Model != "" || c.ReasoningEffort != "" || c.Profile != "")
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
	if err := requireNativeTaskAuthority(*c); err != nil {
		return err
	}
	if err := validateExecutionProfiles(*c); err != nil {
		return err
	}
	if c.WorkflowProtocol == WorkerFlowVersion && c.hostName() != "local" {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "only local Codex worker-flow is delivered; Orca targets are deferred"}
	}
	if c.hostName() == "local" && c.agentName() != "codex" {
		return errors.New("local_process currently supports only agent=codex")
	}
	commands := []*Command{&c.Git}
	var err error
	if c.needsCodexExecutable() {
		commands = append(commands, &c.Codex)
	}
	for _, command := range commands {
		if err := resolveCommand(command); err != nil {
			return err
		}
	}
	git := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	if c.hostName() == "orca" {
		if _, err := (Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}).Output(ctx, Command{Executable: c.Orca.Executable, Args: c.Orca.Args}, "status", "--json"); err != nil {
			return fmt.Errorf("orca host is not reachable: %w", err)
		}
	}
	root, err := git.Output(ctx, c.Git, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	c.Repository, err = filepath.EvalSymlinks(strings.TrimSpace(string(root)))
	if err != nil {
		return err
	}
	git.Dir = c.Repository
	common, err := git.Output(ctx, c.Git, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	c.identity, err = filepath.EvalSymlinks(strings.TrimSpace(string(common)))
	if err != nil {
		return err
	}
	c.identity = canonical(c.identity)
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
		if err = validateSupervisedOrcaConfig(c); err != nil {
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
		if err = validateNativeRuntimeBinding(*c); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if err = ensureNativeStateClean(c.RunDir, c.Repository); err != nil {
		return err
	}
	if c.PR.Kind == "gitea" {
		if err = (&PRClient{Config: c.PR, Runner: git}).Check(ctx); err != nil {
			return err
		}
	}
	return nil
}

func validateNativeRuntimeBinding(c Config) error {
	path := filepath.Join(c.RunDir, c.RuntimeFile)
	runtime, err := LoadRuntime(path)
	if os.IsNotExist(err) || err == nil && runtime == nil {
		return nil
	}
	if err != nil {
		return nativeErr("unsupported_schema", "native runtime projection cannot be read")
	}
	if runtime.SchemaVersion != runtimeSchemaFor(c) || canonical(runtime.Repository.Identity) != canonical(c.identity) || canonical(runtime.Repository.Path) != canonical(c.Repository) || runtime.Repository.Remote != c.Remote || runtime.Repository.Branch != c.Branch {
		return nativeErr("authority_mismatch", "native runtime projection does not match the configured repository authority")
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
