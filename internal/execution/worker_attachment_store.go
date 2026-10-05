package execution

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (s *Service) commitWorkerBinding(run *reconstructiontypes.RunView, ref executionbackend.ExecutionRef) error {
	return s.commitWorkerAttached(run, workerCommitAttachment{Binding: &ref})
}

func (s *Service) commitExecutionOperation(run *reconstructiontypes.RunView, op executionbackend.Operation, expected string) error {
	var before *executionbackend.Operation
	if expected != "" {
		saved, err := (executionRecords{root: s.Config.RunDir, reader: s.Config.ExecutionRecords, resources: s.Config.Resources}).loadOperation(op.RunID, op.AttemptID, op.ID)
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
