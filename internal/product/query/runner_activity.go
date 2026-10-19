package query

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func runnerWorkflowActivity(v api.ActivityEvent, t runnercontract.WorkflowTransition) api.ActivityEvent {
	v.Kind = "worker_state"
	v.Status = t.To.Status
	v.Meta = map[string]any{"from": t.From, "to": t.To}
	states := map[string]string{"accepted": "已接受", "queued": "排队中", "running": "执行中", "preparing": "准备中", "blocked": "已阻塞", "waiting_input": "等待回答", "waiting_external": "等待外部事实", "reconciling": "状态核验中", "pr_ready": "PR 已就绪", "stopped": "已停止", "failed": "失败", "completed": "已完成"}
	label := func(s string) string {
		if text := states[s]; text != "" {
			return text
		}
		if s == "" {
			return "未开始"
		}
		return s
	}
	switch {
	case t.From.Stage == "":
		v.Title = "Workflow 开始：" + t.To.Stage + "（" + label(t.To.Status) + "）"
	case t.From.Stage != t.To.Stage:
		v.Title = "Workflow 切换：" + t.From.Stage + " → " + t.To.Stage + "（" + label(t.To.Status) + "）"
	case t.From.Status != t.To.Status:
		v.Title = "状态切换：" + label(t.From.Status) + " → " + label(t.To.Status)
	default:
		v.Title = "诊断信息更新"
	}
	v.Detail = t.To.Diagnostic
	return v
}
