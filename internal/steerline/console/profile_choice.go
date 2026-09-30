package console

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/erranliu/SteerLine/internal/steerline"
)

func (m Model) handleExecutionProfileChoice(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	choice := m.profileChoice
	if choice == nil {
		return m, nil
	}
	index := m.projectIndex(choice.projectKey)
	if index < 0 || index != m.selected {
		m.profileChoice = nil
		m.status = "项目已切换；执行配置选择已取消"
		return m, nil
	}
	state := &m.projects[index]
	profiles := state.ref.Config.AvailableExecutionProfiles()
	switch key.String() {
	case "esc", "q":
		m.profileChoice = nil
		return m, nil
	case "up", "k":
		choice.index = (choice.index - 1 + len(profiles)) % len(profiles)
		return m, nil
	case "down", "j":
		choice.index = (choice.index + 1) % len(profiles)
		return m, nil
	case "enter":
		if choice.index < 0 || choice.index >= len(profiles) {
			m.status = "执行配置选择无效"
			return m, nil
		}
		profile := profiles[choice.index]
		if reason := profile.StaticUnavailableReason(); reason != "" {
			m.status = reason
			return m, nil
		}
		runtime := state.snapshot.Connection.Snapshot.Runtime
		row := m.selectedRow(index)
		if runtime == nil || runtime.Service.InstanceID != choice.instance || runtime.Control.Revision != choice.revision || row == nil || row.kind != rowReady || row.taskID != choice.taskID {
			m.profileChoice = nil
			m.status = "任务或服务状态已变化；请刷新后重新选择执行配置"
			return m, m.readProject(index)
		}
		if reason := candidateActionReason(state.ref, state.snapshot, m.clock.Now()); reason != "" {
			m.profileChoice = nil
			m.status = "不能开始：" + reason
			return m, nil
		}
		if _, _, err := state.ref.Config.ResolveExecutionProfile(profile.ID, profile.Digest()); err != nil {
			m.profileChoice = nil
			m.status = "执行配置已变化：" + err.Error()
			return m, nil
		}
		m.profileChoice = nil
		return m.createActionWithProfile("start_task", choice.taskID, &profile)
	}
	return m, nil
}

func renderExecutionProfileChoice(m Model, state projectState, height int) string {
	choice := m.profileChoice
	if choice == nil {
		return ""
	}
	profiles := state.ref.Config.AvailableExecutionProfiles()
	lines := []string{"开始任务 " + safeTerminalInlineText(choice.taskID) + " · 选择已配置的执行方案", ""}
	visible := max(1, height-13)
	start := max(0, choice.index-visible/2)
	if start+visible > len(profiles) {
		start = max(0, len(profiles)-visible)
	}
	end := min(len(profiles), start+visible)
	for i := start; i < end; i++ {
		profile := profiles[i]
		marker := "  "
		if i == choice.index {
			marker = "› "
		}
		line := marker + safeTerminalInlineText(profile.Name) + " [" + safeTerminalInlineText(profile.ID) + "]"
		if profile.StaticUnavailableReason() != "" {
			line += " · 不可用"
		}
		lines = append(lines, line)
	}
	profile := profiles[choice.index]
	lines = append(lines, "", "选中方案", "  位置："+safeTerminalInlineText(executionHostLabel(profileConfig(profile))),
		"  Agent："+safeTerminalInlineText(profile.Agent),
		"  Model："+safeTerminalInlineText(executionModelLabel(profileConfig(profile))),
		"  Thinking effort："+safeTerminalInlineText(executionEffortLabel(profileConfig(profile))),
		"  Sandbox："+safeTerminalInlineText(profile.Sandbox))
	lines = append(lines, fmt.Sprintf("  项目要求：读 %s / 写 %s / Git %t / 网络 %s / 越权 %s", profile.ExecutionPolicy.Filesystem.ReadScope, profile.ExecutionPolicy.Filesystem.WriteScope, profile.ExecutionPolicy.Filesystem.GitMetadataWrite, profile.ExecutionPolicy.Network, profile.ExecutionPolicy.Escalation), "  受理版本："+safeTerminalInlineText(profile.AcceptedConfigVersion), "  发布责任："+safeTerminalInlineText(profile.PublicationPolicy.Publisher), "  发布 provider："+safeTerminalInlineText(profile.Publication.Provider.Kind)+" · "+safeTerminalInlineText(profile.Publication.Provider.BaseURL)+"/"+safeTerminalInlineText(profile.Publication.Provider.Repository))
	if profile.WindowsSandbox != "" {
		lines = append(lines, "  Windows sandbox："+safeTerminalInlineText(profile.WindowsSandbox))
	}
	if profile.CodexProfile != "" {
		lines = append(lines, "  Codex profile："+safeTerminalInlineText(profile.CodexProfile))
	}
	if profile.Host == "orca" && profile.Agent == "codex" {
		lines = append(lines, "  Windows sandbox："+safeTerminalInlineText(profile.WindowsSandbox), "  Approval policy：never")
	}
	if reason := profile.StaticUnavailableReason(); reason != "" {
		lines = append(lines, "  不可用："+safeTerminalText(reason))
	}
	if profile.Host == "orca_supervised" && profile.Orca.Supervised != nil {
		lines = append(lines, "  目标环境："+safeTerminalInlineText(profile.Orca.Supervised.EnvironmentID), "  连接："+safeTerminalInlineText(profile.Orca.Supervised.ConnectionID))
	}
	if executionConfigMismatch(state.ref, state.snapshot) {
		lines = append(lines, "", executionConfigChanged)
	} else {
		lines = append(lines, "", "服务会在领取前核验目标能力；未验证的方案不会启动执行。")
	}
	return strings.Join(fitLines(wrapLines(lines, m.width), m.width, height), "\n")
}

func profileConfig(profile steerline.ExecutionProfile) steerline.Config {
	return steerline.Config{Host: profile.Host, Agent: profile.Agent, Model: profile.Model, ReasoningEffort: profile.ReasoningEffort, Sandbox: profile.Sandbox, WindowsSandbox: profile.WindowsSandbox, Orca: profile.Orca}
}
