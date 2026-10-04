package ports

import (
	"context"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type DiscardCommit struct {
	Before, After                   taskmodel.TaskRecord
	Intent                          taskmodel.DiscardIntent
	AuthorityVersion, GraphRevision uint64
	Facts                           executionmodel.DiscardFacts
	Known                           []executionmodel.VerifiedCandidatePR
	Operation                       executionmodel.OperationRecord
}

type Discards interface {
	CommitTaskDiscard(context.Context, DiscardCommit) (taskmodel.TaskDiscardReceipt, error)
}
