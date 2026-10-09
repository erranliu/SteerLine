package console

import (
	"fmt"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleHead     = lipgloss.NewStyle().Bold(true)
	styleDim      = lipgloss.NewStyle().Faint(true)
	styleSelected = lipgloss.NewStyle().Bold(true).Reverse(true)
	styleAlert    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "203"})
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
	if headline := candidateHeadline(*state, m.selectedRow(m.selected)); headline != "" {
		header = styleHead.Render(truncate(headline, m.width))
	}
	projects := renderProjectStrip(m)
	status := renderStatusLine(m, *state)
	footerLines := workspaceFooter(m, *state)
	contentHeight := m.height - 3 - len(footerLines)
	if contentHeight < 3 {
		contentHeight = 3
	}
	var body string
	if m.operationDetails {
		body = strings.Join(fitLines(scrollLines(wrapLines(operationDetailLines(m, *state), m.width), m.expandedDetailScroll, contentHeight), m.width, contentHeight), "\n")
	} else if m.help {
		body = strings.Join(fitLines(renderHelp(*state), m.width, contentHeight), "\n")
	} else if m.answer != nil && m.answer.details {
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
		rightWidth := detailPanelWidth(m)
		left := paneFrame(renderRows(m, *state, leftWidth-2, contentHeight-2), leftWidth, contentHeight, state.focus == 0)
		right := renderRightPanes(m, *state, rightWidth, contentHeight)

		body = joinLines(left, right, leftWidth, contentHeight)
	}
	lines := []string{header, projects, status, body}
	lines = append(lines, footerLines...)
	return strings.Join(fitLines(strings.Split(strings.Join(lines, "\n"), "\n"), m.width, m.height), "\n")
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
	if m.prefsErr != "" {
		return truncate("只读："+safeTerminalText(m.prefsErr), m.width)
	}
	runtimeState := state.snapshot.Connection.Snapshot.Page
	if runtimeState == nil {
		runtimeState = state.snapshot.Observation.Page
	}
	service := projectStatus(state.ref, state.snapshot)
	mode, paused, slot := "未知", false, ""
	if runtimeState != nil {
		mode = runtimeState.Control.DispatchMode
		paused = runtimeState.Control.DispatchPaused
		if runtimeState.SlotOccupied {
			active := 0
			for _, run := range runtimeState.Runs {
				if candidateBusinessOpen(run) && (run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" || run.ExecutionStatus == "unknown" || waitingForDecision(run)) {
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
	if candidateHeadline(state, m.selectedRow(m.selected)) != "" {
		parts := []string{"服务 " + service, "派发 " + mode}
		if m.status != "" {
			parts = append(parts, safeTerminalInlineText(m.status))
		}
		if executionConfigMismatch(state.ref, state.snapshot) {
			parts = append(parts, "执行配置待核对")
		}
		if diagnostic := state.snapshot.Connection.Diagnostic; diagnostic != "" {
			parts = append(parts, safeTerminalText(diagnostic))
		}
		return truncate(strings.Join(parts, " · "), m.width)
	}
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
	if state.snapshot.Connection.State != api.ConnectionConnected && !state.snapshot.Observation.SampledAt.IsZero() {
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

func renderNarrowPanel(m Model, state projectState, height int) []string {
	if state.focus == 0 {
		return paneFrame(renderRows(m, state, m.width-2, height-2), m.width, height, true)
	}
	return renderRightPanes(m, state, m.width, height)
}

func renderDetail(m Model, state projectState, row *consoleRow, width, height int) []string {
	if m.detailExpanded {
		switch m.detailSection {
		case "final":
			return scrollLines(wrapLines(finalReplyLines(state), width), state.detailScroll, height)
		case "history":
			return renderDecisionHistory(state, row, width, height, m.clock.Now())
		case "inspection":
			return renderRecoveryInspection(m, state, width, height)
		}
	}
	return scrollLines(wrapLines(candidateDetailLines(m, state, row), width), state.detailScroll, height)
}

func candidateDetailLines(m Model, state projectState, row *consoleRow) []string {
	lines := []string{"候选详情"}
	if row == nil {
		return append(lines, renderEmptySelection(state)...)
	}
	lines = append(lines, runtimeIdentityLines(state.ref)...)
	lines = append(lines, "Task："+safeTerminalInlineText(row.taskID)+" · "+safeTerminalInlineText(row.title))
	if row.native != nil {
		lines = append(lines, "任务关系")
		lines = append(lines, renderNativeTaskDetail(state, *row.native)...)
	}
	if row.run == nil {
		if state.candidates[row.taskID] != "" {
			lines = append(lines, "原所选候选暂不在快照中；未改选其他候选。F6/F7 显式选择。")
		}
		lines = append(lines, renderControlAvailability(state, row, m.clock.Now())...)
		return append(lines, renderActionLines(state, row.taskID)...)
	}
	run := row.run
	lines = append(lines, renderCandidateOverview(state, row)...)
	lines = append(lines, renderRunExecutionProfileSummary(state, run)...)
	if m.detailExpanded {
		lines = append(lines, renderCandidates(m, state, row)...)
	}
	lines = append(lines, "任务状态："+safeTerminalText(rowTaskStatus(*row)), "执行状态："+safeTerminalText(displayExecutionStatus(run)), "需要你操作："+taskNextAction(run))
	lines = append(lines, renderActionLines(state, run.TaskID)...)
	if m.detailExpanded {
		lines = append(lines, renderFixedRunExecutionProfile(state, run)...)
	}
	lines = append(lines, renderControlAvailability(state, row, m.clock.Now())...)
	if run.PR.URL != nil {
		lines = append(lines, "PR 地址："+prLink(*run.PR.URL, *run.PR.URL))
	}
	for _, url := range run.PRHistory {
		if run.PR.URL == nil || url != *run.PR.URL {
			lines = append(lines, "历史 PR 地址："+prLink(url, url))
		}
	}
	lines = append(lines, "PR 状态："+safeTerminalText(firstText(run.PR.State, "未核验")))
	if run.PR.QueryError != nil {
		lines = append(lines, "PR 查询失败："+safeTerminalText(*run.PR.QueryError))
	}
	if run.EnvironmentBlocker != nil {
		lines = append(lines, "环境阻断："+safeTerminalText(run.EnvironmentBlocker.Fact.Reason+" · "+run.EnvironmentBlocker.Fact.Detail), "Ctrl+E 重新检查 · Ctrl+O 打开原终端（能力及原身份须核实）；解除不补投")
	}
	if nativeSessionAvailable(run) {
		lines = append(lines, "Ctrl+N 用 Codex CLI 打开原会话 · 实时查看；任务操作请通过 SteerLine")
	}
	if run.Error != nil {
		lines = append(lines, "错误："+safeTerminalText(*run.Error))
	}
	if run.Summary != nil {
		lines = append(lines, "摘要："+safeTerminalText(*run.Summary))
	}
	if run.Decision != nil {
		if waitingForMerge(run) {
			lines = append(lines, "可选反馈：按 a 请求原 worker 继续工作；无需反馈也可审阅合并 PR")
			if m.detailExpanded {
				lines = append(lines, "原问题："+safeTerminalText(run.Decision.Request.Question))
			}
		} else {
			lines = append(lines, "问题："+safeTerminalText(run.Decision.Request.Question))
		}
		if m.detailExpanded {
			lines = append(lines, renderDecisionIdentity(state, run, m.clock.Now())...)
			lines = append(lines, "上下文："+safeTerminalText(run.Decision.Request.Context))
			for i, option := range run.Decision.Request.Options {
				lines = append(lines, fmt.Sprintf("选项 %d · %s：%s", i+1, safeTerminalText(option.ID), safeTerminalText(option.Label)), "  详情："+safeTerminalText(option.Detail))
			}
		}
		lines = append(lines, "决策阶段："+decisionStatusLabel(run.Decision.Status), "当前答案："+safeTerminalText(DecisionAnswerSummary(run)))
		lines = append(lines, decisionAnswerStatusLines(state, row)...)
	} else if run.FinishTaskStatus == "waiting_decision" {
		lines = append(lines, "当前问题尚未同步，不能提交答案。")
	}
	if run.Abandonment != nil || run.TaskStatus == "discarded" || run.TaskStatus == "closed" {
		lines = append(lines, "资源：保留，需人工处置（未确认当前存在）")
	}
	lines = append(lines, "采样于 "+sampleTime(state.snapshot))
	if m.detailExpanded {
		lines = append(lines, "run_id："+safeTerminalText(run.RunID), "execution/session："+safeTerminalText(run.ExecutionID+" / "+run.SessionID), "attempt_id："+safeTerminalText(run.AttemptID), "执行："+safeTerminalText(run.ExecutionStatus)+" · 交付："+safeTerminalText(run.FinishTaskStatus))
		lines = append(lines, "工作目录："+safeTerminalText(firstText(stringValue(run.WorktreePath), run.Target.WorktreeID)), "分支："+safeTerminalText(firstText(stringValue(run.WorktreeBranch), run.Target.Branch)), "原生定位："+safeTerminalText(run.Backend+" / "+run.Protocol+" / "+run.Target.ID))
		if run.Process != nil {
			lines = append(lines, "宿主终端标识："+safeTerminalText(run.Process.Terminal))
		}
	}
	lines = append(lines, "过程记录请手动查看原生会话/诊断文件；没有通用打开入口时不能推定可用。", "f 完整最终回复 · v 历史决策 · u 只读核验")
	return append(lines, finalReplyLines(state)...)
}

func nativeDetailForTask(state projectState, taskID string) *api.TaskDetail {
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

func renderNativeTaskDetail(state projectState, task api.TaskDetail) []string {
	lines := []string{
		"任务 " + safeTerminalInlineText(task.TaskID) + " · " + safeTerminalInlineText(task.Title),
		"状态：" + safeTerminalText(task.Status),
	}
	return append(lines, renderNativeTaskRelations(state, task)...)
}

func renderNativeTaskRelations(state projectState, task api.TaskDetail) []string {
	var lines []string
	if len(task.StopPending) > 0 {
		lines = append(lines, "待核实原执行停止："+safeTerminalText(strings.Join(task.StopPending, "、")))
	}
	if state.snapshot.Connection.State == api.ConnectionOffline || state.snapshot.Connection.State == api.ConnectionNoRecord {
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
				if rt := state.snapshot.Connection.Snapshot.Page; rt != nil && rt.Control.DispatchPaused {
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
	if task.SpecPath != "" {
		lines = append(lines, "Spec："+safeTerminalInlineText(task.SpecPath))
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
		if action.Action != "start_candidate" && action.Action != "abandon_candidate" && action.Action != "answer_decision" && action.EnvironmentTarget == nil {
			continue
		}
		target := action.RunID
		if action.Action == "start_candidate" {
			target = action.RequestID
		}
		if action.EnvironmentTarget != nil {
			target = action.EnvironmentTarget.RunID
		}
		lines = append(lines, safeTerminalText(action.Action+" · request "+action.RequestID+" · workflow_run_id "+displayDash(target)+" · 实例 "+action.TargetInstance))
		if action.Payload.ExpectedTaskVersion != 0 {
			lines = append(lines, fmt.Sprintf("Task版本 %d · 候选版本 %d · 冻结方案 %s", action.Payload.ExpectedTaskVersion, action.Payload.ExpectedCandidateVersion, safeTerminalText(displayDash(action.Payload.ExecutionProfileID))))
		}
	}
	return lines
}

func actionStatusLabel(status string) string {
	switch status {
	case "submitting":

		return "提交中"
	case "submitted":
		return "已提交，待受理"
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
	if row != nil && row.kind == rowRun && pendingDecision(row.run) {
		reason := decisionAnswerAvailability(state, row)
		label := "回答当前决策"
		if waitingForMerge(row.run) {
			label = "可选反馈（请求继续工作）"
		}
		if strings.HasPrefix(reason, "提交后") {
			lines = append(lines, "操作：a "+label)
		} else {
			lines = append(lines, label+"不可用："+safeTerminalText(reason))
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
	if state.snapshot.Connection.State == api.ConnectionOffline || state.snapshot.Connection.State == api.ConnectionNoRecord {
		lines := []string{"服务离线 · 可以查看已有 runs 与日志", "服务入口：" + safeTerminalText(serviceWindowInstruction(state.ref)), "配置路径：" + safeTerminalText(state.ref.ConfigPath), terminalCapabilities()}
		return append(lines, runtimeIdentityLines(state.ref)...)
	}
	if state.snapshot.Connection.Diagnostic != "" {
		lines := []string{"服务状态：" + safeTerminalText(string(state.snapshot.Connection.State)), "连接诊断：" + safeTerminalText(state.snapshot.Connection.Diagnostic), "配置路径：" + safeTerminalText(state.ref.ConfigPath)}
		return append(lines, runtimeIdentityLines(state.ref)...)
	}
	return append([]string{"(没有选中对象)", "Enter 仅打开详情；开始任务请按 s。"}, runtimeIdentityLines(state.ref)...)
}

func runtimeIdentityLines(ref ProjectRef) []string {
	if ref.ExecutablePath == "" && ref.ApplicationHome == "" && ref.SourceCommit == "" && ref.SourceDirty == "" {
		return nil
	}
	return []string{
		"运行 exe：" + safeTerminalText(firstText(ref.ExecutablePath, "未知")),
		"运行 home：" + safeTerminalText(firstText(ref.ApplicationHome, "未知")),
		"项目配置：" + safeTerminalText(firstText(ref.ConfigPath, "未知")),
		"构建源码：" + safeTerminalText(firstText(ref.SourceCommit, "未知")) + " · dirty=" + safeTerminalText(firstText(ref.SourceDirty, "未知")),
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
	if state.snapshot.History == nil {
		lines = append(lines, "SQLite 决策历史不可用："+safeTerminalText(state.snapshot.HistoryError))
		return scrollLines(wrapLines(lines, width), state.detailScroll, height)
	}
	records := make([]api.DecisionHistory, 0)
	for _, record := range state.snapshot.History.Decisions {
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
		if record.Request == nil {
			lines = append(lines, "", "decision_id："+safeTerminalText(record.DecisionID), "问题记录格式未知；原记录保留，未猜测或导入。")
			continue
		}
		request := *record.Request
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
		if record.Answer != nil {
			answer := *record.Answer
			{
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

func renderDecisionIdentity(state projectState, run *api.CandidateSummary, now time.Time) []string {
	if run == nil || run.Decision == nil {
		return nil
	}
	waiting := "等待时长：未知"
	if state.snapshot.History != nil {
		for _, record := range state.snapshot.History.Decisions {
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
	if row != nil && row.run != nil && row.run.TaskStatus == "discarded" {
		return "Task 已丢弃；不再反馈"
	}
	if row != nil && row.run != nil && row.run.Abandonment != nil {
		return "候选已退出实验；不再反馈"
	}
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
	if row != nil && row.run != nil && row.run.Abandonment == nil && row.run.TaskStatus != "discarded" && row.run.EnvironmentBlocker != nil {
		return []string{"a 编辑本地草稿 · 发送被环境阻断：" + safeTerminalText(reason), "解除阻断后仍需人重新提交；没有自动补投队列。"}
	}
	if strings.HasPrefix(reason, "提交后") {
		if row != nil && row.run != nil && state.ref.Presentation.NativeWorkflow {
			return []string{"a 核验原会话身份并打开回答草稿 · " + reason, "显式提交后分别显示答案保存、发送回执与原会话轮次开始。"}
		}
		return []string{"a 核验固定后端能力并打开回答草稿 · " + reason, "显式提交后仍分别核验答案接受、后端送达、轮次启动和答案应用。"}
	}
	return []string{"a 不可用 · " + safeTerminalText(reason)}
}

func renderAnswerDetails(m Model, state projectState, height int) string {
	if m.answer == nil {
		return ""
	}
	height = max(1, height)
	wrapped := wrapLines(answerDetailLines(m, state), max(1, m.width-2))
	return strings.Join(fitLines(scrollLines(wrapped, m.answer.scroll, height), m.width, height), "\n")
}

func answerDetailLines(m Model, state projectState) []string {
	dialog := m.answer
	if dialog == nil {
		return nil
	}
	request, ok := dialog.request, dialog.request.Question != ""
	lines := []string{"完整问题背景 · 只读（F8 返回输入）", "项目：" + safeTerminalText(state.ref.Name), "task_id：" + safeTerminalText(displayDash(m.answerRunTask(state, dialog))), "run_id：" + safeTerminalText(dialog.runID), "decision_id：" + safeTerminalText(dialog.decisionID), fmt.Sprintf("运行版本：%d · 问题版本：%d · 目标服务：%s", dialog.runRevision, dialog.decisionVersion, safeTerminalText(dialog.instance))}
	if identity := strings.Split(dialog.identity, "\x00"); len(identity) == 4 {
		lines = append(lines, "原 execution_id："+safeTerminalText(identity[2]), "原 session："+safeTerminalText(identity[3]))
	}
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
	if len(note) == 0 {
		lines = append(lines, "（未填写）")
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
	lines = append(lines, "答案保存、送达和原会话轮次开始分别核实，不推断已落实。")
	if dialog.error != "" {
		lines = append(lines, styleAlert.Render(safeTerminalText(dialog.error)))
	}
	return lines
}

func (m Model) answerRunTask(state projectState, dialog *decisionAnswerDialog) string {
	if dialog == nil {
		return ""
	}
	if runtimeState := state.snapshot.Connection.Snapshot.Page; runtimeState != nil {
		for _, run := range runtimeState.Runs {
			if run != nil && run.RunID == dialog.runID {
				return run.TaskID
			}
		}
	}
	return ""
}

func renderFooter(m Model, state projectState) string {
	if m.operationDetails {
		return "↑↓/PgUp/PgDn 阅读 · F8/Esc 返回原操作"
	}
	if m.detailExpanded {
		return "↑↓/PgUp/PgDn 阅读 · Esc/d/Enter 返回 · q 退出"
	}
	if m.help {
		return "? / Esc 关闭帮助"
	}
	if m.answer != nil && m.answer.details {
		return "↑↓/PgUp/PgDn 阅读 · F8/Esc 返回输入"
	}
	if m.profileChoice != nil {
		return "Tab 列表/方案 · ↑↓ 选择 · Enter 确认 · F8 完整方案 · Esc 取消"
	}
	if m.confirm != nil {
		return "Tab 列表/确认 · Enter 确认 · F8 详情 · Esc/n 取消"
	}
	if m.answer != nil {
		enter := "Enter 转输入"
		if state.focus == 2 {
			enter = "Enter 换行"
		}
		if state.focus == 0 {
			enter = "Enter 返回草稿"
		}
		return "Tab 列表/选项/输入 · " + enter + " · F2 提交 · F8 背景 · Esc 保留"
	}
	row := m.selectedRow(m.selected)
	action := "Enter 到右下"
	if row != nil && pendingDecision(row.run) && !feedbackInProgress(state, row.run) {
		if waitingForMerge(row.run) {
			action = "a 可选反馈 · Enter 到右下"
		} else {
			action = "a/Enter 回复"
		}
	}
	if row != nil && row.kind == rowReady {
		action = "s 选方案"
	}
	if state.recoveryActive {
		action = "u/r 核验 · F8 报告 · Esc 动态"
	}
	if row != nil && nativeSessionAvailable(row.run) {
		action += " · Ctrl+N Codex CLI"
	}
	move := "↑↓ 选任务"
	if state.focus != 0 {
		move = "↑↓ 滚动"
	}
	return "Tab 列表/反馈 · " + move + " · " + action + " · d 完整详情\nF6/F7 候选 · 1–7 筛选 · f 完整回复 · ? 更多 · q 退出"
}

func renderHelp(state projectState) []string {
	return []string{"快捷键", "←/→ 切换项目 · Tab 列表/选项/输入", "↑/↓ 或 PgUp/PgDn 选择/滚动", "Enter 到右下或当前问题 · d 完整详情 · Esc 返回", "F6/F7（或 { / }）切换候选 · b 新增 · z 退出实验", "s 选择方案（Enter 明确确认开始）", "a 右下原地回复 · Enter 换行 · F2 明确提交 · F8 完整背景/身份", "f 最终回复 · [ / ] 历史业务轮次", "v 历史决策 · u 右下只读核验（r 重读/F8 全文）", "Ctrl+N 用 Codex CLI 打开本机候选（实时查看；任务操作通过 SteerLine）", "Ctrl+E 重新检查环境 · Ctrl+O 原终端（需能力）", "1 全部 · 2 Ready · 3 进行中 · 4 待决策 · 5 待合并 · 6 需处理 · 7 历史", "m 派发模式 · x 停止服务", "q 只退出控制台；服务继续运行"}
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
		height -= len(workspaceFooter(m, m.projects[m.selected]))
	}
	return max(3, height)
}

func compactLayout(m Model) bool {
	return m.width < 100
}

func listPanelWidth(m Model) int {
	left := m.width * 44 / 100
	if left < 38 {
		left = 38
	}
	if left > 90 {
		left = 90
	}
	return left
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
	if snapshot.Observation.State == api.SnapshotStale || snapshot.Observation.State == api.SnapshotReadFailed || snapshot.Observation.State == api.SnapshotUnknownVersion {
		suffix = " · 缓存/未验证"
	}
	return snapshot.Observation.SampledAt.Local().Format(timeLayout) + suffix
}

func executionVerifiedAt(state projectState, run *api.CandidateSummary) string {
	if run == nil || state.snapshot.History == nil || run.ExecutionID == "" {

		return "未知"
	}
	for _, execution := range state.snapshot.History.Executions {
		if execution.ExecutionID == run.ExecutionID && !execution.VerifiedAt.IsZero() {
			return execution.VerifiedAt.Local().Format(timeLayout)
		}
	}

	return "未知"
}

func detailPanelWidth(m Model) int {
	if compactLayout(m) {
		return m.width
	}
	return m.width - listPanelWidth(m) - 1
}
