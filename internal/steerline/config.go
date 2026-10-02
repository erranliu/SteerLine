package steerline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	ExecutionDirectory      *ExecutionDirectory             `json:"-"`
	ProcessHost             executionapi.ManagedProcessHost `json:"-"`
	WorkspaceBranchPrefix   string                          `json:"-"`
	StateRoot               string                          `json:"-"`
	RepositoryStateKey      string                          `json:"-"`
	fixedProfileDigest      string
	frozenExecutionConfig   bool
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

func (c Config) ValidateProfiles() error { return validateExecutionProfiles(c) }

func (c Config) ValidateApplicationPaths() error {
	if err := validateNativeStateLocation(c); err != nil {
		return err
	}
	return preflightNativeStatePath(c)
}

// BindApplicationConfig attaches the selected application directory and the
// exact files used to load this configuration. It contains no loaded native
// settings; those remain in the implementation envelopes on the profiles.
func (c *Config) BindApplicationConfig(directory *ExecutionDirectory, projectPath, directoryPath, digest string, global bool) {
	c.ExecutionDirectory = directory
	c.policySource = projectPath
	c.executionConfigPath = directoryPath
	c.configDigest = digest
	c.globalExecution = global
}

// LegacyCompletionConfig is decoded only as a marker for rejecting old runtime
// projections; it is never accepted in Config or executed.

// trimBOM removes a leading UTF-8 byte-order mark. Windows tooling (notably
// PowerShell) prepends one when writing JSON, and encoding/json rejects it.
func trimBOM(b []byte) []byte         { return contracts.TrimBOM(b) }
func decode(data []byte, v any) error { return contracts.DecodeJSON(data, v) }

func validateWorkflowConfig(c *Config, _ []byte) error {
	entry, err := registrationFor(*c)
	if err != nil {
		return err
	}
	if err := c.ExecutionRequirements.Validate(); err != nil {
		return err
	}
	if err := entry.ValidatePublicationPolicy(c.PublicationPolicy, c.PR); err != nil {
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

func positiveDuration(value string, fallback time.Duration) time.Duration {
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

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
	if c.frozenExecutionConfig {
		return validateImplementationProfileData(*c)
	}
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

// hostName and agentName expose configured implementation identity values.
func (c Config) hostName() string {
	return c.Host
}
func (c Config) agentName() string {
	return c.Agent
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
	entry, err := registrationFor(*c)
	if err != nil {
		return err
	}
	if entry.ValidateConfig != nil {
		if err := entry.ValidateConfig(ctx, *c); err != nil {
			return err
		}
	}
	commands := []*Command{&c.Git}
	for _, command := range commands {
		if err := resolveCommand(command); err != nil {
			return err
		}
	}
	git := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	repository, err := workspace.ResolveRepository(ctx, workspaceGit(*c), c.Repository)
	if err != nil {
		return err
	}
	c.Repository, c.identity = repository.Path, canonical(repository.CommonDir)
	git.Dir = c.Repository
	if entry.Preflight != nil {
		if err := entry.Preflight(ctx, *c); err != nil {
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

func inside(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

type Command = contracts.Command

type LegacyCompletionConfig = contracts.LegacyCompletionConfig
