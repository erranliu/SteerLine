// Package evidence verifies original product receipts through their owning
// codecs. Native implementation types stay inside this test adapter.
package evidence

import (
	"encoding/json"
	"errors"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	transport "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

type ModelSettings struct{ Model, Effort string }

func ModelSelection(domain reconstruction.Snapshot) (ModelSettings, error) {
	if len(domain.Executions) == 1 && domain.Executions[0].Protocol == string(executionbackend.ProtocolOrcaTerminalV3) {
		return ModelSettings{}, errors.New("Orca launcher model settings are host-owned")
	}
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
	if e.Protocol == string(executionbackend.ProtocolOrcaTerminalV3) {
		if profile == nil || e.Process == nil || e.Process.Terminal == "" || e.Process.RuntimeID == "" || e.Process.Incarnation == "" || e.Process.WorktreeID == "" {
			return errors.New("missing bound Orca terminal")
		}
		for _, op := range domain.Operations {
			if op.Kind != "start_worker_turn" || op.LifecycleIdentity == nil || op.LifecycleIdentity.ExecutionID != e.ExecutionID || op.LifecycleIdentity.ResourceGeneration != e.Generation || op.LifecycleIdentity.ConfigurationDigest != e.ConfigurationDigest || op.LifecycleIdentity.Ref.AttemptID != e.CurrentAttemptID {
				continue
			}
			var receipt executionbackend.LifecycleReceipt
			if json.Unmarshal(op.Receipt, &receipt) == nil && receipt.Process != nil && *receipt.Process == *e.Process && receipt.State.InputAccepted {
				return nil
			}
		}
		return errors.New("missing original terminal input receipt")
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

	if e.Protocol == string(executionbackend.ProtocolOrcaTerminalV3) && e.Process != nil {
		return e.Process.Terminal, nil
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
	if e.Protocol != string(executionbackend.ProtocolOrcaTerminalV3) || e.Process == nil || e.CurrentTurnID == "" || e.CurrentTurnID != e.LastStoppedTurnID {
		return errors.New("Orca original input is not settled")
	}
	return nil
}
func OrcaStagedContinuity(domain reconstruction.Snapshot) error {
	if len(domain.Executions) != 1 || len(domain.WorkflowRuns) != 1 {
		return errors.New("missing original workflow")
	}
	e, w := domain.Executions[0], domain.WorkflowRuns[0]
	if e.TurnNumber != 3 || len(w.StageResults) != 3 {
		return errors.New("missing three settled inputs")
	}
	for _, result := range w.StageResults {
		if !result.Succeeded || result.SessionID != e.SessionID || result.FinalSource != "steerline.worker.terminal-result.v1" || result.FinalNativeTurnID != "" {
			return errors.New("invalid terminal return provenance")
		}
	}
	return nil
}
func OrcaExplicitEmptyArgs(domain reconstruction.Snapshot) error {
	if len(domain.WorkflowRuns) != 1 || domain.WorkflowRuns[0].ExecutionProfile == nil {
		return errors.New("missing fixed profile")
	}
	for _, d := range domain.WorkflowRuns[0].ExecutionProfile.ImplementationData {
		if d.ImplementationKey == transport.ProfileKey {
			p, err := transport.DecodeProfile(d.Payload)
			if err != nil {
				return err
			}
			if len(p.Args) != 0 {
				return errors.New("expected empty CLI prefix args")
			}
			return nil
		}
	}
	return errors.New("missing Orca profile")
}
