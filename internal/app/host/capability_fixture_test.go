package host

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func capabilityReport(backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec, version string) executionbackend.CapabilityReport {
	names := []string{"start", "observe", "stop", "detach_observation", "operation_reconciliation", "workspace_inspection", "output_reading", "output_turns", "output_final_answer", "resource_settlement"}
	caps := make([]executionbackend.Capability, 0, len(names))
	for _, name := range names {
		caps = append(caps, executionbackend.Capability{Name: name, Status: executionbackend.CapabilityNotTested})
	}
	return executionbackend.CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: now(), Capabilities: caps}
}
