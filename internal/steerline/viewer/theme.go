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

// deliveryBadge is the primary badge. It reflects the SteerLine delivery axis
// (finish_task_status plus the pending-decision pause), not task status.
func deliveryBadge(r *runRow) string {
	if r == nil || r.run == nil {
		return "IDLE"
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
		return "ACTIVE"
	}
	return "IDLE"
}

// badgeColor picks a color for a delivery badge; delimiters are plain text so
// they can be width-measured reliably.
func badgeColor(badge string) lipgloss.Style {
	switch badge {
	case "DECIDE":
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	case "BLOCK":
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	case "MERGE":
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	case "DONE":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	case "ACTIVE":
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	}
	return lipgloss.NewStyle().Faint(true)
}
