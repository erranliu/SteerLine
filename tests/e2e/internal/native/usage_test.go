package native

import (
	"errors"
	"strings"
	"testing"
)

func TestAppServerUsageBindsIdentityAndDoesNotSumCumulativeUpdates(t *testing.T) {
	event := `{"method":"thread/tokenUsage/updated","params":{"threadId":"session","turnId":"turn","tokenUsage":{"total":{"inputTokens":100,"cachedInputTokens":20,"outputTokens":30,"reasoningOutputTokens":10,"totalTokens":130}}}}` + "\n"
	u, _, err := AppServerUsage([]byte(event+event), "session", "turn")
	if err != nil || u.Tokens.Total != 130 {
		t.Fatalf("cumulative usage duplicated: %+v %v", u, err)
	}
	for _, raw := range []string{"", event + "{", strings.ReplaceAll(event, "130", "0"), strings.ReplaceAll(event, "session", "other"), strings.ReplaceAll(event, "turnId\":\"turn", "turnId\":\"other")} {
		if _, _, err := AppServerUsage([]byte(raw), "session", "turn"); err == nil {
			t.Fatalf("invalid usage accepted: %s", raw)
		}
	}
}
func TestRolloutUsageRequiresOwnedCwdAndPinnedModelEffort(t *testing.T) {
	raw := `{"type":"session_meta","payload":{"id":"session","cwd":"C:/owned"}}
{"type":"turn_context","payload":{"turn_id":"turn","model":"gpt-6-luna","effort":"medium"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":30,"reasoning_output_tokens":10,"total_tokens":130}}}}
{"type":"event_msg","payload":{"type":"task_complete","turn_id":"turn"}}
`
	u, _, err := RolloutUsage([]byte(raw), "C:/owned")
	if err != nil || u.Tokens.Total != 130 || u.Session != "session" {
		t.Fatalf("%+v %v", u, err)
	}
	for _, bad := range []string{strings.ReplaceAll(raw, "C:/owned", "C:/other"), strings.ReplaceAll(raw, "medium", "high"), strings.ReplaceAll(raw, "gpt-6-luna", "gpt-6-sol"), strings.ReplaceAll(raw, "130", "0")} {
		if _, _, err := RolloutUsage([]byte(bad), "C:/owned"); err == nil {
			t.Fatal("unbound usage accepted")
		}
	}
}

func TestRolloutUsageWaitsForMatchingTurnCompletion(t *testing.T) {
	pending := `{"type":"session_meta","payload":{"id":"session","cwd":"C:/owned"}}
{"type":"event_msg","payload":{"type":"task_started","turn_id":"turn"}}
{"type":"turn_context","payload":{"turn_id":"turn","model":"gpt-6-luna","effort":"medium"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":30,"reasoning_output_tokens":10,"total_tokens":130}}}}
`
	u, _, err := RolloutUsage([]byte(pending), "C:/owned")
	if !errors.Is(err, ErrNativeTurnPending) || u.Turn != "turn" {
		t.Fatalf("unfinished usage became a final baseline: %+v %v", u, err)
	}
	final := pending + `{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":110,"cached_input_tokens":20,"output_tokens":35,"reasoning_output_tokens":10,"total_tokens":145}}}}
{"type":"event_msg","payload":{"type":"task_complete","turn_id":"turn"}}
`
	u, _, err = RolloutUsage([]byte(final), "C:/owned")
	if err != nil || u.Turn != "turn" || u.Tokens.Total != 145 {
		t.Fatalf("final native total missing: %+v %v", u, err)
	}
	if _, _, err := RolloutUsage([]byte(pending+`{"type":"event_msg","payload":{"type":"task_complete","turn_id":"other"}}`), "C:/owned"); err == nil {
		t.Fatal("another turn completed the usage baseline")
	}
	if _, _, err := RolloutUsage([]byte(final+`{"type":"event_msg","payload":{"type":"task_started","turn_id":"next"}}`), "C:/owned"); !errors.Is(err, ErrNativeTurnPending) {
		t.Fatal("an earlier completed turn hid a later active turn")
	}
	if _, _, err := RolloutUsage([]byte(final+`{"type":"event_msg","payload":{"type":"task_started","turn_id":"next"}}
{"type":"event_msg","payload":{"type":"task_complete","turn_id":"next"}}`), "C:/owned"); err == nil || errors.Is(err, ErrNativeTurnPending) {
		t.Fatal("a later completed turn inherited another turn's model/usage evidence")
	}
}
