package viewer

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func strptr(s string) *string { return &s }

type fakeSource struct {
	runs           []*steerline.Run
	activity       map[string][]steerline.ActivityEvent
	events         map[string][]string
	taskDetail     map[string]steerline.TaskDetail
	answeredOption string
	answeredNote   string
	answeredBy     string
	answerErr      error
}

func (f *fakeSource) Snapshot() (*steerline.Runtime, error) {
	return &steerline.Runtime{
		Repository: steerline.Repository{Path: "C:/repo/merge-h-308-code", Branch: "main"},
		Service:    steerline.ServiceState{Status: "running", PID: 4321},
		Runs:       f.runs,
	}, nil
}

func (f *fakeSource) Activity(logPath string, from int64) ([]steerline.ActivityEvent, int64, error) {
	ev := f.activity[logPath]
	if from > int64(len(ev)) {
		from = int64(len(ev))
	}
	return ev[from:], int64(len(ev)), nil
}

func (f *fakeSource) Events(logPath string, from int64) ([]string, int64, error) {
	ev := f.events[logPath]
	if from > int64(len(ev)) {
		from = int64(len(ev))
	}
	return ev[from:], int64(len(ev)), nil
}

func (f *fakeSource) TaskDetail(taskID string) (steerline.TaskDetail, error) {
	return f.taskDetail[taskID], nil
}

func (f *fakeSource) Answer(_ *steerline.Run, optionID, note, by string) error {
	if f.answerErr != nil {
		return f.answerErr
	}
	f.answeredOption, f.answeredNote, f.answeredBy = optionID, note, by
	return nil
}

func key(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func pendingRun() *steerline.Run {
	return &steerline.Run{
		TaskID:           "n308-21",
		RunID:            "r1",
		Title:            "DigHole migration",
		Host:             "orca",
		ExecutionStatus:  "succeeded",
		FinishTaskStatus: "waiting_decision",
		LogPath:          strptr("log"),
		Decision: &steerline.DecisionState{
			ID:     "d1",
			Status: "pending",
			Request: steerline.DecisionRequest{
				Question:    "Which migration option?",
				Options:     []steerline.DecisionOption{{ID: "a", Label: "Keep compat"}, {ID: "b", Label: "Relax persistence"}},
				Recommended: "a",
			},
		},
	}
}

func newTestModel(f *fakeSource) Model {
	m := NewModel(steerline.Config{}, f, Options{})
	m.width, m.height = 100, 30
	rt, _ := f.Snapshot()
	mm, _ := m.Update(runsMsg{runs: f.runs, rt: rt})
	return mm.(Model)
}

func TestViewerShowsStatusAndAxes(t *testing.T) {
	run := pendingRun()
	run.StartedAt = timePtr(time.Now().Add(-2 * time.Minute))
	f := &fakeSource{runs: []*steerline.Run{run}}
	m := newTestModel(f)
	view := m.View()
	for _, want := range []string{"merge-h-308-code", "main", "svc running", "slot free", "DECIDE", "decide 1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("status/axes missing %q:\n%s", want, view)
		}
	}
	// The execution axis must be shown separately from the delivery badge.
	if !strings.Contains(view, "ok") {
		t.Fatalf("execution axis marker missing:\n%s", view)
	}
}

func timePtr(t time.Time) *time.Time { return &t }

func TestViewerShowsTaskDetailAndTabs(t *testing.T) {
	run := pendingRun()
	prio := 2
	f := &fakeSource{
		runs: []*steerline.Run{run},
		taskDetail: map[string]steerline.TaskDetail{
			"n308-21": {ID: "n308-21", Type: "bug", Status: "open", Priority: &prio, Labels: []string{"compat"}, Description: "keep old saves loadable"},
		},
	}
	m := newTestModel(f)
	// The first load is lazy; simulate the async result arriving.
	mm, _ := m.Update(taskDetailMsg{taskID: "n308-21", detail: f.taskDetail["n308-21"]})
	m = mm.(Model)
	view := m.View()
	for _, want := range []string{"Detail · n308-21", "Summary", "Workspace", "PR", "Decision", "Result", "bug"} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail pane missing %q:\n%s", want, view)
		}
	}
	// Switch to the Workspace tab and confirm the tab bar moved and content changed.
	mm, _ = m.Update(key(']'))
	m = mm.(Model)
	if m.detailTab != 1 {
		t.Fatalf("expected tab 1, got %d", m.detailTab)
	}
	ws := m.View()
	if !strings.Contains(ws, "worktree") {
		t.Fatalf("workspace tab missing workspace fields:\n%s", ws)
	}
	if strings.Contains(ws, "waiting_decision") {
		t.Fatalf("workspace tab must replace summary content")
	}
}

func TestViewerLazyLoadsTaskDetailOnce(t *testing.T) {
	f := &fakeSource{runs: []*steerline.Run{pendingRun()}}
	m := newTestModel(f)
	// newTestModel goes through runsMsg, which should have issued one load.
	if !m.taskLoading["n308-21"] {
		t.Fatal("selecting a run must lazily request its task detail")
	}
	if _, cmd := m.ensureTaskDetail(); cmd != nil {
		t.Fatal("a second request must be deduplicated while one is in flight")
	}
	// Cache the result and confirm no further load is issued.
	mm, _ := m.Update(taskDetailMsg{taskID: "n308-21"})
	m = mm.(Model)
	if _, cmd := m.ensureTaskDetail(); cmd != nil {
		t.Fatal("cached detail must not be requested again")
	}
}

func TestFoldEventsMergesAndAccumulates(t *testing.T) {
	in := []steerline.ActivityEvent{
		{Kind: "command", Status: "started", ItemID: "c1", Title: "go test"},
		{Kind: "command", Status: "completed", ItemID: "c1", Title: "go test", Detail: "ok"},
		{Kind: "message", Status: "running", ItemID: "m1", Detail: "Hel"},
		{Kind: "message", Status: "running", ItemID: "m1", Detail: "lo"},
		{Kind: "message", Status: "completed", Detail: "Hello"},
	}
	out := foldEvents(in, true)
	if len(out) != 2 {
		t.Fatalf("expected 2 folded events, got %d: %+v", len(out), out)
	}
	if out[0].Status != "completed" || out[0].Detail != "ok" {
		t.Fatalf("command started/completed not merged: %+v", out[0])
	}
	if out[1].Detail != "Hello" {
		t.Fatalf("streamed deltas not accumulated: %q", out[1].Detail)
	}
}

func TestViewerRawMode(t *testing.T) {
	run := pendingRun()
	f := &fakeSource{runs: []*steerline.Run{run}}
	m := newTestModel(f)
	mm, _ := m.Update(key('v')) // conversation
	m = mm.(Model)
	mm, _ = m.Update(key('v')) // raw
	m = mm.(Model)
	if m.mode != modeRaw {
		t.Fatalf("expected raw mode, got %v", m.mode)
	}
	mm, _ = m.Update(rawMsg{logPath: "log", lines: []string{`{"type":"thread.started"}`}, from: 28})
	m = mm.(Model)
	if !strings.Contains(m.View(), `{"type":"thread.started"}`) {
		t.Fatalf("raw line not rendered:\n%s", m.View())
	}
	mm, _ = m.Update(key('v')) // back to activity
	m = mm.(Model)
	if m.mode != modeActivity {
		t.Fatalf("expected activity mode, got %v", m.mode)
	}
}

func TestViewerScrollDecouplesFromFollow(t *testing.T) {
	f := &fakeSource{runs: []*steerline.Run{pendingRun()}}
	m := newTestModel(f)
	if !m.follow || !m.autoScroll {
		t.Fatal("follow and autoScroll must default on")
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = mm.(Model)
	if m.follow == false {
		t.Fatal("scrolling must not disable follow")
	}
	if m.autoScroll {
		t.Fatal("scrolling up must disable autoScroll")
	}
	if m.logScroll == 0 {
		t.Fatal("scrolling up must move the offset")
	}
	mm, _ = m.Update(key('G'))
	m = mm.(Model)
	if !m.autoScroll || m.logScroll != 0 {
		t.Fatal("G must return to the bottom and re-enable autoScroll")
	}
}

func TestViewerSurfacesPendingDecision(t *testing.T) {
	f := &fakeSource{
		runs: []*steerline.Run{
			{TaskID: "n308-27", RunID: "r0", ExecutionStatus: "succeeded", FinishTaskStatus: "completed", LogPath: strptr("log0")},
			pendingRun(),
		},
	}
	m := newTestModel(f)
	view := m.View()
	if !strings.Contains(view, "DECIDE") {
		t.Fatalf("pending badge missing:\n%s", view)
	}
	if !strings.Contains(view, "press a to answer") {
		t.Fatalf("answer hint missing:\n%s", view)
	}
	if m.current().TaskID != "n308-21" {
		t.Fatalf("pending run should sort first, got %s", m.current().TaskID)
	}
}

func TestViewerAnswersDecision(t *testing.T) {
	f := &fakeSource{runs: []*steerline.Run{pendingRun()}}
	m := newTestModel(f)

	mm, _ := m.Update(key('a'))
	m = mm.(Model)
	if m.modal == nil {
		t.Fatal("pressing a on a pending decision must open the modal")
	}
	// choose option 2 (b)
	mm, _ = m.Update(key('2'))
	m = mm.(Model)
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	if cmd == nil {
		t.Fatal("submitting must produce an answer command")
	}
	if f.answeredOption != "" {
		t.Fatal("answer must be written by the command, not during Update")
	}
	mm, _ = m.Update(cmd())
	m = mm.(Model)
	if f.answeredOption != "b" {
		t.Fatalf("wrong option answered: %q", f.answeredOption)
	}
	if f.answeredBy == "" {
		t.Fatal("answered_by must be filled")
	}
	if m.modal != nil {
		t.Fatal("modal must close after a successful answer")
	}
	if !strings.Contains(m.status, "answer submitted") {
		t.Fatalf("no status after answer: %q", m.status)
	}
}

func TestViewerIgnoresAnswerWithoutDecision(t *testing.T) {
	f := &fakeSource{runs: []*steerline.Run{{
		TaskID: "n308-27", RunID: "r0", ExecutionStatus: "succeeded", FinishTaskStatus: "completed", LogPath: strptr("log0"),
	}}}
	m := newTestModel(f)
	mm, _ := m.Update(key('a'))
	m = mm.(Model)
	if m.modal != nil {
		t.Fatal("modal must not open without a pending decision")
	}
}

func TestViewerRendersActivity(t *testing.T) {
	run := pendingRun()
	f := &fakeSource{
		runs: []*steerline.Run{run},
		activity: map[string][]steerline.ActivityEvent{
			"log": {{Kind: "claim", Status: "completed", Title: "claimed n308-21"}, {Kind: "decision", Status: "pending", Title: "decision requested"}},
		},
	}
	m := newTestModel(f)
	mm, _ := m.Update(activityMsg{logPath: "log", events: f.activity["log"], from: 2})
	m = mm.(Model)
	view := m.View()
	if !strings.Contains(view, "claimed n308-21") || !strings.Contains(view, "decision requested") {
		t.Fatalf("activity not rendered:\n%s", view)
	}
}
