package steerline

import (
	"context"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/query"
)

type ConversationFilter = contracts.ConversationFilter
type ConversationPage = contracts.ConversationPage
type ConversationBodyPage = contracts.ConversationBodyPage
type ConversationRawPage = contracts.ConversationRawPage
type ConversationQueryService = query.ConversationService

// NewConversationQueryService wires read capabilities; the caller retains connection ownership.
func NewConversationQueryService(store *SQLiteStore, project string) ConversationQueryService {
	if store == nil {
		return ConversationQueryService{Project: project}
	}
	reader := store.reader()
	return ConversationQueryService{Project: project, Events: reader, Bodies: reader, Raw: reader, Capabilities: reader}
}
func ConversationCursorAfter(project string, filter ConversationFilter, after int64) (string, error) {
	return query.ConversationCursorAfter(project, filter, after)
}
func (s *SQLiteStore) ConversationCapabilityReports(ctx context.Context, backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, targetID string, limit int) ([]ConversationCapabilityReport, error) {
	return NewConversationQueryService(s, "").ReadCapabilities(ctx, backend, protocol, targetID, limit)
}
