package evidence

import (
	"errors"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func RemoteModelSelection(domain reconstruction.Snapshot) (ModelSettings, error) {
	if len(domain.WorkflowRuns) != 1 || len(domain.Executions) != 1 || domain.WorkflowRuns[0].ExecutionProfile == nil {
		return ModelSettings{}, errors.New("remote selection needs one frozen execution")
	}
	profile := domain.WorkflowRuns[0].ExecutionProfile
	for _, data := range profile.ImplementationData {
		if data.ImplementationKey != remote.ProfileCodecKey {
			continue
		}
		if data.CodecVersion != remote.ProfileCodecVersion {
			return ModelSettings{}, errors.New("remote profile codec version mismatch")
		}
		if err := data.ValidateProfileIdentity(domain.Executions[0].ProjectIdentity, profile.ID, profile.AcceptedConfigVersion); err != nil {
			return ModelSettings{}, err
		}
		settings, err := remote.DecodeProfileConfig(data.Payload)
		return ModelSettings{Model: settings.Model, Effort: settings.ReasoningEffort}, err
	}
	return ModelSettings{}, errors.New("remote Orca profile missing")
}
