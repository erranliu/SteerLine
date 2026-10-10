package console

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

// Labels and colors are keyed by the original field and exact value. Unknown
// values remain visible and never acquire a guessed state or translation.
var workerLabels = map[string]map[string]string{
	"step_id":          {"implement.execute": "实现执行", "implement.report": "实现报告", "publish": "发布", "await_merge": "等待合并", "delivered": "已交付"},
	"status":           {"queued": "待启动", "running": "运行中", "waiting_input": "等待输入", "blocked": "阻塞", "reconciling": "对账中", "waiting_external": "等待外部结果", "completed": "已完成", "abandoned": "已放弃"},
	"execution_status": {"pending": "待开始", "running": "运行中", "succeeded": "成功", "failed": "失败", "unknown": "未核实", "blocked": "阻断", "completed": "已完成"},
	"recovery_state":   {"unverified": "未核实", "interrupted": "已中断", "completed": "原轮次已结束", "live": "存活", "running": "运行中", "question": "等待原问题", "resuming": "恢复中"},
	"task_status":      {"Ready": "可开始", "open": "未开始", "in_progress": "进行中", "closed": "已关闭", "discarded": "已丢弃"},
}

func workerValue(field, value string, chinese bool) string {
	if value == "" {
		return "—"
	}
	text := safeTerminalInlineText(value)
	if chinese {
		if label := workerLabels[field][value]; label != "" {
			text += "（" + label + "）"
		}
	}
	_, known := workerLabels[field][value]
	if known && (field == "status" || field == "execution_status" || field == "recovery_state" || field == "task_status") {
		if field == "recovery_state" && value == "completed" {
			return styleWaiting.Render(text)
		}
		switch value {
		case "blocked", "failed":
			return styleAlert.Render(text)
		case "queued", "pending", "waiting_input", "waiting_external", "reconciling", "unknown", "unverified", "interrupted", "question", "resuming":
			return styleWaiting.Render(text)
		case "running", "in_progress", "live":
			return styleRunning.Render(text)
		case "completed", "succeeded", "closed":
			return styleSuccess.Render(text)
		}
	}
	return text
}

func workerStateLines(run *api.CandidateSummary, chinese bool) []string {
	if run == nil {
		return nil
	}
	lines := []string{"task_status:      " + workerValue("task_status", run.TaskStatus, chinese), "step_id:          " + workerValue("step_id", run.StepID, chinese), "status:           " + workerValue("status", run.BusinessStatus, chinese), "execution_status: " + workerValue("execution_status", run.ExecutionStatus, chinese)}
	if run.RecoveryRequired || run.RecoveryState != "" {
		lines = append(lines, fmt.Sprintf("recovery_required: %t · recovery_state: %s", run.RecoveryRequired, workerValue("recovery_state", run.RecoveryState, chinese)))
	}
	return lines
}

type workerTransitionFields struct {
	StepID            string `json:"step_id"`
	Status            string `json:"status"`
	ExecutionStatus   string `json:"execution_status"`
	ResourceStatus    string `json:"resource_status"`
	CurrentTurnID     string `json:"current_turn_id"`
	InputVersion      uint64 `json:"input_version"`
	RecoveryRequired  bool   `json:"recovery_required"`
	RecoveryState     string `json:"recovery_state"`
	EnvironmentReason string `json:"environment_reason"`
}

type workerTransition struct {
	From   workerTransitionFields `json:"from"`
	To     workerTransitionFields `json:"to"`
	Time   time.Time              `json:"-"`
	Detail string                 `json:"-"`
}

func decodeWorkerTransition(event api.ActivityEvent) (workerTransition, bool) {
	if event.Kind != "worker_state" {
		return workerTransition{}, false
	}
	raw, err := json.Marshal(event.Meta)
	var transition workerTransition
	if err != nil || json.Unmarshal(raw, &transition) != nil || transition.To.StepID == "" || transition.To.Status == "" {
		return workerTransition{}, false
	}
	transition.Time, transition.Detail = event.TS, event.Detail
	return transition, true
}

func workerTransitionLines(changes []workerTransition) []string {
	type message struct{ text, detail string }
	var messages []message
	stepWidth := 0
	for _, change := range changes {
		stepWidth = max(stepWidth, lipgloss.Width(safeTerminalInlineText(change.To.StepID)))
	}
	stepWidth = min(stepWidth, 32)
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		stamp := "时间未知"
		if !change.Time.IsZero() {
			stamp = change.Time.Local().Format("15:04:05")
		}
		var current []message
		if change.From.Status != change.To.Status {
			current = append(current, message{stamp + "  状态  " + padDisplay(safeTerminalInlineText(change.To.StepID), stepWidth) + "  " + workerValue("status", change.From.Status, false) + " → " + workerValue("status", change.To.Status, false), shortSummary(change.Detail, 120)})
		}
		if change.From.StepID != change.To.StepID {
			current = append(current, message{stamp + "  流程  " + workerValue("step_id", change.From.StepID, false) + " → " + workerValue("step_id", change.To.StepID, false), ""})
		}
		messages = append(messages, current...)
		if len(messages) >= 5 {
			messages = messages[:5]
			break
		}
	}
	if len(messages) == 0 {
		return nil
	}
	lines := []string{"流程与状态"}
	for _, item := range messages {
		lines = append(lines, item.text)
		if item.detail != "" {
			lines = append(lines, "          原因："+safeTerminalText(item.detail))
		}
	}
	return lines
}

func currentWorkerLines(run *api.CandidateSummary) []string {
	return []string{"当前流程：" + workerValue("step_id", run.StepID, true) + "  当前状态：" + workerValue("status", run.BusinessStatus, true)}
}

func diagnosticInWorkerTimeline(changes []workerTransition, run *api.CandidateSummary, diagnostic string) bool {
	visible := 0
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		if change.From.Status != change.To.Status {
			visible++
			if visible <= 5 && change.To.StepID == run.StepID && change.To.Status == run.BusinessStatus && change.Detail == diagnostic {
				return true
			}
		}
		if change.From.StepID != change.To.StepID {
			visible++
		}
		if visible >= 5 {
			break
		}
	}
	return false
}

func workerRawTransitionLines(changes []workerTransition) []string {
	if len(changes) == 0 {
		return nil
	}
	lines := []string{"原始状态变化（含技术字段）"}
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		stamp := "未提供时间"
		if !change.Time.IsZero() {
			stamp = change.Time.Local().Format("15:04:05")
		}
		lines = append(lines, stamp)
		if change.From.StepID != change.To.StepID || change.From.Status != change.To.Status {
			lines = append(lines, "step_id/status: "+workerValue("step_id", change.From.StepID, false)+" / "+workerValue("status", change.From.Status, false)+" → "+workerValue("step_id", change.To.StepID, false)+" / "+workerValue("status", change.To.Status, false))
		}
		for _, field := range []struct{ Name, From, To string }{
			{"execution_status", change.From.ExecutionStatus, change.To.ExecutionStatus},
			{"resource_status", change.From.ResourceStatus, change.To.ResourceStatus},
			{"recovery_state", change.From.RecoveryState, change.To.RecoveryState},
			{"environment_reason", change.From.EnvironmentReason, change.To.EnvironmentReason},
		} {
			if field.From != field.To {
				lines = append(lines, field.Name+": "+workerValue(field.Name, field.From, false)+" → "+workerValue(field.Name, field.To, false))
			}
		}
		if change.From.RecoveryRequired != change.To.RecoveryRequired {
			lines = append(lines, fmt.Sprintf("recovery_required: %t → %t", change.From.RecoveryRequired, change.To.RecoveryRequired))
		}
		if change.From.InputVersion != change.To.InputVersion {
			lines = append(lines, fmt.Sprintf("input_version: %d → %d", change.From.InputVersion, change.To.InputVersion))
		}
		if change.From.CurrentTurnID != change.To.CurrentTurnID {
			lines = append(lines, "current_turn_id: "+safeTerminalInlineText(change.From.CurrentTurnID)+" → "+safeTerminalInlineText(change.To.CurrentTurnID))
		}
		if change.Detail != "" {
			lines = append(lines, "原因："+safeTerminalText(change.Detail))
		}
	}
	return lines
}

func compactWorkerSummary(run *api.CandidateSummary) string {
	if run == nil {
		return "worker: —"
	}
	return workerValue("step_id", run.StepID, false) + " / " + workerValue("status", run.BusinessStatus, false)
}

func workerDiagnostic(run *api.CandidateSummary) string {
	if run == nil {
		return ""
	}
	if run.EnvironmentBlocker != nil {
		return run.EnvironmentBlocker.Fact.Reason + ": " + run.EnvironmentBlocker.Fact.Detail
	}
	if run.StageDiagnostic != "" {
		return run.StageDiagnostic
	}
	if run.Error != nil {
		return *run.Error
	}
	if run.PR.QueryError != nil {
		return *run.PR.QueryError
	}
	return ""
}

func shortResultPreview(text string) []string {
	lines := strings.Split(safeTerminalText(text), "\n")
	more := len(lines) > 3
	lines = lines[:min(3, len(lines))]
	for i := range lines {
		lines[i] = shortSummary(lines[i], 120)
	}
	if more {
		lines[len(lines)-1] += " …"
	}
	return lines
}
