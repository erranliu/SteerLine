package steerline

import (
	"context"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"os"
	"strings"
)

func queryHostRequirementTool(_ context.Context, c Config, id string) executionapi.RequirementEvidence {
	e := executionapi.RequirementEvidence{ID: id, Site: "target", Target: targetFor(c), Status: executionapi.RequirementUnknown, Source: "neutral:declared-tool-fact", CheckedAt: now()}
	if id == "git" {
		if c.Git.Executable == "git" || len(c.Git.Args) > 0 {
			e.Status, e.Version = executionapi.RequirementPresent, "fixture-tool-v1"
			return e
		}
		if _, err := os.Stat(c.Git.Executable); err != nil {
			e.Status = executionapi.RequirementMissing
		}
		return e
	}
	if id == "go" || id == "python" {
		e.Status, e.Version = executionapi.RequirementPresent, "fixture-tool-v1"
		if strings.EqualFold(c.PR.TokenEnv, "PATH") {
			e.Status, e.Version = executionapi.RequirementMissing, ""
		}
	}
	return e
}
func queryRemoteRequirementTool(_ context.Context, c Config, id string) executionapi.RequirementEvidence {
	return executionapi.RequirementEvidence{ID: id, Site: "target", Target: targetFor(c), Status: executionapi.RequirementUnknown, Source: "neutral:unavailable-target-tool", CheckedAt: now()}
}
func queryNativeRequirementTool(ctx context.Context, c Config, id string) executionapi.RequirementEvidence {
	return queryRemoteRequirementTool(ctx, c, id)
}
func capabilityReport(backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, target executionapi.ExecutionTarget, agent executionapi.AgentSpec, version string) executionapi.CapabilityReport {
	names := []string{"start", "observe", "stop", "detach_observation", "operation_reconciliation", "workspace_inspection", "output_reading", "output_turns", "output_final_answer", "resource_settlement"}
	caps := make([]executionapi.Capability, 0, len(names))
	for _, name := range names {
		caps = append(caps, executionapi.Capability{Name: name, Status: executionapi.CapabilityNotTested})
	}
	return executionapi.CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: now(), Capabilities: caps}
}
