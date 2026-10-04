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
		refreshNow, reopen := applyDecisionCommandResult(state, action, msg.command, msg.err)
		if reopen && action.Action == "answer_decision" && index == m.selected {
			m.reopenDecisionDraft(index, action)
		}
		if action.Resolved {
			delete(state.view.Outstanding, action.RequestID)
		}
		if index == m.selected {
			m.status = action.Message
		}
		m.saveProjectView(index)
		if refreshNow {
			state.lastRefresh = time.Time{}
			return m, m.readProject(index)
		}
		return m, nil
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
			m.status = action.Message
		}
	} else if msg.err != nil {
		action.Status, action.Message = "indeterminate", "提交结果待确认："+msg.err.Error()
		if strings.Contains(msg.err.Error(), "stale_revision") || strings.Contains(msg.err.Error(), "stale_instance") {
			action.Status, action.Resolved = "rejected", true
			action.Message = msg.err.Error() + "；正在刷新状态，请重新选择"
			delete(state.view.Outstanding, msg.requestID)
			if action.Action != "start_candidate" && action.Action != "abandon_candidate" {
				state.selectedKey = ""
				state.selectionHold = true
			}
			refreshNow = true
		}
		if index == m.selected {
			m.status = action.Message
		}
	} else {
		action.Status, action.Message = "submitted", "请求已提交，等待服务确认"
		if index == m.selected {
			m.status = fmt.Sprintf("%s 请求已提交 · task %s · request %s", msg.action, displayDash(msg.taskID), msg.requestID)
		}
	}
	m.saveProjectView(index)
	if refreshNow {
		state.lastRefresh = time.Time{}
		return m, m.readProject(index)
	}
	return m, nil
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
		refreshNow, reopen := applyDecisionCommandResult(state, action, msg.command, msg.err)
		if reopen && action.Action == "answer_decision" && index == m.selected {
			m.reopenDecisionDraft(index, action)
		}
		if action.Resolved {
			delete(state.view.Outstanding, action.RequestID)
		}
		if index == m.selected {
			m.status = action.Message
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
		expectedRun := ""
		if action.Action == "start_candidate" {
			expectedRun = action.RequestID
		}
		if action.Action == "abandon_candidate" {
			expectedRun = action.Payload.WorkflowRunID
		}
		candidateMismatch := expectedRun != "" && (msg.receipt.Status == "applied" || msg.receipt.Status == "applying") && msg.receipt.RunID != expectedRun
		if msg.receipt.RequestID != action.RequestID || msg.receipt.Action != action.Action || msg.receipt.InstanceID != action.TargetInstance || candidateMismatch {
			action.Status, action.Message, action.Resolved = "indeterminate", "receipt 与原控制动作或实例不匹配；已停止自动跟踪", true
			m.saveProjectView(index)
			if index == m.selected {
				m.status = action.Message
			}
			return m, nil
		}
		action.Status, action.Message, action.RunID = msg.receipt.Status, msg.receipt.Message, msg.receipt.RunID
		action.Resolved = msg.receipt.Status == "applied" || msg.receipt.Status == "rejected" || msg.receipt.Status == "failed" || msg.receipt.Status == "indeterminate"
		if msg.receipt.Status == "applied" {
			refreshNow = true
		}
		if msg.receipt.InstanceID != action.TargetInstance {
			action.Status, action.Message, action.Resolved = "indeterminate", "receipt 指向另一服务实例；保留原目标并停止自动跟踪", true
		}
		if msg.receipt.Code == "stale_revision" {
			action.Status, action.Message, action.Resolved = "rejected", "stale_revision：控制状态已变化；正在刷新状态，请重新选择", true
			if action.Action != "start_candidate" && action.Action != "abandon_candidate" {
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
				m.status = action.Message
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
		m.status = fmt.Sprintf("%s · task %s · request %s · %s", action.Status, displayDash(action.TaskID), action.RequestID, action.Message)
	}
	m.saveProjectView(index)
	if refreshNow {
		state.lastRefresh = time.Time{}
		return m, m.readProject(index)
	}
	return m, nil
}
