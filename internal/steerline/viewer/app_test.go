package viewer

import (
	"fmt"
	"os"
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
	observations   []steerline.ObservationSnapshot
	snapshotCalls  int
	taskDetail     map[string]steerline.TaskDetail
	answeredOption string
	answeredNote   string
	answeredBy     string
	answerErr      error
}

func (f *fakeSource) Snapshot() steerline.ObservationSnapshot {
	if f.snapshotCalls < len(f.observations) {
		obs := f.observations[f.snapshotCalls]
		f.snapshotCalls++
		return obs
	}
	return steerline.ObservationSnapshot{State: steerline.SnapshotValid, SampledAt: time.Now(), Runtime: &steerline.Runtime{
		Repository: steerline.Repository{Path: "C:/repo/merge-h-308-code", Branch: "main"},
		Service:    steerline.ServiceState{Status: "running", PID: 4321},
		Runs:       f.runs,
	}}
}

func (f *fakeSource) ReadLog(logPath string, cursor steerline.JournalCursor, raw bool) (LogBatch, error) {
	batch := LogBatch{Cursor: cursor}
	if !cursor.Initialized {
		batch.Reset = true
		batch.Cursor = steerline.JournalCursor{Path: logPath, FileID: "fake-file", Generation: 1, Initialized: true}
	}
	if raw {
		batch.Raw = append(batch.Raw, f.events[logPath]...)
	} else {
		batch.Activity = append(batch.Activity, f.activity[logPath]...)
	}
	batch.Cursor.Offset++
	return batch, nil
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
	obs := f.Snapshot()
	mm, _ := m.Update(runsMsg{observation: obs})
	return mm.(Model)
}

func pendingLogMessage(m Model, batch LogBatch) logMsg {
	r := m.current()
	return logMsg{runID: r.RunID, logPath: *r.LogPath, key: m.activeLogKey, requestID: m.inFlightLogs[m.activeLogKey], generation: m.logGeneration, base: m.logCursor, batch: batch}
}

func observationFor(runs ...*steerline.Run) steerline.ObservationSnapshot {
	return steerline.ObservationSnapshot{State: steerline.SnapshotValid, SampledAt: time.Now(), Runtime: &steerline.Runtime{Runs: runs}}
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
	// The execution axis now lives in the detail pane (list is title-focused).
	if !strings.Contains(view, "succeeded") {
		t.Fatalf("execution status missing:\n%s", view)
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
	m.width, m.height = 170, 40
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

func TestViewerShowsMinimalWorkflowStageAndGateEvidence(t *testing.T) {
	run := pendingRun()
	run.WorkflowVersion = steerline.MinimalWorkflowVersion
	run.ExecutionID = "execution-1"
	run.Agent = "codex"
	run.Backend = "orca"
	run.Protocol = "orca_supervised"
	rt := &steerline.Runtime{
		Runs:         []*steerline.Run{run},
		WorkflowRuns: []steerline.WorkflowRunRecord{{WorkflowRunID: run.RunID, Phase: "executing"}},
		Executions:   []steerline.ExecutionRecord{{WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, CurrentStep: "verify", Status: "active", RepairRounds: 1, Target: steerline.ExecutionTarget{ID: "target-1", ConnectionID: "test-connection", OrchestrationRun: "orca-run-1", Dispatch: "dispatch-verify"}}},
		Attempts: []steerline.StepAttemptRecord{
			{ExecutionID: run.ExecutionID, StepID: "implement", AttemptNumber: 1, Status: "succeeded", ResultID: "implement-result"},
			{ExecutionID: run.ExecutionID, StepID: "verify", AttemptNumber: 1, Status: "running"},
		},
	}
	f := &fakeSource{runs: []*steerline.Run{run}, observations: []steerline.ObservationSnapshot{{State: steerline.SnapshotValid, SampledAt: time.Now(), Runtime: rt}}}
	m := NewModel(steerline.Config{}, f, Options{})
	m.width, m.height = 170, 50
	obs := f.Snapshot()
	updated, _ := m.Update(runsMsg{observation: obs})
	view := updated.(Model).View()
	for _, want := range []string{"workflow_phase", "executing", "current_step", "verify", "step_attempts", "implement#1=succeeded", "verify#1=running", "orchestration_run", "orca-run-1", "remote_cancellation", "unavailable"} {
		if !strings.Contains(view, want) {
			t.Fatalf("workflow stage or attempt evidence missing %q:\n%s", want, view)
		}
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
	mm, _ = m.Update(pendingLogMessage(m, LogBatch{Raw: []string{`{"type":"thread.started"}`}, Reset: true, Cursor: steerline.JournalCursor{Path: "log", FileID: "fake-file", Generation: 1, Initialized: true, Offset: 28}}))
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
	mm, _ := m.Update(pendingLogMessage(m, LogBatch{Activity: f.activity["log"], Reset: true, Cursor: steerline.JournalCursor{Path: "activity", FileID: "fake-file", Generation: 1, Initialized: true, Offset: 2}}))
	m = mm.(Model)
	view := m.View()
	if !strings.Contains(view, "claimed n308-21") || !strings.Contains(view, "decision requested") {
		t.Fatalf("activity not rendered:\n%s", view)
	}
}

func TestViewerSelectionStaysOnRunWhenPendingRunMovesToTop(t *testing.T) {
	runA := &steerline.Run{RunID: "A", TaskID: "task-A", ExecutionStatus: "running", LogPath: strptr("log-A")}
	runB := &steerline.Run{RunID: "B", TaskID: "task-B", ExecutionStatus: "running", LogPath: strptr("log-B")}
	f := &fakeSource{runs: []*steerline.Run{runA, runB}}
	m := NewModel(steerline.Config{}, f, Options{RunID: "B"})
	mm, _ := m.Update(runsMsg{observation: observationFor(runA, runB)})
	m = mm.(Model)
	if m.current().RunID != "B" {
		t.Fatalf("initial focus failed: %s", m.current().RunID)
	}
	runA = &steerline.Run{RunID: "A", TaskID: "task-A", ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision", LogPath: strptr("log-A"), Decision: &steerline.DecisionState{ID: "dA", Status: "pending"}}
	mm, _ = m.Update(runsMsg{observation: observationFor(runA, runB)})
	m = mm.(Model)
	if m.current().RunID != "B" {
		t.Fatalf("selection followed sorted index instead of stable run id: %s", m.current().RunID)
	}
}

func TestViewerAttemptPathChangeClearsOldCursorAndLines(t *testing.T) {
	run := &steerline.Run{RunID: "R", TaskID: "task-R", ExecutionStatus: "running", LogPath: strptr("runs/R/attempts/one/events.jsonl")}
	f := &fakeSource{runs: []*steerline.Run{run}}
	m := NewModel(steerline.Config{}, f, Options{RunID: "R"})
	mm, _ := m.Update(runsMsg{observation: observationFor(run)})
	m = mm.(Model)
	old := pendingLogMessage(m, LogBatch{Activity: []steerline.ActivityEvent{{Kind: "message", Detail: "old attempt"}}, Reset: true, Cursor: steerline.JournalCursor{Path: "activity-one", FileID: "a", Generation: 1, Offset: 99, Initialized: true}})
	mm, _ = m.Update(old)
	m = mm.(Model)
	if len(m.log) != 1 {
		t.Fatal("old attempt fixture did not load")
	}
	run.LogPath = strptr("runs/R/attempts/two/events.jsonl")
	mm, _ = m.Update(runsMsg{observation: observationFor(run)})
	m = mm.(Model)
	if len(m.log) != 0 || m.logCursor.Offset != 0 || m.logPath != *run.LogPath {
		t.Fatalf("attempt change retained old view/cursor: log=%+v cursor=%+v path=%s", m.log, m.logCursor, m.logPath)
	}
}

func TestViewerDropsOutOfOrderRunResponsesAndOnlyOneReadPerStream(t *testing.T) {
	runA := &steerline.Run{RunID: "A", TaskID: "task-A", ExecutionStatus: "running", LogPath: strptr("log-A")}
	runB := &steerline.Run{RunID: "B", TaskID: "task-B", ExecutionStatus: "running", LogPath: strptr("log-B")}
	f := &fakeSource{runs: []*steerline.Run{runA, runB}}
	m := NewModel(steerline.Config{}, f, Options{RunID: "A"})
	mm, _ := m.Update(runsMsg{observation: observationFor(runA, runB)})
	m = mm.(Model)
	keyA, idA := m.activeLogKey, m.inFlightLogs[m.activeLogKey]
	generationA := m.logGeneration
	mm, _ = m.move(1)
	m = mm.(Model)
	keyB, idB := m.activeLogKey, m.inFlightLogs[m.activeLogKey]
	if keyA == keyB || idA == 0 || idB == 0 {
		t.Fatalf("selection did not create separate stream reads: %q %q", keyA, keyB)
	}
	mm, cmd := m.move(-1)
	m = mm.(Model)
	if cmd != nil || m.inFlightLogs[keyA] != idA {
		t.Fatal("returning to A started a duplicate in-flight read")
	}
	stale := logMsg{runID: "B", logPath: "log-B", key: keyB, requestID: idB, base: steerline.JournalCursor{}, batch: LogBatch{Activity: []steerline.ActivityEvent{{Kind: "message", Detail: "B stale"}}, Cursor: steerline.JournalCursor{Path: "log-B", Generation: 1, Initialized: true}}}
	mm, _ = m.Update(stale)
	m = mm.(Model)
	if len(m.log) != 0 {
		t.Fatalf("late B response appeared in A: %+v", m.log)
	}
	oldA := logMsg{runID: "A", logPath: "log-A", key: keyA, requestID: idA, generation: generationA, base: steerline.JournalCursor{}, batch: LogBatch{Activity: []steerline.ActivityEvent{{Kind: "message", Detail: "A old visit"}}, Cursor: steerline.JournalCursor{Path: "log-A", FileID: "a", Generation: 1, Offset: 1, Initialized: true}}}
	mm, cmd = m.Update(oldA)
	m = mm.(Model)
	if len(m.log) != 0 || cmd == nil || m.inFlightLogs[keyA] == idA {
		t.Fatal("returning to A must discard the old visit then schedule a fresh read")
	}
	freshA := pendingLogMessage(m, LogBatch{Activity: []steerline.ActivityEvent{{Kind: "message", Detail: "A current"}}, Cursor: steerline.JournalCursor{Path: "log-A", FileID: "a", Generation: 1, Offset: 1, Initialized: true}})
	mm, _ = m.Update(freshA)
	m = mm.(Model)
	if len(m.log) != 1 || m.log[0].Detail != "A current" {
		t.Fatalf("valid A response lost or mismatched: %+v", m.log)
	}
}

func TestViewerRetainsLastGoodRuntimeOnReadError(t *testing.T) {
	run := &steerline.Run{RunID: "R", TaskID: "task-R", ExecutionStatus: "running", LogPath: strptr("log")}
	good := observationFor(run)
	f := &fakeSource{observations: []steerline.ObservationSnapshot{good, {State: steerline.SnapshotReadFailed, SampledAt: time.Now(), Error: "runtime unreadable"}}}
	m := NewModel(steerline.Config{}, f, Options{})
	m.width, m.height = 100, 30
	mm, _ := m.Update(runsMsg{observation: f.Snapshot()})
	m = mm.(Model)
	sampled := m.observation.SampledAt
	mm, _ = m.Update(runsMsg{observation: f.Snapshot()})
	m = mm.(Model)
	if m.current() == nil || m.current().RunID != "R" || m.rt == nil {
		t.Fatalf("last good runtime disappeared: %+v", m)
	}
	if !m.observation.SampledAt.Equal(sampled) || m.observation.State != steerline.SnapshotReadFailed || !strings.Contains(m.View(), "snapshot error") {
		t.Fatalf("stale view was not marked with its original sample time: %+v", m.observation)
	}
}

func TestViewerClearsLogWhenSelectedRunHasNoLog(t *testing.T) {
	runA := &steerline.Run{RunID: "A", TaskID: "task-A", ExecutionStatus: "running", LogPath: strptr("log-A")}
	runB := &steerline.Run{RunID: "B", TaskID: "task-B", ExecutionStatus: "running"}
	f := &fakeSource{runs: []*steerline.Run{runA, runB}}
	m := NewModel(steerline.Config{}, f, Options{RunID: "A"})
	m.width, m.height = 100, 30
	mm, _ := m.Update(runsMsg{observation: observationFor(runA, runB)})
	m = mm.(Model)
	msg := pendingLogMessage(m, LogBatch{Activity: []steerline.ActivityEvent{{Kind: "message", Detail: "A log"}}, Reset: true, Cursor: steerline.JournalCursor{Path: "activity", FileID: "f", Generation: 1, Initialized: true}})
	mm, _ = m.Update(msg)
	m = mm.(Model)
	mm, _ = m.move(1)
	m = mm.(Model)
	if len(m.log) != 0 || !strings.Contains(m.View(), "selected run has no log file") {
		t.Fatalf("old log leaked into run without log: %s", m.View())
	}
}

func TestViewerBoundsHistoryAndKeepsScrolledAnchorWhileFollowing(t *testing.T) {
	run := &steerline.Run{RunID: "R", TaskID: "task-R", ExecutionStatus: "running", LogPath: strptr("log")}
	f := &fakeSource{runs: []*steerline.Run{run}}
	m := NewModel(steerline.Config{}, f, Options{RunID: "R"})
	m.width, m.height = 100, 30
	mm, _ := m.Update(runsMsg{observation: observationFor(run)})
	m = mm.(Model)
	events := make([]steerline.ActivityEvent, maxLogLines+100)
	for i := range events {
		events[i] = steerline.ActivityEvent{Kind: "note", Title: "line", Seq: uint64(i + 1)}
	}
	initial := pendingLogMessage(m, LogBatch{Activity: events, Reset: true, Cursor: steerline.JournalCursor{Path: "activity", FileID: "f", Generation: 1, Offset: 1, Initialized: true}})
	mm, _ = m.Update(initial)
	m = mm.(Model)
	if len(m.log) != maxLogLines || m.log[len(m.log)-1].Seq != uint64(len(events)) {
		t.Fatalf("viewer history not bounded to newest %d: len=%d tail=%d", maxLogLines, len(m.log), m.log[len(m.log)-1].Seq)
	}
	m.logScroll = 5
	m.autoScroll = false
	m.inFlightLogs[m.activeLogKey] = 2
	base := m.logCursor
	nextCursor := base
	nextCursor.Offset++
	msg := logMsg{runID: "R", logPath: "log", key: m.activeLogKey, requestID: 2, generation: m.logGeneration, base: base, batch: LogBatch{Activity: []steerline.ActivityEvent{{Kind: "note", Title: "new line", Seq: uint64(len(events) + 1)}}, Cursor: nextCursor}}
	mm, _ = m.Update(msg)
	m = mm.(Model)
	if m.logScroll != 6 || m.autoScroll {
		t.Fatalf("new events moved a scrolled view instead of preserving its anchor: scroll=%d follow=%v", m.logScroll, m.autoScroll)
	}
}

func TestViewerRevisitRejectsResponseFromEarlierVisit(t *testing.T) {
	a := &steerline.Run{RunID: "A", TaskID: "A", LogPath: strptr("A.log")}
	b := &steerline.Run{RunID: "B", TaskID: "B", LogPath: strptr("B.log")}
	m := newTestModel(&fakeSource{runs: []*steerline.Run{a, b}})
	old := pendingLogMessage(m, LogBatch{Activity: []steerline.ActivityEvent{{Kind: "note", Title: "old visit"}}})
	mm, _ := m.move(1)
	m = mm.(Model)
	mm, _ = m.move(-1)
	m = mm.(Model)
	mm, cmd := m.Update(old)
	m = mm.(Model)
	if len(m.log) != 0 || cmd == nil {
		t.Fatal("old visit response was accepted or replacement read was not scheduled")
	}
}

func TestViewerScrollAnchorSurvivesUpdateAboveViewport(t *testing.T) {
	m := newTestModel(&fakeSource{runs: []*steerline.Run{{RunID: "R", TaskID: "R", LogPath: strptr("R.log")}}})
	m.log = []steerline.ActivityEvent{{Seq: 1, Kind: "command", ItemID: "cmd", Status: "running", Title: "build", Detail: "one"}}
	for i := 2; i < 25; i++ {
		m.log = append(m.log, steerline.ActivityEvent{Seq: uint64(i), Kind: "note", Title: fmt.Sprintf("note-%d", i)})
	}
	m.autoScroll, m.logScroll = false, 5
	_, width := paneWidths(m.width)
	before := m.viewLog(width, 8)
	msg := pendingLogMessage(m, LogBatch{Activity: []steerline.ActivityEvent{{Seq: 25, Kind: "command", ItemID: "cmd", Status: "completed", Title: "build", Detail: "one\ntwo\nthree"}}})
	mm, _ := m.Update(msg)
	m = mm.(Model)
	after := m.viewLog(width, 8)
	if before[len(before)-1] != after[len(after)-1] {
		t.Fatalf("folded update above viewport moved anchor: before=%q after=%q", before, after)
	}
}

func TestViewerMarksMissingLogAsReadFailure(t *testing.T) {
	m := newTestModel(&fakeSource{runs: []*steerline.Run{{RunID: "R", TaskID: "R", LogPath: strptr("R.log")}}})
	m.log = []steerline.ActivityEvent{{Seq: 1, Kind: "note", Title: "cached"}}
	msg := pendingLogMessage(m, LogBatch{})
	msg.err = os.ErrNotExist
	mm, _ := m.Update(msg)
	m = mm.(Model)
	if m.logError == "" || len(m.log) != 1 {
		t.Fatal("missing source must label the cached log as unreadable")
	}
}

func TestViewerSerializesSnapshotReloads(t *testing.T) {
	m := newTestModel(&fakeSource{})
	mm, first := m.Update(key('r'))
	m = mm.(Model)
	if first == nil {
		t.Fatal("first reload was not scheduled")
	}
	mm, second := m.Update(key('r'))
	m = mm.(Model)
	if second != nil {
		t.Fatal("a second snapshot read could finish before and be overwritten by the first")
	}
	mm, _ = m.Update(first())
	m = mm.(Model)
	_, next := m.Update(key('r'))
	if next == nil {
		t.Fatal("completed snapshot read did not release its slot")
	}
}
