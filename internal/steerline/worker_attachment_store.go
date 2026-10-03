package steerline

import (
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (s *Service) commitWorkerBinding(run *Run, ref executionapi.ExecutionRef) error {
	return s.commitWorkerAttached(run, workerCommitAttachment{Binding: &ref})
}
func (s *Service) commitExecutionOperation(run *Run, op executionapi.Operation, expected string) error {
	var before *executionapi.Operation
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
	return s.commitWorkerAttached(run, workerCommitAttachment{ExecutionOperation: &recordWrite[executionapi.Operation]{Before: before, After: op}})
}
