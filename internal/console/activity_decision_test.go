package console

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"testing"
)

func TestTM05DecisionAnswerSummary(t *testing.T) {
	for _, tc := range []struct{ name, option, note, want string }{
		{"empty", "", "", ""},
		{"free_text", "", "继续检查原会话", "继续检查原会话"},
		{"option", "continue", "", "continue: 继续"},
		{"option_and_note", "continue", "保留原身份", "continue: 继续 — 保留原身份"},
		{"unknown_option", "old-option", "保留备注", "old-option"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &api.CandidateSummary{Decision: &api.DecisionSummary{OptionID: tc.option, Note: tc.note, Request: api.DecisionRequest{Options: []api.DecisionOption{{ID: "continue", Label: "继续"}}}}}
			if got := DecisionAnswerSummary(r); got != tc.want {
				t.Fatalf("summary=%q, want %q", got, tc.want)
			}
		})
	}
	if got := DecisionAnswerSummary(&api.CandidateSummary{}); got != "" {
		t.Fatalf("candidate without decision: %q", got)
	}
}
