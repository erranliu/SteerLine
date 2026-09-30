package steerline

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

const ProjectExecutionProfileID = "project-default"

// ExecutionProfile is one complete, named execution route for a native task.
// Repository authority, PR provider, and task DAG stay project-wide.
type ExecutionProfile struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Host            string     `json:"host"`
	Agent           string     `json:"agent"`
	Model           string     `json:"model,omitempty"`
	ReasoningEffort string     `json:"reasoning_effort,omitempty"`
	Sandbox         string     `json:"sandbox"`
	WindowsSandbox  string     `json:"windows_sandbox,omitempty"`
	CodexProfile    string     `json:"profile,omitempty"`
	Codex           Command    `json:"codex"`
	Orca            OrcaConfig `json:"orca,omitempty"`
}

// FixedExecutionProfile is stored with a WorkflowRun.
type FixedExecutionProfile = contracts.FixedExecutionProfile

func (p ExecutionProfile) Freeze() FixedExecutionProfile {
	p = p.withWindowsSandboxDefault()
	policy := ""
	if p.managedOrcaPermissions() {
		policy = "never"
	}
	return FixedExecutionProfile{ID: p.ID, Name: p.Name, Digest: p.Digest(), Host: p.Host, Agent: p.Agent, Model: p.Model, ReasoningEffort: p.ReasoningEffort, Sandbox: p.Sandbox, WindowsSandbox: p.WindowsSandbox, ApprovalPolicy: policy, CodexProfile: p.CodexProfile, Codex: p.Codex, Orca: p.Orca}
}

func verifyFixedExecutionProfile(p FixedExecutionProfile) bool {
	rebuilt := ExecutionProfile{ID: p.ID, Name: p.Name, Host: p.Host, Agent: p.Agent, Model: p.Model, ReasoningEffort: p.ReasoningEffort, Sandbox: p.Sandbox, WindowsSandbox: p.WindowsSandbox, CodexProfile: p.CodexProfile, Codex: p.Codex, Orca: p.Orca}
	return p.Digest != "" && p.Digest == rebuilt.Digest() && (!rebuilt.managedOrcaPermissions() || p.ApprovalPolicy == "never" && (p.WindowsSandbox == "elevated" || p.WindowsSandbox == "unelevated"))
}

func (c Config) ApplyFixedExecutionProfile(p FixedExecutionProfile) Config {
	c.Host, c.Agent, c.Model, c.ReasoningEffort = p.Host, p.Agent, p.Model, p.ReasoningEffort
	c.Sandbox, c.Profile, c.Codex, c.Orca = p.Sandbox, p.CodexProfile, p.Codex, p.Orca
	c.WindowsSandbox = p.WindowsSandbox
	return c
}

func (p ExecutionProfile) Digest() string {
	p = p.withWindowsSandboxDefault()
	if p.managedOrcaPermissions() {
		return digestValue(struct {
			ExecutionProfile
			ApprovalPolicy     string `json:"managed_approval_policy"`
			PermissionContract string `json:"managed_permission_contract"`
		}{p, "never", "windows-codex-v1:explicit-linked-roots:exclude-temp:strict-config"})
	}
	return digestValue(p)
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
	if p.Host == "local" && p.Agent == "codex" {
		if _, err := codexapp.LocalSandboxArgs(runtime.GOOS, p.Sandbox, p.WindowsSandbox, p.Codex.Args); err != nil {
			return err.Error()
		}
	}
	if p.managedOrcaPermissions() {
		if runtime.GOOS != "windows" {
			return "Windows Codex 受管方案需要 Windows 宿主"
		}
		if _, err := nativeOrcaFixedAgentCommand(Config{Host: p.Host, Agent: p.Agent, Model: p.Model, ReasoningEffort: p.ReasoningEffort, Sandbox: p.Sandbox, WindowsSandbox: p.WindowsSandbox, Codex: p.Codex, Orca: p.Orca}); err != nil {
			return err.Error()
		}
	}
	if p.Host == "local" && p.Agent != "codex" {
		return "本地执行后端目前只支持 codex agent"
	}
	return ""
}

func (p ExecutionProfile) withWindowsSandboxDefault() ExecutionProfile {
	if runtime.GOOS == "windows" && p.Host == "local" && p.Agent == "codex" && p.Sandbox != "danger-full-access" && p.WindowsSandbox == "" {
		p.WindowsSandbox = "elevated"
	}
	return p
}

func (p ExecutionProfile) effective() ExecutionProfile {
	p = p.withWindowsSandboxDefault()
	if p.Host == "local" && p.Agent == "codex" && p.ReasoningEffort == "" {
		p.ReasoningEffort = "high"
	}
	return p
}

func (c Config) projectExecutionProfile() ExecutionProfile {
	return ExecutionProfile{ID: ProjectExecutionProfileID, Name: "项目默认", Host: c.hostName(), Agent: c.agentName(), Model: c.Model, ReasoningEffort: c.ReasoningEffort, Sandbox: c.Sandbox, WindowsSandbox: c.WindowsSandbox, CodexProfile: c.Profile, Codex: c.Codex, Orca: c.Orca}
}

func (c Config) AvailableExecutionProfiles() []ExecutionProfile {
	if c.globalExecution {
		out := make([]ExecutionProfile, 0, len(c.ExecutionProfiles))
		for _, profile := range c.ExecutionProfiles {
			out = append(out, profile.effective())
		}
		return out
	}
	out := make([]ExecutionProfile, 0, len(c.ExecutionProfiles)+1)
	out = append(out, c.projectExecutionProfile())
	for _, profile := range c.ExecutionProfiles {
		out = append(out, profile.effective())
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
		selected := c.ApplyExecutionProfile(profile)
		return selected, profile, nil
	}
	return Config{}, ExecutionProfile{}, errors.New("execution profile is not configured")
}

func (c Config) ApplyExecutionProfile(p ExecutionProfile) Config {
	c.Host, c.Agent, c.Model, c.ReasoningEffort = p.Host, p.Agent, p.Model, p.ReasoningEffort
	c.Sandbox, c.Profile, c.Codex, c.Orca = p.Sandbox, p.CodexProfile, p.Codex, p.Orca
	c.WindowsSandbox = p.WindowsSandbox
	return c
}

func validateExecutionProfiles(c Config) error {
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
		if p.Host != "local" && p.Host != "orca" && p.Host != "orca_supervised" {
			return fmt.Errorf("execution profile %s has an unsupported host", p.ID)
		}
		if p.Agent == "" || p.Sandbox != "read-only" && p.Sandbox != "workspace-write" && p.Sandbox != "danger-full-access" {
			return fmt.Errorf("execution profile %s requires an agent and valid sandbox", p.ID)
		}
		if p.WindowsSandbox != "" && p.WindowsSandbox != "elevated" && p.WindowsSandbox != "unelevated" {
			return fmt.Errorf("execution profile %s has invalid windows_sandbox", p.ID)
		}
		if p.ReasoningEffort != "" && !validReasoningEffort(p.ReasoningEffort) {
			return fmt.Errorf("execution profile %s has invalid reasoning effort", p.ID)
		}
		if p.Host == "local" && p.Agent != "codex" {
			return fmt.Errorf("execution profile %s requires codex for a resumable worker session", p.ID)
		}
		if p.Host == "orca" && (p.CodexProfile != "" || p.Sandbox != "workspace-write") {
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
		if p.Host == "orca" && p.Agent == "codex" && runtime.GOOS == "windows" {
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

func validReasoningEffort(value string) bool {
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		return true
	default:
		return false
	}
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
