package execution

import (
	"context"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// ConversationStore is the execution consumer's collector port. Query pages
// remain a separate read-only capability and no database handle crosses here.
type ConversationStore interface {
	storageports.ConversationReader
	CommitConversation(context.Context, []executionvalues.ConversationWrite, executionvalues.ConversationCheckpoint) ([]executionvalues.ConversationEvent, error)
	ConversationCheckpoint(context.Context, string, string, string, string, string) (executionvalues.ConversationCheckpoint, error)
	ConversationSourceCheckpoints(context.Context, string, string, string, string) ([]executionvalues.ConversationCheckpoint, error)
	ConversationMapperSnapshot(context.Context, string, string, string, string, string) (executionvalues.ConversationMapperSnapshot, error)
	ValidateConversationCheckpointFormat(context.Context, string, []string) error
	SaveConversationCapabilityReport(context.Context, executionvalues.ConversationCapabilityReport) error
}
