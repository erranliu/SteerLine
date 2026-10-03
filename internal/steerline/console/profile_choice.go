package console

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	profiles := state.ref.Presentation.Profiles
	if len(profiles) == 0 {
		m.profileChoice = nil
		m.status = "没有可用的执行方案"
		return m, nil
	}
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
		if reason := profile.UnavailableReason; reason != "" {
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
		if reason := candidateActionReason(*state, m.clock.Now()); reason != "" {
			m.profileChoice = nil
			m.status = "不能开始：" + reason
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
	profiles := state.ref.Presentation.Profiles
	if choice.index < 0 || choice.index >= len(profiles) {
		return "执行方案不可用；请刷新"
	}
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
		if profile.UnavailableReason != "" {
			line += " · 不可用"
		}
		lines = append(lines, line)
	}
	profile := profiles[choice.index]
	lines = append(lines, "", "选中方案", "  位置："+safeTerminalInlineText(profile.Host),
		"  Agent："+safeTerminalInlineText(profile.Agent))
	lines = append(lines, fmt.Sprintf("  项目要求：读 %s / 写 %s / Git %t / 网络 %s / 越权 %s", profile.ExecutionPolicy.Filesystem.ReadScope, profile.ExecutionPolicy.Filesystem.WriteScope, profile.ExecutionPolicy.Filesystem.GitMetadataWrite, profile.ExecutionPolicy.Network, profile.ExecutionPolicy.Escalation), "  受理版本："+safeTerminalInlineText(profile.AcceptedConfigVersion), "  发布责任："+safeTerminalInlineText(profile.PublicationPolicy.Publisher), "  发布 provider："+safeTerminalInlineText(profile.Publication.Provider.Kind)+" · "+safeTerminalInlineText(profile.Publication.Provider.BaseURL)+"/"+safeTerminalInlineText(profile.Publication.Provider.Repository))
	for _, field := range profile.ImplementationDisplay {
		lines = append(lines, "  实现设置："+safeTerminalInlineText(field))
	}
	if reason := profile.UnavailableReason; reason != "" {
		lines = append(lines, "  不可用："+safeTerminalText(reason))
	}
	if executionConfigMismatch(state.ref, state.snapshot) {
		lines = append(lines, "", executionConfigChanged)
	} else {
		lines = append(lines, "", "服务会在领取前核验目标能力；未验证的方案不会启动执行。")
	}
	return strings.Join(fitLines(wrapLines(lines, m.width), m.width, height), "\n")
}
