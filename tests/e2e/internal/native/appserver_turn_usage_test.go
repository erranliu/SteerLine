package native

import (
	"strings"
	"testing"
)

func TestResumedAppServerUsageRequiresStartAndRejectsLateTurns(t *testing.T) {
	previous := `{"method":"thread/tokenUsage/updated","params":{"threadId":"session","turnId":"previous"}}` + "\n"
	start := `{"method":"turn/started","params":{"threadId":"session","turn":{"id":"current"}}}` + "\n"
	current := `{"method":"thread/tokenUsage/updated","params":{"threadId":"session","turnId":"current","tokenUsage":{"total":{"inputTokens":10,"outputTokens":5,"totalTokens":15}}}}` + "\n"
	u, proof, err := AppServerTurnUsage([]byte(previous+start+current), "session", "current")
	if err != nil || u.Tokens.Total != 15 || !strings.Contains(string(proof), "turn/started") || strings.Contains(string(proof), "previous") {
		t.Fatalf("resumed proof: %+v %s %v", u, proof, err)
	}
	for _, raw := range []string{previous + current, start + current + previous, strings.Replace(previous, "session", "foreign", 1) + start + current} {
		if _, _, err := AppServerTurnUsage([]byte(raw), "session", "current"); err == nil {
			t.Fatal("missing start/foreign session/late turn accepted")
		}
	}
}
