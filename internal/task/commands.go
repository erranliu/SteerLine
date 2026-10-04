package task

import (
	"context"
	"time"

	"github.com/erranliu/SteerLine/internal/task/model"
)

// Input is the fixed task configuration. Document access is supplied separately;
// neither execution configuration nor execution write capabilities cross here.
type Input struct {
	Project, Authority, Repository, Remote, Branch string
	ExecutableTypes                                []string
	DefaultRequestedBy                             string
}

type Documents interface {
	RepositoryTarget(context.Context) (string, error)
	ReadCommittedRegularFile(context.Context, model.FileReference) ([]byte, error)
}

// Command is the task command's replay and audit envelope, without execution
// identity, provider evidence or control records.
type Command struct {
	ProjectIdentity, CommandID, Kind, TargetID string
	ExpectedVersion                            uint64
	IdempotencyKey, RequestedBy                string
	Payload                                    []byte
	Status                                     string
	Result                                     []byte
	CreatedAt, UpdatedAt                       time.Time
}

type Event struct {
	ProjectIdentity, TaskID, ExternalSource, ExternalID, Type string
	ReceivedAt                                                time.Time
	Payload                                                   []byte
}

type CommandTx interface {
	LookupCommand(context.Context, string, string) (Command, bool, error)
	EnqueueCommand(Command) (Command, bool, error)
	AppendEvent(Event) (int64, error)
	TaskSnapshot(context.Context, string) (model.Snapshot, error)
}

type BatchTx interface {
	CommandTx
	UpdateCommand(Command) error
	PutTask(model.TaskRecord) error
	PutTaskContract(string, string, model.TaskContract) error
	PutTaskBatch(model.TaskBatchRecord, uint64, uint64) error
	PutTaskDependency(model.TaskDependencyRecord) error
}

type PredecessorTx interface {
	CommandTx
	PutTaskDependency(model.TaskDependencyRecord) error
	BumpOpenTaskVersionCAS(model.TaskRecord, uint64) error
	AdvanceTaskGraphRevision(string, uint64, uint64) (uint64, error)
	RecordPredecessorAudit(Command) error
}

type CommandStore interface {
	ReadGraph(context.Context, Input) (model.Snapshot, error)
	LookupCommand(context.Context, string, string) (Command, bool, error)
	CheckTarget(context.Context, Input) error
	TransactTaskBatch(context.Context, func(BatchTx) error) error
	TransactPredecessors(context.Context, func(PredecessorTx) error) error
}

type Commands struct {
	Store     CommandStore
	Documents Documents
	Input     Input
}
