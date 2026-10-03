// Package persistence owns the value contracts for center-owned atomic writes.
// Callbacks run once inside the same transaction; their errors abort every write.
// Implementations must not retain callbacks or publish results before commit.
package persistence

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/task"
)

var ErrNotFound = errors.New("persistent record not found")

type RecordWrite[T any] struct {
	Before *T
	After  T
}

// WorkerCommit declares only records changed by one transition, with the exact
// authoritative values from which the center computed its private candidate.
type WorkerCommit struct {
	Binding                                             *execution.ExecutionRef
	ExecutionOperation                                  *RecordWrite[execution.Operation]
	Project, TaskID, WorkflowID, ExecutionID, AttemptID string
	ControlRevision, AuthorityRevision                  uint64
	ExpectedAttemptID                                   string
	Workflow                                            *RecordWrite[contracts.WorkflowRunRecord]
	Execution                                           *RecordWrite[contracts.ExecutionRecord]
	Attempt, RetiredAttempt                             *RecordWrite[contracts.StepAttemptRecord]
	Decisions                                           []RecordWrite[contracts.DecisionRecord]
	Operations                                          []RecordWrite[contracts.OperationRecord]
	Events                                              []contracts.DomainEvent
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
type Projects interface {
	InitializeProject(context.Context, task.ProjectTaskAuthority) error
}

type WorkerMetadata interface {
	Worker
	Metadata
}

type Commands interface {
	EnqueueCommand(context.Context, contracts.CommandRecord) (contracts.CommandRecord, bool, error)
	AcceptDecisionAnswer(context.Context, contracts.CommandRecord, contracts.DecisionAnswer) (contracts.DecisionRecord, bool, error)
	UpdateCommand(context.Context, contracts.CommandRecord) error
	Commands(context.Context, string, string, int) ([]contracts.CommandRecord, error)
	ServiceCommands(context.Context, string, string, int) ([]contracts.CommandRecord, error)
	Command(context.Context, string, string) (contracts.CommandRecord, error)
	CommandByIdempotency(context.Context, string, string) (contracts.CommandRecord, error)
	LookupCommand(context.Context, string, string) (contracts.CommandRecord, bool, error)
}
type commandInsertTx interface {
	LookupCommand(context.Context, string, string) (contracts.CommandRecord, bool, error)
	EnqueueCommand(contracts.CommandRecord) (contracts.CommandRecord, bool, error)
	AppendEvent(contracts.DomainEvent) (int64, error)
}
type commandTx interface {
	commandInsertTx
	UpdateCommand(contracts.CommandRecord) error
}
type operationTx interface {
	PutOperation(contracts.OperationRecord) (contracts.OperationRecord, bool, error)
}
type graphTx interface {
	TaskGraphSnapshot(context.Context, string) (task.Snapshot, error)
}
type workflowTx interface {
	Workflow(context.Context, string, string, string) (contracts.WorkflowRunRecord, error)
	Execution(context.Context, string, string, string) (contracts.ExecutionRecord, error)
}

type BatchTx interface {
	commandTx
	graphTx
	PutTask(contracts.TaskRecord) error
	PutTaskContract(string, string, task.TaskContract) error
	PutTaskBatch(task.TaskBatchRecord, uint64, uint64) error
	PutTaskDependency(task.TaskDependencyRecord) error
}
type ClaimTx interface {
	commandTx
	operationTx
	graphTx
	OccupiedExecutions(context.Context, string) (int, error)
	UpdateNativeTaskCAS(contracts.TaskRecord, uint64) error
	PutWorkflowRun(contracts.WorkflowRunRecord) error
	PutExecution(contracts.ExecutionRecord) error
	PutStepAttempt(contracts.StepAttemptRecord) error
}
type DispatchTx interface {
	commandTx
	operationTx
	graphTx
	UpdateProjectTaskAuthorityCAS(task.ProjectTaskAuthority, uint64) error
}
type PredecessorTx interface {
	commandInsertTx
	operationTx
	graphTx
	PutTaskDependency(task.TaskDependencyRecord) error
	BumpOpenTaskVersionCAS(contracts.TaskRecord, uint64) error
	AdvanceTaskGraphRevision(string, uint64, uint64) (uint64, error)
}
type DiscardIntentTx interface {
	commandTx
	operationTx
	Task(context.Context, string, string) (contracts.TaskRecord, error)
	DeliveryCount(context.Context, string, string) (int, error)
}
type DiscardPendingTx interface {
	UpdateOperation(contracts.OperationRecord) error
	UpdateCommand(contracts.CommandRecord) error
	AppendEvent(contracts.DomainEvent) (int64, error)
}
type DiscardTx interface {
	DiscardPendingTx
	graphTx
	Query(context.Context, string) (contracts.DomainSnapshot, error)
	DeliveryCount(context.Context, string, string) (int, error)
	UpdateNativeTaskCAS(contracts.TaskRecord, uint64) error
}
type DeliveryTx interface {
	graphTx
	workflowTx
	Query(context.Context, string) (contracts.DomainSnapshot, error)
	TaskContract(context.Context, string, contracts.TaskRecord) (task.TaskContract, error)
	PRBindingsOnOtherTasks(context.Context, string, string, string) (int, error)
	PutTaskDelivery(task.TaskDeliveryRecord) error
	UpdateNativeTaskCAS(contracts.TaskRecord, uint64) error
	AppendEvent(contracts.DomainEvent) (int64, error)
}
type CleanupProgressTx interface {
	operationTx
	Task(context.Context, string, string) (contracts.TaskRecord, error)
}
type CleanupTx interface {
	Attempt(context.Context, string, string, string) (contracts.StepAttemptRecord, error)
	commandInsertTx
	operationTx
	workflowTx
	Task(context.Context, string, string) (contracts.TaskRecord, error)
	NativeAuthority(context.Context, string) (task.ProjectTaskAuthority, error)
	UpdateDiscardedTaskCAS(contracts.TaskRecord, uint64) error
	AdvanceTaskGraphRevision(string, uint64, uint64) (uint64, error)
	PutWorkflowRun(contracts.WorkflowRunRecord) error
	PutExecution(contracts.ExecutionRecord) error
	PutStepAttempt(contracts.StepAttemptRecord) error
}
type Tasks interface {
	Query(context.Context, string) (contracts.DomainSnapshot, error)
	TaskGraphSnapshot(context.Context, string) (task.Snapshot, error)
	NativeAuthority(context.Context, string) (task.ProjectTaskAuthority, error)
	TaskContract(context.Context, string, contracts.TaskRecord) (task.TaskContract, error)
	Operation(context.Context, string, string) (contracts.OperationRecord, error)
	WorkflowOwned(context.Context, string, string, string) (bool, error)
	UpdateDiscardReceipt(context.Context, contracts.OperationRecord) error
	TransactTaskBatch(context.Context, func(BatchTx) error) error
	TransactClaim(context.Context, func(ClaimTx) error) error
	TransactDispatch(context.Context, func(DispatchTx) error) error
	TransactPredecessors(context.Context, func(PredecessorTx) error) error
	TransactDiscardIntent(context.Context, func(DiscardIntentTx) error) error
	TransactDiscardPending(context.Context, func(DiscardPendingTx) error) error
	TransactDiscard(context.Context, func(DiscardTx) error) error
	TransactDelivery(context.Context, func(DeliveryTx) error) error
	TransactCleanupProgress(context.Context, func(CleanupProgressTx) error) error
	TransactCleanup(context.Context, func(CleanupTx) error) error
}

// CommandStore composes the operations consumed by the command application
// service. It supplies no shared transaction, connection or arbitrary row write.
type CommandStore interface {
	Commands
	Tasks
}

type CenterReader interface {
	Query(context.Context, string) (contracts.DomainSnapshot, error)
	NativeAuthority(context.Context, string) (task.ProjectTaskAuthority, error)
	DatabaseVersion(context.Context) (string, uint64, error)
}
type Conversations interface {
	CommitConversation(context.Context, []contracts.ConversationWrite, contracts.ConversationCheckpoint) ([]contracts.ConversationEvent, error)
	ConversationCheckpoint(context.Context, string, string, string, string, string) (contracts.ConversationCheckpoint, error)
	ConversationSourceCheckpoints(context.Context, string, string, string, string) ([]contracts.ConversationCheckpoint, error)
	ConversationMapperSnapshot(context.Context, string, string, string, string, string) (contracts.ConversationMapperSnapshot, error)
	ValidateConversationCheckpointFormat(context.Context, string, []string) error
	ConversationCapabilityReports(context.Context, execution.BackendKind, execution.ExecutionProtocol, string, int) ([]contracts.ConversationCapabilityReport, error)
	SaveConversationCapabilityReport(context.Context, contracts.ConversationCapabilityReport) error
}

var ErrAnswerConflict = errors.New("answer_conflict: this decision already has a different accepted answer")
