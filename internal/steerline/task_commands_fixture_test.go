package steerline

import (
	"context"
	"errors"
	"time"

	execution "github.com/erranliu/SteerLine/internal/execution"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// The adapter projects task-only transactions. In particular RecordPredecessorAudit
// can only persist the audit of this task command, not arbitrary execution records.
type taskCommandStore struct{ store persistence.CommandStore }

func taskCommand(v storageports.CommandRecord) taskapp.Command {
	return taskapp.Command{ProjectIdentity: v.ProjectIdentity, CommandID: v.CommandID, Kind: v.Kind, TargetID: v.TargetID, ExpectedVersion: v.ExpectedVersion, IdempotencyKey: v.IdempotencyKey, RequestedBy: v.RequestedBy, Payload: v.Payload, Status: v.Status, Result: v.Result, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func storedTaskCommand(v taskapp.Command) storageports.CommandRecord {
	return storageports.CommandRecord{ProjectIdentity: v.ProjectIdentity, CommandID: v.CommandID, Kind: v.Kind, TargetID: v.TargetID, ExpectedVersion: v.ExpectedVersion, IdempotencyKey: v.IdempotencyKey, RequestedBy: v.RequestedBy, Payload: v.Payload, Status: v.Status, Result: v.Result, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func storedTaskEvent(v taskapp.Event) storageports.DomainEvent {
	return storageports.DomainEvent{ProjectIdentity: v.ProjectIdentity, TaskID: v.TaskID, ExternalSource: v.ExternalSource, ExternalID: v.ExternalID, Type: v.Type, ReceivedAt: v.ReceivedAt, Payload: v.Payload}
}

func (a taskCommandStore) ReadGraph(ctx context.Context, in taskapp.Input) (taskmodel.Snapshot, error) {
	graph, err := a.store.TaskGraphSnapshot(ctx, in.Project)
	if err != nil {
		return taskmodel.Snapshot{}, err
	}
	v := graph.Authority
	if v.ProjectIdentity != in.Project || v.Authority != in.Authority || v.Repository != in.Repository || v.Remote != in.Remote || v.Branch != in.Branch {
		return taskmodel.Snapshot{}, &taskmodel.Error{Code: "authority_mismatch", Message: "configured Git repository, remote or branch differs from the registered native project"}
	}
	return graph.TaskGraph(), nil
}
func (a taskCommandStore) LookupCommand(ctx context.Context, project, id string) (taskapp.Command, bool, error) {
	v, found, err := a.store.LookupCommand(ctx, project, id)
	return taskCommand(v), found, err
}
func (a taskCommandStore) CheckTarget(ctx context.Context, in taskapp.Input) error {
	v, err := a.store.NativeAuthority(ctx, in.Project)
	if err != nil {
		return err
	}
	if v.ProjectIdentity != in.Project || v.Authority != in.Authority || v.Repository != in.Repository || v.Remote != in.Remote || v.Branch != in.Branch {
		return &taskmodel.Error{Code: "authority_mismatch", Message: "configured Git repository, remote or branch differs from the registered native project"}
	}
	return nil
}
func (a taskCommandStore) TransactTaskBatch(ctx context.Context, apply func(taskapp.BatchTx) error) error {
	return a.store.TransactTaskBatch(ctx, func(tx persistence.BatchTx) error { return apply(taskBatchTx{tx}) })
}
func (a taskCommandStore) TransactPredecessors(ctx context.Context, apply func(taskapp.PredecessorTx) error) error {
	return a.store.TransactPredecessors(ctx, func(tx persistence.PredecessorTx) error { return apply(taskPredecessorTx{tx}) })
}

type taskBatchTx struct{ tx persistence.BatchTx }

func (a taskBatchTx) LookupCommand(ctx context.Context, project, id string) (taskapp.Command, bool, error) {
	v, found, err := a.tx.LookupCommand(ctx, project, id)
	return taskCommand(v), found, err
}
func (a taskBatchTx) EnqueueCommand(v taskapp.Command) (taskapp.Command, bool, error) {
	made, found, err := a.tx.EnqueueCommand(storedTaskCommand(v))
	return taskCommand(made), found, err
}
func (a taskBatchTx) UpdateCommand(v taskapp.Command) error {
	return a.tx.UpdateCommand(storedTaskCommand(v))
}
func (a taskBatchTx) AppendEvent(v taskapp.Event) (int64, error) {
	return a.tx.AppendEvent(storedTaskEvent(v))
}
func (a taskBatchTx) TaskSnapshot(ctx context.Context, project string) (taskmodel.Snapshot, error) {
	v, err := a.tx.TaskGraphSnapshot(ctx, project)
	if err != nil {
		return taskmodel.Snapshot{}, err
	}
	return v.TaskGraph(), nil
}
func (a taskBatchTx) PutTask(v taskmodel.TaskRecord) error { return a.tx.PutTask(v) }
func (a taskBatchTx) PutTaskContract(project, id string, v taskmodel.TaskContract) error {
	return a.tx.PutTaskContract(project, id, v)
}
func (a taskBatchTx) PutTaskBatch(v taskmodel.TaskBatchRecord, authority, graph uint64) error {
	return a.tx.PutTaskBatch(v, authority, graph)
}
func (a taskBatchTx) PutTaskDependency(v taskmodel.TaskDependencyRecord) error {
	return a.tx.PutTaskDependency(v)
}

type taskPredecessorTx struct{ tx persistence.PredecessorTx }

func (a taskPredecessorTx) LookupCommand(ctx context.Context, project, id string) (taskapp.Command, bool, error) {
	v, found, err := a.tx.LookupCommand(ctx, project, id)
	return taskCommand(v), found, err
}
func (a taskPredecessorTx) EnqueueCommand(v taskapp.Command) (taskapp.Command, bool, error) {
	made, found, err := a.tx.EnqueueCommand(storedTaskCommand(v))
	return taskCommand(made), found, err
}
func (a taskPredecessorTx) AppendEvent(v taskapp.Event) (int64, error) {
	return a.tx.AppendEvent(storedTaskEvent(v))
}
func (a taskPredecessorTx) TaskSnapshot(ctx context.Context, project string) (taskmodel.Snapshot, error) {
	v, err := a.tx.TaskGraphSnapshot(ctx, project)
	if err != nil {
		return taskmodel.Snapshot{}, err
	}
	return v.TaskGraph(), nil
}
func (a taskPredecessorTx) PutTaskDependency(v taskmodel.TaskDependencyRecord) error {
	return a.tx.PutTaskDependency(v)
}
func (a taskPredecessorTx) BumpOpenTaskVersionCAS(v taskmodel.TaskRecord, version uint64) error {
	return a.tx.BumpOpenTaskVersionCAS(v, version)
}
func (a taskPredecessorTx) AdvanceTaskGraphRevision(project string, authority, graph uint64) (uint64, error) {
	return a.tx.AdvanceTaskGraphRevision(project, authority, graph)
}
func (a taskPredecessorTx) RecordPredecessorAudit(v taskapp.Command) error {
	if v.Kind != "add_task_predecessors" || v.Status != "accepted" {
		return &taskmodel.Error{Code: "invalid_batch", Message: "audit requires an accepted predecessor command"}
	}
	op := executionmodel.OperationRecord{ProjectIdentity: v.ProjectIdentity, OperationID: v.CommandID, IdempotencyKey: v.IdempotencyKey, Kind: v.Kind, Status: "confirmed", PayloadDigest: taskPayloadDigest(v.Payload), Request: v.Payload, Receipt: v.Result, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
	if _, made, err := a.tx.PutOperation(op); err != nil || !made {
		return errors.Join(err, &taskmodel.Error{Code: "idempotency_conflict", Message: "predecessor operation already exists"})
	}
	return nil
}

// NewTaskCommands extracts only fixed task input and injects narrow I/O ports.
func NewTaskCommands(store persistence.CommandStore, config execution.Config) taskapp.Commands {
	timeout, _ := time.ParseDuration(config.CommandTimeout)
	return taskapp.Commands{Store: taskCommandStore{store}, Documents: taskDocuments{config.TaskRepositoryTarget, config.Repository, config.Git, timeout}, Input: taskapp.Input{Project: config.Identity(), Authority: config.TaskAuthority, Remote: config.Remote, Branch: config.Branch, ExecutableTypes: append([]string(nil), config.ExecutableTypes...), DefaultRequestedBy: DefaultRequestedBy()}}
}

func (c taskTestCommands) ValidateTaskBatch(ctx context.Context, config execution.Config, data []byte) (taskmodel.TaskBatchValidation, error) {
	commands := NewTaskCommands(c.Store, config)
	return commands.ValidateTaskBatch(ctx, data)
}
func (c taskTestCommands) CreateTaskBatch(ctx context.Context, config execution.Config, data []byte, request string, version uint64, by string) (taskmodel.TaskBatchReceipt, error) {
	if err := config.ValidateTaskCommandConfiguration(); err != nil {
		return taskmodel.TaskBatchReceipt{}, err
	}
	commands := NewTaskCommands(c.Store, config)
	return commands.CreateTaskBatch(ctx, data, request, version, by)
}
func (c taskTestCommands) AddNativeTaskPredecessors(ctx context.Context, config execution.Config, in productapi.TaskPredecessorAddInput) (productapi.TaskPredecessorAddReceipt, error) {
	if err := config.ValidateTaskCommandConfiguration(); err != nil {
		return productapi.TaskPredecessorAddReceipt{}, err
	}
	commands := NewTaskCommands(c.Store, config)
	receipt, err := commands.AddNativeTaskPredecessors(ctx, taskmodel.PredecessorAddInput(in))
	return productapi.TaskPredecessorAddReceipt(receipt), err
}
func validateTaskBatchAgainstGraph(ctx context.Context, config execution.Config, batch taskmodel.TaskBatchInput, graph taskGraphSnapshot, verify bool) ([]taskmodel.TaskBatchIssue, []string) {
	return taskmodel.ValidateBatchAgainstGraph(batch, graph.TaskGraph(), config.ExecutableTypes, taskmodel.ValidateContract)
}
func validateNativePredecessorAddition(graph taskGraphSnapshot, id string, predecessors []string) error {
	return taskmodel.ValidatePredecessorAddition(graph.TaskGraph(), id, predecessors)
}
