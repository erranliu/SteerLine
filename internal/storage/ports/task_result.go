package ports

import (
	"context"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// ResultCommit combines two fixed declarations. There is no transaction handle
// and no write callback; the store rechecks both domains before publishing a receipt.
type ResultCommit struct {
	Task             taskmodel.ResultDeclaration
	AuthorityVersion uint64
	Workflow         executionmodel.WorkflowRunRecord
	Execution        executionmodel.ExecutionRecord
	Delivery         executionmodel.TaskDeliveryRecord
}

type Results interface {
	CommitTaskResult(context.Context, ResultCommit) (ResultCommitReceipt, error)
	TaskResult(context.Context, string, string) (taskmodel.ResultReceipt, bool, error)
}

type ResultCommitReceipt struct {
	Task     taskmodel.ResultReceipt
	Delivery executionmodel.TaskDeliveryRecord
}
