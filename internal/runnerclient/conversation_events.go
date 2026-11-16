package runnerclient

import "slices"

const ConversationEventsCommand = "conversation-events"
const ConversationEventsContract = "steerrunner-conversation-events/v1"

func (c *Catalog) SupportsConversationEvents() bool {
	if c == nil {
		return false
	}
	binding := c.Capabilities.InstanceBinding.FullBinding
	return binding != nil && binding.BeforeEffects && binding.NoImplicitStart &&
		slices.Contains(binding.Commands, ConversationEventsCommand)
}
