package app

import (
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
)

// The retained Terminal adapter has no product registration. Supervised workers
// publish verified declared finals directly through the center's conversation
// store for both targets; no external history reader is required for them.
func newConversationSources() conversationtypes.ConversationSourcePort {
	return codexworker.NewConversationAdapter()
}
