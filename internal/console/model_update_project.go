package console

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width, m.height = msg.Width, msg.Height
	return m, nil
}

func (m Model) updateTick() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	now := m.clock.Now()
	tickInterval := 500 * time.Millisecond
	for i := range m.projects {
		state := &m.projects[i]
		interval := 5 * time.Second
		if i == m.selected {
			interval = time.Second
		}
		if !state.refreshing && (state.lastRefresh.IsZero() || now.Sub(state.lastRefresh) >= interval) {
			cmds = append(cmds, m.readProject(i))
		}
		for _, action := range state.actions {
			if !action.Resolved && action.Status != "indeterminate" && now.Before(state.actionPollUntil) {
				tickInterval = 100 * time.Millisecond
			}
			if !action.InFlight && !action.Resolved && action.Status != "submitting" {
				action.InFlight = true
				cmds = append(cmds, m.pollAction(i, action))
			}
		}
	}
	if m.selected >= 0 && m.selected < len(m.projects) {
		var cmd tea.Cmd
		m, cmd = m.refreshSelected(m.selected)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	}
	cmds = append(cmds, waitTick(m.clock, tickInterval))
	return m, tea.Batch(cmds...)
}

func (m Model) updateProject(msg projectMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.key)
	if index < 0 {
		return m, nil
	}
	state := &m.projects[index]
	if state.refreshID != msg.requestID {
		return m, nil
	}
	state.refreshing = false
	if msg.identity != state.ref.Identity {
		return m, nil
	}
	previousSelection := state.selectedKey
	selectedTaskID := selectedNativeTaskID(*state)
	if msg.snapshot.SelectedNative != nil && selectedTaskID != msg.snapshot.SelectedNative.TaskID {
		msg.snapshot.SelectedNative = nil
	}
	if msg.snapshot.NativeTasks == nil && msg.snapshot.SelectedNative == nil && state.snapshot.SelectedNative != nil && selectedTaskID == state.snapshot.SelectedNative.TaskID {
		msg.snapshot.SelectedNative = state.snapshot.SelectedNative
	}
	state.snapshot, state.snapshotLoaded = preserveLastGood(state.snapshot, msg.snapshot), true
	if state.client != nil {
		state.ref.Presentation = state.client.Project().Presentation
	}
	state.lastRefresh = m.clock.Now()
	rows := m.rows(index)
	if state.selectedKey == "" && !state.selectionHold {
		state.selectedKey = firstRowKey(rows)
	} else if index == m.selected && len(rows) > 0 && !rowExists(rows, state.selectedKey) {
		if state.filter == 0 && strings.HasPrefix(state.selectedKey, "task:") {
			taskID := strings.TrimPrefix(state.selectedKey, "task:")
			for _, row := range rows {
				if row.kind == rowRun && row.taskID == taskID {
					state.selectedKey = row.key
					break
				}
			}
		}
		if !rowExists(rows, state.selectedKey) {
			m.status = "当前选择不在最新快照中；未切换到其他任务，请刷新或重新选择"
		}
	}
	if index == m.selected && m.runFocus != "" {
		found := false
		for _, run := range candidateRuns(*state, "") {
			if run.RunID == m.runFocus || run.TaskID == m.runFocus {
				if state.candidates == nil {
					state.candidates = map[string]string{}
				}
				state.candidates[run.TaskID] = run.RunID
				state.selectedKey, m.runFocus = "task:"+run.TaskID, ""
				found = true
				break
			}
		}
		if !found {
			m.status = "找不到 run_id/task_id：" + safeTerminalText(m.runFocus) + "；未切换到其他任务"
			state.selectedKey = ""
			state.selectionHold = true
			m.runFocus = ""
		}
	}
	m.selectAfterCandidateStop(index)
	if index == m.selected {
		m.syncAnswer()
	}
	var cmds []tea.Cmd
	if index == m.selected {
		var cmd tea.Cmd
		m, cmd = m.refreshSelected(index)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if previousSelection != state.selectedKey {
		m.saveProjectView(index)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) updateRecoveryInspection(msg recoveryInspectionMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	state := &m.projects[index]
	if msg.identity != state.ref.Identity || msg.generation != state.recoveryGeneration || msg.runID != state.recoveryRunID {
		return m, nil
	}
	state.recoveryBusy = false
	if msg.err != nil {
		state.recoveryError = msg.err.Error()
		state.recoveryReport = nil
		if index == m.selected {
			m.status = "恢复核验失败：" + msg.err.Error()
		}
		return m, nil
	}
	if msg.report.RunID != msg.runID {
		state.recoveryError = "核验响应不属于原候选"
		state.recoveryReport = nil
		return m, nil
	}
	state.recoveryError = ""
	state.recoveryReport = &msg.report
	if index == m.selected {
		m.status = "恢复核验报告已刷新 · run " + msg.report.RunID + " · revision " + strconv.FormatUint(msg.report.RunRevision, 10)
	}
	return m, nil
}
