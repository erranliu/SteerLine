package console

import (
	"fmt"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

const executionConfigChanged = api.ExecutionConfigChanged

func renderRequirementsEvaluation(e *executionbackend.RequirementsEvaluation, label string) []string {
	if e == nil {
		return nil
	}
	lines := []string{label + "：" + e.CheckedAt.Format("2006-01-02 15:04:05Z07:00") + " · " + safeTerminalInlineText(e.Target.ID) + "（工具/引用存在性，不证明沙箱权限或远端授权）"}
	if e.Capabilities != nil {
		for _, cap := range e.Capabilities.Capabilities {
			lines = append(lines, "能力："+safeTerminalInlineText(cap.Name)+" · "+safeTerminalInlineText(string(cap.Status))+" · 版本 "+safeTerminalInlineText(e.Capabilities.PlatformVersion))
		}
	}
	for _, values := range [][]executionbackend.RequirementEvidence{e.Tools, e.Credentials} {
		for _, item := range values {
			lines = append(lines, "需求证据："+safeTerminalInlineText(item.Site+"/"+item.ID)+" · "+safeTerminalInlineText(string(item.Status))+" · "+safeTerminalInlineText(item.Version)+" · "+safeTerminalInlineText(item.Source))
		}
	}
	return lines
}

func executionConfigMismatch(ref ProjectRef, snapshot ProjectSnapshot) bool {
	return api.ExecutionConfigMismatch(ref, snapshot)
}

func renderFixedRunExecutionProfile(state projectState, run *contracts.Run) []string {
	fixed := fixedRunExecutionProfile(state, run)
	if fixed == nil {
		return nil
	}
	lines := []string{
		"执行方案：" + safeTerminalInlineText(fixed.Name) + " [" + safeTerminalInlineText(fixed.ID) + "] · " + safeTerminalInlineText(fixed.Host),
		"Agent：" + safeTerminalInlineText(fixed.Agent),
	}
	lines = append(lines, "发布责任："+safeTerminalInlineText(fixed.PublicationPolicy.Publisher), "冻结发布目标："+safeTerminalInlineText(fixed.Publication.Provider.Kind)+" · "+safeTerminalInlineText(fixed.Publication.Provider.BaseURL)+"/"+safeTerminalInlineText(fixed.Publication.Provider.Repository)+" · base "+safeTerminalInlineText(fixed.Publication.BaseBranch), "Git remote："+safeTerminalInlineText(fixed.Publication.Remote)+" · "+safeTerminalInlineText(fixed.Publication.RemoteURL), "发布工具："+safeTerminalInlineText(fixed.Publication.Git.Executable)+" · query "+safeTerminalInlineText(fixed.Publication.Provider.Query.Executable)+" · close "+safeTerminalInlineText(fixed.Publication.Provider.Close.Executable), "认证引用："+safeTerminalInlineText(strings.Join(fixed.Publication.AuthenticationRefs, ", ")))
	for _, tool := range []struct {
		name    string
		command executionbackend.Command
	}{{"Git", fixed.Publication.Git}, {"Query", fixed.Publication.Provider.Query}, {"Close", fixed.Publication.Provider.Close}} {
		description := tool.command.Executable + " " + strings.Join(tool.command.Args, " ")
		if tool.command.Executable == "" {
			description = "无宿主命令"
		}
		lines = append(lines, "冻结 "+tool.name+"："+safeTerminalInlineText(strings.TrimSpace(description)))
	}

	lines = append(lines, fmt.Sprintf("项目要求：读取 %s · 写入 %s · Git %t · 网络 %s · 越权 %s", fixed.ExecutionPolicy.Filesystem.ReadScope, fixed.ExecutionPolicy.Filesystem.WriteScope, fixed.ExecutionPolicy.Filesystem.GitMetadataWrite, fixed.ExecutionPolicy.Network, fixed.ExecutionPolicy.Escalation), "受理版本："+safeTerminalInlineText(fixed.AcceptedConfigVersion)+" · 来源："+safeTerminalInlineText(fixed.PolicySource))
	lines = append(lines, "附加能力："+safeTerminalInlineText(strings.Join(fixed.ExecutionRequirements.Capabilities, ", ")))
	for _, field := range fixed.ImplementationDisplay {
		lines = append(lines, "实现配置："+safeTerminalInlineText(field))
	}
	for _, tool := range fixed.ExecutionRequirements.Tools {
		version := tool.ExactVersion
		if version == "" {
			version = "存在即可"
		}
		lines = append(lines, "必需工具："+safeTerminalInlineText(tool.ID)+" · "+safeTerminalInlineText(version))
	}
	lines = append(lines, renderRequirementsEvaluation(fixed.RequirementsEvaluation, "领取时需求检查")...)
	if state.snapshot.Domain != nil {
		for _, execution := range state.snapshot.Domain.Executions {
			if execution.ExecutionID != run.ExecutionID {
				continue
			}
			lines = append(lines, renderRequirementsEvaluation(execution.CurrentRequirementsEvaluation, "当前需求检查")...)
			if execution.CurrentRequirementsError != nil {
				lines = append(lines, "需求阻断："+safeTerminalInlineText(*execution.CurrentRequirementsError))
			}
			status := "资源与工具未核验"
			if resource := execution.PolicyResources; resource != nil {
				status = "原生设置已核验；工具未核验"
				lines = append(lines, "已绑定写根："+safeTerminalInlineText(strings.Join(resource.WriteRoots, ", ")), "临时写根：无（隐式临时目录已排除）")
			}
			if execution.Environment != nil && execution.Environment.Ready {
				status = "原会话工具与准备结算已核验"
			}
			lines = append(lines, "权限核验："+status)
		}
	}
	if state.snapshot.Domain != nil {
		for _, execution := range state.snapshot.Domain.Executions {
			if execution.ExecutionID != run.ExecutionID || execution.Environment == nil {
				continue
			}
			status := "准备未核实"
			if execution.Environment.Ready {
				status = "准备结算已核实"
			}
			lines = append(lines, "环境："+status)
			if blocker := execution.Environment.Blocker; blocker != nil {
				lines = append(lines, "宿主阻断："+safeTerminalInlineText(blocker.Fact.Reason)+" · "+safeTerminalInlineText(blocker.Fact.Confidence)+" · "+safeTerminalInlineText(blocker.Fact.Source))
			}
		}
	}
	return lines
}

func renderRunExecutionProfileSummary(state projectState, run *contracts.Run) []string {
	fixed := fixedRunExecutionProfile(state, run)
	if fixed == nil {
		return nil
	}
	return []string{"执行方案：" + safeTerminalInlineText(fixed.Host) + " · " + safeTerminalInlineText(fixed.Agent) + " · " + safeTerminalInlineText(strings.Join(fixed.ImplementationDisplay, " · "))}
}

func fixedRunExecutionProfile(state projectState, run *contracts.Run) *executionmodel.FixedExecutionProfile {
	if run == nil {
		return nil
	}
	if run.ExecutionProfile != nil {
		return run.ExecutionProfile
	}
	workflows := []executionmodel.WorkflowRunRecord(nil)
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
