package host

import (
	"context"
	"os"
	"strings"

	execution "github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func queryHostRequirementTool(_ context.Context, c execution.Config, id string) executionbackend.RequirementEvidence {
	e := executionbackend.RequirementEvidence{ID: id, Site: "target", Target: execution.TargetFor(c), Status: executionbackend.RequirementUnknown, Source: "neutral:declared-tool-fact", CheckedAt: now()}
	if id == "git" {
		if c.Git.Executable == "git" || len(c.Git.Args) > 0 {
			e.Status, e.Version = executionbackend.RequirementPresent, "fixture-tool-v1"
			return e
		}
		if _, err := os.Stat(c.Git.Executable); err != nil {
			e.Status = executionbackend.RequirementMissing
		}
		return e
	}
	if id == "go" || id == "python" {
		e.Status, e.Version = executionbackend.RequirementPresent, "fixture-tool-v1"
		if strings.EqualFold(c.PR.TokenEnv, "PATH") {
			e.Status, e.Version = executionbackend.RequirementMissing, ""
		}
	}
	return e
}

func queryRemoteRequirementTool(_ context.Context, c execution.Config, id string) executionbackend.RequirementEvidence {
	return executionbackend.RequirementEvidence{ID: id, Site: "target", Target: execution.TargetFor(c), Status: executionbackend.RequirementUnknown, Source: "neutral:unavailable-target-tool", CheckedAt: now()}
}

func capabilityReport(backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec, version string) executionbackend.CapabilityReport {
	names := []string{"start", "observe", "stop", "detach_observation", "operation_reconciliation", "workspace_inspection", "output_reading", "output_turns", "output_final_answer", "resource_settlement"}
	caps := make([]executionbackend.Capability, 0, len(names))
	for _, name := range names {
		caps = append(caps, executionbackend.Capability{Name: name, Status: executionbackend.CapabilityNotTested})
	}
	return executionbackend.CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: now(), Capabilities: caps}
}
