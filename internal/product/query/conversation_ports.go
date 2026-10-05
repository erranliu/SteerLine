package query

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

type ConversationEventReader interface {
	NextConversationEvent(context.Context, string, contracts.ConversationFilter, int64) (*contracts.ConversationEventRecord, error)
}

type ConversationStore interface {
	ConversationEventReader
	ConversationBodyReader
	ConversationRawReader
	ConversationCapabilityReader
}

type ConversationBodyReader interface {
	ConversationBody(context.Context, string, contracts.ConversationBodyRef, int) (contracts.ConversationBodyRecord, error)
}

type ConversationRawReader interface {
	ConversationRaw(context.Context, string, contracts.ConversationRawRef, int) (contracts.ConversationRawRecord, error)
}

type ConversationCapabilityReader interface {
	ConversationCapabilityReports(context.Context, executionbackend.BackendKind, executionbackend.ExecutionProtocol, string, int) ([]contracts.ConversationCapabilityReport, error)
}
