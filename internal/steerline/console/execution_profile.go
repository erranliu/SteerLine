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

func renderExecutionProfile(ref ProjectRef, snapshot ProjectSnapshot) []string {
	if ref.Legacy != nil || ref.Error != "" || ref.Config.TaskAuthority != "native" {
		return nil
	}
	if executionConfigMismatch(ref, snapshot) {
		return []string{"", "执行配置：" + executionConfigChanged}
	}
	label := "执行配置 · 配置预览；服务未连接"
	if snapshot.Connection.State == steerline.ConnectionConnected && snapshot.Connection.Snapshot.Runtime != nil {
		label = "执行配置 · 当前服务"
		if ref.Config.LoadedConfigDigest() == "" || snapshot.Connection.Snapshot.Runtime.Service.ConfigDigest == "" {
			label = "执行配置 · 服务版本未核对"
		}
	}
	c := ref.Config
	lines := []string{"", label,
		"位置：" + safeTerminalInlineText(executionHostLabel(c)),
		"Agent：" + safeTerminalInlineText(executionAgentName(c)),
		"Model：" + safeTerminalInlineText(executionModelLabel(c)),
		"Thinking effort：" + safeTerminalInlineText(executionEffortLabel(c)),
		"Sandbox：" + safeTerminalInlineText(c.Sandbox),
	}
	if c.Profile != "" && executionAgentName(c) == "codex" {
		lines = append(lines, "Codex profile："+safeTerminalInlineText(c.Profile))
	}
	if c.Host == "orca_supervised" && c.Orca.Supervised != nil {
		lines = append(lines,
			"目标环境："+safeTerminalInlineText(c.Orca.Supervised.EnvironmentID),
			"连接："+safeTerminalInlineText(c.Orca.Supervised.ConnectionID))
	}
	return lines
}
