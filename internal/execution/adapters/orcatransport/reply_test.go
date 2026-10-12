package orcatransport

import (
	"strings"
	"testing"
)

func TestRenderedReplyPreservesWhitespaceAndWrappedJSON(t *testing.T) {
	prompt := "Report the implementation.\nReturn one JSON object."
	lines := []string{"› Report the implementation.", "  Return one JSON object.", "• Checking results.", "• Ran git status", "  └ clean", `• {"outcome":"done","summary":"two words 中文 `, `  🙂","outputs":{"pr_url":"https://github.com/erranliu/`, `  SteerlineTest/pull/1"}}`, "  Worked for 12s • 18:50", "› Ask Codex to do anything"}
	text, _, problem := renderedReply("codex", lines, prompt)
	want := `{"outcome":"done","summary":"two words 中文 \n🙂","outputs":{"pr_url":"https://github.com/erranliu/SteerlineTest/pull/1"}}`
	if problem != "" || text != want {
		t.Fatalf("body changed: %q, %s", text, problem)
	}
	plain := []string{"› Implement.", "• Finished.", "  ", "  Two words and 中文🙂.", "  Worked for 1s • 18:50", "› Ask Codex to do anything"}
	text, _, problem = renderedReply("codex", plain, "Implement.")
	if problem != "" || text != "Finished.\n\nTwo words and 中文🙂." {
		t.Fatalf("plain body changed: %q %s", text, problem)
	}
}

func TestRenderedReplyRejectsMissingBoundariesEchoToolsAndWrongInput(t *testing.T) {
	for name, lines := range map[string][]string{
		"old reply":       {"› Old input.", "• Old reply.", "  Worked for 1s • 18:50"},
		"clipped input":   {"• Reply.", "  Worked for 1s • 18:50"},
		"running":         {"› New input.", "• Commentary."},
		"echo only":       {"› New input.", "  Worked for 1s • 18:50"},
		"tool only":       {"› New input.", "• Ran git status", "  └ clean", "  Worked for 1s • 18:50"},
		"collapsed reply": {"› New input.", "• Summary", "  + 4 lines (ctrl+t to expand)", "  Worked for 1s • 18:50"},
		"invalid JSON":    {"› New input.", `• {"summary":"incomplete`, "  Worked for 1s • 18:50"},
	} {
		t.Run(name, func(t *testing.T) {
			text, _, problem := renderedReply("codex", lines, "New input.")
			if text != "" || problem == "" {
				t.Fatalf("unverified result accepted: %q", text)
			}
		})
	}
	if _, _, problem := renderedReply("opaque", []string{}, "New input."); !strings.Contains(problem, "unsupported") {
		t.Fatal("unknown renderer accepted")
	}
}
