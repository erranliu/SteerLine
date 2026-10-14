package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

type Options struct {
	RunID           string
	Clients         map[string]ConsoleClient
	Clock           Clock
	ApplicationHome string
	ApplicationBin  string
	ApplicationEtc  string
}

type rowKind int

const (
	rowReady rowKind = iota
	rowRun
	rowNative
)

type consoleRow struct {
	key       string
	kind      rowKind
	taskID    string
	title     string
	run       *api.CandidateSummary
	candidate *api.TaskCandidate
	native    *api.TaskDetail
}

type actionState struct {
	StopSelectionPending   bool
	CandidateNumber        int       `json:"candidate_number,omitempty"`
	CreatedAt              time.Time `json:"created_at,omitempty"`
	DecisionIdentity       string
	Payload                api.ControlPayload
	EnvironmentTarget      *api.HostActionTarget
	RequestID              string
	Action                 string
	TaskID                 string
	ExecutionProfileID     string
	ExecutionProfileDigest string
	RunID                  string
	DecisionID             string
	DecisionDraftOptionID  string
	DecisionDraftNote      string
	DecisionRevision       uint64
	DecisionVersion        uint64
	Status                 string
	Message                string
	TargetInstance         string
	InFlight               bool
	Resolved               bool
	UpdatedAt              time.Time
}

type decisionAnswerDialog struct {
	request         api.DecisionRequest
	details         bool
	identity        string
	projectKey      string
	instance        string
	runID           string
	decisionID      string
	runRevision     uint64
	decisionVersion uint64
	sourceAttemptID string
	recoveryVersion uint64
	optionIndex     int
	note            []rune
	cursor          int
	noteFocus       bool
	scroll          int
	error           string
}

type projectState struct {
	client ConsoleClient
	ref    ProjectRef
	view   ProjectView

	snapshot             ProjectSnapshot
	snapshotLoaded       bool
	refreshing           bool
	nativeSessionOpening bool
	refreshID            uint64
	lastRefresh          time.Time
	actionPollUntil      time.Time

	selectedKey        string
	selectionHold      bool
	filter             int
	focus              int // 0: task list, 1: lower pane/options, 2: answer text
	lowerScroll        int
	feedbackKey        string
	activityKey        string
	activityGeneration uint64
	activityBusy       bool
	activityReadAt     time.Time
	recentActivity     []activitySummary
	workerTransitions  []workerTransition
	activityError      string

	actions map[string]*actionState

	final                finalReplyReader
	detailScroll         int
	candidates           map[string]string
	recoveryRunID        string
	recoveryReport       *api.RecoveryInspection
	recoveryError        string
	recoveryBusy         bool
	recoveryGeneration   uint64
	recoveryBlockerIndex int
	recoveryActive       bool
}

type confirmAction struct {
	payload        api.ControlPayload
	candidateLabel string
	profile        string
	projectKey     string
	instance       string
	revision       uint64
	action         string
	taskID         string
	requestID      string
}

type executionProfileChoice struct {
	action        string
	projectKey    string
	taskID        string
	instance      string
	revision      uint64
	index         int
	profiles      []api.ExecutionProfileSummary
	taskVersion   uint64
	graphRevision uint64
}

type Model struct {
	ctx context.Context

	clock          Clock
	projects       []projectState
	selected       int
	applicationEtc string
	width          int
	height         int
	generation     uint64
	refreshID      uint64

	profile  *viewProfile
	prefs    ViewFile
	prefsErr string

	confirm              *confirmAction
	profileChoice        *executionProfileChoice
	answer               *decisionAnswerDialog
	answerCheck          *decisionEligibilityCheck
	help                 bool
	detailSection        string
	detailExpanded       bool
	expandedDetailScroll int
	operationDetails     bool
	status               string

	runFocus string
}

func NewModel(ctx context.Context, refs []ProjectRef, options Options) (Model, error) {
	seen := map[string]bool{}
	for _, ref := range refs {
		if seen[ref.Key] {
			return Model{}, errors.New("duplicate project view key")
		}
		seen[ref.Key] = true
		if ref.Error != "" {
			continue
		}
		client := options.Clients[ref.Key]
		if client == nil {
			return Model{}, fmt.Errorf("project %s requires an injected product client", ref.Name)
		}
		actual := client.Project()
		if actual.ConfigPath != ref.ConfigPath || actual.Identity != ref.Identity || actual.RunDir != ref.RunDir {
			return Model{}, errors.New("injected client belongs to another project view")
		}
	}

	profile, prefs, diagnostic, err := acquireViewProfile(options.ApplicationHome)
	if err != nil {
		return Model{}, err
	}

	if options.Clock == nil {
		options.Clock = realClock{}
	}
	m := Model{ctx: ctx, clock: options.Clock, profile: profile, prefs: prefs, prefsErr: diagnostic, runFocus: options.RunID, selected: -1, applicationEtc: options.ApplicationEtc}
	for _, ref := range refs {
		view, exists := prefs.Projects[ref.Key]
		if !exists {
			view = ProjectView{}
		}
		if view.Outstanding == nil {
			view.Outstanding = map[string]string{}
		}
		if view.Actions == nil {
			view.Actions = map[string]SavedAction{}
		}
		state := projectState{client: options.Clients[ref.Key], ref: ref, view: view, candidates: map[string]string{}, actions: map[string]*actionState{}}
		for requestID, saved := range view.Actions {
			_, outstanding := view.Outstanding[requestID]
			if saved.Status == "submitting" {
				saved.Status = "indeterminate"
			}
			message := "已恢复历史控制结果"
			if outstanding {
				message = "控制台重开后重新查询此 request_id"
			}
			state.actions[requestID] = &actionState{StopSelectionPending: saved.StopSelectionPending, CandidateNumber: saved.CandidateNumber, CreatedAt: saved.CreatedAt, RequestID: requestID, Action: saved.Action, TaskID: saved.TaskID, ExecutionProfileID: saved.ExecutionProfileID, ExecutionProfileDigest: saved.ExecutionProfileDigest, Status: saved.Status, Message: message, RunID: saved.RunID, DecisionID: saved.DecisionID, TargetInstance: saved.TargetInstance, Resolved: !outstanding, UpdatedAt: saved.UpdatedAt, Payload: saved.Payload, EnvironmentTarget: saved.EnvironmentTarget, DecisionIdentity: saved.DecisionIdentity, DecisionDraftOptionID: saved.DecisionDraftOptionID, DecisionDraftNote: saved.DecisionDraftNote, DecisionRevision: saved.DecisionRevision, DecisionVersion: saved.DecisionVersion}
		}
		m.projects = append(m.projects, state)
	}
	if len(m.projects) > 0 {
		m.selected = 0

	}
	return m, nil
}

func Run(ctx context.Context, refs []ProjectRef, options Options) (err error) {
	m, err := NewModel(ctx, refs, options)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, m.Close()) }()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err = p.Run()
	return err
}

// Close releases only UI/client resources; it never sends a stop command.
func (m Model) Close() error {
	m.profile.close()
	var err error
	for _, state := range m.projects {
		if state.client != nil {
			err = errors.Join(err, state.client.Close())
		}
	}
	return err
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{waitTick(m.clock, 500*time.Millisecond)}
	if len(m.projects) > 0 {
		for i := range m.projects {
			if i == m.selected {
				cmds = append(cmds, m.readProject(i))
			}
		}
	}
	return tea.Batch(cmds...)
}

func waitTick(clock Clock, d time.Duration) tea.Cmd {
	return func() tea.Msg { return tickMsg(<-clock.After(d)) }
}

func (m Model) readProject(index int) tea.Cmd {
	if index < 0 || index >= len(m.projects) {
		return nil
	}
	ref := m.projects[index].ref
	selectedTaskID := selectedNativeTaskID(m.projects[index])
	previous := m.projects[index].snapshot
	generation := m.generation
	m.projects[index].refreshID++
	id := m.projects[index].refreshID
	m.projects[index].refreshing = true
	query := m.projects[index].client
	ctx := m.ctx
	return func() tea.Msg {
		if query == nil {
			return projectMsg{key: ref.Key, identity: ref.Identity, generation: generation, requestID: id, snapshot: ProjectSnapshot{TaskError: ref.Error, HistoryError: ref.Error}}
		}
		return projectMsg{key: ref.Key, identity: ref.Identity, generation: generation, requestID: id, snapshot: query.ReadProject(ctx, selectedTaskID, previous)}
	}
}

func selectedNativeTaskID(state projectState) string {
	if state.ref.Presentation.TaskAuthority != "native" {
		return ""
	}
	if strings.HasPrefix(state.selectedKey, "task:") {
		return strings.TrimPrefix(state.selectedKey, "task:")
	}
	return ""
}

func (m Model) submitControl(index int, action, taskID, requestID string, instance string, revision uint64) tea.Cmd {
	state := &m.projects[index]
	ref := state.ref
	payload := api.ControlPayload{}
	switch action {
	case "start_candidate", "stop_candidate":
		payload = state.actions[requestID].Payload
	case "open_terminal", "recheck_environment":
		if pending := state.actions[requestID]; pending != nil {
			payload.Environment = pending.EnvironmentTarget
		}
	case "start_task":
		profile := state.actions[requestID]
		if profile != nil && profile.ExecutionProfileID != "" {
			payload = profile.Payload
			payload.TaskID, payload.ExecutionProfileID, payload.ExecutionProfileDigest = taskID, profile.ExecutionProfileID, profile.ExecutionProfileDigest
		} else {
			payload = api.ControlPayload{TaskID: taskID}
		}
	case "set_dispatch_mode":
		payload = api.ControlPayload{Mode: taskID}
	}
	query := state.client
	ctx := m.ctx
	return func() tea.Msg {
		request, err := query.SubmitControl(ctx, api.ControlInput{Action: api.ControlAction(action), Payload: payload, RequestID: requestID, InstanceID: instance, ExpectedRevision: &revision})
		return actionSubmittedMsg{projectKey: ref.Key, identity: ref.Identity, instance: instance, requestID: requestID, taskID: taskID, action: action, request: request, err: err}
	}
}

func (m Model) pollAction(index int, action *actionState) tea.Cmd {
	state := &m.projects[index]
	ref := state.ref
	query := state.client
	ctx := m.ctx
	requestID := action.RequestID
	if query == nil {
		return func() tea.Msg {
			return actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: requestID, err: errors.New(firstText(ref.Error, "product client is unavailable"))}
		}
	}
	if action.Action == "answer_decision" {
		decisionQuery := query
		ok := query != nil
		return func() tea.Msg {
			if !ok {
				return actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: requestID, err: errors.New("decision command query is unavailable")}
			}
			command, err := decisionQuery.Command(ctx, requestID)
			if err != nil {
				return actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: requestID, err: err}
			}
			return actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: requestID, command: &command}
		}
	}
	return func() tea.Msg {
		receipt, err := query.Receipt(ctx, requestID)
		if err == nil {
			return actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: requestID, receipt: receipt}
		}
		request, requestErr := query.Request(ctx, requestID)
		return actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: requestID, request: request, err: errors.Join(err, requestErr)}
	}
}

type tickMsg time.Time

type projectMsg struct {
	key, identity         string
	generation, requestID uint64
	snapshot              ProjectSnapshot
}

type actionSubmittedMsg struct {
	projectKey, identity, instance, requestID, taskID, action string
	request                                                   api.ControlRequest
	command                                                   *api.CommandReceipt
	err                                                       error
}

func (m Model) submitDecisionCommand(index int, answer api.DecisionAnswer, requestedBy, commandID string) tea.Cmd {
	state := &m.projects[index]
	ref := state.ref
	query := state.client
	ok := query != nil
	ctx := m.ctx
	return func() tea.Msg {
		if !ok {
			return actionSubmittedMsg{projectKey: ref.Key, identity: ref.Identity, instance: answer.ServiceInstanceID, requestID: commandID, taskID: "", action: "answer_decision", err: errors.New("decision command submission is unavailable")}
		}
		command, _, err := query.SubmitDecision(ctx, api.DecisionInput{Answer: answer, RequestedBy: requestedBy, CommandID: commandID})
		message := actionSubmittedMsg{projectKey: ref.Key, identity: ref.Identity, instance: answer.ServiceInstanceID, requestID: commandID, taskID: answer.RunID, action: "answer_decision", err: err}
		if err == nil {
			message.command = &command
		}
		return message
	}
}

func (m Model) answerSelectedDecision() (tea.Model, tea.Cmd) {
	return m.prepareDecisionAnswer(false)
}

func (m Model) prepareDecisionAnswer(qualified bool) (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	row := m.selectedRow(m.selected)
	if row == nil || row.run == nil || row.run.Decision == nil || row.run.Decision.Status != "pending" {
		m.status = "请选择带有 pending 决策的问题"
		return m, nil
	}
	if feedbackInProgress(*state, row.run) {
		m.status = candidateLabel(*state, row.run) + " 的反馈已提交；等待原请求回执"
		return m, nil
	}
	reason := decisionAnswerAvailability(*state, row)
	if !strings.HasPrefix(reason, "提交后") && row.run.EnvironmentBlocker == nil {
		m.status = reason
		return m, nil
	}
	eligibility := state.client
	ok := eligibility != nil
	if !ok {
		m.status = "当前控制客户端不支持决策资格核验"
		return m, nil
	}
	runtimeState := state.snapshot.Connection.Snapshot.Page
	if runtimeState == nil {
		m.status = "缺少当前服务实例快照"
		return m, nil
	}
	if !qualified {
		check := decisionEligibilityCheck{ID: newRequestID(), ProjectKey: state.ref.Key, RepositoryIdentity: state.ref.Identity, CandidateIdentity: candidateIdentity(row.run), InstanceID: state.snapshot.Connection.Snapshot.Page.Service.InstanceID, RunID: row.run.RunID, DecisionID: row.run.Decision.ID, RunRevision: row.run.ControlRevision, DecisionVersion: row.run.Decision.Version, AttemptID: row.run.AttemptID, RecoveryVersion: row.run.RecoveryVersion}
		m.answerCheck = &check
		m.status = "正在核验原问题的回答资格；可继续导航或取消"
		candidate := *row.run
		ctx := m.ctx
		return m, func() tea.Msg {
			return decisionEligibilityMsg{check: check, err: eligibility.DecisionEligibility(ctx, &candidate)}
		}
	}
	decision := row.run.Decision
	m.leaveRecovery()
	optionIndex := 0
	if len(decision.Request.Options) == 0 {
		optionIndex = -1
	} else if decision.Request.Recommended != "" {
		for i, option := range decision.Request.Options {
			if option.ID == decision.Request.Recommended {
				optionIndex = i
				break
			}
		}
	}
	m.answer = &decisionAnswerDialog{identity: candidateIdentity(row.run), projectKey: state.ref.Key, instance: runtimeState.Service.InstanceID, runID: row.run.RunID, decisionID: decision.ID, runRevision: row.run.ControlRevision, decisionVersion: decision.Version, sourceAttemptID: row.run.AttemptID, recoveryVersion: row.run.RecoveryVersion, optionIndex: optionIndex}
	m.answer.request = decision.Request
	m.answer.request.Options = append([]api.DecisionOption(nil), decision.Request.Options...)
	m.answer.noteFocus = len(decision.Request.Options) == 0
	if draft, ok := state.view.Drafts[m.answer.identity+"\x00"+decision.ID]; ok {
		m.answer.optionIndex, m.answer.note, m.answer.cursor, m.answer.noteFocus = draft.OptionIndex, []rune(draft.Note), len([]rune(draft.Note)), true
	}
	state.focus = 1
	state.lowerScroll = 0
	if m.answer.noteFocus {
		state.focus = 2
	}
	m.detailExpanded = false
	m.status = "决策草稿已打开；推荐选项只是预选，需要显式提交"
	return m, nil
}

func (m Model) submitDecisionAnswer() (tea.Model, tea.Cmd) {
	dialog := m.answer
	if dialog == nil {
		return m, nil
	}
	index := m.projectIndex(dialog.projectKey)
	if index < 0 || index != m.selected {
		dialog.error = "项目已变化；草稿保留，请返回原项目"
		return m, nil
	}
	state := &m.projects[index]
	runtimeState := state.snapshot.Connection.Snapshot.Page
	var run *api.CandidateSummary
	if runtimeState != nil {
		for _, candidate := range runtimeState.Runs {
			if candidate != nil && candidate.RunID == dialog.runID {
				run = candidate
				break
			}
		}
	}
	if runtimeState == nil || runtimeState.Service.InstanceID != dialog.instance || run == nil || run.ControlRevision != dialog.runRevision || run.Decision == nil || run.Decision.ID != dialog.decisionID || run.Decision.Version != dialog.decisionVersion || run.Decision.Status != "pending" {
		dialog.error = "stale_revision：问题或运行身份已变化；草稿已保留，请刷新并核对原问题"
		return m, nil
	}
	if dialog.identity != "" && candidateIdentity(run) != dialog.identity || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		dialog.error = "原候选身份已变化或任务已结束；草稿保留"
		return m, nil
	}
	if dialog.sourceAttemptID != run.AttemptID || dialog.recoveryVersion != run.RecoveryVersion {
		dialog.error = "原 Attempt 或恢复版本已变化；草稿保留，请刷新并核对原问题"
		return m, nil
	}
	if run.EnvironmentBlocker != nil {
		dialog.error = "环境阻断：" + run.EnvironmentBlocker.Fact.Reason + "；未发送，解除后请重新提交"
		return m, nil
	}
	for _, pending := range state.actions {
		if pending.Action == "answer_decision" && pending.DecisionID == dialog.decisionID && !pending.Resolved {
			dialog.error = "原答案请求仍在核实：" + pending.RequestID
			return m, nil
		}
	}
	request := run.Decision.Request
	optionID := ""
	custom := len(request.Options) == 0
	if len(request.Options) > 0 {
		if dialog.optionIndex >= 0 && dialog.optionIndex < len(request.Options) {
			optionID = request.Options[dialog.optionIndex].ID
		} else if request.AllowFreeText && dialog.optionIndex == len(request.Options) {
			custom = true
		} else {
			dialog.error = "请选择一个声明选项"
			return m, nil
		}
	}
	note := strings.TrimSpace(string(dialog.note))
	if len([]rune(note)) > 4000 {
		dialog.error = fmt.Sprintf("补充说明超过 4000 字符上限（当前 %d）；内容未截断，请编辑后再提交", len([]rune(note)))
		return m, nil
	}
	if custom && note == "" {
		dialog.error = "自定义答复需要填写非空说明"
		return m, nil
	}
	requestID := newRequestID()
	answer := api.DecisionAnswer{DecisionID: dialog.decisionID, RunID: dialog.runID, OptionID: optionID, Note: note, ExpectedDecisionVersion: dialog.decisionVersion, ExpectedRunRevision: dialog.runRevision, SourceAttemptID: dialog.sourceAttemptID, ExpectedRecoveryVersion: dialog.recoveryVersion, ServiceInstanceID: dialog.instance}
	action := &actionState{RequestID: requestID, Action: "answer_decision", TaskID: run.TaskID, RunID: run.RunID, DecisionID: dialog.decisionID, DecisionIdentity: dialog.identity, DecisionDraftOptionID: optionID, DecisionDraftNote: string(dialog.note), DecisionRevision: dialog.runRevision, DecisionVersion: dialog.decisionVersion, Status: "submitting", Message: "保存 command_id 并提交答案", TargetInstance: dialog.instance, UpdatedAt: m.clock.Now()}
	state.actions[requestID] = action
	state.view.Outstanding[requestID] = "decision:" + run.RunID
	if err := m.saveProjectView(index); err != nil {
		delete(state.actions, requestID)
		delete(state.view.Outstanding, requestID)
		dialog.error = "未提交答案：无法先保存 command_id；请检查 UI 视图目录权限"
		return m, nil
	}
	m.answer = nil
	state.focus = 1
	state.lowerScroll = 0
	m.detailExpanded = false
	m.status = candidateLabel(*state, run) + " 反馈已提交；等待保存/送达回执"
	return m, m.submitDecisionCommand(index, answer, "", requestID)
}

type actionReceiptMsg struct {
	projectKey, identity, requestID string
	receipt                         *api.ControlReceipt
	request                         *api.ControlRequest
	command                         *api.CommandReceipt
	err                             error
}

type recoveryInspectionMsg struct {
	projectKey, identity, runID string
	generation                  uint64
	report                      api.RecoveryInspection
	err                         error
}

func remapDecisionActionID(state *projectState, action *actionState, oldID, newID string) {
	if state == nil || action == nil || oldID == newID || newID == "" {
		return
	}
	delete(state.actions, oldID)
	delete(state.view.Outstanding, oldID)
	action.RequestID = newID
	state.actions[newID] = action
	state.view.Outstanding[newID] = "decision:" + action.RunID
}

func applyDecisionCommandResult(state *projectState, action *actionState, command *api.CommandReceipt, commandErr error) bool {
	if state == nil || action == nil {
		return false
	}
	if commandErr != nil {
		diagnostic := commandErr.Error()
		definite := strings.Contains(diagnostic, "stale_revision") || strings.Contains(diagnostic, "stale_instance") || strings.Contains(diagnostic, "answer_conflict") || strings.Contains(diagnostic, "unsupported") || strings.Contains(diagnostic, "exceeds") || strings.Contains(diagnostic, "requires") || strings.Contains(diagnostic, "not waiting")
		if definite {
			action.Status, action.Message, action.Resolved = "rejected", diagnostic, true
			return true
		}
		action.Status, action.Message, action.Resolved = "indeterminate", "答案命令结果待确认；继续核实原 command_id："+diagnostic, false
		return false
	}
	if command == nil || command.CommandID != action.RequestID || command.Kind != action.Action || command.TargetID != action.DecisionID || viewCanonical(command.ProjectIdentity) != viewCanonical(state.ref.Identity) {
		action.Status, action.Message, action.Resolved = "indeterminate", "决策命令回执身份不匹配；保留原 command_id 并停止自动提交", true
		return false
	}
	action.Status = command.Status
	switch command.Status {
	case "pending", "in_flight":
		action.Message, action.Resolved = "答案已提交；等待服务接受", false
	case "accepted":
		action.Message = "答案已保存；服务将单独核验原执行与续跑条件"
		action.Resolved = true
		return true
	case "rejected":
		code, reason := "rejected", "服务拒绝了这次答案"
		result := struct{ Code, Error string }{command.ResultCode, command.ResultError}
		if true {
			if result.Code != "" {
				code = result.Code
			}
			if result.Error != "" {
				reason = result.Error
			}
		}
		action.Message, action.Resolved = code+"："+reason, true
		return true
	default:
		action.Status, action.Message, action.Resolved = "indeterminate", "决策命令状态未知；保留原 command_id", false
	}
	return false
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		return m.updateWindowSize(msg)
	case tickMsg:
		return m.updateTick()
	case projectMsg:
		return m.updateProject(msg)
	case recentActivityMsg:
		return m.updateRecentActivity(msg)
	case recoveryInspectionMsg:
		return m.updateRecoveryInspection(msg)
	case decisionEligibilityMsg:
		return m.updateDecisionEligibility(msg)
	case finalRepliesMsg:
		return m.updateFinalReplies(msg)
	case bodyMsg:
		return m.updateBody(msg)
	case finalPreviewMsg:
		return m.updateFinalPreview(msg)
	case actionSubmittedMsg:
		return m.updateActionSubmitted(msg)
	case nativeSessionOpenedMsg:
		return m.applyNativeSessionOpened(msg)
	case actionReceiptMsg:
		return m.updateActionReceipt(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func controlRequestMatchesAction(request *api.ControlRequest, action *actionState, repositoryIdentity string) bool {
	if request == nil || action == nil || request.RequestID != action.RequestID || request.Action != action.Action || request.InstanceID != action.TargetInstance || viewCanonical(request.RepositoryIdentity) != viewCanonical(repositoryIdentity) {
		return false
	}
	var payload api.ControlPayload
	if len(request.Payload) > 0 && json.Unmarshal(request.Payload, &payload) != nil {
		return false
	}
	switch action.Action {
	case "start_candidate", "stop_candidate":
		return payload == action.Payload
	case "open_terminal", "recheck_environment":
		return payload.Environment != nil && payload.Environment.RunID == action.TaskID && (action.EnvironmentTarget == nil || *payload.Environment == *action.EnvironmentTarget)
	case "start_task":
		return payload.TaskID == action.TaskID && payload.Mode == "" && payload.ExecutionProfileID == action.ExecutionProfileID && payload.ExecutionProfileDigest == action.ExecutionProfileDigest
	case "set_dispatch_mode":
		return payload.Mode == action.TaskID && payload.TaskID == ""
	default:
		return len(request.Payload) == 0
	}
}

func (m Model) projectIndex(key string) int {
	for i := range m.projects {
		if m.projects[i].ref.Key == key {
			return i
		}
	}
	return -1
}

func preserveLastGood(previous, next ProjectSnapshot) ProjectSnapshot {
	if next.Observation.Page == nil && previous.Observation.Page != nil {
		next.Observation.Page = previous.Observation.Page
		next.Observation.SampledAt = previous.Observation.SampledAt
		next.Observation.HeartbeatAge = previous.Observation.HeartbeatAge
		next.Observation.FreshnessLimit = previous.Observation.FreshnessLimit
		next.Observation.ClockSkew = previous.Observation.ClockSkew
	}
	if next.Tasks == nil && previous.Tasks != nil {
		next.Tasks = previous.Tasks
	}
	if next.NativeTasks == nil && previous.NativeTasks != nil {
		next.NativeTasks = previous.NativeTasks
		if next.NativeTaskError == "" {
			next.NativeTaskError = "当前原生任务查询不可用；保留最近成功的只读快照"
		}
	}
	if next.History == nil && previous.History != nil {
		next.History = previous.History
		if next.HistoryError == "" {
			next.HistoryError = "当前格式查询不可用；保留最近成功的只读投影"
		}
	}
	return next
}

func (m Model) rows(index int) []consoleRow {
	if index >= 0 && index < len(m.projects) && m.projects[index].filter == 7 {
		local := m
		local.projects = append([]projectState(nil), m.projects...)
		local.projects[index].filter = 0
		var blocked []consoleRow
		for _, row := range local.rows(index) {
			if taskRowBlocked(row) {
				blocked = append(blocked, row)
			}
		}
		return blocked
	}
	if index < 0 || index >= len(m.projects) {
		return nil
	}
	state := &m.projects[index]
	var rows []consoleRow
	runsByTask := map[string]*api.CandidateSummary{}
	for _, run := range candidateRuns(*state, "") {
		id := firstText(run.DomainTaskID, run.TaskID)
		if runsByTask[id] == nil || state.candidates[id] == run.RunID {
			runsByTask[id] = run
		}
	}
	shownRuns := map[string]bool{}
	nativeByID := map[string]*api.TaskDetail{}
	if state.snapshot.NativeTasks != nil {
		for i := range state.snapshot.NativeTasks.Tasks {
			detail := &state.snapshot.NativeTasks.Tasks[i]
			nativeByID[detail.TaskID] = detail
		}
	}
	if state.snapshot.SelectedNative != nil {
		detail := state.snapshot.SelectedNative
		nativeByID[detail.TaskID] = detail
	}
	appendRun := func(run *api.CandidateSummary) {
		row := consoleRow{key: "task:" + run.TaskID, kind: rowRun, taskID: run.TaskID, title: run.Title, run: run, native: nativeByID[firstText(run.DomainTaskID, run.TaskID)]}
		if pin := state.candidates[run.TaskID]; pin != "" && pin != run.RunID {
			row.run = nil
			row.kind = rowNative
		}
		rows = append(rows, row)
		shownRuns[run.TaskID] = true
	}
	seenTasks := map[string]bool{}
	if state.snapshot.Tasks != nil {
		if filterAllows(state.filter, rowReady, nil) {
			for i := range state.snapshot.Tasks.Tasks {
				candidate := &state.snapshot.Tasks.Tasks[i]
				if runsByTask[candidate.TaskID] != nil {
					continue
				}
				row := consoleRow{key: "task:" + candidate.TaskID, kind: rowReady, taskID: candidate.TaskID, title: candidate.Title, candidate: candidate, native: nativeByID[candidate.TaskID]}
				rows = append(rows, row)
				seenTasks[candidate.TaskID] = true
			}
		}
	}
	if state.filter == 0 && state.snapshot.NativeTasks != nil {
		for i := range state.snapshot.NativeTasks.Tasks {
			detail := &state.snapshot.NativeTasks.Tasks[i]
			if seenTasks[detail.TaskID] {
				continue
			}
			if run := runsByTask[detail.TaskID]; run != nil {
				appendRun(run)
			} else {
				rows = append(rows, consoleRow{key: "task:" + detail.TaskID, kind: rowNative, taskID: detail.TaskID, title: detail.Title, native: detail})
			}
			seenTasks[detail.TaskID] = true
		}
	}
	if state.filter == 0 && state.snapshot.SelectedNative != nil && state.selectedKey == "task:"+state.snapshot.SelectedNative.TaskID && !seenTasks[state.snapshot.SelectedNative.TaskID] {
		detail := state.snapshot.SelectedNative
		if run := runsByTask[detail.TaskID]; run != nil {
			appendRun(run)
		} else {
			rows = append(rows, consoleRow{key: "task:" + detail.TaskID, kind: rowNative, taskID: detail.TaskID, title: detail.Title, native: detail})
		}
	}
	if runtime := state.snapshot.Observation.Page; runtime != nil {
		for _, run := range runtime.Runs {
			if run == nil || shownRuns[run.TaskID] || !candidateFilterAllows(*state, state.filter, run.TaskID) {
				continue
			}
			appendRun(runsByTask[run.TaskID])
		}
	}
	return rows
}

func filterAllows(filter int, kind rowKind, run *api.CandidateSummary) bool {
	if filter == 0 {
		return true
	}
	if filter == 1 {
		return kind == rowReady
	}
	if kind == rowReady {
		return false
	}
	if kind == rowNative {
		return filter == 0
	}
	switch filter {
	case 2:
		return run != nil && run.TaskStatus == "in_progress" && (run.ExecutionStatus == "pending" || run.ExecutionStatus == "running")
	case 3:
		return waitingForDecision(run)
	case 4:
		return waitingForMerge(run)
	case 5:
		return runNeedsAttention(run)
	case 6:
		return run != nil && (run.WorkflowStatus == "abandoned" || run.TaskStatus == "closed" && !runNeedsAttention(run) || runStatus(run) == "completed")
	}
	return true
}

func filterIndex(name string) int {
	switch name {
	case "ready":
		return 1
	case "running":
		return 2
	case "decision":
		return 3
	case "merge":
		return 4
	case "attention":
		return 5
	case "history":
		return 6
	case "blocked":
		return 7
	}
	return 0
}

func filterName(index int) string {
	switch index {
	case 1:

		return "ready"
	case 2:

		return "running"
	case 3:

		return "decision"
	case 4:

		return "merge"
	case 5:

		return "attention"
	case 6:

		return "history"
	case 7:
		return "blocked"
	}

	return "all"
}

func rowExists(rows []consoleRow, key string) bool {
	for _, row := range rows {
		if row.key == key {
			return true
		}
	}
	return false
}

func firstRowKey(rows []consoleRow) string {
	if len(rows) == 0 {
		return ""
	}
	return rows[0].key
}

func (m Model) selectedRow(index int) *consoleRow {
	rows := m.rows(index)
	if index < 0 || index >= len(m.projects) {
		return nil
	}
	for i := range rows {
		if rows[i].key == m.projects[index].selectedKey {
			return &rows[i]
		}
	}
	return nil
}

func (m Model) refreshFinalReplies(index int) (Model, tea.Cmd) {
	if index < 0 || index >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[index]
	key := ""
	filter := api.ConversationFilter{}
	if row := m.selectedRow(index); row != nil && row.run != nil && row.run.ExecutionID != "" {
		key = candidateIdentity(row.run)
		filter = api.ConversationFilter{WorkflowID: row.run.RunID, ExecutionID: row.run.ExecutionID, SessionID: row.run.SessionID, FinalsOnly: true}
	}
	full, attempt, turn := m.finalQueryScope(index)
	changed, cmd := state.final.refresh(m.ctx, state.client, m.clock.Now(), finalReplyRoute{state.ref.Key, state.ref.Identity}, key, filter, full, attempt, turn)
	if changed {
		state.detailScroll = 0
		state.recoveryGeneration++
		state.recoveryReport, state.recoveryError, state.recoveryBusy = nil, "", false
	}
	return m, cmd
}

func (m *Model) saveProjectView(index int) error {
	if m.prefsErr != "" {
		return errors.New(m.prefsErr)
	}
	if index < 0 || index >= len(m.projects) || m.profile == nil {
		return nil
	}
	state := &m.projects[index]
	state.view.ConfigPath, state.view.RunDir = state.ref.ConfigPath, state.ref.RunDir
	unresolved := 0
	for _, action := range state.actions {
		if !action.Resolved {
			unresolved++
		}
	}
	if unresolved > 256 {
		return errors.New("未知请求超过操作日志上限；先按原 ID 对账，不能丢弃请求身份")
	}
	state.view.Actions = savedActions(state.actions, 256)
	state.view.Outstanding = map[string]string{}
	for id, action := range state.actions {
		if _, saved := state.view.Actions[id]; saved && !action.Resolved && action.Status != "applied" && action.Status != "rejected" && action.Status != "failed" {
			state.view.Outstanding[id] = "task:" + action.TaskID
		}
	}
	m.prefs.Projects[state.ref.Key] = state.view
	if err := m.profile.write(m.prefs); err != nil {
		return fmt.Errorf("保存操作记录失败：%w", err)
	}
	return nil
}

func savedActions(actions map[string]*actionState, limit int) map[string]SavedAction {
	active, terminal := make([]*actionState, 0), make([]*actionState, 0)
	for _, action := range actions {
		if action.Resolved || action.Status == "applied" || action.Status == "rejected" || action.Status == "failed" {
			terminal = append(terminal, action)
		} else {
			active = append(active, action)
		}
	}
	byRecent := func(values []*actionState) {
		sort.SliceStable(values, func(i, j int) bool { return values[i].UpdatedAt.After(values[j].UpdatedAt) })
	}
	byRecent(active)
	byRecent(terminal)
	out := make(map[string]SavedAction, min(limit, len(active)+len(terminal)))
	for _, action := range active {
		if len(out) >= limit {
			break
		}
		out[action.RequestID] = SavedAction{StopSelectionPending: action.StopSelectionPending, CandidateNumber: action.CandidateNumber, CreatedAt: action.CreatedAt, Action: action.Action, TaskID: action.TaskID, ExecutionProfileID: action.ExecutionProfileID, ExecutionProfileDigest: action.ExecutionProfileDigest, Status: action.Status, RunID: action.RunID, DecisionID: action.DecisionID, TargetInstance: action.TargetInstance, UpdatedAt: action.UpdatedAt, Payload: action.Payload, EnvironmentTarget: action.EnvironmentTarget, DecisionIdentity: action.DecisionIdentity, DecisionDraftOptionID: action.DecisionDraftOptionID, DecisionDraftNote: action.DecisionDraftNote, DecisionRevision: action.DecisionRevision, DecisionVersion: action.DecisionVersion}
	}
	for _, action := range terminal {
		if len(out) >= limit {
			break
		}
		out[action.RequestID] = SavedAction{StopSelectionPending: action.StopSelectionPending, CandidateNumber: action.CandidateNumber, CreatedAt: action.CreatedAt, Action: action.Action, TaskID: action.TaskID, ExecutionProfileID: action.ExecutionProfileID, ExecutionProfileDigest: action.ExecutionProfileDigest, Status: action.Status, RunID: action.RunID, DecisionID: action.DecisionID, TargetInstance: action.TargetInstance, UpdatedAt: action.UpdatedAt, Payload: action.Payload, EnvironmentTarget: action.EnvironmentTarget, DecisionIdentity: action.DecisionIdentity, DecisionDraftOptionID: action.DecisionDraftOptionID, DecisionDraftNote: action.DecisionDraftNote, DecisionRevision: action.DecisionRevision, DecisionVersion: action.DecisionVersion}
	}
	return out
}

func (m Model) moveProject(delta int) (tea.Model, tea.Cmd) {
	if len(m.projects) == 0 {
		return m, nil
	}
	next := m.selected + delta
	if next < 0 {
		next = len(m.projects) - 1
	}
	if next >= len(m.projects) {
		next = 0
	}
	if next == m.selected {
		return m, nil
	}
	if !m.leaveAnswerSaved() {
		return m, nil
	}
	m.cancelWorkspace()
	m.selected = next
	m.generation++
	m.status = ""
	m.saveProjectView(next)
	if m.projects[next].lastRefresh.IsZero() || m.clock.Now().Sub(m.projects[next].lastRefresh) > time.Second {
		return m, m.readProject(next)
	}
	var cmd tea.Cmd
	m, cmd = m.refreshSelected(next)
	return m, cmd
}

func (m Model) moveSelection(delta int) (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	rows := m.rows(m.selected)
	if len(rows) == 0 {
		return m, nil
	}
	index := -1
	for i := range rows {
		if rows[i].key == state.selectedKey {
			index = i
			break
		}
	}
	if index < 0 {
		index = -1
	}
	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(rows) {
		index = len(rows) - 1
	}
	if rows[index].key == state.selectedKey {
		return m, nil
	}
	if !m.leaveAnswerSaved() {
		return m, nil
	}
	m.cancelWorkspace()
	state.selectedKey = rows[index].key
	state.selectionHold = false
	m.status = ""
	m.saveProjectView(m.selected)
	var cmd tea.Cmd
	m, cmd = m.refreshSelected(m.selected)
	return m, cmd
}

func (m Model) changeFilter(filter int) (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	if state.filter == filter {
		return m, nil
	}
	if !m.leaveAnswerSaved() {
		return m, nil
	}
	m.cancelWorkspace()
	state.filter = filter
	rows := m.rows(m.selected)
	if !rowExists(rows, state.selectedKey) {
		state.selectedKey = firstRowKey(rows)
		state.selectionHold = state.selectedKey == ""
	}
	m.saveProjectView(m.selected)
	return m.refreshSelected(m.selected)
}

func (m Model) View() string { return render(m) }

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func displayDash(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func terminalCapabilities() string {
	if runtime.GOOS == "windows" {
		return "Windows 服务窗口由用户独立打开"
	}
	return "Linux 服务按平台启动路径运行"
}

func projectStatus(ref ProjectRef, snapshot ProjectSnapshot) string {
	if ref.Error != "" {

		return "配置错误"
	}
	if ref.Conflict != "" {

		return "关联冲突"
	}
	if snapshot.Connection.State == api.ConnectionConnected {

		return "已连接"
	}
	if snapshot.Connection.State == api.ConnectionOffline || snapshot.Connection.State == api.ConnectionNoRecord {

		return "离线"
	}
	if snapshot.Connection.State == "" {

		return "读取中"
	}
	return string(snapshot.Connection.State)
}
