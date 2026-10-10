package app

import (
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
)

// The retained Terminal adapter has no product registration. Supervised workers
// collect step replies through worker-read and persist their bound content in
// the center's conversation store for both targets.
func newConversationSources() conversationtypes.ConversationSourcePort {
	return codexworker.NewConversationAdapter()
}
