package console

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleExecutionProfileChoice(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.String() == "f8" {
		m.operationDetails = true
		m.expandedDetailScroll = 0
		return m, nil
	}
	if next, cmd, handled := m.handleWorkspaceNavigation(key); handled {
		return next, cmd
	}
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
	profiles := choiceProfiles(m, *state)
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
		if state.focus == 0 {
			state.focus = 1
			return m, nil
		}
		if !m.operationVisible() {
			m.status = "窗口过小，请调整后操作；未开始"
			return m, nil
		}
		if !profileChoiceMatches(choice, *state, m.selectedRow(index)) {
			m.profileChoice = nil
			m.status = "原任务/方案/版本已变化，请重新选择"
			return m, nil
		}
		if choice.index < 0 || choice.index >= len(profiles) {
			m.status = "执行配置选择无效"
			return m, nil
		}
		profile := profiles[choice.index]
		if reason := profile.UnavailableReason; reason != "" {
			m.status = reason
			return m, nil
		}
		runtime := state.snapshot.Connection.Snapshot.Page
		row := m.selectedRow(index)
		if choice.action == "start_candidate" {
			if runtime == nil || runtime.Service.InstanceID != choice.instance || runtime.Control.Revision != choice.revision || row == nil || row.taskID != choice.taskID {
				m.profileChoice = nil
				m.status = "原任务或服务版本已变化，请重新选择"
				return m, nil
			}
			if reason := candidateStartReason(*state, row, m.clock.Now()); reason != "" {
				m.status = reason
				return m, nil
			}
			p := api.ControlPayload{TaskID: choice.taskID, ExpectedTaskVersion: row.native.Version, ExpectedGraphRevision: state.snapshot.NativeTasks.GraphRevision, ExecutionProfileID: profile.ID, ExecutionProfileDigest: profile.Digest}
			m.profileChoice = nil
			return m.submitCandidateAction("start_candidate", p, newRequestID(), choice.instance, choice.revision)
		}
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
	profiles := choiceProfiles(m, state)
	if choice.index < 0 || choice.index >= len(profiles) {
		return "执行方案不可用；请刷新"
	}
	lines := []string{"运行与反馈 · 选择执行方案"}
	required := profileChoiceRequired(m, state)
	lines = append(lines, required[:len(required)-1]...)
	visible := max(1, height-len(required)-1)
	start := max(0, min(choice.index-visible/2, len(profiles)-visible))
	for i := start; i < min(len(profiles), start+visible); i++ {
		marker := "  "
		if i == choice.index {
			marker = "▸ "
		}
		line := marker + safeTerminalInlineText(profiles[i].Name+" ["+profiles[i].ID+"]")
		if i == choice.index {
			line = styleSelected.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, required[len(required)-1])
	return strings.Join(fitLines(lines, m.width, height), "\n")
}
