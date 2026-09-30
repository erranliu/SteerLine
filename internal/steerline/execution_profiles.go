package steerline

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
)

const ProjectExecutionProfileID = "project-default"

// ExecutionProfile is one complete, named execution route for a native task.
// Repository authority, PR provider, and task DAG stay project-wide.
type ExecutionProfile struct {
	ExecutionPolicy       executionapi.ExecutionPolicy `json:"-"`
	AcceptedConfigVersion string                       `json:"-"`
	PolicySource          string                       `json:"-"`
	ID                    string                       `json:"id"`
	Name                  string                       `json:"name"`
	Host                  string                       `json:"host"`
	Agent                 string                       `json:"agent"`
	Model                 string                       `json:"model,omitempty"`
	ReasoningEffort       string                       `json:"reasoning_effort,omitempty"`
	Sandbox               string                       `json:"-"`
	WindowsSandbox        string                       `json:"-"`
	CodexProfile          string                       `json:"profile,omitempty"`
	Codex                 Command                      `json:"codex"`
	Orca                  OrcaConfig                   `json:"orca,omitempty"`
}

// FixedExecutionProfile is stored with a WorkflowRun.
type FixedExecutionProfile = contracts.FixedExecutionProfile

func (p ExecutionProfile) Freeze() FixedExecutionProfile {
	p = p.withWindowsSandboxDefault()
	policy := "never"
	return FixedExecutionProfile{Version: 2, ExecutionPolicy: p.ExecutionPolicy, PolicyDigest: digestValue(p.ExecutionPolicy), AcceptedConfigVersion: p.AcceptedConfigVersion, PolicySource: p.PolicySource, ID: p.ID, Name: p.Name, Digest: p.Digest(), Host: p.Host, Agent: p.Agent, Model: p.Model, ReasoningEffort: p.ReasoningEffort, Sandbox: p.Sandbox, WindowsSandbox: p.WindowsSandbox, ApprovalPolicy: policy, CodexProfile: p.CodexProfile, Codex: p.Codex, Orca: p.Orca}
}

func verifyFixedExecutionProfile(p FixedExecutionProfile) bool {
	permissions, err := codexworker.CompilePolicy(p.ExecutionPolicy, runtime.GOOS, p.Host, p.Agent, p.Codex.WindowsSandbox, p.CodexProfile, p.Codex.Args)
	if err != nil || permissions.Sandbox != p.Sandbox || permissions.WindowsSandbox != p.WindowsSandbox {
		return false
	}
	rebuilt := ExecutionProfile{ExecutionPolicy: p.ExecutionPolicy, AcceptedConfigVersion: p.AcceptedConfigVersion, PolicySource: p.PolicySource, ID: p.ID, Name: p.Name, Host: p.Host, Agent: p.Agent, Model: p.Model, ReasoningEffort: p.ReasoningEffort, Sandbox: p.Sandbox, WindowsSandbox: p.WindowsSandbox, CodexProfile: p.CodexProfile, Codex: p.Codex, Orca: p.Orca}
	return p.Version == 2 && p.ExecutionPolicy.Validate() == nil && p.PolicyDigest == digestValue(p.ExecutionPolicy) && p.AcceptedConfigVersion != "" && p.PolicySource != "" && p.ApprovalPolicy == "never" && p.Digest != "" && p.Digest == rebuilt.Digest() && (!rebuilt.managedOrcaPermissions() || p.ApprovalPolicy == "never" && codexworker.ValidWindowsSandbox(p.WindowsSandbox))
}

func (c Config) ApplyFixedExecutionProfile(p FixedExecutionProfile) Config {
	c.Host, c.Agent, c.Model, c.ReasoningEffort = p.Host, p.Agent, p.Model, p.ReasoningEffort
	c.Sandbox, c.Profile, c.Codex, c.Orca = p.Sandbox, p.CodexProfile, p.Codex, p.Orca
	c.WindowsSandbox = p.WindowsSandbox
	c.fixedProfileDigest = p.Digest
	c.ExecutionPolicy, c.configDigest, c.policySource = p.ExecutionPolicy, p.AcceptedConfigVersion, p.PolicySource
	return c
}

func (p ExecutionProfile) Digest() string {
	p = p.withWindowsSandboxDefault()
	return digestValue(struct {
		ExecutionProfile
		Version               int
		Policy                executionapi.ExecutionPolicy
		AcceptedConfigVersion string
		PolicySource          string
		Sandbox               string
		WindowsSandbox        string
		Contract              string
	}{p, 2, p.ExecutionPolicy, p.AcceptedConfigVersion, p.PolicySource, p.Sandbox, p.WindowsSandbox, codexworker.PolicyContract})
}

func (p ExecutionProfile) managedOrcaPermissions() bool {
	return p.Host == "orca" && p.Agent == "codex" && (runtime.GOOS == "windows" || p.WindowsSandbox != "")
}

func (p ExecutionProfile) MatchesConfig(c Config) bool {
	actual := c.projectExecutionProfile()
	actual.ID, actual.Name = p.ID, p.Name
	return p.ID != "" && actual.Digest() == p.Digest()
}

func (p ExecutionProfile) StaticUnavailableReason() string {
	if p.Host == "orca" {
		if err := validateLocalOrcaTargetArgs(p.Orca.Args); err != nil {
			return err.Error()
		}
	}
	if p.Host == "orca_supervised" {
		c := Config{Orca: p.Orca}
		if err := remote.CheckPolicy(p.ExecutionPolicy, remoteWorkerConfig(c).Settings); err != nil {
			return err.Error()
		}
		return ""
	}
	_, err := codexworker.CompilePolicy(p.ExecutionPolicy, runtime.GOOS, p.Host, p.Agent, p.Codex.WindowsSandbox, p.CodexProfile, p.Codex.Args)
	if err != nil {
		return err.Error()
	}
	return ""
}

func (p ExecutionProfile) withWindowsSandboxDefault() ExecutionProfile {
	permissions, err := codexworker.CompilePolicy(p.ExecutionPolicy, runtime.GOOS, p.Host, p.Agent, p.Codex.WindowsSandbox, p.CodexProfile, p.Codex.Args)
	if err == nil {
		p.Sandbox, p.WindowsSandbox = permissions.Sandbox, permissions.WindowsSandbox
		p.Codex.WindowsSandbox = permissions.WindowsSandbox
	}
	return p
}

func (c Config) bindProjectPolicy(p ExecutionProfile) ExecutionProfile {
	p.ExecutionPolicy, p.AcceptedConfigVersion, p.PolicySource = c.ExecutionPolicy, c.acceptedConfigVersion(), c.policySource
	if p.PolicySource == "" {
		p.PolicySource = "service:explicit-config"
	}
	return p.effective()
}
func (c Config) acceptedConfigVersion() string {
	if c.configDigest != "" {
		return c.configDigest
	}
	return digestValue(struct {
		Policy             executionapi.ExecutionPolicy
		Repository, Branch string
	}{c.ExecutionPolicy, c.Repository, c.Branch})
}

func (p ExecutionProfile) effective() ExecutionProfile {
	p = p.withWindowsSandboxDefault()
	if p.Host == "local" && p.Agent == "codex" && p.ReasoningEffort == "" {
		p.ReasoningEffort = codexworker.LocalReasoningEffortDefault(p.ReasoningEffort)
	}
	return p
}

func (c Config) projectExecutionProfile() ExecutionProfile {
	return c.bindProjectPolicy(ExecutionProfile{ID: ProjectExecutionProfileID, Name: "项目默认", Host: c.hostName(), Agent: c.agentName(), Model: c.Model, ReasoningEffort: c.ReasoningEffort, Sandbox: c.Sandbox, WindowsSandbox: c.WindowsSandbox, CodexProfile: c.Profile, Codex: c.Codex, Orca: c.Orca})
}

func (c Config) AvailableExecutionProfiles() []ExecutionProfile {
	if c.globalExecution {
		out := make([]ExecutionProfile, 0, len(c.ExecutionProfiles))
		for _, profile := range c.ExecutionProfiles {
			out = append(out, c.bindProjectPolicy(profile))
		}
		return out
	}
	out := make([]ExecutionProfile, 0, len(c.ExecutionProfiles)+1)
	out = append(out, c.projectExecutionProfile())
	for _, profile := range c.ExecutionProfiles {
		out = append(out, c.bindProjectPolicy(profile))
	}
	return out
}

func (c Config) DefaultProfileID() string {
	if c.DefaultExecutionProfile != "" {
		return c.DefaultExecutionProfile
	}
	if c.globalExecution {
		if len(c.ExecutionProfiles) == 1 {
			return c.ExecutionProfiles[0].ID
		}
		return ""
	}
	if len(c.ExecutionProfiles) == 0 {
		return ProjectExecutionProfileID
	}
	return ""
}

func (c Config) ResolveExecutionProfile(id, expectedDigest string) (Config, ExecutionProfile, error) {
	for _, profile := range c.AvailableExecutionProfiles() {
		if profile.ID != id {
			continue
		}
		if expectedDigest != "" && expectedDigest != profile.Digest() {
			return Config{}, ExecutionProfile{}, errors.New("stale_profile: execution profile changed")
		}
		if reason := profile.StaticUnavailableReason(); reason != "" {
			return Config{}, ExecutionProfile{}, errors.New(reason)
		}
		selected := c.ApplyExecutionProfile(profile)
		return selected, profile, nil
	}
	return Config{}, ExecutionProfile{}, errors.New("execution profile is not configured")
}

func (c Config) ApplyExecutionProfile(p ExecutionProfile) Config {
	p = c.bindProjectPolicy(p)
	c.Host, c.Agent, c.Model, c.ReasoningEffort = p.Host, p.Agent, p.Model, p.ReasoningEffort
	c.Sandbox, c.Profile, c.Codex, c.Orca = p.Sandbox, p.CodexProfile, p.Codex, p.Orca
	c.WindowsSandbox = p.WindowsSandbox
	c.fixedProfileDigest = p.Digest()
	return c
}

func validateExecutionProfiles(c Config) error {
	if err := c.ExecutionPolicy.Validate(); err != nil {
		return err
	}
	ids := map[string]bool{}
	if !c.globalExecution {
		ids[ProjectExecutionProfileID] = true
	}
	pattern := regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	for _, p := range c.ExecutionProfiles {
		if !pattern.MatchString(p.ID) || ids[p.ID] || strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("execution profile ID or name is invalid: %q", p.ID)
		}
		ids[p.ID] = true
		if p.Model != strings.TrimSpace(p.Model) || strings.ContainsAny(p.Model, "\r\n\x00") {
			return errors.New("invalid model")
		}
		if p.Host == "orca" {
			if err := validateLocalOrcaTargetArgs(p.Orca.Args); err != nil {
				return err
			}
		}
		if p.CodexProfile != "" {
			return errors.New("native Codex profile overrides are unsupported")
		}
		if err := codexworker.ValidateManagedArgs(p.Codex.Args); err != nil {
			return err
		}
		if p.Host != "local" && p.Host != "orca" && p.Host != "orca_supervised" {
			return fmt.Errorf("execution profile %s has an unsupported host", p.ID)
		}
		if p.Agent == "" {
			return fmt.Errorf("execution profile %s requires an agent and valid sandbox", p.ID)
		}
		if p.Codex.WindowsSandbox != "" && !codexworker.ValidWindowsSandbox(p.Codex.WindowsSandbox) {
			return fmt.Errorf("execution profile %s has invalid windows_sandbox", p.ID)
		}
		if p.ReasoningEffort != "" && !codexworker.ValidReasoningEffort(p.ReasoningEffort) {
			return fmt.Errorf("execution profile %s has invalid reasoning effort", p.ID)
		}
		if p.Host == "local" && p.Agent != "codex" {
			return fmt.Errorf("execution profile %s requires codex for a resumable worker session", p.ID)
		}
		if p.Host == "orca" && (p.CodexProfile != "") {
			return fmt.Errorf("execution profile %s: Orca requires workspace-write and does not support a Codex profile override", p.ID)
		}
		if p.Host == "orca" && p.Model != "" && p.Agent != "codex" {
			return fmt.Errorf("execution profile %s: Orca model and reasoning_effort overrides require codex", p.ID)
		}
		if p.Host == "orca" && p.Model != "" && p.Codex.Executable == "" {
			return fmt.Errorf("execution profile %s: fixed Orca Codex launch requires codex.executable", p.ID)
		}
		if p.Host == "orca" && p.Model != "" && p.Orca.Shell != "powershell.exe" && p.Orca.Shell != "pwsh.exe" {
			return fmt.Errorf("execution profile %s: fixed Orca Codex launch requires powershell.exe or pwsh.exe", p.ID)
		}
		selected := c.ApplyExecutionProfile(p.effective())
		if p.Host == "orca" && p.Agent == "codex" && runtime.GOOS == "windows" && c.bindProjectPolicy(p).StaticUnavailableReason() == "" {
			if _, err := nativeOrcaFixedAgentCommand(selected); err != nil {
				return fmt.Errorf("execution profile %s: %w", p.ID, err)
			}
		}
		if p.Host == "orca_supervised" {
			if err := validateSupervisedOrcaConfig(&selected); err != nil {
				return fmt.Errorf("execution profile %s: %w", p.ID, err)
			}
		} else if p.Host == "orca" {
			if p.Orca.Executable == "" || p.Orca.Shell == "" || p.Orca.WaitTimeout == "" {
				return fmt.Errorf("execution profile %s requires Orca executable, shell and wait timeout", p.ID)
			}
			if d, err := time.ParseDuration(p.Orca.WaitTimeout); err != nil || d <= 0 {
				return fmt.Errorf("execution profile %s has invalid Orca wait timeout", p.ID)
			}
		}
	}
	if c.DefaultExecutionProfile != "" && !ids[c.DefaultExecutionProfile] {
		return errors.New("default_execution_profile is not configured")
	}
	return nil
}

func hostForExecution(backend, protocol string) string {
	switch {
	case backend == string(executionapi.BackendLocal) && (protocol == string(executionapi.ProtocolLocalProcess) || protocol == string(executionapi.ProtocolLocalCodexAppServer)):
		return "local"
	case backend == string(executionapi.BackendOrca) && protocol == string(executionapi.ProtocolOrcaTerminalV2):
		return "orca"
	case backend == string(executionapi.BackendOrca) && protocol == string(executionapi.ProtocolOrcaSupervised):
		return "orca_supervised"
	default:
		return ""
	}
}

func validateLocalOrcaTargetArgs(args []string) error {
	for _, arg := range args {
		if arg == "--environment" || arg == "--pairing-code" || strings.HasPrefix(arg, "--environment=") || strings.HasPrefix(arg, "--pairing-code=") {
			return errors.New("local Orca target cannot be redirected through orca.args; remote execution_policy requires the remote protocol adapter")
		}
	}
	return nil
}
