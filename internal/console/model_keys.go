package console

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.String() == "ctrl+n" && m.confirm == nil && m.profileChoice == nil && !m.help {
		return m.openNativeSession()
	}
	if m.operationDetails {
		return m.handleOperationDetails(key)
	}
	if m.detailExpanded {
		return m.handleReaderKey(key)
	}
	if m.answer != nil && m.answer.details {
		return m.handleDecisionAnswerKey(key)
	}
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
			if !m.operationVisible() {
				m.status = "窗口过小，请调整后操作；尚未提交"
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
		if m.projects[m.selected].focus == 0 && (key.String() == "enter" || key.String() == "a") {
			m.projects[m.selected].focus = 1
			if m.answer.noteFocus {
				m.projects[m.selected].focus = 2
			}
			return m, nil
		}
		if m.projects[m.selected].focus == 0 {
			switch key.String() {
			case "s", "b", "u", "x":
				if !m.leaveAnswerSaved() {
					return m, nil
				}
			}
		}
		if m.answer != nil && (m.answer.details || m.projects[m.selected].focus != 0) {
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
	switch key.String() {
	case "esc":
		m.leaveRecovery()
		state.lowerScroll = 0
		state.focus = 0
		return m, nil
	case "f8":
		if state.recoveryActive {
			m.operationDetails = true
			m.expandedDetailScroll = 0
		}
		return m, nil
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
		if state.recoveryActive && state.focus != 0 {
			return m.inspectSelectedRecovery()
		}
		state.lastRefresh = time.Time{}
		state.final.retry()
		return m, m.readProject(m.selected)
	case "enter":
		if row := m.selectedRow(m.selected); row != nil && pendingDecision(row.run) && waitingForDecision(row.run) && !feedbackInProgress(*state, row.run) {
			return m.answerSelectedDecision()
		}
		state.focus = 1
		return m, nil
	case "d":
		if !m.leaveAnswerSaved() {
			return m, nil
		}
		m.detailExpanded = true
		m.detailSection = ""
		m.expandedDetailScroll = 0
		return m, nil
	case "f":
		if !m.leaveAnswerSaved() {
			return m, nil
		}
		m.detailExpanded = true
		m.detailSection = "final"
		m.expandedDetailScroll = 0
		return m, m.readSelectedFinalFull()
	case "v":
		if !m.leaveAnswerSaved() {
			return m, nil
		}
		m.detailExpanded = true
		m.detailSection = "history"
		m.expandedDetailScroll = 0
		return m, nil
	case "u":
		if !m.leaveAnswerSaved() {
			return m, nil
		}
		return m.inspectSelectedRecovery()
	case "[":
		return m, nil
	case "]":
		return m, nil
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
	case "8":
		return m.changeFilter(7)
	case "?":
		if !m.leaveAnswerSaved() {
			return m, nil
		}
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
			dialog.scroll = clampReadingScroll(dialog.scroll-8, len(wrapLines(answerDetailLines(m, m.projects[m.selected]), max(1, m.width-2))), consoleContentHeight(m))
		case "pgdown", "down":
			dialog.scroll = clampReadingScroll(dialog.scroll+8, len(wrapLines(answerDetailLines(m, m.projects[m.selected]), max(1, m.width-2))), consoleContentHeight(m))
		case "esc":
			dialog.details = false
		}
		return m, nil
	}
	defer m.saveDecisionDraft()
	switch keyName {
	case "esc", "ctrl+c":
		if !m.leaveAnswerSaved() {
			return m, nil
		}
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
	if key.String() == "f8" {
		m.operationDetails = true
		m.expandedDetailScroll = 0
		return m, nil
	}
	if next, cmd, handled := m.handleWorkspaceNavigation(key); handled {
		return next, cmd
	}
	confirm := m.confirm
	switch key.String() {
	case "esc", "n":
		m.confirm = nil
	case "enter":
		if m.projects[m.selected].focus == 0 {
			m.projects[m.selected].focus = 1
			return m, nil
		}
		if !m.operationVisible() {
			m.status = "窗口过小，请调整后操作；未确认"
			return m, nil
		}
		m.confirm = nil
		return m.beginAction(confirm.projectKey, confirm.instance, confirm.revision, confirm.action, confirm.taskID, confirm.requestID)
	}
	return m, nil
}
