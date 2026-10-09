package console

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestCandidatePRLinkKeepsTargetWhenClippedAndSwitched(t *testing.T) {
	m, _ := ccFixture(t, 2)
	row := m.selectedRow(0)
	for _, raw := range []string{"https://github.com/erranliu/SteerlineTest/pull/177", "http://forge.example/team/repo/pulls/178"} {
		row.run.PR.URL = &raw
		row.run.PR.State = "open"
		for _, width := range []int{8, 40, 90} {
			lines := candidateOverviewLines(m, m.projects[0], row, width)
			var link string
			for _, line := range lines {
				if strings.HasPrefix(ansi.Strip(line), "PR：") {
					link = line
				}
			}
			if !strings.Contains(link, "\x1b]8;;"+raw+"\x1b\\") || !strings.Contains(link, "\x1b]8;;\x1b\\") || lipgloss.Width(link) > width {
				t.Fatalf("width %d lost target, closing sequence or layout: %q", width, link)
			}
			if width >= 40 && !strings.Contains(ansi.Strip(link), shortPR(raw)) {
				t.Fatalf("PR number is missing: %q", link)
			}
		}
	}
	row.run.PR.URL = nil
	if strings.Contains(strings.Join(candidateOverviewLines(m, m.projects[0], row, 90), "\n"), "\x1b]8;;") {
		t.Fatal("missing PR reused a previous hyperlink")
	}
}

func TestPRLinkRejectsUnsafeTargetsAndEscapesLabels(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "file:///C:/test", "/pull/177", "https://example.test/\x1b]8;;evil", "https://example.test/\x07", "https://example.test/\u009c", "https://example.test/\u202e177", "https://user@example.test/pull/177"} {
		if got := prLink(raw, "#177"); strings.Contains(got, "\x1b") {
			t.Fatalf("unsafe target became a link: %q", got)
		}
	}
	got := prLink("https://example.test/pull/177", "#177\x1b[2J")
	if strings.Contains(ansi.Strip(got), "\x1b") || !strings.Contains(ansi.Strip(got), "␛[2J") {
		t.Fatalf("label inserted terminal controls: %q", got)
	}
}
