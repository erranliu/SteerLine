package ports

import (
	"context"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// ExecutionSnapshot contains the persisted execution facts consumed by the
// execution actor and read projections. Task records remain in TaskGraphSnapshot.
type ExecutionSnapshot struct {
	ProjectIdentity string
	WorkflowRuns    []executionmodel.WorkflowRunRecord
	Executions      []executionmodel.ExecutionRecord
	Attempts        []executionmodel.StepAttemptRecord
	Decisions       []executionmodel.DecisionRecord
	Operations      []executionmodel.OperationRecord
}

// ExecutionStateSnapshot is sampled atomically for the execution actor's
// recovery projection. Product reads use their narrower task/activity ports.
type ExecutionStateSnapshot struct {
	Graph      TaskGraphSnapshot
	Records    ExecutionSnapshot
	Checkpoint *ServiceCheckpoint
}

type ExecutionStateReader interface {
	ReadExecutionStateSnapshot(context.Context, string) (ExecutionStateSnapshot, error)
}
