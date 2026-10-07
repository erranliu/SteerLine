package console

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	if row == nil || row.run == nil || candidateIdentity(row.run) != d.identity || row.run.Decision == nil || row.run.Decision.ID != d.decisionID || row.run.Decision.Status != "pending" || row.run.Abandonment != nil || row.run.TaskStatus == "closed" {
		m.leaveAnswer()
		m.status = "原问题或候选已变化；原草稿保留，请按 a 打开当前问题"
		return
	}
	rt := m.projects[m.selected].snapshot.Connection.Snapshot.Page
	if rt == nil || rt.Service.InstanceID != d.instance || row.run.ControlRevision != d.runRevision || row.run.Decision.Version != d.decisionVersion {
		d.error = "问题/实例/版本已变化；原草稿未改目标。Esc 保留后按 a 重新核对"
	}
}

func rightPaneHeights(height int) (int, int) {
	if height < 13 {
		return 0, height
	}
	top := min(9, max(5, height/3))
	return top, height - top - 1
}

func renderRightPanes(m Model, state projectState, width, height int) []string {
	topHeight, lowerHeight := rightPaneHeights(height)
	if m.answer != nil && lowerHeight < 9 {
		topHeight, lowerHeight = 0, height
	}
	row := m.selectedRow(m.selected)
	var lines []string
	if topHeight > 0 {
		lines = fitLines(candidateOverviewLines(m, state, row, width), width, topHeight)
		lines = append(lines, styleRule.Render(strings.Repeat("─", width)))
	}
	local := m
	local.width = width
	if m.answer != nil && !m.answer.details {
		lines = append(lines, strings.Split(renderAnswerDialog(local, state, lowerHeight), "\n")...)
	} else {
		lower := wrapLines(feedbackLines(m, state, row), width)
		lines = append(lines, fitLines(scrollLines(lower, state.lowerScroll, lowerHeight), width, lowerHeight)...)
	}
	return lines
}

func candidateOverviewLines(m Model, state projectState, row *consoleRow, width int) []string {
	lines := []string{"候选与执行 · d 完整详情"}
	if row == nil {
		return append(lines, renderEmptySelection(state)...)
	}
	if row.run == nil {
		lines = append(lines, "任务 "+row.taskID+" · "+row.title)
		return append(lines, renderControlAvailability(state, row, m.clock.Now())...)
	}
	run := row.run
	lines[0] += " · 当前 " + candidateLabel(state, run) + " · session " + shortSession(run.SessionID)
	lines = append(lines, renderRunExecutionProfileSummary(state, run)...)
	lines = append(lines, "任务 "+row.taskID+" · "+displayExecutionStatus(run)+" · "+candidateHumanStatus(state, run))
	if run.EnvironmentBlocker != nil {
		lines = append(lines, "环境阻断："+safeTerminalInlineText(run.EnvironmentBlocker.Fact.Reason))
	}
	if run.PR.URL != nil {
		lines = append(lines, "PR："+safeTerminalInlineText(*run.PR.URL)+" · "+safeTerminalInlineText(run.PR.State))
	} else {
		lines = append(lines, "PR：尚未核验绑定")
	}
	lines = append(lines, renderCandidateOverview(state, row)[0])
	if run.Error != nil {
		lines = append(lines, "错误："+safeTerminalInlineText(*run.Error))
	}
	elapsed := "未知"
	if !run.CandidateCreatedAt.IsZero() {
		end := m.clock.Now()
		if run.CandidateEndedAt != nil {
			end = *run.CandidateEndedAt
		}
		elapsed = fmt.Sprint(max(0, end.Sub(run.CandidateCreatedAt)).Round(1e9))
	}
	lines = append(lines, "耗时 "+elapsed+" · 资源 "+firstText(run.ResourceStatus, "未核实"), "采样 "+sampleTime(state.snapshot))
	for i, line := range lines {
		lines[i] = truncate(safeTerminalInlineText(line), width)
	}
	return lines
}
