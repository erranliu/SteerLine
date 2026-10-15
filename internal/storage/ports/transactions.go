// Package ports owns the value contracts for center-owned atomic writes.
// Callbacks run once inside the same transaction; their errors abort every write.
// Implementations must not retain callbacks or publish results before commit.
package ports

import (
	"context"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type Metadata interface {
	SaveCheckpoint(context.Context, ServiceCheckpoint) (ServiceCheckpoint, error)
	Heartbeat(context.Context, string) error
}

type commandInsertTx interface {
	LookupCommand(context.Context, string, string) (CommandRecord, bool, error)
	EnqueueCommand(CommandRecord) (CommandRecord, bool, error)
	AppendEvent(DomainEvent) (int64, error)
}

type commandTx interface {
	commandInsertTx
	UpdateCommand(CommandRecord) error
}

type graphTx interface {
	TaskGraphSnapshot(context.Context, string) (TaskGraphSnapshot, error)
}

type BatchTx interface {
	commandTx
	graphTx
	PutTask(taskmodel.TaskRecord) error
	PutTaskContract(string, string, taskmodel.TaskContract) error
	PutTaskBatch(taskmodel.TaskBatchRecord, uint64, uint64) error
	PutTaskDependency(taskmodel.TaskDependencyRecord) error
}

type PredecessorTx interface {
	commandInsertTx
	RecordPredecessorAudit(CommandRecord) error
	graphTx
	PutTaskDependency(taskmodel.TaskDependencyRecord) error
	BumpOpenTaskVersionCAS(taskmodel.TaskRecord, uint64) error
	AdvanceTaskGraphRevision(string, uint64, uint64) (uint64, error)
}

type ProjectInitializer interface {
	InitializeProject(context.Context, executionmodel.ProjectTaskAuthority) error
}
