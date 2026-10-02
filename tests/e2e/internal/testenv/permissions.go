package testenv

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

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
	if profile == nil || profile.ExecutionPolicy.Filesystem.WriteScope != "host" || profile.ExecutionPolicy.Network != "allow" || resources == nil || len(resources.WriteRoots) != 0 {
		p.Scenario.t.Fatal("execution did not bind explicit full-access policy")
	}
	settings, err := frozenCodexSettings(snapshot)
	p.Scenario.Must(err)
	compiled, err := codexworker.CompilePolicy(profile.ExecutionPolicy, "windows", profile.Host, profile.Agent, settings.WindowsSandbox, settings.Profile, settings.Args)
	p.Scenario.Must(err)
	e := snapshot.Domain.Executions[0]
	bound, err := codexworker.DecodePolicyResourceEnvelope(resources.ImplementationData, codexworker.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest})
	p.Scenario.Must(err)
	if compiled.Sandbox != "danger-full-access" || compiled.ApprovalPolicy != "never" || compiled.WindowsSandbox != "" || bound.Contract != codexworker.FullAccessPolicyContract {
		p.Scenario.t.Fatal("native codec did not bind full-access settings")
	}
}

func frozenCodexSettings(snapshot Snapshot) (codexworker.ProfileConfig, error) {
	if len(snapshot.Domain.WorkflowRuns) != 1 || len(snapshot.Domain.Executions) != 1 || snapshot.Domain.WorkflowRuns[0].ExecutionProfile == nil {
		return codexworker.ProfileConfig{}, errors.New("missing fixed execution profile")
	}
	profile := snapshot.Domain.WorkflowRuns[0].ExecutionProfile
	for _, data := range profile.ImplementationData {
		if data.ImplementationKey == codexworker.ProfileCodecKey {
			if err := data.ValidateProfileIdentity(snapshot.Domain.Executions[0].ProjectIdentity, profile.ID, profile.AcceptedConfigVersion); err != nil {
				return codexworker.ProfileConfig{}, err
			}
			return codexworker.DecodeProfileConfig(data.Payload)
		}
	}
	return codexworker.ProfileConfig{}, errors.New("missing Codex profile envelope")
}

func ExecutionSession(e contracts.ExecutionRecord) (string, error) {
	if e.Protocol == string(executionapi.ProtocolOrcaTerminalV2) && e.Process != nil {
		return orcaworker.SessionID(*e.Process), nil
	}
	for _, data := range e.ImplementationData {
		if data.ImplementationKey == codexworker.ExecutionBindingCodecKey {
			binding, err := codexworker.DecodeAppServerBindingEnvelope(data, codexworker.AppServerBindingIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest})
			return binding.SessionID, err
		}
	}
	return "", errors.New("missing native session binding")
}

func nativeOrcaState(e contracts.ExecutionRecord) (orcaworker.NativeExecutionState, error) {
	if e.Process == nil {
		return orcaworker.NativeExecutionState{}, errors.New("missing native Orca process")
	}
	for _, data := range e.ImplementationData {
		if data.ImplementationKey == orcaworker.ExecutionStateCodecKey {
			return orcaworker.DecodeExecutionStateEnvelope(data, orcaworker.ExecutionStateIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest, Protocol: executionapi.ExecutionProtocol(e.Protocol), Process: *e.Process})
		}
	}
	return orcaworker.NativeExecutionState{}, errors.New("missing native Orca state envelope")
}
