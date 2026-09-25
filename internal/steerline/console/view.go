package console

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/erranliu/SteerLine/internal/steerline"
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
		return header + "\n\n" + renderEmptyProjects() + "\n\nq 退出"
	}
	if m.selected < 0 || m.selected >= len(m.projects) {
		return header + "\n" + renderEmptyProjects()
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
	if m.confirm != nil {
		body = renderStopConfirmation(m, *state)
	} else if m.width < 100 {
		panel := renderNarrowPanel(m, *state, contentHeight)
		body = strings.Join(fitLines(panel, m.width, contentHeight), "\n")
	} else {
		leftWidth := m.width * 38 / 100
		if leftWidth < 34 {
			leftWidth = 34
		}
		if leftWidth > 60 {
			leftWidth = 60
		}
		rightWidth := m.width - leftWidth - 1
		left := fitLines(renderRows(m, *state, leftWidth, contentHeight), leftWidth, contentHeight)
		right := fitLines(renderPanel(m, *state, rightWidth, contentHeight), rightWidth, contentHeight)
		body = joinLines(left, right, contentHeight)
	}
	lines := []string{header, projects, status, body}
	lines = append(lines, footerLines...)
	return strings.Join(lines, "\n")
}

func renderEmptyProjects() string {
	return "没有可用项目配置。\n配置目录：" + safeTerminalText(steerline.SteerlineEtcDir()) + "\n建立配置后再次运行 steerline console；可从 examples/config.json 复制并按项目填写。"
}

func renderProjectStrip(m Model) string {
	parts := make([]string, 0, len(m.projects))
	for i, state := range m.projects {
		name := safeTerminalText(state.ref.Name)
		status := projectShortSummary(state, m.clock.Now())
		if i == m.selected {
			parts = append(parts, styleSelected.Render(" "+name+" · "+status+" "))
		} else {
			parts = append(parts, " "+name+" · "+status+" ")
		}
	}
	line := strings.Join(parts, "│")
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
	mode, paused, slot := "未知", false, "未知"
	if runtimeState != nil {
		service = string(state.snapshot.Connection.State)
		mode = runtimeState.Control.DispatchMode
		paused = runtimeState.Control.DispatchPaused
		slot = "空闲"
		if runtimeState.SlotOccupied {
			slot = "占用 · " + stringValue(runtimeState.CurrentTask)
		}
	}
	if paused {
		mode += " · 已暂停"
	}
	ready, cached, running, decision, merge, attention := projectCounts(state, m.clock.Now())
	sampled := "最近更新：未知"
	if !state.snapshot.Observation.SampledAt.IsZero() {
		sampled = "采样于 " + sampleTime(state.snapshot)
	}
	parts := []string{"服务 " + service, "派发 " + mode, "槽 " + slot, fmt.Sprintf("Ready %d · 缓存 %d · 运行 %d · 决策 %d · 合并 %d · 需处理 %d", ready, cached, running, decision, merge, attention), sampled}
	if m.status != "" {
		parts = append(parts, safeTerminalText(m.status))
	}
	if diagnostic := state.snapshot.Connection.Diagnostic; diagnostic != "" {
		parts = append(parts, "连接诊断 "+safeTerminalText(diagnostic))
	}
	return truncate(strings.Join(parts, "  |  "), m.width)
}

func projectCounts(state projectState, now time.Time) (ready, cached, running, decision, merge, attention int) {
	if state.snapshot.Tasks != nil {
		if valid, _ := candidateFreshness(state.ref, state.snapshot, now); valid {
			ready = len(state.snapshot.Tasks.Tasks)
		} else {
			cached = len(state.snapshot.Tasks.Tasks)
		}
	}
	if rt := state.snapshot.Observation.Runtime; rt != nil {
		for _, run := range rt.Runs {
			if run == nil {
				continue
			}
			if run.ExecutionStatus == "running" {
				running++
			}
			if run.FinishTaskStatus == "waiting_decision" {
				decision++
			}
			if run.FinishTaskStatus == "waiting_pr_merge" {
				merge++
			}
			if runNeedsAttention(run) {
				attention++
			}
		}
	}
	return
}

func projectShortSummary(state projectState, now time.Time) string {
	status := projectStatus(state.ref, state.snapshot)
	ready, cached, running, decision, merge, attention := projectCounts(state, now)
	current := "空闲"
	if rt := state.snapshot.Observation.Runtime; rt != nil && rt.CurrentTask != nil && *rt.CurrentTask != "" {
		current = "执行 " + safeTerminalText(*rt.CurrentTask)
	}
	return fmt.Sprintf("%s · %s · run %d · R%d C%d D%d M%d !%d", status, current, running, ready, cached, decision, merge, attention)
}

func renderRows(m Model, state projectState, width, height int) []string {
	rows := m.rows(m.selected)
	out := []string{styleLabel.Render(fmt.Sprintf("任务与运行 · %d 项 · 筛选 %s", len(rows), filterName(state.filter)))}
	if state.ref.Error != "" {
		out = append(out, styleAlert.Render("配置错误："+safeTerminalText(state.ref.Error)))
		return out
	}
	if state.ref.Conflict != "" {
		out = append(out, styleAlert.Render(safeTerminalText(state.ref.Conflict)))
	}
	if state.snapshot.NativeTasks != nil && state.snapshot.NativeTasks.HasMore {
		out = append(out, "原生任务列表达到 500 项上限；未加载项不属于当前视图。")
	}
	if state.snapshot.NativeTaskError != "" {
		out = append(out, "原生任务查询："+safeTerminalText(state.snapshot.NativeTaskError))
	}
	if len(rows) == 0 {
		snapshot := state.snapshot
		switch {
		case !state.snapshotLoaded:
			out = append(out, "正在读取项目快照…")
		case snapshot.Connection.State == steerline.ConnectionOffline || snapshot.Connection.State == steerline.ConnectionNoRecord:
			out = append(out, "服务离线；已有运行和日志仍可查看。")
			out = append(out, "服务入口："+safeTerminalText(serviceWindowInstruction(state.ref)))
			out = append(out, "配置："+safeTerminalText(state.ref.ConfigPath))
		case snapshot.TaskError != "":
			out = append(out, "候选查询失败："+safeTerminalText(snapshot.TaskError))
		case snapshot.Observation.Runtime != nil && snapshot.Observation.Runtime.SlotOccupied:
			out = append(out, "没有可开始的任务；执行槽正被 "+safeTerminalText(stringValue(snapshot.Observation.Runtime.CurrentTask))+" 占用。")
		case snapshot.Tasks != nil && len(snapshot.Tasks.Tasks) == 0:
			out = append(out, "当前没有 Ready 任务。")
		default:
			out = append(out, "当前筛选没有匹配项。")
		}
		if snapshot.Connection.Diagnostic != "" {
			out = append(out, "连接诊断："+safeTerminalText(snapshot.Connection.Diagnostic))
		}
		return out
	}
	if len(out) >= height {
		out = out[:max(0, height-1)]
	}
	visible := contentWindow(rows, state.selectedKey, max(1, height-len(out)))
	for _, row := range visible {
		prefix := "RUN "
		stateText := ""
		switch row.kind {
		case rowReady:
			prefix = "READY "
			valid, reason := candidateFreshness(state.ref, state.snapshot, m.clock.Now())
			if valid {
				stateText = "Ready"
			} else {
				stateText = "缓存 · 不可操作"
				if reason != "" {
					stateText += " · " + reason
				}
			}
		case rowNative:
			prefix = "TASK "
			stateText = row.native.Status
			if row.native.DependencyReady {
				stateText += " · 依赖就绪"
			} else if len(row.native.UnmetPredecessors) > 0 {
				stateText += " · 依赖阻塞"
			}
		default:
			stateText = runStatus(row.run)
		}
		visibleID := safeTerminalInlineText(row.taskID)
		line := prefix + visibleID + "  " + truncate(safeTerminalInlineText(row.title), max(8, width-lipgloss.Width(prefix)-lipgloss.Width(visibleID)-4))
		line += "  " + safeTerminalInlineText(stateText)
		if row.key == state.selectedKey {
			line = styleSelected.Render(line)
		} else if row.kind == rowReady && strings.HasPrefix(stateText, "缓存") {
			line = styleDim.Render(line)
		}
		out = append(out, truncate(line, width))
	}
	return out
}

func contentWindow(rows []consoleRow, selected string, size int) []consoleRow {
	if size < 1 {
		size = 1
	}
	index := 0
	for i, row := range rows {
		if row.key == selected {
			index = i
			break
		}
	}
	start := index - size/2
	if start < 0 {
		start = 0
	}
	if start+size > len(rows) {
		start = len(rows) - size
	}
	if start < 0 {
		start = 0
	}
	end := start + size
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}

func runStatus(run *steerline.Run) string {
	if run == nil {
		return "unknown"
	}
	if run.FinishTaskStatus == "waiting_decision" {
		return "待决策"
	}
	if run.ExecutionStatus == "unknown" || run.ExecutionStatus == "failed" || run.ExecutionStatus == "blocked" || run.FinishTaskStatus == "blocked" {
		return "需处理 · " + firstText(run.ExecutionStatus, run.FinishTaskStatus)
	}
	if run.Error != nil && strings.TrimSpace(*run.Error) != "" {
		return "需处理 · error"
	}
	if run.PR.QueryError != nil && strings.TrimSpace(*run.PR.QueryError) != "" {
		return "需处理 · PR query error"
	}
	switch run.FinishTaskStatus {
	case "waiting_pr_merge":
		return "待合并"
	case "completed":
		return "completed"
	}
	if run.ExecutionStatus == "running" {
		return "运行中"
	}
	if run.FinishTaskStatus != "" {
		return run.FinishTaskStatus
	}
	return firstText(run.ExecutionStatus, "unknown")
}

func runNeedsAttention(run *steerline.Run) bool {
	if run == nil {
		return false
	}
	return run.FinishTaskStatus == "waiting_decision" || run.ExecutionStatus == "unknown" || run.ExecutionStatus == "failed" || run.ExecutionStatus == "blocked" || run.FinishTaskStatus == "blocked" || (run.Error != nil && strings.TrimSpace(*run.Error) != "") || (run.PR.QueryError != nil && strings.TrimSpace(*run.PR.QueryError) != "")
}

func renderPanel(m Model, state projectState, width, height int) []string {
	row := m.selectedRow(m.selected)
	switch state.panel {
	case 1:
		return renderActivity(state, row, width, height)
	case 2:
		return renderSession(state, row, width, height)
	case 3:
		return renderFinal(state, row, width, height)
	default:
		return renderDetail(m, state, row, width, height)
	}
}

func renderNarrowPanel(m Model, state projectState, height int) []string {
	if state.panel == 4 {
		return renderRows(m, state, m.width, height)
	}
	lines := []string{"窄屏面板 · " + panelName(state.panel) + "  [t]列表 [d]详情 [a]活动 [c]会话 [f]最终回复"}
	return append(lines, renderPanel(m, state, m.width, height-1)...)
}

func renderDetail(m Model, state projectState, row *consoleRow, width, height int) []string {
	lines := []string{styleLabel.Render("详情")}
	if row == nil {
		lines = append(lines, renderEmptySelection(state)...)
		return scrollLines(wrapLines(lines, width), state.detailScroll, height)
	}
	if row.native != nil {
		lines = append(lines, renderNativeTaskDetail(state, *row.native)...)
		if row.kind == rowReady {
			valid, reason := candidateFreshness(state.ref, state.snapshot, m.clock.Now())
			if valid {
				lines = append(lines, "Ready 候选：当前缓存与服务身份匹配")
			} else {
				lines = append(lines, "Ready 候选：缓存 · 禁止开始")
				if reason != "" {
					lines = append(lines, "不可操作原因："+safeTerminalText(reason))
				}
			}
			if state.snapshot.Tasks != nil {
				lines = append(lines, "候选查询："+state.snapshot.Tasks.QueriedAt.Local().Format(timeLayout))
				if state.snapshot.Tasks.LastSuccessAt != nil {
					lines = append(lines, "最后成功查询："+state.snapshot.Tasks.LastSuccessAt.Local().Format(timeLayout))
				}
			}
			lines = append(lines, "依赖就绪只描述任务关系，不授予启动许可。")
			lines = append(lines, renderControlAvailability(state, row, m.clock.Now())...)
			lines = append(lines, renderActionLines(state)...)
		}
		return scrollLines(wrapLines(lines, width), state.detailScroll, height)
	}
	if row.kind == rowReady {
		valid, reason := candidateFreshness(state.ref, state.snapshot, m.clock.Now())
		stateText := "Ready 候选"
		if !valid {
			stateText = "候选缓存 · 禁止开始"
		}
		lines = append(lines, "Task："+safeTerminalInlineText(row.taskID), "标题："+safeTerminalInlineText(row.title), "提供方状态："+stateText)
		if row.candidate.Summary != "" {
			lines = append(lines, "摘要："+safeTerminalText(row.candidate.Summary))
		}
		if reason != "" {
			lines = append(lines, "不可操作原因："+safeTerminalText(reason))
		}
		if state.snapshot.Tasks != nil {
			lines = append(lines, "候选查询："+state.snapshot.Tasks.QueriedAt.Local().Format(timeLayout))
			if state.snapshot.Tasks.LastSuccessAt != nil {
				lines = append(lines, "最后成功查询："+state.snapshot.Tasks.LastSuccessAt.Local().Format(timeLayout))
			}
		}
		lines = append(lines, renderControlAvailability(state, row, m.clock.Now())...)
		lines = append(lines, renderActionLines(state)...)
		return scrollLines(wrapLines(lines, width), state.detailScroll, height)
	}
	run := row.run
	if state.snapshot.Connection.State == steerline.ConnectionOffline || state.snapshot.Connection.State == steerline.ConnectionNoRecord {
		lines = append(lines, "服务入口："+safeTerminalText(serviceWindowInstruction(state.ref)), "配置："+safeTerminalText(state.ref.ConfigPath))
	}
	lines = append(lines,
		"Task："+safeTerminalInlineText(run.TaskID)+" · "+safeTerminalInlineText(run.Title),
		"run_id："+safeTerminalText(run.RunID),
		"execution："+safeTerminalText(firstText(run.ExecutionStatus, "unknown")),
		"finish："+safeTerminalText(firstText(run.FinishTaskStatus, "unknown")),
		"任务状态："+safeTerminalText(firstText(run.TaskStatus, run.BeadsStatus, "unknown")),
		"整体："+runStatus(run),
		"采样时间："+sampleTime(state.snapshot),
		"最后核验时间："+executionVerifiedAt(state, run),
		"execution_id："+safeTerminalText(displayDash(run.ExecutionID)),
		"attempt_id："+safeTerminalText(displayDash(selectedAttempt(state, run))),
		"后端/协议/目标："+safeTerminalText(strings.Join([]string{run.Backend, run.Protocol, run.Target.ID}, " / ")),
		"PR："+safeTerminalText(firstText(run.PR.State, "unknown")),
		"PR queried_at："+safeTerminalText(ptrTime(run.PR.QueriedAt)),
		"PR query_error："+safeTerminalText(ptrString(run.PR.QueryError)),
		"PR URL："+safeTerminalText(ptrString(run.PR.URL)),
		"工作目录："+safeTerminalText(ptrString(run.WorktreePath)),
		"分支："+safeTerminalText(ptrString(run.WorktreeBranch)),
		"错误："+safeTerminalText(ptrString(run.Error)),
		"摘要："+safeTerminalText(ptrString(run.Summary)),
	)
	if runtimeState := state.snapshot.Observation.Runtime; runtimeState != nil && runtimeState.ProjectionError != "" {
		lines = append(lines, "runtime 缓存来源："+safeTerminalText(runtimeState.ProjectionError))
	}
	if state.snapshot.DomainError != "" {
		lines = append(lines, "execution/attempt 查询："+safeTerminalText(state.snapshot.DomainError))
	}
	if runtimeState := state.snapshot.Observation.Runtime; runtimeState != nil {
		paused := "否"
		if runtimeState.Control.DispatchPaused {
			paused = "是"
		}
		lines = append(lines, "派发模式："+safeTerminalText(runtimeState.Control.DispatchMode)+" · 已暂停："+paused, "p 只暂停新派发，当前 worker 会继续运行。")
	}
	lines = append(lines, renderControlAvailability(state, row, m.clock.Now())...)
	if run.Decision != nil && run.Decision.Status == "pending" {
		lines = append(lines, "", "待决策 · 只读展示", "decision_id："+safeTerminalText(run.Decision.ID), "attempt_id："+safeTerminalText(run.Decision.AttemptID), "问题："+safeTerminalText(run.Decision.Request.Question), "上下文："+safeTerminalText(run.Decision.Request.Context))
		for i, option := range run.Decision.Request.Options {
			lines = append(lines, fmt.Sprintf("选项 %d · %s：%s %s", i+1, safeTerminalText(option.ID), safeTerminalText(option.Label), safeTerminalText(option.Detail)))
		}
		lines = append(lines, "本阶段不会从控制台提交答案。")
	} else if run.FinishTaskStatus == "waiting_decision" {
		lines = append(lines, "待决策状态存在，但当前 decision_id/问题记录缺失或尚未同步；答案通道未启用。")
	}
	lines = append(lines, renderActionLines(state)...)
	if state.ref.Config.TaskAuthority == "native" {
		if task := nativeDetailForTask(state, firstText(run.DomainTaskID, run.TaskID)); task != nil {
			lines = append(lines, "", "任务关系 · 中心只读快照")
			lines = append(lines, renderNativeTaskDetail(state, *task)...)
		}
	}
	return scrollLines(wrapLines(lines, width), state.detailScroll, height)
}

func nativeDetailForTask(state projectState, taskID string) *steerline.NativeTaskDetail {
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

func renderNativeTaskDetail(state projectState, task steerline.NativeTaskDetail) []string {
	lines := []string{
		"Task：" + safeTerminalInlineText(task.TaskID),
		"标题：" + safeTerminalInlineText(task.Title),
		"状态：" + safeTerminalText(task.Status) + " · version " + fmt.Sprint(task.Version),
		"依赖就绪：" + fmt.Sprint(task.DependencyReady),
	}
	if state.snapshot.NativeTasks != nil {
		sampled := "未知"
		if !state.snapshot.NativeTasks.SampledAt.IsZero() {
			sampled = state.snapshot.NativeTasks.SampledAt.Local().Format(timeLayout)
		}
		if state.snapshot.Connection.State == steerline.ConnectionOffline || state.snapshot.Connection.State == steerline.ConnectionNoRecord {
			sampled += " · 离线只读快照"
		}
		lines = append(lines, "关系采样时间："+sampled)
	}
	if state.snapshot.NativeTaskError != "" {
		lines = append(lines, "查询状态："+safeTerminalText(state.snapshot.NativeTaskError))
	}
	lines = append(lines, "", "前置")
	if len(task.Predecessors) == 0 {
		lines = append(lines, "无")
	}
	for _, predecessor := range task.Predecessors {
		delivery := "未交付"
		if predecessor.Delivered {
			delivery = "已交付"
		}
		line := "task " + predecessor.TaskID + " · " + firstText(predecessor.Status, "unknown") + " · " + delivery
		if predecessor.Blocker != "" {
			line += " · 阻塞：" + predecessor.Blocker
		}
		lines = append(lines, safeTerminalInlineText(line))
	}
	lines = append(lines, "后继")
	if len(task.Successors) == 0 {
		lines = append(lines, "无")
	} else {
		for _, successor := range task.Successors {
			lines = append(lines, "task "+safeTerminalInlineText(successor))
		}
	}
	lines = append(lines, "阻塞原因")
	if len(task.UnmetPredecessors) == 0 {
		lines = append(lines, "无未满足前置")
	} else {
		for _, blocker := range task.UnmetPredecessors {
			lines = append(lines, safeTerminalInlineText(fmt.Sprintf("task %s · %s · %s", blocker.TaskID, firstText(blocker.Status, "unknown"), firstText(blocker.Blocker, "predecessor_not_delivered"))))
		}
	}
	if len(task.StartupBlockers) > 0 {
		lines = append(lines, "启动阻塞："+safeTerminalText(strings.Join(task.StartupBlockers, ", ")))
	}
	lines = append(lines, "", "规格与交付定位")
	lines = append(lines, "Spec："+safeTerminalInlineText(fileReferenceText(task.Contract.SpecRef)))
	lines = append(lines, "Plan："+safeTerminalInlineText(fileReferenceText(task.Contract.PlanRef)))
	for _, ref := range task.Contract.AcceptanceRefs {
		lines = append(lines, "验收："+safeTerminalInlineText(fileReferenceText(ref)))
	}
	for _, ref := range task.Contract.CommonRefs {
		lines = append(lines, "共同约束："+safeTerminalInlineText(fileReferenceText(ref)))
	}
	if task.Delivery == nil {
		lines = append(lines, "交付：无")
	} else {
		lines = append(lines,
			"PR："+safeTerminalInlineText(task.Delivery.PRURL),
			"合并提交："+safeTerminalText(task.Delivery.MergeCommit),
			"合并时间："+safeTerminalText(task.Delivery.MergedAt.Local().Format(timeLayout)),
		)
	}
	return lines
}

func fileReferenceText(ref steerline.FileReference) string {
	return ref.Path + " @ " + ref.Commit + " · sha256 " + ref.SHA256
}

func renderActionLines(state projectState) []string {
	actions := make([]*actionState, 0, len(state.actions))
	for _, action := range state.actions {
		actions = append(actions, action)
	}
	sort.SliceStable(actions, func(i, j int) bool { return actions[i].UpdatedAt.After(actions[j].UpdatedAt) })
	if len(actions) > 5 {
		actions = actions[:5]
	}
	if len(actions) == 0 {
		return nil
	}
	lines := []string{"", "控制请求 · 按 i 查询最近 request_id"}
	for _, action := range actions {
		lines = append(lines, fmt.Sprintf("%s · task %s · request %s · %s", safeTerminalText(action.Status), safeTerminalText(displayDash(action.TaskID)), safeTerminalText(action.RequestID), safeTerminalText(action.Message)))
	}
	return lines
}

func renderControlAvailability(state projectState, row *consoleRow, now time.Time) []string {
	lines := []string{"", "控制操作"}
	connection := state.snapshot.Connection
	if state.ref.Error != "" {
		return append(lines, "控制不可用：配置错误 · "+safeTerminalText(state.ref.Error))
	}
	if state.ref.Conflict != "" {
		return append(lines, "控制不可用："+safeTerminalText(state.ref.Conflict))
	}
	runtimeState := connection.Snapshot.Runtime
	connected := connection.State == steerline.ConnectionConnected && connection.CanControl && runtimeState != nil
	if row != nil && row.kind == rowReady {
		reason := candidateActionReason(state.ref, state.snapshot, now)
		if reason == "" {
			lines = append(lines, "s 开始当前 Ready task "+safeTerminalText(row.taskID))
		} else {
			lines = append(lines, "s 开始不可用："+safeTerminalText(reason))
		}
	} else {
		lines = append(lines, "s 开始不可用：请先选择 Ready 候选")
	}
	if connected {
		if hasCapability(runtimeState, "set_dispatch_mode") {
			lines = append(lines, "m 派发模式："+safeTerminalText(runtimeState.Control.DispatchMode)+" ↔ 手动/自动")
		} else {
			lines = append(lines, "m 不可用：服务未声明 set_dispatch_mode")
		}
		pauseAction := "pause_dispatch"
		if runtimeState.Control.DispatchPaused {
			pauseAction = "resume_dispatch"
		}
		if hasCapability(runtimeState, pauseAction) {
			lines = append(lines, "p "+pauseAction)
		} else {
			lines = append(lines, "p 不可用：服务未声明 "+pauseAction)
		}
	} else {
		reason := firstText(connection.Diagnostic, string(connection.State), "服务未连接")
		lines = append(lines, "m/p 不可用："+safeTerminalText(reason))
	}
	if connection.CanStop && runtimeState != nil {
		lines = append(lines, "x 停止实例 "+safeTerminalText(runtimeState.Service.InstanceID)+"（需确认）")
	} else {
		lines = append(lines, "x 不可用："+safeTerminalText(firstText(connection.Diagnostic, "没有可定位且支持停止的服务实例")))
	}
	return lines
}

const timeLayout = "2006-01-02 15:04:05"

func renderEmptySelection(state projectState) []string {
	if state.snapshot.Connection.State == steerline.ConnectionOffline || state.snapshot.Connection.State == steerline.ConnectionNoRecord {
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
		if state.activityKey == "" {
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
			rendered := steerline.RenderActivity(event, steerline.RenderOptions{MaxColumns: width})
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

func renderSession(state projectState, row *consoleRow, width, height int) []string {
	lines := []string{styleLabel.Render("会话 · Turn / Session")}
	if row == nil || row.run == nil {
		return append(lines, "选择一个 run 后查看会话。")
	}
	lines = append(lines, "execution："+safeTerminalText(row.run.ExecutionID)+" · attempt："+safeTerminalText(selectedAttempt(state, row.run)))
	if state.conversationError != "" {
		lines = append(lines, styleAlert.Render("会话读取错误："+safeTerminalText(state.conversationError)))
	}
	view, changed := BuildConversationView(state.conversationEvents, state.view.Groups)
	_ = changed
	if len(state.conversationEvents) == 0 {
		if len(state.activityEvents) > 0 {
			lines = append(lines, "能力降级：执行过程 / 执行摘要 · 来源 activity journal；尚未验证精确 turn/final。")
		} else {
			lines = append(lines, "没有可读会话记录。查看 activity 页确认执行日志状态。")
		}
	}
	if len(view.Groups) > 0 {
		lines = append(lines, "来源：版本化会话事件 · [ ] 切 attempt · e 折叠/展开 · = 保持展开 · n 阅读最终回复")
		for _, group := range view.Groups {
			choice := state.view.Groups[group.ID]
			heading := groupHeading(group, choice)
			if !group.Precise {
				heading = "执行过程 · turn 边界未知 · " + heading
			}
			if group.ID == state.view.AnchorGroup {
				heading = "● " + heading
			}
			lines = append(lines, safeTerminalText(heading))
			if !Folded(group, choice) {
				for _, event := range group.Process {
					lines = append(lines, renderConversationEvent(event, width-2)...)
				}
			}
			for _, event := range group.Finals {
				lines = append(lines, renderFinalEventLine(state, event))
			}
			if group.EndStatus != "" {
				switch group.FinalState {
				case steerline.FinalPending:
					lines = append(lines, "最终回复待同步")
				case steerline.FinalAbsent:
					lines = append(lines, "本轮已结束，未识别到最终回复")
				case steerline.FinalUnknown:
					lines = append(lines, "最终回复状态未知")
				}
				reason := group.EndReason
				if reason == "" {
					reason = "来源未提供终态原因"
				}
				lines = append(lines, "终态原因："+safeTerminalText(reason))
			}
		}
	}
	for _, event := range view.Summaries {
		lines = append(lines, "执行摘要 · "+safeTerminalText(event.Status)+" · "+safeTerminalText(event.Source)+" · "+safeTerminalText(event.ItemID))
	}
	for _, warning := range view.Warnings {
		lines = append(lines, styleAlert.Render("轮次外提示 · "+safeTerminalText(firstText(warning.Diagnostic, warning.Status, string(warning.Type)))))
	}
	if row.run.Decision != nil && row.run.Decision.Status == "pending" {
		lines = append(lines, "业务待决策 · 折叠区外保留", "decision_id："+safeTerminalText(row.run.Decision.ID), "attempt_id："+safeTerminalText(row.run.Decision.AttemptID), "问题："+safeTerminalText(row.run.Decision.Request.Question), "上下文："+safeTerminalText(row.run.Decision.Request.Context))
		for i, option := range row.run.Decision.Request.Options {
			lines = append(lines, fmt.Sprintf("选项 %d · %s：%s %s", i+1, safeTerminalText(option.ID), safeTerminalText(option.Label), safeTerminalText(option.Detail)))
		}
		lines = append(lines, "本阶段只读展示，不提交答案。")
	}
	if state.conversationMore {
		lines = append(lines, "还有更多会话记录 · 按 y 加载下一页")
	}
	if state.capabilityError != "" {
		lines = append(lines, "能力报告读取失败："+safeTerminalText(state.capabilityError))
	}
	if len(state.capabilities) > 0 {
		for _, report := range state.capabilities {
			lines = append(lines, "能力："+safeTerminalText(string(report.Backend)+" / "+string(report.Protocol)+" / "+report.Agent)+" · 来源 "+safeTerminalText(report.Source+"/"+report.SourceIdentity)+" · 检查 "+report.CheckedAt.Local().Format(timeLayout))
			for _, capability := range report.Capabilities {
				if capability.Status != steerline.CapabilityVerified {
					lines = append(lines, "  "+safeTerminalText(capability.Name)+" = "+safeTerminalText(string(capability.Status))+" · "+safeTerminalText(capability.Evidence))
				}
			}
		}
		for _, event := range state.conversationEvents {
			if event.Backend == "" || event.Protocol == "" || event.TargetID == "" {
				continue
			}
			matched := false
			for _, report := range state.capabilities {
				if report.Backend == event.Backend && report.Protocol == event.Protocol && report.Target.ID == event.TargetID && report.ExecutionID == event.ExecutionID && report.AttemptID == event.AttemptID && report.Source == event.Source && report.SourceIdentity == event.SourceIdentity {
					matched = true
					break
				}
			}
			if !matched {
				lines = append(lines, "能力原因：来源 "+safeTerminalText(event.Source+"/"+event.SourceIdentity)+" 没有匹配的已验证报告；不推断 turn/final。")
			}
		}
	} else if len(state.conversationEvents) > 0 {
		lines = append(lines, "能力原因：没有匹配当前 execution/attempt 的已验证报告；不推断 turn 能力。")
	}
	return scrollLines(wrapLines(lines, width), state.detailScroll, height)
}

func allSessionLines(state projectState, row *consoleRow, width int) []string {
	state.detailScroll = 0
	return renderSession(state, row, width, 1<<20)
}

func captureSessionAnchor(state *projectState, row *consoleRow, width int) {
	if state == nil {
		return
	}
	lines := allSessionLines(*state, row, width)
	if len(lines) == 0 {
		return
	}
	top := state.detailScroll
	if top < 0 {
		top = 0
	}
	if top >= len(lines) {
		top = len(lines) - 1
	}
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	selected, headerLine := -1, -1
	for i, group := range view.Groups {
		needle := "[" + groupToken(group.ID) + "]"
		for lineIndex := 0; lineIndex <= top && lineIndex < len(lines); lineIndex++ {
			if strings.Contains(lines[lineIndex], needle) {
				selected, headerLine = i, lineIndex
			}
		}
	}
	if selected < 0 {
		return
	}
	group := view.Groups[selected]
	state.view.ReadGroup = group.ID
	state.view.ReadItem = ""
	state.view.ReadRevision = 0
	itemLine := -1
	events := append(append([]steerline.ConversationEvent(nil), group.Process...), group.Finals...)
	for _, event := range events {
		needle := "[" + itemToken(event) + "]"
		for lineIndex := headerLine; lineIndex <= top && lineIndex < len(lines); lineIndex++ {
			if strings.Contains(lines[lineIndex], needle) {
				itemLine = lineIndex
				state.view.ReadItem = sessionItemAnchorKey(event)
				if event.Body != nil {
					state.view.ReadRevision = event.Body.Revision
				}
			}
		}
	}
	if itemLine >= 0 {
		state.view.ReadLine = top - itemLine
	} else {
		state.view.ReadLine = top - headerLine
	}
}

func restoreSessionAnchor(state *projectState, row *consoleRow, width int) bool {
	if state == nil || state.view.ReadGroup == "" {
		return false
	}
	lines := allSessionLines(*state, row, width)
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	groupIndex := -1
	for i, group := range view.Groups {
		if group.ID == state.view.ReadGroup {
			groupIndex = i
			break
		}
	}
	if groupIndex < 0 {
		return false
	}
	group := view.Groups[groupIndex]
	headerLine := -1
	needle := "[" + groupToken(group.ID) + "]"
	for i, line := range lines {
		if strings.Contains(line, needle) {
			headerLine = i
			break
		}
	}
	if headerLine < 0 {
		return false
	}
	anchorLine := headerLine
	itemHidden := false
	if state.view.ReadItem != "" {
		foundItem := false
		itemVisible := false
		events := append(append([]steerline.ConversationEvent(nil), group.Process...), group.Finals...)
		for _, event := range events {
			if sessionItemAnchorKey(event) != state.view.ReadItem {
				continue
			}
			foundItem = true
			if event.Body != nil && state.view.ReadRevision != event.Body.Revision {
				itemHidden = true
				continue
			}
			itemNeedle := "[" + itemToken(event) + "]"
			for i := headerLine; i < len(lines); i++ {
				if strings.Contains(lines[i], itemNeedle) {
					anchorLine = i
					itemVisible = true
					break
				}
			}
			if itemVisible {
				break
			}
		}
		if !foundItem {
			return false
		}
		itemHidden = !itemVisible
	}
	if itemHidden {
		state.detailScroll = headerLine
		return true
	}
	state.detailScroll = anchorLine + state.view.ReadLine
	if state.detailScroll < 0 {
		state.detailScroll = 0
	}
	return false
}

func renderConversationEvent(event steerline.ConversationEvent, width int) []string {
	identity := firstText(event.ItemID, event.SourceEventID, string(event.Type))
	phase := string(event.Phase)
	status := event.Status
	line := fmt.Sprintf("  [%s] %s · %s · %s · %s", itemToken(event), safeTerminalText(phase), safeTerminalText(string(event.Type)), safeTerminalText(status), safeTerminalText(identity))
	if event.OccurredAt != nil {
		line = "  " + event.OccurredAt.Local().Format("15:04:05") + line
	}
	if event.Body != nil {
		line += " · 正文 " + safeTerminalText(string(event.Body.Completeness)) + fmt.Sprintf(" %d bytes · 按 o 阅读", event.Body.Bytes)
	}
	if len(line) > width {
		line = truncate(line, width)
	}
	return []string{line}
}

func renderFinalEventLine(state projectState, event steerline.ConversationEvent) string {
	key := conversationItemKey(event)
	line := "  [" + itemToken(event) + "] final_answer · " + safeTerminalText(firstText(event.ItemID, event.SourceEventID)) + " · 正文缺失"
	if event.Body != nil {
		line = fmt.Sprintf("  [%s] final_answer · %s · %s · %d bytes · 按 n 阅读", itemToken(event), safeTerminalText(firstText(event.ItemID, event.SourceEventID)), safeTerminalText(string(event.Body.Completeness)), event.Body.Bytes)
	}
	if state.bodyEvent == key && state.bodyText != "" {
		line += " · 已打开"
	}
	return line
}

func shortStableToken(prefix, value string) string {
	hash := sha256.Sum256([]byte(value))
	return prefix + hex.EncodeToString(hash[:4])
}

func groupToken(groupID string) string { return shortStableToken("g:", groupID) }
func itemToken(event steerline.ConversationEvent) string {
	return shortStableToken("i:", conversationItemKey(event))
}

func sessionItemAnchorKey(event steerline.ConversationEvent) string {
	if event.Body != nil {
		return "body:" + event.Body.ItemKey
	}
	if event.ItemKey != "" {
		return "item:" + event.ItemKey
	}
	if event.ItemID != "" && (event.ItemQuality == steerline.IdentityNative || event.ItemQuality == steerline.IdentityDerived) {
		return "item:" + event.ItemID
	}
	return conversationItemKey(event)
}

func renderFinal(state projectState, row *consoleRow, width, height int) []string {
	title := "最终回复 · 纯文本正文"
	if state.bodyKind == "process" {
		title = "会话消息正文 · 纯文本"
	}
	lines := []string{styleLabel.Render(title)}
	if row == nil || row.run == nil {
		return append(lines, "选择一个 run 后阅读正文。")
	}
	if state.bodyRef == nil {
		return append(lines, "按 n 选择并读取最终回复。过程消息不会转成 final。", "")
	}
	lines = append(lines, fmt.Sprintf("execution %s · attempt %s · body %s r%d · offset %d/%d", safeTerminalText(state.bodyRef.ExecutionID), safeTerminalText(state.bodyRef.AttemptID), safeTerminalText(state.bodyRef.ItemKey), state.bodyRef.Revision, state.bodyViewOffset, state.bodyRef.Bytes))
	if state.bodyError != "" {
		lines = append(lines, styleAlert.Render("正文读取错误："+safeTerminalText(state.bodyError)))
	}
	if state.bodyText != "" {
		lines = append(lines, strings.Split(safeTerminalText(state.bodyText), "\n")...)
	} else if state.bodyBusy {
		lines = append(lines, "正在读取正文…")
	}
	if state.bodyMore {
		lines = append(lines, "正文尚有后续 · > 下一页 · < 上一页")
	}
	if state.bodyRef.Completeness != steerline.BodyComplete {
		lines = append(lines, "正文完整性："+safeTerminalText(string(state.bodyRef.Completeness)))
	}
	return scrollLines(wrapLines(lines, width), state.detailScroll, height)
}

func renderStopConfirmation(m Model, state projectState) string {
	if m.confirm == nil {
		return ""
	}
	grace := safeTerminalText(state.ref.Config.ShutdownGrace)
	return strings.Join([]string{"停止服务确认", "", "目标实例：" + safeTerminalText(m.confirm.instance), "这会停止新派发，并等待当前 worker 最长 " + grace + " 后按服务策略结束；工作现场会保留。", "request_id：" + m.confirm.requestID, "", "Enter/y 确认停止 · Esc/n 取消"}, "\n")
}

func renderFooter(m Model, state projectState) string {
	if m.width < 100 {
		return "↑↓选择/滚动 · ←→项目 · t列表 · d详情 · a活动 · c会话 · f正文\n" + "s开始 · m模式 · p暂停/恢复 · x停服 · i查request · q退出\n" + ",/.轮次 · e折叠 · =保持展开 · o过程正文 · n最终回复 · y更多"
	}
	return "↑↓ 选择/滚动 · PgUp/PgDn 阅读 · ←→ 项目 · 1-7 筛选 · Enter 详情 · s 开始 · m 手动/自动 · p 暂停/恢复 · x 停止 · i 查询 request · d/a/c/f 面板 · ,/. 轮次 · e 折叠 · = 保持展开 · o 正文 · n final · q 退出"
}

func joinLines(left, right []string, height int) string {
	var lines []string
	for i := 0; i < height; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		lines = append(lines, l+" "+r)
	}
	return strings.Join(lines, "\n")
}

func fitLines(lines []string, width, height int) []string {
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		if i < len(lines) {
			out = append(out, truncate(lines[i], width))
		} else {
			out = append(out, "")
		}
	}
	return out
}

func scrollLines(lines []string, offset, height int) []string {
	if height < 1 {
		return nil
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(lines) {
		offset = max(0, len(lines)-height)
	}
	end := offset + height
	if end > len(lines) {
		end = len(lines)
	}
	return lines[offset:end]
}

func scrollLinesFromEnd(lines []string, offset, height int) []string {
	if height < 1 {
		return nil
	}
	if offset < 0 {
		offset = 0
	}
	end := len(lines) - offset
	if end < 0 {
		end = 0
	}
	start := end - height
	if start < 0 {
		start = 0
	}
	return lines[start:end]
}

func activityPanelWidth(m Model) int {
	if m.width < 100 {
		return m.width
	}
	left := m.width * 38 / 100
	if left < 34 {
		left = 34
	}
	if left > 60 {
		left = 60
	}
	return m.width - left - 1
}

func activityPanelHeight(m Model) int {
	height := m.height - 4
	if m.selected >= 0 && m.selected < len(m.projects) {
		footerLines := wrapLines(strings.Split(renderFooter(m, m.projects[m.selected]), "\n"), m.width)
		height = m.height - 3 - len(footerLines)
		if height < 3 {
			height = 3
		}
		if m.width < 100 {
			height--
		}
	}
	if height < 1 {
		height = 1
	}
	return height
}

func allActivityLines(state projectState, row *consoleRow, width int) []string {
	state.view.Follow = true
	state.logScroll = 0
	return renderActivity(state, row, width, 1<<20)
}

func activityEventLineCount(event steerline.ActivityEvent, width int) int {
	if width < 1 {
		width = 1
	}
	event.Title = safeTerminalText(event.Title)
	event.Detail = safeTerminalText(event.Detail)
	event.Status = safeTerminalText(event.Status)
	return max(1, len(strings.Split(steerline.RenderActivity(event, steerline.RenderOptions{MaxColumns: width}), "\n")))
}

func wrapLines(lines []string, width int) []string {
	if width < 1 {
		return lines
	}
	style := lipgloss.NewStyle().Width(width).MaxWidth(width)
	var wrapped []string
	for _, line := range lines {
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		wrapped = append(wrapped, strings.Split(style.Render(line), "\n")...)
	}
	return wrapped
}

func truncate(value string, width int) string {
	value = strings.ReplaceAll(value, "\r", "␍")
	if width < 1 {
		return ""
	}
	return ansi.Truncate(value, width, "…")
}

func safeTerminalText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r == '\n' || r == '\t':
			builder.WriteRune(r)
		case r == 0x1b:
			builder.WriteString("␛")
		case unicode.IsControl(r) || isBidiControl(r):
			builder.WriteString(fmt.Sprintf("\\u%04x", r))
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func safeTerminalInlineText(value string) string {
	return strings.NewReplacer("\n", "\\n", "\t", "\\t").Replace(safeTerminalText(value))
}

func isBidiControl(r rune) bool {
	return r == '\u061c' || r == '\u200e' || r == '\u200f' || r >= '\u202a' && r <= '\u202e' || r >= '\u2066' && r <= '\u2069'
}

func ptrString(value *string) string {
	if value == nil {
		return "—"
	}
	if *value == "" {
		return "—"
	}
	return *value
}

func serviceWindowInstruction(ref ProjectRef) string {
	if runtime.GOOS != "windows" {
		return "steerline start-service -config " + ref.ConfigPath
	}
	launcher := filepath.Join(steerline.SteerlineHome(), "service-window.bat")
	binary := filepath.Join(steerline.SteerlineBinDir(), "steerline.exe")
	if _, err := os.Stat(binary); err != nil {
		return fmt.Sprintf("服务窗口不可用：缺少 %s；运行 steerline install", binary)
	}
	if _, err := os.Stat(launcher); err != nil {
		return fmt.Sprintf("服务窗口不可用：缺少 %s；运行 steerline launchers", launcher)
	}
	return launcher
}
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
	if snapshot.Observation.State == steerline.SnapshotStale || snapshot.Observation.State == steerline.SnapshotReadFailed || snapshot.Observation.State == steerline.SnapshotUnknownVersion {
		suffix = " · 缓存/未验证"
	}
	return snapshot.Observation.SampledAt.Local().Format(timeLayout) + suffix
}
func executionVerifiedAt(state projectState, run *steerline.Run) string {
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

func selectedAttempt(state projectState, run *steerline.Run) string {
	if state.attemptID != "" {
		return state.attemptID
	}
	if run == nil {
		return ""
	}
	return firstText(run.AttemptID, run.RunID)
}

func conversationItemKey(event steerline.ConversationEvent) string {
	if event.Body != nil {
		return fmt.Sprintf("%s\x00%s\x00%s\x00%d", event.ExecutionID, event.AttemptID, event.Body.ItemKey, event.Body.Revision)
	}
	return strings.Join([]string{event.ExecutionID, event.AttemptID, event.Source, event.SourceSegment, event.SessionID, event.TurnID, event.ItemID, event.SourceEventID}, "\x00")
}
