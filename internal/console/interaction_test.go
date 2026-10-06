package console

import (
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestUX1ParallelNumbersAndPrimarySurface(t *testing.T) {
	m, _ := ccFixture(t, 5)
	m.rows(0)
	view := m.View()
	for _, want := range []string{"并行 5 · 待交互 5 · 当前 P1", "F6/F7", "[P1] 待交互"} {
		if !strings.Contains(view, want) {
			t.Fatal(want, view)
		}
	}
	for _, hidden := range []string{"execution-0", "candidate-0", "decision-0", "候选版本："} {
		if strings.Contains(view, hidden) {
			t.Fatal("technical identity crowded primary view", hidden)
		}
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF7})
	if m.selectedRow(0).run.RunID != "candidate-1" || !strings.Contains(m.View(), "当前 P2") {
		t.Fatal("native function-key navigation lost binding")
	}
	runs := m.projects[0].snapshot.Observation.Page.Runs
	runs[0], runs[4] = runs[4], runs[0]
	if candidateLabel(m.projects[0], m.selectedRow(0).run) != "P2" {
		t.Fatal("reorder renamed selected parallel")
	}
	data, err := json.Marshal(m.projects[0].view)
	if err != nil {
		t.Fatal(err)
	}
	var restored ProjectView
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	m.projects[0].view = restored
	if candidateLabel(m.projects[0], m.selectedRow(0).run) != "P2" {
		t.Fatal("reopen renamed parallel")
	}
	newRun := *runs[0]
	newRun.RunID = "late-added"
	m.projects[0].snapshot.Observation.Page.Runs = append(runs, &newRun)
	if candidateLabel(m.projects[0], &newRun) != "P6" {
		t.Fatal("display number implied five fixed slots")
	}
}

func TestUX5TaskTriggerOrderAcrossWindows(t *testing.T) {
	m, _ := ccFixture(t, 3)
	runs := m.projects[0].snapshot.Observation.Page.Runs
	at := time.Date(2026, 10, 4, 1, 0, 0, 0, time.UTC)
	// IDs deliberately disagree with trigger order; an exited candidate still
	// owns P2 even when only the third candidate has a projected visible run.
	history := []api.WorkflowSummary{
		{TaskID: "38", WorkflowRunID: "z-first", CreatedAt: at},
		{TaskID: "38", WorkflowRunID: "b-exited", CreatedAt: at.Add(time.Second)},
		{TaskID: "38", WorkflowRunID: "a-third", CreatedAt: at.Add(2 * time.Second)},
		{TaskID: "other", WorkflowRunID: "unrelated", CreatedAt: at.Add(-time.Second)},
	}
	m.projects[0].snapshot.History = &api.HistorySummary{WorkflowRuns: history}
	runs[2].RunID = "a-third"
	m.projects[0].snapshot.Observation.Page.Runs = runs[2:]
	if candidateLabel(m.projects[0], runs[2]) != "P3" {
		t.Fatal("visible subset renumbered complete task history")
	}
	secondWindow := m.projects[0]
	secondWindow.view = ProjectView{}
	if candidateLabel(secondWindow, runs[2]) != "P3" {
		t.Fatal("new window has a different task number")
	}
	history[0], history[2] = history[2], history[0]
	if candidateLabel(secondWindow, runs[2]) != "P3" {
		t.Fatal("query reorder changed trigger number")
	}
	history = append(history, api.WorkflowSummary{TaskID: "38", WorkflowRunID: "later", CreatedAt: at.Add(3 * time.Second)})
	secondWindow.snapshot.History = &api.HistorySummary{WorkflowRuns: history}
	if candidateLabel(secondWindow, runs[2]) != "P3" {
		t.Fatal("later trigger changed earlier number")
	}
	if candidateLabel(secondWindow, &api.CandidateSummary{TaskID: "38", RunID: "b-exited"}) != "P2" {
		t.Fatal("exited history did not retain its number")
	}
	// Equal timestamps use the immutable workflow ID as a common tie-breaker.
	history[0].CreatedAt = at
	history[1].CreatedAt = at
	first := candidateLabel(secondWindow, runs[2])
	history[0], history[1] = history[1], history[0]
	if candidateLabel(secondWindow, runs[2]) != first {
		t.Fatal("equal timestamp numbering depended on delivery order")
	}
}

func TestUX2PreviewSubmitEndsCurrentHumanInteraction(t *testing.T) {
	m, c := ccFixture(t, 5)
	m.rows(0)
	run := m.selectedRow(0).run
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.answer == nil {
		t.Fatal("Enter did not open current interaction")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("候选比较备注")})
	view := m.View()
	for _, want := range []string{"session session-0", "提交预览 → P1", "选择：选项 11", "内容：候选比较备注"} {
		if !strings.Contains(view, want) {
			t.Fatal(want, view)
		}
	}
	for _, hidden := range []string{run.RunID, run.ExecutionID, run.Decision.ID, "背景第二行"} {
		if strings.Contains(view, hidden) {
			t.Fatal("default editor showed technical background", hidden)
		}
	}
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	if cmd == nil || m.answer != nil || m.projects[0].focus != 1 {
		t.Fatal("F2 did not return to original session")
	}
	if !strings.Contains(m.View(), "待交互 4") || run.Decision.Status != "pending" {
		t.Fatal("UI submission count rewrote business Decision or retained current input prompt")
	}
	m, _ = ccKey(m, runeKey('a'))
	if m.answer != nil {
		t.Fatal("old question opened duplicate submission")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if c.decisionAnswer.RunID != run.RunID || c.decisionAnswer.OptionID != "o10" || c.decisionAnswer.Note != "候选比较备注" {
		t.Fatal(c.decisionAnswer)
	}
	// A newly committed question is a new human interaction, not the old draft.
	d := *run.Decision
	d.ID = "next-question"
	run.Decision = &d
	if !strings.Contains(m.View(), "待交互 5") {
		t.Fatal("new question was hidden by previous receipt")
	}
	m, _ = ccKey(m, runeKey('a'))
	if m.answer == nil || len(m.answer.note) != 0 {
		t.Fatal("next question reused prior draft")
	}
}

func TestUX3FreeInputAndNarrowPinnedPreview(t *testing.T) {
	m, _ := ccFixture(t, 1)
	run := m.selectedRow(0).run
	run.Decision.Request.Options = nil
	run.Decision.Request.Recommended = ""
	m, _ = ccKey(m, runeKey('a'))
	if m.answer == nil || !m.answer.noteFocus {
		t.Fatal("free input required an invisible option/Tab step")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("直接输入中文答复")})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 50, Height: 24})
	m = next.(Model)
	view := m.View()
	for _, want := range []string{"无固定选项", "提交预览 → P1", "选择：自由输入", "内容：直接输入中文答复", "F2 提交"} {
		if !strings.Contains(view, want) {
			t.Fatal(want, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 50 {
			t.Fatal("narrow renderer exceeded actual viewport", line)
		}
	}
	if len(strings.Split(view, "\n")) > 24 {
		t.Fatal("preview was pushed outside viewport")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF8})
	if !strings.Contains(m.View(), "run_id：") {
		t.Fatal("full binding disappeared from detail entry")
	}
}

func TestUX4RejectPreservesDraftAndBlockerStaysIndependent(t *testing.T) {
	m, c := ccFixture(t, 2)
	run := m.selectedRow(0).run
	m, _ = ccKey(m, runeKey('a'))
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("不要自动补投")})
	run.EnvironmentBlocker = &api.EnvironmentBlocker{ID: "original", Version: 2, Fact: api.EnvironmentFact{Reason: "原终端不可输入"}}
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	if cmd != nil || m.answer == nil || c.decisionCalls != 0 || run.Decision.Status != "pending" {
		t.Fatal("blocker was treated as an accepted response")
	}
	if !strings.Contains(m.View(), "提交预览") || !strings.Contains(m.View(), "未提交：") {
		t.Fatal("blocked editor lost preview/reason")
	}
	run.EnvironmentBlocker = nil
	if c.decisionCalls != 0 {
		t.Fatal("environment ready automatically submitted draft")
	}
}
