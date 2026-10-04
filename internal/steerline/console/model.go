package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
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
	Clients         map[string]api.ConsoleClient
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
	run       *contracts.Run
	candidate *contracts.TaskCandidate
	native    *contracts.NativeTaskDetail
}

type actionState struct {
	EnvironmentTarget      *productapi.HostActionTarget
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
	projectKey      string
	instance        string
	runID           string
	decisionID      string
	runRevision     uint64
	decisionVersion uint64
	optionIndex     int
	note            []rune
	cursor          int
	noteFocus       bool
	scroll          int
	error           string
}

type projectState struct {
	client api.ConsoleClient
	ref    ProjectRef
	view   ProjectView

	snapshot       ProjectSnapshot
	snapshotLoaded bool
	refreshing     bool
	refreshID      uint64
	lastRefresh    time.Time

	selectedKey   string
	selectionHold bool
	filter        int
	panel         int
	focus         int // 0: task list, 1: task activity
	attemptID     string

	actions map[string]*actionState

	activityKey        string
	activityCursor     productapi.ActivityCursor
	activityEvents     []contracts.ActivityEvent
	activityError      string
	activityInFlight   bool
	activityGeneration uint64

	conversationKey        string
	conversationFilter     contracts.ConversationFilter
	conversationCursor     string
	conversationTailCursor string
	conversationEvents     []contracts.ConversationEvent
	conversationMore       bool
	conversationTailMore   bool
	conversationError      string
	conversationInFlight   bool
	conversationGeneration uint64
	capabilities           []contracts.ConversationCapabilityReport
	capabilityError        string
	capabilityInFlight     bool
	capabilityGeneration   uint64

	bodyRef            *contracts.ConversationBodyRef
	bodyOffset         int64
	bodyViewOffset     int64
	bodyPages          []int64
	bodyText           string
	bodyError          string
	bodyMore           bool
	bodyBusy           bool
	bodyGeneration     uint64
	bodyEvent          string
	bodyRestorePending bool
	bodyKind           string
	detailScroll       int
	logScroll          int

	recoveryRunID        string
	recoveryReport       *api.RecoveryInspection
	recoveryError        string
	recoveryBusy         bool
	recoveryGeneration   uint64
	recoveryBlockerIndex int
}

type confirmAction struct {
	scroll       int
	requireBasis bool
	basisCursor  int
	basisError   string
	projectKey   string
	instance     string
	revision     uint64
	action       string
	taskID       string
	requestID    string
	runID        string
	decisionID   string
	basis        string
}

type executionProfileChoice struct {
	projectKey string
	taskID     string
	instance   string
	revision   uint64
	index      int
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
	help                 bool
	detailExpanded       bool
	expandedDetailScroll int
	status               string
	finalIndex           int

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
			view = ProjectView{ConfigPath: ref.ConfigPath, RunDir: ref.RunDir, Follow: true, Panel: "activity", Groups: map[string]FoldChoice{}, Outstanding: map[string]string{}}
		}
		if view.Groups == nil {
			view.Groups = map[string]FoldChoice{}
		}
		if view.Outstanding == nil {
			view.Outstanding = map[string]string{}
		}
		if view.Actions == nil {
			view.Actions = map[string]SavedAction{}
		}
		attemptID := ""
		if view.AttemptPinned {
			attemptID = view.AttemptID
		}
		state := projectState{client: options.Clients[ref.Key], ref: ref, view: view, selectedKey: view.SelectedKey, selectionHold: view.SelectionPending, filter: filterIndex(view.Filter), panel: panelIndex(view.Panel), attemptID: attemptID, detailScroll: view.DetailScroll, logScroll: view.LogScroll, actions: map[string]*actionState{}}
		state.bodyKind = view.BodyKind
		state.bodyPages = append([]int64(nil), view.BodyPages...)
		for requestID, saved := range view.Actions {
			_, outstanding := view.Outstanding[requestID]
			if saved.Status == "submitting" {
				saved.Status = "indeterminate"
			}
			message := "已恢复历史控制结果"
			if outstanding {
				message = "控制台重开后重新查询此 request_id"
			}
			state.actions[requestID] = &actionState{RequestID: requestID, Action: saved.Action, TaskID: saved.TaskID, ExecutionProfileID: saved.ExecutionProfileID, ExecutionProfileDigest: saved.ExecutionProfileDigest, Status: saved.Status, Message: message, RunID: saved.RunID, DecisionID: saved.DecisionID, TargetInstance: saved.TargetInstance, Resolved: !outstanding, UpdatedAt: saved.UpdatedAt}
		}
		m.projects = append(m.projects, state)
	}
	if len(m.projects) > 0 {
		m.selected = 0
		if prefs.ActiveProject != "" {
			for i := range m.projects {
				if m.projects[i].ref.Key == prefs.ActiveProject {
					m.selected = i
					break
				}
			}
		}
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
			return projectMsg{key: ref.Key, identity: ref.Identity, generation: generation, requestID: id, snapshot: ProjectSnapshot{TaskError: ref.Error, DomainError: ref.Error}}
		}
		return projectMsg{key: ref.Key, identity: ref.Identity, generation: generation, requestID: id, snapshot: query.ReadProject(ctx, selectedTaskID, previous)}
	}
}

func (m Model) readActivity(index int, runID, executionID, attemptID, key string) tea.Cmd {
	state := &m.projects[index]
	if state.activityInFlight {
		return nil
	}
	state.activityGeneration++
	generation := state.activityGeneration
	state.activityInFlight = true
	base := state.activityCursor
	query, ctx, ref := state.client, m.ctx, state.ref
	return func() tea.Msg {
		batch, err := query.Activity(ctx, runID, executionID, attemptID, base)
		return activityResultMsg{projectKey: ref.Key, identity: ref.Identity, key: key, generation: generation, base: base, batchActivity: batch.Activity, next: batch.Cursor, reset: batch.Reset, diagnostics: batch.Diagnostics, err: err}
	}
}

func selectedNativeTaskID(state projectState) string {
	if state.ref.Presentation.TaskAuthority != "native" {
		return ""
	}
	if strings.HasPrefix(state.selectedKey, "task:") {
		return strings.TrimPrefix(state.selectedKey, "task:")
	}
	if strings.HasPrefix(state.selectedKey, "run:") && state.snapshot.Observation.Runtime != nil {
		runID := strings.TrimPrefix(state.selectedKey, "run:")
		for _, run := range state.snapshot.Observation.Runtime.Runs {
			if run != nil && run.RunID == runID {
				return firstText(run.DomainTaskID, run.TaskID)
			}
		}
	}
	return ""
}

func (m Model) readConversation(index int, cursor, key, mode string) tea.Cmd {
	state := &m.projects[index]
	if state.conversationInFlight {
		return nil
	}
	state.conversationGeneration++
	generation := state.conversationGeneration
	state.conversationInFlight = true
	query, ctx, ref, filter := state.client, m.ctx, state.ref, state.conversationFilter
	return func() tea.Msg {
		page, err := query.Conversation(ctx, filter, cursor)
		return conversationMsg{projectKey: ref.Key, identity: ref.Identity, key: key, generation: generation, cursor: cursor, mode: mode, page: page, err: err}
	}
}

func (m Model) readCapabilities(index int, events []contracts.ConversationEvent, key string) tea.Cmd {
	state := &m.projects[index]
	if state.capabilityInFlight {
		return nil
	}
	state.capabilityInFlight = true
	state.capabilityGeneration++
	generation := state.capabilityGeneration
	query, ctx, ref := state.client, m.ctx, state.ref
	events = append([]contracts.ConversationEvent(nil), events...)
	return func() tea.Msg {
		seen := map[string]bool{}
		var reports []contracts.ConversationCapabilityReport
		var queryErrors []error
		for _, event := range events {
			if event.Backend == "" || event.Protocol == "" || event.TargetID == "" {
				continue
			}
			targetKey := strings.Join([]string{string(event.Backend), string(event.Protocol), event.TargetID, event.ExecutionID, event.AttemptID, event.Source, event.SourceIdentity}, "\x00")
			if seen[targetKey] {
				continue
			}
			seen[targetKey] = true
			current, err := query.Capabilities(ctx, api.CapabilityFilter{Backend: event.Backend, Protocol: event.Protocol, TargetID: event.TargetID, ExecutionID: event.ExecutionID, AttemptID: event.AttemptID, Source: event.Source, SourceIdentity: event.SourceIdentity})
			if err != nil {
				queryErrors = append(queryErrors, err)
				continue
			}
			reports = append(reports, current...)
		}
		return capabilityMsg{projectKey: ref.Key, identity: ref.Identity, key: key, generation: generation, reports: reports, err: errors.Join(queryErrors...)}
	}
}

func (m Model) readBody(index int, event contracts.ConversationEvent, offset int64) tea.Cmd {
	state := &m.projects[index]
	if event.Body == nil || state.bodyBusy {
		return nil
	}
	state.bodyBusy = true
	state.bodyGeneration++
	generation := state.bodyGeneration
	ref, body, query, ctx := state.ref, *event.Body, state.client, m.ctx
	return func() tea.Msg {
		page, err := query.Body(ctx, body, offset, 64<<10)
		return bodyMsg{projectKey: ref.Key, identity: ref.Identity, eventKey: conversationItemKey(event), generation: generation, body: body, offset: offset, page: page, err: err}
	}
}

func (m Model) submitControl(index int, action, taskID, requestID string, instance string, revision uint64) tea.Cmd {
	state := &m.projects[index]
	ref := state.ref
	payload := productapi.ControlPayload{}
	switch action {
	case "open_terminal", "recheck_environment":
		if pending := state.actions[requestID]; pending != nil {
			payload.Environment = pending.EnvironmentTarget
		}
	case "start_task":
		profile := state.actions[requestID]
		if profile != nil && profile.ExecutionProfileID != "" {
			payload = productapi.ControlPayload{TaskID: taskID, ExecutionProfileID: profile.ExecutionProfileID, ExecutionProfileDigest: profile.ExecutionProfileDigest}
		} else {
			payload = productapi.ControlPayload{TaskID: taskID}
		}
	case "set_dispatch_mode":
		payload = productapi.ControlPayload{Mode: taskID}
	}
	query := state.client
	ctx := m.ctx
	return func() tea.Msg {
		request, err := query.SubmitControl(ctx, productapi.ControlInput{Action: productapi.ControlAction(action), Payload: payload, RequestID: requestID, InstanceID: instance, ExpectedRevision: &revision})
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

type activityResultMsg struct {
	projectKey, identity, key string
	generation                uint64
	base                      productapi.ActivityCursor
	batchActivity             []contracts.ActivityEvent
	next                      productapi.ActivityCursor
	reset                     bool
	diagnostics               []string
	err                       error
}

type conversationMsg struct {
	projectKey, identity, key string
	generation                uint64
	cursor                    string
	mode                      string
	page                      contracts.ConversationPage
	err                       error
}

type capabilityMsg struct {
	projectKey, identity, key string
	generation                uint64
	reports                   []contracts.ConversationCapabilityReport
	err                       error
}

type bodyMsg struct {
	projectKey, identity, eventKey string
	generation                     uint64
	body                           contracts.ConversationBodyRef
	offset                         int64
	page                           contracts.ConversationBodyPage
	err                            error
}

type actionSubmittedMsg struct {
	projectKey, identity, instance, requestID, taskID, action string
	request                                                   productapi.ControlRequest
	command                                                   *storageports.CommandRecord
	err                                                       error
}

func (m Model) submitDecisionCommand(index int, answer executionmodel.DecisionAnswer, requestedBy, commandID string) tea.Cmd {
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
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	row := m.selectedRow(m.selected)
	if row == nil || row.run == nil || row.run.Decision == nil || row.run.Decision.Status != "pending" {
		m.status = "请选择带有 pending 决策的问题"
		return m, nil
	}
	reason := decisionAnswerAvailability(*state, row)
	if !strings.HasPrefix(reason, "提交后") {
		m.status = reason
		return m, nil
	}
	eligibility := state.client
	ok := eligibility != nil
	if !ok {
		m.status = "当前控制客户端不支持决策资格核验"
		return m, nil
	}
	if err := eligibility.DecisionEligibility(m.ctx, row.run); err != nil {
		m.status = "回答资格核验失败：" + err.Error()
		return m, nil
	}
	runtimeState := state.snapshot.Connection.Snapshot.Runtime
	if runtimeState == nil {
		m.status = "缺少当前服务实例快照"
		return m, nil
	}
	decision := row.run.Decision
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
	m.answer = &decisionAnswerDialog{projectKey: state.ref.Key, instance: runtimeState.Service.InstanceID, runID: row.run.RunID, decisionID: decision.ID, runRevision: row.run.ControlRevision, decisionVersion: decision.Version, optionIndex: optionIndex}
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
	runtimeState := state.snapshot.Connection.Snapshot.Runtime
	var run *contracts.Run
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
	answer := executionmodel.DecisionAnswer{DecisionID: dialog.decisionID, RunID: dialog.runID, OptionID: optionID, Note: note, ExpectedDecisionVersion: dialog.decisionVersion, ExpectedRunRevision: dialog.runRevision, ServiceInstanceID: dialog.instance}
	action := &actionState{RequestID: requestID, Action: "answer_decision", TaskID: run.TaskID, RunID: run.RunID, DecisionID: dialog.decisionID, DecisionDraftOptionID: optionID, DecisionDraftNote: string(dialog.note), DecisionRevision: dialog.runRevision, DecisionVersion: dialog.decisionVersion, Status: "submitting", Message: "保存 command_id 并提交答案", TargetInstance: dialog.instance, UpdatedAt: m.clock.Now()}
	state.actions[requestID] = action
	state.view.Outstanding[requestID] = "decision:" + run.RunID
	if err := m.saveProjectView(index); err != nil {
		delete(state.actions, requestID)
		delete(state.view.Outstanding, requestID)
		dialog.error = "未提交答案：无法先保存 command_id；请检查 UI 视图目录权限"
		return m, nil
	}
	m.answer = nil
	m.status = "答案命令已发布；command_id=" + requestID
	return m, m.submitDecisionCommand(index, answer, "", requestID)
}

type actionReceiptMsg struct {
	projectKey, identity, requestID string
	receipt                         *productapi.ControlReceipt
	request                         *productapi.ControlRequest
	command                         *storageports.CommandRecord
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

func applyDecisionCommandResult(state *projectState, action *actionState, command *storageports.CommandRecord, commandErr error) (refresh, reopen bool) {
	if state == nil || action == nil {
		return false, false
	}
	if commandErr != nil {
		diagnostic := commandErr.Error()
		definite := strings.Contains(diagnostic, "stale_revision") || strings.Contains(diagnostic, "stale_instance") || strings.Contains(diagnostic, "answer_conflict") || strings.Contains(diagnostic, "unsupported") || strings.Contains(diagnostic, "exceeds") || strings.Contains(diagnostic, "requires") || strings.Contains(diagnostic, "not waiting")
		if definite {
			action.Status, action.Message, action.Resolved = "rejected", diagnostic, true
			return true, true
		}
		action.Status, action.Message, action.Resolved = "indeterminate", "答案命令结果待确认；继续核实原 command_id："+diagnostic, false
		return false, false
	}
	if command == nil || command.CommandID != action.RequestID || command.Kind != action.Action || command.TargetID != action.DecisionID || viewCanonical(command.ProjectIdentity) != viewCanonical(state.ref.Identity) {
		action.Status, action.Message, action.Resolved = "indeterminate", "决策命令回执身份不匹配；保留原 command_id 并停止自动提交", true
		return false, false
	}
	action.Status = command.Status
	switch command.Status {
	case "pending", "in_flight":
		action.Message, action.Resolved = "答案已提交；等待服务接受", false
	case "accepted":
		action.Message = "答案已保存；服务将单独核验原执行与续跑条件"
		action.Resolved = true
		return true, false
	case "rejected":
		code, reason := "rejected", "服务拒绝了这次答案"
		var result struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		if json.Unmarshal(command.Result, &result) == nil {
			if result.Code != "" {
				code = result.Code
			}
			if result.Error != "" {
				reason = result.Error
			}
		}
		action.Message, action.Resolved = code+"："+reason, true
		return true, true
	default:
		action.Status, action.Message, action.Resolved = "indeterminate", "决策命令状态未知；保留原 command_id", false
	}
	return false, false
}

func decisionRequestFor(state projectState, runID, decisionID string) (executionmodel.DecisionRequest, bool) {
	if runtimeState := state.snapshot.Connection.Snapshot.Runtime; runtimeState != nil {
		for _, run := range runtimeState.Runs {
			if run != nil && run.RunID == runID && run.Decision != nil && run.Decision.ID == decisionID {
				return run.Decision.Request, true
			}
		}
	}
	if runtimeState := state.snapshot.Observation.Runtime; runtimeState != nil {
		for _, run := range runtimeState.Runs {
			if run != nil && run.RunID == runID && run.Decision != nil && run.Decision.ID == decisionID {
				return run.Decision.Request, true
			}
		}
	}
	if state.snapshot.Domain != nil {
		for _, record := range state.snapshot.Domain.Decisions {
			if record.DecisionID == decisionID && record.WorkflowRunID == runID {
				var request executionmodel.DecisionRequest
				if json.Unmarshal(record.Request, &request) == nil {
					return request, true
				}
			}
		}
	}
	return executionmodel.DecisionRequest{}, false
}

func (m *Model) reopenDecisionDraft(index int, action *actionState) {
	if index < 0 || index >= len(m.projects) || action == nil {
		return
	}
	state := &m.projects[index]
	request, ok := decisionRequestFor(*state, action.RunID, action.DecisionID)
	optionIndex := 0
	if !ok || len(request.Options) == 0 {
		optionIndex = -1
	} else if action.DecisionDraftOptionID == "" {
		optionIndex = len(request.Options)
	} else {
		for i, option := range request.Options {
			if option.ID == action.DecisionDraftOptionID {
				optionIndex = i
				break
			}
		}
	}
	note := []rune(action.DecisionDraftNote)
	m.answer = &decisionAnswerDialog{projectKey: state.ref.Key, instance: action.TargetInstance, runID: action.RunID, decisionID: action.DecisionID, runRevision: action.DecisionRevision, decisionVersion: action.DecisionVersion, optionIndex: optionIndex, note: note, cursor: len(note), noteFocus: true, error: action.Message}
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		return m.updateWindowSize(msg)
	case tickMsg:
		return m.updateTick()
	case projectMsg:
		return m.updateProject(msg)
	case recoveryInspectionMsg:
		return m.updateRecoveryInspection(msg)
	case activityResultMsg:
		return m.updateActivity(msg)
	case conversationMsg:
		return m.updateConversation(msg)
	case capabilityMsg:
		return m.updateCapability(msg)
	case bodyMsg:
		return m.updateBody(msg)
	case actionSubmittedMsg:
		return m.updateActionSubmitted(msg)
	case actionReceiptMsg:
		return m.updateActionReceipt(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func controlRequestMatchesAction(request *productapi.ControlRequest, action *actionState, repositoryIdentity string) bool {
	if request == nil || action == nil || request.RequestID != action.RequestID || request.Action != action.Action || request.InstanceID != action.TargetInstance || viewCanonical(request.RepositoryIdentity) != viewCanonical(repositoryIdentity) {
		return false
	}
	var payload productapi.ControlPayload
	if len(request.Payload) > 0 && json.Unmarshal(request.Payload, &payload) != nil {
		return false
	}
	switch action.Action {
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
	if next.Observation.Runtime == nil && previous.Observation.Runtime != nil {
		next.Observation.Runtime = previous.Observation.Runtime
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
	if next.Domain == nil && previous.Domain != nil {
		next.Domain = previous.Domain
		if next.DomainError == "" {
			next.DomainError = "当前格式查询不可用；保留最近成功的只读投影"
		}
	}
	return next
}

func (m Model) rows(index int) []consoleRow {
	if index < 0 || index >= len(m.projects) {
		return nil
	}
	state := &m.projects[index]
	var rows []consoleRow
	runsByTask := map[string]*contracts.Run{}
	if runtime := state.snapshot.Observation.Runtime; runtime != nil {
		for _, run := range runtime.Runs {
			if run != nil {
				runsByTask[firstText(run.DomainTaskID, run.TaskID)] = run
			}
		}
	}
	shownRuns := map[string]bool{}
	nativeByID := map[string]*contracts.NativeTaskDetail{}
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
	appendRun := func(run *contracts.Run) {
		rows = append(rows, consoleRow{key: "run:" + run.RunID, kind: rowRun, taskID: run.TaskID, title: run.Title, run: run, native: nativeByID[firstText(run.DomainTaskID, run.TaskID)]})
		shownRuns[run.RunID] = true
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
	if runtime := state.snapshot.Observation.Runtime; runtime != nil {
		for _, run := range runtime.Runs {
			if run == nil || shownRuns[run.RunID] || !filterAllows(state.filter, rowRun, run) {
				continue
			}
			appendRun(run)
		}
	}
	return rows
}

func filterAllows(filter int, kind rowKind, run *contracts.Run) bool {
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
		return run != nil && run.FinishTaskStatus == "waiting_decision"
	case 4:
		return run != nil && run.FinishTaskStatus == "waiting_pr_merge"
	case 5:
		return runNeedsAttention(run)
	case 6:
		return run != nil && (run.TaskStatus == "closed" && !runNeedsAttention(run) || runStatus(run) == "completed")
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
	}
	return "all"
}

func panelIndex(name string) int {
	switch name {
	case "activity":
		return 1
	case "session":
		return 2
	case "final":
		return 3
	case "list":
		return 4
	case "decisions":
		return 5
	case "recovery":
		return 6
	}
	return 0
}

func panelName(index int) string {
	switch index {
	case 1:
		return "activity"
	case 2:
		return "session"
	case 3:
		return "final"
	case 4:
		return "list"
	case 5:
		return "decisions"
	case 6:
		return "recovery"
	}
	return "activity"
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

func (m Model) refreshSelectedLogs(index int) (Model, tea.Cmd) {
	if index < 0 || index >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[index]
	row := m.selectedRow(index)
	if row == nil || row.run == nil {
		state.activityKey, state.conversationKey = "", ""
		state.activityEvents = nil
		state.conversationEvents = nil
		return m, nil
	}
	run := row.run
	attemptID := run.AttemptID
	if state.attemptID != "" {
		attemptID = state.attemptID
	}
	if attemptID == "" {
		attemptID = run.RunID
	}
	activityKey := strings.Join([]string{run.RunID, run.ExecutionID, attemptID}, "\x00")
	if state.activityKey != activityKey {
		state.activityKey, state.activityCursor = activityKey, productapi.ActivityCursor{}
		state.activityEvents, state.activityError = nil, ""
		state.activityGeneration++
	}
	conversationKey := strings.Join([]string{run.ExecutionID, attemptID}, "\x00")
	var cmds []tea.Cmd
	if state.conversationKey != conversationKey {
		state.conversationKey = conversationKey
		state.conversationFilter = contracts.ConversationFilter{WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: attemptID}
		state.conversationCursor, state.conversationTailCursor, state.conversationMore, state.conversationTailMore = "", "", false, false
		state.conversationEvents, state.conversationError = nil, ""
		state.capabilityError = ""
		state.conversationGeneration++
		state.capabilityInFlight = false
		state.capabilityGeneration++
		state.bodyGeneration++
		state.bodyBusy = false
		state.bodyRef, state.bodyText, state.bodyError, state.bodyOffset, state.bodyEvent = nil, "", "", 0, ""
		state.bodyViewOffset = 0
		state.bodyRestorePending = state.view.AnchorItem != ""
		if run.ExecutionID != "" {
			cmds = append(cmds, m.readConversation(index, "", conversationKey, "head"))
		} else {
			state.conversationError = "当前执行缺少 execution_id，不能安全读取会话"
		}
	}
	if state.view.Follow && !state.activityInFlight {
		cmds = append(cmds, m.readActivity(index, run.RunID, run.ExecutionID, attemptID, activityKey))
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) saveProjectView(index int) error {
	if index < 0 || index >= len(m.projects) || m.profile == nil {
		return nil
	}
	state := &m.projects[index]
	state.view.ConfigPath, state.view.RunDir = state.ref.ConfigPath, state.ref.RunDir
	state.view.SelectedKey, state.view.Filter, state.view.Panel = state.selectedKey, filterName(state.filter), panelName(state.panel)
	state.view.SelectionPending = state.selectionHold
	state.view.AttemptID = state.attemptID
	state.view.AttemptPinned = state.attemptID != ""
	state.view.DetailScroll, state.view.LogScroll = state.detailScroll, state.logScroll
	state.view.BodyOffset = state.bodyViewOffset
	state.view.BodyPages = append([]int64(nil), state.bodyPages...)
	state.view.BodyKind = state.bodyKind
	state.view.AnchorLine = state.detailScroll
	state.view.Groups = m.foldChoices(index)
	state.view.Actions = savedActions(state.actions, 256)
	state.view.Outstanding = make(map[string]string)
	for id, action := range state.actions {
		if _, saved := state.view.Actions[id]; saved && !action.Resolved && action.Status != "applied" && action.Status != "rejected" && action.Status != "failed" {
			state.view.Outstanding[id] = "task:" + action.TaskID
		}
	}
	if index == m.selected {
		m.prefs.ActiveProject = state.ref.Key
	}
	m.prefs.Projects[state.ref.Key] = state.view
	if err := writeViewFile(m.profile.path(), m.prefs); err != nil {
		m.prefsErr = "保存视图偏好失败：" + err.Error()
		return err
	}
	m.prefsErr = ""
	return nil
}

func clearReadingAnchor(state *projectState) {
	if state == nil {
		return
	}
	state.view.AnchorGroup, state.view.AnchorItem, state.view.ReadGroup, state.view.ReadItem, state.view.BodyKind = "", "", "", "", ""
	state.view.AnchorRevision, state.view.ReadRevision, state.view.AnchorLine, state.view.ReadLine, state.view.BodyOffset = 0, 0, 0, 0, 0
	state.view.BodyPages = nil
	state.bodyRef, state.bodyEvent = nil, ""
	state.bodyGeneration++
	state.bodyBusy = false
	state.bodyText, state.bodyError = "", ""
	state.bodyOffset, state.bodyViewOffset = 0, 0
	state.bodyPages = nil
	state.bodyMore, state.bodyRestorePending, state.bodyKind = false, false, ""
	state.detailScroll, state.logScroll = 0, 0
}

func appendBodyPage(values []int64, offset int64) []int64 {
	for _, value := range values {
		if value == offset {
			return values
		}
	}
	if len(values) >= 257 {
		return values
	}
	values = append(values, offset)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values
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
		out[action.RequestID] = SavedAction{Action: action.Action, TaskID: action.TaskID, ExecutionProfileID: action.ExecutionProfileID, ExecutionProfileDigest: action.ExecutionProfileDigest, Status: action.Status, RunID: action.RunID, DecisionID: action.DecisionID, TargetInstance: action.TargetInstance, UpdatedAt: action.UpdatedAt}
	}
	for _, action := range terminal {
		if len(out) >= limit {
			break
		}
		out[action.RequestID] = SavedAction{Action: action.Action, TaskID: action.TaskID, ExecutionProfileID: action.ExecutionProfileID, ExecutionProfileDigest: action.ExecutionProfileDigest, Status: action.Status, RunID: action.RunID, DecisionID: action.DecisionID, TargetInstance: action.TargetInstance, UpdatedAt: action.UpdatedAt}
	}
	return out
}

func (m Model) foldChoices(index int) map[string]FoldChoice {
	if index < 0 || index >= len(m.projects) {
		return nil
	}
	return m.projects[index].view.Groups
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
	m.selected = next
	m.generation++
	m.status = ""
	m.saveProjectView(next)
	if m.projects[next].lastRefresh.IsZero() || m.clock.Now().Sub(m.projects[next].lastRefresh) > time.Second {
		return m, m.readProject(next)
	}
	var cmd tea.Cmd
	m, cmd = m.refreshSelectedLogs(next)
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
	state.selectedKey = rows[index].key
	state.selectionHold = false
	state.attemptID = ""
	clearReadingAnchor(state)
	m.status = ""
	m.saveProjectView(m.selected)
	var cmd tea.Cmd
	m, cmd = m.refreshSelectedLogs(m.selected)
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
	state.filter = filter
	rows := m.rows(m.selected)
	if !rowExists(rows, state.selectedKey) {
		oldSelection := state.selectedKey
		state.selectedKey = firstRowKey(rows)
		state.selectionHold = state.selectedKey == ""
		state.attemptID = ""
		if oldSelection != state.selectedKey {
			clearReadingAnchor(state)
		}
	}
	m.saveProjectView(m.selected)
	return m.refreshSelectedLogs(m.selected)
}

func (m Model) View() string { return render(m) }

func sameCursor(a, b productapi.ActivityCursor) bool { return a == b }

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
	if snapshot.Connection.State == productapi.ConnectionConnected {
		return "已连接"
	}
	if snapshot.Connection.State == productapi.ConnectionOffline || snapshot.Connection.State == productapi.ConnectionNoRecord {
		return "离线"
	}
	if snapshot.Connection.State == "" {
		return "读取中"
	}
	return string(snapshot.Connection.State)
}
