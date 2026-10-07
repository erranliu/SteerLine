package console

import tea "github.com/charmbracelet/bubbletea"

func (m Model) scrollPanel(delta int) (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	_, height := m.lowerGeometry()
	raw := feedbackLines(m, *state, m.selectedRow(m.selected))
	if state.recoveryActive {
		raw = recoverySummary(*state, m.selectedRow(m.selected))
	}
	lines := wrapLines(raw, max(1, detailPanelWidth(m)-2))
	state.lowerScroll = min(max(0, state.lowerScroll+delta), max(0, len(lines)-height))
	return m, nil
}

func (m Model) scrollExpandedDetail(delta int) (tea.Model, tea.Cmd) {
	state := m.projects[m.selected]
	state.detailScroll = 0
	lines := renderDetail(m, state, m.selectedRow(m.selected), m.width, 1<<20)
	m.expandedDetailScroll = min(max(0, m.expandedDetailScroll+delta), max(0, len(lines)-consoleContentHeight(m)))
	return m, nil
}
