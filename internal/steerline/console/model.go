package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/erranliu/SteerLine/internal/steerline"
)

type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

type Options struct {
	RunID   string
	Query   QuerySource
	Control ControlClient
	Clock   Clock
}

type rowKind int

const (
	rowReady rowKind = iota
	rowRun
	rowNative
	rowLegacy
)

type consoleRow struct {
	key       string
	kind      rowKind
	taskID    string
	title     string
	run       *steerline.Run
	candidate *steerline.TaskCandidate
	native    *steerline.NativeTaskDetail
	legacy    *steerline.LegacyBeadsTask
}

type actionState struct {
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
	ref  ProjectRef
	view ProjectView

	snapshot       ProjectSnapshot
	snapshotLoaded bool
	refreshing     bool
	refreshID      uint64
	lastRefresh    time.Time

	selectedKey   string
	selectionHold bool
	filter        int
	panel         int
	attemptID     string

	actions map[string]*actionState

	activityKey        string
	activityCursor     steerline.JournalCursor
	activityEvents     []steerline.ActivityEvent
	activityRaw        []string
	activityError      string
	activityInFlight   bool
	activityGeneration uint64

	conversationKey        string
	conversationFilter     steerline.ConversationFilter
	conversationCursor     string
	conversationTailCursor string
	conversationEvents     []steerline.ConversationEvent
	conversationMore       bool
	conversationTailMore   bool
	conversationError      string
	conversationInFlight   bool
	conversationGeneration uint64
	capabilities           []steerline.ConversationCapabilityReport
	capabilityError        string
	capabilityInFlight     bool
	capabilityGeneration   uint64

	bodyRef            *steerline.ConversationBodyRef
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
	recoveryReport       *steerline.RecoveryInspection
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
	ctx        context.Context
	query      QuerySource
	control    ControlClient
	clock      Clock
	projects   []projectState
	selected   int
	width      int
	height     int
	generation uint64
	refreshID  uint64

	profile  *viewProfile
	prefs    ViewFile
	prefsErr string

	confirm       *confirmAction
	profileChoice *executionProfileChoice
	answer        *decisionAnswerDialog
	help          bool
	status        string
	finalIndex    int

	runFocus string
}

// LoadProjects reads either one explicit config or all configs under etc.
// Invalid project files stay in the list so their errors remain visible.
func LoadProjects(configPath string) ([]ProjectRef, error) {
	var refs []ProjectRef
	if strings.TrimSpace(configPath) != "" {
		absolute, err := filepath.Abs(configPath)
		if err != nil {
			return nil, err
		}
		config, loadErr := steerline.LoadConfig(absolute)
		name := strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute))
		if loadErr != nil {
			return []ProjectRef{legacyProjectRef(name, absolute, loadErr)}, nil
		}
		refs = append(refs, makeProjectRef(name, absolute, config))
	} else {
		projects, err := steerline.ListProjects()
		if err != nil {
			return nil, err
		}
		for _, project := range projects {
			path, _ := filepath.Abs(project.ConfigPath)
			if project.Err != nil {
				refs = append(refs, legacyProjectRef(project.Name, path, project.Err))
				continue
			}
			config, loadErr := steerline.LoadConfig(path)
			if loadErr != nil {
				refs = append(refs, legacyProjectRef(project.Name, path, loadErr))
				continue
			}
			refs = append(refs, makeProjectRef(project.Name, path, config))
		}
	}
	identityNames := map[string][]string{}
	for i := range refs {
		if refs[i].Identity != "" {
			key := viewCanonical(refs[i].Identity)
			identityNames[key] = append(identityNames[key], refs[i].Name)
		}
	}
	for i := range refs {
		if refs[i].Identity == "" {
			continue
		}
		names := identityNames[viewCanonical(refs[i].Identity)]
		if len(names) > 1 {
			refs[i].Conflict = "同仓库配置关联/冲突：" + strings.Join(names, ", ") + "；共享单仓库服务锁，控制操作已禁用"
		}
	}
	return refs, nil
}

func legacyProjectRef(name, path string, configErr error) ProjectRef {
	legacy, err := steerline.LoadLegacyBeadsProject(path)
	if err == nil {
		key, _ := absoluteViewKey(path, legacy.RunDir)
		return ProjectRef{Key: key, Name: name, ConfigPath: path, RunDir: legacy.RunDir, Legacy: legacy}
	}
	key, _ := absoluteViewKey(path, "")
	if errors.Is(err, steerline.ErrNotLegacyBeadsConfig) {
		return ProjectRef{Key: key, Name: name, ConfigPath: path, Error: configErr.Error()}
	}
	return ProjectRef{Key: key, Name: name, ConfigPath: path, Error: "旧 Beads 配置：" + err.Error()}
}

func makeProjectRef(name, path string, config steerline.Config) ProjectRef {
	absoluteRunDir, _ := filepath.Abs(config.RunDir)
	configPath, _ := filepath.Abs(path)
	key, _ := absoluteViewKey(configPath, absoluteRunDir)
	identity := ""
	if err := config.ResolveIdentity(context.Background()); err != nil {
		return ProjectRef{Key: key, Name: name, ConfigPath: configPath, Config: config, RunDir: absoluteRunDir, Error: "resolve repository identity: " + err.Error()}
	}
	identity = config.Identity()
	return ProjectRef{Key: key, Name: name, ConfigPath: configPath, Config: config, Identity: identity, RunDir: absoluteRunDir}
}

func NewModel(ctx context.Context, refs []ProjectRef, options Options) (Model, error) {
	profile, prefs, diagnostic, err := acquireViewProfile()
	if err != nil {
		return Model{}, err
	}
	if options.Query == nil {
		options.Query = FileQuerySource{}
	}
	if options.Control == nil {
		options.Control = FileControlClient{}
	}
	if options.Clock == nil {
		options.Clock = realClock{}
	}
	m := Model{ctx: ctx, query: options.Query, control: options.Control, clock: options.Clock, profile: profile, prefs: prefs, prefsErr: diagnostic, runFocus: options.RunID, selected: -1}
	for _, ref := range refs {
		view, exists := prefs.Projects[ref.Key]
		if !exists {
			view = ProjectView{ConfigPath: ref.ConfigPath, RunDir: ref.RunDir, Follow: true, Panel: "detail", Groups: map[string]FoldChoice{}, Outstanding: map[string]string{}}
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
		state := projectState{ref: ref, view: view, selectedKey: view.SelectedKey, selectionHold: view.SelectionPending, filter: filterIndex(view.Filter), panel: panelIndex(view.Panel), attemptID: view.AttemptID, detailScroll: view.DetailScroll, logScroll: view.LogScroll, actions: map[string]*actionState{}}
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

func Run(ctx context.Context, refs []ProjectRef, options Options) error {
	m, err := NewModel(ctx, refs, options)
	if err != nil {
		return err
	}
	defer m.profile.close()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err = p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{waitTick(m.clock, 800*time.Millisecond)}
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
	generation := m.generation
	m.projects[index].refreshID++
	id := m.projects[index].refreshID
	m.projects[index].refreshing = true
	query := m.query
	ctx := m.ctx
	return func() tea.Msg {
		return projectMsg{key: ref.Key, identity: ref.Identity, generation: generation, requestID: id, snapshot: query.ReadProject(ctx, ref, selectedTaskID)}
	}
}

func (m Model) readActivity(index int, runID, executionID, attemptID, path, key string) tea.Cmd {
	state := &m.projects[index]
	if state.activityInFlight {
		return nil
	}
	state.activityGeneration++
	generation := state.activityGeneration
	state.activityInFlight = true
	base := state.activityCursor
	query, ctx, ref := m.query, m.ctx, state.ref
	return func() tea.Msg {
		batch, err := query.ReadActivity(ctx, ref, runID, executionID, attemptID, path, base)
		return activityResultMsg{projectKey: ref.Key, identity: ref.Identity, key: key, generation: generation, base: base, batchActivity: batch.Activity, raw: batch.Raw, next: batch.Cursor, reset: batch.Reset, diagnostics: batch.Diagnostics, err: err}
	}
}

func selectedNativeTaskID(state projectState) string {
	if state.ref.Config.TaskAuthority != "native" {
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
	query, ctx, ref, filter := m.query, m.ctx, state.ref, state.conversationFilter
	return func() tea.Msg {
		page, err := query.ReadConversation(ctx, ref, filter, cursor)
		return conversationMsg{projectKey: ref.Key, identity: ref.Identity, key: key, generation: generation, cursor: cursor, mode: mode, page: page, err: err}
	}
}

func (m Model) readCapabilities(index int, events []steerline.ConversationEvent, key string) tea.Cmd {
	state := &m.projects[index]
	if state.capabilityInFlight {
		return nil
	}
	state.capabilityInFlight = true
	state.capabilityGeneration++
	generation := state.capabilityGeneration
	query, ctx, ref := m.query, m.ctx, state.ref
	events = append([]steerline.ConversationEvent(nil), events...)
	return func() tea.Msg {
		seen := map[string]bool{}
		var reports []steerline.ConversationCapabilityReport
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
			current, err := query.ReadCapabilities(ctx, ref, event.Backend, event.Protocol, event.TargetID, event.ExecutionID, event.AttemptID, event.Source, event.SourceIdentity)
			if err != nil {
				queryErrors = append(queryErrors, err)
				continue
			}
			reports = append(reports, current...)
		}
		return capabilityMsg{projectKey: ref.Key, identity: ref.Identity, key: key, generation: generation, reports: reports, err: errors.Join(queryErrors...)}
	}
}

func (m Model) readBody(index int, event steerline.ConversationEvent, offset int64) tea.Cmd {
	state := &m.projects[index]
	if event.Body == nil || state.bodyBusy {
		return nil
	}
	state.bodyBusy = true
	state.bodyGeneration++
	generation := state.bodyGeneration
	ref, body, query, ctx := state.ref, *event.Body, m.query, m.ctx
	return func() tea.Msg {
		page, err := query.ReadBody(ctx, ref, body, offset, 64<<10)
		return bodyMsg{projectKey: ref.Key, identity: ref.Identity, eventKey: conversationItemKey(event), generation: generation, body: body, offset: offset, page: page, err: err}
	}
}

func (m Model) submitControl(index int, action, taskID, requestID string, instance string, revision uint64) tea.Cmd {
	state := &m.projects[index]
	ref := state.ref
	payload := any(nil)
	switch action {
	case "start_task":
		profile := state.actions[requestID]
		if profile != nil && profile.ExecutionProfileID != "" {
			payload = map[string]string{"task_id": taskID, "execution_profile_id": profile.ExecutionProfileID, "execution_profile_digest": profile.ExecutionProfileDigest}
		} else {
			payload = map[string]string{"task_id": taskID}
		}
	case "set_dispatch_mode":
		payload = map[string]string{"mode": taskID}
	}
	query := m.control
	ctx := m.ctx
	return func() tea.Msg {
		request, err := query.Submit(ctx, ref, action, payload, requestID, instance, &revision)
		return actionSubmittedMsg{projectKey: ref.Key, identity: ref.Identity, instance: instance, requestID: requestID, taskID: taskID, action: action, request: request, err: err}
	}
}

func (m Model) pollAction(index int, action *actionState) tea.Cmd {
	state := &m.projects[index]
	ref := state.ref
	query := m.control
	ctx := m.ctx
	requestID := action.RequestID
	if action.Action == "answer_decision" || validRecoveryAction(action.Action) {
		decisionQuery, ok := query.(DecisionCommandClient)
		return func() tea.Msg {
			if !ok {
				return actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: requestID, err: errors.New("decision command query is unavailable")}
			}
			command, err := decisionQuery.DecisionCommand(ctx, ref, requestID)
			if err != nil {
				return actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: requestID, err: err}
			}
			return actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: requestID, command: &command}
		}
	}
	return func() tea.Msg {
		receipt, err := query.Receipt(ctx, ref, requestID)
		if err == nil {
			return actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: requestID, receipt: receipt}
		}
		request, requestErr := query.Request(ctx, ref, requestID)
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
	base                      steerline.JournalCursor
	batchActivity             []steerline.ActivityEvent
	raw                       []string
	next                      steerline.JournalCursor
	reset                     bool
	diagnostics               []string
	err                       error
}
type conversationMsg struct {
	projectKey, identity, key string
	generation                uint64
	cursor                    string
	mode                      string
	page                      steerline.ConversationPage
	err                       error
}
type capabilityMsg struct {
	projectKey, identity, key string
	generation                uint64
	reports                   []steerline.ConversationCapabilityReport
	err                       error
}
type bodyMsg struct {
	projectKey, identity, eventKey string
	generation                     uint64
	body                           steerline.ConversationBodyRef
	offset                         int64
	page                           steerline.ConversationBodyPage
	err                            error
}
type actionSubmittedMsg struct {
	projectKey, identity, instance, requestID, taskID, action string
	request                                                   steerline.ControlRequest
	command                                                   *steerline.CommandRecord
	err                                                       error
}

func (m Model) submitDecisionCommand(index int, answer steerline.DecisionAnswer, requestedBy, commandID string) tea.Cmd {
	state := &m.projects[index]
	ref := state.ref
	query, ok := m.control.(DecisionCommandClient)
	ctx := m.ctx
	return func() tea.Msg {
		if !ok {
			return actionSubmittedMsg{projectKey: ref.Key, identity: ref.Identity, instance: answer.ServiceInstanceID, requestID: commandID, taskID: "", action: "answer_decision", err: errors.New("decision command submission is unavailable")}
		}
		command, _, err := query.SubmitDecision(ctx, ref, answer, requestedBy, commandID)
		message := actionSubmittedMsg{projectKey: ref.Key, identity: ref.Identity, instance: answer.ServiceInstanceID, requestID: commandID, taskID: answer.RunID, action: "answer_decision", err: err}
		if err == nil {
			message.command = &command
		}
		return message
	}
}

func (m Model) submitServiceAction(index int, action, targetID, requestedBy, commandID string, revision uint64, body any) tea.Cmd {
	state := &m.projects[index]
	ref := state.ref
	query, ok := m.control.(DecisionCommandClient)
	ctx := m.ctx
	return func() tea.Msg {
		if !ok {
			return actionSubmittedMsg{projectKey: ref.Key, identity: ref.Identity, instance: "", requestID: commandID, taskID: targetID, action: action, err: errors.New("service command submission is unavailable")}
		}
		command, _, err := query.SubmitServiceAction(ctx, ref, action, targetID, requestedBy, revision, commandID, body)
		instance := ""
		if runtimeState := state.snapshot.Connection.Snapshot.Runtime; runtimeState != nil {
			instance = runtimeState.Service.InstanceID
		}
		message := actionSubmittedMsg{projectKey: ref.Key, identity: ref.Identity, instance: instance, requestID: commandID, taskID: targetID, action: action, err: err}
		if err == nil {
			message.command = &command
		}
		return message
	}
}

func validRecoveryAction(action string) bool {
	switch action {
	case "retry_run", "mark_stopped", "resolve_command":
		return true
	default:
		return false
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
	eligibility, ok := m.control.(DecisionCommandClient)
	if !ok {
		m.status = "当前控制客户端不支持决策资格核验"
		return m, nil
	}
	if err := eligibility.DecisionEligibility(m.ctx, state.ref, row.run); err != nil {
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
	var run *steerline.Run
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
	requestID := steerline.NewControlRequestID()
	answer := steerline.DecisionAnswer{DecisionID: dialog.decisionID, RunID: dialog.runID, OptionID: optionID, Note: note, AnsweredBy: steerline.AnswererName(), ExpectedDecisionVersion: dialog.decisionVersion, ExpectedRunRevision: dialog.runRevision, ServiceInstanceID: dialog.instance}
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
	return m, m.submitDecisionCommand(index, answer, answer.AnsweredBy, requestID)
}

type actionReceiptMsg struct {
	projectKey, identity, requestID string
	receipt                         *steerline.ControlReceipt
	request                         *steerline.ControlRequest
	command                         *steerline.CommandRecord
	err                             error
}
type recoveryInspectionMsg struct {
	projectKey, identity, runID string
	generation                  uint64
	report                      steerline.RecoveryInspection
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

func applyDecisionCommandResult(state *projectState, action *actionState, command *steerline.CommandRecord, commandErr error) (refresh, reopen bool) {
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
		switch action.Action {
		case "answer_decision":
			action.Message = "答案已保存；服务将单独核验原执行与续跑条件"
		case "retry_run":
			action.Message = "新 attempt 已启动并保存执行身份"
		case "mark_stopped":
			action.Message = "原执行停止事实已记录；任务和工作目录保留"
		case "resolve_command":
			action.Message = "指定 blocker 已按证据解除；原指令未重放"
		}
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

func decisionRequestFor(state projectState, runID, decisionID string) (steerline.DecisionRequest, bool) {
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
				var request steerline.DecisionRequest
				if json.Unmarshal(record.Request, &request) == nil {
					return request, true
				}
			}
		}
	}
	return steerline.DecisionRequest{}, false
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
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		var cmds []tea.Cmd
		now := m.clock.Now()
		for i := range m.projects {
			state := &m.projects[i]
			interval := 5 * time.Second
			if i == m.selected {
				interval = time.Second
			}
			if state.ref.Legacy != nil {
				interval = 15 * time.Second
				if i == m.selected {
					interval = 5 * time.Second
				}
			}
			if !state.refreshing && (state.lastRefresh.IsZero() || now.Sub(state.lastRefresh) >= interval) {
				cmds = append(cmds, m.readProject(i))
			}
			for _, action := range state.actions {
				if !action.InFlight && !action.Resolved && action.Status != "submitting" {
					action.InFlight = true
					cmds = append(cmds, m.pollAction(i, action))
				}
			}
		}
		if m.selected >= 0 && m.selected < len(m.projects) {
			var cmd tea.Cmd
			m, cmd = m.refreshSelectedLogs(m.selected)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			state := &m.projects[m.selected]
			if state.panel == 2 && state.conversationKey != "" && !state.conversationInFlight && !state.conversationTailMore {
				cursor, mode := "", "head"
				if state.conversationTailCursor != "" {
					cursor, mode = state.conversationTailCursor, "tail"
				}
				cmds = append(cmds, m.readConversation(m.selected, cursor, state.conversationKey, mode))
			}
		}
		cmds = append(cmds, waitTick(m.clock, 800*time.Millisecond))
		return m, tea.Batch(cmds...)
	case projectMsg:
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
			for _, row := range rows {
				if row.kind == rowRun && (row.run.RunID == m.runFocus || row.run.TaskID == m.runFocus) {
					state.selectedKey, m.runFocus = row.key, ""
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
		var cmds []tea.Cmd
		if index == m.selected {
			var cmd tea.Cmd
			m, cmd = m.refreshSelectedLogs(index)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if previousSelection != state.selectedKey {
			clearReadingAnchor(state)
			m.saveProjectView(index)
		}
		return m, tea.Batch(cmds...)
	case recoveryInspectionMsg:
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
		state.recoveryError = ""
		state.recoveryReport = &msg.report
		if index == m.selected {
			m.status = "恢复核验报告已刷新 · run " + msg.report.RunID + " · revision " + strconv.FormatUint(msg.report.RunRevision, 10)
		}
		return m, nil
	case activityResultMsg:
		index := m.projectIndex(msg.projectKey)
		if index < 0 {
			return m, nil
		}
		state := &m.projects[index]
		state.activityInFlight = false
		if index != m.selected || msg.identity != state.ref.Identity || msg.generation != state.activityGeneration || msg.key != state.activityKey || !sameCursor(msg.base, state.activityCursor) {
			return m, nil
		}
		if msg.err != nil {
			state.activityError = msg.err.Error()
			return m, nil
		}
		if msg.reset {
			state.activityEvents, state.activityRaw = nil, nil
		}
		state.activityEvents = append(state.activityEvents, msg.batchActivity...)
		state.activityRaw = append(state.activityRaw, msg.raw...)
		if len(state.activityEvents) > 4000 {
			excess := len(state.activityEvents) - 4000
			if !state.view.Follow {
				for _, event := range state.activityEvents[:excess] {
					state.logScroll -= activityEventLineCount(event, activityPanelWidth(m))
				}
				if state.logScroll < 0 {
					state.logScroll = 0
				}
			}
			state.activityEvents = state.activityEvents[excess:]
		}
		if len(state.activityRaw) > 4000 {
			state.activityRaw = state.activityRaw[len(state.activityRaw)-4000:]
		}
		state.activityCursor = msg.next
		state.activityError = strings.Join(msg.diagnostics, "; ")
		return m, nil
	case conversationMsg:
		index := m.projectIndex(msg.projectKey)
		if index < 0 {
			return m, nil
		}
		state := &m.projects[index]
		state.conversationInFlight = false
		if index != m.selected || msg.identity != state.ref.Identity || msg.generation != state.conversationGeneration || msg.key != state.conversationKey {
			return m, nil
		}
		if msg.err != nil {
			state.conversationError = msg.err.Error()
			return m, nil
		}
		if msg.mode == "head" {
			state.conversationEvents = append([]steerline.ConversationEvent(nil), msg.page.Events...)
		} else {
			seen := map[int64]bool{}
			for _, event := range state.conversationEvents {
				if event.EventID > 0 {
					seen[event.EventID] = true
				}
			}
			for _, event := range msg.page.Events {
				if event.EventID == 0 || !seen[event.EventID] {
					state.conversationEvents = append(state.conversationEvents, event)
					if event.EventID > 0 {
						seen[event.EventID] = true
					}
				}
			}
		}
		sort.SliceStable(state.conversationEvents, func(i, j int) bool { return state.conversationEvents[i].EventID < state.conversationEvents[j].EventID })
		switch msg.mode {
		case "head":
			state.conversationMore = msg.page.HasMore
			state.conversationCursor = msg.page.NextCursor
			state.conversationTailMore = false
		case "page":
			state.conversationMore = msg.page.HasMore
			state.conversationCursor = msg.page.NextCursor
			if !msg.page.HasMore {
				state.conversationTailMore = false
			}
		case "tail":
			state.conversationTailMore = msg.page.HasMore
			if state.conversationCursor == "" && msg.page.HasMore {
				state.conversationMore = true
				state.conversationCursor = msg.page.NextCursor
			}
		}
		if len(state.conversationEvents) > 0 {
			latest := state.conversationEvents[len(state.conversationEvents)-1].EventID
			if cursor, err := steerline.ConversationCursorAfter(state.ref.Identity, state.conversationFilter, latest); err == nil {
				state.conversationTailCursor = cursor
			}
		}
		state.conversationError = ""
		state.capabilities = nil
		conversationView, changed := BuildConversationView(state.conversationEvents, state.view.Groups)
		if state.view.AnchorGroup == "" && len(conversationView.Groups) > 0 {
			state.view.AnchorGroup = conversationView.Groups[len(conversationView.Groups)-1].ID
			changed = true
		}
		if changed {
			m.saveProjectView(index)
		}
		if state.panel == 2 && state.view.ReadGroup != "" {
			oldScroll := state.detailScroll
			hidden := restoreSessionAnchor(state, m.selectedRow(index), activityPanelWidth(m))
			if hidden {
				m.status = "会话阅读锚点已收起；已定位到轮次标题"
			}
			if state.detailScroll != oldScroll {
				m.saveProjectView(index)
			}
		}
		var cmds []tea.Cmd
		if len(state.conversationEvents) > 0 {
			cmds = append(cmds, m.readCapabilities(index, state.conversationEvents, state.conversationKey))
		}
		if state.bodyRestorePending {
			if event, ok := findBodyEvent(state.conversationEvents, state.view.AnchorItem, state.view.AnchorRevision); ok {
				state.bodyRef, state.bodyEvent = event.Body, conversationItemKey(event)
				state.bodyViewOffset = state.view.BodyOffset
				cmds = append(cmds, m.readBody(index, event, state.bodyViewOffset))
			} else if !msg.page.HasMore {
				state.bodyError = "先前阅读的正文版本未出现在本页；继续加载会话记录"
				state.bodyRestorePending = false
			}
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil
	case capabilityMsg:
		index := m.projectIndex(msg.projectKey)
		if index < 0 {
			return m, nil
		}
		state := &m.projects[index]
		if msg.generation != state.capabilityGeneration {
			return m, nil
		}
		state.capabilityInFlight = false
		if index != m.selected || msg.identity != state.ref.Identity || msg.key != state.conversationKey {
			return m, nil
		}
		state.capabilities = msg.reports
		if msg.err != nil {
			state.capabilityError = msg.err.Error()
		} else {
			state.capabilityError = ""
		}
		return m, nil
	case bodyMsg:
		index := m.projectIndex(msg.projectKey)
		if index < 0 {
			return m, nil
		}
		state := &m.projects[index]
		if msg.generation != state.bodyGeneration {
			return m, nil
		}
		state.bodyBusy = false
		if msg.identity != state.ref.Identity || state.bodyEvent != msg.eventKey || state.bodyRef == nil || *state.bodyRef != msg.body {
			return m, nil
		}
		if msg.err != nil {
			state.bodyError = msg.err.Error()
			return m, nil
		}
		if !validBodyPage(msg.body, msg.offset, msg.page) {
			state.bodyError = "正文响应与请求的 item/revision/offset 或完整性不匹配"
			return m, nil
		}
		state.bodyOffset = msg.page.NextOffset
		state.bodyViewOffset = msg.page.Offset
		state.bodyPages = appendBodyPage(state.bodyPages, msg.page.Offset)
		state.bodyText = string(msg.page.Data)
		state.bodyMore = !msg.page.Complete
		state.bodyError = ""
		state.bodyRestorePending = false
		m.saveProjectView(index)
		return m, nil
	case actionSubmittedMsg:
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
		if action.Action == "answer_decision" || validRecoveryAction(action.Action) {
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
				state.selectedKey = ""
				state.selectionHold = true
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
	case actionReceiptMsg:
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
		if action.Action == "answer_decision" || validRecoveryAction(action.Action) {
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
			if msg.receipt.RequestID != action.RequestID || msg.receipt.Action != action.Action || msg.receipt.InstanceID != action.TargetInstance {
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
				state.selectedKey = ""
				state.selectionHold = true
				refreshNow = true
			}
			if msg.receipt.Status == "applied" && msg.receipt.RunID != "" && index == m.selected {
				state.selectedKey = "run:" + msg.receipt.RunID
				clearReadingAnchor(state)
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
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func controlRequestMatchesAction(request *steerline.ControlRequest, action *actionState, repositoryIdentity string) bool {
	if request == nil || action == nil || request.RequestID != action.RequestID || request.Action != action.Action || request.InstanceID != action.TargetInstance || viewCanonical(request.RepositoryIdentity) != viewCanonical(repositoryIdentity) {
		return false
	}
	var payload struct {
		TaskID                 string `json:"task_id"`
		Mode                   string `json:"mode"`
		ExecutionProfileID     string `json:"execution_profile_id"`
		ExecutionProfileDigest string `json:"execution_profile_digest"`
	}
	if len(request.Payload) > 0 && json.Unmarshal(request.Payload, &payload) != nil {
		return false
	}
	switch action.Action {
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
	runsByTask := map[string]*steerline.Run{}
	if runtime := state.snapshot.Observation.Runtime; runtime != nil {
		for _, run := range runtime.Runs {
			if run != nil {
				runsByTask[firstText(run.DomainTaskID, run.TaskID)] = run
			}
		}
	}
	shownRuns := map[string]bool{}
	appendRun := func(run *steerline.Run) {
		rows = append(rows, consoleRow{key: "run:" + run.RunID, kind: rowRun, taskID: run.TaskID, title: run.Title, run: run})
		shownRuns[run.RunID] = true
	}
	nativeByID := map[string]*steerline.NativeTaskDetail{}
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
	if state.filter == 0 && state.ref.Legacy != nil && state.snapshot.Legacy != nil {
		for i := range state.snapshot.Legacy.Tasks {
			task := &state.snapshot.Legacy.Tasks[i]
			rows = append(rows, consoleRow{key: "legacy:" + task.ID, kind: rowLegacy, taskID: task.ID, title: task.Title, legacy: task})
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

func filterAllows(filter int, kind rowKind, run *steerline.Run) bool {
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
		return run != nil && run.ExecutionStatus == "running"
	case 3:
		return run != nil && run.FinishTaskStatus == "waiting_decision"
	case 4:
		return run != nil && run.FinishTaskStatus == "waiting_pr_merge"
	case 5:
		return runNeedsAttention(run)
	case 6:
		return run != nil && runStatus(run) == "completed"
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
	return "detail"
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
		state.activityEvents, state.activityRaw = nil, nil
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
	state.attemptID = attemptID
	var activityPath string
	if attemptID == run.AttemptID && run.LogPath != nil {
		activityPath = *run.LogPath
	}
	activityKey := strings.Join([]string{run.RunID, run.ExecutionID, attemptID, activityPath}, "\x00")
	if state.activityKey != activityKey {
		state.activityKey, state.activityCursor = activityKey, steerline.JournalCursor{}
		state.activityEvents, state.activityRaw, state.activityError = nil, nil, ""
		state.activityGeneration++
	}
	conversationKey := strings.Join([]string{run.ExecutionID, attemptID}, "\x00")
	var cmds []tea.Cmd
	if state.conversationKey != conversationKey {
		state.conversationKey = conversationKey
		state.conversationFilter = steerline.ConversationFilter{WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: attemptID}
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
		cmds = append(cmds, m.readActivity(index, run.RunID, run.ExecutionID, attemptID, activityPath, activityKey))
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
	if state.ref.Legacy != nil && filter != 0 {
		m.status = "旧 Beads 项目仅提供只读全量任务列表"
		return m, nil
	}
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

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.profileChoice != nil {
		return m.handleExecutionProfileChoice(key)
	}
	if m.answer != nil {
		return m.handleDecisionAnswerKey(key)
	}
	if m.confirm != nil {
		return m.handleConfirm(key)
	}
	if m.help {
		if key.String() == "?" || key.String() == "esc" {
			m.help = false
		}
		return m, nil
	}
	if m.selected < 0 || m.selected >= len(m.projects) {
		if key.String() == "q" || key.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	state := &m.projects[m.selected]
	if state.panel == 6 {
		switch key.String() {
		case "esc":
			state.panel = 0
			m.saveProjectView(m.selected)
			return m, nil
		case "r":
			return m.inspectSelectedRecovery()
		case "R":
			return m.prepareRecoveryAction("retry_run")
		case "m":
			return m.prepareRecoveryAction("mark_stopped")
		case "v":
			return m.prepareRecoveryAction("resolve_command")
		case "[", "]":
			if state.recoveryReport != nil && len(state.recoveryReport.Blockers) > 0 {
				count := len(state.recoveryReport.Blockers)
				if key.String() == "]" {
					state.recoveryBlockerIndex = (state.recoveryBlockerIndex + 1) % count
				} else {
					state.recoveryBlockerIndex = (state.recoveryBlockerIndex - 1 + count) % count
				}
			}
			return m, nil
		}
	}
	switch key.String() {
	case "q", "ctrl+c":
		m.saveProjectView(m.selected)
		return m, tea.Quit
	case "left", "h":
		return m.moveProject(-1)
	case "right", "l":
		return m.moveProject(1)
	case "up", "k":
		if m.width < 100 && state.panel != 4 || m.width >= 100 && state.panel != 0 {
			return m.scrollPanel(-1)
		}
		return m.moveSelection(-1)
	case "down", "j":
		if m.width < 100 && state.panel != 4 || m.width >= 100 && state.panel != 0 {
			return m.scrollPanel(1)
		}
		return m.moveSelection(1)
	case "r":
		state.lastRefresh = time.Time{}
		return m, m.readProject(m.selected)
	case "enter":
		if state.panel != 0 {
			state.panel = 0
			m.saveProjectView(m.selected)
			return m, nil
		}
		state.panel = 0
		return m, nil
	case "d":
		state.panel = 0
		m.saveProjectView(m.selected)
		return m, nil
	case "t":
		if m.width < 100 {
			state.panel = 4
		} else {
			state.panel = 0
		}
		m.saveProjectView(m.selected)
		return m, nil
	case "a":
		if row := m.selectedRow(m.selected); row != nil && row.run != nil && row.run.Decision != nil && row.run.Decision.Status == "pending" {
			return m.answerSelectedDecision()
		}
		state.panel = 1
		m.saveProjectView(m.selected)
		return m, nil
	case "A":
		state.panel = 1
		m.saveProjectView(m.selected)
		return m, nil
	case "c":
		state.panel = 2
		m.saveProjectView(m.selected)
		return m, nil
	case "f":
		state.panel = 3
		m.finalIndex = 0
		m.saveProjectView(m.selected)
		return m, nil
	case "u":
		return m.inspectSelectedRecovery()
	case "v":
		state.panel = 5
		m.saveProjectView(m.selected)
		return m, nil
	case "1":
		return m.changeFilter(0)
	case "2":
		return m.changeFilter(1)
	case "3":
		return m.changeFilter(2)
	case "4":
		return m.changeFilter(3)
	case "5":
		return m.changeFilter(4)
	case "6":
		return m.changeFilter(5)
	case "7":
		return m.changeFilter(6)
	case "[", "]":
		return m.cycleAttempt(key.String() == "]")
	case "s":
		return m.startSelected()
	case "m":
		return m.toggleMode()
	case "p":
		return m.togglePause()
	case "x":
		return m.prepareStop()
	case "i":
		return m.reconcileLatest()
	case "e":
		return m.toggleGroup()
	case ",":
		return m.cycleGroup(-1)
	case ".":
		return m.cycleGroup(1)
	case "=":
		return m.keepGroupOpen()
	case "n":
		return m.nextFinal()
	case "o":
		return m.openProcessBody()
	case "y":
		return m.loadMoreConversation()
	case "<":
		return m.pageBody(false)
	case ">":
		return m.pageBody(true)
	case "g":
		if state.panel == 1 {
			lines := allActivityLines(*state, m.selectedRow(m.selected), activityPanelWidth(m))
			height := activityPanelHeight(m)
			state.logScroll = max(0, len(lines)-height)
		}
		state.view.Follow = false
		m.saveProjectView(m.selected)
		return m, nil
	case "G":
		state.view.Follow = true
		state.logScroll = 0
		m.status = "已跳到最新"
		m.saveProjectView(m.selected)
		var cmd tea.Cmd
		m, cmd = m.refreshSelectedLogs(m.selected)
		return m, cmd
	case "pgup":
		return m.scrollPanel(-8)
	case "pgdown":
		return m.scrollPanel(8)
	case "?":
		m.help = true
		return m, nil
	}
	return m, nil
}

func (m Model) handleDecisionAnswerKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	dialog := m.answer
	if dialog == nil {
		return m, nil
	}
	keyName := key.String()
	switch keyName {
	case "esc", "ctrl+c":
		m.answer = nil
		m.status = "已关闭答案草稿；答案未提交"
		return m, nil
	case "f2":
		return m.submitDecisionAnswer()
	case "tab":
		dialog.noteFocus = !dialog.noteFocus
		dialog.error = ""
		return m, nil
	case "pgup":
		dialog.scroll = max(0, dialog.scroll-8)
		return m, nil
	case "pgdown":
		dialog.scroll += 8
		return m, nil
	case "up", "down":
		if dialog.noteFocus {
			dialog.cursor = moveRuneCursorVertical(dialog.note, dialog.cursor, keyName == "down")
			return m, nil
		}
		state := &m.projects[m.selected]
		request, ok := decisionRequestFor(*state, dialog.runID, dialog.decisionID)
		if !ok {
			dialog.error = "原问题详情暂不可读；草稿保留"
			return m, nil
		}
		count := len(request.Options)
		if request.AllowFreeText {
			count++
		}
		if count > 0 {
			if keyName == "down" {
				dialog.optionIndex = (dialog.optionIndex + 1 + count) % count
			} else {
				dialog.optionIndex = (dialog.optionIndex - 1 + count) % count
			}
		}
		dialog.error = ""
		return m, nil
	case "left":
		if dialog.noteFocus && dialog.cursor > 0 {
			dialog.cursor--
		}
		return m, nil
	case "right":
		if dialog.noteFocus && dialog.cursor < len(dialog.note) {
			dialog.cursor++
		}
		return m, nil
	case "home":
		if dialog.noteFocus {
			dialog.cursor = runeLineStart(dialog.note, dialog.cursor)
		}
		return m, nil
	case "end":
		if dialog.noteFocus {
			dialog.cursor = runeLineEnd(dialog.note, dialog.cursor)
		}
		return m, nil
	case "backspace":
		if dialog.noteFocus && dialog.cursor > 0 {
			dialog.note = append(dialog.note[:dialog.cursor-1], dialog.note[dialog.cursor:]...)
			dialog.cursor--
		}
		dialog.error = ""
		return m, nil
	case "delete":
		if dialog.noteFocus && dialog.cursor < len(dialog.note) {
			dialog.note = append(dialog.note[:dialog.cursor], dialog.note[dialog.cursor+1:]...)
		}
		dialog.error = ""
		return m, nil
	case "enter":
		if !dialog.noteFocus {
			dialog.noteFocus = true
			return m, nil
		}
		insertAnswerRunes(dialog, []rune{'\n'})
		dialog.error = ""
		return m, nil
	}
	if !dialog.noteFocus {
		if key.Type == tea.KeyRunes && len(key.Runes) == 1 && key.Runes[0] >= '1' && key.Runes[0] <= '9' {
			state := &m.projects[m.selected]
			request, ok := decisionRequestFor(*state, dialog.runID, dialog.decisionID)
			if ok && int(key.Runes[0]-'1') < len(request.Options) {
				dialog.optionIndex = int(key.Runes[0] - '1')
			}
		}
		return m, nil
	}
	if key.Type == tea.KeyRunes && len(key.Runes) > 0 {
		insertAnswerRunes(dialog, key.Runes)
		dialog.error = ""
	}
	return m, nil
}

func insertAnswerRunes(dialog *decisionAnswerDialog, runes []rune) {
	if dialog == nil || len(runes) == 0 {
		return
	}
	insert := strings.ReplaceAll(string(runes), "\r\n", "\n")
	insert = strings.ReplaceAll(insert, "\r", "\n")
	filtered := make([]rune, 0, len(insert))
	for _, char := range insert {
		if char == '\n' || char == '\t' || char >= 0x20 && char != 0x7f {
			filtered = append(filtered, char)
		}
	}
	if len(filtered) == 0 {
		return
	}
	joined := make([]rune, 0, len(dialog.note)+len(filtered))
	joined = append(joined, dialog.note[:dialog.cursor]...)
	joined = append(joined, filtered...)
	joined = append(joined, dialog.note[dialog.cursor:]...)
	dialog.note = joined
	dialog.cursor += len(filtered)
}

func runeLineStart(text []rune, cursor int) int {
	if cursor > len(text) {
		cursor = len(text)
	}
	for cursor > 0 && text[cursor-1] != '\n' {
		cursor--
	}
	return cursor
}

func runeLineEnd(text []rune, cursor int) int {
	if cursor > len(text) {
		cursor = len(text)
	}
	for cursor < len(text) && text[cursor] != '\n' {
		cursor++
	}
	return cursor
}

func moveRuneCursorVertical(text []rune, cursor int, down bool) int {
	start, end := runeLineStart(text, cursor), runeLineEnd(text, cursor)
	column := cursor - start
	if down {
		if end >= len(text) {
			return cursor
		}
		nextStart := end + 1
		nextEnd := runeLineEnd(text, nextStart)
		return min(nextStart+column, nextEnd)
	}
	if start == 0 {
		return cursor
	}
	previousEnd := start - 1
	previousStart := runeLineStart(text, previousEnd)
	return min(previousStart+column, previousEnd)
}

func (m Model) handleConfirm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	confirm := m.confirm
	if confirm.requireBasis {
		switch key.String() {
		case "esc", "ctrl+c":
			m.confirm = nil
			return m, nil
		case "f2":
			basis := strings.TrimSpace(confirm.basis)
			if basis == "" {
				confirm.basisError = "请填写亲自核验的停止或解除依据；未提交命令"
				return m, nil
			}
			if len([]rune(basis)) > 2000 {
				confirm.basisError = "依据超过 2000 字符；内容未截断，请编辑后再提交"
				return m, nil
			}
			confirm.basis = basis
			m.confirm = nil
			return m.beginRecoveryAction(confirm)
		case "pgup":
			confirm.scroll = max(0, confirm.scroll-8)
			return m, nil
		case "pgdown":
			confirm.scroll += 8
			return m, nil
		case "up", "down":
			confirm.basisCursor = moveRuneCursorVertical([]rune(confirm.basis), confirm.basisCursor, key.String() == "down")
			confirm.scroll = 1 << 20
			return m, nil
		case "left":
			confirm.basisCursor = max(0, confirm.basisCursor-1)
			return m, nil
		case "right":
			confirm.basisCursor = min(len([]rune(confirm.basis)), confirm.basisCursor+1)
			return m, nil
		case "home":
			confirm.basisCursor = runeLineStart([]rune(confirm.basis), confirm.basisCursor)
			return m, nil
		case "end":
			confirm.basisCursor = runeLineEnd([]rune(confirm.basis), confirm.basisCursor)
			return m, nil
		case "backspace":
			if confirm.basisCursor > 0 {
				text := []rune(confirm.basis)
				confirm.basis = string(append(text[:confirm.basisCursor-1], text[confirm.basisCursor:]...))
				confirm.basisCursor--
			}
			confirm.basisError = ""
			confirm.scroll = 1 << 20
			return m, nil
		case "delete":
			text := []rune(confirm.basis)
			if confirm.basisCursor < len(text) {
				confirm.basis = string(append(text[:confirm.basisCursor], text[confirm.basisCursor+1:]...))
			}
			confirm.basisError = ""
			confirm.scroll = 1 << 20
			return m, nil
		case "enter":
			insertRecoveryBasisRunes(confirm, []rune{'\n'})
			return m, nil
		}
		if key.Type == tea.KeyRunes {
			insertRecoveryBasisRunes(confirm, key.Runes)
		}
		return m, nil
	}
	switch key.String() {
	case "up":
		confirm.scroll = max(0, confirm.scroll-1)
		return m, nil
	case "down":
		confirm.scroll++
		return m, nil
	case "pgup":
		confirm.scroll = max(0, confirm.scroll-8)
		return m, nil
	case "pgdown":
		confirm.scroll += 8
		return m, nil
	case "esc", "n":
		m.confirm = nil
		return m, nil
	case "enter", "y":
		m.confirm = nil
		if confirm.action != "stop_service" {
			return m.beginRecoveryAction(confirm)
		}
		return m.beginAction(confirm.projectKey, confirm.instance, confirm.revision, confirm.action, confirm.taskID, confirm.requestID)
	default:
		return m, nil
	}
}

func insertRecoveryBasisRunes(confirm *confirmAction, input []rune) {
	if confirm == nil || len(input) == 0 {
		return
	}
	filtered := make([]rune, 0, len(input))
	for _, r := range input {
		if r == '\r' {
			r = '\n'
		}
		if r == '\n' || r == '\t' || r >= 0x20 && r != 0x7f {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		return
	}
	text := []rune(confirm.basis)
	confirm.basisCursor = min(max(0, confirm.basisCursor), len(text))
	joined := make([]rune, 0, len(text)+len(filtered))
	joined = append(joined, text[:confirm.basisCursor]...)
	joined = append(joined, filtered...)
	joined = append(joined, text[confirm.basisCursor:]...)
	confirm.basis, confirm.basisCursor = string(joined), confirm.basisCursor+len(filtered)
	confirm.basisError = ""
	confirm.scroll = 1 << 20
}

func (m Model) startSelected() (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	if state.ref.Legacy != nil {
		m.status = "旧 Beads 项目只读；原执行由旧服务结算，切换到 native 后才能在此启动"
		return m, nil
	}
	row := m.selectedRow(m.selected)
	if row == nil || row.kind != rowReady || row.candidate == nil {
		m.status = "请选择 Ready 候选后再开始"
		return m, nil
	}
	valid, reason := candidateFreshness(state.ref, state.snapshot, m.clock.Now())
	if !valid {
		m.status = "不能开始：" + reason
		return m, nil
	}
	if executionConfigMismatch(state.ref, state.snapshot) {
		m.status = "不能开始：" + executionConfigChanged
		return m, nil
	}
	runtimeState := state.snapshot.Connection.Snapshot.Runtime
	if runtimeState == nil {
		m.status = "不能开始：缺少当前服务控制快照"
		return m, nil
	}
	if state.ref.Conflict != "" {
		m.status = state.ref.Conflict
		return m, nil
	}
	if runtimeState.Control.DispatchMode != "manual" {
		m.status = "开始任务要求手动派发模式"
		return m, nil
	}
	if runtimeState.Control.DispatchPaused {
		m.status = "派发已暂停，先恢复派发"
		return m, nil
	}
	if runtimeState.SlotOccupied {
		m.status = "执行槽正忙：" + stringValue(runtimeState.CurrentTask)
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
		if m.projects[m.selected].ref.Config.DefaultProfileID() == "" {
			m.status = "自动派发需要在项目配置中指定 default_execution_profile"
			return m, nil
		}
		if executionConfigMismatch(m.projects[m.selected].ref, m.projects[m.selected].snapshot) {
			m.status = "不能切换自动派发：" + executionConfigChanged
			return m, nil
		}
		m.status = "确认前请留意：切到 auto 后服务可能领取 Ready 任务"
	}
	return m.createAction("set_dispatch_mode", mode)
}

func (m Model) togglePause() (tea.Model, tea.Cmd) {
	_, runtimeState := m.readyControlTarget()
	if runtimeState == nil {
		return m, nil
	}
	if runtimeState.Control.DispatchPaused {
		if runtimeState.Control.DispatchMode == "auto" && executionConfigMismatch(m.projects[m.selected].ref, m.projects[m.selected].snapshot) {
			m.status = "不能恢复自动派发：" + executionConfigChanged
			return m, nil
		}
		return m.createAction("resume_dispatch", "")
	}
	return m.createAction("pause_dispatch", "")
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
	m.confirm = &confirmAction{projectKey: state.ref.Key, instance: runtimeState.Service.InstanceID, revision: runtimeState.Control.Revision, action: "stop_service", requestID: steerline.NewControlRequestID()}
	return m, nil
}

func (m Model) reconcileLatest() (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	var latest *actionState
	for _, action := range state.actions {
		if latest == nil || action.UpdatedAt.After(latest.UpdatedAt) {
			latest = action
		}
	}
	if latest == nil {
		m.status = "没有可对账的 request_id"
		return m, nil
	}
	if latest.InFlight {
		m.status = "该 request_id 正在查询：" + latest.RequestID
		return m, nil
	}
	latest.InFlight = true
	return m, m.pollAction(m.selected, latest)
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
	state.recoveryRunID = row.run.RunID
	state.recoveryReport = nil
	state.recoveryError = ""
	state.recoveryBusy = true
	state.recoveryGeneration++
	generation := state.recoveryGeneration
	ref, runID, ctx := state.ref, row.run.RunID, m.ctx
	m.status = "正在只读核验运行、工作目录、领取和 PR 身份"
	return m, func() tea.Msg {
		report, err := steerline.InspectRecovery(ctx, ref.Config, runID)
		return recoveryInspectionMsg{projectKey: ref.Key, identity: ref.Identity, runID: runID, generation: generation, report: report, err: err}
	}
}

func (m Model) prepareRecoveryAction(action string) (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	report := state.recoveryReport
	if report == nil || report.RunID == "" {
		m.status = "先读取当前只读恢复报告"
		return m, nil
	}
	runtimeState := state.snapshot.Connection.Snapshot.Runtime
	if runtimeState == nil || !state.snapshot.Connection.CanControl {
		m.status = "服务未就绪；恢复命令不会提交"
		return m, nil
	}
	requestID := steerline.NewControlRequestID()
	confirm := &confirmAction{projectKey: state.ref.Key, instance: runtimeState.Service.InstanceID, action: action, taskID: report.TaskID, runID: report.RunID, requestID: requestID}
	switch action {
	case "retry_run":
		if report.RetryReason != "" {
			m.status = "retry_run 不可用：" + report.RetryReason
			return m, nil
		}
		if !hasCapability(runtimeState, action) {
			m.status = "服务未声明 retry_run 能力"
			return m, nil
		}
		confirm.revision = report.RunRevision
	case "mark_stopped":
		if !strings.HasPrefix(report.MarkStoppedReason, "confirm") {
			m.status = "mark_stopped 不可用：" + report.MarkStoppedReason
			return m, nil
		}
		if !hasCapability(runtimeState, action) {
			m.status = "服务未声明 mark_stopped 能力"
			return m, nil
		}
		confirm.revision, confirm.requireBasis = report.RunRevision, true
	case "resolve_command":
		if len(report.Blockers) == 0 || state.recoveryBlockerIndex < 0 || state.recoveryBlockerIndex >= len(report.Blockers) {
			m.status = "没有可解除的 control blocker"
			return m, nil
		}
		if !hasCapability(runtimeState, action) {
			m.status = "服务未声明 resolve_command 能力"
			return m, nil
		}
		blocker := report.Blockers[state.recoveryBlockerIndex]
		confirm.taskID, confirm.runID = blocker.TaskID, blocker.RunID
		confirm.decisionID = blocker.RequestID
		confirm.revision = runtimeState.Control.Revision
		confirm.requireBasis = true
	default:
		m.status = "不支持的恢复动作：" + action
		return m, nil
	}
	m.confirm = confirm
	return m, nil
}

func (m Model) beginRecoveryAction(confirm *confirmAction) (tea.Model, tea.Cmd) {
	if confirm == nil {
		return m, nil
	}
	if confirm.requireBasis && (strings.TrimSpace(confirm.basis) == "" || len([]rune(strings.TrimSpace(confirm.basis))) > 2000) {
		confirm.basisError = "恢复依据必须由操作者填写，且不超过 2000 字符；未提交命令"
		m.confirm = confirm
		return m, nil
	}
	index := m.projectIndex(confirm.projectKey)
	if index < 0 || index != m.selected {
		m.status = "恢复目标项目已切换；请回到该项目重新读取报告"
		return m, nil
	}
	state := &m.projects[index]
	runtimeState := state.snapshot.Connection.Snapshot.Runtime
	if runtimeState == nil || runtimeState.Service.InstanceID != confirm.instance || !state.snapshot.Connection.CanControl {
		m.status = "stale_instance：服务实例已变化；未提交恢复命令"
		state.lastRefresh = time.Time{}
		return m, m.readProject(index)
	}
	targetID := confirm.runID
	if confirm.action == "resolve_command" {
		targetID = confirm.decisionID
		if runtimeState.Control.Revision != confirm.revision {
			m.status = "stale_revision：control blocker 已变化；未提交恢复命令"
			return m.inspectSelectedRecovery()
		}
	} else {
		var run *steerline.Run
		for _, candidate := range runtimeState.Runs {
			if candidate != nil && candidate.RunID == confirm.runID {
				run = candidate
				break
			}
		}
		if run == nil || run.ControlRevision != confirm.revision {
			m.status = "stale_revision：run 已变化；未提交恢复命令，请重新核验"
			return m.inspectSelectedRecovery()
		}
	}
	if !hasCapability(runtimeState, confirm.action) {
		m.status = "服务未声明 " + confirm.action + " 能力"
		return m, nil
	}
	requestedBy := steerline.DefaultRequestedBy()
	body := steerline.RecoveryCommandBody{ConfirmedProcessStopped: confirm.action == "mark_stopped" || confirm.action == "retry_run", ConfirmationBasis: confirm.basis}
	if confirm.action == "resolve_command" {
		body.ConfirmationBasis = confirm.basis
	}
	action := &actionState{RequestID: confirm.requestID, Action: confirm.action, TaskID: confirm.taskID, RunID: confirm.runID, DecisionID: targetID, DecisionRevision: confirm.revision, Status: "submitting", Message: "保存 command_id 并提交恢复命令", TargetInstance: confirm.instance, UpdatedAt: m.clock.Now()}
	state.actions[confirm.requestID] = action
	state.view.Outstanding[confirm.requestID] = "recovery:" + confirm.runID
	if err := m.saveProjectView(index); err != nil {
		delete(state.actions, confirm.requestID)
		delete(state.view.Outstanding, confirm.requestID)
		m.status = "未提交恢复命令：无法先保存 command_id"
		return m, nil
	}
	m.confirm = nil
	m.status = fmt.Sprintf("恢复命令已发布 · %s · command_id %s", confirm.action, confirm.requestID)
	return m, m.submitServiceAction(index, confirm.action, targetID, requestedBy, confirm.requestID, confirm.revision, body)
}

func (m *Model) readyControlTarget() (*projectState, *steerline.Runtime) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		m.status = "没有项目"
		return nil, nil
	}
	state := &m.projects[m.selected]
	connection := state.snapshot.Connection
	runtimeState := connection.Snapshot.Runtime
	if state.ref.Error != "" || state.ref.Conflict != "" || runtimeState == nil || !connection.CanControl || connection.State != steerline.ConnectionConnected {
		m.status = "服务未就绪，不能提交控制指令：" + firstText(connection.Diagnostic, string(connection.State), "未知状态")
		return state, nil
	}
	return state, runtimeState
}

func (m Model) createAction(action, taskID string) (tea.Model, tea.Cmd) {
	return m.createActionWithProfile(action, taskID, nil)
}

func (m Model) createActionWithProfile(action, taskID string, profile *steerline.ExecutionProfile) (tea.Model, tea.Cmd) {
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
		valid, reason := candidateFreshness(state.ref, state.snapshot, m.clock.Now())
		if !valid {
			m.status = "候选已不可操作：" + reason
			return m, nil
		}
		if profile == nil {
			first := state.ref.Config.AvailableExecutionProfiles()[0]
			profile = &first
		}
		if _, _, err := state.ref.Config.ResolveExecutionProfile(profile.ID, profile.Digest()); err != nil {
			m.status = "执行配置不可用：" + err.Error()
			return m, nil
		}
	}
	requestID := steerline.NewControlRequestID()
	instance := runtimeState.Service.InstanceID
	revision := runtimeState.Control.Revision
	newAction := &actionState{RequestID: requestID, Action: action, TaskID: taskID, Status: "submitting", Message: "准备提交", TargetInstance: instance, UpdatedAt: m.clock.Now()}
	if profile != nil {
		newAction.ExecutionProfileID, newAction.ExecutionProfileDigest = profile.ID, profile.Digest()
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

func hasCapability(runtimeState *steerline.Runtime, action string) bool {
	for _, capability := range runtimeState.Service.Capabilities {
		if capability == action {
			return true
		}
	}
	return false
}

func candidateActionReason(ref ProjectRef, snapshot ProjectSnapshot, now time.Time) string {
	valid, reason := candidateFreshness(ref, snapshot, now)
	if !valid {
		return reason
	}
	if executionConfigMismatch(ref, snapshot) {
		return executionConfigChanged
	}
	if ref.Conflict != "" {
		return ref.Conflict
	}
	runtimeState := snapshot.Connection.Snapshot.Runtime
	if runtimeState == nil {
		return "缺少当前服务快照"
	}
	if runtimeState.Control.DispatchMode != "manual" {
		return "开始任务要求手动派发模式"
	}
	if runtimeState.Control.DispatchPaused {
		return "派发已暂停，请先恢复派发"
	}
	if runtimeState.SlotOccupied {
		return "执行槽被占用：" + stringValue(runtimeState.CurrentTask)
	}
	if !hasCapability(runtimeState, "start_task") {
		return "服务未声明 start_task 能力"
	}
	return ""
}

func (m Model) cycleAttempt(next bool) (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	row := m.selectedRow(m.selected)
	if row == nil || row.run == nil || state.snapshot.Domain == nil {
		m.status = "没有可切换的执行尝试"
		return m, nil
	}
	attempts := make([]steerline.StepAttemptRecord, 0)
	seenAttempts := map[string]bool{}
	maxAttemptNumber := 0
	for _, attempt := range state.snapshot.Domain.Attempts {
		if attempt.ExecutionID == row.run.ExecutionID && !seenAttempts[attempt.AttemptID] {
			attempts = append(attempts, attempt)
			seenAttempts[attempt.AttemptID] = true
			if attempt.AttemptNumber > maxAttemptNumber {
				maxAttemptNumber = attempt.AttemptNumber
			}
		}
	}
	if row.run.AttemptID != "" {
		found := false
		for _, attempt := range attempts {
			if attempt.AttemptID == row.run.AttemptID {
				found = true
			}
		}
		if !found {
			created := time.Time{}
			if row.run.StartedAt != nil {
				created = *row.run.StartedAt
			}
			attempts = append(attempts, steerline.StepAttemptRecord{ExecutionID: row.run.ExecutionID, AttemptID: row.run.AttemptID, AttemptNumber: maxAttemptNumber + 1, CreatedAt: created})
		}
	}
	sort.SliceStable(attempts, func(i, j int) bool {
		if attempts[i].CreatedAt.Equal(attempts[j].CreatedAt) {
			return attempts[i].AttemptNumber < attempts[j].AttemptNumber
		}
		if !attempts[i].CreatedAt.IsZero() && !attempts[j].CreatedAt.IsZero() {
			return attempts[i].CreatedAt.Before(attempts[j].CreatedAt)
		}
		return attempts[i].AttemptNumber < attempts[j].AttemptNumber
	})
	if len(attempts) < 2 {
		m.status = "此 execution 只有一个已记录 attempt"
		return m, nil
	}
	current := 0
	for i, attempt := range attempts {
		if attempt.AttemptID == state.attemptID {
			current = i
			break
		}
	}
	if next {
		current = (current + 1) % len(attempts)
	} else {
		current = (current + len(attempts) - 1) % len(attempts)
	}
	state.attemptID = attempts[current].AttemptID
	clearReadingAnchor(state)
	m.saveProjectView(m.selected)
	var cmd tea.Cmd
	m, cmd = m.refreshSelectedLogs(m.selected)
	return m, cmd
}

func (m Model) toggleGroup() (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	selected := -1
	for i, group := range view.Groups {
		if group.ID == state.view.AnchorGroup {
			selected = i
			break
		}
	}
	if selected < 0 && len(view.Groups) > 0 {
		selected = len(view.Groups) - 1
	}
	if selected >= 0 {
		group := view.Groups[selected]
		if !group.Precise {
			m.status = "turn 能力未验证；保留当前执行过程视图，不按猜测折叠"
			return m, nil
		}
		ToggleFold(state.view.Groups, group)
		state.view.AnchorGroup = group.ID
		if Folded(group, state.view.Groups[group.ID]) && state.bodyRef != nil && turnGroupIDForBody(state.conversationEvents, state.bodyEvent) == group.ID {
			state.panel = 2
			state.detailScroll = 0
			m.status = "当前阅读正文所在轮次已收起；已定位到轮次标题"
		}
		if state.panel == 2 && state.view.ReadGroup != "" && restoreSessionAnchor(state, m.selectedRow(m.selected), activityPanelWidth(m)) {
			m.status = "会话阅读锚点已收起；已定位到轮次标题"
		}
		m.saveProjectView(m.selected)
		return m, nil
	}
	m.status = "当前会话没有稳定轮次组可折叠"
	return m, nil
}

func turnGroupIDForBody(events []steerline.ConversationEvent, bodyKey string) string {
	for _, event := range events {
		if event.Body != nil && conversationItemKey(event) == bodyKey {
			return turnGroupID(event)
		}
	}
	return ""
}

func (m Model) cycleGroup(delta int) (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	if len(view.Groups) == 0 {
		m.status = "当前会话没有轮次组"
		return m, nil
	}
	index := -1
	for i, group := range view.Groups {
		if group.ID == state.view.AnchorGroup {
			index = i
			break
		}
	}
	if index < 0 {
		if delta < 0 {
			index = 0
		} else {
			index = len(view.Groups) - 1
		}
	}
	index = (index + delta) % len(view.Groups)
	if index < 0 {
		index += len(view.Groups)
	}
	state.view.AnchorGroup = view.Groups[index].ID
	m.saveProjectView(m.selected)
	return m, nil
}

func (m Model) keepGroupOpen() (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	for _, group := range view.Groups {
		if group.Precise && (state.view.AnchorGroup == "" || state.view.AnchorGroup == group.ID) {
			SetKeepExpanded(state.view.Groups, group, true)
			state.view.AnchorGroup = group.ID
			m.saveProjectView(m.selected)
			return m, nil
		}
	}
	m.status = "当前没有可保持展开的轮次"
	return m, nil
}

func (m Model) openProcessBody() (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	var events []steerline.ConversationEvent
	for _, group := range view.Groups {
		if Folded(group, state.view.Groups[group.ID]) {
			continue
		}
		for _, event := range group.Process {
			if event.Body != nil {
				events = append(events, event)
			}
		}
	}
	if len(events) == 0 {
		m.status = "没有可分页读取的展开过程正文"
		return m, nil
	}
	selected := 0
	for i, event := range events {
		if conversationItemKey(event) == state.bodyEvent {
			selected = (i + 1) % len(events)
			break
		}
	}
	event := events[selected]
	state.bodyGeneration++
	state.bodyBusy = false
	state.panel = 3
	state.bodyRef = event.Body
	state.bodyText = ""
	state.bodyOffset = 0
	state.bodyViewOffset = 0
	state.bodyPages = []int64{0}
	state.bodyError = ""
	state.bodyEvent = conversationItemKey(event)
	state.bodyKind = "process"
	state.view.AnchorItem = event.Body.ItemKey
	state.view.AnchorRevision = event.Body.Revision
	state.view.AnchorGroup = turnGroupID(event)
	state.view.BodyOffset = 0
	state.view.BodyKind = "process"
	m.saveProjectView(m.selected)
	return m, m.readBody(m.selected, event, 0)
}

func (m Model) nextFinal() (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	if len(view.Groups) == 0 {
		m.status = "没有最终回复"
		return m, nil
	}
	var finals []steerline.ConversationEvent
	for _, group := range view.Groups {
		finals = append(finals, group.Finals...)
	}
	if len(finals) == 0 {
		m.status = "本轮最终回复尚未同步或来源未识别"
		return m, nil
	}
	if m.finalIndex >= len(finals) {
		m.finalIndex = 0
	}
	event := finals[m.finalIndex]
	m.finalIndex = (m.finalIndex + 1) % len(finals)
	state.bodyGeneration++
	state.bodyBusy = false
	state.panel = 3
	state.bodyRef = event.Body
	state.bodyText = ""
	state.bodyOffset = 0
	state.bodyViewOffset = 0
	state.bodyPages = []int64{0}
	state.bodyError = ""
	state.bodyEvent = conversationItemKey(event)
	state.bodyKind = "final"
	if event.Body != nil {
		state.view.AnchorItem = event.Body.ItemKey
		state.view.AnchorRevision = event.Body.Revision
		state.view.AnchorGroup = turnGroupID(event)
		state.view.BodyOffset = 0
		state.view.BodyKind = "final"
	}
	m.saveProjectView(m.selected)
	if event.Body == nil {
		state.bodyError = "最终回复正文缺失"
		return m, nil
	}
	return m, m.readBody(m.selected, event, 0)
}

func (m Model) pageBody(next bool) (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	if state.bodyRef == nil {
		m.status = "先在会话页选择最终回复"
		return m, nil
	}
	if state.bodyBusy {
		m.status = "正文页仍在读取"
		return m, nil
	}
	pageIndex := -1
	for i, offset := range state.bodyPages {
		if offset == state.bodyViewOffset {
			pageIndex = i
			break
		}
	}
	if pageIndex < 0 {
		m.status = "正文分页锚点无效；请重新打开该消息"
		return m, nil
	}
	if next {
		nextOffset := int64(-1)
		if pageIndex+1 < len(state.bodyPages) {
			nextOffset = state.bodyPages[pageIndex+1]
		} else if state.bodyOffset < state.bodyRef.Bytes {
			nextOffset = state.bodyOffset
			state.bodyPages = appendBodyPage(state.bodyPages, nextOffset)
		}
		if nextOffset >= 0 {
			state.bodyViewOffset = nextOffset
			state.bodyText = ""
			state.detailScroll = 0
			m.saveProjectView(m.selected)
			return m, m.readBodyByRef(m.selected, *state.bodyRef, nextOffset, state.bodyEvent)
		}
		m.status = "已到最终回复末尾"
	} else if pageIndex > 0 {
		state.bodyViewOffset = state.bodyPages[pageIndex-1]
		state.bodyText = ""
		state.detailScroll = 0
		m.saveProjectView(m.selected)
		return m, m.readBodyByRef(m.selected, *state.bodyRef, state.bodyViewOffset, state.bodyEvent)
	}
	m.saveProjectView(m.selected)
	return m, nil
}

func (m Model) loadMoreConversation() (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	if state.conversationInFlight {
		m.status = "会话读取仍在进行；完成后再次按 y 加载"
		return m, nil
	}
	if !state.conversationMore || state.conversationCursor == "" {
		m.status = "没有更多会话事件"
		return m, nil
	}
	cursor := state.conversationCursor
	return m, m.readConversation(m.selected, cursor, state.conversationKey, "page")
}

func (m Model) scrollPanel(delta int) (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	if m.width < 100 && state.panel == 4 {
		return m.moveSelection(delta)
	}
	if state.panel == 1 {
		lines := allActivityLines(*state, m.selectedRow(m.selected), activityPanelWidth(m))
		height := activityPanelHeight(m)
		if state.view.Follow {
			state.logScroll = max(0, len(lines)-height)
			state.view.Follow = false
		}
		state.logScroll -= delta
		maxStart := max(0, len(lines)-height)
		if state.logScroll < 0 {
			state.logScroll = 0
		}
		if state.logScroll > maxStart {
			state.logScroll = maxStart
		}
		m.saveProjectView(m.selected)
		return m, nil
	}
	if state.panel == 2 {
		state.detailScroll += delta
		if state.detailScroll < 0 {
			state.detailScroll = 0
		}
		lines := allSessionLines(*state, m.selectedRow(m.selected), activityPanelWidth(m))
		maxStart := max(0, len(lines)-activityPanelHeight(m))
		if state.detailScroll > maxStart {
			state.detailScroll = maxStart
		}
		captureSessionAnchor(state, m.selectedRow(m.selected), activityPanelWidth(m))
		m.saveProjectView(m.selected)
		return m, nil
	}
	state.detailScroll += delta
	if state.detailScroll < 0 {
		state.detailScroll = 0
	}
	return m, nil
}

func (m Model) readBodyByRef(index int, body steerline.ConversationBodyRef, offset int64, eventKey string) tea.Cmd {
	state := &m.projects[index]
	if state.bodyBusy {
		return nil
	}
	state.bodyBusy = true
	state.bodyGeneration++
	generation := state.bodyGeneration
	ref, query, ctx := state.ref, m.query, m.ctx
	return func() tea.Msg {
		page, err := query.ReadBody(ctx, ref, body, offset, 64<<10)
		return bodyMsg{projectKey: ref.Key, identity: ref.Identity, eventKey: eventKey, generation: generation, body: body, offset: offset, page: page, err: err}
	}
}

func validBodyPage(ref steerline.ConversationBodyRef, requestedOffset int64, page steerline.ConversationBodyPage) bool {
	if page.ItemKey != ref.ItemKey || page.Revision != ref.Revision || page.Offset != requestedOffset || page.TotalBytes != ref.Bytes || page.SHA256 != ref.SHA256 || page.Completeness != ref.Completeness || !utf8.Valid(page.Data) {
		return false
	}
	if page.NextOffset != page.Offset+int64(len(page.Data)) || page.NextOffset > page.TotalBytes || page.NextOffset < page.Offset {
		return false
	}
	return page.Complete == (page.NextOffset >= page.TotalBytes)
}

func findBodyEvent(events []steerline.ConversationEvent, itemKey string, revision int64) (steerline.ConversationEvent, bool) {
	for _, event := range events {
		if event.Body != nil && event.Body.ItemKey == itemKey && event.Body.Revision == revision {
			return event, true
		}
	}
	return steerline.ConversationEvent{}, false
}

func (m Model) View() string { return render(m) }

func sameCursor(a, b steerline.JournalCursor) bool {
	return a.Path == b.Path && a.FileID == b.FileID && a.Generation == b.Generation && a.Offset == b.Offset && a.ScanOffset == b.ScanOffset && a.LastSeq == b.LastSeq && a.HaveSeq == b.HaveSeq && a.FallbackSeq == b.FallbackSeq && a.Dropping == b.Dropping && a.SkipUntilNewline == b.SkipUntilNewline && a.AnchorOffset == b.AnchorOffset && a.AnchorLength == b.AnchorLength && a.AnchorHash == b.AnchorHash && a.HeaderLength == b.HeaderLength && a.HeaderHash == b.HeaderHash && a.Initialized == b.Initialized && bytes.Equal(a.Pending, b.Pending)
}

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
	if ref.Legacy != nil {
		if snapshot.LegacyError != "" {
			return "旧任务查询失败"
		}
		if snapshot.Legacy == nil {
			return "旧任务读取中"
		}
		if snapshot.Legacy.ServiceLiveness == "live" {
			return "旧服务活动"
		}
		return "旧 Beads 只读"
	}
	if ref.Conflict != "" {
		return "关联冲突"
	}
	if snapshot.Connection.State == steerline.ConnectionConnected {
		return "已连接"
	}
	if snapshot.Connection.State == steerline.ConnectionOffline || snapshot.Connection.State == steerline.ConnectionNoRecord {
		return "离线"
	}
	if snapshot.Connection.State == "" {
		return "读取中"
	}
	return string(snapshot.Connection.State)
}
