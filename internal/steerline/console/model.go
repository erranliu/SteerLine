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
)

type consoleRow struct {
	key       string
	kind      rowKind
	taskID    string
	title     string
	run       *steerline.Run
	candidate *steerline.TaskCandidate
	native    *steerline.NativeTaskDetail
}

type actionState struct {
	RequestID      string
	Action         string
	TaskID         string
	Status         string
	Message        string
	RunID          string
	TargetInstance string
	InFlight       bool
	Resolved       bool
	UpdatedAt      time.Time
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
}

type confirmAction struct {
	projectKey string
	instance   string
	revision   uint64
	action     string
	taskID     string
	requestID  string
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

	confirm    *confirmAction
	status     string
	finalIndex int

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
		ref := ProjectRef{Name: name, ConfigPath: absolute}
		if loadErr != nil {
			ref.Error = loadErr.Error()
			ref.Key, _ = absoluteViewKey(absolute, "")
			return []ProjectRef{ref}, nil
		}
		refs = append(refs, makeProjectRef(name, absolute, config))
	} else {
		projects, err := steerline.ListProjects()
		if err != nil {
			return nil, err
		}
		for _, project := range projects {
			path, _ := filepath.Abs(project.ConfigPath)
			ref := ProjectRef{Name: project.Name, ConfigPath: path}
			if project.Err != nil {
				ref.Error = project.Err.Error()
				ref.Key, _ = absoluteViewKey(path, "")
				refs = append(refs, ref)
				continue
			}
			config, loadErr := steerline.LoadConfig(path)
			if loadErr != nil {
				ref.Error = loadErr.Error()
				ref.Key, _ = absoluteViewKey(path, "")
				refs = append(refs, ref)
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
			state.actions[requestID] = &actionState{RequestID: requestID, Action: saved.Action, TaskID: saved.TaskID, Status: saved.Status, Message: message, RunID: saved.RunID, TargetInstance: saved.TargetInstance, Resolved: !outstanding, UpdatedAt: saved.UpdatedAt}
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
		payload = map[string]string{"task_id": taskID}
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
	err                                                       error
}
type actionReceiptMsg struct {
	projectKey, identity, requestID string
	receipt                         *steerline.ControlReceipt
	request                         *steerline.ControlRequest
	err                             error
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
		} else if len(rows) > 0 && !rowExists(rows, state.selectedKey) {
			m.status = "当前选择不在最新快照中；未切换到其他任务，请刷新或重新选择"
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
		TaskID string `json:"task_id"`
		Mode   string `json:"mode"`
	}
	if len(request.Payload) > 0 && json.Unmarshal(request.Payload, &payload) != nil {
		return false
	}
	switch action.Action {
	case "start_task":
		return payload.TaskID == action.TaskID && payload.Mode == ""
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
			rows = append(rows, consoleRow{key: "task:" + detail.TaskID, kind: rowNative, taskID: detail.TaskID, title: detail.Title, native: detail})
			seenTasks[detail.TaskID] = true
		}
	}
	if state.filter == 0 && state.snapshot.SelectedNative != nil && state.selectedKey == "task:"+state.snapshot.SelectedNative.TaskID && !seenTasks[state.snapshot.SelectedNative.TaskID] {
		detail := state.snapshot.SelectedNative
		rows = append(rows, consoleRow{key: "task:" + detail.TaskID, kind: rowNative, taskID: detail.TaskID, title: detail.Title, native: detail})
	}
	if runtime := state.snapshot.Observation.Runtime; runtime != nil {
		for _, run := range runtime.Runs {
			if run == nil || !filterAllows(state.filter, rowRun, run) {
				continue
			}
			row := consoleRow{key: "run:" + run.RunID, kind: rowRun, taskID: run.TaskID, title: run.Title, run: run}
			rows = append(rows, row)
		}
	}
	return rows
}

func filterAllows(filter int, kind rowKind, run *steerline.Run) bool {
	if filter == 0 {
		return true
	}
	if kind == rowReady {
		return filter == 1
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
		out[action.RequestID] = SavedAction{Action: action.Action, TaskID: action.TaskID, Status: action.Status, RunID: action.RunID, TargetInstance: action.TargetInstance, UpdatedAt: action.UpdatedAt}
	}
	for _, action := range terminal {
		if len(out) >= limit {
			break
		}
		out[action.RequestID] = SavedAction{Action: action.Action, TaskID: action.TaskID, Status: action.Status, RunID: action.RunID, TargetInstance: action.TargetInstance, UpdatedAt: action.UpdatedAt}
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

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirm != nil {
		return m.handleConfirm(key)
	}
	if m.selected < 0 || m.selected >= len(m.projects) {
		if key.String() == "q" || key.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	state := &m.projects[m.selected]
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
		m.status = "帮助：↑↓ 选择 · ←→ 项目 · Enter 详情 · s 开始 · m 模式 · p 派发 · x 停止 · e 折叠 · = 保持展开 · y 更多 · / 筛选 · q 退出"
		return m, nil
	}
	return m, nil
}

func (m Model) handleConfirm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	confirm := m.confirm
	switch key.String() {
	case "esc", "n":
		m.confirm = nil
		return m, nil
	case "enter", "y":
		m.confirm = nil
		return m.beginAction(confirm.projectKey, confirm.instance, confirm.revision, confirm.action, confirm.taskID, confirm.requestID)
	default:
		return m, nil
	}
}

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
	valid, reason := candidateFreshness(state.ref, state.snapshot, m.clock.Now())
	if !valid {
		m.status = "不能开始：" + reason
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
	return m.createAction("start_task", row.taskID)
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
	}
	requestID := steerline.NewControlRequestID()
	instance := runtimeState.Service.InstanceID
	revision := runtimeState.Control.Revision
	state.actions[requestID] = &actionState{RequestID: requestID, Action: action, TaskID: taskID, Status: "submitting", Message: "准备提交", TargetInstance: instance, UpdatedAt: m.clock.Now()}
	state.view.Outstanding[requestID] = "task:" + taskID
	if err := m.saveProjectView(m.selected); err != nil {
		delete(state.actions, requestID)
		delete(state.view.Outstanding, requestID)
		m.status = "未提交：无法持久化 request_id；请检查 UI 视图目录权限后重试"
		return m, nil
	}
	m.status = fmt.Sprintf("提交中 · task %s · request %s", displayDash(taskID), requestID)
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
