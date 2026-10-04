package ports

import (
	"context"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type ClaimCommit struct {
	Before, After                   taskmodel.TaskRecord
	Contract                        taskmodel.TaskContract
	AuthorityVersion, GraphRevision uint64
	Candidate, Explicit             bool
	ExpectedPRBindings              string
	PredecessorMerges               map[string]string
	Workflow                        executionmodel.WorkflowRunRecord
	Execution                       executionmodel.ExecutionRecord
	Attempt                         executionmodel.StepAttemptRecord
	Command                         CommandRecord
	Operation                       executionmodel.OperationRecord
}

type Claims interface {
	CommitTaskClaim(context.Context, ClaimCommit) (executionmodel.TaskClaimReceipt, error)
}
