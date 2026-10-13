package console

import (
	"encoding/json"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestHTTPStartWaitsForSelectedTaskVersion(t *testing.T) {
	ref := testProject("version-loading")
	snapshot := readySnapshot(ref, time.Now())
	snapshot.SelectedNative = nil
	control := &testControlClient{}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, control, time.Now())
	m.profile = &viewProfile{dir: t.TempDir()}
	m.projects[0].selectedKey = "task:task-B"
	next, refresh := m.startSelected()
	m = next.(Model)
	if refresh == nil || m.profileChoice != nil || len(m.projects[0].actions) != 0 {
		t.Fatal("start was offered before native task version loaded")
	}
	next, _ = m.createActionWithProfile("start_task", "task-B", &ref.Presentation.Profiles[0])
	m = next.(Model)
	if len(m.projects[0].actions) != 0 || control.calls != 0 {
		t.Fatal("invalid zero-version start was journaled or submitted")
	}
	m.projects[0].snapshot.SelectedNative = &api.TaskDetail{TaskSummary: api.TaskSummary{TaskID: "task-B", Status: "open", Version: 7}}
	next, _ = m.startSelected()
	m = next.(Model)
	if m.profileChoice == nil {
		t.Fatal("loaded task version did not enable profile selection")
	}
	next, submit := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if submit == nil {
		t.Fatal("confirmed versioned start was not submitted")
	}
	_ = submit()
	var payload api.ControlPayload
	if err := json.Unmarshal(control.request.Payload, &payload); err != nil || payload.ExpectedTaskVersion != 7 || payload.TaskID != "task-B" {
		t.Fatal("original loaded task version was not preserved", payload, err)
	}
}
