package viewer

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/erranliu/SteerLine/internal/steerline"
)

const helpKeys = "↑↓ select · a answer · [ ] tab · v view · f follow · g/G pgup/pgdn scroll · r reload · q quit"

func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "loading…"
	}
	if m.modal != nil {
		return m.viewModal()
	}
	header := styleHeader.Render("SteerLine 查看器")
	status := m.viewStatusBar(m.width)
	bodyH := m.height - 3
	if bodyH < 3 {
		bodyH = 3
	}
	leftW, rightW := paneWidths(m.width)
	if m.width < 64 || rightW < 12 {
		body := strings.Join(pane(m.viewRuns(m.width, bodyH), m.width, bodyH), "\n")
		return header + "\n" + status + "\n" + body + "\n" + m.viewFooter(m.width)
	}
	body := joinPanes(bodyH, pane(m.viewRuns(leftW, bodyH), leftW, bodyH), pane(m.viewRight(rightW, bodyH), rightW, bodyH))
	return header + "\n" + status + "\n" + body + "\n" + m.viewFooter(m.width)
}

// --- L2 run detail ---

const detailTabCount = 4

var detailTabs = []string{"Summary", "Workspace", "PR", "Decision"}
var detailTabsCompact = []string{"Sum", "Work", "PR", "Dec"}

// paneWidths splits the terminal into the run-list and detail/activity columns.
// The list gets 46% so task titles stay readable; the detail pane keeps the rest.
func paneWidths(width int) (left, right int) {
	left = width * 46 / 100
	if left < 40 {
		left = 40
	}
	if left > 72 {
		left = 72
	}
	return left, width - left - 1
}

// viewRight stacks the detail pane above the activity pane in the right column.
func (m Model) viewRight(w, h int) []string {
	detailH := h * 2 / 5
	if detailH < 8 {
		detailH = 8
	}
	if detailH > 20 {
		detailH = 20
	}
	if detailH > h-3 {
		detailH = h - 3
	}
	if detailH < 2 {
		detailH = 2
	}
	out := m.viewDetail(w, detailH-1)
	out = append(out, styleFaint.Render(strings.Repeat("─", w)))
	return append(out, m.viewLog(w, h-detailH)...)
}

func (m Model) viewDetail(w, h int) []string {
	r := m.current()
	if r == nil {
		return []string{styleLabel.Render("Detail"), styleFaint.Render("  (no run selected)")}
	}
	prefix := "Detail · " + r.TaskID
	tabBar := m.tabBar(detailTabs)
	if lipgloss.Width(prefix)+lipgloss.Width(tabBar) > w {
		tabBar = m.tabBar(detailTabsCompact)
	}
	if r.Title != "" {
		// Reserve room for the tab bar; only show the title when it fits.
		budget := w - lipgloss.Width(prefix) - lipgloss.Width(tabBar) - 3
		if budget >= 8 {
			prefix += " · " + truncate(r.Title, budget)
		}
	}
	lines := []string{styleLabel.Render(truncate(prefix+tabBar, w))}
	content := m.detailContent(r, w)
	body := h - len(lines)
	if body < 1 {
		body = 1
	}
	if len(content) > body {
		content = content[:body]
	}
	return append(lines, content...)
}

func (m Model) tabBar(labels []string) string {
	var b strings.Builder
	for i, name := range labels {
		if i == m.detailTab {
			b.WriteString(" [" + name + "]")
		} else {
			b.WriteString(" " + name)
		}
	}
	return b.String()
}

func (m Model) detailContent(r *steerline.Run, w int) []string {
	td, hasTD := m.taskDetail[r.TaskID]
	errMsg := m.taskErr[r.TaskID]
	switch m.detailTab {
	case 0:
		return detailSummary(r, td, hasTD, errMsg, w)
	case 1:
		return detailWorkspace(r, w)
	case 2:
		return detailPR(r, w)
	case 3:
		return detailDecision(r, w)
	default:
		return detailSummary(r, td, hasTD, errMsg, w)
	}
}

type detailKV struct{ k, v string }

func detailFields(kvs []detailKV, w int) []string {
	// " " + label(labelW) + " " + value == labelW+2+valW columns.
	labelW := 13
	valW := w - labelW - 2
	if valW < 6 {
		valW = 6
	}
	var out []string
	for _, e := range kvs {
		v := e.v
		if v == "" {
			v = "—"
		}
		lines := wrapText(v, valW)
		if len(lines) == 0 {
			lines = []string{"—"}
		}
		for i, ln := range lines {
			key := ""
			if i == 0 {
				key = e.k
			}
			out = append(out, " "+styleFaint.Render(fmt.Sprintf("%-*s", labelW, key))+" "+ln)
		}
	}
	return out
}

func detailSummary(r *steerline.Run, td steerline.TaskDetail, hasTD bool, errMsg string, w int) []string {
	// Key lifecycle facts first, so they survive a short pane; task metadata
	// next; long free text last.
	kvs := []detailKV{
		{"delivery", r.FinishTaskStatus},
		{"execution", r.ExecutionStatus},
		{"error", ptrStr(r.Error)},
		{"summary", ptrStr(r.Summary)},
	}
	switch {
	case hasTD:
		kvs = append(kvs,
			detailKV{"type", td.Type},
			detailKV{"priority", ptrInt(td.Priority)},
			detailKV{"task", td.Status},
			detailKV{"assignee", td.Assignee},
			detailKV{"labels", strings.Join(td.Labels, ", ")},
			detailKV{"spec", td.SpecID},
		)
	case errMsg != "":
		kvs = append(kvs, detailKV{"task", "unavailable: " + errMsg})
	default:
		kvs = append(kvs, detailKV{"task", "loading…"})
	}
	kvs = append(kvs,
		detailKV{"task", r.TaskStatus},
		detailKV{"started", ptrTime(r.StartedAt)},
		detailKV{"ended", ptrTime(r.EndedAt)},
		detailKV{"elapsed", elapsedShort(r)},
		detailKV{"host", r.Host},
		detailKV{"claimant", r.Claimant},
		detailKV{"run", r.RunID},
		detailKV{"last_event", ptrStr(r.LastEvent)},
	)
	if hasTD && td.Description != "" {
		kvs = append(kvs, detailKV{"desc", clipText(td.Description, 600)})
	}
	return detailFields(kvs, w)
}

func detailWorkspace(r *steerline.Run, w int) []string {
	kvs := []detailKV{
		{"worktree", ptrStr(r.WorktreePath)},
		{"branch", ptrStr(r.WorktreeBranch)},
		{"base", ptrStr(r.BaseCommit)},
	}
	if wi := r.WorkspaceIntent; wi != nil {
		kvs = append(kvs,
			detailKV{"intent_path", wi.Path},
			detailKV{"intent_branch", wi.Branch},
			detailKV{"intent_base", wi.BaseCommit},
		)
	}
	return detailFields(kvs, w)
}

func detailPR(r *steerline.Run, w int) []string {
	return detailFields([]detailKV{
		{"url", ptrStr(r.PR.URL)},
		{"state", r.PR.State},
		{"merged_at", ptrTime(r.PR.MergedAt)},
		{"merge_commit", ptrStr(r.PR.MergeCommit)},
		{"queried_at", ptrTime(r.PR.QueriedAt)},
		{"query_error", ptrStr(r.PR.QueryError)},
	}, w)
}

func detailDecision(r *steerline.Run, w int) []string {
	d := r.Decision
	if d == nil {
		return detailFields([]detailKV{{"decision", "none"}}, w)
	}
	kvs := []detailKV{
		{"status", d.Status},
		{"question", d.Request.Question},
		{"context", clipText(d.Request.Context, 600)},
		{"recommended", d.Request.Recommended},
	}
	for i, o := range d.Request.Options {
		v := o.Label
		if o.Detail != "" {
			v += " — " + o.Detail
		}
		kvs = append(kvs, detailKV{fmt.Sprintf("option %d", i+1), o.ID + ": " + v})
	}
	kvs = append(kvs,
		detailKV{"answered_by", d.AnsweredBy},
		detailKV{"option_id", d.OptionID},
		detailKV{"note", d.Note},
	)
	return detailFields(kvs, w)
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ptrTime(p *time.Time) string {
	if p == nil || p.IsZero() {
		return ""
	}
	return p.Local().Format("2006-01-02 15:04:05")
}

func ptrInt(p *int) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%d", *p)
}

func clipText(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// wrapText hard-wraps to a display width, rune-aware so CJK content cannot push
// the detail pane off screen.
func wrapText(s string, width int) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if width <= 1 {
		return []string{s}
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		var cur strings.Builder
		curW := 0
		for _, r := range para {
			rw := lipgloss.Width(string(r))
			if curW+rw > width {
				out = append(out, cur.String())
				cur.Reset()
				curW = 0
			}
			cur.WriteRune(r)
			curW += rw
		}
		if cur.Len() > 0 {
			out = append(out, cur.String())
		}
	}
	return out
}

// viewStatusBar is the L0 line: repository identity, service liveness (heartbeat
// freshness, not just service.status), execution slot, and attention counts.
func (m Model) viewStatusBar(w int) string {
	if m.rt == nil {
		if m.observation.State == steerline.SnapshotUnknownVersion {
			return stylePending.Render(truncate("runtime schema unknown · sampled "+m.observation.SampledAt.Format("15:04:05")+" · "+m.observation.Error, w))
		}
		if m.err != nil {
			return stylePending.Render(truncate("snapshot read failed · "+m.observation.Error, w))
		}
		return styleFaint.Render(truncate("no runtime snapshot — waiting for the service", w))
	}
	rt := m.rt
	repo := filepath.Base(rt.Repository.Path)
	if repo == "" || repo == "." {
		repo = "?"
	}
	branch := rt.Repository.Branch
	if branch == "" {
		branch = "?"
	}
	svc := rt.Service.Status
	if svc == "" {
		svc = "?"
	}
	slot := "free"
	if rt.SlotOccupied {
		slot = "busy"
	}
	if rt.CurrentTask != nil && *rt.CurrentTask != "" {
		slot += " " + *rt.CurrentTask
	}
	parts := []string{
		repo + " · " + branch,
		fmt.Sprintf("svc %s pid %d", svc, rt.Service.PID),
		heartbeatText(m.observation),
		"slot " + slot,
	}
	if m.observation.State == steerline.SnapshotReadFailed || m.observation.State == steerline.SnapshotUnknownVersion {
		parts = append(parts, "snapshot error · last good sample "+m.observation.SampledAt.Format("15:04:05"))
	}
	if counts := countRuns(rt.Runs); counts != "" {
		parts = append(parts, counts)
	}
	line := "SteerLine · " + strings.Join(parts, " · ")
	if m.err != nil {
		line += "  err: " + m.err.Error()
	}
	style := styleFaint
	if rt.Service.Status == "error" {
		style = stylePending
	}
	return style.Render(truncate(line, w))
}

// heartbeatText reports how fresh the heartbeat is. A stopped process cannot
// refresh heartbeat_at, so age beyond three heartbeat intervals is flagged.
func heartbeatText(obs steerline.ObservationSnapshot) string {
	if obs.Runtime == nil || obs.Runtime.HeartbeatAt.IsZero() {
		return "hb n/a"
	}
	if obs.ClockSkew {
		return "hb clock-skew · timestamp in future"
	}
	age := obs.HeartbeatAge
	text := "hb " + shortDur(age) + " ago"
	if obs.State == steerline.SnapshotStale || age > obs.FreshnessLimit {
		text += " stale!"
	}
	if obs.State == steerline.SnapshotReadFailed || obs.State == steerline.SnapshotUnknownVersion {
		text += " unverified"
	}
	return text
}

func countRuns(runs []*steerline.Run) string {
	var decide, active, merge, block, done int
	for _, r := range runs {
		if r == nil {
			continue
		}
		switch {
		case isPending(r):
			decide++
		case r.FinishTaskStatus == "completed":
			done++
		case r.FinishTaskStatus == "blocked" || r.ExecutionStatus == "unknown":
			block++
		case r.FinishTaskStatus == "waiting_pr_merge":
			merge++
		case r.ExecutionStatus == "running":
			active++
		}
	}
	var parts []string
	add := func(label string, n int) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", label, n))
		}
	}
	add("decide", decide)
	add("block", block)
	add("active", active)
	add("merge", merge)
	add("done", done)
	return strings.Join(parts, " · ")
}

func shortDur(d time.Duration) string {
	switch {
	case d < time.Second:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func elapsedShort(r *steerline.Run) string {
	if r == nil || r.StartedAt == nil {
		return ""
	}
	end := time.Now()
	if r.EndedAt != nil {
		end = *r.EndedAt
	}
	d := end.Sub(*r.StartedAt)
	if d < 0 {
		d = 0
	}
	return shortDur(d)
}

func (m Model) viewRuns(w, h int) []string {
	body := h - 2
	if body < 1 {
		body = 1
	}
	start := 0
	if m.sel >= body {
		start = m.sel - body + 1
	}
	lines := []string{
		styleLabel.Render(fmt.Sprintf("Runs · %d", len(m.rows))),
		styleFaint.Render(truncate("! needs you · STAGE delivery · TIME worker run · EXEC in Detail", w)),
	}
	if len(m.rows) == 0 {
		lines = append(lines, styleFaint.Render("(no runs)"))
	}
	taskW := 6
	for _, rr := range m.rows {
		if n := lipgloss.Width(rr.run.TaskID); n > taskW {
			taskW = n
		}
	}
	if taskW > 10 {
		taskW = 10
	}
	for i := start; i < len(m.rows) && len(lines) <= body; i++ {
		r := m.rows[i]
		mark := " "
		if r.pending {
			mark = "!"
		}
		badge := deliveryBadge(&r)
		el := elapsedShort(r.run)
		// mark(1)+sp+task+sp+badge(6)+sp+elapsed(5)+sp
		prefixW := 1 + 1 + taskW + 1 + 6 + 1 + 5 + 1
		title := ""
		titleBudget := w - prefixW
		if titleBudget >= 4 {
			title = truncate(r.run.Title, titleBudget)
		}
		plain := fmt.Sprintf("%s %-*s %-6s %5s %s", mark, taskW, truncate(r.run.TaskID, taskW), badge, el, title)
		if lipgloss.Width(plain) > w {
			plain = truncate(plain, w)
		}
		var line string
		switch {
		case i == m.sel:
			line = styleSel.Render(plain)
		case titleBudget >= 4:
			line = fmt.Sprintf("%s %-*s %s %5s %s", mark, taskW, truncate(r.run.TaskID, taskW), badgeColor(badge).Render(fmt.Sprintf("%-6s", badge)), el, title)
		default:
			line = plain
		}
		lines = append(lines, line)
	}
	return lines
}

func (m Model) viewLog(w, h int) []string {
	title := "Activity"
	if r := m.current(); r != nil {
		title = "Activity · " + r.TaskID
		if r.LogPath != nil && *r.LogPath != "" {
			attemptDir := filepath.Dir(*r.LogPath)
			if filepath.Base(filepath.Dir(attemptDir)) == "attempts" {
				title += " · attempt " + filepath.Base(attemptDir)
			}
		}
	}
	var rendered []string
	if m.mode == modeRaw {
		for _, line := range m.raw {
			rendered = append(rendered, styleFaint.Render(truncate(line, w)))
		}
	} else {
		for _, ev := range foldEvents(m.log, m.mode == modeConversation) {
			for _, ln := range strings.Split(steerline.RenderActivity(ev, steerline.RenderOptions{Color: m.opts.Color, MaxColumns: w}), "\n") {
				rendered = append(rendered, ln)
			}
		}
	}
	scroll := m.logScroll
	if scroll > len(rendered) {
		scroll = len(rendered)
	}
	chips := []string{m.mode.name()}
	if m.follow {
		chips = append(chips, "follow")
	} else {
		chips = append(chips, "paused")
	}
	if scroll > 0 {
		chips = append(chips, fmt.Sprintf("+%d", scroll))
	}
	chip := "[" + strings.Join(chips, " · ") + "]"
	titleWidth := w - lipgloss.Width(chip) - 2
	lines := []string{styleLabel.Render(truncate(title, titleWidth)) + "  " + styleFaint.Render(chip)}
	if m.logError != "" {
		lines = append(lines, stylePending.Render(truncate("log read · "+m.logError, w)))
	}
	if len(rendered) == 0 {
		message := "waiting for complete log records"
		if r := m.current(); r == nil || r.LogPath == nil || *r.LogPath == "" {
			message = "selected run has no log file"
		}
		lines = append(lines, styleFaint.Render(truncate(message, w)))
		return lines
	}
	body := h - len(lines)
	if body < 1 {
		body = 1
	}
	end := len(rendered) - scroll
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

// conversationKind is the subset of activity kinds that form the agent's
// conversation: what it said, thought, ran, changed or failed on, plus the
// scheduler lifecycle markers worth keeping in context.
func conversationKind(kind string) bool {
	switch kind {
	case "message", "reasoning", "command", "file_change", "error",
		"claim", "prepare", "worker_start", "worker_exit", "result",
		"retry", "pr", "completion", "decision":
		return true
	}
	return false
}

func isStreamingStatus(s string) bool { return s == "running" || s == "started" }

// foldEvents collapses repeated updates for the same item_id into one row and
// accumulates streamed message/reasoning deltas. Presentation-only: the stored
// JSONL is never changed. When conversation is true, non-conversation kinds are
// dropped and duplicated full messages that follow streamed deltas are removed.
func foldEvents(events []steerline.ActivityEvent, conversation bool) []steerline.ActivityEvent {
	out := make([]steerline.ActivityEvent, 0, len(events))
	index := map[string]int{}
	seen := map[string]bool{}
	for _, ev := range events {
		if conversation && !conversationKind(ev.Kind) {
			continue
		}
		streaming := ev.Kind == "message" || ev.Kind == "reasoning"
		detail := strings.TrimSpace(ev.Detail)
		if streaming && detail == "" {
			continue
		}
		if ev.ItemID == "" {
			if streaming && seen[detail] {
				continue
			}
			if detail != "" {
				seen[detail] = true
			}
			out = append(out, ev)
			continue
		}
		if idx, ok := index[ev.ItemID]; ok {
			prev := out[idx]
			if streaming && isStreamingStatus(prev.Status) && isStreamingStatus(ev.Status) {
				prev.Detail = strings.TrimRight(prev.Detail+ev.Detail, "\n")
				prev.Status = ev.Status
				prev.TS = ev.TS
			} else {
				prev = ev
			}
			out[idx] = prev
			if d := strings.TrimSpace(out[idx].Detail); d != "" {
				seen[d] = true
			}
			continue
		}
		index[ev.ItemID] = len(out)
		out = append(out, ev)
		if detail != "" {
			seen[detail] = true
		}
	}
	return out
}

func (m Model) viewFooter(w int) string {
	if r := m.current(); isPending(r) {
		return stylePending.Render(truncate("! decision pending for "+r.TaskID+" — press a to answer  "+helpKeys, w))
	}
	if m.status != "" {
		return styleFaint.Render(truncate(m.status+"  "+helpKeys, w))
	}
	return styleFaint.Render(truncate(helpKeys, w))
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
