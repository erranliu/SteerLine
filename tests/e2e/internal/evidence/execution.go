// Package evidence verifies original product receipts through their owning
// codecs. Native implementation types stay inside this test adapter.
package evidence

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

type ModelSettings struct{ Model, Effort string }

func ModelSelection(domain contracts.DomainSnapshot) (ModelSettings, error) {
	settings, err := codexSettings(domain)
	return ModelSettings{Model: settings.Model, Effort: settings.ReasoningEffort}, err
}

func codexSettings(domain contracts.DomainSnapshot) (codexworker.ProfileConfig, error) {
	if len(domain.WorkflowRuns) != 1 || len(domain.Executions) != 1 || domain.WorkflowRuns[0].ExecutionProfile == nil {
		return codexworker.ProfileConfig{}, errors.New("missing fixed execution profile")
	}
	profile := domain.WorkflowRuns[0].ExecutionProfile
	for _, data := range profile.ImplementationData {
		if data.ImplementationKey == codexworker.ProfileCodecKey {
			if err := data.ValidateProfileIdentity(domain.Executions[0].ProjectIdentity, profile.ID, profile.AcceptedConfigVersion); err != nil {
				return codexworker.ProfileConfig{}, err
			}
			return codexworker.DecodeProfileConfig(data.Payload)
		}
	}
	return codexworker.ProfileConfig{}, errors.New("missing Codex profile envelope")
}

// FullAccess checks frozen authority and the identity-bound launch receipt.
func FullAccess(domain contracts.DomainSnapshot) error {
	if len(domain.WorkflowRuns) != 1 || len(domain.Executions) != 1 {
		return errors.New("full-access evidence needs one bound workflow/execution")
	}
	profile := domain.WorkflowRuns[0].ExecutionProfile
	e := domain.Executions[0]
	resources := e.PolicyResources
	if profile == nil || profile.ExecutionPolicy.Filesystem.WriteScope != "host" || profile.ExecutionPolicy.Network != "allow" || resources == nil || len(resources.WriteRoots) != 0 {
		return errors.New("execution did not bind explicit full-access policy")
	}
	settings, err := codexSettings(domain)
	if err != nil {
		return err
	}
	compiled, err := codexworker.CompilePolicy(profile.ExecutionPolicy, "windows", profile.Host, profile.Agent, settings.WindowsSandbox, settings.Profile, settings.Args)
	if err != nil {
		return err
	}
	bound, err := codexworker.DecodePolicyResourceEnvelope(resources.ImplementationData, codexworker.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest})
	if err != nil {
		return err
	}
	if compiled.Sandbox != "danger-full-access" || compiled.ApprovalPolicy != "never" || compiled.WindowsSandbox != "" || bound.Contract != codexworker.FullAccessPolicyContract {
		return errors.New("native codec did not bind full-access settings")
	}
	return nil
}

func Session(e contracts.ExecutionRecord) (string, error) {
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

func OrcaReadiness(e contracts.ExecutionRecord) error {
	state, err := orcaState(e)
	if err != nil {
		return err
	}
	if len(state.Turns) != 2 || e.Environment == nil || !e.Environment.Ready || e.Environment.Preparation == nil || !e.Environment.Preparation.Verified {
		return errors.New("real Orca readiness/business continuity missing")
	}
	for _, turn := range state.Turns {
		if !turn.Settled || turn.Outcome != "succeeded" {
			return errors.New("real Orca turn did not succeed")
		}
	}
	return nil
}

func orcaState(e contracts.ExecutionRecord) (orcaworker.NativeExecutionState, error) {
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
