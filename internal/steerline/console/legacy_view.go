package console

import (
	"fmt"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func renderLegacyTaskDetail(state projectState, task steerline.LegacyBeadsTask) []string {
	lines := []string{
		"旧 Beads Task：" + safeTerminalInlineText(task.ID),
		"标题：" + safeTerminalText(task.Title),
		"状态：" + safeTerminalText(task.Status),
		"类型 / 优先级：" + safeTerminalText(task.Type) + " / " + fmt.Sprint(task.Priority),
		"此项目只读；原执行仍由旧服务结算。s/m/p/x 在此项目不可用。",
	}
	if task.Assignee != "" {
		lines = append(lines, "负责人："+safeTerminalText(task.Assignee))
	}
	if legacy := state.snapshot.Legacy; legacy != nil {
		lines = append(lines, "任务采样时间："+legacy.SampledAt.Local().Format(timeLayout))
		status := firstText(legacy.ServiceStatus, "未知")
		liveness := map[string]string{"live": "原进程身份仍存活", "exited": "原进程已退出", "unknown": "原进程存活待核实"}[legacy.ServiceLiveness]
		if liveness == "" {
			liveness = "原进程存活待核实"
		}
		lines = append(lines, "旧服务记录："+safeTerminalText(status)+" · "+liveness)
		if legacy.ServicePID > 0 {
			lines = append(lines, "旧服务 PID："+fmt.Sprint(legacy.ServicePID))
		}
		if !legacy.HeartbeatAt.IsZero() {
			lines = append(lines, "旧心跳："+legacy.HeartbeatAt.Local().Format(timeLayout))
		}
		if legacy.RuntimeError != "" {
			lines = append(lines, "旧运行投影："+safeTerminalText(legacy.RuntimeError))
		}
	}
	lines = append(lines, "", "Beads 硬前置")
	if len(task.Predecessors) == 0 {
		lines = append(lines, "无")
	} else {
		for _, id := range task.Predecessors {
			lines = append(lines, safeTerminalInlineText(id))
		}
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
	return append(lines, "", "只读视图：任务以 Beads 当前库为准；旧运行投影仅作诊断。")
}
