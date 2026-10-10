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
		state.workerTransitions = nil
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
	state.workerTransitions = nil
	events := msg.batch.Activity
	diagnostics := append([]string(nil), msg.batch.Diagnostics...)
	for _, event := range events {
		if transition, ok := decodeWorkerTransition(event); ok {
			state.workerTransitions = append(state.workerTransitions, transition)
		} else if event.Kind == "worker_state" {
			diagnostics = append(diagnostics, fmt.Sprintf("状态变化记录字段不完整 · seq=%d", event.Seq))
		}
	}
	if len(state.workerTransitions) > 10 {
		state.workerTransitions = state.workerTransitions[len(state.workerTransitions)-10:]
	}
	for _, event := range events {
		if event.Kind != "worker_state" {
			state.recentActivity = append(state.recentActivity, activitySummary{event.TS, shortSummary(event.Title, 100), shortSummary(event.Status, 40), shortSummary(event.Detail, 160)})
		}
	}
	if len(state.recentActivity) > 10 {
		state.recentActivity = state.recentActivity[len(state.recentActivity)-10:]
	}
	state.activityError = strings.Join(diagnostics, "；")
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
	lines = append(lines, receiptSummary(state, row)...)
	if row.run == nil {
		return lines
	}
	run := row.run
	if pendingDecision(run) && waitingForDecision(run) {
		lines = append(lines, styleHead.Render("问题："+safeTerminalText(run.Decision.Request.Question)))
		for i, option := range run.Decision.Request.Options {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, safeTerminalInlineText(option.Label)))
		}
		lines = append(lines, "a 在此编辑 · F2 提交 · F8 完整背景")
		if state.activityError != "" {
			lines = append(lines, "活动采集："+safeTerminalText(shortSummary(state.activityError, 160)))
		}
		lines = append(lines, currentWorkerLines(run)...)
		return append(lines, workerTransitionLines(state.workerTransitions)...)
	}
	active := run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" || run.ExecutionStatus == "unknown" || run.WorkflowStatus == "running" || run.WorkflowStatus == "queued"
	if !active && !runNeedsAttention(run) {
		if state.final.previewEvent != nil && state.final.previewEvent.AttemptID == run.AttemptID && (run.CurrentTurnID == "" || state.final.previewEvent.TurnID == run.CurrentTurnID) && state.final.previewText != "" {
			lines = append(lines, "最终回复预览（截断，非独立测试验收）")
			lines = append(lines, shortResultPreview(state.final.previewText)...)
		}
		if gap := firstText(state.final.previewError, state.final.finalError); gap != "" {
			lines = append(lines, "采集缺口："+safeTerminalText(shortSummary(gap, 160)))
		}
	}
	lines = append(lines, currentWorkerLines(run)...)
	transitions := workerTransitionLines(state.workerTransitions)
	lines = append(lines, transitions...)
	if state.activityError != "" {
		lines = append(lines, "活动采集："+safeTerminalText(shortSummary(state.activityError, 160)))
	}
	if len(transitions) == 0 {
		lines = append(lines, "尚无流程或状态变化 · d 查看原始记录")
	}
	return lines
}
