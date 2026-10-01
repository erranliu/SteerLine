package query

import (
	"context"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type ConversationEventReader interface {
	NextConversationEvent(context.Context, string, contracts.ConversationFilter, int64) (*contracts.ConversationEventRecord, error)
}
type ConversationBodyReader interface {
	ConversationBody(context.Context, string, contracts.ConversationBodyRef, int) (contracts.ConversationBodyRecord, error)
}
type ConversationRawReader interface {
	ConversationRaw(context.Context, string, contracts.ConversationRawRef, int) (contracts.ConversationRawRecord, error)
}
type ConversationCapabilityReader interface {
	ConversationCapabilityReports(context.Context, executionapi.BackendKind, executionapi.ExecutionProtocol, string, int) ([]contracts.ConversationCapabilityReport, error)
}
