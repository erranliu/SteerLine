package persistence

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/query"
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
