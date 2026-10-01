package execution

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// ExecutionRequirements adds necessary conditions; it never grants permission.
type ExecutionRequirements struct {
	Capabilities []string                `json:"capabilities,omitempty"`
	Tools        []ToolRequirement       `json:"tools,omitempty"`
	Credentials  []CredentialRequirement `json:"credentials,omitempty"`
}
type ToolRequirement struct {
	ID           string `json:"id"`
	ExactVersion string `json:"exact_version,omitempty"`
}
type CredentialRequirement struct {
	ID   string `json:"id"`
	Site string `json:"site"`
}
type RequirementStatus string

const (
	RequirementPresent RequirementStatus = "present"
	RequirementMissing RequirementStatus = "missing"
	RequirementUnknown RequirementStatus = "unknown"
)

// RequirementEvidence contains references and normalized versions, never secrets
// or arbitrary command output. Target is the configured target, not a worktree.
type RequirementEvidence struct {
	ID        string            `json:"id"`
	Site      string            `json:"site"`
	Target    ExecutionTarget   `json:"target"`
	Status    RequirementStatus `json:"status"`
	Version   string            `json:"version,omitempty"`
	Source    string            `json:"source"`
	CheckedAt time.Time         `json:"checked_at"`
}
type RequirementsEvaluation struct {
	Version      int                   `json:"version"`
	Backend      BackendKind           `json:"backend"`
	Protocol     ExecutionProtocol     `json:"protocol"`
	Target       ExecutionTarget       `json:"target"`
	Agent        AgentSpec             `json:"agent"`
	CheckedAt    time.Time             `json:"checked_at"`
	Capabilities *CapabilityReport     `json:"capabilities,omitempty"`
	Tools        []RequirementEvidence `json:"tools,omitempty"`
	Credentials  []RequirementEvidence `json:"credentials,omitempty"`
}

func RegisteredCapability(id string) bool {
	switch id {
	case "start", "observe", "stop", "detach_observation", "operation_reconciliation", "workspace_inspection", "output_reading", "output_turns", "output_final_answer", "resource_settlement", "message_inbox", "remote_cancellation":
		return true
	}
	return false
}
func (r ExecutionRequirements) Empty() bool {
	return len(r.Capabilities)+len(r.Tools)+len(r.Credentials) == 0
}
func (r ExecutionRequirements) Clone() ExecutionRequirements {
	r.Capabilities = slices.Clone(r.Capabilities)
	r.Tools = slices.Clone(r.Tools)
	r.Credentials = slices.Clone(r.Credentials)
	return r
}
func (e RequirementsEvaluation) Clone() RequirementsEvaluation {
	e.Tools = slices.Clone(e.Tools)
	e.Credentials = slices.Clone(e.Credentials)
	if e.Capabilities != nil {
		c := *e.Capabilities
		c.Capabilities = slices.Clone(c.Capabilities)
		e.Capabilities = &c
	}
	return e
}
func (r ExecutionRequirements) Validate() error {
	seen := map[string]bool{}
	for _, id := range r.Capabilities {
		if !RegisteredCapability(id) || seen["cap:"+id] {
			return fmt.Errorf("execution_requirements: unknown or duplicate capability %q", id)
		}
		seen["cap:"+id] = true
	}
	for _, tool := range r.Tools {
		if tool.ID != "git" && tool.ID != "go" && tool.ID != "python" || seen["tool:"+tool.ID] {
			return fmt.Errorf("execution_requirements: unknown or duplicate tool %q", tool.ID)
		}
		if tool.ExactVersion != strings.TrimSpace(tool.ExactVersion) || strings.ContainsAny(tool.ExactVersion, "\r\n\x00") {
			return fmt.Errorf("execution_requirements: invalid exact_version for %s", tool.ID)
		}
		seen["tool:"+tool.ID] = true
	}
	for _, ref := range r.Credentials {
		// First built-in reference. Additional references need a registered
		// provider/agent checker; a service environment cannot prove target auth.
		if ref.ID != "pr.token_env" || ref.Site != "service" || seen["auth:"+ref.Site+":"+ref.ID] {
			return fmt.Errorf("execution_requirements: unknown credential or unsupported site %s/%s", ref.Site, ref.ID)
		}
		seen["auth:"+ref.Site+":"+ref.ID] = true
	}
	return nil
}

// Match requires a current, exactly bound version key for authenticated reports.
// It does not interpret native evidence or replace mandatory flow/policy checks.
func (r ExecutionRequirements) Match(e RequirementsEvaluation, key CapabilityReport) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if e.Version != 1 || e.Backend != key.Backend || e.Protocol != key.Protocol || e.Target != key.Target || e.Agent != key.Agent || e.CheckedAt.IsZero() || key.Target.ID == "" || key.Agent.Name == "" {
		return fmt.Errorf("execution requirements evaluation identity/version mismatch")
	}
	if len(r.Capabilities) > 0 {
		c := e.Capabilities
		if c == nil || key.Version != 1 || c.Version != key.Version || c.Backend != key.Backend || c.Protocol != key.Protocol || c.Target != key.Target || c.Agent != key.Agent || c.PlatformVersion == "" || c.PlatformVersion != key.PlatformVersion || c.AgentVersion != key.AgentVersion || c.CenterVersion != key.CenterVersion || c.ExpectedAgentVersion != key.ExpectedAgentVersion || c.CheckedAt.IsZero() {
			return fmt.Errorf("execution requirements capability target/version mismatch")
		}
		for _, id := range r.Capabilities {
			count := 0
			for _, cap := range c.Capabilities {
				if cap.Name == id {
					if cap.Status != CapabilityVerified || cap.Evidence == "" {
						return fmt.Errorf("required capability %s is unverified or unsupported", id)
					}
					count++
				}
			}
			if count != 1 {
				return fmt.Errorf("required capability %s is unverified or unsupported", id)
			}
		}
	}
	for _, tool := range r.Tools {
		if err := matchEvidence(e.Tools, tool.ID, "target", key.Target, tool.ExactVersion); err != nil {
			return err
		}
	}
	for _, ref := range r.Credentials {
		if err := matchEvidence(e.Credentials, ref.ID, ref.Site, key.Target, ""); err != nil {
			return err
		}
	}
	return nil
}
func matchEvidence(values []RequirementEvidence, id, site string, target ExecutionTarget, exact string) error {
	count := 0
	for _, v := range values {
		if v.ID != id || v.Site != site {
			continue
		}
		count++
		if v.Target != target || v.CheckedAt.IsZero() || v.Source == "" || v.Status != RequirementPresent || exact != "" && (v.Version == "" || v.Version != exact) {
			return fmt.Errorf("required %s/%s is missing, unknown, or has a target/version mismatch", site, id)
		}
	}
	if count != 1 {
		return fmt.Errorf("required %s/%s has no unique evidence", site, id)
	}
	return nil
}
