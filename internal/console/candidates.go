package console

import (
	"fmt"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type CandidateDraft struct {
	OptionIndex int
	Note        string
}

func candidateIdentity(r *api.CandidateSummary) string {
	return strings.Join([]string{r.TaskID, r.RunID, r.ExecutionID, r.SessionID}, "\x00")
}

func candidateRuns(state projectState, taskID string) []*api.CandidateSummary {
	var runs []*api.CandidateSummary
	if rt := state.snapshot.Observation.Page; rt != nil {
		for _, r := range rt.Runs {
			if r != nil && (taskID == "" || r.TaskID == taskID) {
				runs = append(runs, r)
			}
		}
	}
	seen := map[string]bool{}
	for _, r := range runs {
		seen[r.RunID] = true
	}
	for _, a := range state.actions {
		if a.CandidateNumber == 0 || (a.Action != "start_task" && a.Action != "start_candidate") || seen[a.RequestID] || taskID != "" && a.TaskID != taskID {
			continue
		}
		// This is a display overlay for the original request, not an assertion
		// that the service has admitted or started a worker.
		r := &api.CandidateSummary{RunID: a.RequestID, TaskID: a.TaskID, DomainTaskID: a.TaskID, TaskStatus: "in_progress", CandidateNumber: a.CandidateNumber, CandidateCreatedAt: a.CreatedAt, WorkflowStatus: "queued", WorkflowStage: "start.submit", ExecutionStatus: "pending", StageDiagnostic: a.Message}
		for _, profile := range state.ref.Presentation.Profiles {
			if profile.ID == a.ExecutionProfileID {
				selected := profile
				r.ExecutionProfile = &selected
				break
			}
		}
		if a.Status == "failed" || a.Status == "rejected" {
			r.WorkflowStatus, r.WorkflowStage, r.ExecutionStatus = "abandoned", "start.failed", "failed"
		}
		runs = append(runs, r)
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].CandidateNumber != 0 && runs[j].CandidateNumber != 0 && runs[i].CandidateNumber != runs[j].CandidateNumber {
			return runs[i].CandidateNumber < runs[j].CandidateNumber
		}
		if runs[i].CandidateCreatedAt.Equal(runs[j].CandidateCreatedAt) {
			return runs[i].RunID < runs[j].RunID
		}
		return runs[i].CandidateCreatedAt.Before(runs[j].CandidateCreatedAt)
	})
	return runs
}

func candidateFilterAllows(state projectState, filter int, taskID string) bool {
	for _, r := range candidateRuns(state, taskID) {
		if filterAllows(filter, rowRun, r) {
			return true
		}
	}
	return false
}

func (m Model) cycleCandidate(delta int) (tea.Model, tea.Cmd) {
	s := &m.projects[m.selected]
	row := m.selectedRow(m.selected)
	if row == nil {
		m.status = "当前任务没有候选"
		return m, nil
	}
	runs := candidateRuns(*s, row.taskID)
	if len(runs) == 0 {
		m.status = "当前任务没有候选"
		return m, nil
	}
	i := -1
	for n, r := range runs {
		if row.run != nil && r.RunID == row.run.RunID {
			i = n
			break
		}
	}
	i = (i + delta + len(runs)) % len(runs)
	if row.run != nil && runs[i].RunID == row.run.RunID {
		return m, nil
	}
	if !m.leaveAnswerSaved() {
		return m, nil
	}
	m.cancelWorkspace()
	if s.candidates == nil {
		s.candidates = map[string]string{}
	}
	s.candidates[row.taskID] = runs[i].RunID
	m.expandedDetailScroll = 0
	s.final = finalReplyReader{finalGeneration: s.final.finalGeneration + 1, bodyGeneration: s.final.bodyGeneration + 1, previewGeneration: s.final.previewGeneration + 1}
	s.activityGeneration++
	s.activityKey, s.activityError, s.activityBusy, s.activityReadAt = "", "", false, time.Time{}
	s.recentActivity, s.workerTransitions = nil, nil
	m.saveProjectView(m.selected)
	return m.refreshSelected(m.selected)
}

func candidateStartReason(s projectState, row *consoleRow, now time.Time) string {
	if row == nil || row.run == nil || row.native == nil || (row.native.Status != "in_progress" && row.run.WorkflowStage != "start.submit") {
		return "需要进行中的 Task"
	}
	if s.snapshot.NativeTaskError != "" || s.snapshot.HistoryError != "" || s.snapshot.NativeTasks == nil || row.native == nil {
		return "当前 Task/版本快照不可用，请刷新"
	}
	if valid, reason := api.CandidateFreshness(s.ref.Identity, s.snapshot, now); !valid {
		return reason
	}
	if rt := s.snapshot.Connection.Snapshot.Page; rt == nil || rt.Control.DispatchPaused || rt.Control.DispatchMode != "manual" {

		return "新增候选需要当前手动派发且未暂停"
	}
	if api.ExecutionConfigMismatch(s.ref, s.snapshot) {
		return api.ExecutionConfigChanged
	}
	for _, r := range candidateRuns(s, row.taskID) {
		if r.PR.State == "merged" {
			return "已知 PR 已合并，不能增加候选"
		}
	}
	count := 0
	for _, r := range candidateRuns(s, row.taskID) {
		if candidateParticipating(r) {
			count++
		}
	}
	if count >= 5 {
		return "当前参与数已达5个；启动失败历史不占名额"
	}
	return ""
}

func (m Model) startAdditionalCandidate() (tea.Model, tea.Cmd) {
	s, rt := m.readyControlTarget()
	if rt == nil {
		return m, nil
	}
	row := m.selectedRow(m.selected)
	if reason := candidateStartReason(*s, row, m.clock.Now()); reason != "" {
		m.status = reason
		return m, nil
	}
	if !hasCapability(rt, "start_candidate") || len(s.ref.Presentation.Profiles) == 0 {
		m.status = "服务或配置未提供新增候选能力"
		return m, nil
	}
	m.beginProfileChoice(&executionProfileChoice{action: "start_candidate", projectKey: s.ref.Key, taskID: row.taskID, instance: rt.Service.InstanceID, revision: rt.Control.Revision})
	return m, nil
}

func (m Model) submitCandidateAction(action string, p api.ControlPayload, id, instance string, revision uint64) (tea.Model, tea.Cmd) {
	s := &m.projects[m.selected]
	number := nextCandidateNumber(*s, p.TaskID)
	a := &actionState{CandidateNumber: number, CreatedAt: m.clock.Now(), Payload: p, RequestID: id, Action: action, TaskID: p.TaskID, TargetInstance: instance, Status: "submitting", UpdatedAt: m.clock.Now(), ExecutionProfileID: p.ExecutionProfileID, ExecutionProfileDigest: p.ExecutionProfileDigest}
	s.actions[id] = a
	if err := m.saveProjectView(m.selected); err != nil {
		delete(s.actions, id)
		m.status = "未提交：无法保存原请求身份"
		return m, nil
	}
	m.status = "提交候选动作 · request " + id
	m.selectNewCandidate(p.TaskID, id)
	return m, m.submitControl(m.selected, action, p.TaskID, id, instance, revision)
}

func nextCandidateNumber(s projectState, taskID string) int {
	number := 0
	for i, r := range candidateRuns(s, taskID) {
		number = max(number, r.CandidateNumber, i+1)
	}
	if s.snapshot.History != nil {
		for _, w := range s.snapshot.History.WorkflowRuns {
			if w.TaskID == taskID {
				number = max(number, w.CandidateNumber)
			}
		}
	}
	return number + 1
}

func (m *Model) selectNewCandidate(taskID, runID string) {
	m.leaveAnswer()
	m.cancelWorkspace()
	s := &m.projects[m.selected]
	if s.candidates == nil {
		s.candidates = map[string]string{}
	}
	s.candidates[taskID], s.selectedKey = runID, "task:"+taskID
	m.expandedDetailScroll = 0
	s.final = finalReplyReader{finalGeneration: s.final.finalGeneration + 1, bodyGeneration: s.final.bodyGeneration + 1, previewGeneration: s.final.previewGeneration + 1}
	s.activityGeneration++
	s.activityKey, s.activityError, s.activityBusy, s.activityReadAt = "", "", false, time.Time{}
	s.recentActivity, s.workerTransitions = nil, nil
	// Invalidate old body/activity responses immediately, before submission.
	*m, _ = m.refreshSelected(m.selected)
}

func (m *Model) saveDecisionDraft() {
	d := m.answer
	if d == nil {
		return
	}
	i := m.projectIndex(d.projectKey)
	if i < 0 {
		return
	}
	s := &m.projects[i]
	if s.view.Drafts == nil {
		s.view.Drafts = map[string]CandidateDraft{}
	}
	s.view.Drafts[d.identity+"\x00"+d.decisionID] = CandidateDraft{d.optionIndex, string(d.note)}
	m.saveProjectView(i)
}

func renderCandidates(m Model, s projectState, row *consoleRow) []string {
	runs := candidateRuns(s, row.taskID)
	lines := []string{fmt.Sprintf("候选 · %d 个（{ / } 切换）", len(runs))}
	for _, r := range runs {
		marker := "  "
		if row.run != nil && r.RunID == row.run.RunID {
			marker = "› "
		}
		profile := "冻结配置缺失"
		if r.ExecutionProfile != nil {
			profile = r.ExecutionProfile.Name + " [" + r.ExecutionProfile.ID + "]"
		}
		outcome := "参与实验"
		if r.WorkflowStage == "start.failed" {
			outcome = "启动失败历史 · 不占参与名额"
		}
		loser := false
		if r.TaskStatus == "discarded" {
			outcome = "Task 已丢弃/保留，需人工处置"
		} else if r.TaskStatus == "closed" {
			outcome = "交付定位缺失/保留资源"
			switch r.DeliveryOutcome {
			case "winner":
				outcome = "获胜/唯一交付"
			case "loser":
				outcome = "落选/保留资源"
				loser = true
			}
		}
		if r.WorkflowStatus == "abandoned" {
			outcome = "已停止参与 · 不占参与名额"
			if r.CandidateStopReason == "startup_failed" || r.WorkflowStage == "start.failed" {
				outcome = "启动失败历史 · 不占参与名额"
			}
		}
		if candidateBusinessOpen(r) && r.WorkflowStatus == "waiting_input" {
			outcome += " · 待决策/冲突请看原问题"
		}
		if r.PR.State == "merged" {
			switch {
			case r.TaskStatus == "discarded":
				outcome += " · 冲突：已丢弃 Task 的 PR 已合并，需人工处置"
			case loser:
				outcome += " · PR 已合并"
			case r.TaskStatus != "closed":
				outcome += " · PR 已合并，交付待核验"
			}
		}
		elapsed := "未知"
		if !r.CandidateCreatedAt.IsZero() {
			end := m.clock.Now()
			if r.CandidateEndedAt != nil {
				end = *r.CandidateEndedAt
			}
			elapsed = max(time.Duration(0), end.Sub(r.CandidateCreatedAt)).Round(time.Second).String()
		}
		lines = append(lines, safeTerminalText(marker+candidateLabel(s, r)+" · "+r.RunID+" · "+profile+" · "+runStatus(r)+" · "+outcome), safeTerminalText("  session "+displayDash(r.SessionID)+" · 资源 "+firstText(r.ResourceStatus, "未知/待核实")+" · 耗时 "+elapsed))
		if r.ExecutionProfile != nil {
			lines = append(lines, safeTerminalText("  冻结："+strings.Join(r.ExecutionProfile.ImplementationDisplay, " · ")))
		}
		if r.PR.URL != nil {
			lines = append(lines, "  PR："+prLink(*r.PR.URL, *r.PR.URL)+" · "+safeTerminalText(r.PR.State))
		}
		if r.Summary != nil {
			lines = append(lines, "  最终回复摘要："+safeTerminalText(*r.Summary)+"（完整正文在候选详情查看；[ / ] 选择历史回复）")
		}
	}
	lines = append(lines, "最多5个参与中候选；准备中的候选计入，明确启动失败历史不占名额。", "每次确认独立创建；不会自动补位。", "指标缺口：未提供评分/排名、token/费用比较。", "采样："+sampleTime(s.snapshot))
	if reason := candidateStartReason(s, row, m.clock.Now()); reason != "" {
		lines = append(lines, "b 新增不可用："+safeTerminalText(reason))
	} else {
		lines = append(lines, "b 选择不同冻结方案新增候选")
	}
	if row.run != nil {
		r := row.run
		if r.WorkflowStatus == "abandoned" {
			lines = append(lines, safeTerminalInlineText("已停止参与 · 原因："+displayDash(r.CandidateStopReason)+" · 操作者："+displayDash(r.CandidateStoppedBy)), "资源保留；执行状态为最后已知事实，不表示 worker 已停止。")
		}
		if reason := candidateStopReason(s, row.taskID, r.RunID, m.clock.Now()); reason != "" {
			lines = append(lines, "F9 停止不可用："+reason)
		} else {
			lines = append(lines, "F9 停止当前候选参与（保留历史和资源）")
		}
		started := "未知"
		if !r.CandidateCreatedAt.IsZero() {
			started = r.CandidateCreatedAt.Format("2006-01-02 15:04:05Z07:00")
		}
		lines = append(lines, "所选 workflow_run_id："+safeTerminalText(r.RunID), "execution/session："+safeTerminalText(r.ExecutionID+" / "+r.SessionID), fmt.Sprintf("候选版本：%d · 开始：%s", r.ControlRevision, started))
		for _, url := range r.PRHistory {
			lines = append(lines, "历史 PR："+prLink(url, url))
		}
		lines = append(lines, "差异定位："+safeTerminalText(stringValue(r.WorktreePath)+" · "+stringValue(r.BaseCommit)+".."+stringValue(r.WorktreeBranch)))
	}
	return lines
}
