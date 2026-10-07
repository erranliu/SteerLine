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
	if m.confirm != nil {
		return m.handleConfirm(key)
	}
	if m.help {
		if key.String() == "?" || key.String() == "esc" {
			m.help = false
		}
		return m, nil
	}
	if m.answer != nil {
		switch key.String() {
		case "f6", "f7":
			m.leaveAnswer()
			delta := 1
			if key.String() == "f6" {
				delta = -1
			}
			return m.cycleCandidate(delta)
		case "tab", "shift+tab":
			if m.answer.details {
				return m, nil
			}
			state := &m.projects[m.selected]
			delta := 1
			if key.String() == "shift+tab" {
				delta = -1
			}
			state.focus = (state.focus + delta + 3) % 3
			if len(m.answer.request.Options) == 0 && state.focus == 1 {
				state.focus = (state.focus + delta + 3) % 3
			}
			m.answer.noteFocus = state.focus == 2
			return m, nil
		case "f2":
			if !m.answer.details && compactLayout(m) && m.projects[m.selected].focus == 0 {
				m.projects[m.selected].focus = 1
				if m.answer.noteFocus {
					m.projects[m.selected].focus = 2
				}
				m.status = "已返回原答案草稿；核对预览后按 F2 提交"
				return m, nil
			}
			return m.handleDecisionAnswerKey(key)
		case "f8", "esc", "ctrl+c":
			return m.handleDecisionAnswerKey(key)
		case "ctrl+e":
			return m.createAction("recheck_environment", "")
		case "ctrl+o":
			return m.createAction("open_terminal", "")
		}
		if m.answer.details || m.projects[m.selected].focus != 0 {
			return m.handleDecisionAnswerKey(key)
		}
	}
	if m.selected < 0 || m.selected >= len(m.projects) {
		if key.String() == "q" || key.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	state := &m.projects[m.selected]
	if m.detailExpanded {
		switch key.String() {
		case "esc", "d", "enter":
			m.detailExpanded = false
			m.detailSection = ""
			return m, nil
		case "up", "k":
			return m.scrollExpandedDetail(-1)
		case "down", "j":
			return m.scrollExpandedDetail(1)
		case "pgup":
			return m.scrollExpandedDetail(-8)
		case "pgdown":
			return m.scrollExpandedDetail(8)
		}
	}
	switch key.String() {
	case "q", "ctrl+c":
		m.saveProjectView(m.selected)
		return m, tea.Quit
	case "ctrl+e":
		return m.createAction("recheck_environment", "")
	case "ctrl+o":
		return m.createAction("open_terminal", "")
	case "{", "f6":
		return m.cycleCandidate(-1)
	case "}", "f7":
		return m.cycleCandidate(1)
	case "b":
		m.detailExpanded = false
		return m.startAdditionalCandidate()
	case "z":
		return m.prepareCandidateAbandon()
	case "left", "h":
		m.detailExpanded = false
		return m.moveProject(-1)
	case "right", "l":
		m.detailExpanded = false
		return m.moveProject(1)
	case "tab", "shift+tab":
		if state.focus == 0 {
			state.focus = 1
		} else {
			state.focus = 0
		}
		return m, nil
	case "up", "k":
		if state.focus != 0 {
			return m.scrollPanel(-1)
		}
		return m.moveSelection(-1)
	case "down", "j":
		if state.focus != 0 {
			return m.scrollPanel(1)
		}
		return m.moveSelection(1)
	case "pgup":
		if state.focus != 0 {
			return m.scrollPanel(-8)
		}
		return m.moveSelection(-8)
	case "pgdown":
		if state.focus != 0 {
			return m.scrollPanel(8)
		}
		return m.moveSelection(8)
	case "r":
		state.lastRefresh = time.Time{}
		state.finalReadAt = time.Time{}
		return m, m.readProject(m.selected)
	case "enter":
		if row := m.selectedRow(m.selected); row != nil && row.run != nil && row.run.Decision != nil && row.run.Decision.Status == "pending" && row.run.Abandonment == nil && row.run.TaskStatus != "closed" && !feedbackInProgress(*state, row.run) {
			return m.answerSelectedDecision()
		}
		m.detailExpanded = true
		m.detailSection = ""
		m.expandedDetailScroll = 0
		return m, nil
	case "d":
		m.leaveAnswer()
		m.detailExpanded = true
		m.detailSection = ""
		m.expandedDetailScroll = 0
		return m, nil
	case "f":
		m.leaveAnswer()
		m.detailExpanded = true
		m.detailSection = "final"
		m.expandedDetailScroll = 0
		return m, nil
	case "v":
		m.leaveAnswer()
		m.detailExpanded = true
		m.detailSection = "history"
		m.expandedDetailScroll = 0
		return m, nil
	case "u":
		m.leaveAnswer()
		m.detailSection = "inspection"
		return m.inspectSelectedRecovery()
	case "[":
		m.expandedDetailScroll = 0
		return m.cycleFinal(-1)
	case "]":
		m.expandedDetailScroll = 0
		return m.cycleFinal(1)
	case "t":
		state.focus = 0
		return m, nil
	case "a":
		return m.answerSelectedDecision()
	case "s":
		m.detailExpanded = false
		return m.startSelected()
	case "m":
		return m.toggleMode()
	case "x":
		return m.prepareStop()
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
	case "?":
		m.leaveAnswer()
		m.help = true
	}
	return m, nil
}

func (m Model) handleDecisionAnswerKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	dialog := m.answer
	if dialog == nil {
		return m, nil
	}
	if key.String() == "f8" {
		dialog.details = !dialog.details
		dialog.scroll = 0
		return m, nil
	}
	keyName := key.String()
	if dialog.details {
		switch keyName {
		case "pgup", "up":
			dialog.scroll = max(0, dialog.scroll-8)
		case "pgdown", "down":
			dialog.scroll += 8
		case "esc":
			dialog.details = false
		}
		return m, nil
	}
	defer m.saveDecisionDraft()
	switch keyName {
	case "esc", "ctrl+c":
		m.saveDecisionDraft()
		m.answer = nil
		m.projects[m.selected].focus = 1
		m.status = "已关闭答案草稿；答案未提交"
		return m, nil
	case "f2":
		return m.submitDecisionAnswer()
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
		request, ok := dialog.request, dialog.request.Question != ""
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
			m.projects[m.selected].focus = 2
			return m, nil
		}
		insertAnswerRunes(dialog, []rune{'\n'})
		dialog.error = ""
		return m, nil
	}
	if !dialog.noteFocus {
		if key.Type == tea.KeyRunes && len(key.Runes) == 1 && key.Runes[0] >= '1' && key.Runes[0] <= '9' {
			request, ok := dialog.request, dialog.request.Question != ""
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
