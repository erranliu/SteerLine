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
type CandidateReading struct {
	AttemptID, AnchorGroup, AnchorItem, ReadGroup, ReadItem, BodyKind string
	AnchorRevision, ReadRevision, BodyOffset                          int64
	AnchorLine, ReadLine, DetailScroll, LogScroll                     int
	BodyPages                                                         []int64
	Follow                                                            bool
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
	sort.Slice(runs, func(i, j int) bool {
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

func storeCandidateReading(s *projectState) {
	if s.readingIdentity == "" {
		return
	}
	if s.view.Reading == nil {
		s.view.Reading = map[string]CandidateReading{}
	}
	v := s.view
	s.view.Reading[s.readingIdentity] = CandidateReading{s.attemptID, v.AnchorGroup, v.AnchorItem, v.ReadGroup, v.ReadItem, s.bodyKind, v.AnchorRevision, v.ReadRevision, s.bodyViewOffset, v.AnchorLine, v.ReadLine, s.detailScroll, s.logScroll, append([]int64(nil), s.bodyPages...), v.Follow}
}

func switchCandidateReading(s *projectState, run *api.CandidateSummary) {
	if s.view.Candidates == nil {
		s.view.Candidates = map[string]string{}
	}
	s.view.Candidates[run.TaskID] = run.RunID
	id := candidateIdentity(run)
	if s.readingIdentity == id {
		return
	}
	if s.readingIdentity != "" {
		storeCandidateReading(s)
		clearReadingAnchor(s)
		s.attemptID = ""
		s.view.Follow = true
	} else if len(s.view.Reading) > 0 {
		clearReadingAnchor(s)
		s.attemptID = ""
		s.view.Follow = true
	}
	s.readingIdentity = id
	if v, ok := s.view.Reading[id]; ok {
		s.attemptID = v.AttemptID
		s.view.AnchorGroup, s.view.AnchorItem, s.view.ReadGroup, s.view.ReadItem = v.AnchorGroup, v.AnchorItem, v.ReadGroup, v.ReadItem
		s.view.AnchorRevision, s.view.ReadRevision, s.view.AnchorLine, s.view.ReadLine = v.AnchorRevision, v.ReadRevision, v.AnchorLine, v.ReadLine
		s.view.BodyOffset, s.view.BodyKind, s.view.Follow = v.BodyOffset, v.BodyKind, v.Follow
		s.bodyViewOffset, s.bodyKind, s.bodyPages = v.BodyOffset, v.BodyKind, append([]int64(nil), v.BodyPages...)
		s.detailScroll, s.logScroll = v.DetailScroll, v.LogScroll
	}
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
	storeCandidateReading(s)
	if s.view.Candidates == nil {
		s.view.Candidates = map[string]string{}
	}
	s.view.Candidates[row.taskID] = runs[i].RunID
	switchCandidateReading(s, runs[i])
	m.expandedDetailScroll = 0
	m.saveProjectView(m.selected)
	return m.refreshSelectedLogs(m.selected)
}

func candidateStartReason(s projectState, row *consoleRow, now time.Time) string {
	if row == nil || row.run == nil || row.run.TaskStatus != "in_progress" {
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
		if r.Abandonment == nil && r.FinishTaskStatus != "completed" {
			count++
		}
	}
	if count >= 5 {
		return "开始检查：当前参与数已达5个；不接受新开始（无硬名额、队列或自动补位）"
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
	m.profileChoice = &executionProfileChoice{action: "start_candidate", projectKey: s.ref.Key, taskID: row.taskID, instance: rt.Service.InstanceID, revision: rt.Control.Revision}
	return m, nil
}

func candidateAbandonReason(s projectState, row *consoleRow) string {
	if row == nil || row.run == nil || row.native == nil {

		return "缺少精确候选/Task版本"
	}
	if s.snapshot.NativeTaskError != "" || s.snapshot.HistoryError != "" {
		return "当前候选/Task版本查询不可用；请刷新"
	}
	if row.run.Abandonment != nil {

		return "候选已退出实验"
	}
	if row.run.TaskStatus != "in_progress" || row.run.FinishTaskStatus == "completed" {
		return "任务已完成，不能放弃"
	}
	for _, r := range candidateRuns(s, row.taskID) {
		if r.PR.State == "merged" {
			return "已知 PR 已合并，禁止放弃"
		}
	}
	return ""
}

func (m Model) prepareCandidateAbandon() (tea.Model, tea.Cmd) {
	s, rt := m.readyControlTarget()
	if rt == nil {
		return m, nil
	}
	row := m.selectedRow(m.selected)
	if reason := candidateAbandonReason(*s, row); reason != "" {
		m.status = reason
		return m, nil
	}
	if !hasCapability(rt, "abandon_candidate") {
		m.status = "服务未声明 abandon_candidate 能力"
		return m, nil
	}
	p := api.ControlPayload{TaskID: row.taskID, WorkflowRunID: row.run.RunID, ExpectedTaskVersion: row.native.Version, ExpectedCandidateVersion: row.run.ControlRevision, Reason: "人工方案比较：不可撤销退出实验"}
	m.confirm = &confirmAction{projectKey: s.ref.Key, instance: rt.Service.InstanceID, revision: rt.Control.Revision, action: "abandon_candidate", taskID: row.taskID, runID: row.run.RunID, requestID: newRequestID(), payload: p, basis: strings.Join(renderFixedRunExecutionProfile(*s, row.run), "\n")}
	return m, nil
}

func (m Model) submitCandidateAction(action string, p api.ControlPayload, id, instance string, revision uint64) (tea.Model, tea.Cmd) {
	s := &m.projects[m.selected]
	for _, a := range s.actions {
		if a.Action == action && a.TaskID == p.TaskID && !a.Resolved {
			m.status = "原请求仍在核实 · " + a.RequestID
			return m, nil
		}
	}
	a := &actionState{Payload: p, RequestID: id, Action: action, TaskID: p.TaskID, RunID: p.WorkflowRunID, TargetInstance: instance, Status: "submitting", UpdatedAt: m.clock.Now()}
	s.actions[id] = a
	if err := m.saveProjectView(m.selected); err != nil {
		delete(s.actions, id)
		m.status = "未提交：无法保存原请求身份"
		return m, nil
	}
	m.status = "提交候选动作 · request " + id
	return m, m.submitControl(m.selected, action, p.TaskID, id, instance, revision)
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
		loser := false
		if r.Abandonment != nil {
			outcome = "历史/已放弃"
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
		if r.FinishTaskStatus == "waiting_decision" {
			outcome += " · 待决策/冲突请看原问题"
		}
		if r.PR.State == "merged" {
			switch {
			case r.Abandonment != nil:
				outcome += " · 冲突：已放弃候选 PR 已合并，需人工核实"
			case loser:
				outcome += " · 冲突：非交付候选 PR 已合并，保留原唯一交付"
			case r.TaskStatus != "closed":
				merged := 0
				for _, candidate := range runs {
					if candidate.PR.State == "merged" {
						merged++
					}
				}
				if merged > 1 {
					outcome += " · 冲突：多个候选 PR 已合并，需人工处置"
				} else {
					outcome += " · PR 已合并，交付待核验"
				}
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
			lines = append(lines, "  PR："+safeTerminalText(*r.PR.URL)+" · "+safeTerminalText(r.PR.State))
		}
		if r.Summary != nil {
			lines = append(lines, "  最终回复摘要："+safeTerminalText(*r.Summary)+"（完整正文按 f/n 读取规范记录）")
		}
	}
	lines = append(lines, "新增只做开始时5个参与检查；并发偶尔超额照实展示。", "无硬名额、预留、队列或自动补位。", "指标缺口：未提供评分/排名、token/费用比较。", "采样："+sampleTime(s.snapshot))
	if reason := candidateStartReason(s, row, m.clock.Now()); reason != "" {
		lines = append(lines, "b 新增不可用："+safeTerminalText(reason))
	} else {
		lines = append(lines, "b 选择不同冻结方案新增候选")
	}
	if reason := candidateAbandonReason(s, row); reason != "" {
		lines = append(lines, "z 放弃不可用："+safeTerminalText(reason))
	} else {
		lines = append(lines, "z 不可撤销退出实验（停止独立核实）")
	}
	if row.run != nil {
		r := row.run
		started := "未知"
		if !r.CandidateCreatedAt.IsZero() {
			started = r.CandidateCreatedAt.Format("2006-01-02 15:04:05Z07:00")
		}
		lines = append(lines, "所选 workflow_run_id："+safeTerminalText(r.RunID), "execution/session："+safeTerminalText(r.ExecutionID+" / "+r.SessionID), fmt.Sprintf("候选版本：%d · 开始：%s", r.ControlRevision, started))
		if r.Abandonment != nil {
			lines = append(lines, "不可撤销退出实验；worker 停止独立核实，不再反馈。")
		}
		for _, url := range r.PRHistory {
			lines = append(lines, "历史 PR："+safeTerminalText(url))
		}
		lines = append(lines, "差异定位："+safeTerminalText(stringValue(r.WorktreePath)+" · "+stringValue(r.BaseCommit)+".."+stringValue(r.WorktreeBranch)))
	}
	return lines
}
