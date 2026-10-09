// Package evidence verifies original product receipts through their owning
// codecs. Native implementation types stay inside this test adapter.
package evidence

import (
	"encoding/json"
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
	if len(domain.Executions) == 1 && domain.Executions[0].Protocol == string(executionbackend.ProtocolOrcaTerminalV2) {
		settings, err := orcaSettings(domain)
		return ModelSettings{Model: settings.Model, Effort: settings.ReasoningEffort}, err
	}
	settings, err := codexSettings(domain)
	return ModelSettings{Model: settings.Model, Effort: settings.ReasoningEffort}, err
}

func orcaSettings(domain reconstruction.Snapshot) (orcaworker.ProfileConfig, error) {
	if len(domain.WorkflowRuns) != 1 || len(domain.Executions) != 1 || domain.WorkflowRuns[0].ExecutionProfile == nil {
		return orcaworker.ProfileConfig{}, errors.New("missing fixed Orca execution profile")
	}
	profile := domain.WorkflowRuns[0].ExecutionProfile
	for _, data := range profile.ImplementationData {
		if data.ImplementationKey == orcaworker.ProfileCodecKey {
			if err := data.ValidateProfileIdentity(domain.Executions[0].ProjectIdentity, profile.ID, profile.AcceptedConfigVersion); err != nil {
				return orcaworker.ProfileConfig{}, err
			}
			return orcaworker.DecodeProfileConfig(data.Payload)
		}
	}
	return orcaworker.ProfileConfig{}, errors.New("missing native Orca profile envelope")
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
	if e.Protocol == string(executionbackend.ProtocolOrcaTerminalV2) {
		settings, err := orcaSettings(domain)
		if err != nil {
			return err
		}
		state, err := orcaState(e)
		if err != nil || e.Process == nil || e.Process.Terminal == "" || e.Process.Incarnation == "" || len(state.Turns) == 0 {
			return errors.New("native Orca original launch identity missing")
		}
		for _, op := range domain.Operations {
			if op.Kind != "start_worker_turn" || op.LifecycleIdentity == nil || op.LifecycleIdentity.ExecutionID != e.ExecutionID || op.LifecycleIdentity.Ref.AttemptID != e.CurrentAttemptID {
				continue
			}
			var receipt struct {
				TaskID     string `json:"taskId"`
				DispatchID string `json:"dispatchId"`
				Launch     struct {
					Effective struct {
						Agent  string `json:"agent"`
						Model  string `json:"model"`
						Effort string `json:"effort"`
					} `json:"effective"`
				} `json:"launch"`
			}
			if json.Unmarshal(op.Receipt, &receipt) == nil && receipt.TaskID == state.Turns[0].TaskID && receipt.DispatchID == state.Turns[0].DispatchID && receipt.Launch.Effective.Agent == profile.Agent && receipt.Launch.Effective.Model == settings.Model && receipt.Launch.Effective.Effort == settings.ReasoningEffort {
				return nil
			}
		}
		return errors.New("native Orca first input lacks actual frozen agent/model/effort launch receipt")
	}
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

func OrcaNativeBusiness(e executionmodel.ExecutionRecord) error {
	state, err := orcaState(e)
	if err != nil {
		return err
	}
	if len(state.Turns) == 0 || uint64(len(state.Turns)) != e.TurnNumber || e.Process == nil || e.CurrentTurnID == "" || e.CurrentTurnID != e.LastStoppedTurnID {
		return errors.New("original native business turn is not settled")
	}
	turn := state.Turns[len(state.Turns)-1]
	if turn.Purpose != "business" || !turn.Settled || turn.Outcome != "succeeded" || turn.TurnID != e.CurrentTurnID || turn.TaskID != e.Process.Task || turn.DispatchID != e.Process.Dispatch {
		return errors.New("original native Task/Dispatch/business outcome differs")
	}
	return nil
}

func OrcaStagedContinuity(domain reconstruction.Snapshot) error {
	if len(domain.Executions) != 1 || len(domain.WorkflowRuns) != 1 {
		return errors.New("native Orca staged continuity lacks one original execution")
	}
	e, w := domain.Executions[0], domain.WorkflowRuns[0]
	state, err := orcaState(e)
	if err != nil || len(state.Turns) != 3 || len(w.StageResults) != 3 {
		return errors.New("native Orca did not settle exactly three original inputs")
	}
	for i, turn := range state.Turns {
		result := w.StageResults[i]
		if !turn.Started || !turn.Settled || turn.Outcome != "succeeded" || turn.Purpose != "business" || turn.TurnID != result.CommandID || turn.DispatchID != result.FinalNativeTurnID || result.FinalSource != "orca-output-material" || turn.TaskID == "" || result.SessionID != e.SessionID {
			return errors.New("native Orca original Task/Dispatch/final continuity changed")
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
