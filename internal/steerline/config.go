package steerline

import (
	"bytes"
	"context"
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
	Repository      string            `json:"repository"`
	Remote          string            `json:"remote"`
	Branch          string            `json:"branch"`
	RunDir          string            `json:"run_dir"`
	WorktreeRoot    string            `json:"worktree_root"`
	RuntimeFile     string            `json:"runtime_file"`
	Claimant        string            `json:"claimant"`
	BD              Command           `json:"bd"`
	Git             Command           `json:"git"`
	Codex           Command           `json:"codex"`
	StartSkill      Skill             `json:"start_skill"`
	FinishSkill     Skill             `json:"finish_skill"`
	Workflow        string            `json:"workflow"`
	ExecutableTypes []string          `json:"executable_types"`
	TaskPoll        string            `json:"task_poll"`
	PRPoll          string            `json:"pr_poll"`
	Heartbeat       string            `json:"heartbeat"`
	CommandTimeout  string            `json:"command_timeout"`
	ShutdownGrace   string            `json:"shutdown_grace"`
	Model           string            `json:"model"`
	ReasoningEffort string            `json:"reasoning_effort"`
	Sandbox         string            `json:"sandbox"`
	Profile         string            `json:"profile"`
	Agent           string            `json:"agent"`
	Host            string            `json:"host"`
	StallTimeout    string            `json:"stall_timeout"`
	Orca            OrcaConfig        `json:"orca"`
	PR              PRConfig          `json:"pr"`
	Completion      *CompletionConfig `json:"completion"`
	identity        string
	startText       string
	finishText      string
	workerEnv       map[string]string
}

type OrcaConfig struct {
	Executable   string   `json:"executable"`
	Args         []string `json:"args"`
	Shell        string   `json:"shell"`
	WaitTimeout  string   `json:"wait_timeout"`
	KeepTerminal bool     `json:"keep_terminal"`
	Activate     bool     `json:"activate"`
	Fallback     string   `json:"fallback"`
}

type CompletionConfig struct {
	Command Command `json:"command"`
	Timeout string  `json:"timeout"`
}

func (c *CompletionConfig) validate() error {
	if c == nil {
		return nil
	}
	if c.Timeout == "" {
		c.Timeout = "2m"
	}
	if d, e := time.ParseDuration(c.Timeout); e != nil || d <= 0 {
		return errors.New("completion.timeout must be positive")
	}
	if c.Command.Executable == "" {
		return errors.New("completion.command.executable is required")
	}
	return nil
}

type PRConfig struct {
	Kind       string  `json:"kind"`
	BaseURL    string  `json:"base_url"`
	Repository string  `json:"repository"`
	TokenEnv   string  `json:"token_env"`
	Query      Command `json:"query"`
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
// service and viewer always run one stable binary regardless of which source
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
	c := Config{Remote: "origin", RuntimeFile: "runtime.json", TaskPoll: "10s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "30s", ShutdownGrace: "10s", Sandbox: "workspace-write", Git: Command{Executable: "git"}, BD: Command{Executable: "bd"}, Codex: Command{Executable: "codex"}, ExecutableTypes: []string{"task", "bug", "feature", "chore"}, Agent: "codex", Host: "orca", StallTimeout: "5m", Orca: OrcaConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "15m", Fallback: "local"}}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err = decode(b, &c); err != nil {
		return c, err
	}
	if c.agentName() == "codex" && c.ReasoningEffort == "" {
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
	if c.WorktreeRoot == "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	}
	if !filepath.IsAbs(c.WorktreeRoot) {
		c.WorktreeRoot = filepath.Join(base, c.WorktreeRoot)
	}
	if filepath.Base(c.RuntimeFile) != c.RuntimeFile || c.RuntimeFile == "." {
		return c, errors.New("runtime_file must be a filename inside run_dir")
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
	if c.Host != "" && c.Host != "local" && c.Host != "orca" {
		return c, errors.New("host must be local or orca")
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
		if c.Orca.Fallback != "" && c.Orca.Fallback != "local" && c.Orca.Fallback != "fail" {
			return c, errors.New("orca.fallback must be local or fail")
		}
		if d, e := time.ParseDuration(c.Orca.WaitTimeout); e != nil || d <= 0 {
			return c, errors.New("orca.wait_timeout must be a positive duration")
		}
	}
	if len(c.ExecutableTypes) == 0 {
		return c, errors.New("executable_types is empty")
	}
	if err = c.Completion.validate(); err != nil {
		return c, err
	}
	for _, s := range c.ExecutableTypes {
		if !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(s) || s == "epic" || s == "molecule" || s == "convoy" {
			return c, fmt.Errorf("invalid executable type %q", s)
		}
	}
	return c, nil
}
func duration(s string) time.Duration { d, _ := time.ParseDuration(s); return d }
func (c Config) Identity() string     { return c.identity }

// hostName, agentName and fallbackMode apply the documented defaults for
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
func (c Config) fallbackMode() string {
	if c.Orca.Fallback == "" {
		return "local"
	}
	return c.Orca.Fallback
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
	if e := c.Completion.validate(); e != nil {
		return e
	}
	if c.Completion != nil {
		if e := resolveCommand(&c.Completion.Command); e != nil {
			return e
		}
	}
	for _, cmd := range []*Command{&c.Git, &c.BD, &c.Codex} {
		if e := resolveCommand(cmd); e != nil {
			return e
		}
	}
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	if c.hostName() == "orca" {
		if _, e := r.Output(ctx, Command{Executable: c.Orca.Executable}, "status", "--json"); e != nil {
			return fmt.Errorf("orca host is not reachable: %w", e)
		}
	}
	root, e := r.Output(ctx, c.Git, "rev-parse", "--show-toplevel")
	if e != nil {
		return e
	}
	c.Repository, e = filepath.EvalSymlinks(strings.TrimSpace(string(root)))
	if e != nil {
		return e
	}
	r.Dir = c.Repository
	common, e := r.Output(ctx, c.Git, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil {
		return e
	}
	c.identity, e = filepath.EvalSymlinks(strings.TrimSpace(string(common)))
	if e != nil {
		return e
	}
	c.identity = canonical(c.identity)
	if c.Claimant == "" {
		c.Claimant = "steerline-" + shortHash(c.identity)
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9._-]+$`).MatchString(c.Claimant) {
		return errors.New("claimant must contain only letters, numbers, dot, underscore or hyphen")
	}
	if _, e = r.Output(ctx, c.Git, "check-ref-format", "refs/heads/"+c.Branch); e != nil {
		return e
	}
	remote, e := r.Output(ctx, c.Git, "remote", "get-url", c.Remote)
	if e != nil {
		return e
	}
	if e = c.PR.ValidateRemote(strings.TrimSpace(string(remote))); e != nil {
		return e
	}
	if c.PR.Kind == "command" {
		if e = resolveCommand(&c.PR.Query); e != nil {
			return e
		}
	}
	// Keep runtime data outside every checkout; never edit the target's ignore rules.
	if e = os.MkdirAll(c.RunDir, 0700); e != nil {
		return e
	}
	c.RunDir, e = filepath.EvalSymlinks(c.RunDir)
	if e != nil {
		return e
	}
	if inside(c.Repository, c.RunDir) {
		return errors.New("run_dir must be outside the target checkout")
	}
	wt, e := r.Output(ctx, c.Git, "worktree", "list", "--porcelain", "-z")
	if e != nil {
		return e
	}
	if e = c.prepareWorktreeRoot(); e != nil {
		return e
	}
	for _, line := range strings.Split(string(wt), "\x00") {
		if strings.HasPrefix(line, "worktree ") && inside(strings.TrimPrefix(line, "worktree "), c.RunDir) {
			return errors.New("run_dir must be outside all linked worktrees")
		}
		if strings.HasPrefix(line, "worktree ") && inside(strings.TrimPrefix(line, "worktree "), c.WorktreeRoot) {
			return errors.New("worktree_root must be outside all existing checkouts")
		}
	}
	for _, probe := range []struct {
		cmd   Command
		args  []string
		flags []string
	}{
		{c.BD, []string{"ready", "--help"}, []string{"--unassigned", "--limit"}},
		{c.BD, []string{"update", "--help"}, []string{"--claim", "--actor"}},
		{c.BD, []string{"list", "--help"}, []string{"--assignee", "--limit", "--all"}},
		{c.BD, []string{"close", "--help"}, []string{"--reason"}},
		{c.Codex, []string{"exec", "--help"}, []string{"--json", "--output-schema", "--output-last-message", "--sandbox"}},
	} {
		b, e := r.Output(ctx, probe.cmd, probe.args...)
		if e != nil {
			return e
		}
		for _, flag := range probe.flags {
			if !strings.Contains(string(b), flag) {
				return fmt.Errorf("tool help missing required flag %s", flag)
			}
		}
	}
	b := Beads{Runner: r, Command: c.BD, Claimant: c.Claimant}
	if _, e = b.Ready(ctx); e != nil {
		return fmt.Errorf("Beads preflight (no initialization performed): %w", e)
	}
	if _, e = b.Owned(ctx); e != nil {
		return e
	}
	if _, e = b.Context(ctx); e != nil {
		return fmt.Errorf("shared Beads context: %w", e)
	}
	if !checkSkills {
		return nil
	}
	if e = c.validateSkills(); e != nil {
		return e
	}
	if c.PR.Kind == "gitea" {
		if e = (&PRClient{Config: c.PR, Runner: r}).Check(ctx); e != nil {
			return e
		}
	}
	return nil
}

func (c *Config) validateSkills() error {
	for _, entry := range []struct {
		s      *Skill
		text   *string
		marker string
	}{{&c.StartSkill, &c.startText, "steerline:start-v1"}, {&c.FinishSkill, &c.finishText, "steerline:finish-v1"}} {
		if entry.s.Path == "" || entry.s.Invocation == "" {
			return errors.New("both skill paths and invocation instructions are required")
		}
		p := entry.s.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(c.Repository, p)
		}
		p, e := filepath.EvalSymlinks(p)
		if e != nil {
			return fmt.Errorf("missing skill: %w", e)
		}
		if !inside(c.Repository, p) {
			return errors.New("skill entry must resolve inside the configured repository")
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		*entry.text = string(b)
		entry.s.Path = p
		if !strings.Contains(*entry.text, "<!-- "+entry.marker+" -->") {
			return fmt.Errorf("skill contract conflict: %s lacks reviewed <!-- %s --> adapter; see docs/skill-contract.md", p, entry.marker)
		}
	}
	return nil
}
func inside(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
