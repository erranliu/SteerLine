package console

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateFinalPreview(msg finalPreviewMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	state := &m.projects[index]
	target := finalPreviewTarget{}
	if row := m.selectedRow(index); row != nil && row.run != nil {
		target = finalPreviewTarget{candidateIdentity(row.run), row.run.AttemptID, row.run.CurrentTurnID}
	}
	state.final.acceptPreview(state.ref.Identity, target, msg)
	return m, nil
}

func (m Model) readSelectedFinalFull() tea.Cmd { return m.readFinalReplies(m.selected, false) }
