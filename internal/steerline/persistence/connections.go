package persistence

import (
	"context"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/query"
	"github.com/erranliu/SteerLine/internal/steerline/task"
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
	query.Store
	query.ConversationEventReader
	query.ConversationBodyReader
	query.ConversationRawReader
	query.ConversationCapabilityReader
	RuntimeReader
	Close() error
	LoadProjection(context.Context, string) ([]byte, uint64, error)
	BindAuthority(task.ProjectTaskAuthority)
	ValidateStoredFormat(context.Context) error
	ExecutionBinding(context.Context, string, string) (execution.ExecutionRef, error)
	ExecutionOperation(context.Context, string, string, string) (execution.Operation, error)
}

// Factory is injected by application composition; the center never chooses a
// driver or constructs a concrete connection.
type Factory interface {
	OpenWriter(string) (Connection, error)
	OpenReader(string) (Connection, error)
}
