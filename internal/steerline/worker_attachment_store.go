package steerline

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (s *Service) commitWorkerBinding(run *Run, ref executionbackend.ExecutionRef) error {
	return s.commitWorkerAttached(run, workerCommitAttachment{Binding: &ref})
}

func (s *Service) commitExecutionOperation(run *Run, op executionbackend.Operation, expected string) error {
	var before *executionbackend.Operation
	if expected != "" {
		saved, err := (executionRecords{root: s.Config.RunDir, persistence: s.Config.Persistence}).loadOperation(op.RunID, op.AttemptID, op.ID)
		if err != nil {
			return err
		}
		if recordFingerprint(saved) != expected {
			return nativeErr("stale_revision", "execution operation changed before commitment")
		}
		before = &saved
	}
	return s.commitWorkerAttached(run, workerCommitAttachment{ExecutionOperation: &recordWrite[executionbackend.Operation]{Before: before, After: op}})
}
