package viewer

import "github.com/charmbracelet/lipgloss"

// Styles are layout-only where possible; individual log lines are colored by
// steerline.RenderActivity so the viewer and the stream stay visually consistent.
var (
	styleHeader  = lipgloss.NewStyle().Bold(true)
	styleFaint   = lipgloss.NewStyle().Faint(true)
	styleSel     = lipgloss.NewStyle().Bold(true).Reverse(true)
	stylePending = lipgloss.NewStyle().Bold(true)
	styleBox     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	styleLabel   = lipgloss.NewStyle().Bold(true)
)

func runBadge(r *runRow) string {
	if r == nil {
		return ""
	}
	switch {
	case r.pending:
		return "DECIDE"
	case r.run.FinishTaskStatus == "waiting_pr_merge":
		return "MERGE"
	case r.run.FinishTaskStatus == "completed":
		return "DONE"
	case r.run.FinishTaskStatus == "blocked":
		return "BLOCK"
	case r.run.ExecutionStatus == "running":
		return "RUN"
	}
	return "IDLE"
}
