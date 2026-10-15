package evidence

import (
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func CandidateFinalCount(events []api.ConversationEvent, session string) (int, error) {
	if session == "" {
		return 0, errors.New("original session missing")
	}
	items := map[string]bool{}
	for _, e := range events {
		if e.Type != api.ConversationMessageComplete || e.Phase != api.MessagePhaseFinalAnswer || e.SourceRole != api.ConversationSourceCanonical {
			continue
		}
		if e.SessionID != session || e.TurnID == "" || e.ItemKey == "" || e.Body == nil {
			return 0, errors.New("canonical final lost its original native identity")
		}
		items[e.ItemKey] = true
	}
	return len(items), nil
}
