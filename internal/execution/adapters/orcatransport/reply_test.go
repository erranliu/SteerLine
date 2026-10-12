package orcatransport

import (
	"strings"
	"testing"
)

func TestRenderedReplyPreservesWhitespaceAndWrappedJSON(t *testing.T) {
	lines := []string{"› Report the implementation.", "  Return one JSON object.", "• Checking results.", "• Ran git status", "  └ clean", `• {"outcome":"done","summary":"two words 中文 `, `  🙂","outputs":{"pr_url":"https://github.com/erranliu/`, `  SteerlineTest/pull/1"}}`, "  Worked for 12s • 18:50", "› Ask Codex to do anything"}
	text, _, problem := renderedReply("codex", lines)
	want := `{"outcome":"done","summary":"two words 中文 \n🙂","outputs":{"pr_url":"https://github.com/erranliu/SteerlineTest/pull/1"}}`
	if problem != "" || text != want {
		t.Fatalf("body changed: %q, %s", text, problem)
	}
	plain := []string{"› Implement.", "• Finished.", "  ", "  Two words and 中文🙂.", "  Worked for 1s • 18:50", "› Ask Codex to do anything"}
	text, _, problem = renderedReply("codex", plain)
	if problem != "" || text != "Finished.\n\nTwo words and 中文🙂." {
		t.Fatalf("plain body changed: %q %s", text, problem)
	}
}

func TestRenderedReplyAcceptsCollapsedInputAndClippedToolHistory(t *testing.T) {
	lines := []string{
		"› Implement the following Spec and Plan. Spec: docs/spec/native-105.spec.md Pl…",
		"     1 -# Native task 105 evidence", "    + Show details",
		"• Ran git commit", "  └ committed",
		"• Implemented the evidence-only candidate.",
		"  Committed locally as 2374d57. Publication remains for the publish step.",
		"  Worked for 1m 47s • 10:35", "› Ask Codex to do anything",
	}
	text, fingerprint, problem := renderedReply("codex", lines)
	want := "Implemented the evidence-only candidate.\nCommitted locally as 2374d57. Publication remains for the publish step."
	if problem != "" || text != want || fingerprint != screenBaseline(lines) {
		t.Fatalf("collapsed input blocked a complete reply: %q, %s", text, problem)
	}
}

func TestRenderedReplyRejectsMissingBoundariesEchoAndTools(t *testing.T) {
	for name, lines := range map[string][]string{
		"clipped input":   {"• Reply.", "  Worked for 1s • 18:50"},
		"running":         {"› New input.", "• Commentary."},
		"echo only":       {"› New input.", "  Worked for 1s • 18:50"},
		"tool only":       {"› New input.", "• Ran git status", "  └ clean", "  Worked for 1s • 18:50"},
		"collapsed reply": {"› New input.", "• Summary", "  + 4 lines (ctrl+t to expand)", "  Worked for 1s • 18:50"},
		"invalid JSON":    {"› New input.", `• {"summary":"incomplete`, "  Worked for 1s • 18:50"},
	} {
		t.Run(name, func(t *testing.T) {
			text, _, problem := renderedReply("codex", lines)
			if text != "" || problem == "" {
				t.Fatalf("unverified result accepted: %q", text)
			}
		})
	}
	if _, _, problem := renderedReply("opaque", []string{}); !strings.Contains(problem, "unsupported") {
		t.Fatal("unknown renderer accepted")
	}
}
