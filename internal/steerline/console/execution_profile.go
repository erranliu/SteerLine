package console

import (
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline"
)

const executionConfigChanged = "服务与控制台加载的执行配置不同；重开控制台并核对服务配置"

func executionConfigMismatch(ref ProjectRef, snapshot ProjectSnapshot) bool {
	runtime := snapshot.Connection.Snapshot.Runtime
	return runtime != nil && ref.Config.LoadedConfigDigest() != "" &&
		runtime.Service.ConfigDigest != ref.Config.LoadedConfigDigest()
}

func executionHostLabel(c steerline.Config) string {
	switch c.Host {
	case "local", "":
		return "本地 Codex"
	case "orca":
		return "Orca 本机终端"
	case "orca_supervised":
		return "Orca 受监管远程"
	default:
		return "执行后端未知"
	}
}

func executionAgentName(c steerline.Config) string {
	if strings.TrimSpace(c.Agent) == "" {
		return "codex"
	}
	return c.Agent
}

func executionModelLabel(c steerline.Config) string {
	if executionAgentName(c) != "codex" {
		return "由所选 agent 决定"
	}
	if c.Model != "" {
		return c.Model
	}
	return "未在项目配置固定（由 Codex 配置决定）"
}

func executionEffortLabel(c steerline.Config) string {
	if executionAgentName(c) != "codex" {
		return "不适用"
	}
	if c.ReasoningEffort != "" {
		return c.ReasoningEffort
	}
	return "默认强度（未固定，由目标 agent 决定）"
}

func renderFixedRunExecutionProfile(state projectState, run *steerline.Run) []string {
	if run == nil {
		return nil
	}
	workflows := []steerline.WorkflowRunRecord(nil)
	if state.snapshot.Domain != nil {
		workflows = state.snapshot.Domain.WorkflowRuns
	} else if state.snapshot.Observation.Runtime != nil {
		workflows = state.snapshot.Observation.Runtime.WorkflowRuns
	}
	for _, workflow := range workflows {
		if workflow.WorkflowRunID != run.RunID || workflow.ExecutionProfile == nil {
			continue
		}
		fixed := workflow.ExecutionProfile
		config := steerline.Config{Host: fixed.Host, Agent: fixed.Agent, Model: fixed.Model, ReasoningEffort: fixed.ReasoningEffort}
		return []string{
			"执行方案：" + safeTerminalInlineText(fixed.Name) + " [" + safeTerminalInlineText(fixed.ID) + "] · " + safeTerminalInlineText(executionHostLabel(config)),
			"Agent / Model / Thinking：" + safeTerminalInlineText(fixed.Agent) + " / " + safeTerminalInlineText(executionModelLabel(config)) + " / " + safeTerminalInlineText(executionEffortLabel(config)),
		}
	}
	return nil
}
