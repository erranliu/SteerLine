package console

import (
	"fmt"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"sort"
	"strings"
)

// Derive labels from the task's complete persisted workflow history, including
// exited candidates. Display labels never replace the immutable control ID.
func candidateLabel(state projectState, run *api.CandidateSummary) string {
	if run == nil {

		return "未选择"
	}
	var workflows []api.WorkflowSummary
	if state.snapshot.History != nil {
		workflows = state.snapshot.History.WorkflowRuns
	}
	if len(workflows) == 0 {
		if detail := nativeDetailForTask(state, run.TaskID); detail != nil {
			workflows = detail.WorkflowRuns
		}
	}
	if len(workflows) == 0 {
		for _, r := range candidateRuns(state, run.TaskID) {
			workflows = append(workflows, api.WorkflowSummary{WorkflowRunID: r.RunID, TaskID: r.TaskID, CreatedAt: r.CandidateCreatedAt})
		}
	}
	ordered := make([]api.WorkflowSummary, 0, len(workflows))
	for _, w := range workflows {
		if w.TaskID == run.TaskID {
			ordered = append(ordered, w)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].WorkflowRunID < ordered[j].WorkflowRunID
		}
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})
	for i, w := range ordered {
		if w.WorkflowRunID == run.RunID {
			return fmt.Sprintf("P%d", i+1)
		}
	}

	return "未编号"
}

func shortSession(id string) string {
	if id == "" {

		return "尚未建立"
	}
	r := []rune(id)
	if len(r) <= 16 {
		return id
	}
	return string(r[:8]) + "…" + string(r[len(r)-4:])
}

func feedbackAction(state projectState, run *api.CandidateSummary) *actionState {
	if run == nil || run.Decision == nil {
		return nil
	}
	var latest *actionState
	for _, a := range state.actions {
		if a.Action != "answer_decision" || a.RunID != run.RunID || a.DecisionID != run.Decision.ID {
			continue
		}
		if a.DecisionIdentity != "" && a.DecisionIdentity != candidateIdentity(run) {
			continue
		}
		if latest == nil || a.UpdatedAt.After(latest.UpdatedAt) || a.UpdatedAt.Equal(latest.UpdatedAt) && a.RequestID > latest.RequestID {
			latest = a
		}
	}
	return latest
}

func feedbackInProgress(state projectState, run *api.CandidateSummary) bool {
	a := feedbackAction(state, run)
	return a != nil && a.Status != "rejected" && a.Status != "failed"
}

func feedbackReceiptStatus(state projectState, a *actionState) string {
	if a.Action != "answer_decision" {
		return a.Message
	}
	return candidateLabel(state, &api.CandidateSummary{TaskID: a.TaskID, RunID: a.RunID}) + " · " + a.Message
}

func candidateHumanStatus(state projectState, run *api.CandidateSummary) string {
	if run.Abandonment != nil {

		return "已退出实验"
	}
	if run.TaskStatus == "closed" {

		return "实验已结束"
	}
	if a := feedbackAction(state, run); a != nil {
		switch a.Status {
		case "submitting":

			return "提交中"
		case "pending", "in_flight", "submitted":
			return "已提交，等待回执"
		case "accepted", "applied":

			return "反馈已保存"
		case "indeterminate":

			return "提交结果待核实"
		}
	}
	if run.Decision != nil && run.Decision.Status == "pending" {
		if run.EnvironmentBlocker != nil {

			return "交互受阻"
		}

		return "待交互"
	}
	return runStatus(run)
}

func candidateHeadline(state projectState, row *consoleRow) string {
	if row == nil {
		return ""
	}
	runs := candidateRuns(state, row.taskID)
	if len(runs) == 0 {
		return ""
	}
	participating, waiting, blocked := 0, 0, 0
	for _, r := range runs {
		if r.Abandonment == nil && r.TaskStatus != "closed" && r.FinishTaskStatus != "completed" {
			participating++
		}
		s := candidateHumanStatus(state, r)
		if s == "待交互" || s == "交互受阻" {
			waiting++
		}
		if s == "交互受阻" {
			blocked++
		}
	}
	line := fmt.Sprintf("并行 %d · 待交互 %d · 当前 %s", participating, waiting, candidateLabel(state, row.run))
	if blocked > 0 {
		line += fmt.Sprintf(" · 受阻 %d", blocked)
	}
	return line
}

func renderCandidateOverview(state projectState, row *consoleRow) []string {
	if row == nil || row.run == nil {
		return nil
	}
	var chips []string
	for _, r := range candidateRuns(state, row.taskID) {
		label := candidateLabel(state, r)
		if r.RunID == row.run.RunID {
			label = "[" + label + "]"
		}
		chips = append(chips, label+" "+candidateHumanStatus(state, r))
	}
	controls := "F6/F7 切换并行 · Enter 详情 · d 完整详情"
	if row.run.Decision != nil && row.run.Decision.Status == "pending" && row.run.Abandonment == nil && row.run.TaskStatus != "closed" && !feedbackInProgress(state, row.run) {
		controls = "a/Enter 回复 · F6/F7 切换并行 · d 完整详情"
	}
	return []string{strings.Join(chips, " · "), "当前 " + candidateLabel(state, row.run) + " · session " + shortSession(row.run.SessionID), controls}
}

func feedbackPreview(state projectState, dialog *decisionAnswerDialog, request api.DecisionRequest) []string {
	var run *api.CandidateSummary
	for _, r := range candidateRuns(state, "") {
		if r.RunID == dialog.runID {
			run = r
			break
		}
	}
	option := "自由输入"
	if dialog.optionIndex >= 0 && dialog.optionIndex < len(request.Options) {
		option = request.Options[dialog.optionIndex].Label
	}
	note := strings.TrimSpace(string(dialog.note))
	if note == "" {
		note = "（未填写）"
	}
	return []string{"提交预览 → " + candidateLabel(state, run), "选择：" + safeTerminalText(option), "内容：" + safeTerminalText(strings.ReplaceAll(note, "\n", " / "))}
}

func renderInteractionSummary(state projectState, row *consoleRow) []string {
	lines := []string{"任务 " + safeTerminalInlineText(row.taskID) + " · " + safeTerminalInlineText(row.title)}
	lines = append(lines, renderCandidateOverview(state, row)...)
	lines = append(lines, renderRunExecutionProfileSummary(state, row.run)...)
	status := candidateHumanStatus(state, row.run)
	lines = append(lines, "当前状态："+status)
	if status == "待交互" || status == "交互受阻" {
		lines = append(lines, "问题："+safeTerminalText(row.run.Decision.Request.Question))
	}
	if row.run.EnvironmentBlocker != nil {
		lines = append(lines, "环境阻断："+safeTerminalText(row.run.EnvironmentBlocker.Fact.Reason))
	}
	if row.native != nil {
		relations := renderNativeTaskRelations(state, *row.native)
		if len(relations) > 0 {
			lines = append(lines, "任务关系")
			lines = append(lines, relations...)
		}
	}
	return lines
}

func renderAnswerDialog(m Model, state projectState, height int) string {
	d := m.answer
	if d == nil {
		return ""
	}
	if d.details {
		return renderAnswerDetails(m, state, height)
	}
	request := d.request
	var run *api.CandidateSummary
	for _, r := range candidateRuns(state, "") {
		if r.RunID == d.runID {
			run = r
			break
		}
	}
	header := answerHeader(state, d)
	option, detail := "自由输入", "无固定选项，直接输入答复"
	if d.optionIndex >= 0 && d.optionIndex < len(request.Options) {
		option = request.Options[d.optionIndex].Label
		detail = request.Options[d.optionIndex].Detail
	}
	optionLine := "[x] " + safeTerminalInlineText(option)
	if state.focus == 1 {
		optionLine = styleSelected.Render("▸ " + optionLine)
	}
	label := fmt.Sprintf("你的答复 · %d/4000", len(d.note))
	if state.focus == 2 {
		label = styleFocus.Render("▶ ┃ " + label)
	} else {
		label = "  │ " + label
	}
	note := append([]rune(nil), d.note...)
	if state.focus == 2 && d.cursor >= 0 && d.cursor <= len(note) {
		note = append(note, 0)
		copy(note[d.cursor+1:], note[d.cursor:])
		note[d.cursor] = '│'
	}
	noteLines := wrapLines(strings.Split(safeTerminalText(string(note)), "\n"), m.width)
	cursorLine := max(0, len(noteLines)-1)
	for i, line := range noteLines {
		if strings.Contains(line, "│") {
			cursorLine = i
			break
		}
	}
	rows := max(1, min(3, height-answerMinimum(m)+1))
	start := max(0, cursorLine-rows+1)
	lines := []string{header, styleHead.Render("问题：" + safeTerminalInlineText(request.Question)), optionLine, "说明：" + safeTerminalInlineText(shortSummary(detail, 160)), label}
	lines = append(lines, fitLines(noteLines[start:], m.width, rows)...)
	preview := feedbackPreview(state, d, request)
	lines = append(lines, preview[0]+" · "+preview[1], preview[2])
	if d.error != "" {
		lines = append(lines, styleAlert.Render("未提交："+safeTerminalInlineText(d.error)))
	}
	if run != nil && run.EnvironmentBlocker != nil {
		lines = append(lines, styleAlert.Render("环境受阻，草稿保留；解除后重新提交"))
	}
	enter := "Enter 转输入"
	if state.focus == 2 {
		enter = "Enter 换行"
	}
	if state.focus == 0 {
		enter = "Enter 返回草稿"
	}
	lines = append(lines, "Tab 切换 · "+enter+" · F2 提交 · F8 背景 · Esc 保留")
	return strings.Join(fitLines(lines, m.width, height), "\n")
}

func answerHeader(state projectState, d *decisionAnswerDialog) string {
	task := "未核实"
	if ids := strings.Split(d.identity, "\x00"); len(ids) == 4 {
		task = ids[0]
	}
	return "运行与反馈 · Task " + safeTerminalInlineText(task) + " / " + candidateLabel(state, &api.CandidateSummary{TaskID: task, RunID: d.runID})
}
