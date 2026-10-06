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
	if row == nil || row.run == nil {
		return append(lines, "所选任务暂无执行；按 s 选择方案开始。")
	}
	run := row.run
	lines = append(lines, "当前 "+candidateLabel(state, run)+" · "+safeTerminalText(displayExecutionStatus(run))+" · "+candidateHumanStatus(state, run))
	if run.Decision != nil {
		if action := feedbackAction(state, run); action != nil {
			lines = append(lines, "答案预览："+safeTerminalText(firstText(action.DecisionDraftOptionID, "自由文字"))+" · "+safeTerminalText(action.DecisionDraftNote), "回答回执："+safeTerminalText(action.Message))
			lines = append(lines, decisionAnswerStatusLines(state, row)...)
		}
		if run.Decision.Status == "pending" && !feedbackInProgress(state, run) && run.Abandonment == nil && run.TaskStatus != "closed" {
			lines = append(lines, "问题："+safeTerminalText(run.Decision.Request.Question))
			for i, option := range run.Decision.Request.Options {
				lines = append(lines, fmt.Sprintf("%d. %s", i+1, safeTerminalInlineText(option.Label)))
			}
			lines = append(lines, "a 在此编辑 · F2 提交 · F8 编辑时查看完整背景")
			return lines
		}
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
		lines = append(lines, stamp+" · "+safeTerminalText(summary.Title+" · "+summary.Status+" · "+summary.Detail))
	}
	return append(lines, "采样于 "+sampleTime(state.snapshot)+" · f 完整最终回复")
}
