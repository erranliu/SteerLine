// Package ports owns the value contracts for center-owned atomic writes.
// Callbacks run once inside the same transaction; their errors abort every write.
// Implementations must not retain callbacks or publish results before commit.
package ports

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type WorkerCommit struct {
	Binding                                             *executionbackend.ExecutionRef
	ExecutionOperation                                  *RecordWrite[executionbackend.Operation]
	Project, TaskID, WorkflowID, ExecutionID, AttemptID string
	ControlRevision, AuthorityRevision                  uint64
	ExpectedAttemptID                                   string
	Workflow                                            *RecordWrite[executionmodel.WorkflowRunRecord]
	Execution                                           *RecordWrite[executionmodel.ExecutionRecord]
	Attempt, RetiredAttempt                             *RecordWrite[executionmodel.StepAttemptRecord]
	Decisions                                           []RecordWrite[executionmodel.DecisionRecord]
	Operations                                          []RecordWrite[executionmodel.OperationRecord]
	Events                                              []DomainEvent
	Checkpoint                                          ServiceCheckpoint
}

type WorkerCommitResult struct {
	ControlRevision uint64
	Checkpoint      ServiceCheckpoint
}

type Worker interface {
	CommitWorker(context.Context, WorkerCommit) (WorkerCommitResult, error)
}

type Metadata interface {
	SaveCheckpoint(context.Context, ServiceCheckpoint) (ServiceCheckpoint, error)
	Heartbeat(context.Context, string) error
}

type WorkerMetadata interface {
	Worker
	Metadata
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

type operationTx interface {
	PutOperation(executionmodel.OperationRecord) (executionmodel.OperationRecord, bool, error)
}

type graphTx interface {
	TaskGraphSnapshot(context.Context, string) (TaskGraphSnapshot, error)
}

type workflowTx interface {
	Workflow(context.Context, string, string, string) (executionmodel.WorkflowRunRecord, error)
	Execution(context.Context, string, string, string) (executionmodel.ExecutionRecord, error)
}

type BatchTx interface {
	commandTx
	graphTx
	PutTask(taskmodel.TaskRecord) error
	PutTaskContract(string, string, taskmodel.TaskContract) error
	PutTaskBatch(taskmodel.TaskBatchRecord, uint64, uint64) error
	PutTaskDependency(taskmodel.TaskDependencyRecord) error
}

type CandidateTx interface {
	ParticipatingCandidateCount(context.Context, string, string) (int, error)
	commandTx
	operationTx
	graphTx
	workflowTx
	TaskCandidateReader
	PutWorkflowRun(executionmodel.WorkflowRunRecord) error
	PutExecution(executionmodel.ExecutionRecord) error
	PutStepAttempt(executionmodel.StepAttemptRecord) error
	UpdateOperation(executionmodel.OperationRecord) error
}

type DispatchTx interface {
	commandTx
	operationTx
	graphTx
	UpdateProjectTaskAuthorityCAS(executionmodel.ProjectTaskAuthority, uint64) error
}

type PredecessorTx interface {
	commandInsertTx
	RecordPredecessorAudit(CommandRecord) error
	graphTx
	PutTaskDependency(taskmodel.TaskDependencyRecord) error
	BumpOpenTaskVersionCAS(taskmodel.TaskRecord, uint64) error
	AdvanceTaskGraphRevision(string, uint64, uint64) (uint64, error)
}

type DiscardIntentTx interface {
	commandTx
	operationTx
	Task(context.Context, string, string) (taskmodel.TaskRecord, error)
	DeliveryCount(context.Context, string, string) (int, error)
}

type DiscardPendingTx interface {
	UpdateOperation(executionmodel.OperationRecord) error
	UpdateCommand(CommandRecord) error
	AppendEvent(DomainEvent) (int64, error)
}

type DeliveryAuditTx interface {
	graphTx
	workflowTx
	TaskCandidateReader
	AppendEvent(DomainEvent) (int64, error)
}

type StopReceiptTx interface {
	Attempt(context.Context, string, string, string) (executionmodel.StepAttemptRecord, error)
	commandInsertTx
	operationTx
	workflowTx
	Task(context.Context, string, string) (taskmodel.TaskRecord, error)
	NativeAuthority(context.Context, string) (executionmodel.ProjectTaskAuthority, error)
	UpdateDiscardedTaskCAS(taskmodel.TaskRecord, uint64) error
	AdvanceTaskGraphRevision(string, uint64, uint64) (uint64, error)
	PutExecution(executionmodel.ExecutionRecord) error
	PutStepAttempt(executionmodel.StepAttemptRecord) error
}

type ProjectInitializer interface {
	InitializeProject(context.Context, executionmodel.ProjectTaskAuthority) error
}
