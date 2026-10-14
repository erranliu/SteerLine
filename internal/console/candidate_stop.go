package console

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func candidateParticipating(r *api.CandidateSummary) bool {
	return r != nil && r.WorkflowStatus != "" && r.WorkflowStatus != "completed" && r.WorkflowStatus != "abandoned"
}

// Only server-persisted runs count for the last-candidate protection. Local
// start-request overlays may be shown immediately but cannot grant this action.
func confirmedCandidates(s projectState, taskID string) []*api.CandidateSummary {
	var runs []*api.CandidateSummary
	if page := s.snapshot.Observation.Page; page != nil {
		control := s.snapshot.Connection.Snapshot.Page
		if control == nil || page.Service.InstanceID != control.Service.InstanceID || viewCanonical(page.Repository.Identity) != viewCanonical(s.ref.Identity) {
			return nil
		}
		for _, r := range page.Runs {
			if r != nil && r.TaskID == taskID {
				runs = append(runs, r)
			}
		}
	}
	return runs
}

func candidateStopReason(s projectState, taskID, runID string, at time.Time) string {
	if valid, reason := api.CandidateFreshness(s.ref.Identity, s.snapshot, at); !valid {
		return reason
	}
	page := s.snapshot.Connection.Snapshot.Page
	if !hasCapability(page, "stop_candidate") {
		return "服务未声明停止候选能力"
	}
	var target *api.CandidateSummary
	count := 0
	for _, r := range confirmedCandidates(s, taskID) {
		if candidateParticipating(r) {
			count++
		}
		if r.RunID == runID {
			target = r
		}
	}
	if target == nil || target.ControlRevision == 0 {
		return "候选尚未由服务器确认；请等待或刷新"
	}
	if !candidateParticipating(target) {
		return "候选已结束，历史只读；资源保留"
	}
	if count <= 1 {
		return "只剩一个参与候选，不能停止"
	}
	for _, a := range s.actions {
		if a.Action == "stop_candidate" && a.Payload.RunID == runID && !a.Resolved {
			return "正在停止该候选；查询原请求回执"
		}
	}
	return ""
}

func (m Model) prepareCandidateStop() (tea.Model, tea.Cmd) {
	s, page := m.readyControlTarget()
	if page == nil {
		return m, nil
	}
	row := m.selectedRow(m.selected)
	if row == nil || row.run == nil {
		m.status = "请选择服务器已确认的候选"
		return m, nil
	}
	if reason := candidateStopReason(*s, row.taskID, row.run.RunID, m.clock.Now()); reason != "" {
		m.status = reason
		return m, nil
	}
	if !m.leaveAnswerSaved() {
		return m, nil
	}
	r := row.run
	profile := "未核实"
	if r.ExecutionProfile != nil {
		profile = firstText(r.ExecutionProfile.Name, r.ExecutionProfile.ID)
	}
	m.cancelWorkspace()
	m.detailExpanded = false
	m.confirm = &confirmAction{projectKey: s.ref.Key, taskID: row.taskID, instance: page.Service.InstanceID,
		revision: page.Control.Revision, action: "stop_candidate", requestID: newRequestID(), candidateLabel: candidateLabel(*s, r), profile: profile,
		payload: api.ControlPayload{TaskID: row.taskID, RunID: r.RunID, ExpectedRunRevision: r.ControlRevision}}
	s.focus, s.lowerScroll = 1, 0
	return m, nil
}

func (m Model) submitCandidateStop(c confirmAction) (tea.Model, tea.Cmd) {
	i := m.projectIndex(c.projectKey)
	if i < 0 || i != m.selected {
		m.status = "项目已切换，未提交停止"
		return m, nil
	}
	s, page := m.readyControlTarget()
	if page == nil {
		return m, nil
	}
	if page.Service.InstanceID != c.instance || page.Control.Revision != c.revision {
		m.status = "服务或控制版本已变化；刷新后重新确认"
		return m, m.readProject(i)
	}
	if reason := candidateStopReason(*s, c.taskID, c.payload.RunID, m.clock.Now()); reason != "" {
		m.status = reason
		return m, nil
	}
	for _, r := range confirmedCandidates(*s, c.taskID) {
		if r.RunID == c.payload.RunID && r.ControlRevision != c.payload.ExpectedRunRevision {
			m.status = "候选版本已变化；请重新确认"
			return m, m.readProject(i)
		}
	}
	a := &actionState{RequestID: c.requestID, Action: c.action, TaskID: c.taskID, RunID: c.payload.RunID, Payload: c.payload,
		TargetInstance: c.instance, Status: "submitting", Message: "正在停止参与；等待服务器确认", UpdatedAt: m.clock.Now()}
	s.actions[a.RequestID] = a
	s.view.Outstanding[a.RequestID] = "task:" + c.taskID
	if err := m.saveProjectView(i); err != nil {
		delete(s.actions, a.RequestID)
		delete(s.view.Outstanding, a.RequestID)
		m.status = "未提交：无法保存原停止请求身份"
		return m, nil
	}
	m.status = a.Message
	return m, m.submitControl(i, c.action, c.taskID, c.requestID, c.instance, c.revision)
}

func (m *Model) selectAfterCandidateStop(index int) {
	s := &m.projects[index]
	if valid, _ := api.CandidateFreshness(s.ref.Identity, s.snapshot, m.clock.Now()); !valid {
		return
	}
	for _, a := range s.actions {
		if a.Action != "stop_candidate" || !a.StopSelectionPending || a.Status != "applied" {
			continue
		}
		row := m.selectedRow(index)
		if row == nil || row.run == nil || row.run.RunID != a.Payload.RunID {
			a.StopSelectionPending = false
			continue
		}
		var target, survivor *api.CandidateSummary
		for _, r := range confirmedCandidates(*s, a.TaskID) {
			if r.RunID == a.Payload.RunID {
				target = r
			}
			if candidateParticipating(r) && r.ControlRevision > 0 && (survivor == nil || r.CandidateNumber > survivor.CandidateNumber) {
				survivor = r
			}
		}
		if target == nil || candidateParticipating(target) || survivor == nil {
			continue
		}
		a.StopSelectionPending = false
		s.candidates[a.TaskID] = survivor.RunID
		s.detailScroll, s.lowerScroll = 0, 0
		s.final = finalReplyReader{finalGeneration: s.final.finalGeneration + 1, bodyGeneration: s.final.bodyGeneration + 1, previewGeneration: s.final.previewGeneration + 1}
		s.activityGeneration++
		s.activityKey, s.activityError, s.activityBusy, s.activityReadAt = "", "", false, time.Time{}
		s.recentActivity, s.workerTransitions = nil, nil
		if index == m.selected {
			m.expandedDetailScroll = 0
			m.leaveAnswer()
			m.cancelWorkspace()
		}
		m.saveProjectView(index)
	}
}
