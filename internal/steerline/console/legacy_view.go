package console

import (
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func renderLegacyTaskDetail(state projectState, task steerline.LegacyBeadsTask) []string {
	lines := []string{
		"旧任务 " + safeTerminalInlineText(task.ID) + " · " + safeTerminalText(task.Title),
		"状态：" + safeTerminalText(task.Status),
		"只读 · 原执行仍由旧服务结算",
	}
	if task.Assignee != "" {
		lines = append(lines, "负责人："+safeTerminalText(task.Assignee))
	}
	if legacy := state.snapshot.Legacy; legacy != nil {
		status := firstText(legacy.ServiceStatus, "未知")
		liveness := map[string]string{"live": "原进程身份仍存活", "exited": "原进程已退出", "unknown": "原进程存活待核实"}[legacy.ServiceLiveness]
		if liveness == "" {
			liveness = "原进程存活待核实"
		}
		lines = append(lines, "旧服务记录："+safeTerminalText(status)+" · "+liveness)
		if legacy.RuntimeError != "" {
			lines = append(lines, "旧运行投影："+safeTerminalText(legacy.RuntimeError))
		}
	}
	if len(task.Predecessors) > 0 {
		lines = append(lines, "前置："+safeTerminalInlineText(strings.Join(task.Predecessors, "、")))
	}
	if legacy := state.snapshot.Legacy; legacy != nil {
		for _, run := range legacy.Runs {
			if run.TaskID != task.ID {
				continue
			}
			lines = append(lines, "", "旧运行记录")
			lines = append(lines, "执行 / 结算："+safeTerminalText(run.ExecutionStatus)+" / "+safeTerminalText(run.FinishStatus))
			if run.PRURL != "" {
				lines = append(lines, "PR："+safeTerminalText(run.PRURL)+" · "+safeTerminalText(run.PRState))
			}
			if run.Error != "" {
				lines = append(lines, "阻塞："+safeTerminalText(run.Error))
			}
		}
		if legacy.DependencyError != "" {
			lines = append(lines, "依赖查询："+safeTerminalText(legacy.DependencyError))
		}
	}
	if state.snapshot.LegacyError != "" {
		lines = append(lines, "任务查询："+safeTerminalText(state.snapshot.LegacyError))
	}
	return lines
}
