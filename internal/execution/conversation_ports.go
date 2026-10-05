package execution

import (
	"context"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// ConversationStore is the execution consumer's collector port. Query pages
// remain a separate read-only capability and no database handle crosses here.
type ConversationStore interface {
	storageports.ConversationReader
	CommitConversation(context.Context, []contracts.ConversationWrite, contracts.ConversationCheckpoint) ([]contracts.ConversationEvent, error)
	ConversationCheckpoint(context.Context, string, string, string, string, string) (contracts.ConversationCheckpoint, error)
	ConversationSourceCheckpoints(context.Context, string, string, string, string) ([]contracts.ConversationCheckpoint, error)
	ConversationMapperSnapshot(context.Context, string, string, string, string, string) (contracts.ConversationMapperSnapshot, error)
	ValidateConversationCheckpointFormat(context.Context, string, []string) error
	SaveConversationCapabilityReport(context.Context, contracts.ConversationCapabilityReport) error
}
