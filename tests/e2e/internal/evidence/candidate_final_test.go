package evidence

import (
	"testing"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

func TestCandidateFinalCountKeepsDistinctItemsOnOneNativeTurn(t *testing.T) {
	final := productapi.ConversationEvent{Type: productapi.ConversationMessageComplete,
		Phase: productapi.MessagePhaseFinalAnswer, SourceRole: productapi.ConversationSourceCanonical,
		SessionID: "original-session", TurnID: "same-native-turn", ItemKey: "readiness-item",
		Body: &productapi.ConversationBodyRef{ItemKey: "readiness-item"}}
	business := final
	business.ItemKey, business.Body = "business-item", &productapi.ConversationBodyRef{ItemKey: "business-item"}
	fallback := business
	fallback.SourceRole = productapi.ConversationSourceFallback
	snapshot := business
	snapshot.Type = productapi.ConversationMessageSnapshot
	count, err := CandidateFinalCount([]productapi.ConversationEvent{final, business, final, fallback, snapshot}, final.SessionID)
	if err != nil || count != 2 {
		t.Fatalf("distinct final items or replay filtering lost: count=%d err=%v", count, err)
	}
	for _, field := range []string{"session", "turn", "item", "body"} {
		t.Run("missing or foreign "+field, func(t *testing.T) {
			invalid := final
			switch field {
			case "session":
				invalid.SessionID = "sibling-session"
			case "turn":
				invalid.TurnID = ""
			case "item":
				invalid.ItemKey = ""
			case "body":
				invalid.Body = nil
			}
			if _, err := CandidateFinalCount([]productapi.ConversationEvent{invalid}, final.SessionID); err == nil {
				t.Fatal("unbound canonical final was counted")
			}
		})
	}
}
