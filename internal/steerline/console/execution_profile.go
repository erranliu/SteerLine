package console

import (
	"fmt"
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
	return "未在全局配置固定（由 Codex 配置决定）"
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
	fixed := fixedRunExecutionProfile(state, run)
	if fixed == nil {
		return nil
	}
	config := steerline.Config{Host: fixed.Host, Agent: fixed.Agent, Model: fixed.Model, ReasoningEffort: fixed.ReasoningEffort}
	lines := []string{
		"执行方案：" + safeTerminalInlineText(fixed.Name) + " [" + safeTerminalInlineText(fixed.ID) + "] · " + safeTerminalInlineText(executionHostLabel(config)),
		"Agent / Model / Thinking：" + safeTerminalInlineText(fixed.Agent) + " / " + safeTerminalInlineText(executionModelLabel(config)) + " / " + safeTerminalInlineText(executionEffortLabel(config)),
	}
	lines = append(lines, "发布责任："+safeTerminalInlineText(fixed.PublicationPolicy.Publisher), "冻结发布目标："+safeTerminalInlineText(fixed.Publication.Provider.Kind)+" · "+safeTerminalInlineText(fixed.Publication.Provider.BaseURL)+"/"+safeTerminalInlineText(fixed.Publication.Provider.Repository)+" · base "+safeTerminalInlineText(fixed.Publication.BaseBranch), "Git remote："+safeTerminalInlineText(fixed.Publication.Remote)+" · "+safeTerminalInlineText(fixed.Publication.RemoteURL), "发布工具："+safeTerminalInlineText(fixed.Publication.Git.Executable)+" · query "+safeTerminalInlineText(fixed.Publication.Provider.Query.Executable)+" · close "+safeTerminalInlineText(fixed.Publication.Provider.Close.Executable), "认证引用："+safeTerminalInlineText(strings.Join(fixed.Publication.AuthenticationRefs, ", ")))
	for _, tool := range []struct {
		name    string
		command steerline.Command
	}{{"Git", fixed.Publication.Git}, {"Query", fixed.Publication.Provider.Query}, {"Create", fixed.Publication.Create}, {"Close", fixed.Publication.Provider.Close}} {
		description := tool.command.Executable + " " + strings.Join(tool.command.Args, " ")
		if tool.command.Executable == "" {
			description = "无宿主命令"
		}
		lines = append(lines, "冻结 "+tool.name+"："+safeTerminalInlineText(strings.TrimSpace(description)))
	}

	lines = append(lines, fmt.Sprintf("项目要求：读取 %s · 写入 %s · Git %t · 网络 %s · 越权 %s", fixed.ExecutionPolicy.Filesystem.ReadScope, fixed.ExecutionPolicy.Filesystem.WriteScope, fixed.ExecutionPolicy.Filesystem.GitMetadataWrite, fixed.ExecutionPolicy.Network, fixed.ExecutionPolicy.Escalation), "受理版本："+safeTerminalInlineText(fixed.AcceptedConfigVersion)+" · 来源："+safeTerminalInlineText(fixed.PolicySource))
	if state.snapshot.Domain != nil {
		for _, execution := range state.snapshot.Domain.Executions {
			if execution.ExecutionID != run.ExecutionID {
				continue
			}
			status := "资源与工具未核验"
			if resource := execution.PolicyResources; resource != nil {
				status = "原生设置已核验；工具未核验"
				lines = append(lines, "原生版本："+safeTerminalInlineText(resource.NativeVersion)+" · 写根："+safeTerminalInlineText(strings.Join(resource.WriteRoots, ", ")), "临时写根：无（隐式临时目录已排除）")
			}
			if execution.Environment != nil && execution.Environment.Ready {
				status = "原会话工具与准备结算已核验"
			}
			lines = append(lines, "权限核验："+status)
		}
	}
	if fixed.Agent == "codex" {
		lines = append(lines, "冻结权限："+safeTerminalInlineText(fixed.Sandbox)+" · Windows "+safeTerminalInlineText(fixed.WindowsSandbox)+" · Approval "+safeTerminalInlineText(fixed.ApprovalPolicy))
		if state.snapshot.Domain != nil {
			for _, execution := range state.snapshot.Domain.Executions {
				if execution.ExecutionID != run.ExecutionID || execution.Environment == nil {
					continue
				}
				status := "准备未核实"
				if execution.Environment.Ready {
					status = "工具探针与原生结算已核实"
				}
				lines = append(lines, "环境："+status)
				if blocker := execution.Environment.Blocker; blocker != nil {
					lines = append(lines, "宿主阻断："+safeTerminalInlineText(blocker.Fact.Reason)+" · "+safeTerminalInlineText(blocker.Fact.Confidence)+" · "+safeTerminalInlineText(blocker.Fact.Source))
				}
			}
		}
	}
	return lines
}

func renderRunExecutionProfileSummary(state projectState, run *steerline.Run) []string {
	fixed := fixedRunExecutionProfile(state, run)
	if fixed == nil {
		return nil
	}
	config := steerline.Config{Host: fixed.Host, Agent: fixed.Agent, Model: fixed.Model, ReasoningEffort: fixed.ReasoningEffort}
	return []string{"执行方案：" + safeTerminalInlineText(executionHostLabel(config)) + " · " + safeTerminalInlineText(executionAgentName(config)) + " · " + safeTerminalInlineText(executionModelLabel(config)) + " · " + safeTerminalInlineText(executionEffortLabel(config))}
}

func fixedRunExecutionProfile(state projectState, run *steerline.Run) *steerline.FixedExecutionProfile {
	if run == nil {
		return nil
	}
	workflows := []steerline.WorkflowRunRecord(nil)
	if state.snapshot.Domain != nil {
		workflows = state.snapshot.Domain.WorkflowRuns
	}
	for _, workflow := range workflows {
		if workflow.WorkflowRunID != run.RunID || workflow.ExecutionProfile == nil {
			continue
		}
		return workflow.ExecutionProfile
	}
	return nil
}
