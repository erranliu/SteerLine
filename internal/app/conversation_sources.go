package app

import (
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
)

// The retained Terminal adapter has no product registration. Supervised workers
// use their current completion-message behavior for both execution targets.
func newConversationSources() conversationtypes.ConversationSourcePort {
	return codexworker.NewConversationAdapter()
}
