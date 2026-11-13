package maintenance

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestDecisionRoleBoundary(t *testing.T) {
	if !json.Valid(ReportSchema()) {
		t.Fatal("invalid output schema")
	}
	for _, tc := range []struct {
		mode           string
		accept, finish bool
	}{
		{"first", false, true}, {"select", false, true}, {"audit", true, false},
		{"finish", false, false},
	} {
		var schema struct {
			Properties map[string]struct{ Enum []string }
		}
		if err := json.Unmarshal(Schema(tc.mode), &schema); err != nil {
			t.Fatal(tc.mode, err)
		}
		kinds := schema.Properties["kind"].Enum
		if slices.Contains(kinds, "accept") != tc.accept || slices.Contains(kinds, "finish") != tc.finish ||
			!slices.Contains(kinds, "execute") || !slices.Contains(kinds, "ask") || !slices.Contains(kinds, "blocked") {
			t.Fatal(tc.mode, kinds)
		}
	}
	for _, v := range []struct {
		body, mode string
		valid      bool
	}{
		{`{"kind":"execute","prompt":"验证并修改","summary":"有限切片"}`, "select", true},
		{`{"kind":"accept","prompt":"","summary":"证据充分"}`, "audit", true},
		{`{"kind":"accept","prompt":"","summary":""}`, "select", false},
		{`{"kind":"finish","prompt":"","summary":""}`, "finish", false},
		{`{"kind":"execute","prompt":"","summary":""}`, "select", false},
		{`{"kind":"ask","summary":"needs a decision"}`, "audit", false},
		{`{"kind":"execute","prompt":"x","summary":"","tools":["write"]}`, "select", false},
		{`{"kind":"ask","summary":"范围冲突","question":{"prompt":"如何处理？","facts":["依赖公开接口"],"attempts":[],"options":[{"id":"keep","label":"保留","impact":"保持兼容"}],"recommended_option_id":"keep","recommendation_reason":"低风险"}}`, "first", true},
		{`{"kind":"ask","summary":"范围冲突","prompt":"execute","question":{"prompt":"如何处理？","facts":["依赖公开接口"],"attempts":[],"options":[{"id":"keep","label":"保留","impact":"保持兼容"}],"recommended_option_id":"keep","recommendation_reason":"低风险"}}`, "audit", false},
		{`{"kind":"accept","summary":"证据充分"}`, "evidence", false},
		{`{"kind":"accept","summary":"已交付"}`, "finish", false},
	} {
		_, e := Accept([]byte(v.body), v.mode)
		if (e == nil) != v.valid {
			t.Fatal(v, e)
		}
	}
}

func TestReviewOnlyReturnsRecommendationAndReport(t *testing.T) {
	if _, err := AcceptReview([]byte(`{"recommendation":"revise","report":"新增结构债"}`)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"recommendation":"merge","report":"评审","prompt":"同步正文"}`,
		`{"recommendation":"merge","report":""}`,
		`{"recommendation":"approve","report":"评审"}`,
	} {
		if _, err := AcceptReview([]byte(raw)); err == nil {
			t.Fatal("accepted", raw)
		}
	}
}
