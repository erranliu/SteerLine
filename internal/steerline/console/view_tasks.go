package console

import (
	"fmt"
	"strings"
	"time"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func projectCounts(state projectState, now time.Time) (ready, cached, running, decision, merge, attention int) {
	if state.snapshot.Tasks != nil {
		if valid, _ := state.client.CandidateAvailability(state.snapshot, now); valid {
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
			if run.TaskStatus == "in_progress" && (run.ExecutionStatus == "pending" || run.ExecutionStatus == "running") {
				running++
			}
			if run.FinishTaskStatus == "waiting_decision" && run.TaskStatus != "closed" {
				decision++
			}
			if run.FinishTaskStatus == "waiting_pr_merge" && run.TaskStatus != "closed" {
				merge++
			}
			if runNeedsAttention(run) {
				attention++
			}
		}
	}
	return
}

func renderRows(m Model, state projectState, width, height int) []string {
	rows := m.rows(m.selected)
	focus := ""
	if state.focus == 0 {
		focus = " [焦点]"
	}
	out := []string{styleLabel.Render(fmt.Sprintf("任务列表%s · %d 项 · %d %s", focus, len(rows), state.filter+1, filterDisplayName(state.filter)))}
	if height >= 6 {
		out = append(out, styleDim.Render("筛选 1全部  2Ready  3进行中  4待决策"), styleDim.Render("     5待合并  6需处理  7历史"))
	}
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
		case snapshot.Connection.State == productapi.ConnectionOffline || snapshot.Connection.State == productapi.ConnectionNoRecord:
			out = append(out, "服务离线；已有运行和日志仍可查看。")
			out = append(out, "服务入口："+safeTerminalText(serviceWindowInstruction(state.ref)))
			out = append(out, "配置："+safeTerminalText(state.ref.ConfigPath))
		case snapshot.TaskError != "":
			out = append(out, "候选查询失败："+safeTerminalText(snapshot.TaskError))
		case snapshot.Observation.Runtime != nil && snapshot.Observation.Runtime.SlotOccupied && snapshot.Observation.Runtime.Control.DispatchMode == "auto":
			out = append(out, "没有可开始的任务；执行槽正被 "+safeTerminalText(stringValue(snapshot.Observation.Runtime.CurrentTask))+" 占用。")
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
	if len(out)+1 < height {
		taskHeader, executionHeader := "任务状态", "执行状态"
		if width-2 < 48 {
			taskHeader, executionHeader = "状态", "执行"
		}
		out = append(out, renderTaskListRow("任务编号", "任务名称", taskHeader, executionHeader, max(1, width-2)))
	}
	visible := contentWindow(rows, state.selectedKey, max(1, height-len(out)))
	for _, row := range visible {
		taskStatus := rowTaskStatus(row)
		if row.kind == rowReady {
			if valid, _ := state.client.CandidateAvailability(state.snapshot, m.clock.Now()); valid {
				taskStatus = "可开始"
			} else {
				taskStatus = "缓存"
			}
		}
		line := renderTaskListRow(row.taskID, row.title, taskStatus, rowExecutionStatus(row), max(1, width-2))
		if row.key == state.selectedKey {
			line = styleSelected.Render("▸ " + line)
		} else if row.kind == rowReady && taskStatus == "缓存" {
			line = "  " + styleDim.Render(line)
		} else {
			line = "  " + line
		}
		out = append(out, line)
	}
	return out
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

func rowTaskStatus(row consoleRow) string {
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

func displayExecutionStatus(run *contracts.Run) string {
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
		return "阻塞"
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

func runStatus(run *contracts.Run) string {
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
	if run.TaskStatus == "in_progress" && (run.ExecutionStatus == "pending" || run.ExecutionStatus == "running") {
		return "进行中"
	}
	if run.FinishTaskStatus != "" {
		return run.FinishTaskStatus
	}
	return firstText(run.ExecutionStatus, "unknown")
}

func taskNextAction(run *contracts.Run) string {
	switch {
	case run == nil:
		return "无"
	case run.FinishTaskStatus == "waiting_decision" && run.TaskStatus != "closed":
		return "回答当前问题（按 a）"
	case runNeedsAttention(run):
		return "查看错误与恢复核验（按 u）"
	case run.FinishTaskStatus == "waiting_pr_merge":
		return "审阅并人工合并 PR"
	default:
		return "无"
	}
}

func runNeedsAttention(run *contracts.Run) bool {
	if run == nil {
		return false
	}
	if historicalFailedRun(run) {
		return false
	}
	if run.TaskStatus == "closed" && (run.ExecutionStatus == "pending" || run.ExecutionStatus == "running") {
		return true
	}
	return run.FinishTaskStatus == "waiting_decision" || run.ExecutionStatus == "unknown" || run.ExecutionStatus == "failed" || run.ExecutionStatus == "blocked" || run.FinishTaskStatus == "blocked" || (run.Error != nil && strings.TrimSpace(*run.Error) != "") || (run.PR.QueryError != nil && strings.TrimSpace(*run.PR.QueryError) != "")
}

func historicalFailedRun(run *contracts.Run) bool {
	return run != nil &&
		run.TaskStatus == "closed" && run.EndedAt != nil &&
		(run.ExecutionStatus == "failed" || run.ExecutionStatus == "blocked") &&
		run.FinishTaskStatus == "blocked" && run.Decision == nil &&
		(run.Error == nil || strings.TrimSpace(*run.Error) == "") &&
		(run.PR.QueryError == nil || strings.TrimSpace(*run.PR.QueryError) == "")
}

func filterDisplayName(index int) string {
	return []string{"全部", "Ready", "进行中", "待决策", "待合并", "需处理", "历史"}[min(max(index, 0), 6)]
}
