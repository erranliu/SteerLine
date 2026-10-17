package app

import (
	"context"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// The adapter projects task-only transactions. In particular RecordPredecessorAudit
// can only persist the audit of this task command, not arbitrary execution records.
type taskStorage interface {
	storageports.Commands
	TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error)
	TaskContract(context.Context, string, taskmodel.TaskRecord) (taskmodel.TaskContract, error)
	NativeAuthority(context.Context, string) (taskmodel.ProjectTaskAuthority, error)
	TransactTaskBatch(context.Context, func(storageports.BatchTx) error) error
	TransactPredecessors(context.Context, func(storageports.PredecessorTx) error) error
}

type taskCommandStore struct{ store taskStorage }

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
func (a taskCommandStore) TaskSnapshot(ctx context.Context, project string) (taskmodel.Snapshot, error) {
	graph, err := a.store.TaskGraphSnapshot(ctx, project)
	if err != nil {
		return taskmodel.Snapshot{}, err
	}
	return graph.TaskGraph(), nil
}
func (a taskCommandStore) TaskContract(ctx context.Context, project string, task taskmodel.TaskRecord) (taskmodel.TaskContract, error) {
	return a.store.TaskContract(ctx, project, task)
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
	return a.store.TransactTaskBatch(ctx, func(tx storageports.BatchTx) error { return apply(taskBatchTx{tx}) })
}
func (a taskCommandStore) TransactPredecessors(ctx context.Context, apply func(taskapp.PredecessorTx) error) error {
	return a.store.TransactPredecessors(ctx, func(tx storageports.PredecessorTx) error { return apply(taskPredecessorTx{tx}) })
}

type taskBatchTx struct{ tx storageports.BatchTx }

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

type taskPredecessorTx struct{ tx storageports.PredecessorTx }

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
	return a.tx.RecordPredecessorAudit(storedTaskCommand(v))
}
