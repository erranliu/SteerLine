package viewer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/erranliu/SteerLine/internal/steerline"
)

const helpKeys = "↑↓ select · a answer · f follow · pgup/pgdn scroll · r reload · q quit"

func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "loading…"
	}
	if m.modal != nil {
		return m.viewModal()
	}
	header := styleHeader.Render("SteerLine 查看器")
	if m.err != nil {
		header += "  " + styleFaint.Render("err: "+truncate(m.err.Error(), 80))
	}
	bodyH := m.height - 2
	if bodyH < 3 {
		bodyH = 3
	}
	leftW := m.width / 3
	if leftW < 22 {
		leftW = 22
	}
	if leftW > 40 {
		leftW = 40
	}
	rightW := m.width - leftW - 1
	if m.width < 64 || rightW < 12 {
		body := strings.Join(pane(m.viewRuns(m.width, bodyH), m.width, bodyH), "\n")
		return header + "\n" + body + "\n" + m.viewFooter()
	}
	body := joinPanes(bodyH, pane(m.viewRuns(leftW, bodyH), leftW, bodyH), pane(m.viewLog(rightW, bodyH), rightW, bodyH))
	return header + "\n" + body + "\n" + m.viewFooter()
}

func (m Model) viewRuns(w, h int) []string {
	body := h - 1
	if body < 1 {
		body = 1
	}
	start := 0
	if m.sel >= body {
		start = m.sel - body + 1
	}
	lines := []string{styleLabel.Render("Runs")}
	if len(m.rows) == 0 {
		lines = append(lines, styleFaint.Render("(no runs)"))
	}
	for i := start; i < len(m.rows) && len(lines) <= body; i++ {
		r := m.rows[i]
		mark := " "
		if r.pending {
			mark = "!"
		}
		titleBudget := w - 20
		if titleBudget < 4 {
			titleBudget = 4
		}
		line := fmt.Sprintf("%s %-9s %-6s %s", mark, truncate(r.run.TaskID, 9), runBadge(&r), truncate(r.run.Title, titleBudget))
		if i == m.sel {
			line = styleSel.Render(line)
		}
		lines = append(lines, line)
	}
	return lines
}

func (m Model) viewLog(w, h int) []string {
	title := "Activity"
	if r := m.current(); r != nil {
		title = "Activity · " + r.TaskID
	}
	mode := "follow"
	if !m.follow {
		mode = "paused"
	}
	lines := []string{styleLabel.Render(title) + "  " + styleFaint.Render("["+mode+"]")}
	body := h - 1
	if body < 1 {
		body = 1
	}
	var rendered []string
	for _, ev := range m.log {
		for _, ln := range strings.Split(steerline.RenderActivity(ev, steerline.RenderOptions{Color: m.opts.Color, MaxColumns: w}), "\n") {
			rendered = append(rendered, ln)
		}
	}
	end := len(rendered) - m.logScroll
	if end > len(rendered) {
		end = len(rendered)
	}
	if end < body {
		end = body
	}
	start := end - body
	if start < 0 {
		start = 0
	}
	for i := start; i < end && i < len(rendered); i++ {
		lines = append(lines, rendered[i])
	}
	return lines
}

func (m Model) viewFooter() string {
	if r := m.current(); isPending(r) {
		return stylePending.Render("! decision pending for "+r.TaskID+" — press a to answer") + "  " + styleFaint.Render(helpKeys)
	}
	if m.status != "" {
		return styleFaint.Render(m.status) + "  " + styleFaint.Render(helpKeys)
	}
	return styleFaint.Render(helpKeys)
}

func (m Model) viewModal() string {
	mo := m.modal
	req := mo.run.Decision.Request
	var b strings.Builder
	b.WriteString(styleHeader.Render("Decision · "+mo.run.TaskID) + "\n\n")
	b.WriteString(req.Question + "\n\n")
	for i, o := range req.Options {
		cur := "  "
		if i == mo.choice {
			cur = "▸ "
		}
		label := fmt.Sprintf("%s(%d) %s  %s", cur, i+1, o.ID, truncate(o.Label, 60))
		if i == mo.choice {
			label = styleSel.Render(label)
		}
		b.WriteString(label + "\n")
	}
	if req.Context != "" {
		b.WriteString("\n" + styleFaint.Render(truncate(req.Context, 240)) + "\n")
	}
	note := "note: " + mo.note
	if mo.noteFocus {
		note += "_"
	}
	b.WriteString("\n" + note + "\n\n")
	if mo.err != "" {
		b.WriteString(stylePending.Render("error: "+mo.err) + "\n")
	}
	b.WriteString(styleFaint.Render("[↑↓/1-9] choose · [tab] note · [enter] submit · [esc] cancel"))
	width := 78
	if m.width-4 < width {
		width = m.width - 4
	}
	if width < 20 {
		width = 20
	}
	box := styleBox.Width(width).Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// pane pads/truncates lines to exactly h rows (used for tests that only inspect
// content, not exact padding).
func pane(lines []string, w, h int) []string {
	out := make([]string, 0, h)
	style := lipgloss.NewStyle().Width(w).MaxWidth(w)
	for i := 0; i < h; i++ {
		if i < len(lines) {
			out = append(out, style.Render(lines[i]))
		} else {
			out = append(out, style.Render(""))
		}
	}
	return out
}

func joinPanes(h int, left, right []string) string {
	var b strings.Builder
	for i := 0; i < h; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		b.WriteString(l + " " + r)
	}
	return b.String()
}
