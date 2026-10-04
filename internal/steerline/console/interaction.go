package console

import (
	"fmt"
	"sort"
	"strings"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

// Derive labels from the task's complete persisted workflow history, including
// exited candidates. Display labels never replace the immutable control ID.
func candidateLabel(state projectState, run *contracts.Run) string {
	if run == nil {
		return "未选择"
	}
	var workflows []executionmodel.WorkflowRunRecord
	if state.snapshot.Domain != nil {
		workflows = state.snapshot.Domain.WorkflowRuns
	}
	if len(workflows) == 0 && state.snapshot.Observation.Runtime != nil {
		workflows = state.snapshot.Observation.Runtime.WorkflowRuns
	}
	if len(workflows) == 0 {
		if detail := nativeDetailForTask(state, run.TaskID); detail != nil {
			workflows = detail.WorkflowRuns
		}
	}
	if len(workflows) == 0 {
		for _, r := range candidateRuns(state, run.TaskID) {
			workflows = append(workflows, executionmodel.WorkflowRunRecord{WorkflowRunID: r.RunID, TaskID: r.TaskID, CreatedAt: r.CandidateCreatedAt})
		}
	}
	ordered := make([]executionmodel.WorkflowRunRecord, 0, len(workflows))
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

func feedbackAction(state projectState, run *contracts.Run) *actionState {
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

func feedbackInProgress(state projectState, run *contracts.Run) bool {
	a := feedbackAction(state, run)
	return a != nil && a.Status != "rejected" && a.Status != "failed"
}

func feedbackReceiptStatus(state projectState, a *actionState) string {
	if a.Action != "answer_decision" {
		return a.Message
	}
	return candidateLabel(state, &contracts.Run{TaskID: a.TaskID, RunID: a.RunID}) + " · " + a.Message
}

func candidateHumanStatus(state projectState, run *contracts.Run) string {
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
	controls := "F6/F7 切换并行 · Enter 会话 · d 完整详情"
	if row.run.Decision != nil && row.run.Decision.Status == "pending" && row.run.Abandonment == nil && row.run.TaskStatus != "closed" && !feedbackInProgress(state, row.run) {
		controls = "a/Enter 回复 · F6/F7 切换并行 · d 完整详情"
	}
	return []string{strings.Join(chips, " · "), "当前 " + candidateLabel(state, row.run) + " · session " + shortSession(row.run.SessionID), controls}
}

func feedbackPreview(state projectState, dialog *decisionAnswerDialog, request executionmodel.DecisionRequest) []string {
	var run *contracts.Run
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
	request, ok := decisionRequestFor(state, d.runID, d.decisionID)
	var run *contracts.Run
	for _, r := range candidateRuns(state, "") {
		if r.RunID == d.runID {
			run = r
			break
		}
	}
	session := "尚未建立"
	if ids := strings.Split(d.identity, "\x00"); len(ids) == 4 {
		session = shortSession(ids[3])
	}
	header := wrapLines([]string{"人工决策 · 显式提交 → " + candidateLabel(state, run), "session " + safeTerminalText(session)}, m.width)
	note := append([]rune(nil), d.note...)
	if d.noteFocus && d.cursor >= 0 && d.cursor <= len(note) {
		note = append(note, 0)
		copy(note[d.cursor+1:], note[d.cursor:])
		note[d.cursor] = '│'
	}
	if len(note) == 0 {
		note = []rune("│")
	}
	noteLines := wrapLines(strings.Split(safeTerminalText(string(note)), "\n"), m.width)
	noteRows := max(1, min(3, height/6))
	cursorLine := len(noteLines) - 1
	for i, line := range noteLines {
		if strings.Contains(line, "│") {
			cursorLine = i
			break
		}
	}
	noteStart := max(0, cursorLine-noteRows+1)
	noteTitle := "补充说明（可选）"
	if len(request.Options) == 0 || d.optionIndex == len(request.Options) {
		noteTitle = "你的答复"
	}
	if d.noteFocus {
		noteTitle += " [输入焦点]"
	}
	noteTitle += fmt.Sprintf(" · %d/4000", len(d.note))
	bottom := []string{noteTitle}
	bottom = append(bottom, fitLines(noteLines[noteStart:], m.width, noteRows)...)
	bottom = append(bottom, "")
	for _, line := range feedbackPreview(state, d, request) {
		bottom = append(bottom, truncate(line, m.width))
	}
	if d.error != "" {
		bottom = append(bottom, truncate("未提交："+safeTerminalText(d.error), m.width))
	}
	if run != nil && run.EnvironmentBlocker != nil {
		bottom = append(bottom, truncate("环境受阻，草稿保留；解除后重新提交", m.width))
	}
	area := max(1, height-len(header)-len(bottom)-1)
	questionRows := max(1, min(3, area/3))
	question := []string{"问题：" + safeTerminalText(request.Question)}
	if !ok {
		question = []string{"原问题暂不可读；草稿保留"}
	}
	upper := fitLines(wrapLines(question, m.width), m.width, questionRows)
	if len(request.Options) == 0 {
		upper = append(upper, "无固定选项，直接输入答复")
	} else {
		optionRows := max(1, area-questionRows-1)
		count := len(request.Options)
		if request.AllowFreeText {
			count++
		}
		upper = append(upper, fmt.Sprintf("选择 %d/%d · ↑↓（F8 查看完整背景/选项详情）", d.optionIndex+1, count))
		start := max(0, d.optionIndex-optionRows/2)
		start = min(start, max(0, count-optionRows))
		for i := start; i < min(count, start+optionRows); i++ {
			label := "自定义答复"
			if i < len(request.Options) {
				label = fmt.Sprintf("%d. %s", i+1, request.Options[i].Label)
			}
			marker := "[ ] "
			if i == d.optionIndex {
				marker = "[x] "
			}
			upper = append(upper, truncate(marker+safeTerminalInlineText(label), m.width))
		}
	}
	lines := append(header, fitLines(upper, m.width, area)...)
	lines = append(lines, "")
	lines = append(lines, bottom...)
	return strings.Join(fitLines(lines, m.width, height), "\n")
}
