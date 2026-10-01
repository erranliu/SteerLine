package execution

import (
	"testing"
	"time"
)

func TestRequirementsMatchBoundEvidence(t *testing.T) {
	key := CapabilityReport{Version: 1, Backend: BackendLocal, Protocol: ProtocolLocalCodexAppServer, Target: ExecutionTarget{ID: "target"}, Agent: AgentSpec{Name: "codex"}, PlatformVersion: "codex-cli 0.159.2", CheckedAt: time.Now()}
	report := key
	report.Capabilities = []Capability{{Name: "workspace_inspection", Status: CapabilityVerified, Evidence: "native-evidence-reference"}}
	r := ExecutionRequirements{Capabilities: []string{"workspace_inspection"}, Tools: []ToolRequirement{{ID: "git", ExactVersion: "2.51.0"}}, Credentials: []CredentialRequirement{{ID: "pr.token_env", Site: "service"}}}
	e := RequirementsEvaluation{Version: 1, Backend: key.Backend, Protocol: key.Protocol, Target: key.Target, Agent: key.Agent, CheckedAt: time.Now(), Capabilities: &report, Tools: []RequirementEvidence{{ID: "git", Site: "target", Target: key.Target, Version: "2.51.0", Status: RequirementPresent, Source: "frozen-git", CheckedAt: time.Now()}}, Credentials: []RequirementEvidence{{ID: "pr.token_env", Site: "service", Target: key.Target, Status: RequirementPresent, Source: "environment:reference", CheckedAt: time.Now()}}}
	if err := r.Match(e, key); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*RequirementsEvaluation)
	}{
		{"target", func(e *RequirementsEvaluation) { e.Target.ID = "other" }},
		{"agent", func(e *RequirementsEvaluation) { e.Agent.Name = "other" }},
		{"protocol", func(e *RequirementsEvaluation) { e.Protocol = ProtocolLocalProcess }},
		{"schema", func(e *RequirementsEvaluation) { e.Version = 2 }},
		{"platform-version", func(e *RequirementsEvaluation) { e.Capabilities.PlatformVersion = "other" }},
		{"agent-version", func(e *RequirementsEvaluation) { e.Capabilities.AgentVersion = "other" }},
		{"unsupported", func(e *RequirementsEvaluation) { e.Capabilities.Capabilities[0].Status = CapabilityUnsupported }},
		{"not-tested", func(e *RequirementsEvaluation) { e.Capabilities.Capabilities[0].Status = CapabilityNotTested }},
		{"no-evidence", func(e *RequirementsEvaluation) { e.Capabilities.Capabilities[0].Evidence = "" }},
		{"tool-missing", func(e *RequirementsEvaluation) { e.Tools[0].Status = RequirementMissing }},
		{"tool-unknown", func(e *RequirementsEvaluation) { e.Tools[0].Status = RequirementUnknown }},
		{"tool-version", func(e *RequirementsEvaluation) { e.Tools[0].Version = "2.52.0" }},
		{"tool-target", func(e *RequirementsEvaluation) { e.Tools[0].Target.ID = "center" }},
		{"credential-missing", func(e *RequirementsEvaluation) { e.Credentials[0].Status = RequirementMissing }},
		{"credential-unknown", func(e *RequirementsEvaluation) { e.Credentials[0].Status = RequirementUnknown }},
		{"credential-site", func(e *RequirementsEvaluation) { e.Credentials[0].Site = "target" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := e.Clone()
			tc.mutate(&copy)
			if r.Match(copy, key) == nil {
				t.Fatal("mismatched evidence accepted")
			}
		})
	}
	if err := r.Match(e, key); err != nil {
		t.Fatal("mutations leaked into original evidence", err)
	}
	r.Tools[0].ExactVersion = ""
	e.Tools[0].Version = "different"
	if err := r.Match(e, key); err != nil {
		t.Fatal("unpinned tool rejected", err)
	}
}
func TestRequirementsRejectUnregisteredAndDuplicateItems(t *testing.T) {
	for _, r := range []ExecutionRequirements{
		{Capabilities: []string{"unknown"}}, {Capabilities: []string{"start", "start"}},
		{Tools: []ToolRequirement{{ID: "shell"}}}, {Tools: []ToolRequirement{{ID: "go"}, {ID: "go"}}},
		{Tools: []ToolRequirement{{ID: "git", ExactVersion: " 2.5"}}},
		{Credentials: []CredentialRequirement{{ID: "pr.token_env", Site: "target"}}},
		{Credentials: []CredentialRequirement{{ID: "login", Site: "service"}}},
	} {
		if r.Validate() == nil {
			t.Fatalf("invalid requirements accepted: %+v", r)
		}
	}
	if (ExecutionRequirements{}).Validate() != nil || !(ExecutionRequirements{}).Empty() {
		t.Fatal("empty requirements rejected")
	}
	for _, id := range []string{"git", "go", "python"} {
		if (ExecutionRequirements{Tools: []ToolRequirement{{ID: id}}}).Validate() != nil {
			t.Fatalf("registered tool %s rejected", id)
		}
	}
}
