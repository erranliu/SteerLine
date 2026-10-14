package execution

import (
	"context"
	"slices"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func clonePublicationSettings(p executionmodel.PublicationSettings) executionmodel.PublicationSettings {
	p.Provider.Query.Args = slices.Clone(p.Provider.Query.Args)
	p.Git.Args = slices.Clone(p.Git.Args)
	p.AuthenticationRefs = slices.Clone(p.AuthenticationRefs)
	return p
}

func verifyClaimConfigFiles(verifier ConfigVerifier) error {
	if verifier == nil {
		return nil
	}
	return verifier.VerifyCurrentConfig()
}

func (s *Service) resumeRegisteredWorkerFlow(ctx context.Context, task *taskmodel.TaskRecord, run *reconstructiontypes.RunView, entry ExecutionRegistration) (bool, error) {
	if _, err := s.workerRecords(run); err != nil {
		return false, err
	}
	return s.resumeLifecycleWorkerFlow(ctx, task, run)
}
