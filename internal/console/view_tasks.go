package console

import (
	"fmt"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
	"time"
)

func projectCounts(state projectState, now time.Time) (ready, cached, running, decision, merge, attention int) {
	if state.snapshot.Tasks != nil {
		if valid, _ := state.client.CandidateAvailability(state.snapshot, now); valid {
			ready = len(state.snapshot.Tasks.Tasks)
		} else {
			cached = len(state.snapshot.Tasks.Tasks)
		}
	}
	if rt := state.snapshot.Observation.Page; rt != nil {
		runningTasks, decisionTasks, mergeTasks, attentionTasks := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, run := range rt.Runs {
			if run == nil {
				continue
			}
			if run.TaskStatus == "in_progress" && (run.ExecutionStatus == "pending" || run.ExecutionStatus == "running") {
				runningTasks[run.TaskID] = true
			}
			if waitingForDecision(run) {
				decisionTasks[run.TaskID] = true
			}
			if waitingForMerge(run) {
				mergeTasks[run.TaskID] = true
			}
			if runNeedsAttention(run) {
				attentionTasks[run.TaskID] = true
			}
		}
		running, decision, merge, attention = len(runningTasks), len(decisionTasks), len(mergeTasks), len(attentionTasks)
	}
	return
}

func renderRows(m Model, state projectState, width, height int) []string {
	rows := m.rows(m.selected)
	selected := 0
	for i, row := range rows {
		if row.key == state.selectedKey {
			selected = i
		}
	}
	out := []string{styleLabel.Render(fmt.Sprintf("任务列表 · %d/%d · %d %s", min(selected+1, len(rows)), len(rows), state.filter+1, filterDisplayName(state.filter)))}
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
		case snapshot.Connection.State == api.ConnectionOffline || snapshot.Connection.State == api.ConnectionNoRecord:
			out = append(out, "服务离线；已有运行和日志仍可查看。")
			out = append(out, "服务入口："+safeTerminalText(serviceWindowInstruction(state.ref)))
			out = append(out, "配置："+safeTerminalText(state.ref.ConfigPath))
		case snapshot.TaskError != "":
			out = append(out, "候选查询失败："+safeTerminalText(snapshot.TaskError))
		case snapshot.Observation.Page != nil && snapshot.Observation.Page.SlotOccupied && snapshot.Observation.Page.Control.DispatchMode == "auto":
			out = append(out, "没有可开始的任务；执行槽正被 "+safeTerminalText(stringValue(snapshot.Observation.Page.CurrentTask))+" 占用。")
		case state.filter != 0:
			out = append(out, "“"+filterDisplayName(state.filter)+"”筛选没有匹配任务；按 1 查看全部任务。")
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
	if height-len(out) > 2 {
		idWidth, statusWidth, countWidth := taskListWidths(width)
		out = append(out, "  "+truncate(padDisplay("ID", idWidth)+" "+padDisplay("任务提示", statusWidth)+" "+padDisplay("候选数量", countWidth)+" 标题", max(1, width-2)))
	}

	available := max(1, height-len(out))
	if available < 2 && len(out) > 1 {
		out = out[:1]
		available = max(1, height-1)
	}
	entries := make([][]string, len(rows))
	for i, row := range rows {
		entries[i] = compactTaskRow(m, state, row, width)
		if row.key == state.selectedKey {
			selected = i
		}
	}
	if len(entries[selected]) > available && len(out) > 1 {
		out = out[:1]
		available = max(1, height-len(out))
	}
	start := selected
	used := len(entries[selected])
	for start > 0 && used+len(entries[start-1]) <= available {
		start--
		used += len(entries[start])
	}
	for index := start; index < len(rows); index++ {
		entry, row := entries[index], rows[index]
		if len(out)+len(entry) > height && index != selected {
			break
		}
		for i, line := range entry {
			prefix := "  "
			if i == 0 && row.key == state.selectedKey {
				prefix = "▸ "
			}
			if row.key == state.selectedKey {
				line = padDisplay(line, max(1, width-2))
				if state.focus == 0 {
					line = styleSelected.Render(line)
				} else {
					line = styleHead.Render(line)
				}
			}
			out = append(out, prefix+line)
		}
	}
	return out[:min(len(out), height)]
}

func compactTaskRow(m Model, state projectState, row consoleRow, width int) []string {
	available := max(1, width-2)
	idWidth, statusWidth, countWidth := taskListWidths(width)
	hint := taskRowHint(m, state, row)
	cell := safeTerminalInlineText(hint)
	if hint == "阻塞" {
		cell = styleAlert.Render(padDisplay(truncate(cell, statusWidth), statusWidth))
	} else {
		cell = padDisplay(truncate(cell, statusWidth), statusWidth)
	}
	count := padDisplay(fmt.Sprint(len(candidateRuns(state, row.taskID))), countWidth)
	line := padDisplay(truncate(safeTerminalInlineText(row.taskID), idWidth), idWidth) + " " + cell + " " + count + " " + safeTerminalInlineText(row.title)
	return []string{truncate(line, available)}
}

func taskListWidths(width int) (int, int, int) {
	available := max(1, width-2)
	if available < 42 {
		return 5, min(10, max(1, available-18)), 8
	}
	return 8, 10, 8
}

func taskBlockerLines(row consoleRow) []string {
	var lines []string
	if row.native == nil || taskRowRawStatus(row) != "open" {
		return lines
	}
	task := row.native
	blockers := task.UnmetPredecessors
	if len(blockers) == 0 {
		blockers = task.Predecessors
	}
	for _, p := range blockers {
		if len(task.UnmetPredecessors) > 0 || !p.Delivered && p.Blocker != "" {
			lines = append(lines, "依赖阻塞：Task "+safeTerminalInlineText(p.TaskID)+" · "+safeTerminalInlineText(firstText(p.Blocker, "未提供阻塞原因"))+" · status="+safeTerminalInlineText(firstText(p.Status, "—")))
		}
	}
	if task.Status == "open" {
		for _, reason := range task.StartupBlockers {
			lines = append(lines, "启动阻塞："+safeTerminalInlineText(reason))
		}
	}
	return lines
}

func taskRowRawStatus(row consoleRow) string {
	if row.run != nil && row.run.TaskStatus != "" {
		return row.run.TaskStatus
	}
	if row.native != nil {
		return row.native.Status
	}
	return ""
}

func taskRowBlocked(row consoleRow) bool {
	status := taskRowRawStatus(row)
	if status == "closed" || status == "discarded" {
		return false
	}
	if row.run != nil && (row.run.WorkflowStatus == "blocked" || row.run.EnvironmentBlocker != nil) {
		return true
	}
	if row.native != nil && status == "open" {
		if len(row.native.UnmetPredecessors) > 0 || len(row.native.StartupBlockers) > 0 {
			return true
		}
		for _, p := range row.native.Predecessors {
			if !p.Delivered && p.Blocker != "" {
				return true
			}
		}
	}
	return status == "blocked"
}

func taskRowHint(m Model, state projectState, row consoleRow) string {
	status := taskRowRawStatus(row)
	if status == "closed" || status == "completed" || status == "discarded" {
		if label := workerLabels["task_status"][status]; label != "" {
			return label
		}
		return displayTaskStatus(status)
	}
	if status == "in_progress" {
		runs := candidateRuns(state, row.taskID)
		if len(runs) == 0 && row.run != nil {
			runs = []*api.CandidateSummary{row.run}
		}
		var merging, input, running, queued, external, uncertain, blocked, failed bool
		for _, run := range runs {
			if !candidateBusinessOpen(run) {
				continue
			}
			switch {
			case run.EnvironmentBlocker != nil, run.WorkflowStatus == "blocked", run.ExecutionStatus == "blocked":
				blocked = true
			case run.ExecutionStatus == "failed":
				failed = true
			case recoveryNeedsAttention(run), run.ExecutionStatus == "unknown", run.WorkflowStatus == "reconciling", run.Error != nil && strings.TrimSpace(*run.Error) != "", run.PR.QueryError != nil && strings.TrimSpace(*run.PR.QueryError) != "":
				uncertain = true
			case waitingForMerge(run):
				merging = true
			case waitingForDecision(run):
				input = true
			case run.WorkflowStatus == "running", run.ExecutionStatus == "running", run.ExecutionStatus == "active":
				running = true
			case run.WorkflowStatus == "queued", run.ExecutionStatus == "pending":
				queued = true
			case run.WorkflowStatus == "waiting_external":
				external = true
			default:
				uncertain = true
			}
		}
		// A candidate that can still deliver takes priority over a sibling's
		// failure. Task terminal state above always takes priority over all runs.
		switch {
		case merging:
			return "等待合并"
		case input:
			return "待决策"
		case running:
			return "进行中"
		case external:
			return "等待外部"
		}
		if progress := taskStartProgress(state, row.taskID); progress != "" {
			return progress
		}
		switch {
		case queued:
			return "启动中"
		case uncertain:
			return "待核验"
		case blocked:
			return "阻塞"
		case failed:
			return "失败"
		default:
			return "未核实"
		}
	}
	if taskRowBlocked(row) {
		return "阻塞"
	}
	if progress := taskStartProgress(state, row.taskID); progress != "" {
		return progress
	}
	if row.kind == rowReady {
		if valid, _ := state.client.CandidateAvailability(state.snapshot, m.clock.Now()); valid {
			return "可开始"
		}
		return "候选缓存"
	}
	if label := workerLabels["task_status"][taskRowRawStatus(row)]; label != "" {
		return label
	}
	return "未核实"
}

func taskStartProgress(state projectState, taskID string) string {
	if action := taskOperation(state, taskID); action != nil && (action.Action == "start_task" || action.Action == "start_candidate") {
		return operationProgress(action)
	}
	return ""
}

func renderTaskListRow(id, title, taskStatus, executionStatus string, width int) string {
	idWidth, taskWidth, executionWidth := 8, 8, 14
	if width < 48 {
		taskWidth, executionWidth = 6, 8
		if executionStatus == "失败（历史）" {
			executionStatus = "失败"
		}
	}
	identity := padDisplay(truncate(safeTerminalInlineText(id), idWidth), idWidth)
	task := padDisplay(truncate(safeTerminalInlineText(taskStatus), taskWidth), taskWidth)
	execution := padDisplay(truncate(safeTerminalInlineText(executionStatus), executionWidth), executionWidth)
	titleWidth := max(1, width-idWidth-taskWidth-executionWidth-3)
	return truncate(identity+" "+task+" "+execution+" "+truncate(safeTerminalInlineText(title), titleWidth), width)
}

func renderWorkspaceRow(id, title, task, execution, attention string, width int) string {
	columns := []int{8, 8, 14, 10}
	if width < 60 {
		columns = []int{6, 6, 8, 6}
		if execution == "失败（历史）" {
			execution = "历史失败"
		}
		if fields := strings.Fields(attention); len(fields) > 0 {
			attention = fields[0]
		}
	}
	parts := []string{id, task, execution, attention}
	var prefix string
	for i, part := range parts {
		cell := padDisplay(truncate(safeTerminalInlineText(part), columns[i]), columns[i])
		if i > 0 && part != "任务状态" && part != "执行状态" && part != "状态" && part != "执行" && part != "待处理" {
			switch {
			case strings.Contains(part, "失败") || strings.Contains(part, "受阻") || strings.Contains(part, "阻塞"):
				cell = styleAlert.Render(cell)
			case strings.Contains(part, "待") || strings.Contains(part, "核验") || strings.Contains(part, "缓存"):
				cell = styleWaiting.Render(cell)
			case strings.Contains(part, "进行中") || strings.Contains(part, "执行中"):
				cell = styleRunning.Render(cell)
			case strings.Contains(part, "成功") || strings.Contains(part, "已完成"):
				cell = styleSuccess.Render(cell)
			}
		}
		prefix += cell + " "
	}
	return truncate(prefix+safeTerminalInlineText(title), width)
}

func rowTaskStatus(row consoleRow) string {
	if taskRowBlocked(row) {
		return "阻塞"
	}
	if row.kind == rowReady {
		return "可开始"
	}
	if row.run != nil && row.run.TaskStatus != "" {
		return displayTaskStatus(row.run.TaskStatus)
	}
	if row.native != nil {
		return displayTaskStatus(row.native.Status)
	}
	if row.kind == rowReady {

		return "可开始"
	}

	return "未知"
}

func rowExecutionStatus(row consoleRow) string {
	if row.run == nil {
		return "—"
	}
	return displayExecutionStatus(row.run)
}

func displayExecutionStatus(run *api.CandidateSummary) string {
	if run == nil {
		return "—"
	}
	if historicalFailedRun(run) {
		return "失败（历史）"
	}
	if run.RecoveryRequired {
		switch run.RecoveryState {
		case "interrupted":
			return "已中断，待手动继续"
		case "unverified", "":

			return "状态未核实"
		case "running", "question", "live":
			return "原 worker 仍在运行"
		case "completed":
			return "原轮次已完成，待反馈"
		}
	}
	switch run.ExecutionStatus {
	case "pending":

		return "待开始"
	case "running":

		return "执行中"
	case "unknown":

		return "待核实"
	case "failed":

		return "失败"
	case "blocked":

		return "阻塞"
	case "succeeded":

		return "成功"
	case "completed":

		return "已完成"
	case "":
		return "—"
	default:
		return run.ExecutionStatus
	}
}

func displayTaskStatus(status string) string {
	switch status {
	case "Ready":

		return "可开始"
	case "open":

		return "未开始"
	case "in_progress":

		return "进行中"
	case "blocked":

		return "阻塞"
	case "closed", "completed":

		return "已完成"
	default:
		if strings.HasPrefix(status, "需处理") {

			return "需处理"
		}
		return status
	}
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

func runStatus(run *api.CandidateSummary) string {
	if run == nil {

		return "unknown"
	}
	if waitingForDecision(run) {

		return "待决策"
	}
	if run.EnvironmentBlocker != nil {
		return "需处理 · 环境受阻"
	}
	if recoveryNeedsAttention(run) {
		return "需处理 · 恢复待核实"
	}
	if run.ExecutionStatus == "unknown" || run.ExecutionStatus == "failed" || run.ExecutionStatus == "blocked" {
		return "需处理 · " + run.ExecutionStatus
	}
	if run.WorkflowStatus == "blocked" {
		return "需处理 · blocked"
	}
	if run.TaskStatus == "discarded" {
		return "需处理 · discarded"
	}
	if run.Error != nil && strings.TrimSpace(*run.Error) != "" {
		return "需处理 · error"
	}
	if run.PR.QueryError != nil && strings.TrimSpace(*run.PR.QueryError) != "" {
		return "需处理 · PR query error"
	}
	if waitingForMerge(run) {
		return "待合并"
	}
	if run.WorkflowStatus == "completed" {
		return "completed"
	}
	if run.TaskStatus == "in_progress" && (run.ExecutionStatus == "pending" || run.ExecutionStatus == "running") {

		return "进行中"
	}
	if run.WorkflowStatus != "" {
		return run.WorkflowStatus
	}
	return firstText(run.ExecutionStatus, "unknown")
}

func taskNextAction(run *api.CandidateSummary) string {
	switch {
	case run == nil:

		return "无"
	case run.TaskStatus == "discarded":
		if run.PR.State == "merged" {
			return "合并冲突，需人工处置"
		}
		return "保留资源，需人工处置；核实原执行停止"
	case waitingForDecision(run):
		return "回答当前问题（按 a）"
	case runNeedsAttention(run):
		return "查看错误与恢复核验（按 u）"
	case waitingForMerge(run):
		return "审阅并人工合并 PR"
	default:

		return "无"
	}
}

func runNeedsAttention(run *api.CandidateSummary) bool {
	if run == nil {
		return false
	}
	if historicalFailedRun(run) {
		return false
	}
	if run.TaskStatus == "closed" && (run.ExecutionStatus == "pending" || run.ExecutionStatus == "running") {
		return true
	}
	return waitingForDecision(run) || run.EnvironmentBlocker != nil || recoveryNeedsAttention(run) || run.ExecutionStatus == "unknown" || run.ExecutionStatus == "failed" || run.ExecutionStatus == "blocked" || run.WorkflowStatus == "blocked" || run.TaskStatus == "discarded" || (run.Error != nil && strings.TrimSpace(*run.Error) != "") || (run.PR.QueryError != nil && strings.TrimSpace(*run.PR.QueryError) != "")
}

func recoveryNeedsAttention(run *api.CandidateSummary) bool {
	return run != nil && run.RecoveryRequired && (run.RecoveryState == "" || run.RecoveryState == "unverified" || run.RecoveryState == "interrupted")
}

func historicalFailedRun(run *api.CandidateSummary) bool {
	return run != nil &&
		run.TaskStatus == "closed" && run.EndedAt != nil &&
		(run.ExecutionStatus == "failed" || run.ExecutionStatus == "blocked") &&
		run.WorkflowStatus == "blocked" && run.Decision == nil &&
		(run.Error == nil || strings.TrimSpace(*run.Error) == "") &&
		(run.PR.QueryError == nil || strings.TrimSpace(*run.PR.QueryError) == "")
}

func filterDisplayName(index int) string {
	return []string{"全部", "Ready", "进行中", "待决策", "待合并", "需处理", "历史", "阻塞"}[min(max(index, 0), 7)]
}
