package console

import (
	"fmt"

	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateActionSubmitted(msg actionSubmittedMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	state := &m.projects[index]
	if msg.identity != state.ref.Identity {
		return m, nil
	}
	action := state.actions[msg.requestID]
	if action == nil {
		return m, nil
	}
	if action.Action == "answer_decision" {
		action.InFlight = false
		action.UpdatedAt = m.clock.Now()
		if msg.command != nil && msg.command.CommandID != action.RequestID {
			remapDecisionActionID(state, action, msg.requestID, msg.command.CommandID)
		}
		refreshNow := applyDecisionCommandResult(state, action, msg.command, msg.err)
		if action.Resolved {
			delete(state.view.Outstanding, action.RequestID)
		}
		if index == m.selected {
			m.status = feedbackReceiptStatus(*state, action)
		}
		m.saveProjectView(index)
		return m, m.actionFollowup(index, action, refreshNow)
	}
	responseMismatch := msg.instance != action.TargetInstance
	if responseMismatch {
		action.Status, action.Message, action.Resolved = "indeterminate", "响应目标实例与原 request_id 不匹配；不提交替代动作并停止自动跟踪", true
	}
	action.InFlight = false
	action.UpdatedAt = m.clock.Now()
	refreshNow := false
	if responseMismatch {
		if index == m.selected {
			m.status = feedbackReceiptStatus(*state, action)
		}
	} else if msg.err != nil {
		action.Status, action.Message = "indeterminate", "提交结果待确认："+msg.err.Error()
		if strings.Contains(msg.err.Error(), "stale_revision") || strings.Contains(msg.err.Error(), "stale_instance") {
			action.Status, action.Resolved = "rejected", true
			action.Message = msg.err.Error() + "；正在刷新状态，请重新选择"
			delete(state.view.Outstanding, msg.requestID)
			if action.Action != "start_candidate" && action.Action != "stop_candidate" {
				state.selectedKey = ""
				state.selectionHold = true
			}
			refreshNow = true
		}
		if index == m.selected {
			m.status = feedbackReceiptStatus(*state, action)
		}
	} else {
		action.Status, action.Message = "submitted", "请求已提交，等待服务确认"
		if index == m.selected {
			m.status = fmt.Sprintf("%s 请求已提交 · task %s · request %s", msg.action, displayDash(msg.taskID), msg.requestID)
		}
	}
	m.saveProjectView(index)
	return m, m.actionFollowup(index, action, refreshNow)
}

// Read the original request immediately after submission, without resubmitting
// it or waiting for the next project refresh.
func (m Model) actionFollowup(index int, action *actionState, refresh bool) tea.Cmd {
	var cmds []tea.Cmd
	if refresh {
		m.projects[index].lastRefresh = time.Time{}
		cmds = append(cmds, m.readProject(index))
	}
	if !action.Resolved && !action.InFlight {
		m.projects[index].actionPollUntil = m.clock.Now().Add(5 * time.Second)
		action.InFlight = true
		cmds = append(cmds, m.pollAction(index, action))
	}
	return tea.Batch(cmds...)
}

func (m Model) updateActionReceipt(msg actionReceiptMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	state := &m.projects[index]
	if msg.identity != state.ref.Identity {
		return m, nil
	}
	action := state.actions[msg.requestID]
	if action == nil {
		return m, nil
	}
	if action.Action == "answer_decision" {
		action.InFlight = false
		action.UpdatedAt = m.clock.Now()
		refreshNow := applyDecisionCommandResult(state, action, msg.command, msg.err)
		if action.Resolved {
			delete(state.view.Outstanding, action.RequestID)
		}
		if index == m.selected {
			m.status = feedbackReceiptStatus(*state, action)
		}
		m.saveProjectView(index)
		if refreshNow {
			state.lastRefresh = time.Time{}
			return m, m.readProject(index)
		}
		return m, nil
	}
	action.InFlight = false
	action.UpdatedAt = m.clock.Now()
	refreshNow := false
	if msg.receipt != nil {
		previousStatus, previousRun := action.Status, action.RunID
		expectedRun := ""
		if action.Action == "start_candidate" || action.Action == "start_task" && action.CandidateNumber > 0 {
			expectedRun = action.RequestID
		}
		if action.Action == "stop_candidate" {
			expectedRun = action.Payload.RunID
		}
		candidateMismatch := expectedRun != "" && (msg.receipt.Status == "applied" || msg.receipt.Status == "applying") && msg.receipt.RunID != expectedRun
		if msg.receipt.RequestID != action.RequestID || msg.receipt.Action != action.Action || msg.receipt.InstanceID != action.TargetInstance || candidateMismatch {
			action.Status, action.Message, action.Resolved = "indeterminate", "receipt 与原控制动作或实例不匹配；已停止自动跟踪", true
			m.saveProjectView(index)
			if index == m.selected {
				m.status = feedbackReceiptStatus(*state, action)
			}
			return m, nil
		}
		action.Status, action.Message, action.RunID = msg.receipt.Status, msg.receipt.Message, msg.receipt.RunID
		if action.Action == "stop_candidate" && msg.receipt.Status == "applied" && previousStatus != "applied" {
			row := m.selectedRow(index)
			action.StopSelectionPending = row != nil && row.run != nil && row.run.RunID == action.Payload.RunID
		}
		action.Resolved = msg.receipt.Status == "applied" || msg.receipt.Status == "rejected" || msg.receipt.Status == "failed" || msg.receipt.Status == "indeterminate"
		if action.Action == "stop_service" && msg.receipt.Status == "applied" && msg.receipt.Code == "stop_exit_unverified" {
			action.Status, action.Resolved = "pending_verification", false
		}
		if action.Resolved || msg.receipt.Status == "applying" && (previousStatus != "applying" || previousRun != action.RunID) {
			refreshNow = true
		}
		if msg.receipt.InstanceID != action.TargetInstance {
			action.Status, action.Message, action.Resolved = "indeterminate", "receipt 指向另一服务实例；保留原目标并停止自动跟踪", true
		}
		if msg.receipt.Code == "stale_revision" {
			action.Status, action.Message, action.Resolved = "rejected", "stale_revision：控制状态已变化；正在刷新状态，请重新选择", true
			if action.Action != "start_candidate" && action.Action != "stop_candidate" {
				state.selectedKey = ""
				state.selectionHold = true
			}
			refreshNow = true
		}
	} else if msg.request != nil {
		if !controlRequestMatchesAction(msg.request, action, state.ref.Identity) {
			action.Status, action.Message, action.Resolved = "indeterminate", "request_id 内容与原控制动作、payload 或目标不匹配；已停止自动跟踪", true
			m.saveProjectView(index)
			if index == m.selected {
				m.status = feedbackReceiptStatus(*state, action)
			}
			return m, nil
		}
		action.Status, action.Message, action.Action = "submitted", "请求已发布，回执尚未写入；继续查询原 request_id", msg.request.Action
		action.Resolved = false
	} else if msg.err != nil {
		action.Status, action.Message = "indeterminate", "结果待确认；查询原 request_id："+msg.err.Error()
		action.Resolved = false
	}
	if action.Status == "applied" || action.Status == "rejected" || action.Status == "failed" {
		delete(state.view.Outstanding, msg.requestID)
	}
	if index == m.selected {
		m.status = fmt.Sprintf("%s · task %s · request %s · %s", actionStatusLabel(action.Status), displayDash(action.TaskID), action.RequestID, action.Message)
	}
	m.saveProjectView(index)
	if refreshNow {
		state.lastRefresh = time.Time{}
		return m, m.readProject(index)
	}
	return m, nil
}
