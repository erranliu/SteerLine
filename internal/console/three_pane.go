package console

import (
	"fmt"
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
	if row == nil || row.run == nil || candidateIdentity(row.run) != d.identity || row.run.Decision == nil || row.run.Decision.ID != d.decisionID || row.run.Decision.Status != "pending" || row.run.Abandonment != nil || row.run.TaskStatus == "closed" || row.run.TaskStatus == "discarded" {
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
	if row.run == nil {
		lines = append(lines, "任务 "+safeTerminalInlineText(row.taskID+" · "+row.title))
		if action := rowOperation(state, *row); action != nil {
			lines = append(lines, stateText(operationProgress(action)))
		} else if state.candidates[row.taskID] != "" {
			lines = append(lines, "原候选暂不可读取/未核实")
		} else {
			lines = append(lines, "暂无候选")
		}
		lines = append(lines, safeTerminalInlineText("下一步："+nextAction(m, state, row)))
		if row.native != nil {
			lines = append(lines, renderNativeTaskRelations(state, *row.native)...)
		}
		return lines
	}
	run := row.run
	if state.snapshot.Connection.State != api.ConnectionConnected || state.snapshot.Observation.State != api.SnapshotValid {
		lines = append(lines, styleWaiting.Render("离线/缓存/未核实 · "+sampleTime(state.snapshot)))
	}
	lines = append(lines, "Task "+safeTerminalInlineText(row.taskID+" · "+row.title), "当前 "+candidateLabel(state, run)+" · "+candidateStrip(state, row, width))
	lines = append(lines, "任务 "+rowTaskStatus(*row)+" · "+stateText(rowExecutionProgress(state, *row))+" · "+stateText(candidateHumanStatus(state, run)))
	if run.EnvironmentBlocker != nil {
		lines = append(lines, "环境阻断："+safeTerminalInlineText(run.EnvironmentBlocker.Fact.Reason))
	}
	if run.PR.URL != nil {
		lines = append(lines, "PR："+shortPR(*run.PR.URL)+" · "+safeTerminalInlineText(run.PR.State))
	} else {
		lines = append(lines, "PR：尚未核验绑定")
	}
	if run.Error != nil {
		lines = append(lines, "错误："+safeTerminalInlineText(*run.Error))
	}
	if run.PR.QueryError != nil {
		lines = append(lines, "PR 查询失败："+safeTerminalInlineText(*run.PR.QueryError))
	}
	lines = append(lines, styleHead.Render(safeTerminalInlineText("下一步："+nextAction(m, state, row))))

	if p := fixedRunExecutionProfile(state, run); p != nil {
		lines = append(lines, profileProductLines(*p)...)
	} else {
		lines = append(lines, "冻结方案未核实")
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
		lines[i] = truncate(line, width)
	}
	return lines
}
