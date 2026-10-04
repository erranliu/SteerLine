package console

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func (m Model) startSelected() (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	row := m.selectedRow(m.selected)
	if row == nil || row.kind != rowReady || row.candidate == nil {
		m.status = "请选择 Ready 候选后再开始"
		return m, nil
	}
	if reason := candidateActionReason(*state, m.clock.Now()); reason != "" {
		m.status = "不能开始：" + reason
		return m, nil
	}
	runtimeState := state.snapshot.Connection.Snapshot.Runtime
	if len(state.ref.Presentation.Profiles) == 0 {
		m.status = "没有可用的执行方案"
		return m, nil
	}
	for _, action := range state.actions {
		if action.TaskID == row.taskID && action.Status != "applied" && action.Status != "rejected" && action.Status != "failed" {
			m.status = "此视图已有该任务请求 · task " + row.taskID + " · request " + action.RequestID
			return m, nil
		}
	}
	m.profileChoice = &executionProfileChoice{projectKey: state.ref.Key, taskID: row.taskID, instance: runtimeState.Service.InstanceID, revision: runtimeState.Control.Revision}
	return m, nil
}

func (m Model) toggleMode() (tea.Model, tea.Cmd) {
	_, runtimeState := m.readyControlTarget()
	if runtimeState == nil {
		return m, nil
	}
	mode := "manual"
	if runtimeState.Control.DispatchMode == "manual" {
		mode = "auto"
	}
	if mode == "auto" {
		if reason := m.projects[m.selected].client.AutoDispatchAvailability(m.projects[m.selected].snapshot); reason != "" {
			m.status = reason
			return m, nil
		}
		m.status = "确认前请留意：切到 auto 后服务可能领取 Ready 任务"
	}
	return m.createAction("set_dispatch_mode", mode)
}

func (m Model) prepareStop() (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	connection := state.snapshot.Connection
	if state.ref.Error != "" || state.ref.Conflict != "" || !connection.CanStop || connection.Snapshot.Runtime == nil {
		m.status = "当前没有可定位且支持 stop_service 的服务实例：" + connection.Diagnostic
		return m, nil
	}
	runtimeState := connection.Snapshot.Runtime
	if runtimeState.Service.InstanceID == "" {
		m.status = "服务实例身份不可用，不能停止"
		return m, nil
	}
	m.confirm = &confirmAction{projectKey: state.ref.Key, instance: runtimeState.Service.InstanceID, revision: runtimeState.Control.Revision, action: "stop_service", requestID: newRequestID()}
	return m, nil
}

func (m Model) inspectSelectedRecovery() (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	row := m.selectedRow(m.selected)
	if row == nil || row.run == nil {
		m.status = "请选择一个 run 后查看恢复核验报告"
		return m, nil
	}
	state.panel = 6
	state.focus = 1
	state.recoveryRunID = row.run.RunID
	state.recoveryReport = nil
	state.recoveryError = ""
	state.recoveryBusy = true
	state.recoveryGeneration++
	generation := state.recoveryGeneration
	ref, runID, ctx := state.ref, row.run.RunID, m.ctx
	m.status = "正在只读核验运行、工作目录、领取和 PR 身份"
	return m, func() tea.Msg {
		report, err := state.client.InspectRecovery(ctx, runID)
		return recoveryInspectionMsg{projectKey: ref.Key, identity: ref.Identity, runID: runID, generation: generation, report: report, err: err}
	}
}

func (m *Model) readyControlTarget() (*projectState, *contracts.Runtime) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		m.status = "没有项目"
		return nil, nil
	}
	state := &m.projects[m.selected]
	connection := state.snapshot.Connection
	runtimeState := connection.Snapshot.Runtime
	if state.ref.Error != "" || state.ref.Conflict != "" || runtimeState == nil || !connection.CanControl || connection.State != api.ConnectionConnected {
		m.status = "服务未就绪，不能提交控制指令：" + firstText(connection.Diagnostic, string(connection.State), "未知状态")
		return state, nil
	}
	return state, runtimeState
}

func (m Model) createAction(action, taskID string) (tea.Model, tea.Cmd) {
	return m.createActionWithProfile(action, taskID, nil)
}

func (m Model) createActionWithProfile(action, taskID string, profile *api.ExecutionProfileSummary) (tea.Model, tea.Cmd) {
	state, runtimeState := m.readyControlTarget()
	if runtimeState == nil {
		return m, nil
	}
	if state.ref.Conflict != "" {
		m.status = state.ref.Conflict
		return m, nil
	}
	if !hasCapability(runtimeState, action) {
		m.status = "服务未声明能力：" + action
		return m, nil
	}
	for _, pending := range state.actions {
		if pending.Action == action && !pending.Resolved && pending.Status != "applied" && pending.Status != "rejected" && pending.Status != "failed" {
			m.status = "此操作已有请求在跟踪 · request " + pending.RequestID
			return m, nil
		}
	}
	if action == "start_task" {
		if reason := candidateActionReason(*state, m.clock.Now()); reason != "" {
			m.status = "候选已不可操作：" + reason
			return m, nil
		}
		if profile == nil {
			if len(state.ref.Presentation.Profiles) == 0 {
				m.status = "没有可用的执行方案"
				return m, nil
			}
			first := state.ref.Presentation.Profiles[0]
			profile = &first
		}
		if reason := profile.UnavailableReason; reason != "" {
			m.status = "执行配置不可用：" + reason
			return m, nil
		}
	}
	if action == "open_terminal" || action == "recheck_environment" {
		row := m.selectedRow(m.selected)
		if row == nil || row.run == nil || row.run.EnvironmentBlocker == nil {
			m.status = "当前没有环境阻断"
			return m, nil
		}
		taskID = row.run.RunID
	}
	requestID := newRequestID()
	instance := runtimeState.Service.InstanceID
	revision := runtimeState.Control.Revision
	newAction := &actionState{RequestID: requestID, Action: action, TaskID: taskID, Status: "submitting", Message: "准备提交", TargetInstance: instance, UpdatedAt: m.clock.Now()}
	if profile != nil {
		newAction.ExecutionProfileID, newAction.ExecutionProfileDigest = profile.ID, profile.Digest
	}
	if action == "open_terminal" || action == "recheck_environment" {
		row := m.selectedRow(m.selected)
		if row != nil {
			newAction.EnvironmentTarget, _ = api.EnvironmentTarget(row.run)
		}
	}
	state.actions[requestID] = newAction
	state.view.Outstanding[requestID] = "task:" + taskID
	if err := m.saveProjectView(m.selected); err != nil {
		delete(state.actions, requestID)
		delete(state.view.Outstanding, requestID)
		m.status = "未提交：无法持久化 request_id；请检查 UI 视图目录权限后重试"
		return m, nil
	}
	m.status = fmt.Sprintf("提交中 · task %s · request %s", displayDash(taskID), requestID)
	if profile != nil {
		m.status += " · " + safeTerminalInlineText(profile.Name)
	}
	if action == "set_dispatch_mode" && taskID == "auto" {
		m.status += " · auto 可能开始领取 Ready 任务"
	}
	return m, m.submitControl(m.selected, action, taskID, requestID, instance, revision)
}

func (m Model) beginAction(projectKey, instance string, revision uint64, action, taskID, requestID string) (tea.Model, tea.Cmd) {
	index := m.projectIndex(projectKey)
	if index < 0 {
		m.status = "停止目标项目已不存在"
		return m, nil
	}
	state := &m.projects[index]
	runtimeState := state.snapshot.Connection.Snapshot.Runtime
	if runtimeState == nil || runtimeState.Service.InstanceID != instance {
		m.status = "stale_instance：服务实例已变化；刷新后重新选择，未提交动作"
		state.lastRefresh = time.Time{}
		return m, m.readProject(index)
	}
	if index != m.selected {
		m.status = "项目已切换，请回到原项目重新发起动作"
		return m, nil
	}
	if action != "stop_service" && runtimeState.Control.Revision != revision {
		m.status = "stale_revision：控制状态已变化；刷新后重新选择，未提交动作"
		state.lastRefresh = time.Time{}
		return m, m.readProject(index)
	}
	// The request ID was allocated before the confirmation prompt, and remains
	// the sole identity used while publishing and reconciling this action.
	return m.createActionWithID(index, action, taskID, requestID, instance, revision)
}

func (m Model) createActionWithID(index int, action, taskID, requestID, instance string, revision uint64) (tea.Model, tea.Cmd) {
	state := &m.projects[index]
	for _, pending := range state.actions {
		if pending.Action == action && !pending.Resolved && pending.Status != "applied" && pending.Status != "rejected" && pending.Status != "failed" {
			m.status = "此操作已有请求在跟踪 · request " + pending.RequestID
			return m, nil
		}
	}
	state.actions[requestID] = &actionState{RequestID: requestID, Action: action, TaskID: taskID, Status: "submitting", Message: "准备提交", TargetInstance: instance, UpdatedAt: m.clock.Now()}
	state.view.Outstanding[requestID] = "task:" + taskID
	if err := m.saveProjectView(index); err != nil {
		delete(state.actions, requestID)
		delete(state.view.Outstanding, requestID)
		m.status = "未提交：无法持久化 request_id；请检查 UI 视图目录权限后重试"
		return m, nil
	}
	m.status = fmt.Sprintf("提交中 · task %s · request %s", displayDash(taskID), requestID)
	if action == "set_dispatch_mode" && taskID == "auto" {
		m.status += " · auto 可能开始领取 Ready 任务"
	}
	return m, m.submitControl(index, action, taskID, requestID, instance, revision)
}

func hasCapability(runtimeState *contracts.Runtime, action string) bool {
	for _, capability := range runtimeState.Service.Capabilities {
		if capability == action {
			return true
		}
	}
	return false
}

func candidateActionReason(state projectState, now time.Time) string {
	if state.client == nil {
		return firstText(state.ref.Error, "产品输入资格尚未核验")
	}
	return state.client.StartTaskAvailability(state.snapshot, now)
}
