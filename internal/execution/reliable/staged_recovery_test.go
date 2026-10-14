package reliable

import (
	"testing"
	"time"
)

func TestReliableRecoveredNativeStartRequiresOriginalIntent(t *testing.T) {
	s := TurnState{SessionID: "session", PendingTurnID: "original", CurrentTurnID: "previous", LastStoppedTurnID: "previous"}
	if next, err := BeginTurn(s, "session", "original", TurnAuthority{Allowed: true, OriginalOnly: true}, time.Now()); err != nil || next.CurrentTurnID != "original" || next.Status != "active" {
		t.Fatalf("original reconciled start rejected: %+v %v", next, err)
	}
	for _, pending := range []string{"", "other"} {
		s.PendingTurnID = pending
		if _, err := BeginTurn(s, "session", "original", TurnAuthority{Allowed: true, OriginalOnly: true}, time.Now()); err == nil {
			t.Fatal("unknown outcome authorized an unrelated input")
		}
	}
}
