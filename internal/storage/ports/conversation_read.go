package ports

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

type ConversationEventReader interface {
	NextConversationEvent(context.Context, string, api.ConversationFilter, int64) (*ConversationEventRecord, error)
}

type ConversationBodyReader interface {
	ConversationBody(context.Context, string, api.ConversationBodyRef, int) (ConversationBodyRecord, error)
}

type ConversationRawReader interface {
	ConversationRaw(context.Context, string, api.ConversationRawRef, int) (ConversationRawRecord, error)
}

type ConversationCapabilityReader interface {
	ConversationCapabilityReports(context.Context, executionbackend.BackendKind, executionbackend.ExecutionProtocol, string, int) ([]api.ConversationCapabilityReport, error)
}

type ConversationReader interface {
	ConversationEventReader
	ConversationBodyReader
	ConversationRawReader
	ConversationCapabilityReader
}

type ConversationEventRecord struct {
	EventID      int64
	PayloadBytes int
	Payload      []byte
}

type ConversationBodyRecord struct {
	Body         []byte
	TotalBytes   int64
	SHA256       string
	Completeness string
}

type ConversationRawRecord struct {
	Data       []byte
	TotalBytes int64
	SHA256     string
}
