package execution

import (
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (s *Service) commitWorkerBinding(run *contracts.Run, ref executionbackend.ExecutionRef) error {
	return s.commitWorkerAttached(run, workerCommitAttachment{Binding: &ref})
}

func (s *Service) commitExecutionOperation(run *contracts.Run, op executionbackend.Operation, expected string) error {
	var before *executionbackend.Operation
	if expected != "" {
		saved, err := (executionRecords{root: s.Config.RunDir, persistence: s.Config.Persistence, resources: s.Config.Resources}).loadOperation(op.RunID, op.AttemptID, op.ID)
		if err != nil {
			return err
		}
		if recordFingerprint(saved) != expected {
			return nativeErr("stale_revision", "execution operation changed before commitment")
		}
		before = &saved
	}
	return s.commitWorkerAttached(run, workerCommitAttachment{ExecutionOperation: &storageports.RecordWrite[executionbackend.Operation]{Before: before, After: op}})
}
