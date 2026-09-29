package steerline

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const ProjectExecutionProfileID = "project-default"

// ExecutionProfile is one complete, named execution route for every step of a
// native task. Repository authority, PR provider, and task DAG stay project-wide.
type ExecutionProfile struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	Host            string        `json:"host"`
	Agent           string        `json:"agent"`
	Model           string        `json:"model,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
	Sandbox         string        `json:"sandbox"`
	CodexProfile    string        `json:"profile,omitempty"`
	Codex           Command       `json:"codex"`
	Orca            OrcaConfig    `json:"orca,omitempty"`
	ProjectSteps    *ProjectSteps `json:"project_steps"`
}

// FixedExecutionProfile is stored with a WorkflowRun. ProjectSteps are already
// fixed in that record, so only the execution route is stored here.
type FixedExecutionProfile struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Digest          string     `json:"digest"`
	Host            string     `json:"host"`
	Agent           string     `json:"agent"`
	Model           string     `json:"model,omitempty"`
	ReasoningEffort string     `json:"reasoning_effort,omitempty"`
	Sandbox         string     `json:"sandbox"`
	CodexProfile    string     `json:"profile,omitempty"`
	Codex           Command    `json:"codex"`
	Orca            OrcaConfig `json:"orca,omitempty"`
}

func (p ExecutionProfile) Freeze() FixedExecutionProfile {
	return FixedExecutionProfile{ID: p.ID, Name: p.Name, Digest: p.Digest(), Host: p.Host, Agent: p.Agent, Model: p.Model, ReasoningEffort: p.ReasoningEffort, Sandbox: p.Sandbox, CodexProfile: p.CodexProfile, Codex: p.Codex, Orca: p.Orca}
}

func (p FixedExecutionProfile) Verify(steps ProjectSteps) bool {
	rebuilt := ExecutionProfile{ID: p.ID, Name: p.Name, Host: p.Host, Agent: p.Agent, Model: p.Model, ReasoningEffort: p.ReasoningEffort, Sandbox: p.Sandbox, CodexProfile: p.CodexProfile, Codex: p.Codex, Orca: p.Orca, ProjectSteps: &steps}
	return p.Digest != "" && p.Digest == rebuilt.Digest()
}

func (p FixedExecutionProfile) VerifyWorkerFlow() bool {
	rebuilt := ExecutionProfile{ID: p.ID, Name: p.Name, Host: p.Host, Agent: p.Agent, Model: p.Model, ReasoningEffort: p.ReasoningEffort, Sandbox: p.Sandbox, CodexProfile: p.CodexProfile, Codex: p.Codex, Orca: p.Orca}
	return p.Digest != "" && p.Digest == rebuilt.Digest()
}

func (c Config) ApplyFixedExecutionProfile(p FixedExecutionProfile, steps ProjectSteps) Config {
	c.Host, c.Agent, c.Model, c.ReasoningEffort = p.Host, p.Agent, p.Model, p.ReasoningEffort
	c.Sandbox, c.Profile, c.Codex, c.Orca = p.Sandbox, p.CodexProfile, p.Codex, p.Orca
	c.ProjectSteps = &steps
	return c
}

func (p ExecutionProfile) Digest() string { return digestValue(p) }

func (p ExecutionProfile) MatchesConfig(c Config) bool {
	actual := c.projectExecutionProfile()
	actual.ID, actual.Name = p.ID, p.Name
	return p.ID != "" && actual.Digest() == p.Digest()
}

func (p ExecutionProfile) StaticUnavailableReason() string {
	if p.ProjectSteps == nil && p.Host != "local" {
		return "Orca worker-flow 本次延期交付；只开放本地 Codex"
	}
	if p.Host == "local" && p.Agent != "codex" {
		return "本地执行后端目前只支持 codex agent"
	}
	if p.ProjectSteps == nil || p.Agent == "codex" {
		return ""
	}
	steps := []StepContract{p.ProjectSteps.Implement, p.ProjectSteps.Verify, p.ProjectSteps.PublishPR, p.ProjectSteps.Review}
	for _, step := range steps {
		if step.Provider == "agent_skill" {
			return "当前 native agent_skill 步骤只支持 codex"
		}
	}
	return ""
}

func (p ExecutionProfile) effective() ExecutionProfile {
	if p.Host == "local" && p.Agent == "codex" && p.ReasoningEffort == "" {
		p.ReasoningEffort = "high"
	}
	return p
}

func (c Config) projectExecutionProfile() ExecutionProfile {
	return ExecutionProfile{ID: ProjectExecutionProfileID, Name: "项目默认", Host: c.hostName(), Agent: c.agentName(), Model: c.Model, ReasoningEffort: c.ReasoningEffort, Sandbox: c.Sandbox, CodexProfile: c.Profile, Codex: c.Codex, Orca: c.Orca, ProjectSteps: c.ProjectSteps}
}

func (c Config) AvailableExecutionProfiles() []ExecutionProfile {
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
		if selected.hostName() == "orca_supervised" && !selected.WorkspaceDirtyIgnore.empty() {
			return Config{}, ExecutionProfile{}, errors.New("supervised Orca cannot independently verify workspace_dirty_ignore paths")
		}
		return selected, profile, nil
	}
	return Config{}, ExecutionProfile{}, errors.New("execution profile is not configured")
}

func (c Config) ApplyExecutionProfile(p ExecutionProfile) Config {
	c.Host, c.Agent, c.Model, c.ReasoningEffort = p.Host, p.Agent, p.Model, p.ReasoningEffort
	c.Sandbox, c.Profile, c.Codex, c.Orca = p.Sandbox, p.CodexProfile, p.Codex, p.Orca
	c.ProjectSteps = p.ProjectSteps
	return c
}

func validateExecutionProfiles(c Config) error {
	ids := map[string]bool{ProjectExecutionProfileID: true}
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
		if p.ReasoningEffort != "" && !validReasoningEffort(p.ReasoningEffort) {
			return fmt.Errorf("execution profile %s has invalid reasoning effort", p.ID)
		}
		if c.WorkflowProtocol == WorkerFlowVersion && p.ProjectSteps != nil {
			return fmt.Errorf("execution profile %s cannot configure project_steps in worker-flow-v1", p.ID)
		}
		if c.WorkflowProtocol != WorkerFlowVersion && p.ProjectSteps == nil {
			return fmt.Errorf("execution profile %s requires fixed project_steps", p.ID)
		}
		if c.WorkflowProtocol == WorkerFlowVersion && p.Agent != "codex" {
			return fmt.Errorf("execution profile %s requires codex for a resumable worker session", p.ID)
		}
		selected := c.ApplyExecutionProfile(p.effective())
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
		if c.WorkflowProtocol != WorkerFlowVersion {
			if _, err := selected.validateNativeWorkflow(); err != nil {
				return fmt.Errorf("execution profile %s: %w", p.ID, err)
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
	case backend == string(BackendLocal) && protocol == string(ProtocolLocalProcess):
		return "local"
	case backend == string(BackendOrca) && protocol == string(ProtocolOrcaTerminalV2):
		return "orca"
	case backend == string(BackendOrca) && protocol == string(ProtocolOrcaSupervised):
		return "orca_supervised"
	default:
		return ""
	}
}
