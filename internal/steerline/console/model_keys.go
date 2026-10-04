package console

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.profileChoice != nil {
		return m.handleExecutionProfileChoice(key)
	}
	if m.answer != nil {
		return m.handleDecisionAnswerKey(key)
	}
	if m.confirm != nil {
		return m.handleConfirm(key)
	}
	if m.help {
		if key.String() == "?" || key.String() == "esc" {
			m.help = false
		}
		return m, nil
	}
	if m.selected < 0 || m.selected >= len(m.projects) {
		if key.String() == "q" || key.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	state := &m.projects[m.selected]
	if key.String() == "ctrl+e" {
		return m.createAction("recheck_environment", "")
	}
	if key.String() == "ctrl+o" {
		return m.createAction("open_terminal", "")
	}
	if m.detailExpanded {
		switch key.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "d", "enter", "esc":
			m.detailExpanded = false
			return m, nil
		case "up", "k":
			return m.scrollExpandedDetail(-1)
		case "down", "j":
			return m.scrollExpandedDetail(1)
		case "pgup":
			return m.scrollExpandedDetail(-8)
		case "pgdown":
			return m.scrollExpandedDetail(8)
		case "s", "a", "m", "p", "x", "i", "{", "}", "b", "z":
			m.detailExpanded = false
		default:
			return m, nil
		}
	}
	if state.panel == 6 {
		switch key.String() {
		case "esc":
			state.panel = 0
			m.saveProjectView(m.selected)
			return m, nil
		case "r":
			return m.inspectSelectedRecovery()
		case "[", "]":
			if state.recoveryReport != nil && len(state.recoveryReport.Blockers) > 0 {
				count := len(state.recoveryReport.Blockers)
				if key.String() == "]" {
					state.recoveryBlockerIndex = (state.recoveryBlockerIndex + 1) % count
				} else {
					state.recoveryBlockerIndex = (state.recoveryBlockerIndex - 1 + count) % count
				}
			}
			return m, nil
		}
	}
	switch key.String() {
	case "{":
		return m.cycleCandidate(-1)
	case "}":
		return m.cycleCandidate(1)
	case "b":
		return m.startAdditionalCandidate()
	case "z":
		return m.prepareCandidateAbandon()
	case "q", "ctrl+c":
		m.saveProjectView(m.selected)
		return m, tea.Quit
	case "left", "h":
		return m.moveProject(-1)
	case "right", "l":
		return m.moveProject(1)
	case "tab", "shift+tab":
		state.focus = 1 - state.focus
		return m, nil
	case "up", "k":
		if state.focus == 1 {
			return m.scrollPanel(-1)
		}
		return m.moveSelection(-1)
	case "down", "j":
		if state.focus == 1 {
			return m.scrollPanel(1)
		}
		return m.moveSelection(1)
	case "r":
		state.lastRefresh = time.Time{}
		return m, m.readProject(m.selected)
	case "enter":
		m.detailExpanded = true
		m.expandedDetailScroll = 0
		return m, nil
	case "d":
		m.detailExpanded = true
		m.expandedDetailScroll = 0
		return m, nil
	case "t":
		state.focus = 0
		return m, nil
	case "a":
		if row := m.selectedRow(m.selected); row != nil && row.run != nil && row.run.Decision != nil && row.run.Decision.Status == "pending" {
			return m.answerSelectedDecision()
		}
		state.panel = 1
		state.focus = 1
		m.saveProjectView(m.selected)
		return m, nil
	case "A":
		state.panel = 1
		state.focus = 1
		m.saveProjectView(m.selected)
		return m, nil
	case "c":
		state.panel = 2
		state.focus = 1
		m.saveProjectView(m.selected)
		return m, nil
	case "f":
		state.panel = 3
		state.focus = 1
		m.finalIndex = 0
		m.saveProjectView(m.selected)
		return m, nil
	case "u":
		return m.inspectSelectedRecovery()
	case "v":
		state.panel = 5
		state.focus = 1
		m.saveProjectView(m.selected)
		return m, nil
	case "1":
		return m.changeFilter(0)
	case "2":
		return m.changeFilter(1)
	case "3":
		return m.changeFilter(2)
	case "4":
		return m.changeFilter(3)
	case "5":
		return m.changeFilter(4)
	case "6":
		return m.changeFilter(5)
	case "7":
		return m.changeFilter(6)
	case "[", "]":
		return m.cycleAttempt(key.String() == "]")
	case "s":
		return m.startSelected()
	case "m":
		return m.toggleMode()
	case "x":
		return m.prepareStop()
	case "e":
		return m.toggleGroup()
	case ",":
		return m.cycleGroup(-1)
	case ".":
		return m.cycleGroup(1)
	case "=":
		return m.keepGroupOpen()
	case "n":
		return m.nextFinal()
	case "o":
		return m.openProcessBody()
	case "y":
		return m.loadMoreConversation()
	case "<":
		return m.pageBody(false)
	case ">":
		return m.pageBody(true)
	case "g":
		if state.panel == 0 || state.panel == 1 {
			lines := allActivityLines(*state, m.selectedRow(m.selected), activityPanelWidth(m))
			height := activityPanelHeight(m)
			state.logScroll = max(0, len(lines)-height)
		}
		state.view.Follow = false
		m.saveProjectView(m.selected)
		return m, nil
	case "G":
		state.view.Follow = true
		state.logScroll = 0
		m.status = "已跳到最新"
		m.saveProjectView(m.selected)
		var cmd tea.Cmd
		m, cmd = m.refreshSelectedLogs(m.selected)
		return m, cmd
	case "pgup":
		if state.focus == 0 {
			return m.moveSelection(-8)
		}
		return m.scrollPanel(-8)
	case "pgdown":
		if state.focus == 0 {
			return m.moveSelection(8)
		}
		return m.scrollPanel(8)
	case "?":
		m.help = true
		return m, nil
	}
	return m, nil
}

func (m Model) handleDecisionAnswerKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	dialog := m.answer
	if dialog == nil {
		return m, nil
	}
	keyName := key.String()
	defer m.saveDecisionDraft()
	switch keyName {
	case "esc", "ctrl+c":
		m.saveDecisionDraft()
		m.answer = nil
		m.status = "已关闭答案草稿；答案未提交"
		return m, nil
	case "f2":
		return m.submitDecisionAnswer()
	case "tab":
		dialog.noteFocus = !dialog.noteFocus
		dialog.error = ""
		return m, nil
	case "pgup":
		dialog.scroll = max(0, dialog.scroll-8)
		return m, nil
	case "pgdown":
		dialog.scroll += 8
		return m, nil
	case "up", "down":
		if dialog.noteFocus {
			dialog.cursor = moveRuneCursorVertical(dialog.note, dialog.cursor, keyName == "down")
			return m, nil
		}
		state := &m.projects[m.selected]
		request, ok := decisionRequestFor(*state, dialog.runID, dialog.decisionID)
		if !ok {
			dialog.error = "原问题详情暂不可读；草稿保留"
			return m, nil
		}
		count := len(request.Options)
		if request.AllowFreeText {
			count++
		}
		if count > 0 {
			if keyName == "down" {
				dialog.optionIndex = (dialog.optionIndex + 1 + count) % count
			} else {
				dialog.optionIndex = (dialog.optionIndex - 1 + count) % count
			}
		}
		dialog.error = ""
		return m, nil
	case "left":
		if dialog.noteFocus && dialog.cursor > 0 {
			dialog.cursor--
		}
		return m, nil
	case "right":
		if dialog.noteFocus && dialog.cursor < len(dialog.note) {
			dialog.cursor++
		}
		return m, nil
	case "home":
		if dialog.noteFocus {
			dialog.cursor = runeLineStart(dialog.note, dialog.cursor)
		}
		return m, nil
	case "end":
		if dialog.noteFocus {
			dialog.cursor = runeLineEnd(dialog.note, dialog.cursor)
		}
		return m, nil
	case "backspace":
		if dialog.noteFocus && dialog.cursor > 0 {
			dialog.note = append(dialog.note[:dialog.cursor-1], dialog.note[dialog.cursor:]...)
			dialog.cursor--
		}
		dialog.error = ""
		return m, nil
	case "delete":
		if dialog.noteFocus && dialog.cursor < len(dialog.note) {
			dialog.note = append(dialog.note[:dialog.cursor], dialog.note[dialog.cursor+1:]...)
		}
		dialog.error = ""
		return m, nil
	case "enter":
		if !dialog.noteFocus {
			dialog.noteFocus = true
			return m, nil
		}
		insertAnswerRunes(dialog, []rune{'\n'})
		dialog.error = ""
		return m, nil
	}
	if !dialog.noteFocus {
		if key.Type == tea.KeyRunes && len(key.Runes) == 1 && key.Runes[0] >= '1' && key.Runes[0] <= '9' {
			state := &m.projects[m.selected]
			request, ok := decisionRequestFor(*state, dialog.runID, dialog.decisionID)
			if ok && int(key.Runes[0]-'1') < len(request.Options) {
				dialog.optionIndex = int(key.Runes[0] - '1')
			}
		}
		return m, nil
	}
	if key.Type == tea.KeyRunes && len(key.Runes) > 0 {
		insertAnswerRunes(dialog, key.Runes)
		dialog.error = ""
	}
	return m, nil
}

func insertAnswerRunes(dialog *decisionAnswerDialog, runes []rune) {
	if dialog == nil || len(runes) == 0 {
		return
	}
	insert := strings.ReplaceAll(string(runes), "\r\n", "\n")
	insert = strings.ReplaceAll(insert, "\r", "\n")
	filtered := make([]rune, 0, len(insert))
	for _, char := range insert {
		if char == '\n' || char == '\t' || char >= 0x20 && char != 0x7f {
			filtered = append(filtered, char)
		}
	}
	if len(filtered) == 0 {
		return
	}
	joined := make([]rune, 0, len(dialog.note)+len(filtered))
	joined = append(joined, dialog.note[:dialog.cursor]...)
	joined = append(joined, filtered...)
	joined = append(joined, dialog.note[dialog.cursor:]...)
	dialog.note = joined
	dialog.cursor += len(filtered)
}

func runeLineStart(text []rune, cursor int) int {
	if cursor > len(text) {
		cursor = len(text)
	}
	for cursor > 0 && text[cursor-1] != '\n' {
		cursor--
	}
	return cursor
}

func runeLineEnd(text []rune, cursor int) int {
	if cursor > len(text) {
		cursor = len(text)
	}
	for cursor < len(text) && text[cursor] != '\n' {
		cursor++
	}
	return cursor
}

func moveRuneCursorVertical(text []rune, cursor int, down bool) int {
	start, end := runeLineStart(text, cursor), runeLineEnd(text, cursor)
	column := cursor - start
	if down {
		if end >= len(text) {
			return cursor
		}
		nextStart := end + 1
		nextEnd := runeLineEnd(text, nextStart)
		return min(nextStart+column, nextEnd)
	}
	if start == 0 {
		return cursor
	}
	previousEnd := start - 1
	previousStart := runeLineStart(text, previousEnd)
	return min(previousStart+column, previousEnd)
}

func (m Model) handleConfirm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	confirm := m.confirm
	switch key.String() {
	case "up":
		confirm.scroll = max(0, confirm.scroll-1)
	case "down":
		confirm.scroll++
	case "pgup":
		confirm.scroll = max(0, confirm.scroll-8)
	case "pgdown":
		confirm.scroll += 8
	case "esc", "n":
		m.confirm = nil
	case "enter", "y":
		if confirm.action == "abandon_candidate" {
			s, rt := m.readyControlTarget()
			row := m.selectedRow(m.selected)
			if rt == nil {
				return m, nil
			}
			if s.ref.Key != confirm.projectKey || rt.Service.InstanceID != confirm.instance || rt.Control.Revision != confirm.revision || row == nil || row.run == nil || row.run.RunID != confirm.runID || row.run.ControlRevision != confirm.payload.ExpectedCandidateVersion || row.native == nil || row.native.Version != confirm.payload.ExpectedTaskVersion {
				m.confirm = nil
				m.status = "原候选或版本已变化；未提交放弃"
				return m, nil
			}
			if reason := candidateAbandonReason(*s, row); reason != "" {
				m.confirm = nil
				m.status = reason
				return m, nil
			}
			m.confirm = nil
			return m.submitCandidateAction(confirm.action, confirm.payload, confirm.requestID, confirm.instance, confirm.revision)
		}
		m.confirm = nil
		return m.beginAction(confirm.projectKey, confirm.instance, confirm.revision, confirm.action, confirm.taskID, confirm.requestID)
	}
	return m, nil
}
