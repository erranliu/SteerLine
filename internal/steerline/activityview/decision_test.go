package activityview

import (
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/execution"
)

func TestDecisionAnswerSummary(t *testing.T) {
	for _, tc := range []struct {
		name, option, note, want string
	}{
		{"empty", "", "", ""},
		{"free_text", "", "继续检查原会话", "继续检查原会话"},
		{"option", "continue", "", "continue: 继续"},
		{"option_and_note", "continue", "保留原身份", "continue: 继续 — 保留原身份"},
		{"unknown_option", "old-option", "保留备注", "old-option"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := contracts.Run{Decision: &contracts.DecisionState{OptionID: tc.option, Note: tc.note,
				Request: contracts.DecisionRequest{Options: []execution.DecisionOption{{ID: "continue", Label: "继续"}}}}}
			if got := DecisionAnswerSummary(&run); got != tc.want {
				t.Fatalf("summary=%q, want %q", got, tc.want)
			}
		})
	}
	if got := DecisionAnswerSummary(&contracts.Run{}); got != "" {
		t.Fatalf("run without decision: %q", got)
	}
}
