package console

import (
	"fmt"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

var (
	styleFocus   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "30", Dark: "81"}).Bold(true)
	styleRunning = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "75"})
	styleWaiting = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "94", Dark: "221"}).Bold(true)
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "114"})
)

func stateText(text string) string {
	text = safeTerminalInlineText(text)
	switch {
	case strings.Contains(text, "失败") || strings.Contains(text, "阻断") || strings.Contains(text, "阻塞") || strings.Contains(text, "受阻") || strings.Contains(text, "错误"):
		return styleAlert.Render("! " + text)
	case strings.Contains(text, "未知") || strings.Contains(text, "核实") || strings.Contains(text, "离线") || strings.Contains(text, "缓存") || strings.Contains(text, "待") || strings.Contains(text, "交互"):
		return styleWaiting.Render("? " + text)
	case strings.Contains(text, "执行中") || strings.Contains(text, "进行中"):
		return styleRunning.Render("● " + text)
	case strings.Contains(text, "成功") || strings.Contains(text, "已完成") || strings.Contains(text, "已送达"):
		return styleSuccess.Render("✓ " + text)
	}
	return text
}

// The frame dimensions are shared by rendering and the visibility gate.
func paneFrame(lines []string, width, height int, focused bool) []string {
	if width < 4 || height < 3 {
		return fitLines(lines, width, height)
	}
	title := ""
	if len(lines) > 0 {
		title = lines[0]
		lines = lines[1:]
	}
	tl, tr, bl, br, h, v := "┌", "┐", "└", "┘", "─", "│"
	border := styleRule
	if focused {
		tl, tr, bl, br, h, v = "╔", "╗", "╚", "╝", "═", "║"
		border = styleFocus
		title = "▶ " + title
	}
	if os.Getenv("TERM") == "dumb" {
		tl, tr, bl, br, v = "+", "+", "+", "+", "|"
		h = "-"
		if focused {
			h = "="
			title = strings.TrimPrefix(title, "▶ ")
			title = "> " + title
		}
	}
	title = truncate(" "+title+" ", width-2)
	out := []string{border.Render(tl + title + strings.Repeat(h, max(0, width-2-lipgloss.Width(title))) + tr)}
	for _, line := range fitLines(lines, width-2, height-2) {
		out = append(out, border.Render(v)+padDisplay(line, width-2)+border.Render(v))
	}
	return append(out, border.Render(bl+strings.Repeat(h, width-2)+br))
}

func workspaceFooter(m Model, s projectState) []string {
	return fitFooter(renderFooter(m, s), m.width)
}
func fitFooter(text string, width int) []string {
	lines := wrapLines(strings.Split(text, "\n"), width)
	if len(lines) > 2 {
		lines = lines[:2]
		lines[1] = truncate(lines[1], max(1, width-9)) + " ? 更多"
	}
	return fitLines(lines, width, len(lines))
}

func profileProductLines(p api.ExecutionProfileSummary) []string {
	lines := []string{"执行方案：" + safeTerminalInlineText(firstText(p.Name, p.ID, "未核实")) + " · " + safeTerminalInlineText(firstText(p.Host, "未核实")+" · Agent："+firstText(p.Agent, "未核实"))}
	for _, field := range p.ImplementationDisplay {
		lines = append(lines, safeTerminalInlineText(field))
	}
	for _, field := range p.Details {
		if strings.Contains(field, "权限") || strings.HasPrefix(field, "项目要求") || strings.HasPrefix(field, "策略") || strings.HasPrefix(field, "文件") || strings.HasPrefix(field, "网络") {
			lines = append(lines, safeTerminalInlineText(field))
		}
	}
	return lines
}

func (m Model) workspaceHeights(height int) (int, int) {
	minimum := 6
	if m.answer != nil {
		minimum = answerMinimum(m) + 2
	}
	if m.profileChoice != nil {
		minimum = len(profileChoiceRequired(m, m.projects[m.selected])) + 4
	}
	if m.confirm != nil {
		minimum = len(confirmRequired(m, m.projects[m.selected])) + 2
	}
	if height < minimum+7 {
		return 0, height
	}
	overview := candidateOverviewLines(m, m.projects[m.selected], m.selectedRow(m.selected), max(1, detailPanelWidth(m)-2))
	top := min(len(overview)+1, height-minimum)
	return top, height - top
}

func (m Model) lowerGeometry() (int, int) {
	_, height := m.workspaceHeights(consoleContentHeight(m))
	return max(1, detailPanelWidth(m)-2), max(0, height-2)
}

func answerMinimum(m Model) int {
	n := 9
	if m.answer != nil && m.answer.error != "" {
		n++
	}
	if r := m.selectedRow(m.selected); r != nil && r.run != nil && r.run.EnvironmentBlocker != nil {
		n++
	}
	return n
}

func (m Model) operationVisible() bool {
	if m.selected < 0 || m.selected >= len(m.projects) || m.detailExpanded || m.operationDetails || m.help || m.answer != nil && m.answer.details {
		return false
	}
	if compactLayout(m) && m.projects[m.selected].focus == 0 {
		return false
	}
	width, height := m.lowerGeometry()
	if width < 32 {
		return false
	}
	required := 0
	switch {
	case m.answer != nil:
		// The frame adds a focus marker and spaces to its title. Counting rows
		// alone permits submission after truncating the Task/candidate identity.
		if lipgloss.Width(answerHeader(m.projects[m.selected], m.answer))+4 > width {
			return false
		}
		required = answerMinimum(m)
	case m.profileChoice != nil:
		lines := profileChoiceRequired(m, m.projects[m.selected])
		if len(lines) > 0 && lipgloss.Width(lines[0]) > width {
			return false
		}
		required = len(lines) + 2
	case m.confirm != nil:
		confirmLines := confirmRequired(m, m.projects[m.selected])
		if len(confirmLines) > 1 && lipgloss.Width(confirmLines[1]) > width {
			return false
		}
		required = len(confirmLines)
	}
	return height >= required
}

func (m *Model) beginProfileChoice(choice *executionProfileChoice) {
	s := &m.projects[m.selected]
	choice.profiles = api.CloneProjectRef(s.ref).Presentation.Profiles
	if row := m.selectedRow(m.selected); row != nil && row.native != nil {
		choice.taskVersion = row.native.Version
	}
	if s.snapshot.NativeTasks != nil {
		choice.graphRevision = s.snapshot.NativeTasks.GraphRevision
	}
	m.profileChoice = choice
	s.focus = 1
	s.lowerScroll = 0
	m.leaveRecovery()
}

func (m *Model) leaveRecovery() {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return
	}
	s := &m.projects[m.selected]
	if s.recoveryActive || s.recoveryBusy {
		s.recoveryGeneration++
	}
	s.recoveryActive = false
	s.recoveryBusy = false
}

func choiceProfiles(m Model, s projectState) []api.ExecutionProfileSummary {
	if m.profileChoice != nil && m.profileChoice.profiles != nil {
		return m.profileChoice.profiles
	}
	return s.ref.Presentation.Profiles
}

func profileChoiceRequired(m Model, s projectState) []string {
	c := m.profileChoice
	if c == nil {
		return nil
	}
	profiles := choiceProfiles(m, s)
	if c.index < 0 || c.index >= len(profiles) {
		return []string{"执行方案不可用"}
	}
	head := "开始任务 " + safeTerminalInlineText(c.taskID)
	if c.action == "start_candidate" {
		head = "新增候选 · Task " + safeTerminalInlineText(c.taskID)
	}
	lines := []string{head + " · " + safeTerminalInlineText(s.ref.Name)}
	lines = append(lines, profileProductLines(profiles[c.index])...)
	reason := profiles[c.index].UnavailableReason
	if executionConfigMismatch(s.ref, s.snapshot) {
		reason = executionConfigChanged
	}
	if reason != "" {
		lines = append(lines, "不可用："+safeTerminalInlineText(reason))
	}
	return append(lines, "Enter 确认并开始 · F8 完整方案 · Esc 取消")
}

func confirmRequired(m Model, s projectState) []string {
	c := m.confirm
	if c == nil {
		return nil
	}
	if c.action == "abandon_candidate" {
		target := "Task " + safeTerminalInlineText(c.taskID) + " / " + candidateLabel(s, &api.CandidateSummary{TaskID: c.taskID, RunID: c.runID})
		lines := []string{"不可撤销放弃候选确认 · " + safeTerminalInlineText(s.ref.Name), target}
		// Captured when confirmation opens; never substitute a newly selected run.
		lines = append(lines, strings.Split(c.basis, "\n")...)
		return append(lines, "退出实验不可撤销；worker 可能仍运行，停止独立核实。", "不会丢弃 Task 或删除资源；已知 PR 合并后禁止放弃。", "Enter 确认退出实验 · F8 完整身份 · Esc/n 取消")
	}
	return []string{"停止服务确认 · " + safeTerminalInlineText(s.ref.Name), "目标实例：" + safeTerminalInlineText(c.instance), "停止新派发，并等待 worker " + firstText(s.ref.Presentation.ShutdownGrace, "配置的宽限时间"), "随后按服务策略结束；工作现场保留。", "Enter 确认停止 · Esc/n 取消"}
}

func (m *Model) cancelWorkspace() {
	m.profileChoice = nil
	m.confirm = nil
	m.operationDetails = false
	if m.selected >= 0 && m.selected < len(m.projects) {
		s := &m.projects[m.selected]
		s.recoveryActive = false
		s.recoveryGeneration++
		s.recoveryBusy = false
		s.lowerScroll = 0
	}
}

func (m *Model) leaveAnswerSaved() bool {
	if m.answer == nil {
		return true
	}
	m.saveDecisionDraft()
	if err := m.saveProjectView(m.projectIndex(m.answer.projectKey)); err != nil {
		m.answer.error = err.Error()
		m.status = err.Error()
		return false
	}
	m.answer = nil
	return true
}

func taskAttention(s projectState, id string) string {
	if action := taskOperation(s, id); action != nil {
		return operationProgress(action)
	}
	best, priority := "", 0
	for _, r := range candidateRuns(s, id) {
		if historicalFailedRun(r) {
			continue
		}
		text, p := "", 0
		switch {
		case r.EnvironmentBlocker != nil:
			text, p = "环境受阻", 3
		case runNeedsAttention(r) && !waitingForDecision(r):
			text, p = "需核验", 3
		case pendingDecision(r) && waitingForDecision(r):
			text, p = "待回答", 2
		case waitingForMerge(r):
			text, p = "待合并", 1
		}
		if p > priority {
			best, priority = text+" "+candidateLabel(s, r), p
		}
	}
	return best
}

func shortPR(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return safeTerminalInlineText(raw)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) >= 2 && (parts[len(parts)-2] == "pull" || parts[len(parts)-2] == "pulls") {
		return "#" + parts[len(parts)-1]
	}
	return safeTerminalInlineText(raw)
}

func nextAction(m Model, s projectState, row *consoleRow) string {
	if row == nil {
		return "请选择任务"
	}
	if action := rowOperation(s, *row); action != nil {
		return operationProgress(action) + " · 正在跟踪原请求回执"
	}
	if row.run == nil {
		if s.candidates[row.taskID] != "" {
			return "原候选暂不可读取；刷新或 F6/F7 显式选择"
		}
		if reason := candidateActionReason(s, m.clock.Now()); reason != "" {
			return "开始不可用：" + displayActionReason(reason)
		}
		if row.kind == rowReady {
			return "s 选择方案并开始"
		}
		return "查看任务前置与阻塞原因（d）"
	}
	r := row.run
	if r.TaskStatus == "discarded" || r.Abandonment != nil {
		return taskNextAction(r)
	}
	if waitingForMerge(r) && !runNeedsAttention(r) {
		action := "审阅并人工合并 PR · f 完整回复"
		if pendingDecision(r) {
			if feedbackInProgress(s, r) {
				action += " · 查询原反馈请求回执"
			} else if strings.HasPrefix(decisionAnswerAvailability(s, row), "提交后") {
				action += " · a 可选反馈（继续工作）"
			}
		}
		return action
	}
	if r.EnvironmentBlocker != nil {
		return "Ctrl+E 重查环境 · Ctrl+O 原终端（按能力核验）"
	}
	if pendingDecision(r) && waitingForDecision(r) {
		if feedbackInProgress(s, r) {
			return "查询原答案请求回执；不重复提交"
		}
		reason := decisionAnswerAvailability(s, row)
		if strings.HasPrefix(reason, "提交后") {
			return "a/Enter 回答当前问题"
		}
		return "回答不可用：" + reason
	}
	if runNeedsAttention(r) {
		return "u 只读核验 · d 完整诊断"
	}
	return "当前无需操作 · 可查看最近动态"
}

func receiptSummary(s projectState, row *consoleRow) []string {
	if row == nil {
		return nil
	}
	var items []*actionState
	for _, a := range s.actions {
		run := a.RunID
		if a.EnvironmentTarget != nil {
			run = a.EnvironmentTarget.RunID
		}
		if run == "" {
			run = a.Payload.WorkflowRunID
		}
		if a.EnvironmentTarget != nil {
			if row.run != nil && run == row.run.RunID {
				items = append(items, a)
			}
			continue
		}
		if a.TaskID == row.taskID && (run == "" || row.run != nil && run == row.run.RunID || row.run == nil && a.Action == "start_task") {
			items = append(items, a)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if receiptPriority(items[i]) != receiptPriority(items[j]) {
			return receiptPriority(items[i]) > receiptPriority(items[j])
		}
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].RequestID < items[j].RequestID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	var lines []string
	for _, a := range items {
		if len(lines) >= 2 {
			break
		}
		if len(lines) > 0 && a.Resolved {
			continue
		}
		lines = append(lines, "操作回执："+stateText(safeTerminalInlineText(actionStatusLabel(a.Status)+" · "+shortSummary(a.Message, 100))))
		if a.Action == "answer_decision" && len(lines) < 2 && !receiptNeedsAttention(a) && (len(items) == 1 || !receiptNeedsAttention(items[1])) {
			lines = append(lines, "答案预览："+safeTerminalInlineText(shortSummary(a.DecisionDraftOptionID+" · "+a.DecisionDraftNote, 100)))
		}
	}
	return lines
}

func receiptNeedsAttention(a *actionState) bool {
	return receiptPriority(a) > 0
}

func receiptPriority(a *actionState) int {
	if !a.Resolved {
		return 3
	}
	if a.Status == "indeterminate" {
		return 2
	}
	if a.Status == "failed" || a.Status == "rejected" {
		return 1
	}
	return 0
}

func recoveryStale(s projectState, row *consoleRow) bool {
	r := s.recoveryReport
	rt := s.snapshot.Connection.Snapshot.Page
	return r != nil && (s.snapshot.Connection.State != api.ConnectionConnected || s.snapshot.Observation.State != api.SnapshotValid || row == nil || row.run == nil || row.run.RunID != r.RunID || row.taskID != r.TaskID || row.run.ControlRevision != r.RunRevision || rt == nil || rt.Service.InstanceID != r.ServiceInstanceID)
}
func recoverySummary(s projectState, row *consoleRow) []string {
	lines := []string{"运行与反馈 · 只读核验"}
	if row != nil {
		lines = append(lines, "Task "+row.taskID+" / "+candidateLabel(s, row.run))
	}
	if s.recoveryBusy {
		return append(lines, "正在核验原执行、工作目录、领取和 PR…")
	}
	if s.recoveryError != "" {
		return append(lines, styleAlert.Render("核验失败："+safeTerminalInlineText(s.recoveryError)), "r 重读报告 · Esc 返回动态")
	}
	r := s.recoveryReport
	if r == nil {
		return append(lines, "尚无报告 · u 获取原报告")
	}
	if recoveryStale(s, row) {
		lines = append(lines, styleWaiting.Render("报告已过期，不能证明当前操作资格；r 重新核验"))
	}
	lines = append(lines, "采样 "+r.SampledAt.Local().Format(timeLayout), "执行/交付："+safeTerminalInlineText(r.ExecutionStatus+" / "+r.DeliveryStatus), "原进程："+safeTerminalInlineText(r.ProcessCheck))
	for _, part := range []struct {
		name, err string
		present   bool
	}{{"工作目录", r.WorkspaceError, r.Workspace != nil}, {"领取", r.ClaimError, r.NativeClaim != nil}, {"PR", r.PRError, r.PR != nil}} {
		text := part.name + "：已读取"
		if part.err != "" {
			text = part.name + "：" + part.err
		} else if !part.present {
			text = part.name + "：无已核实记录"
		}
		lines = append(lines, safeTerminalInlineText(text))
	}
	if r.ConcurrentActionReason != "" {
		lines = append(lines, "操作限制："+safeTerminalInlineText(r.ConcurrentActionReason))
	}
	return append(lines, "下一步：r 重读 · F8 完整报告 · Esc 返回动态")
}

func (m Model) handleWorkspaceNavigation(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	s := &m.projects[m.selected]
	switch key.String() {
	case "tab", "shift+tab":
		s.focus = 1 - s.focus
		return m, nil, true
	case "up", "k", "down", "j", "pgup", "pgdown":
		if s.focus == 0 {
			delta := 1
			if key.String() == "up" || key.String() == "k" {
				delta = -1
			}
			if key.String() == "pgup" {
				delta = -8
			}
			if key.String() == "pgdown" {
				delta = 8
			}
			next, cmd := m.moveSelection(delta)
			return next, cmd, true
		}
	case "f6", "{":
		next, cmd := m.cycleCandidate(-1)
		return next, cmd, true
	case "f7", "}":
		next, cmd := m.cycleCandidate(1)
		return next, cmd, true
	case "left", "h":
		next, cmd := m.moveProject(-1)
		return next, cmd, true
	case "right", "l":
		next, cmd := m.moveProject(1)
		return next, cmd, true
	}
	return m, nil, false
}

func profileChoiceMatches(c *executionProfileChoice, s projectState, row *consoleRow) bool {
	rt := s.snapshot.Connection.Snapshot.Page
	if rt == nil || rt.Service.InstanceID != c.instance || rt.Control.Revision != c.revision || row == nil || row.taskID != c.taskID {
		return false
	}
	if (row.native == nil && c.taskVersion != 0) || (row.native != nil && row.native.Version != c.taskVersion) {
		return false
	}
	if (s.snapshot.NativeTasks == nil && c.graphRevision != 0) || (s.snapshot.NativeTasks != nil && s.snapshot.NativeTasks.GraphRevision != c.graphRevision) {
		return false
	}
	if c.index < 0 || c.index >= len(c.profiles) {
		return false
	}
	frozen := c.profiles[c.index]
	for _, p := range s.ref.Presentation.Profiles {
		if p.ID == frozen.ID {
			return reflect.DeepEqual(p, frozen)
		}
	}
	return false
}

func windowTooSmall(m Model) []string {
	return []string{"运行与反馈", "窗口过小，请调整后操作", "F8 完整阅读 · Esc 取消/返回；尚未发送"}
}

func candidateConfirmBasis(s projectState, r *api.CandidateSummary) string {
	if fixed := fixedRunExecutionProfile(s, r); fixed != nil {
		return strings.Join(profileProductLines(*fixed), "\n")
	}
	return "冻结方案未核实"
}

func candidateStrip(s projectState, row *consoleRow, width int) string {
	runs := candidateRuns(s, row.taskID)
	current := 0
	for i, r := range runs {
		if row.run != nil && r.RunID == row.run.RunID {
			current = i
		}
	}
	start := max(0, current-1)
	end := min(len(runs), start+3)
	var chips []string
	for _, r := range runs[start:end] {
		label := candidateLabel(s, r)
		if row.run != nil && r.RunID == row.run.RunID {
			label = "[" + label + "]"
		}
		chips = append(chips, label+" "+safeTerminalInlineText(candidateHumanStatus(s, r)))
	}
	if end-start < len(runs) {
		chips = append(chips, fmt.Sprintf("其余 %d · F6/F7", len(runs)-(end-start)))
	}
	return truncate(strings.Join(chips, " · "), width)
}

func (m Model) handleReaderKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "d", "enter":
		m.detailExpanded = false
		m.detailSection = ""
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		return m.scrollExpandedDetail(-1)
	case "down", "j":
		return m.scrollExpandedDetail(1)
	case "pgup":
		return m.scrollExpandedDetail(-8)
	case "pgdown":
		return m.scrollExpandedDetail(8)
	case "[", "]":
		if m.detailSection == "final" {
			delta := 1
			if key.String() == "[" {
				delta = -1
			}
			m.expandedDetailScroll = 0
			return m.cycleFinal(delta)
		}
	case "r":
		if m.detailSection == "final" {
			return m, m.readFinalReplies(m.selected, true)
		}
		return m, m.readProject(m.selected)
	}
	return m, nil
}

func (m Model) handleOperationDetails(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "f8", "esc":
		m.operationDetails = false
		m.expandedDetailScroll = 0
	case "up", "pgup":
		m.expandedDetailScroll = clampReadingScroll(m.expandedDetailScroll-4, len(wrapLines(operationDetailLines(m, m.projects[m.selected]), m.width)), consoleContentHeight(m))
	case "down", "pgdown":
		m.expandedDetailScroll = clampReadingScroll(m.expandedDetailScroll+4, len(wrapLines(operationDetailLines(m, m.projects[m.selected]), m.width)), consoleContentHeight(m))
	}
	return m, nil
}

func operationDetailLines(m Model, s projectState) []string {
	if m.profileChoice != nil {
		ps := choiceProfiles(m, s)
		i := m.profileChoice.index
		if i >= 0 && i < len(ps) {
			p := ps[i]
			lines := profileChoiceRequired(m, s)
			lines = append(lines, "ID："+safeTerminalInlineText(p.ID), "摘要："+safeTerminalInlineText(p.Digest))
			for _, f := range p.Details {
				lines = append(lines, safeTerminalText(f))
			}
			return lines
		}
	}
	if m.confirm != nil {
		return append(confirmRequired(m, s), "实例："+safeTerminalInlineText(m.confirm.instance), "run_id："+safeTerminalInlineText(m.confirm.runID), "request_id："+m.confirm.requestID)
	}
	if s.recoveryActive {
		local := m
		local.detailExpanded = true
		return renderRecoveryInspection(local, s, m.width, 1<<20)
	}
	return []string{fmt.Sprint("没有可阅读的操作详情")}
}

func clampReadingScroll(offset, lines, height int) int {
	return min(max(0, offset), max(0, lines-height))
}
