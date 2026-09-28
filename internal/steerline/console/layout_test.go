package console

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/erranliu/SteerLine/internal/steerline"
)

func TestTaskSelectionAndTwoFocusRegions(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	snapshot := readySnapshot(ref, now)
	snapshot.Tasks.Tasks = append(snapshot.Tasks.Tasks, steerline.TaskCandidate{TaskID: "task-C", Title: "Build C"})
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.width, m.height = 120, 40
	view := m.View()
	for _, want := range []string{"任务列表 [焦点]", "▸", "详情", "任务动态 · 活动", "────────"} {
		if !strings.Contains(view, want) {
			t.Fatalf("stacked layout lost %q:\n%s", want, view)
		}
	}
	selected := m.projects[0].selectedKey
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.projects[0].focus != 1 || !strings.Contains(m.View(), "任务动态 [焦点]") || !strings.Contains(m.View(), "▸") {
		t.Fatal("activity focus hid the selected task")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.projects[0].selectedKey != selected {
		t.Fatal("activity scrolling changed the selected task")
	}
	if !m.projects[0].view.Follow {
		t.Fatal("scrolling an empty activity pane disabled follow mode")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.projects[0].focus != 0 || m.projects[0].selectedKey == selected {
		t.Fatal("list focus did not move task selection")
	}
}

func TestProjectSwitchKeepsCurrentIdentityVisibleAndBackgroundNoticeIsolated(t *testing.T) {
	now := time.Date(2026, 9, 26, 1, 0, 0, 0, time.UTC)
	refs := []ProjectRef{testProject("a-long-project-name"), testProject("SteerlineTest"), testProject("merge-h-308-code"), testProject("slot2")}
	snapshots := make([]ProjectSnapshot, len(refs))
	for i := range refs {
		snapshots[i] = readySnapshot(refs[i], now)
	}
	m := modelForTest(refs, snapshots, &testControlClient{}, now)
	m.width = 80
	for i := range refs {
		strip := renderProjectStrip(m)
		if !strings.Contains(strip, refs[i].Name) || !strings.Contains(strip, "项目 "+string(rune('1'+i))+"/4") {
			t.Fatalf("current project lost after switch %d: %q", i, strip)
		}
		if i < len(refs)-1 {
			updated, _ := m.moveProject(1)
			m = updated.(Model)
		}
	}
	m.projects[0].selectedKey = "task:removed"
	m.projects[0].refreshID = 9
	m.status = ""
	updated, _ := m.Update(projectMsg{key: refs[0].Key, identity: refs[0].Identity, requestID: 9, snapshot: readySnapshot(refs[0], now)})
	m = updated.(Model)
	if m.selected != 3 || m.status != "" || !strings.Contains(renderProjectStrip(m), "slot2") {
		t.Fatalf("background refresh changed active project: selected=%d status=%q strip=%q", m.selected, m.status, renderProjectStrip(m))
	}
}

func TestColumnsAlignWithChineseAndLongTaskTitles(t *testing.T) {
	left := []string{"中文", styleSelected.Render("短")}
	right := []string{"详情 A", "详情 B"}
	for _, line := range strings.Split(joinLines(left, right, 24, 2), "\n") {
		before, _, ok := strings.Cut(line, "│")
		if !ok || lipgloss.Width(before) != 24 {
			t.Fatalf("detail column drifted: %q", line)
		}
	}
	for _, width := range []int{34, 80} {
		row := renderTaskListRow("n308-28", strings.Repeat("中文任务", 20), "blocked", width)
		if lipgloss.Width(row) > width || !strings.Contains(row, "blocked") || !strings.Contains(row, "n308-28") {
			t.Fatalf("task row lost state or identity at %d columns: %q", width, row)
		}
	}
}

func TestNativeTaskDetailKeepsBlockersWithoutPlanOrHashes(t *testing.T) {
	task := steerline.NativeTaskDetail{
		TaskRecord:   steerline.TaskRecord{TaskID: "12", Title: "中文任务", Status: "open"},
		Predecessors: []steerline.TaskDependencyStatus{{TaskID: "9", Status: "discarded", Blocker: "discarded_predecessor"}},
	}
	task.Contract.SpecRef.Path = "docs/spec/task.spec.md"
	task.Contract.SpecRef.SHA256 = "secret-document-hash"
	task.Contract.PlanRef.Path = "docs/spec/task.plan.md"
	detail := strings.Join(renderNativeTaskDetail(projectState{}, task), "\n")
	for _, want := range []string{"中文任务", "discarded_predecessor", "docs/spec/task.spec.md"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail lost %q: %q", want, detail)
		}
	}
	for _, hidden := range []string{"task.plan.md", "secret-document-hash", "sha256", "version 0"} {
		if strings.Contains(detail, hidden) {
			t.Fatalf("detail exposed %q: %q", hidden, detail)
		}
	}
}

func TestBackgroundDecisionRejectionDoesNotOpenAnotherProjectsDraft(t *testing.T) {
	now := time.Now().UTC()
	a, b := testProject("a"), testProject("b")
	m := modelForTest([]ProjectRef{a, b}, []ProjectSnapshot{readySnapshot(a, now), readySnapshot(b, now)}, &testControlClient{}, now)
	m.projects[0].actions["decision-req"] = &actionState{RequestID: "decision-req", Action: "answer_decision", RunID: "run-a", DecisionID: "decision-a"}
	m.selected = 1
	updated, _ := m.Update(actionSubmittedMsg{projectKey: a.Key, identity: a.Identity, requestID: "decision-req", err: errors.New("stale_revision")})
	m = updated.(Model)
	if m.answer != nil || m.status != "" || m.selected != 1 || m.projects[0].actions["decision-req"].Status != "rejected" {
		t.Fatalf("background answer changed current project UI: selected=%d status=%q dialog=%+v", m.selected, m.status, m.answer)
	}
}

func TestHelpAndCompactLayoutFitNarrowAndWideScreens(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, &testControlClient{}, now)
	for _, width := range []int{80, 120} {
		m.width, m.height = width, 24
		for _, help := range []bool{false, true} {
			m.help = help
			view := m.View()
			lines := strings.Split(view, "\n")
			if len(lines) != 24 {
				t.Fatalf("view has %d lines at width %d", len(lines), width)
			}
			for _, line := range lines {
				if lipgloss.Width(line) > width {
					t.Fatalf("view exceeds %d columns: %q", width, line)
				}
			}
		}
	}
}
