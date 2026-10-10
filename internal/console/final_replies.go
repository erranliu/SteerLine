package console

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) finalQueryScope(index int) (full bool, attempt, turn string) {
	full = m.detailExpanded && m.detailSection == "final"
	if !full {
		if row := m.selectedRow(index); row != nil && row.run != nil {
			attempt, turn = row.run.AttemptID, row.run.CurrentTurnID
			if row.run.StepID == "implement.report" || row.run.StepID == "publish.report" {
				sourceStep := "implement.execute"
				if row.run.StepID == "publish.report" {
					sourceStep = "publish"
				}
				for i := len(row.run.StageResults) - 1; i >= 0; i-- {
					if row.run.StageResults[i].StepID == sourceStep {
						turn = row.run.StageResults[i].TurnID
						break
					}
				}
			}
		}
	}
	return
}

func (m Model) readFinalReplies(index int, resetPin bool) tea.Cmd {
	state := &m.projects[index]
	full, attempt, turn := m.finalQueryScope(index)
	return state.final.read(m.ctx, state.client, m.clock.Now(), finalReplyRoute{state.ref.Key, state.ref.Identity}, full, attempt, turn, resetPin)
}

func (m Model) updateFinalReplies(msg finalRepliesMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	state := &m.projects[index]
	cmd, bodySelected := state.final.acceptReplies(m.ctx, state.client, finalReplyRoute{state.ref.Key, state.ref.Identity}, m.detailExpanded && m.detailSection == "final", msg)
	if bodySelected {
		state.detailScroll = 0
	}
	return m, cmd
}

func (m Model) updateBody(msg bodyMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index >= 0 {
		state := &m.projects[index]
		state.final.acceptBody(state.ref.Identity, msg)
	}
	return m, nil
}

func (m Model) cycleFinal(delta int) (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	cmd, cycled := state.final.cycle(m.ctx, state.client, finalReplyRoute{state.ref.Key, state.ref.Identity}, delta)
	if !cycled {
		m.status = "最终回复尚未采集或来源未识别"
		return m, nil
	}
	state.detailScroll = 0
	return m, cmd
}
