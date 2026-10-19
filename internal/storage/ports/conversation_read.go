package ports

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type ConversationEventReader interface {
	NextConversationEvent(context.Context, string, ConversationFilter, int64) (*ConversationEventRecord, error)
}

type ConversationBodyReader interface {
	ConversationBody(context.Context, string, executionmodel.ConversationBodyRef, int) (ConversationBodyRecord, error)
}

type ConversationCapabilityReader interface {
	ConversationCapabilityReports(context.Context, executionbackend.BackendKind, executionbackend.ExecutionProtocol, string, int) ([]executionmodel.ConversationCapabilityReport, error)
}

type ConversationReader interface {
	ConversationEventReader
	ConversationBodyReader
	ConversationCapabilityReader
}

type ConversationEventRecord struct {
	EventID                               int64
	ProjectID, TaskID, CandidateID, RunID string
	Event                                 runnercontract.Event
}

// ConversationFilter selects immutable Runner final facts, not display DTOs.
type ConversationFilter struct {
	FinalsOnly                                                    bool
	WorkflowID, ExecutionID, AttemptID, SessionID, TurnID, Source string
}

type ConversationBodyRecord struct {
	Body         []byte
	TotalBytes   int64
	SHA256       string
	Completeness string
}
