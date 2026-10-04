package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time                       { return c.now }
func (c *testClock) After(time.Duration) <-chan time.Time { return make(chan time.Time) }

type testQuerySource struct {
	snapshot ProjectSnapshot
	activity LogBatch
	page     contracts.ConversationPage
	body     contracts.ConversationBodyPage
	selected string
}

type testControlClient struct {
	calls           int
	receipts        int
	requests        int
	request         api.ControlRequest
	err             error
	instance        string
	revision        *uint64
	decisionCalls   int
	decisionAnswer  contracts.DecisionAnswer
	decisionCommand contracts.CommandRecord
	decisionErr     error
	eligibilityErr  error
}

func testProject(key string) ProjectRef {
	return ProjectRef{Key: key, Name: key, ConfigPath: "/tmp/" + key + ".json", RunDir: "/tmp/" + key + "-state", Identity: "/repo/" + key, Presentation: fixturePresentation()}
}

func readySnapshot(ref ProjectRef, now time.Time) ProjectSnapshot {
	last := now.Add(-time.Second)
	runtime := &contracts.Runtime{
		SchemaVersion: contracts.WorkerFlowRuntimeSchema,
		Repository:    contracts.Repository{Identity: ref.Identity},
		Service:       contracts.ServiceState{ConfigDigest: ref.Presentation.ConfigDigest, InstanceID: "instance-" + ref.Name, Capabilities: []string{"start_task", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service"}},
		Control:       contracts.ControlState{DispatchMode: "manual", Revision: 7},
		Runs:          []*contracts.Run{},
	}
	obs := api.ObservationSnapshot{Runtime: runtime, State: api.SnapshotValid, SampledAt: now}
	return ProjectSnapshot{
		Connection:      api.ServiceConnection{State: api.ConnectionConnected, CanControl: true, CanStop: true, IdentityMatched: true, Snapshot: obs},
		Observation:     obs,
		DecisionReasons: map[string]string{},
		Tasks:           &contracts.TaskSnapshot{Version: 1, InstanceID: runtime.Service.InstanceID, RepositoryIdentity: ref.Identity, QueriedAt: now, LastSuccessAt: &last, Tasks: []contracts.TaskCandidate{{TaskID: "task-B", Title: "Build B"}}},
	}
}

func modelForTest(refs []ProjectRef, snapshots []ProjectSnapshot, control *testControlClient, now time.Time) Model {
	clock := &testClock{now: now}
	m := Model{ctx: context.Background(), clock: clock, selected: 0, width: 120, height: 40, profile: nil, prefs: emptyViewFile()}
	for i, ref := range refs {
		m.projects = append(m.projects, projectState{client: &testClient{query: &testQuerySource{}, control: control, ref: ref}, ref: ref, snapshot: snapshots[i], snapshotLoaded: true, selectedKey: "task:task-B", view: ProjectView{Follow: true, Groups: map[string]FoldChoice{}, Outstanding: map[string]string{}, Actions: map[string]SavedAction{}}, actions: map[string]*actionState{}})
	}
	return m
}

func decisionSnapshot(ref ProjectRef, now time.Time) (ProjectSnapshot, *contracts.Run) {
	snapshot := readySnapshot(ref, now)
	runtime := snapshot.Connection.Snapshot.Runtime
	runtime.Service.Capabilities = append(runtime.Service.Capabilities, "answer_decision")
	options := make([]executionapi.DecisionOption, 12)
	for i := range options {
		options[i] = executionapi.DecisionOption{ID: fmt.Sprintf("o%d", i), Label: fmt.Sprintf("选项 %d", i+1), Detail: fmt.Sprintf("详细说明 %02d：完整内容", i+1)}
	}
	run := &contracts.Run{TaskID: "task-decision", RunID: "0123456789abcdef0123456789abcdef", AttemptID: "fedcba9876543210fedcba9876543210", ControlRevision: 9, WorkflowVersion: fixtureWorkflowVersion, ExecutionID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Host: "local", Agent: "codex", Backend: "local", Protocol: "local_process", FinishTaskStatus: "waiting_decision", ExecutionStatus: "succeeded", Decision: &contracts.DecisionState{ID: "decision-0123456789abcdef0123456789ab", AttemptID: "fedcba9876543210fedcba9876543210", Version: 3, Status: "pending", Request: contracts.DecisionRequest{Question: strings.Repeat("问题", 24), Context: "背景第一行\n背景第二行", Options: options, Recommended: "o10", AllowFreeText: true}}}
	runtime.Runs = []*contracts.Run{run}
	snapshot.DecisionReasons[run.RunID] = "提交后核对原会话接收与工作轮次开始"
	snapshot.Connection.Snapshot.Runtime = runtime
	snapshot.Observation.Runtime = runtime
	return snapshot, run
}

func TestDecisionAnswerEditorHandlesTwelveOptionsUnicodePasteAndStableCommandID(t *testing.T) {
	now := time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC)
	ref := testProject("decisions")
	snapshot, run := decisionSnapshot(ref, now)
	control := &testControlClient{}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, control, now)
	m.profile = &viewProfile{dir: t.TempDir()}
	m.projects[0].selectedKey = "task:" + run.TaskID
	updated, cmd := m.answerSelectedDecision()
	m = updated.(Model)
	if cmd != nil || m.answer == nil || m.answer.optionIndex != 10 {
		t.Fatalf("recommended option was not only preselected: dialog=%+v cmd=%v", m.answer, cmd)
	}
	all := renderAnswerDialog(m, m.projects[0], 200)
	for _, want := range []string{"详细说明 12：完整内容", "背景第二行", run.Decision.ID} {
		if !strings.Contains(all, want) {
			t.Fatalf("full decision evidence missing %q", want)
		}
	}
	for i := 0; i < 13; i++ {
		updated, _ = m.handleDecisionAnswerKey(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	if m.answer.optionIndex != 10 {
		t.Fatalf("arrow navigation did not cycle all twelve declared options and custom answer: %d", m.answer.optionIndex)
	}
	updated, _ = m.handleDecisionAnswerKey(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.handleDecisionAnswerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("漢字\n第二行")})
	m = updated.(Model)
	updated, _ = m.handleDecisionAnswerKey(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(Model)
	updated, _ = m.handleDecisionAnswerKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(Model)
	if got := string(m.answer.note); got != "漢字\n第行" {
		t.Fatalf("Unicode multi-line editing lost the cursor or pasted text: %q", got)
	}
	updated, _ = m.handleDecisionAnswerKey(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	updated, cmd = m.handleDecisionAnswerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("numeric option shortcut submitted without explicit confirmation")
	}
	updated, cmd = m.handleDecisionAnswerKey(tea.KeyMsg{Type: tea.KeyF2})
	m = updated.(Model)
	if cmd == nil || m.answer != nil || control.decisionCalls != 0 {
		t.Fatalf("submission did not first persist and defer the command: cmd=%v dialog=%+v calls=%d", cmd, m.answer, control.decisionCalls)
	}
	var requestID string
	for id, action := range m.projects[0].actions {
		if action.Action == "answer_decision" {
			requestID = id
			break
		}
	}
	if requestID == "" || m.projects[0].view.Outstanding[requestID] == "" {
		t.Fatal("stable answer command ID was not retained before publishing")
	}
	persisted, err := readViewFile(m.profile.path())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := persisted.Projects[ref.Key].Actions[requestID]; !ok {
		t.Fatal("answer command ID was not persisted before publication")
	}
	viewBytes, readErr := os.ReadFile(m.profile.path())
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(viewBytes), "漢字") {
		t.Fatal("UI view preferences lost the isolated answer draft")
	}
	message := cmd()
	updated, _ = m.Update(message)
	m = updated.(Model)
	if control.decisionCalls != 1 || control.decisionAnswer.OptionID != "o0" || control.decisionAnswer.Note != "漢字\n第行" || control.decisionAnswer.ExpectedRunRevision != run.ControlRevision || control.decisionAnswer.ExpectedDecisionVersion != run.Decision.Version {
		t.Fatalf("submitted answer did not bind the selected option, Unicode note and displayed identity: %+v", control.decisionAnswer)
	}
	if m.projects[0].actions[requestID].Status != "pending" {
		t.Fatalf("pending command was not tracked by its stable ID: %+v", m.projects[0].actions[requestID])
	}
}

func TestDecisionAnswerDraftSurvivesStaleRevisionAndLengthRejection(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("decision-conflict")
	snapshot, run := decisionSnapshot(ref, now)
	control := &testControlClient{}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, control, now)
	m.projects[0].selectedKey = "task:" + run.TaskID
	updated, _ := m.answerSelectedDecision()
	m = updated.(Model)
	_, _ = m.handleDecisionAnswerKey(tea.KeyMsg{Type: tea.KeyTab})
	m.answer.note = []rune("preserve this answer")
	run.ControlRevision++
	updated, cmd := m.submitDecisionAnswer()
	m = updated.(Model)
	if cmd != nil || m.answer == nil || string(m.answer.note) != "preserve this answer" || !strings.Contains(m.answer.error, "stale_revision") || control.decisionCalls != 0 {
		t.Fatalf("stale decision lost or submitted the draft: dialog=%+v cmd=%v calls=%d", m.answer, cmd, control.decisionCalls)
	}
	run.ControlRevision = m.answer.runRevision
	m.answer.note = []rune(strings.Repeat("界", 4001))
	updated, cmd = m.submitDecisionAnswer()
	m = updated.(Model)
	if cmd != nil || m.answer == nil || len(m.answer.note) != 4001 || !strings.Contains(m.answer.error, "未截断") || control.decisionCalls != 0 {
		t.Fatalf("oversized note was clipped or submitted: len=%d error=%q calls=%d", len(m.answer.note), m.answer.error, control.decisionCalls)
	}
}

func TestConsoleOffersWorkerFlowDecisionWithoutRecoveryActions(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("worker-flow-decision")
	ref.Presentation.WorkflowProtocol = fixtureWorkflowVersion
	snapshot, run := decisionSnapshot(ref, now)
	run.WorkflowVersion, run.Backend, run.Protocol, run.Host = fixtureWorkflowVersion, "local", "local_process", "local"
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.projects[0].selectedKey = "task:" + run.TaskID
	updated, cmd := m.answerSelectedDecision()
	m = updated.(Model)
	if cmd != nil || m.answer == nil {
		t.Fatalf("worker-flow decision was hidden: answer=%+v status=%q", m.answer, m.status)
	}
}

func runeKey(value rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{value}} }

func TestEnterOnlyOpensDetailAndStartKeepsOneRequestID(t *testing.T) {
	now := time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC)
	ref := testProject("alpha")
	control := &testControlClient{}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, control, now)
	profileDir := t.TempDir()
	m.profile = &viewProfile{dir: profileDir}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if control.calls != 0 || len(m.projects[0].actions) != 0 {
		t.Fatal("Enter must only open details")
	}
	updated, cmd := m.Update(runeKey('s'))
	m = updated.(Model)
	if cmd != nil || m.profileChoice == nil || len(m.projects[0].actions) != 0 {
		t.Fatal("start key should open the configured execution selection without submitting")
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil || len(m.projects[0].actions) != 1 {
		t.Fatal("confirmed profile should allocate and persist one request before publishing")
	}
	var originalID string
	for id, action := range m.projects[0].actions {
		originalID = id
		if action.Status != "submitting" || action.TaskID != "task-B" {
			t.Fatalf("bad pending action: %+v", action)
		}
	}
	persisted, err := readViewFile(m.profile.path())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := persisted.Projects[ref.Key].Actions[originalID]; !ok {
		t.Fatal("request ID was not durable before the submit command ran")
	}
	updated, second := m.Update(runeKey('s'))
	m = updated.(Model)
	if second != nil || len(m.projects[0].actions) != 1 {
		t.Fatal("repeated start key must not allocate another request")
	}
	result := cmd()
	updated, _ = m.Update(result)
	m = updated.(Model)
	if control.calls != 1 || control.request.RequestID != originalID || control.instance != "instance-alpha" || control.revision == nil || *control.revision != 7 {
		t.Fatalf("control request retargeted or changed: %+v", control)
	}
	if control.request.Action != "start_task" || !strings.Contains(string(control.request.Payload), `"task_id":"task-B"`) {
		t.Fatalf("wrong start payload: %+v", control.request)
	}
	if m.projects[0].actions[originalID].Status != "submitted" {
		t.Fatalf("expected submitted state, got %+v", m.projects[0].actions[originalID])
	}
}

func TestStartIsNotPublishedWhenRequestIdentityCannotBePersisted(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	control := &testControlClient{}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, control, now)
	m.profile = &viewProfile{dir: filepath.Join(t.TempDir(), "missing-profile")}
	updated, cmd := m.startSelected()
	m = updated.(Model)
	if cmd != nil || m.profileChoice == nil {
		t.Fatal("expected profile selection before start")
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil || control.calls != 0 || len(m.projects[0].actions) != 0 || !strings.Contains(m.status, "未提交") {
		t.Fatalf("request was published without durable request identity: status=%q calls=%d", m.status, control.calls)
	}
}

func TestStartDisablesStaleCandidatesAndAllowsManualParallel(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	control := &testControlClient{}
	snapshot := readySnapshot(ref, now)
	stale := now.Add(-time.Minute)
	snapshot.Tasks.LastSuccessAt = &stale
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, control, now)
	updated, cmd := m.startSelected()
	m = updated.(Model)
	if cmd != nil || control.calls != 0 || !strings.Contains(m.status, "缓存") {
		t.Fatalf("stale candidates must be blocked: %q", m.status)
	}
	snapshot = readySnapshot(ref, now)
	snapshot.Observation.Runtime.SlotOccupied = true
	snapshot.Connection.Snapshot.Runtime.SlotOccupied = true
	m = modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, control, now)
	updated, cmd = m.startSelected()
	m = updated.(Model)
	if cmd != nil || control.calls != 0 || m.profileChoice == nil {
		t.Fatalf("manual start must allow another task while a worker runs: status=%q choice=%+v", m.status, m.profileChoice)
	}
}

func TestCandidateCacheMustMatchCurrentConnectionInstance(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	snapshot := readySnapshot(ref, now)
	// The read-only projection and task cache still describe the old instance,
	// while ConnectService has already identified a new live service.
	snapshot.Observation.Runtime.Service.InstanceID = "old-instance"
	snapshot.Tasks.InstanceID = "old-instance"
	current := *snapshot.Connection.Snapshot.Runtime
	current.Service.InstanceID = "new-instance"
	snapshot.Connection.Snapshot.Runtime = &current
	if valid, reason := api.CandidateFreshness(ref.Identity, snapshot, now); valid || !strings.Contains(reason, "其他服务实例") {
		t.Fatalf("old candidate cache became actionable against a new service: valid=%v reason=%q", valid, reason)
	}
}

func TestUnavailableControlActionKeepsItsReasonInReturnedModel(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	snapshot := readySnapshot(ref, now)
	snapshot.Connection = api.ServiceConnection{State: api.ConnectionOffline, Diagnostic: "service window is closed"}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	updated, cmd := m.toggleMode()
	m = updated.(Model)
	if cmd != nil || !strings.Contains(m.status, "service window is closed") {
		t.Fatalf("unavailable action lost its visible reason: %q", m.status)
	}
}

func TestDefinitiveStaleSubmissionDoesNotRemainOutstanding(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, &testControlClient{}, now)
	m.profile = &viewProfile{dir: t.TempDir()}
	requestID := "0123456789abcdef0123456789abcdef"
	action := &actionState{RequestID: requestID, Action: "start_task", TaskID: "task-B", Status: "submitting", TargetInstance: "instance-alpha", UpdatedAt: now}
	m.projects[0].actions[requestID] = action
	m.projects[0].view.Outstanding[requestID] = "task:task-B"
	updated, _ := m.Update(actionSubmittedMsg{projectKey: ref.Key, identity: ref.Identity, instance: action.TargetInstance, requestID: requestID, action: action.Action, taskID: action.TaskID, err: errors.New("stale_revision: control state changed")})
	m = updated.(Model)
	if !m.projects[0].actions[requestID].Resolved || m.projects[0].actions[requestID].Status != "rejected" {
		t.Fatalf("definitive pre-publication rejection remained unresolved: %+v", m.projects[0].actions[requestID])
	}
	persisted, err := readViewFile(m.profile.path())
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := persisted.Projects[ref.Key].Outstanding[requestID]; exists {
		t.Fatal("definitively rejected request was persisted as outstanding")
	}
}

func TestConfirmedStopCannotCreateASecondOutstandingRequest(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, &testControlClient{}, now)
	m.profile = &viewProfile{dir: t.TempDir()}
	updated, first := m.createActionWithID(0, "stop_service", "", "0123456789abcdef0123456789abcdef", "instance-alpha", 7)
	m = updated.(Model)
	updated, second := m.createActionWithID(0, "stop_service", "", "fedcba9876543210fedcba9876543210", "instance-alpha", 7)
	m = updated.(Model)
	if first == nil || second != nil || len(m.projects[0].actions) != 1 || !strings.Contains(m.status, "已有请求") {
		t.Fatalf("duplicate stop request was not blocked: actions=%d status=%q", len(m.projects[0].actions), m.status)
	}
}

func TestProjectResponseCannotUpdateNewProjectView(t *testing.T) {
	now := time.Now().UTC()
	a, b := testProject("a"), testProject("b")
	control := &testControlClient{}
	m := modelForTest([]ProjectRef{a, b}, []ProjectSnapshot{readySnapshot(a, now), readySnapshot(b, now)}, control, now)
	previousSample := now.Add(-time.Hour)
	m.projects[0].snapshot.Observation.SampledAt = previousSample
	m.projects[0].refreshID = 1
	old := projectMsg{key: a.Key, identity: a.Identity, generation: m.generation, requestID: 1, snapshot: ProjectSnapshot{Observation: api.ObservationSnapshot{State: api.SnapshotReadFailed, SampledAt: now}}}
	updated, _ := m.moveProject(1)
	m = updated.(Model)
	updated, _ = m.Update(old)
	m = updated.(Model)
	if m.selected != 1 || !m.projects[1].snapshotLoaded || !m.projects[0].snapshot.Observation.SampledAt.Equal(previousSample) || m.projects[0].snapshot.Observation.Runtime == nil {
		t.Fatal("late project snapshot crossed the active project identity")
	}
}

func TestRefreshTicksDoNotStackSlowProjectReads(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	control := &testControlClient{}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, control, now)
	updated, _ := m.Update(tickMsg(now))
	m = updated.(Model)
	if !m.projects[0].refreshing || m.projects[0].refreshID != 1 {
		t.Fatal("first foreground tick should start one project read")
	}
	m.clock.(*testClock).now = now.Add(3 * time.Second)
	updated, _ = m.Update(tickMsg(m.clock.Now()))
	m = updated.(Model)
	if m.projects[0].refreshID != 1 {
		t.Fatalf("slow project read stacked another request: %d", m.projects[0].refreshID)
	}
	updated, _ = m.Update(projectMsg{key: ref.Key, identity: ref.Identity, generation: m.generation, requestID: 1, snapshot: readySnapshot(ref, m.clock.Now())})
	m = updated.(Model)
	m.clock.(*testClock).now = now.Add(3*time.Second + 300*time.Millisecond)
	updated, _ = m.Update(tickMsg(m.clock.Now()))
	m = updated.(Model)
	if m.projects[0].refreshID != 1 {
		t.Fatal("foreground reader restarted before its refresh interval")
	}
	m.clock.(*testClock).now = now.Add(4*time.Second + 50*time.Millisecond)
	updated, _ = m.Update(tickMsg(m.clock.Now()))
	m = updated.(Model)
	if m.projects[0].refreshID != 2 || !m.projects[0].refreshing {
		t.Fatal("foreground reader did not restart one second after its last completed refresh")
	}
}

func TestFailedRefreshKeepsLastGoodReadOnlyDataAndCurrentError(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	previous := readySnapshot(ref, now.Add(-time.Minute))
	domain := &contracts.DomainSnapshot{ProjectIdentity: ref.Identity, Executions: []contracts.ExecutionRecord{{ExecutionID: "execution-old"}}}
	previous.Domain = domain
	next := ProjectSnapshot{Connection: api.ServiceConnection{State: api.ConnectionCorrupt, Diagnostic: "runtime schema is unsupported"}, Observation: api.ObservationSnapshot{State: api.SnapshotUnknownVersion, SampledAt: now, Error: "runtime schema is unsupported"}, TaskError: "candidate cache belongs to another instance", DomainError: "database read failed"}
	got := preserveLastGood(previous, next)
	if got.Observation.Runtime != previous.Observation.Runtime || !got.Observation.SampledAt.Equal(previous.Observation.SampledAt) || got.Tasks != previous.Tasks || got.Domain != domain {
		t.Fatal("failed refresh erased the last successful read-only snapshot")
	}
	if got.Connection.State != api.ConnectionCorrupt || got.TaskError == "" || got.DomainError == "" {
		t.Fatalf("current read errors or control state were hidden: %+v", got)
	}
	if valid, _ := api.CandidateFreshness(ref.Identity, got, now); valid {
		t.Fatal("cached candidates remained actionable after a query error")
	}
}

func TestFoldReducerUsesExplicitTurnAndPreservesManualIntent(t *testing.T) {
	turn := contracts.ConversationEvent{Version: contracts.ConversationVersion, ProjectID: "p", ExecutionID: "e", AttemptID: "a", Source: "codex-app-server", SourceSegment: "segment", SessionID: "s", SessionQuality: contracts.IdentityNative, TurnID: "turn-1", TurnQuality: contracts.IdentityNative, Type: contracts.ConversationTurnStarted, Phase: contracts.MessagePhaseUnknown, FinalAvailability: contracts.FinalPending}
	commentary := turn
	commentary.EventID = 2
	commentary.Type = contracts.ConversationMessageDelta
	commentary.ItemID = "same"
	commentary.ItemQuality = contracts.IdentityNative
	commentary.Phase = contracts.MessagePhaseCommentary
	completedMessage := commentary
	completedMessage.EventID = 3
	completedMessage.Type = contracts.ConversationMessageComplete
	ended := turn
	ended.EventID = 4
	ended.Type = contracts.ConversationTurnFailed
	ended.Status = "failed"
	ended.Diagnostic = "worker exit 23"
	choices := map[string]FoldChoice{}
	view, changed := BuildConversationView([]contracts.ConversationEvent{turn, commentary, completedMessage, ended}, choices)
	if !changed || len(view.Groups) != 1 {
		t.Fatalf("expected one explicit turn group: %+v", view)
	}
	group := view.Groups[0]
	if Folded(group, choices[group.ID]) == false || choices[group.ID].AutoProcessed == false {
		t.Fatal("failed turn should auto fold once")
	}
	if len(group.Process) != 2 || group.Process[1].Type != contracts.ConversationMessageComplete || len(group.Finals) != 0 {
		t.Fatalf("message completion must not become final or end boundary: %+v", group)
	}
	open := false
	choices[group.ID] = FoldChoice{AutoProcessed: true, Manual: &open}
	lateFinal := turn
	lateFinal.EventID = 5
	lateFinal.Type = contracts.ConversationMessageComplete
	lateFinal.ItemID = "same"
	lateFinal.Phase = contracts.MessagePhaseFinalAnswer
	lateFinal.Body = &contracts.ConversationBodyRef{ExecutionID: "e", AttemptID: "a", ItemKey: "final-1", Revision: 1, Bytes: 30, Completeness: contracts.BodyComplete}
	view, changed = BuildConversationView([]contracts.ConversationEvent{turn, commentary, completedMessage, ended, lateFinal}, choices)
	if changed || len(view.Groups[0].Finals) != 1 || Folded(view.Groups[0], choices[group.ID]) {
		t.Fatal("late final must retain its turn and manual expansion")
	}
	if view.Groups[0].FinalState != contracts.FinalAvailable {
		t.Fatalf("final availability was lost: %s", view.Groups[0].FinalState)
	}
}

func TestLateFinalReclassifiesSameItemAndOlderRevisionCannotOverwrite(t *testing.T) {
	base := contracts.ConversationEvent{Version: contracts.ConversationVersion, ProjectID: "p", ExecutionID: "e", AttemptID: "a", Source: "codex-app-server", SourceSegment: "segment", SessionID: "s", SessionQuality: contracts.IdentityNative, TurnID: "turn", TurnQuality: contracts.IdentityNative, ItemID: "item", ItemKey: "stable-item", ItemQuality: contracts.IdentityNative, SourceEventQuality: contracts.IdentityNative, Type: contracts.ConversationMessageSnapshot, Phase: contracts.MessagePhaseUnknown}
	process := base
	process.EventID = 2
	process.SourceRevision = 2
	process.Body = &contracts.ConversationBodyRef{ExecutionID: "e", AttemptID: "a", ItemKey: "stable-item", Revision: 2, Bytes: 12, Completeness: contracts.BodyPartial}
	older := base
	older.EventID = 3
	older.SourceRevision = 1
	older.Body = &contracts.ConversationBodyRef{ExecutionID: "e", AttemptID: "a", ItemKey: "stable-item", Revision: 1, Bytes: 5, Completeness: contracts.BodyPartial}
	final := base
	final.EventID = 4
	final.SourceRevision = 3
	final.Type = contracts.ConversationMessageComplete
	final.Phase = contracts.MessagePhaseFinalAnswer
	final.Body = &contracts.ConversationBodyRef{ExecutionID: "e", AttemptID: "a", ItemKey: "stable-item", Revision: 3, Bytes: 20, Completeness: contracts.BodyComplete}
	view, _ := BuildConversationView([]contracts.ConversationEvent{process, older, final}, map[string]FoldChoice{})
	if len(view.Groups) != 1 || len(view.Groups[0].Process) != 0 || len(view.Groups[0].Finals) != 1 {
		t.Fatalf("late final did not reclassify the stable item: %+v", view.Groups)
	}
	if view.Groups[0].Finals[0].Body.Revision != 3 {
		t.Fatalf("older revision overwrote final body: %+v", view.Groups[0].Finals[0])
	}
	processOnly, _ := BuildConversationView([]contracts.ConversationEvent{process, older}, map[string]FoldChoice{})
	if len(processOnly.Groups[0].Process) != 1 || processOnly.Groups[0].Process[0].Body.Revision != 2 {
		t.Fatalf("late older process revision overwrote newer state: %+v", processOnly.Groups[0].Process)
	}
	newerProcess := process
	newerProcess.EventID = 5
	newerProcess.SourceRevision = 4
	newerProcess.Body = &contracts.ConversationBodyRef{ExecutionID: "e", AttemptID: "a", ItemKey: "stable-item", Revision: 4, Bytes: 24, Completeness: contracts.BodyPartial}
	staleFinal, _ := BuildConversationView([]contracts.ConversationEvent{newerProcess, final}, map[string]FoldChoice{})
	if len(staleFinal.Groups[0].Process) != 1 || len(staleFinal.Groups[0].Finals) != 0 || staleFinal.Groups[0].Process[0].Body.Revision != 4 || staleFinal.Groups[0].FinalState == contracts.FinalAvailable {
		t.Fatalf("older final reclassified a newer process revision: %+v", staleFinal.Groups[0])
	}
}

func TestFinalPhaseDoesNotPromoteNonMessageEvents(t *testing.T) {
	event := contracts.ConversationEvent{Version: contracts.ConversationVersion, ProjectID: "p", ExecutionID: "e", AttemptID: "a", Source: "source", SourceSegment: "segment", SessionID: "session", SessionQuality: contracts.IdentityNative, TurnID: "turn", TurnQuality: contracts.IdentityNative, ItemID: "tool", ItemKey: "tool-key", ItemQuality: contracts.IdentityNative, SourceEventQuality: contracts.IdentityNative, Type: contracts.ConversationToolCompleted, Phase: contracts.MessagePhaseFinalAnswer}
	view, _ := BuildConversationView([]contracts.ConversationEvent{event}, map[string]FoldChoice{})
	if len(view.Groups) != 1 || len(view.Groups[0].Finals) != 0 || len(view.Groups[0].Process) != 1 {
		t.Fatalf("non-message event was presented as a final reply: %+v", view.Groups)
	}
}

func TestSessionScrollAnchorMovesToGroupHeaderWhenAutoFoldHidesItem(t *testing.T) {
	base := contracts.ConversationEvent{Version: contracts.ConversationVersion, ProjectID: "p", TaskID: "task", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Source: "codex-app-server", SourceSegment: "segment", SessionID: "session", SessionQuality: contracts.IdentityNative, TurnID: "turn", TurnQuality: contracts.IdentityNative, Phase: contracts.MessagePhaseUnknown}
	started := base
	started.Type = contracts.ConversationTurnStarted
	message := base
	message.EventID = 2
	message.Type = contracts.ConversationMessageComplete
	message.ItemID = "item"
	message.ItemKey = "item-key"
	message.ItemQuality = contracts.IdentityNative
	message.Phase = contracts.MessagePhaseCommentary
	message.Body = &contracts.ConversationBodyRef{ExecutionID: "execution", AttemptID: "attempt", ItemKey: "body-key", Revision: 1, Bytes: 120, Completeness: contracts.BodyComplete}
	completed := base
	completed.EventID = 3
	completed.Type = contracts.ConversationTurnCompleted
	completed.Status = "completed"
	state := projectState{view: ProjectView{Groups: map[string]FoldChoice{}}, conversationEvents: []contracts.ConversationEvent{started, message}, detailScroll: 0}
	run := &contracts.Run{RunID: "workflow", TaskID: "task", ExecutionID: "execution", AttemptID: "attempt", ExecutionStatus: "running"}
	row := &consoleRow{kind: rowRun, run: run}
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	lines := allSessionLines(state, row, 90)
	needle := "[" + itemToken(message) + "]"
	itemLine := -1
	for i, line := range lines {
		if strings.Contains(line, needle) {
			itemLine = i
			break
		}
	}
	if itemLine < 0 {
		t.Fatal("process item was not rendered with a stable anchor")
	}
	state.detailScroll = itemLine
	captureSessionAnchor(&state, row, 90)
	if state.view.ReadGroup != view.Groups[0].ID || state.view.ReadItem != "body:body-key" {
		t.Fatalf("session anchor did not retain group/item identity: %+v", state.view)
	}
	state.conversationEvents = append(state.conversationEvents, completed)
	BuildConversationView(state.conversationEvents, state.view.Groups)
	if hidden := restoreSessionAnchor(&state, row, 90); !hidden {
		t.Fatal("folding the anchored group should relocate reading to its header")
	}
	collapsed := allSessionLines(state, row, 90)
	headerToken := "[" + groupToken(view.Groups[0].ID) + "]"
	headerLine := -1
	for i, line := range collapsed {
		if strings.Contains(line, headerToken) {
			headerLine = i
			break
		}
	}
	if headerLine < 0 || state.detailScroll != headerLine {
		t.Fatalf("scroll anchor missed the folded turn header: scroll=%d header=%d", state.detailScroll, headerLine)
	}
}

func TestConsoleNativeLocalAndOrcaFixtureUseSameFoldScript(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "validation", "pr-04b-public-history-native-2026-09-25.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Samples []struct {
			Backend  string `json:"backend"`
			Protocol string `json:"protocol"`
			Source   string `json:"source"`
			Thread   struct {
				ID    string `json:"id"`
				Turns []struct {
					ID, Status             string
					StartedAt, CompletedAt int64
					Items                  []struct{ ID, Type, Phase, Text, Status string }
				} `json:"turns"`
			} `json:"thread"`
		} `json:"samples"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Samples) != 2 {
		t.Fatalf("need local and Orca native samples, got %d", len(fixture.Samples))
	}
	for _, sample := range fixture.Samples {
		t.Run(sample.Backend, func(t *testing.T) {
			var events []contracts.ConversationEvent
			for turnIndex, turn := range sample.Thread.Turns {
				startTime := time.Unix(turn.StartedAt, 0).UTC()
				endTime := time.Unix(turn.CompletedAt, 0).UTC()
				base := contracts.ConversationEvent{Version: contracts.ConversationVersion, ProjectID: "native-project", WorkflowID: "native-workflow", ExecutionID: "native-execution", AttemptID: "native-attempt", Backend: executionapi.BackendKind(sample.Backend), Protocol: executionapi.ExecutionProtocol(sample.Protocol), Agent: "codex", TargetID: "native-target", Source: sample.Source, SourceSegment: "native-segment", SourceIdentity: "native-source", SessionID: sample.Thread.ID, SessionQuality: contracts.IdentityNative, TurnID: turn.ID, TurnQuality: contracts.IdentityNative, Phase: contracts.MessagePhaseUnknown, OccurredAt: &startTime, ReceivedAt: startTime}
				started := base
				started.EventID = int64(len(events) + 1)
				started.Type = contracts.ConversationTurnStarted
				events = append(events, started)
				for itemIndex, item := range turn.Items {
					event := base
					event.EventID = int64(len(events) + 1)
					event.ItemID = item.ID
					event.ItemKey = fmt.Sprintf("item-%d-%d", turnIndex, itemIndex)
					event.ItemQuality = contracts.IdentityNative
					event.SourceEventID = "fixture:" + item.ID
					event.SourceEventQuality = contracts.IdentityNative
					switch item.Type {
					case "agentMessage":
						event.Type = contracts.ConversationMessageComplete
						event.Phase = contracts.MessagePhase(item.Phase)
						event.Body = &contracts.ConversationBodyRef{ExecutionID: event.ExecutionID, AttemptID: event.AttemptID, ItemKey: event.ItemKey, Revision: 1, Bytes: int64(len(item.Text)), Completeness: contracts.BodyComplete}
					case "commandExecution":
						event.Type = contracts.ConversationToolCompleted
						event.Status = item.Status
					case "reasoning":
						event.Type = contracts.ConversationObservation
					default:
						continue
					}
					events = append(events, event)
				}
				ended := base
				ended.EventID = int64(len(events) + 1)
				ended.Type = contracts.ConversationTurnCompleted
				ended.Status = turn.Status
				ended.FinalAvailability = contracts.FinalAvailable
				ended.OccurredAt = &endTime
				events = append(events, ended)
			}
			choices := map[string]FoldChoice{}
			view, changed := BuildConversationView(events, choices)
			if !changed || len(view.Groups) != 2 {
				t.Fatalf("native %s fixture did not produce two folded turns: %+v", sample.Backend, view)
			}
			finals, commentary, tools := 0, 0, 0
			for _, group := range view.Groups {
				if !Folded(group, choices[group.ID]) {
					t.Fatal("completed native turn was not folded")
				}
				finals += len(group.Finals)
				for _, event := range group.Process {
					if event.Phase == contracts.MessagePhaseCommentary {
						commentary++
					}
					if event.Type == contracts.ConversationToolCompleted {
						tools++
					}
				}
			}
			if finals != 2 || commentary < 1 {
				t.Fatalf("native message phases were lost: finals=%d commentary=%d", finals, commentary)
			}
			if sample.Backend == string(executionapi.BackendOrca) && tools < 2 {
				t.Fatalf("Orca tool activity lost: %d", tools)
			}
			state := projectState{view: ProjectView{Groups: choices, Follow: true}, conversationEvents: events}
			run := &contracts.Run{RunID: "native-workflow", TaskID: "native-task", ExecutionID: "native-execution", AttemptID: "native-attempt", Backend: sample.Backend, Protocol: sample.Protocol}
			row := &consoleRow{kind: rowRun, run: run}
			for _, width := range []int{80, 170} {
				rendered := strings.Join(renderSession(state, row, width, 24, time.Now()), "\n")
				if !strings.Contains(rendered, "final_answer") || !strings.Contains(rendered, "completed") {
					t.Fatalf("native %s render at width %d lost final/terminal status:\n%s", sample.Backend, width, rendered)
				}
			}
		})
	}
}

func TestStableTurnKeysSeparateSameItemIDsAndUnknownTurnDoesNotFold(t *testing.T) {
	base := contracts.ConversationEvent{Version: contracts.ConversationVersion, ProjectID: "p", ExecutionID: "e", AttemptID: "a", Source: "source", SourceSegment: "seg", SessionID: "session", SessionQuality: contracts.IdentityNative, ItemID: "same", ItemQuality: contracts.IdentityNative, Phase: contracts.MessagePhaseUnknown}
	a := base
	a.TurnID = "turn-a"
	a.TurnQuality = contracts.IdentityNative
	a.Type = contracts.ConversationTurnStarted
	b := base
	b.TurnID = "turn-b"
	b.TurnQuality = contracts.IdentityNative
	b.Type = contracts.ConversationTurnStarted
	unknown := base
	unknown.TurnID = ""
	unknown.TurnQuality = contracts.IdentityUnknown
	unknown.Type = contracts.ConversationTurnFailed
	choices := map[string]FoldChoice{}
	view, _ := BuildConversationView([]contracts.ConversationEvent{a, b, unknown}, choices)
	if len(view.Groups) != 3 {
		t.Fatalf("turn IDs and unknown scope must remain distinct: %d", len(view.Groups))
	}
	for _, group := range view.Groups {
		if group.TurnID == "" && Folded(group, choices[group.ID]) {
			t.Fatal("unknown turn boundary must not auto fold")
		}
	}
}

func TestRunStatusKeepsBusinessExecutionAndUnknownAxesSeparate(t *testing.T) {
	cases := []struct {
		run  *contracts.Run
		want string
	}{
		{&contracts.Run{ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision"}, "待决策"},
		{&contracts.Run{ExecutionStatus: "running", FinishTaskStatus: "waiting_pr_merge"}, "待合并"},
		{&contracts.Run{ExecutionStatus: "unknown", FinishTaskStatus: "completed"}, "需处理"},
		{&contracts.Run{ExecutionStatus: "failed", FinishTaskStatus: "completed"}, "需处理"},
		{&contracts.Run{ExecutionStatus: "succeeded", FinishTaskStatus: "completed", PR: contracts.PRState{QueryError: stringPointer("provider unavailable")}}, "需处理"},
		{&contracts.Run{ExecutionStatus: "succeeded", FinishTaskStatus: "completed"}, "completed"},
	}
	for _, tc := range cases {
		if got := runStatus(tc.run); !strings.HasPrefix(got, tc.want) {
			t.Fatalf("runStatus(%+v)=%q, want prefix %q", tc.run, got, tc.want)
		}
	}
}

func TestTaskDetailOnlyShowsItsOwnControlReceipt(t *testing.T) {
	now := time.Now().UTC()
	state := projectState{actions: map[string]*actionState{
		"own-request":   {RequestID: "own-request", TaskID: "task-A", Status: "accepted", Message: "开始请求已接受", UpdatedAt: now},
		"other-request": {RequestID: "other-request", TaskID: "task-B", Status: "failed", Message: "其他任务失败", UpdatedAt: now.Add(time.Minute)},
	}}
	shown := strings.Join(renderActionLines(state, "task-A"), "\n")
	if !strings.Contains(shown, "本任务操作回执") || !strings.Contains(shown, "开始请求已接受") || strings.Contains(shown, "其他任务失败") || strings.Contains(shown, "own-request") {
		t.Fatalf("task receipt was not scoped or was too technical: %q", shown)
	}
}

func stringPointer(value string) *string { return &value }

func TestViewProfilesSeparateLiveWindowsAndRecoverAfterClose(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	first, _, _, err := acquireViewProfile(os.Getenv("STEERLINE_HOME"))
	if err != nil {
		t.Fatal(err)
	}
	second, _, _, err := acquireViewProfile(os.Getenv("STEERLINE_HOME"))
	if err != nil {
		t.Fatal(err)
	}
	if first.dir == second.dir {
		t.Fatal("two live windows must acquire distinct view identities")
	}
	firstDir, secondDir := first.dir, second.dir
	first.close()
	second.close()
	reopened, _, _, err := acquireViewProfile(os.Getenv("STEERLINE_HOME"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close()
	if reopened.dir != firstDir && reopened.dir != secondDir {
		t.Fatal("a later window should reuse one inactive view profile")
	}
}

func TestViewProfileContainmentRejectsSiblingPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "views")
	inside := filepath.Join(root, "profile")
	outside := filepath.Join(filepath.Dir(root), "outside")
	if !insideViewRoot(root, inside) || insideViewRoot(root, outside) {
		t.Fatalf("view profile containment failed: root=%s inside=%s outside=%s", root, inside, outside)
	}
}

func TestViewPreferencesCorruptionIsLocalToViewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.v1.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readViewFile(path); err == nil {
		t.Fatal("corrupt view file should be rejected for reset")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "not json" {
		t.Fatal("read-only load must not overwrite a damaged view file")
	}
}

func TestViewPreferencesRejectInvalidOffsetsAndActionIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view.v1.json")
	view := emptyViewFile()
	view.Projects["project"] = ProjectView{ConfigPath: "config.json", RunDir: "state", BodyOffset: -1, Groups: map[string]FoldChoice{}, Actions: map[string]SavedAction{}}
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readViewFile(path); err == nil {
		t.Fatal("negative body offset was accepted")
	}
	view.Projects["project"] = ProjectView{ConfigPath: "config.json", RunDir: "state", Groups: map[string]FoldChoice{}, Actions: map[string]SavedAction{"not-an-id": {Action: "start_task", Status: "submitted", TargetInstance: "instance"}}}
	data, err = json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readViewFile(path); err == nil {
		t.Fatal("invalid saved request identity was accepted")
	}
	requestID := "0123456789abcdef0123456789abcdef"
	view.Projects["project"] = ProjectView{ConfigPath: "config.json", RunDir: "state", Groups: map[string]FoldChoice{}, Outstanding: map[string]string{requestID: "task:task-A"}, Actions: map[string]SavedAction{}}
	data, err = json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readViewFile(path); err == nil {
		t.Fatal("outstanding request without its immutable action identity was accepted")
	}
	oversized, err := os.Create(filepath.Join(t.TempDir(), "oversized.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = oversized.Truncate(maxViewFileBytes + 1); err != nil {
		oversized.Close()
		t.Fatal(err)
	}
	oversizedPath := oversized.Name()
	if err = oversized.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = readViewFile(oversizedPath); err == nil {
		t.Fatal("oversized view file was accepted")
	}
	if panelIndex(panelName(4)) != 4 {
		t.Fatal("narrow list panel did not survive a preference round trip")
	}
}

func TestBodyPagingUsesExactUTF8SafePageOffsets(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, &testControlClient{}, now)
	state := &m.projects[0]
	state.bodyRef = &contracts.ConversationBodyRef{ExecutionID: "execution", AttemptID: "attempt", ItemKey: "body", Revision: 1, Bytes: 200000, Completeness: contracts.BodyComplete}
	state.bodyEvent = "body-event"
	state.bodyPages = []int64{0, 65533}
	state.bodyViewOffset = 65533
	state.bodyOffset = 131066
	updated, cmd := m.pageBody(false)
	m = updated.(Model)
	if cmd == nil || m.projects[0].bodyViewOffset != 0 {
		t.Fatalf("previous page did not use the exact recorded start: %d", m.projects[0].bodyViewOffset)
	}
	m.projects[0].bodyBusy = false
	m.projects[0].bodyViewOffset = 0
	m.projects[0].bodyOffset = 65533
	updated, cmd = m.pageBody(true)
	m = updated.(Model)
	if cmd == nil || m.projects[0].bodyViewOffset != 65533 {
		t.Fatalf("next page did not reuse the UTF-8-safe offset: %d", m.projects[0].bodyViewOffset)
	}
}

func TestBodyResponseIsBoundToGenerationAndRevision(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, &testControlClient{}, now)
	oldRef := contracts.ConversationBodyRef{ExecutionID: "execution", AttemptID: "attempt", ItemKey: "same", Revision: 1, SHA256: "old", Bytes: 3, Completeness: contracts.BodyComplete}
	newRef := oldRef
	newRef.Revision, newRef.SHA256 = 2, "new"
	state := &m.projects[0]
	state.bodyRef, state.bodyEvent, state.bodyGeneration, state.bodyBusy = &newRef, "same-event", 2, true
	updated, _ := m.Update(bodyMsg{projectKey: ref.Key, identity: ref.Identity, eventKey: "same-event", generation: 1, body: oldRef, page: contracts.ConversationBodyPage{Data: []byte("old")}})
	m = updated.(Model)
	if m.projects[0].bodyText != "" || !m.projects[0].bodyBusy {
		t.Fatal("stale body response changed the active revision")
	}
	updated, _ = m.Update(bodyMsg{projectKey: ref.Key, identity: ref.Identity, eventKey: "same-event", generation: 2, body: newRef, page: contracts.ConversationBodyPage{ItemKey: "same", Revision: 2, Offset: 0, NextOffset: 3, TotalBytes: 3, SHA256: "new", Completeness: contracts.BodyComplete, Complete: true, Data: []byte("new")}})
	m = updated.(Model)
	if m.projects[0].bodyText != "new" || m.projects[0].bodyBusy {
		t.Fatal("matching body response was not accepted")
	}
	m.projects[0].bodyGeneration, m.projects[0].bodyBusy, m.projects[0].bodyText = 3, true, ""
	updated, _ = m.Update(bodyMsg{projectKey: ref.Key, identity: ref.Identity, eventKey: "same-event", generation: 3, body: newRef, page: contracts.ConversationBodyPage{ItemKey: "same", Revision: 1, Offset: 0, NextOffset: 3, TotalBytes: 3, SHA256: "new", Completeness: contracts.BodyComplete, Complete: true, Data: []byte("bad")}})
	m = updated.(Model)
	if m.projects[0].bodyError == "" || m.projects[0].bodyText != "" {
		t.Fatal("mismatched body page was rendered")
	}
}

func TestBackgroundProjectBodyResponseStaysWithItsProject(t *testing.T) {
	now := time.Now().UTC()
	a, b := testProject("a"), testProject("b")
	m := modelForTest([]ProjectRef{a, b}, []ProjectSnapshot{readySnapshot(a, now), readySnapshot(b, now)}, &testControlClient{}, now)
	ref := contracts.ConversationBodyRef{ExecutionID: "execution", AttemptID: "attempt", ItemKey: "body", Revision: 1, SHA256: "digest", Bytes: 4, Completeness: contracts.BodyComplete}
	m.projects[0].bodyRef, m.projects[0].bodyEvent, m.projects[0].bodyGeneration, m.projects[0].bodyBusy = &ref, "event", 1, true
	m.selected = 1
	page := contracts.ConversationBodyPage{ItemKey: "body", Revision: 1, Offset: 0, NextOffset: 4, TotalBytes: 4, SHA256: "digest", Completeness: contracts.BodyComplete, Complete: true, Data: []byte("text")}
	updated, _ := m.Update(bodyMsg{projectKey: a.Key, identity: a.Identity, eventKey: "event", generation: 1, body: ref, page: page})
	m = updated.(Model)
	if m.projects[0].bodyText != "text" || m.projects[0].bodyBusy || m.projects[1].bodyText != "" {
		t.Fatalf("background body response crossed or vanished between projects: a=%q b=%q", m.projects[0].bodyText, m.projects[1].bodyText)
	}
}

func TestViewFileKeepsRequestIdentityWithoutCachingReceiptText(t *testing.T) {
	message := "provider diagnostic accidentally included token=secret-value"
	action := &actionState{RequestID: "0123456789abcdef0123456789abcdef", Action: "start_task", TaskID: "task-A", Status: "failed", Message: message, TargetInstance: "instance-A", UpdatedAt: time.Now()}
	raw, err := json.Marshal(savedActions(map[string]*actionState{action.RequestID: action}, 16))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret-value") || !strings.Contains(string(raw), action.RequestID) {
		t.Fatalf("view preferences stored receipt text or lost the stable request identity: %s", raw)
	}
}

func TestConsoleTextEscapesTerminalControlCharacters(t *testing.T) {
	got := safeTerminalText("hello\x1b[2J\x00world\n")
	if strings.ContainsRune(got, '\x1b') || !strings.Contains(got, "␛") || !strings.Contains(got, "\\u0000") {
		t.Fatalf("unsafe terminal text: %q", got)
	}
	state := projectState{view: ProjectView{Follow: true}, activityEvents: []contracts.ActivityEvent{{Kind: "bad\x1b[2J", ItemID: "item\x00", Status: "failed", Meta: map[string]any{"exit_code": "\x1b[2J"}}}}
	rendered := strings.Join(renderActivity(state, nil, 80, 10), "\n")
	if strings.ContainsRune(rendered, '\x1b') || strings.Contains(rendered, "bad\u001b") {
		t.Fatalf("activity metadata reached terminal unsanitized: %q", rendered)
	}
}

func TestNarrowConsoleKeepsDetailAndSelectionEntryReachable(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	control := &testControlClient{}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, control, now)
	m.width, m.height = 80, 24
	view := m.View()
	for _, want := range []string{"任务列表", "▸", "Build B", "d 完整详情", "s 选方案"} {
		if !strings.Contains(view, want) {
			t.Fatalf("narrow console lost %q:\n%s", want, view)
		}
	}
	updated, _ := m.Update(runeKey('d'))
	m = updated.(Model)
	if !strings.Contains(m.View(), "Task：task-B") {
		t.Fatal("full read-only detail is not reachable")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if !strings.Contains(m.View(), "任务动态 · 活动") || m.projects[0].focus != 1 {
		t.Fatal("Tab did not focus task activity")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if control.calls != 0 || len(m.projects[0].actions) != 0 {
		t.Fatal("narrow list navigation must not create or start an execution")
	}
}

func TestPanelScrollHeightMatchesWrappedFooter(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, &testControlClient{}, now)
	m.width, m.height = 80, 24
	footerLines := wrapLines(strings.Split(renderFooter(m, m.projects[0]), "\n"), m.width)
	expected := max(1, max(3, m.height-3-len(footerLines))-1)
	if got := activityPanelHeight(m); got != expected {
		t.Fatalf("narrow scroll viewport=%d, render viewport=%d", got, expected)
	}
	m.width = 170
	expected = max(1, consoleContentHeight(m)-detailPanelHeight(m)-2)
	if got := activityPanelHeight(m); got != expected {
		t.Fatalf("wide scroll viewport=%d, render viewport=%d", got, expected)
	}
}

func TestLongTaskListKeepsSelectedRowInsideViewport(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	snapshot := readySnapshot(ref, now)
	snapshot.Tasks.Tasks = nil
	for i := 0; i < 50; i++ {
		snapshot.Tasks.Tasks = append(snapshot.Tasks.Tasks, contracts.TaskCandidate{TaskID: fmt.Sprintf("task-%02d", i), Title: "candidate"})
	}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.projects[0].selectedKey = "task:task-40"
	lines := renderRows(m, m.projects[0], 80, 8)
	if len(lines) > 8 || !strings.Contains(strings.Join(lines, "\n"), "task-40") {
		t.Fatalf("selected task fell outside the visible list: %q", strings.Join(lines, "\n"))
	}
}

func TestEmptyRunningFilterExplainsWhyNativeTasksAreHidden(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("native")
	snapshot := readySnapshot(ref, now)
	snapshot.Tasks.Tasks = nil
	snapshot.NativeTasks = &contracts.NativeTaskPage{Tasks: []contracts.NativeTaskDetail{{TaskRecord: contracts.TaskRecord{TaskID: "103", Title: "Waiting for merge", Status: "in_progress"}}}}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.projects[0].filter = 2
	empty := strings.Join(renderRows(m, m.projects[0], 80, 8), "\n")
	if !strings.Contains(empty, "3 进行中") || !strings.Contains(empty, "1全部") || !strings.Contains(empty, "5待合并") || !strings.Contains(empty, "按 1 查看全部任务") || strings.Contains(empty, "当前没有 Ready 任务") {
		t.Fatalf("empty filtered list implied the task data disappeared: %q", empty)
	}
	if !strings.Contains(renderFooter(m, m.projects[0]), "1–7 筛选") || !strings.Contains(strings.Join(renderHelp(m.projects[0]), "\n"), "5 待合并") {
		t.Fatal("the persistent footer and help omitted the filter shortcuts")
	}
	m.projects[0].filter = 0
	all := strings.Join(renderRows(m, m.projects[0], 80, 8), "\n")
	if !strings.Contains(all, "103") {
		t.Fatalf("native task is missing from the all filter: %q", all)
	}
}

func TestNativeTaskViewKeepsOffPageSelectionAndEscapesText(t *testing.T) {
	now := time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC)
	ref := testProject("native")
	ref.Presentation.TaskAuthority = "native"
	snapshot := readySnapshot(ref, now)
	snapshot.Tasks.Tasks = nil
	for i := 1; i <= 501; i++ {
		snapshot.Tasks.Tasks = append(snapshot.Tasks.Tasks, contracts.TaskCandidate{TaskID: fmt.Sprint(i), Title: "candidate"})
	}
	snapshot.NativeTasks = &contracts.NativeTaskPage{SampledAt: now, HasMore: true, Tasks: []contracts.NativeTaskDetail{{TaskRecord: contracts.TaskRecord{TaskID: "1", Title: "first", Status: "open"}}}}
	snapshot.SelectedNative = &contracts.NativeTaskDetail{TaskRecord: contracts.TaskRecord{TaskID: "501", Title: "中文标题\x1b[2J\n伪造按钮\u202e<script>", Status: "open"}, Predecessors: []contracts.TaskDependencyStatus{{TaskID: "1", Status: "discarded", Blocker: "discarded_predecessor"}}}
	snapshot.NativeTaskError = "只读取前 500 个任务；剩余任务不在此快照中"
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.projects[0].selectedKey = "task:501"
	rows := m.rows(0)
	if !rowExists(rows, "task:501") || m.selectedRow(0).native == nil {
		t.Fatal("selected Ready task beyond the detail page lost its native detail")
	}
	lines := renderRows(m, m.projects[0], 80, 8)
	if len(lines) > 8 || !strings.Contains(strings.Join(lines, "\n"), "501") {
		t.Fatalf("selected task was hidden by truncation notices: %q", lines)
	}
	compact := renderRows(m, m.projects[0], 80, 3)
	if len(compact) > 3 || !strings.Contains(strings.Join(compact, "\n"), "501") {
		t.Fatalf("selected task was hidden in a small viewport: %q", compact)
	}
	detail := strings.Join(renderNativeTaskDetail(m.projects[0], *snapshot.SelectedNative), "\n")
	if strings.ContainsRune(detail, '\x1b') || strings.ContainsRune(detail, '\u202e') || !strings.Contains(detail, "␛[2J\\n伪造按钮\\u202e") || !strings.Contains(detail, "discarded_predecessor") {
		t.Fatalf("native detail rendered unsafe or incomplete text: %q", detail)
	}
	m.projects[0].refreshID = 1
	updated, _ := m.Update(projectMsg{key: ref.Key, identity: ref.Identity, requestID: 1, snapshot: snapshot})
	m = updated.(Model)
	if m.projects[0].selectedKey != "task:501" {
		t.Fatalf("refresh lost the stable task selection: %q", m.projects[0].selectedKey)
	}
	m.projects[0].refreshID = 2
	updated, _ = m.Update(projectMsg{key: ref.Key, identity: ref.Identity, requestID: 2, snapshot: ProjectSnapshot{Connection: api.ServiceConnection{State: api.ConnectionOffline}, NativeTaskError: "database unavailable"}})
	m = updated.(Model)
	if m.projects[0].selectedKey != "task:501" || m.selectedRow(0) == nil || m.selectedRow(0).native == nil {
		t.Fatal("offline refresh lost the selected task's last readable detail")
	}
	up := m.projects[0]
	if valid, _ := api.CandidateFreshness(ref.Identity, up.snapshot, now); valid {
		t.Fatal("offline Ready data became a start permit")
	}
	if !strings.Contains(strings.Join(renderNativeTaskDetail(up, *snapshot.SelectedNative), "\n"), "离线只读快照") {
		t.Fatal("offline native detail lost its sample label")
	}
	runSnapshot := snapshot
	runSnapshot.Observation.Runtime.Runs = []*contracts.Run{{RunID: "run-501", TaskID: "501", DomainTaskID: "501", TaskStatus: "in_progress", ExecutionStatus: "running"}}
	m.projects[0].snapshot = runSnapshot
	m.projects[0].selectedKey = "task:501"
	if selectedNativeTaskID(m.projects[0]) != "501" {
		t.Fatal("selected native run did not retain its task identity")
	}
	runDetail := strings.Join(renderDetail(m, m.projects[0], m.selectedRow(0), 120, 200), "\n")
	if !strings.Contains(runDetail, "任务关系") || strings.Contains(runDetail, "中心只读快照") || !strings.Contains(runDetail, "discarded_predecessor") || !strings.Contains(runDetail, "任务状态：进行中") {
		t.Fatalf("selected native run lost its task relationship: %q", runDetail)
	}
	querySource := &testQuerySource{snapshot: runSnapshot}
	m.projects[0].client.(*testClient).query = querySource
	if cmd := m.readProject(0); cmd == nil {
		t.Fatal("native run refresh did not request its task detail")
	} else {
		_ = cmd()
	}
	if querySource.selected != "501" {
		t.Fatalf("native run detail query used task %q", querySource.selected)
	}
}

func TestUnicodeLongErrorFitsNarrowAndWidePanels(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	control := &testControlClient{}
	snapshot := readySnapshot(ref, now)
	longError := strings.Repeat("错误🙂路径过长，", 40) + "\x1b[2J"
	snapshot.Tasks.Tasks = nil
	snapshot.Observation.Runtime.Runs = []*contracts.Run{{TaskID: "任务-一", RunID: "run-unicode", ExecutionID: "execution-unicode", AttemptID: "attempt-unicode", Title: strings.Repeat("中文标题🙂", 30), ExecutionStatus: "failed", FinishTaskStatus: "blocked", Error: &longError}}
	snapshot.Connection.Snapshot.Runtime = snapshot.Observation.Runtime
	for _, width := range []int{80, 170} {
		m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, control, now)
		m.width, m.height = width, 24
		m.projects[0].selectedKey = "task:任务-一"
		for _, panelKey := range []rune{'d', 'a', 'c', 'f', 't'} {
			updated, _ := m.Update(runeKey(panelKey))
			m = updated.(Model)
			view := m.View()
			if strings.ContainsRune(view, '\x1b') && strings.Contains(view, "[2J") {
				t.Fatalf("panel %q rendered source escape controls", panelKey)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > width {
					t.Fatalf("panel %q overflowed width %d with %d columns: %q", panelKey, width, lipgloss.Width(line), line)
				}
			}
		}
	}
}

func TestControlRequestIDFromErrorRemainsQueryOnly(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	control := &testControlClient{err: errors.New("temporary write error")}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, control, now)
	updated, cmd := m.createAction("start_task", "task-B")
	m = updated.(Model)
	var requestID string
	for id := range m.projects[0].actions {
		requestID = id
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.projects[0].actions[requestID].Status != "indeterminate" {
		t.Fatal("uncertain publication must remain indeterminate")
	}
	reconcile := m.pollAction(0, m.projects[0].actions[requestID])
	if reconcile == nil || control.calls != 1 {
		t.Fatal("reconciliation must query the same ID without publishing again")
	}
	updated, _ = m.Update(reconcile())
	m = updated.(Model)
	if control.calls != 1 || control.receipts != 1 || control.requests != 1 || m.projects[0].actions[requestID].Status != "indeterminate" {
		t.Fatal("querying an uncertain action must retain its original ID and avoid a second submission")
	}
}

func TestReconciliationRejectsChangedRequestPayload(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, &testControlClient{}, now)
	requestID := "0123456789abcdef0123456789abcdef"
	m.projects[0].actions[requestID] = &actionState{RequestID: requestID, Action: "start_task", TaskID: "task-B", Status: "indeterminate", TargetInstance: "instance-alpha", UpdatedAt: now}
	wrongPayload, _ := json.Marshal(map[string]string{"task_id": "task-A"})
	request := &api.ControlRequest{ProtocolVersion: 1, RequestID: requestID, RepositoryIdentity: ref.Identity, InstanceID: "instance-alpha", Action: "start_task", Payload: wrongPayload}
	updated, _ := m.Update(actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: requestID, request: request})
	m = updated.(Model)
	action := m.projects[0].actions[requestID]
	if !action.Resolved || action.Status != "indeterminate" || !strings.Contains(action.Message, "payload") {
		t.Fatalf("changed request payload was accepted: %+v", action)
	}
}
