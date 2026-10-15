package sqlite

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"strings"
)

func runnerWorkflowActivity(v api.ActivityEvent, t runnercontract.WorkflowTransition) api.ActivityEvent {
	v.Kind = "worker_state"
	v.Status = t.To.Status
	v.Meta = map[string]any{"from": t.From, "to": t.To}
	stages := map[string]string{"implement.execute": "实施", "implement.report": "实施报告", "publish": "发布 PR", "publish.report": "发布报告", "pr_ready": "等待合并", "await_merge": "等待合并", "delivered": "已交付"}
	states := map[string]string{"accepted": "已接受", "queued": "排队中", "running": "执行中", "preparing": "准备中", "blocked": "已阻塞", "waiting_input": "等待回答", "reconciling": "状态核验中", "pr_ready": "PR 已就绪", "stopped": "已停止", "failed": "失败", "completed": "已完成"}
	label := func(m map[string]string, s string) string {
		if text := m[s]; text != "" {
			return text
		}
		if s == "" {
			return "未开始"
		}
		return s
	}
	stage := label(stages, t.To.Stage)
	parts := []string{}
	if t.From.Stage != t.To.Stage {
		parts = append(parts, "流程："+label(stages, t.From.Stage)+" → "+stage)
	}
	if t.From.Status != t.To.Status {
		parts = append(parts, stage+" · 状态："+label(states, t.From.Status)+" → "+label(states, t.To.Status))
	}
	if len(parts) == 0 {
		parts = append(parts, stage+" · 诊断信息更新")
	}
	v.Title = strings.Join(parts, "；")
	v.Detail = t.To.Diagnostic
	return v
}
