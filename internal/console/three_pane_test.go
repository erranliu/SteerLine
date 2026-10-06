package console

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestPS1ThreePaneInlineFocusAndQuestionChanges(t *testing.T) {
	m, c := ccFixture(t, 2)
	m.width, m.height = 160, 40
	for _, want := range []string{"任务列表", "候选与执行", "运行与反馈"} {
		if !strings.Contains(m.View(), want) {
			t.Fatal("missing pane", want, m.View())
		}
	}
	m, _ = ccKey(m, runeKey('a'))
	if m.answer == nil || m.detailExpanded || m.projects[0].focus != 1 || !strings.Contains(m.View(), "任务列表") {
		t.Fatal("editor left main layout")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.projects[0].focus != 2 {
		t.Fatal("Tab did not enter text")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("中文备注")})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("第二行🧭")})
	if string(m.answer.note) != "中文备注\n第二行🧭" || c.decisionCalls != 0 {
		t.Fatal("Enter submitted or lost Unicode")
	}
	oldIdentity, oldDecision := m.answer.identity, m.answer.decisionID
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF7})
	if m.answer != nil || m.projects[0].view.Drafts[oldIdentity+"\x00"+oldDecision].Note != "中文备注\n第二行🧭" {
		t.Fatal("switch lost or retargeted draft")
	}
	m, _ = ccKey(m, runeKey('a'))
	if len(m.answer.note) != 0 || m.answer.runID != "candidate-1" {
		t.Fatal("A draft reached B")
	}
	frozen := m.answer.request.Question
	run := m.selectedRow(0).run
	run.Decision.Version++
	run.Decision.Request.Question = "新版问题"
	m.syncAnswer()
	if m.answer.request.Question != frozen || m.answer.error == "" {
		t.Fatal("changed question silently rebound")
	}
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	if cmd != nil || c.decisionCalls != 0 || !strings.Contains(m.answer.error, "stale_revision") {
		t.Fatal("stale editor submitted")
	}
	run.Decision.ID = "new-question"
	m.syncAnswer()
	if m.answer != nil {
		t.Fatal("old editor hid new question")
	}
	m, _ = ccKey(m, runeKey('a'))
	if m.answer == nil || m.answer.decisionID != "new-question" || len(m.answer.note) != 0 {
		t.Fatal("new question inherited old answer")
	}
}

func TestPS1InlineListFocusPreservesDraftAndSkipsMissingOptions(t *testing.T) {
	m, _ := ccFixture(t, 1)
	run := m.selectedRow(0).run
	m.projects[0].snapshot.NativeTasks.Tasks = append(m.projects[0].snapshot.NativeTasks.Tasks, api.TaskDetail{TaskSummary: api.TaskSummary{TaskID: "39", Status: "open", Title: "后继任务"}})
	run.Decision.Request.Options = nil
	m, _ = ccKey(m, runeKey('a'))
	if m.projects[0].focus != 2 {
		t.Fatal("free text did not focus input")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("待保存草稿")})
	id := m.answer.identity + "\x00" + m.answer.decisionID
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.projects[0].focus != 0 || m.answer == nil {
		t.Fatal("text/list focus loop wrong")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.answer != nil || m.selectedRow(0).taskID != "39" || m.projects[0].view.Drafts[id].Note != "待保存草稿" {
		t.Fatal("list change mixed editor target")
	}
}

func TestPS1RecentActivityIsBoundedAndAttemptBound(t *testing.T) {
	m, _ := ccFixture(t, 2)
	m, _ = m.refreshRecentActivity(0)
	s := m.projects[0]
	run := m.selectedRow(0).run
	batch := api.LogBatch{Cursor: api.ActivityCursor{RunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, Initialized: true}}
	for i := 0; i < 30; i++ {
		batch.Activity = append(batch.Activity, api.ActivityEvent{Seq: uint64(i + 1), TS: time.Now(), Title: fmt.Sprintf("活动-%d", i), Detail: strings.Repeat("工具正文", 1000), Meta: map[string]any{"tool_body": "不应保留"}})
	}
	old := recentActivityMsg{s.ref.Key, s.ref.Identity, s.activityKey, s.activityGeneration, batch, nil}
	next, _ := m.Update(old)
	m = next.(Model)
	if len(m.projects[0].recentActivity) != 10 || m.projects[0].recentActivity[0].Title != "活动-20" || len([]rune(m.projects[0].recentActivity[0].Detail)) > 161 {
		t.Fatal("buffer not short/bounded")
	}
	if strings.Contains(m.View(), "活动-20") || !strings.Contains(m.View(), "问题：") {
		t.Fatal("activity displaced pending question")
	}
	run.AttemptID = "next-attempt"
	m, _ = m.refreshRecentActivity(0)
	next, _ = m.Update(old)
	m = next.(Model)
	if len(m.projects[0].recentActivity) != 0 {
		t.Fatal("old Attempt response reached new buffer")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF7})
	next, _ = m.Update(old)
	m = next.(Model)
	if len(m.projects[0].recentActivity) != 0 {
		t.Fatal("old candidate response crossed")
	}
}

func TestPS1InlineEditorFitsNarrowMainSurface(t *testing.T) {
	for _, size := range [][2]int{{55, 22}, {80, 24}, {160, 40}} {
		m, _ := ccFixture(t, 2)
		m.width, m.height = size[0], size[1]
		m, _ = ccKey(m, runeKey('a'))
		m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
		m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("中文第一行\n第二行")})
		view := m.View()
		for _, want := range []string{"输入焦点", "提交预览", "F2", "中文"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%dx%d lost %s: %s", size[0], size[1], want, view)
			}
		}
		if m.detailExpanded || len(strings.Split(view, "\n")) > m.height {
			t.Fatal("editor changed view or overflowed height")
		}
	}
}

func TestPS1InlineModalCannotSubmitHiddenDraft(t *testing.T) {
	m, c := ccFixture(t, 1)
	m, _ = ccKey(m, runeKey('a'))
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("隐藏前保存")})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.projects[0].focus != 0 {
		t.Fatal("not in list focus")
	}
	m, _ = ccKey(m, runeKey('f'))
	if m.answer != nil || !m.detailExpanded {
		t.Fatal("reader retained hidden editor")
	}
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	if cmd != nil || c.decisionCalls != 0 {
		t.Fatal("F2 submitted an invisible draft")
	}
}
