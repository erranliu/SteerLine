// Package evidence verifies original product receipts through their owning
// codecs. Native implementation types stay inside this test adapter.
package evidence

import (
	"errors"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

type ModelSettings struct{ Model, Effort string }

func ModelSelection(domain reconstruction.Snapshot) (ModelSettings, error) {
	settings, err := codexSettings(domain)
	return ModelSettings{Model: settings.Model, Effort: settings.ReasoningEffort}, err
}

func codexSettings(domain reconstruction.Snapshot) (codexworker.ProfileConfig, error) {
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

// FixedNativeLaunch checks frozen authority and the identity-bound launch receipt.
func FixedNativeLaunch(domain reconstruction.Snapshot) error {
	if len(domain.WorkflowRuns) != 1 || len(domain.Executions) != 1 {
		return errors.New("native launch evidence needs one bound workflow/execution")
	}
	profile := domain.WorkflowRuns[0].ExecutionProfile
	e := domain.Executions[0]
	resources := e.PolicyResources
	if profile == nil || resources == nil || len(resources.WriteRoots) == 0 {
		return errors.New("execution did not bind fixed native launch roots")
	}
	_, err := codexSettings(domain)
	if err != nil {
		return err
	}
	compiled, err := codexworker.NativePermissions("windows", profile.Host, profile.Agent)
	if err != nil {
		return err
	}
	bound, err := codexworker.DecodePolicyResourceEnvelope(resources.ImplementationData, codexworker.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest})
	if err != nil {
		return err
	}
	if compiled.Sandbox != "workspace-write" || compiled.ApprovalPolicy != "on-request" || compiled.ApprovalsReviewer != "auto_review" || compiled.WindowsSandbox != "unelevated" || bound.Contract != codexworker.PolicyContract {
		return errors.New("native codec did not bind fixed native settings")
	}
	return nil
}

func Session(e executionmodel.ExecutionRecord) (string, error) {
	if e.Protocol == string(executionbackend.ProtocolOrcaSupervised) && e.Process != nil {
		return remote.SessionID(*e.Process), nil
	}
	if e.Protocol == string(executionbackend.ProtocolOrcaTerminalV2) && e.Process != nil {
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

func OrcaReadiness(e executionmodel.ExecutionRecord) error {
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

func OrcaExplicitEmptyArgs(domain reconstruction.Snapshot) error {
	if len(domain.WorkflowRuns) != 1 || len(domain.Executions) != 1 || domain.WorkflowRuns[0].ExecutionProfile == nil {
		return errors.New("explicit Orca args evidence needs one fixed execution")
	}
	profile := domain.WorkflowRuns[0].ExecutionProfile
	for _, data := range profile.ImplementationData {
		if data.ImplementationKey != orcaworker.ProfileCodecKey {
			continue
		}
		if err := data.ValidateProfileIdentity(domain.Executions[0].ProjectIdentity, profile.ID, profile.AcceptedConfigVersion); err != nil {
			return err
		}
		settings, err := orcaworker.DecodeProfileConfig(data.Payload)
		if err != nil {
			return err
		}
		if settings.Args == nil || len(settings.Args) != 0 {
			return errors.New("Orca smoke did not freeze explicit args=[]")
		}
		return nil
	}
	return errors.New("missing frozen Orca profile")
}

func orcaState(e executionmodel.ExecutionRecord) (orcaworker.NativeExecutionState, error) {
	if e.Process == nil {
		return orcaworker.NativeExecutionState{}, errors.New("missing native Orca process")
	}
	for _, data := range e.ImplementationData {
		if data.ImplementationKey == orcaworker.ExecutionStateCodecKey {
			return orcaworker.DecodeExecutionStateEnvelope(data, orcaworker.ExecutionStateIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest, Protocol: executionbackend.ExecutionProtocol(e.Protocol), Process: *e.Process})
		}
	}
	return orcaworker.NativeExecutionState{}, errors.New("missing native Orca state envelope")
}
