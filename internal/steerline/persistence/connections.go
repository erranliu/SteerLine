package persistence

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/taskview"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type RuntimeReader interface {
	LoadRuntime(string, bool) (*contracts.Runtime, error)
}

// Connection is the application-owned resource. Consumers receive only their
// existing write/read ports; no SQL or generic transaction is exposed.
type Connection interface {
	CenterReader
	CommandStore
	WorkerMetadata
	Projects
	Conversations
	// Declare provider capabilities structurally; the application-owned
	// connection must not import the product query use-case package.
	Query(context.Context, string) (contracts.DomainSnapshot, error)
	Events(context.Context, string, string, string, int64, int, int) ([]storageports.DomainEvent, error)
	EventPayload(context.Context, string, int64, int64, int) ([]byte, int64, string, error)
	NativeTaskPage(context.Context, string, string, string, uint64, int, int) (contracts.NativeTaskPage, error)
	TaskGraphSnapshot(context.Context, string) (taskview.Snapshot, error)
	ReadProjectView(context.Context, string, string, int) (contracts.ProjectView, error)
	NextConversationEvent(context.Context, string, contracts.ConversationFilter, int64) (*contracts.ConversationEventRecord, error)
	ConversationBody(context.Context, string, contracts.ConversationBodyRef, int) (contracts.ConversationBodyRecord, error)
	ConversationRaw(context.Context, string, contracts.ConversationRawRef, int) (contracts.ConversationRawRecord, error)
	ConversationCapabilityReports(context.Context, executionbackend.BackendKind, executionbackend.ExecutionProtocol, string, int) ([]contracts.ConversationCapabilityReport, error)
	RuntimeReader
	Close() error
	LoadProjection(context.Context, string) ([]byte, uint64, error)
	BindAuthority(executionmodel.ProjectTaskAuthority)
	ValidateStoredFormat(context.Context) error
	ExecutionBinding(context.Context, string, string) (executionbackend.ExecutionRef, error)
	ExecutionOperation(context.Context, string, string, string) (executionbackend.Operation, error)
}

// Factory is injected by application composition; the center never chooses a
// driver or constructs a concrete connection.
type Factory interface {
	OpenWriter(string) (Connection, error)
	OpenReader(string) (Connection, error)
}
