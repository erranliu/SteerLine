package console

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/activityview"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

var (
	styleHead     = lipgloss.NewStyle().Bold(true)
	styleDim      = lipgloss.NewStyle().Faint(true)
	styleSelected = lipgloss.NewStyle().Bold(true).Reverse(true)
	styleAlert    = lipgloss.NewStyle().Bold(true)
	styleLabel    = lipgloss.NewStyle().Bold(true)
	styleRule     = lipgloss.NewStyle().Faint(true)
)

func render(m Model) string {
	if m.width <= 0 || m.height <= 0 {
		return "SteerLine Console 正在读取…"
	}
	header := styleHead.Render("SteerLine · 执行控制台")
	if len(m.projects) == 0 {
		return header + "\n\n" + renderEmptyProjects(m.applicationEtc) + "\n\nq 退出"
	}
	if m.selected < 0 || m.selected >= len(m.projects) {
		return header + "\n" + renderEmptyProjects(m.applicationEtc)
	}
	state := &m.projects[m.selected]
	projects := renderProjectStrip(m)
	status := renderStatusLine(m, *state)
	footerLines := wrapLines(strings.Split(renderFooter(m, *state), "\n"), m.width)
	contentHeight := m.height - 3 - len(footerLines)
	if contentHeight < 3 {
		contentHeight = 3
	}
	var body string
	if m.profileChoice != nil {
		body = renderExecutionProfileChoice(m, *state, contentHeight)
	} else if m.help {
		body = strings.Join(fitLines(renderHelp(*state), m.width, contentHeight), "\n")
	} else if m.confirm != nil {
		body = renderStopConfirmation(m, *state, contentHeight)
	} else if m.answer != nil {
		body = renderAnswerDialog(m, *state, contentHeight)
	} else if m.detailExpanded {
		expanded := *state
		expanded.detailScroll = m.expandedDetailScroll
		body = strings.Join(fitLines(renderDetail(m, expanded, m.selectedRow(m.selected), m.width, contentHeight), m.width, contentHeight), "\n")
	} else if compactLayout(m) {
		panel := renderNarrowPanel(m, *state, contentHeight)
		body = strings.Join(fitLines(panel, m.width, contentHeight), "\n")
	} else {
		leftWidth := listPanelWidth(m)
		rightWidth := activityPanelWidth(m)
		detailHeight := detailPanelHeight(m)
		activityHeight := activityPanelHeight(m)
		left := fitLines(renderRows(m, *state, leftWidth, contentHeight), leftWidth, contentHeight)
		preview := *state
		preview.detailScroll = 0
		detail := renderDetailPreview(m, preview, m.selectedRow(m.selected), rightWidth, 100000)
		if len(detail) > detailHeight {
			detail = append(detail[:detailHeight-1], styleDim.Render("d / Enter 查看完整详情"))
		}
		right := fitLines(detail, rightWidth, detailHeight)
		right = append(right, styleRule.Render(strings.Repeat("─", rightWidth)))
		focus := ""
		if state.focus == 1 {
			focus = " [焦点]"
		}
		right = append(right, truncate(styleLabel.Render("任务动态"+focus)+" · "+dynamicPanelDisplayName(state.panel)+"  (A 活动 / c 会话 / f 最终回复)", rightWidth))
		right = append(right, fitLines(renderDynamicPanel(m, *state, rightWidth, activityHeight), rightWidth, activityHeight)...)
		body = joinLines(left, right, leftWidth, contentHeight)
	}
	lines := []string{header, projects, status, body}
	lines = append(lines, footerLines...)
	return strings.Join(lines, "\n")
}

func renderEmptyProjects(configDirectory string) string {
	return "没有可用项目配置。\n配置目录：" + safeTerminalText(configDirectory) + "\n建立配置后再次运行 steerline console；可从 examples/config.json 复制并按项目填写。"
}

func renderProjectStrip(m Model) string {
	name := safeTerminalInlineText(m.projects[m.selected].ref.Name)
	line := fmt.Sprintf("项目 %d/%d  %s", m.selected+1, len(m.projects), styleSelected.Render(" "+name+" "))
	if len(m.projects) > 1 {
		line += "  ←/→ 切换"
	}
	if m.prefsErr != "" {
		line += "  " + styleAlert.Render(safeTerminalText(m.prefsErr))
	}
	return truncate(line, m.width)
}

func renderStatusLine(m Model, state projectState) string {
	runtimeState := state.snapshot.Connection.Snapshot.Runtime
	if runtimeState == nil {
		runtimeState = state.snapshot.Observation.Runtime
	}
	service := projectStatus(state.ref, state.snapshot)
	mode, paused, slot := "未知", false, ""
	if runtimeState != nil {
		mode = runtimeState.Control.DispatchMode
		paused = runtimeState.Control.DispatchPaused
		if runtimeState.SlotOccupied {
			active := 0
			for _, run := range runtimeState.Runs {
				if run != nil && (run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" || run.ExecutionStatus == "unknown" || run.FinishTaskStatus == "waiting_decision") {
					active++
				}
			}
			if mode == "manual" {
				if active > 0 {
					slot = fmt.Sprintf("活动 %d", active)
				} else {
					slot = "执行待核实"
				}
			} else if active > 1 {
				slot = fmt.Sprintf("等待活动 %d", active)
			} else {
				slot = "执行 " + stringValue(runtimeState.CurrentTask)
			}
		}
	}
	if paused {
		mode += " · 已暂停"
	}
	switch strings.TrimSuffix(mode, " · 已暂停") {
	case "manual":
		mode = strings.Replace(mode, "manual", "手动", 1)
	case "auto":
		mode = strings.Replace(mode, "auto", "自动", 1)
	}
	ready, cached, running, decision, merge, attention := projectCounts(state, m.clock.Now())
	parts := []string{"服务 " + service, "派发 " + mode, fmt.Sprintf("Ready %d", ready)}
	if executionConfigMismatch(state.ref, state.snapshot) {
		parts = append(parts, "执行配置待核对")
	}
	if slot != "" {
		parts = append(parts, slot)
	}
	for _, item := range []struct {
		count int
		label string
	}{{cached, "缓存"}, {running, "进行中"}, {decision, "待决策"}, {merge, "待合并"}, {attention, "需处理"}} {
		if item.count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", item.label, item.count))
		}
	}
	if state.snapshot.Connection.State != productapi.ConnectionConnected && !state.snapshot.Observation.SampledAt.IsZero() {
		parts = append(parts, "采样于 "+sampleTime(state.snapshot))
	}
	if m.status != "" {
		parts = append([]string{"提示 " + safeTerminalInlineText(m.status)}, parts...)
	}
	if diagnostic := state.snapshot.Connection.Diagnostic; diagnostic != "" {
		parts = append(parts, "连接诊断 "+safeTerminalText(diagnostic))
	}
	return truncate(strings.Join(parts, "  ·  "), m.width)
}

func renderDynamicPanel(m Model, state projectState, width, height int) []string {
	row := m.selectedRow(m.selected)
	switch state.panel {
	case 2:
		return renderSession(state, row, width, height, m.clock.Now())
	case 3:
		return renderFinal(state, row, width, height)
	case 5:
		return renderDecisionHistory(state, row, width, height, m.clock.Now())
	case 6:
		return renderRecoveryInspection(m, state, width, height)
	default:
		return renderActivity(state, row, width, height)
	}
}

func renderNarrowPanel(m Model, state projectState, height int) []string {
	views := "Tab 列表/动态 · d 完整详情"
	views += " · A活动 · c会话 · f最终回复"
	if state.focus == 0 {
		return append([]string{"视图 任务列表  |  " + views}, renderRows(m, state, m.width, height-1)...)
	}
	lines := []string{"视图 任务动态 · " + dynamicPanelDisplayName(state.panel) + "  |  " + views}
	return append(lines, renderDynamicPanel(m, state, m.width, height-1)...)
}

func dynamicPanelDisplayName(index int) string {
	if index == 0 || index == 4 {
		return "活动"
	}
	return panelDisplayName(index)
}

func panelDisplayName(index int) string {
	switch index {
	case 1:
		return "活动"
	case 2:
		return "会话"
	case 3:
		return "最终回复"
	case 4:
		return "列表"
	case 5:
		return "决策历史"
	case 6:
		return "恢复核验"
	default:
		return "详情"
	}
}

func renderDetail(m Model, state projectState, row *consoleRow, width, height int) []string {
	return renderDetailContent(m, state, row, width, height, true)
}

func renderDetailPreview(m Model, state projectState, row *consoleRow, width, height int) []string {
	return renderDetailContent(m, state, row, width, height, false)
}

func renderDetailContent(m Model, state projectState, row *consoleRow, width, height int, full bool) []string {
	lines := []string{styleLabel.Render("详情")}
	if row == nil {
		if state.snapshotLoaded && state.selectedKey != "" {
			lines = append(lines, "上次选中对象已不在当前快照中；按 ↑/↓ 重新选择。")
		}
		lines = append(lines, renderEmptySelection(state)...)
		return scrollLines(wrapLines(lines, width), state.detailScroll, height)
	}
	if row.native != nil && row.run == nil {
		if full {
			lines = append(lines, renderNativeTaskDetail(state, *row.native)...)
		} else {
			lines = append(lines, "任务 "+safeTerminalInlineText(row.native.TaskID)+" · "+safeTerminalInlineText(row.native.Title))
			lines = append(lines, renderNativeTaskRelations(state, *row.native)...)
		}
		if row.kind == rowReady {
			lines = append(lines, renderControlAvailability(state, row, m.clock.Now())...)
			lines = append(lines, renderActionLines(state, row.taskID)...)
		}
		return scrollLines(wrapLines(lines, width), state.detailScroll, height)
	}
	if row.kind == rowReady {
		valid, _ := state.client.CandidateAvailability(state.snapshot, m.clock.Now())
		stateText := "Ready 候选"
		if !valid {
			stateText = "候选缓存 · 禁止开始"
		}
		lines = append(lines, "Task："+safeTerminalInlineText(row.taskID), "标题："+safeTerminalInlineText(row.title), "提供方状态："+stateText)
		if row.candidate.Summary != "" {
			lines = append(lines, "摘要："+safeTerminalText(row.candidate.Summary))
		}
		lines = append(lines, renderControlAvailability(state, row, m.clock.Now())...)
		lines = append(lines, renderActionLines(state, row.taskID)...)
		return scrollLines(wrapLines(lines, width), state.detailScroll, height)
	}
	run := row.run
	if state.snapshot.Connection.State == productapi.ConnectionOffline || state.snapshot.Connection.State == productapi.ConnectionNoRecord {
		lines = append(lines, "服务入口："+safeTerminalText(serviceWindowInstruction(state.ref)), "配置："+safeTerminalText(state.ref.ConfigPath))
	}
	lines = append(lines, "任务 "+safeTerminalInlineText(run.TaskID)+" · "+safeTerminalInlineText(run.Title))
	if full {
		lines = append(lines,
			"任务状态："+safeTerminalText(rowTaskStatus(*row)),
			"执行状态："+safeTerminalText(displayExecutionStatus(run)),
		)
	} else {
		lines = append(lines, renderRunExecutionProfileSummary(state, run)...)
	}
	lines = append(lines, "需要你操作："+taskNextAction(run))
	lines = append(lines, renderActionLines(state, run.TaskID)...)
	if run.PR.URL != nil || run.PR.State != "" && run.PR.State != "unknown" {
		lines = append(lines, "PR："+safeTerminalText(firstText(run.PR.State, "unknown")))
		if run.PR.URL != nil && *run.PR.URL != "" {
			lines = append(lines, "PR 地址："+safeTerminalText(*run.PR.URL))
		}
	}
	if run.PR.QueryError != nil && *run.PR.QueryError != "" {
		lines = append(lines, "PR 查询失败："+safeTerminalText(*run.PR.QueryError))
	}
	if run.EnvironmentBlocker != nil {
		lines = append(lines, "环境阻断："+safeTerminalText(run.EnvironmentBlocker.Fact.Reason+" · "+run.EnvironmentBlocker.Fact.Detail), "Ctrl+E 重新检查 · Ctrl+O 打开原终端；反馈草稿保留，环境就绪后再发送")
	}
	if run.Error != nil && *run.Error != "" {
		lines = append(lines, "错误："+safeTerminalText(*run.Error))
	}
	if run.Summary != nil && *run.Summary != "" {
		lines = append(lines, "摘要："+safeTerminalText(*run.Summary))
	}
	if state.snapshot.Connection.State != productapi.ConnectionConnected {
		lines = append(lines, "采样于 "+sampleTime(state.snapshot))
	}
	if verified := executionVerifiedAt(state, run); verified != "未知" && verified != "—" {
		lines = append(lines, "最后核验："+verified)
	}
	if state.snapshot.DomainError != "" {
		lines = append(lines, "execution/attempt 查询："+safeTerminalText(state.snapshot.DomainError))
	}
	lines = append(lines, renderControlAvailability(state, row, m.clock.Now())...)
	if run.Decision != nil && run.Decision.Status == "pending" {
		lines = append(lines, "")
		if full {
			lines = append(lines, renderDecisionIdentity(state, run, m.clock.Now())...)
		}
		lines = append(lines, "问题："+safeTerminalText(run.Decision.Request.Question), "上下文："+safeTerminalText(run.Decision.Request.Context))
		for i, option := range run.Decision.Request.Options {
			lines = append(lines, fmt.Sprintf("选项 %d · %s：%s", i+1, safeTerminalText(option.ID), safeTerminalText(option.Label)))
			lines = append(lines, "  详情："+safeTerminalText(option.Detail))
		}
		lines = append(lines, decisionAnswerStatusLines(state, row)...)
	} else if run.Decision != nil {
		lines = append(lines, "")
		if full {
			lines = append(lines, renderDecisionIdentity(state, run, m.clock.Now())...)
		}
		lines = append(lines, "决策阶段："+decisionStatusLabel(run.Decision.Status), "当前答案："+safeTerminalText(activityview.DecisionAnswerSummary(run)))
	} else if run.FinishTaskStatus == "waiting_decision" && run.EnvironmentBlocker == nil {
		lines = append(lines, "待决策状态存在，但当前 decision_id/问题记录缺失或尚未同步；答案通道未启用。")
	}
	if state.ref.Presentation.TaskAuthority == "native" {
		if task := nativeDetailForTask(state, firstText(run.DomainTaskID, run.TaskID)); task != nil {
			lines = append(lines, "", "任务关系")
			lines = append(lines, renderNativeTaskRelations(state, *task)...)
		}
	}
	if !full {
		return scrollLines(wrapLines(lines, width), state.detailScroll, height)
	}
	lines = append(lines, "", "技术信息", "run_id："+safeTerminalText(run.RunID))
	lines = append(lines, "执行："+safeTerminalText(firstText(run.ExecutionStatus, "unknown"))+" · 交付："+safeTerminalText(firstText(run.FinishTaskStatus, "unknown")))
	lines = append(lines, renderFixedRunExecutionProfile(state, run)...)
	if run.Process != nil && run.Process.Terminal != "" {
		lines = append(lines, "宿主终端标识："+safeTerminalText(run.Process.Terminal))
	}
	if run.ExecutionID != "" {
		lines = append(lines, "execution_id："+safeTerminalText(run.ExecutionID))
	}
	if attempt := selectedAttempt(state, run); attempt != "" {
		lines = append(lines, "attempt_id："+safeTerminalText(attempt))
	}
	if run.Backend != "" || run.Protocol != "" || run.Target.ID != "" {
		lines = append(lines, "后端/协议/目标："+safeTerminalText(strings.Join([]string{run.Backend, run.Protocol, run.Target.ID}, " / ")))
	}
	if run.PR.QueriedAt != nil {
		lines = append(lines, "PR 查询时间："+safeTerminalText(ptrTime(run.PR.QueriedAt)))
	}
	if run.WorktreePath != nil && *run.WorktreePath != "" {
		lines = append(lines, "工作目录："+safeTerminalText(*run.WorktreePath))
	}
	if run.WorktreeBranch != nil && *run.WorktreeBranch != "" {
		lines = append(lines, "分支："+safeTerminalText(*run.WorktreeBranch))
	}
	return scrollLines(wrapLines(lines, width), state.detailScroll, height)
}

func nativeDetailForTask(state projectState, taskID string) *contracts.NativeTaskDetail {
	if state.snapshot.SelectedNative != nil && state.snapshot.SelectedNative.TaskID == taskID {
		return state.snapshot.SelectedNative
	}
	if state.snapshot.NativeTasks != nil {
		for i := range state.snapshot.NativeTasks.Tasks {
			if state.snapshot.NativeTasks.Tasks[i].TaskID == taskID {
				return &state.snapshot.NativeTasks.Tasks[i]
			}
		}
	}
	return nil
}

func renderNativeTaskDetail(state projectState, task contracts.NativeTaskDetail) []string {
	lines := []string{
		"任务 " + safeTerminalInlineText(task.TaskID) + " · " + safeTerminalInlineText(task.Title),
		"状态：" + safeTerminalText(task.Status),
	}
	return append(lines, renderNativeTaskRelations(state, task)...)
}

func renderNativeTaskRelations(state projectState, task contracts.NativeTaskDetail) []string {
	var lines []string
	if state.snapshot.Connection.State == productapi.ConnectionOffline || state.snapshot.Connection.State == productapi.ConnectionNoRecord {
		lines = append(lines, "离线只读快照 · 采样于 "+sampleTime(state.snapshot))
	}
	if state.snapshot.NativeTaskError != "" {
		lines = append(lines, "关系查询："+safeTerminalText(state.snapshot.NativeTaskError))
	}
	if len(task.UnmetPredecessors) > 0 {
		lines = append(lines, "", "未满足的前置")
		for _, blocker := range task.UnmetPredecessors {
			lines = append(lines, safeTerminalInlineText(fmt.Sprintf("task %s · %s · %s", blocker.TaskID, firstText(blocker.Status, "unknown"), firstText(blocker.Blocker, "predecessor_not_delivered"))))
		}
	} else if task.DependencyReady {
		lines = append(lines, "前置：已满足")
	}
	if len(task.StartupBlockers) > 0 {
		for _, blocker := range task.StartupBlockers {
			if blocker == "dispatch_paused" {
				if rt := state.snapshot.Connection.Snapshot.Runtime; rt != nil && rt.Control.DispatchPaused {
					continue
				}
			}
			lines = append(lines, "任务阻塞："+displayActionReason(blocker))
		}
	}
	if len(task.Predecessors) > 0 {
		ids := make([]string, 0, len(task.Predecessors))
		for _, predecessor := range task.Predecessors {
			ids = append(ids, safeTerminalInlineText(predecessor.TaskID))
			if len(task.UnmetPredecessors) == 0 && !predecessor.Delivered && predecessor.Blocker != "" {
				lines = append(lines, "前置阻塞：task "+safeTerminalInlineText(predecessor.TaskID)+" · "+safeTerminalInlineText(predecessor.Blocker))
			}
		}
		lines = append(lines, "前置任务："+strings.Join(ids, "、"))
	}
	if len(task.Successors) > 0 {
		ids := make([]string, 0, len(task.Successors))
		for _, successor := range task.Successors {
			ids = append(ids, safeTerminalInlineText(successor))
		}
		lines = append(lines, "后继任务："+strings.Join(ids, "、"))
	}
	if task.Contract.SpecRef.Path != "" {
		lines = append(lines, "Spec："+safeTerminalInlineText(task.Contract.SpecRef.Path))
	}
	if task.Delivery != nil {
		lines = append(lines,
			"PR："+safeTerminalInlineText(task.Delivery.PRURL),
		)
		if !task.Delivery.MergedAt.IsZero() {
			lines = append(lines, "合并于 "+task.Delivery.MergedAt.Local().Format(timeLayout))
		}
	}
	return lines
}

func renderActionLines(state projectState, taskID string) []string {
	actions := make([]*actionState, 0, len(state.actions))
	for _, action := range state.actions {
		if action.TaskID == taskID {
			actions = append(actions, action)
		}
	}
	sort.SliceStable(actions, func(i, j int) bool { return actions[i].UpdatedAt.After(actions[j].UpdatedAt) })
	if len(actions) == 0 {
		return nil
	}
	visible := make([]*actionState, 0, 3)
	for _, action := range actions {
		if !action.Resolved && len(visible) < 3 {
			visible = append(visible, action)
		}
	}
	if len(visible) == 0 {
		visible = append(visible, actions[0])
	}
	lines := []string{"", "本任务操作回执"}
	for _, action := range visible {
		lines = append(lines, safeTerminalText(actionStatusLabel(action.Status))+" · "+safeTerminalText(action.Message))
	}
	return lines
}

func actionStatusLabel(status string) string {
	switch status {
	case "submitting":
		return "提交中"
	case "pending", "in_flight", "applying":
		return "处理中"
	case "applied", "accepted":
		return "请求已接受"
	case "rejected":
		return "请求未接受"
	case "failed":
		return "操作失败"
	case "indeterminate":
		return "结果待确认"
	default:
		return status
	}
}

func renderControlAvailability(state projectState, row *consoleRow, now time.Time) []string {
	var lines []string
	if state.ref.Error != "" {
		return []string{"控制不可用：配置错误 · " + safeTerminalText(state.ref.Error)}
	}
	if state.ref.Conflict != "" {
		return []string{"控制不可用：" + safeTerminalText(state.ref.Conflict)}
	}
	if row != nil && row.kind == rowReady {
		reason := candidateActionReason(state, now)
		if reason == "" {
			lines = append(lines, "操作：s 选择执行方案并开始任务 "+safeTerminalText(row.taskID))
		} else {
			lines = append(lines, "开始不可用："+displayActionReason(reason))
		}
	}
	if row != nil && row.kind == rowRun && row.run != nil && row.run.Decision != nil && row.run.Decision.Status == "pending" {
		reason := decisionAnswerAvailability(state, row)
		if strings.HasPrefix(reason, "提交后") {
			lines = append(lines, "操作：a 回答当前决策")
		} else {
			lines = append(lines, "回答不可用："+safeTerminalText(reason))
		}
	}
	return lines
}

func displayActionReason(reason string) string {
	switch {
	case reason == "dispatch_paused":
		return "派发已暂停"
	case strings.HasPrefix(reason, "cursor_invalidated:"):
		return "任务快照版本变化；等待刷新后再试"
	default:
		return safeTerminalText(reason)
	}
}

const timeLayout = "2006-01-02 15:04:05"

func renderEmptySelection(state projectState) []string {
	if state.snapshot.Connection.State == productapi.ConnectionOffline || state.snapshot.Connection.State == productapi.ConnectionNoRecord {
		return []string{"服务离线 · 可以查看已有 runs 与日志", "服务入口：" + safeTerminalText(serviceWindowInstruction(state.ref)), "配置路径：" + safeTerminalText(state.ref.ConfigPath), terminalCapabilities()}
	}
	if state.snapshot.Connection.Diagnostic != "" {
		return []string{"服务状态：" + safeTerminalText(string(state.snapshot.Connection.State)), "连接诊断：" + safeTerminalText(state.snapshot.Connection.Diagnostic), "配置路径：" + safeTerminalText(state.ref.ConfigPath)}
	}
	return []string{"(没有选中对象)", "Enter 仅打开详情；开始任务请按 s。"}
}

func renderActivity(state projectState, row *consoleRow, width, height int) []string {
	title := "活动 · 执行过程"
	if row != nil && row.run != nil {
		title += " · execution " + safeTerminalText(row.run.ExecutionID) + " · attempt " + safeTerminalText(selectedAttempt(state, row.run))
	}
	lines := []string{styleLabel.Render(title)}
	if state.activityError != "" {
		lines = append(lines, styleAlert.Render("日志读取："+safeTerminalText(state.activityError)))
	}
	if len(state.activityEvents) == 0 {
		if row == nil || row.run == nil {
			lines = append(lines, "所选任务尚无执行记录；开始后这里显示任务动态。")
		} else if state.activityKey == "" {
			lines = append(lines, "所选 run/attempt 没有日志记录。")
		} else if state.activityError != "" {
			lines = append(lines, "活动源不可用；会话页可能仍有当前格式记录。")
		} else {
			lines = append(lines, "等待完整日志记录…")
		}
	} else {
		for _, event := range state.activityEvents {
			event.Kind = safeTerminalText(event.Kind)
			event.ItemID = safeTerminalText(event.ItemID)
			event.Title = safeTerminalText(event.Title)
			event.Detail = safeTerminalText(event.Detail)
			event.Status = safeTerminalText(event.Status)
			event.Meta = safeActivityMetadata(event.Meta)
			rendered := activityview.RenderActivity(event, activityview.RenderOptions{MaxColumns: width})
			lines = append(lines, strings.Split(rendered, "\n")...)
		}
	}
	if row != nil && row.run != nil {
		lines = append(lines, "来源：activity journal · attempt "+safeTerminalText(selectedAttempt(state, row.run)))
	}
	lines = wrapLines(lines, width)
	if state.view.Follow {
		return scrollLinesFromEnd(lines, 0, height)
	}
	return scrollLines(lines, state.logScroll, height)
}

func safeActivityMetadata(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	safe := make(map[string]any, len(meta))
	for key, value := range meta {
		safe[safeTerminalText(key)] = safeActivityValue(value)
	}
	return safe
}

func safeActivityValue(value any) any {
	switch typed := value.(type) {
	case string:
		return safeTerminalText(typed)
	case map[string]any:
		return safeActivityMetadata(typed)
	case []any:
		items := make([]any, len(typed))
		for i, item := range typed {
			items[i] = safeActivityValue(item)
		}
		return items
	default:
		return safeTerminalText(fmt.Sprint(value))
	}
}

func decisionStatusLabel(status string) string {
	switch status {
	case "pending":
		return "未提交"
	case "answer_saved":
		return "答案已保存，等待服务处理"
	case "resuming":
		return "服务正在核验原执行并尝试续跑"
	case "reply_accepted":
		return "后端已接受，答案应用待核实"
	case "applied":
		return "答案已应用"
	case "failed":
		return "明确失败，应用未核实"
	case "superseded":
		return "历史问题已被新问题替代"
	default:
		return "状态未知：" + status
	}
}

func renderDecisionHistory(state projectState, row *consoleRow, width, height int, now time.Time) []string {
	lines := []string{styleLabel.Render("决策历史 · 按 decision_id 留档")}
	if row == nil || row.run == nil {
		return append(lines, "选择一个 run 后查看其全部决策。")
	}
	run := row.run
	lines = append(lines, "项目："+safeTerminalText(state.ref.Name), "task_id："+safeTerminalText(run.TaskID), "run_id："+safeTerminalText(run.RunID))
	if state.snapshot.Domain == nil {
		lines = append(lines, "SQLite 决策历史不可用："+safeTerminalText(state.snapshot.DomainError))
		return scrollLines(wrapLines(lines, width), state.detailScroll, height)
	}
	records := make([]executionmodel.DecisionRecord, 0)
	for _, record := range state.snapshot.Domain.Decisions {
		if record.WorkflowRunID == run.RunID {
			records = append(records, record)
		}
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].DecisionID < records[j].DecisionID
		}
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	if len(records) == 0 {
		lines = append(lines, "当前 run 没有持久化决策记录。")
	}
	for _, record := range records {
		var request executionmodel.DecisionRequest
		if err := json.Unmarshal(record.Request, &request); err != nil {
			lines = append(lines, "", "decision_id："+safeTerminalText(record.DecisionID), "问题记录格式未知；原记录保留，未猜测或导入。")
			continue
		}
		lines = append(lines, "", "decision_id："+safeTerminalText(record.DecisionID), "execution_id："+safeTerminalText(displayDash(record.ExecutionID))+" · attempt_id："+safeTerminalText(displayDash(record.AttemptID)), "状态："+safeTerminalText(record.Status)+" · version："+strconv.FormatUint(record.Version, 10))
		if !record.CreatedAt.IsZero() {
			age := now.Sub(record.CreatedAt)
			if age < 0 {
				age = 0
			}
			lines = append(lines, "提问时间："+record.CreatedAt.Local().Format(timeLayout)+" · 经时："+age.Round(time.Second).String())
		}
		lines = append(lines, "问题："+safeTerminalText(request.Question), "上下文："+safeTerminalText(request.Context))
		for i, option := range request.Options {
			recommended := ""
			if request.Recommended == option.ID {
				recommended = " · 推荐"
			}
			lines = append(lines, fmt.Sprintf("选项 %d · %s：%s%s", i+1, safeTerminalText(option.ID), safeTerminalText(option.Label), recommended), "  详情："+safeTerminalText(option.Detail))
		}
		if len(record.Answer) > 0 {
			var answer executionmodel.DecisionAnswer
			if err := json.Unmarshal(record.Answer, &answer); err == nil {
				lines = append(lines, "答案 · option_id："+safeTerminalText(displayDash(answer.OptionID))+" · by："+safeTerminalText(answer.AnsweredBy), "补充说明："+safeTerminalText(answer.Note))
			}
		} else {
			lines = append(lines, "答案：尚未接受")
		}
	}
	return scrollLines(wrapLines(lines, width), state.detailScroll, height)
}

func renderRecoveryInspection(m Model, state projectState, width, height int) []string {
	lines := []string{styleLabel.Render("恢复核验报告 · 只读")}
	if state.recoveryBusy {
		return append(lines, "正在核对执行身份、工作目录、领取和 PR 状态…")
	}
	if state.recoveryError != "" {
		return append(lines, "核验失败："+safeTerminalText(state.recoveryError), "r 重新读取 · Enter 返回")
	}
	report := state.recoveryReport
	if report == nil {
		return append(lines, "没有当前恢复报告。按 u 读取所选 run。")
	}
	lines = append(lines,
		"项目："+safeTerminalText(state.ref.Name),
		"task_id："+safeTerminalText(report.TaskID)+" · run_id："+safeTerminalText(report.RunID),
		"run revision："+strconv.FormatUint(report.RunRevision, 10)+" · 采样时间："+report.SampledAt.Local().Format(timeLayout),
		"service instance/status："+safeTerminalText(report.ServiceInstanceID)+" / "+safeTerminalText(report.ServiceStatus),
		"dispatch："+safeTerminalText(report.DispatchMode)+" · paused："+fmt.Sprint(report.DispatchPaused)+" · slot occupied："+fmt.Sprint(report.SlotOccupied),
		"执行/交付："+safeTerminalText(report.ExecutionStatus)+" / "+safeTerminalText(report.DeliveryStatus),
		"decision："+safeTerminalText(displayDash(report.DecisionID))+" · "+safeTerminalText(displayDash(report.DecisionStatus)),
		"后端/协议/目标："+safeTerminalText(report.Backend+" / "+report.Protocol+" / "+report.Target.ID),
		"宿主/终端核验："+safeTerminalText(report.ProcessCheck),
	)
	if report.Workspace != nil {
		lines = append(lines, "worktree："+safeTerminalText(report.Workspace.Worktree), "base / HEAD："+safeTerminalText(report.Workspace.Base+" / "+report.Workspace.Head), fmt.Sprintf("dirty：%t · 采样：%s", report.Workspace.Dirty, report.Workspace.CheckedAt.Local().Format(timeLayout)))
	} else {
		lines = append(lines, "worktree 核验失败："+safeTerminalText(report.WorkspaceError))
	}
	if report.NativeClaim != nil {
		lines = append(lines, "center claim：task "+safeTerminalText(report.NativeClaim.TaskID)+" · "+safeTerminalText(report.NativeClaim.Status)+" · claimant "+safeTerminalText(report.NativeClaim.Claimant))
	} else {
		lines = append(lines, "claim 核验失败："+safeTerminalText(report.ClaimError))
	}
	if report.PR != nil {
		lines = append(lines, "PR："+safeTerminalText(report.PR.URL)+" · "+safeTerminalText(report.PR.State)+" · repository "+safeTerminalText(report.PR.Repository)+" · base "+safeTerminalText(report.PR.BaseBranch))
	} else if report.PRError != "" {
		lines = append(lines, "PR 核验："+safeTerminalText(report.PRError))
	} else {
		lines = append(lines, "PR：未绑定")
	}
	lines = append(lines, "", "原 worker 会话及反馈请求按当前格式对账；需要输入时请在待决策页回复。")
	return scrollLines(wrapLines(lines, width), state.detailScroll, height)
}

func renderDecisionIdentity(state projectState, run *contracts.Run, now time.Time) []string {
	if run == nil || run.Decision == nil {
		return nil
	}
	waiting := "等待时长：未知"
	if state.snapshot.Domain != nil {
		for _, record := range state.snapshot.Domain.Decisions {
			if record.DecisionID == run.Decision.ID && !record.CreatedAt.IsZero() {
				duration := now.Sub(record.CreatedAt)
				if duration < 0 {
					duration = 0
				}
				waiting = "等待时长：" + duration.Round(time.Second).String()
				break
			}
		}
	}
	return []string{"项目：" + safeTerminalText(state.ref.Name), "task_id：" + safeTerminalText(run.TaskID), "run_id：" + safeTerminalText(run.RunID), "decision_id：" + safeTerminalText(run.Decision.ID), "attempt_id：" + safeTerminalText(run.Decision.AttemptID), waiting}
}

func decisionAnswerAvailability(state projectState, row *consoleRow) string {
	if row == nil || row.run == nil || row.run.Decision == nil || row.run.Decision.Status != "pending" {
		return "当前没有待回答的问题"
	}
	reason, ok := state.snapshot.DecisionReasons[row.run.RunID]
	if !ok {
		return "决策资格尚未核验；请刷新"
	}
	return reason
}

func decisionAnswerStatusLines(state projectState, row *consoleRow) []string {
	reason := decisionAnswerAvailability(state, row)
	if strings.HasPrefix(reason, "提交后") {
		if row != nil && row.run != nil && state.ref.Presentation.NativeWorkflow {
			return []string{"a 核验原会话身份并打开回答草稿 · " + reason, "显式提交后分别显示答案保存、发送回执与原会话轮次开始。"}
		}
		return []string{"a 核验固定后端能力并打开回答草稿 · " + reason, "显式提交后仍分别核验答案接受、后端送达、轮次启动和答案应用。"}
	}
	return []string{"a 不可用 · " + safeTerminalText(reason)}
}

func renderStopConfirmation(m Model, state projectState, height int) string {
	if m.confirm == nil {
		return ""
	}
	grace := state.ref.Presentation.ShutdownGrace
	if grace == "" {
		grace = "配置的宽限时间"
	}
	lines := []string{"停止服务确认", "", "目标实例：" + safeTerminalText(m.confirm.instance), "这会停止新派发，并等待当前 worker 最长 " + grace + " 后按服务策略结束；工作现场会保留。", "request_id：" + m.confirm.requestID, "", "Enter/y 确认停止 · Esc/n 取消"}
	width := max(20, m.width-4)
	visible := fitLines(scrollLines(wrapLines(lines, width), max(0, m.confirm.scroll), height), m.width, height)
	return strings.Join(visible, "\n")
}

func renderAnswerDialog(m Model, state projectState, height int) string {
	dialog := m.answer
	if dialog == nil {
		return ""
	}
	request, ok := decisionRequestFor(state, dialog.runID, dialog.decisionID)
	lines := []string{"人工决策 · 显式提交", "项目：" + safeTerminalText(state.ref.Name), "task_id：" + safeTerminalText(displayDash(m.answerRunTask(state, dialog))), "run_id：" + safeTerminalText(dialog.runID), "decision_id：" + safeTerminalText(dialog.decisionID), fmt.Sprintf("运行版本：%d · 问题版本：%d · 目标服务：%s", dialog.runRevision, dialog.decisionVersion, safeTerminalText(dialog.instance))}
	if !ok {
		lines = append(lines, "原问题正文暂不可读；保留的草稿不会提交。")
	} else {
		lines = append(lines, "", "问题："+safeTerminalText(request.Question), "上下文："+safeTerminalText(request.Context), "")
		for i, option := range request.Options {
			marker := "  "
			if !dialog.noteFocus && dialog.optionIndex == i {
				marker = "› "
			}
			recommendation := ""
			if request.Recommended == option.ID {
				recommendation = " · 推荐"
			}
			lines = append(lines, marker+fmt.Sprintf("%d. %s · %s%s", i+1, safeTerminalText(option.ID), safeTerminalText(option.Label), recommendation), "    详情："+safeTerminalText(option.Detail))
		}
		if request.AllowFreeText {
			marker := "  "
			if !dialog.noteFocus && dialog.optionIndex == len(request.Options) {
				marker = "› "
			}
			lines = append(lines, marker+"自定义答复 · option_id 留空，必须填写说明")
		}
	}
	lines = append(lines, "", "补充说明：")
	note := append([]rune(nil), dialog.note...)
	if dialog.noteFocus && dialog.cursor >= 0 && dialog.cursor <= len(note) {
		note = append(note, 0)
		copy(note[dialog.cursor+1:], note[dialog.cursor:])
		note[dialog.cursor] = '│'
	}
	if len(note) == 0 {
		lines = append(lines, "│")
	} else {
		for _, line := range strings.Split(string(note), "\n") {
			lines = append(lines, safeTerminalText(line))
		}
	}
	length := len(dialog.note)
	lines = append(lines, fmt.Sprintf("字符：%d / 4000", length))
	if length > 4000 {
		lines = append(lines, styleAlert.Render("超过协议上限；未截断，需编辑至 4000 字符以内"))
	}
	lines = append(lines, "发送后可确认答案送达；应用状态仍须由原执行绑定证明。")
	if dialog.error != "" {
		lines = append(lines, styleAlert.Render(safeTerminalText(dialog.error)))
	}
	if dialog.scroll < 0 {
		dialog.scroll = 0
	}
	wrapped := wrapLines(lines, m.width-2)
	if height < 1 {
		height = 1
	}
	visible := fitLines(scrollLines(wrapped, dialog.scroll, height), m.width, height)
	return strings.Join(visible, "\n")
}

func (m Model) answerRunTask(state projectState, dialog *decisionAnswerDialog) string {
	if dialog == nil {
		return ""
	}
	if runtimeState := state.snapshot.Connection.Snapshot.Runtime; runtimeState != nil {
		for _, run := range runtimeState.Runs {
			if run != nil && run.RunID == dialog.runID {
				return run.TaskID
			}
		}
	}
	return ""
}

func renderFooter(m Model, state projectState) string {
	if m.profileChoice != nil {
		return "↑↓ 选择预设方案 · Enter 确认并开始 · Esc 取消"
	}
	if m.help {
		return "? / Esc 关闭帮助"
	}
	if m.confirm != nil {
		return "↑↓/PgUp/PgDn 阅读完整确认内容 · Enter/y 确认 · Esc/n 取消"
	}
	if m.answer != nil {
		return "↑↓ 选择声明选项/移动光标 · Tab 切换选项与说明 · Enter 换到说明/插入换行 · F2 明确提交 · PgUp/PgDn 浏览完整问题 · Esc 丢弃草稿"
	}
	if m.detailExpanded {
		return "↑↓/PgUp/PgDn 阅读完整详情 · d / Enter / Esc 返回 · q 退出"
	}
	if state.panel == 6 {
		return "r 重新核验 · Enter/ Esc 返回"
	}
	parts := []string{"←→ 项目", "Tab 列表/动态", "1–7 筛选", "↑↓ 选择/滚动", "d 详情"}
	row := m.selectedRow(m.selected)
	if row != nil && row.kind == rowReady && candidateActionReason(state, m.clock.Now()) == "" {
		parts = append(parts, "s 选方案")
	}
	if row != nil && row.run != nil && row.run.Decision != nil && row.run.Decision.Status == "pending" && strings.HasPrefix(decisionAnswerAvailability(state, row), "提交后") {
		parts = append(parts, "a 回答")
	}
	connection := state.snapshot.Connection
	if rt := connection.Snapshot.Runtime; rt != nil && connection.State == productapi.ConnectionConnected && connection.CanControl {
		if hasCapability(rt, "set_dispatch_mode") {
			parts = append(parts, "m 模式")
		}
	}
	if connection.CanStop {
		parts = append(parts, "x 停服")
	}
	parts = append(parts, "? 帮助", "q 退出")
	return strings.Join(parts, " · ")
}

func renderHelp(state projectState) []string {
	lines := []string{"快捷键", "", "←/→  切换项目", "Tab  在任务列表与任务动态间切换", "↑/↓  选择任务或滚动动态", "d/Enter  打开完整只读详情；Esc 返回", "q    退出控制台；服务继续运行"}
	if state.ref.Presentation.NativeWorkflow {
		return append(lines,
			"", "任务与服务", "1 全部 · 2 Ready · 3 进行中", "4 待决策 · 5 待合并", "6 需处理 · 7 历史", "s    选择已配置方案并开始 Ready 任务", "m    切换手动/自动派发", "x    停止服务（需确认）",
			"", "运行与会话", "A/c/f  活动 / 会话 / 最终回复", "a    回答当前待决策问题", "v/u    决策历史 / 执行核验", ", / .  选择轮次", "e / =  折叠 / 保持展开", "y / n  更多会话 / 下一条最终回复")
	}
	return append(lines,
		"", "任务与服务", "1 全部 · 2 Ready · 3 进行中", "4 待决策 · 5 待合并", "6 需处理 · 7 历史", "s    选择已配置方案并开始 Ready 任务", "m    切换手动/自动派发", "x    停止服务（需确认）",
		"", "运行与会话", "A/c/f  活动 / 会话 / 最终回复", "a    回答当前待决策问题", "v/u    决策历史 / 恢复核验", "[ / ]  切换 attempt", ", / .  选择轮次", "e / =  折叠 / 保持展开", "y / n  更多会话 / 下一条最终回复")
}

func joinLines(left, right []string, leftWidth, height int) string {
	var lines []string
	for i := 0; i < height; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		padding := max(0, leftWidth-lipgloss.Width(l))
		lines = append(lines, l+strings.Repeat(" ", padding)+"│"+r)
	}
	return strings.Join(lines, "\n")
}

func consoleContentHeight(m Model) int {
	height := m.height - 3
	if m.selected >= 0 && m.selected < len(m.projects) {
		height -= len(wrapLines(strings.Split(renderFooter(m, m.projects[m.selected]), "\n"), m.width))
	}
	return max(3, height)
}

func compactLayout(m Model) bool {
	return m.width < 100 || consoleContentHeight(m) < 17
}

func listPanelWidth(m Model) int {
	left := m.width * 55 / 100
	if left < 55 {
		left = 55
	}
	if left > 90 {
		left = 90
	}
	return left
}

func detailPanelHeight(m Model) int {
	content := consoleContentHeight(m)
	return min(max(7, content*45/100), content-9)
}

func activityPanelWidth(m Model) int {
	if compactLayout(m) {
		return m.width
	}
	return m.width - listPanelWidth(m) - 1
}

func activityPanelHeight(m Model) int {
	if compactLayout(m) {
		return max(1, consoleContentHeight(m)-1)
	}
	return max(1, consoleContentHeight(m)-detailPanelHeight(m)-2)
}

func allActivityLines(state projectState, row *consoleRow, width int) []string {
	state.view.Follow = true
	state.logScroll = 0
	return renderActivity(state, row, width, 1<<20)
}

func activityEventLineCount(event contracts.ActivityEvent, width int) int {
	if width < 1 {
		width = 1
	}
	event.Title = safeTerminalText(event.Title)
	event.Detail = safeTerminalText(event.Detail)
	event.Status = safeTerminalText(event.Status)
	return max(1, len(strings.Split(activityview.RenderActivity(event, activityview.RenderOptions{MaxColumns: width}), "\n")))
}

func serviceWindowInstruction(ref ProjectRef) string { return ref.Presentation.ServiceInstruction }

func ptrTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "—"
	}
	return value.Local().Format(timeLayout)
}

func sampleTime(snapshot ProjectSnapshot) string {
	if snapshot.Observation.SampledAt.IsZero() {
		return "未知"
	}
	suffix := ""
	if snapshot.Observation.State == productapi.SnapshotStale || snapshot.Observation.State == productapi.SnapshotReadFailed || snapshot.Observation.State == productapi.SnapshotUnknownVersion {
		suffix = " · 缓存/未验证"
	}
	return snapshot.Observation.SampledAt.Local().Format(timeLayout) + suffix
}

func executionVerifiedAt(state projectState, run *contracts.Run) string {
	if run == nil || state.snapshot.Domain == nil || run.ExecutionID == "" {
		return "未知"
	}
	for _, execution := range state.snapshot.Domain.Executions {
		if execution.ExecutionID == run.ExecutionID && !execution.Workspace.VerifiedAt.IsZero() {
			return execution.Workspace.VerifiedAt.Local().Format(timeLayout)
		}
	}
	return "未知"
}

func selectedAttempt(state projectState, run *contracts.Run) string {
	if state.attemptID != "" {
		return state.attemptID
	}
	if run == nil {
		return ""
	}
	return firstText(run.AttemptID, run.RunID)
}
