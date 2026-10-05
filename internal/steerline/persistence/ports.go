// Package persistence owns the value contracts for center-owned atomic writes.
// Callbacks run once inside the same transaction; their errors abort every write.
// Implementations must not retain callbacks or publish results before commit.
package persistence

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type WorkerCommit struct {
	Binding                                             *executionbackend.ExecutionRef
	ExecutionOperation                                  *storageports.RecordWrite[executionbackend.Operation]
	Project, TaskID, WorkflowID, ExecutionID, AttemptID string
	ControlRevision, AuthorityRevision                  uint64
	ExpectedAttemptID                                   string
	Workflow                                            *storageports.RecordWrite[executionmodel.WorkflowRunRecord]
	Execution                                           *storageports.RecordWrite[executionmodel.ExecutionRecord]
	Attempt, RetiredAttempt                             *storageports.RecordWrite[executionmodel.StepAttemptRecord]
	Decisions                                           []storageports.RecordWrite[executionmodel.DecisionRecord]
	Operations                                          []storageports.RecordWrite[executionmodel.OperationRecord]
	Events                                              []storageports.DomainEvent
	Checkpoint                                          contracts.ServiceCheckpoint
}

type WorkerCommitResult struct {
	ControlRevision uint64
	Checkpoint      contracts.ServiceCheckpoint
}

type Worker interface {
	CommitWorker(context.Context, WorkerCommit) (WorkerCommitResult, error)
}

type Metadata interface {
	SaveCheckpoint(context.Context, contracts.ServiceCheckpoint) (contracts.ServiceCheckpoint, error)
	Heartbeat(context.Context, string) error
}

type WorkerMetadata interface {
	Worker
	Metadata
}

type commandInsertTx interface {
	LookupCommand(context.Context, string, string) (storageports.CommandRecord, bool, error)
	EnqueueCommand(storageports.CommandRecord) (storageports.CommandRecord, bool, error)
	AppendEvent(storageports.DomainEvent) (int64, error)
}

type commandTx interface {
	commandInsertTx
	UpdateCommand(storageports.CommandRecord) error
}

type operationTx interface {
	PutOperation(executionmodel.OperationRecord) (executionmodel.OperationRecord, bool, error)
}

type graphTx interface {
	TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error)
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
	commandTx
	operationTx
	graphTx
	workflowTx
	Query(context.Context, string) (contracts.DomainSnapshot, error)
	PutWorkflowRun(executionmodel.WorkflowRunRecord) error
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
	operationTx
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
	UpdateCommand(storageports.CommandRecord) error
	AppendEvent(storageports.DomainEvent) (int64, error)
}

type DeliveryAuditTx interface {
	graphTx
	workflowTx
	Query(context.Context, string) (contracts.DomainSnapshot, error)
	AppendEvent(storageports.DomainEvent) (int64, error)
}

type CleanupProgressTx interface {
	operationTx
	Task(context.Context, string, string) (taskmodel.TaskRecord, error)
}

type CleanupTx interface {
	Attempt(context.Context, string, string, string) (executionmodel.StepAttemptRecord, error)
	commandInsertTx
	operationTx
	workflowTx
	Task(context.Context, string, string) (taskmodel.TaskRecord, error)
	NativeAuthority(context.Context, string) (executionmodel.ProjectTaskAuthority, error)
	UpdateDiscardedTaskCAS(taskmodel.TaskRecord, uint64) error
	AdvanceTaskGraphRevision(string, uint64, uint64) (uint64, error)
	PutWorkflowRun(executionmodel.WorkflowRunRecord) error
	PutExecution(executionmodel.ExecutionRecord) error
	PutStepAttempt(executionmodel.StepAttemptRecord) error
}

type Tasks interface {
	TaskSnapshot(context.Context, string) (taskmodel.Snapshot, error)
	storageports.Results
	storageports.Claims
	storageports.Discards
	Query(context.Context, string) (contracts.DomainSnapshot, error)
	TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error)
	NativeAuthority(context.Context, string) (executionmodel.ProjectTaskAuthority, error)
	TaskContract(context.Context, string, taskmodel.TaskRecord) (taskmodel.TaskContract, error)
	Operation(context.Context, string, string) (executionmodel.OperationRecord, error)
	WorkflowOwned(context.Context, string, string, string) (bool, error)
	UpdateDiscardReceipt(context.Context, executionmodel.OperationRecord) error
	TransactTaskBatch(context.Context, func(BatchTx) error) error
	TransactCandidate(context.Context, func(CandidateTx) error) error
	TransactDispatch(context.Context, func(DispatchTx) error) error
	TransactPredecessors(context.Context, func(PredecessorTx) error) error
	TransactDiscardIntent(context.Context, func(DiscardIntentTx) error) error
	TransactDiscardPending(context.Context, func(DiscardPendingTx) error) error
	TransactDeliveryAudit(context.Context, func(DeliveryAuditTx) error) error
	TransactCleanupProgress(context.Context, func(CleanupProgressTx) error) error
	TransactCleanup(context.Context, func(CleanupTx) error) error
}

// CommandStore composes the operations consumed by the command application
// service. It supplies no shared transaction, connection or arbitrary row write.
type CommandStore interface {
	storageports.Commands
	Tasks
}

type CenterReader interface {
	Query(context.Context, string) (contracts.DomainSnapshot, error)
	NativeAuthority(context.Context, string) (executionmodel.ProjectTaskAuthority, error)
	DatabaseVersion(context.Context) (string, uint64, error)
}

type Conversations interface {
	CommitConversation(context.Context, []contracts.ConversationWrite, contracts.ConversationCheckpoint) ([]contracts.ConversationEvent, error)
	ConversationCheckpoint(context.Context, string, string, string, string, string) (contracts.ConversationCheckpoint, error)
	ConversationSourceCheckpoints(context.Context, string, string, string, string) ([]contracts.ConversationCheckpoint, error)
	ConversationMapperSnapshot(context.Context, string, string, string, string, string) (contracts.ConversationMapperSnapshot, error)
	ValidateConversationCheckpointFormat(context.Context, string, []string) error
	ConversationCapabilityReports(context.Context, executionbackend.BackendKind, executionbackend.ExecutionProtocol, string, int) ([]contracts.ConversationCapabilityReport, error)
	SaveConversationCapabilityReport(context.Context, contracts.ConversationCapabilityReport) error
}

type Projects interface {
	InitializeProject(context.Context, executionmodel.ProjectTaskAuthority) error
}
