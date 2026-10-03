package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/query"
)

type ConversationFilter = contracts.ConversationFilter
type ConversationPage = contracts.ConversationPage
type ConversationBodyPage = contracts.ConversationBodyPage
type ConversationRawPage = contracts.ConversationRawPage
type ConversationQueryService = query.ConversationService

// NewConversationQueryService wires read capabilities; the caller retains connection ownership.
func NewConversationQueryService(store conversationReader, project string) ConversationQueryService {
	if store == nil {
		return ConversationQueryService{Project: project}
	}
	reader := store
	return ConversationQueryService{Project: project, Events: reader, Bodies: reader, Raw: reader, Capabilities: reader}
}
func ConversationCursorAfter(project string, filter ConversationFilter, after int64) (string, error) {
	return query.ConversationCursorAfter(project, filter, after)
}

type conversationReader interface {
	query.ConversationEventReader
	query.ConversationBodyReader
	query.ConversationRawReader
	query.ConversationCapabilityReader
}
