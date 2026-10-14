package console

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func (m Model) refreshSelected(index int) (Model, tea.Cmd) {
	m, final := m.refreshFinalReplies(index)
	m, activity := m.refreshRecentActivity(index)
	return m, tea.Batch(final, activity)
}

func (m *Model) leaveAnswer() {
	if m.answer == nil {
		return
	}
	m.saveDecisionDraft()
	if index := m.projectIndex(m.answer.projectKey); index >= 0 {
		m.projects[index].focus = 1
	}
	m.answer = nil
}

func (m *Model) syncAnswer() {
	d := m.answer
	if d == nil {
		return
	}
	row := m.selectedRow(m.selected)
	if row == nil || row.run == nil || candidateIdentity(row.run) != d.identity || row.run.Decision == nil || row.run.Decision.ID != d.decisionID || row.run.Decision.Status != "pending" || row.run.TaskStatus == "closed" || row.run.TaskStatus == "discarded" {
		m.leaveAnswer()
		m.status = "原问题或候选已变化；原草稿保留，请按 a 打开当前问题"
		return
	}
	rt := m.projects[m.selected].snapshot.Connection.Snapshot.Page
	if rt == nil || rt.Service.InstanceID != d.instance || row.run.ControlRevision != d.runRevision || row.run.Decision.Version != d.decisionVersion {
		d.error = "问题/实例/版本已变化；原草稿未改目标。Esc 保留后按 a 重新核对"
	}
}

func renderRightPanes(m Model, state projectState, width, height int) []string {
	topHeight, lowerHeight := m.workspaceHeights(height)
	row := m.selectedRow(m.selected)
	var lines []string
	if topHeight > 0 {
		lines = paneFrame(candidateOverviewLines(m, state, row, width-2), width, topHeight, false)
	}
	local := m
	local.width = max(1, width-2)
	var lower []string
	switch {
	case (m.answer != nil || m.profileChoice != nil || m.confirm != nil) && !m.operationVisible():
		lower = windowTooSmall(m)
	case m.answer != nil:
		lower = strings.Split(renderAnswerDialog(local, state, lowerHeight-2), "\n")
	case m.profileChoice != nil:
		lower = strings.Split(renderExecutionProfileChoice(local, state, lowerHeight-2), "\n")
	case m.confirm != nil:
		lower = fitLines(confirmRequired(m, state), max(1, width-2), len(confirmRequired(m, state)))
	case state.recoveryActive:
		lower = recoverySummary(state, row)
	default:
		lower = feedbackLines(m, state, row)
	}
	if topHeight == 0 && len(lower) > 0 && m.answer == nil && m.profileChoice == nil && m.confirm == nil && !state.recoveryActive {
		body := append(candidateOverviewLines(m, state, row, width-2)[1:], lower[1:]...)
		lower = append([]string{lower[0]}, body...)
	}
	if len(lower) > 0 {
		header := lower[0]
		body := wrapLines(lower[1:], max(1, width-2))
		if m.answer == nil && m.profileChoice == nil && m.confirm == nil {
			body = scrollLines(body, state.lowerScroll, max(0, lowerHeight-3))
		}
		lower = append([]string{header}, body...)
	}
	lines = append(lines, paneFrame(lower, width, lowerHeight, state.focus != 0)...)
	return lines
}

func candidateOverviewLines(m Model, state projectState, row *consoleRow, width int) []string {
	lines := []string{"候选与执行"}
	if row == nil {
		return append(lines, renderEmptySelection(state)...)
	}
	lines = append(lines, "Task "+safeTerminalInlineText(row.taskID)+" · "+truncate(safeTerminalInlineText(row.title), max(1, width-12)))
	if state.snapshot.Connection.State != api.ConnectionConnected || state.snapshot.Observation.State != api.SnapshotValid {
		lines = append(lines, styleWaiting.Render("离线/缓存 · 采样 "+sampleTime(state.snapshot)))
	}
	if row.run == nil {
		lines = append(lines, "task_status:      "+workerValue("task_status", taskRowRawStatus(*row), true), "任务提示："+taskRowHint(m, state, *row))
		lines = append(lines, taskBlockerLines(*row)...)
		if state.candidates[row.taskID] != "" {
			lines = append(lines, "原候选暂不可读取/未核实")
		} else {
			lines = append(lines, "worker: —")
		}
		if action := rowOperation(state, *row); action != nil {
			lines = append(lines, "request status: "+safeTerminalInlineText(action.Status)+" · "+operationProgress(action))
		}
		lines = append(lines, "下一步："+nextAction(m, state, row))
		return append([]string{lines[0]}, wrapLines(lines[1:], width)...)
	}
	run := row.run
	if run.Decision != nil && run.Decision.Request.InteractionKind == "async" && (run.Decision.Status == "pending" || run.Decision.Status == "answer_saved") {
		text := "异步问答 · 有待回答问题（执行可继续）"
		if run.Decision.Request.BlocksProgress {
			text = "异步问答 · 执行已停止，等待回答"
		}
		if run.Decision.Status == "answer_saved" {
			text += " · 答案已保存"
		}
		lines = append(lines, text)
	}
	lines = append(lines, "当前 "+candidateLabel(state, run)+" · F6/F7 候选")
	current := *run
	current.TaskStatus = taskRowRawStatus(*row)
	for _, line := range workerStateLines(&current, width >= 55) {
		if !strings.HasPrefix(line, "execution_status:") && !strings.HasPrefix(line, "workflow_status:") {
			lines = append(lines, line)
		}
	}
	lines = append(lines, taskBlockerLines(*row)...)
	if p := fixedRunExecutionProfile(state, run); p != nil {
		summary := firstText(p.Name, p.ID) + " · " + p.Host + " / " + p.Agent
		if len(p.ImplementationDisplay) > 0 {
			summary += " · " + strings.Join(p.ImplementationDisplay, " · ")
		}
		lines = append(lines, truncate("执行方案："+safeTerminalInlineText(summary), width))
	}
	if run.PR.URL != nil {
		lines = append(lines, "PR："+prLink(*run.PR.URL, shortPR(*run.PR.URL))+" · "+safeTerminalInlineText(run.PR.State))
	}
	if notice := workerResultNotice(run); notice != "" {
		lines = append(lines, styleAlert.Render(notice))
	}
	if diagnostic := workerDiagnostic(run); diagnostic != "" && !diagnosticInWorkerTimeline(state.workerTransitions, run, diagnostic) {
		lines = append(lines, "原因："+safeTerminalText(shortSummary(diagnostic, 160)))
	}
	lines = append(lines, "下一步："+nextAction(m, state, row))
	return append([]string{lines[0]}, wrapLines(lines[1:], width)...)
}
