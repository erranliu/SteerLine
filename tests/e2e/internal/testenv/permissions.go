package testenv

import "github.com/erranliu/SteerLine/internal/steerline/codexworker"

func fullAccessPolicy() map[string]any {
	return map[string]any{"filesystem": map[string]any{"read_scope": "host", "write_scope": "host", "git_metadata_write": false}, "network": "allow", "escalation": "deny"}
}

// AssertFullAccess reads the real frozen execution and launch resource receipt.
// A configured intent alone does not prove which policy the worker used.
func (p *Project) AssertFullAccess(snapshot Snapshot) {
	p.Scenario.t.Helper()
	if len(snapshot.Domain.WorkflowRuns) != 1 || len(snapshot.Domain.Executions) != 1 {
		p.Scenario.t.Fatal("full-access evidence needs one bound workflow/execution")
	}
	profile := snapshot.Domain.WorkflowRuns[0].ExecutionProfile
	resources := snapshot.Domain.Executions[0].PolicyResources
	if profile == nil || profile.Sandbox != "danger-full-access" || profile.WindowsSandbox != "" || profile.ApprovalPolicy != "never" || profile.ExecutionPolicy.Filesystem.WriteScope != "host" || profile.ExecutionPolicy.Network != "allow" || resources == nil || resources.Contract != codexworker.FullAccessPolicyContract || len(resources.WriteRoots) != 0 {
		p.Scenario.t.Fatal("execution did not bind explicit full-access policy")
	}
}
