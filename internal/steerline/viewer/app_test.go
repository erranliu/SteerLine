package viewer

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func strptr(s string) *string { return &s }

type fakeSource struct {
	runs           []*steerline.Run
	activity       map[string][]steerline.ActivityEvent
	answeredOption string
	answeredNote   string
	answeredBy     string
	answerErr      error
}

func (f *fakeSource) Runs() ([]*steerline.Run, error) { return f.runs, nil }

func (f *fakeSource) Activity(logPath string, from int64) ([]steerline.ActivityEvent, int64, error) {
	ev := f.activity[logPath]
	if from > int64(len(ev)) {
		from = int64(len(ev))
	}
	return ev[from:], int64(len(ev)), nil
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
	mm, _ := m.Update(runsMsg{runs: f.runs})
	return mm.(Model)
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
