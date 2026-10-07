package console

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

type activitySummary struct {
	Time                  time.Time
	Title, Status, Detail string
}

type recentActivityMsg struct {
	projectKey, identity, key string
	generation                uint64
	batch                     api.LogBatch
	err                       error
}

func (m Model) refreshRecentActivity(index int) (Model, tea.Cmd) {
	state := &m.projects[index]
	row := m.selectedRow(index)
	key := ""
	if row != nil && row.run != nil {
		key = candidateIdentity(row.run) + "\x00" + row.run.AttemptID
	}
	feedbackKey := key
	if row != nil && row.run != nil && row.run.Decision != nil {
		feedbackKey += fmt.Sprintf("\x00%s\x00%s\x00%d", row.run.Decision.ID, row.run.Decision.Status, row.run.Decision.Version)
	}
	if feedbackKey != state.feedbackKey {
		state.feedbackKey = feedbackKey
		state.lowerScroll = 0
	}
	if key != state.activityKey {
		state.activityKey = key
		state.activityGeneration++
		state.activityBusy = false
		state.activityReadAt = time.Time{}
		state.recentActivity = nil
		state.activityError = ""
		state.lowerScroll = 0
	}
	if key == "" || state.activityBusy || !state.activityReadAt.IsZero() && m.clock.Now().Sub(state.activityReadAt) < time.Second {
		return m, nil
	}
	if row.run.ExecutionID == "" || row.run.AttemptID == "" {
		state.activityError = "当前执行/Attempt身份未完整，无法读取活动摘要"
		return m, nil
	}
	state.activityBusy = true
	state.activityGeneration++
	state.activityReadAt = m.clock.Now()
	generation, ref, run, query, ctx := state.activityGeneration, state.ref, *row.run, state.client, m.ctx
	return m, func() tea.Msg {
		batch, err := query.RecentActivity(ctx, run.RunID, run.ExecutionID, run.AttemptID)
		return recentActivityMsg{ref.Key, ref.Identity, key, generation, batch, err}
	}
}

func (m Model) updateRecentActivity(msg recentActivityMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	state := &m.projects[index]
	if msg.identity != state.ref.Identity || msg.key != state.activityKey || msg.generation != state.activityGeneration {
		return m, nil
	}
	state.activityBusy = false
	if msg.err != nil {
		state.activityError = msg.err.Error()
		return m, nil
	}
	row := m.selectedRow(index)
	cursor := msg.batch.Cursor
	if row == nil || row.run == nil || !cursor.Initialized || cursor.RunID != row.run.RunID || cursor.ExecutionID != row.run.ExecutionID || cursor.AttemptID != row.run.AttemptID {
		state.activityError = "活动响应不属于原候选/Attempt"
		return m, nil
	}
	state.recentActivity = nil
	events := msg.batch.Activity
	for _, event := range events[max(0, len(events)-10):] {
		state.recentActivity = append(state.recentActivity, activitySummary{event.TS, shortSummary(event.Title, 100), shortSummary(event.Status, 40), shortSummary(event.Detail, 160)})
	}
	state.activityError = strings.Join(msg.batch.Diagnostics, "；")
	return m, nil
}

func shortSummary(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}

func feedbackLines(m Model, state projectState, row *consoleRow) []string {
	lines := []string{"运行与反馈"}
	if row == nil {
		return append(lines, renderEmptySelection(state)...)
	}
	lines = append(lines, "Task "+safeTerminalInlineText(row.taskID)+" / "+candidateLabel(state, row.run))
	lines = append(lines, receiptSummary(state, row)...)
	if row.run == nil {
		return append(lines, "下一步："+nextAction(m, state, row))
	}
	run := row.run
	lines = append(lines, stateText(safeTerminalInlineText(displayExecutionStatus(run)+" · "+candidateHumanStatus(state, run))))
	if run.EnvironmentBlocker != nil {
		lines = append(lines, styleAlert.Render("环境阻断："+safeTerminalText(run.EnvironmentBlocker.Fact.Reason)), safeTerminalText(run.EnvironmentBlocker.Fact.Detail), "下一步："+nextAction(m, state, row))
		if rt := state.snapshot.Connection.Snapshot.Page; rt != nil {
			for _, a := range []string{"recheck_environment", "open_terminal"} {
				if !hasCapability(rt, a) {
					label := "Ctrl+E 重查环境"
					if a == "open_terminal" {
						label = "Ctrl+O 原终端"
					}
					lines = append(lines, label+"不可用：服务未声明能力")
				}
			}
		}
	}
	if run.Error != nil {
		lines = append(lines, styleAlert.Render("错误："+safeTerminalText(*run.Error)), "u 只读核验 · d 完整诊断")
	}
	if run.PR.QueryError != nil {
		lines = append(lines, styleAlert.Render("PR 查询失败："+safeTerminalText(*run.PR.QueryError)))
	}
	if run.Decision != nil && run.Decision.Status == "pending" && run.Abandonment == nil && run.TaskStatus != "closed" {
		lines = append(lines, styleHead.Render("问题："+safeTerminalText(run.Decision.Request.Question)))
		for i, option := range run.Decision.Request.Options {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, safeTerminalInlineText(option.Label)))
		}
		return append(lines, "a 在此编辑 · F2 提交 · F8 完整背景")
	}
	active := run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" || run.ExecutionStatus == "unknown" || run.FinishTaskStatus == "pending"
	if !active && !runNeedsAttention(run) {
		if state.previewEvent != nil && state.previewEvent.AttemptID == run.AttemptID && (run.CurrentTurnID == "" || state.previewEvent.TurnID == run.CurrentTurnID) && state.previewText != "" {
			lines = append(lines, "最终回复预览（截断） · turn "+safeTerminalInlineText(state.previewEvent.TurnID), "最终回复报告（非独立测试验收）：", safeTerminalText(state.previewText))
		} else {
			lines = append(lines, "本轮 final 预览未采集/尚未核实；f 按需完整读取")
		}
		if state.previewError != "" {
			lines = append(lines, "采集缺口："+safeTerminalText(state.previewError))
		}
		if run.PR.URL != nil {
			lines = append(lines, "PR："+shortPR(*run.PR.URL)+" · "+safeTerminalInlineText(run.PR.State))
		}
		return append(lines, "下一步："+nextAction(m, state, row))
	}
	if state.activityError != "" {
		lines = append(lines, "规范活动："+safeTerminalText(state.activityError))
	}
	if len(state.recentActivity) == 0 {
		lines = append(lines, "尚无已采集活动摘要；不会从原始终端补画。")
	}
	for _, summary := range state.recentActivity {
		stamp := "未提供时间"
		if !summary.Time.IsZero() {
			stamp = summary.Time.Local().Format("15:04:05")
		}
		lines = append(lines, styleDim.Render(stamp)+" · "+safeTerminalText(summary.Title+" · "+summary.Status+" · "+summary.Detail))
	}
	return append(lines, "采样于 "+sampleTime(state.snapshot)+" · f 完整最终回复")
}
