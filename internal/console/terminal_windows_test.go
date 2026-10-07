//go:build windows && ps1terminal

package console

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// This opt-in fixture runs the actual Bubble Tea program in an isolated Windows
// terminal. It injects product DTOs only; no worker, credential or PR is created.
func TestPS1WindowsTerminalInput(t *testing.T) {
	path := os.Getenv("STEERLINE_PS1_TERMINAL_CAPTURE")
	if path == "" {
		t.Fatal("explicit Windows terminal acceptance requires STEERLINE_PS1_TERMINAL_CAPTURE")
	}
	m, control := ccFixture(t, 2)
	m.clock = realClock{}
	query := m.projects[0].client.(*testClient).query
	query.snapshot = m.projects[0].snapshot
	query.bodies = map[string]string{"final": strings.Repeat("完整最终回复。中文示例。\n", 50) + "最终回复末尾"}
	query.page.Events = append(query.page.Events, finalFixture(m, 1, "business-turn", "final", query.bodies["final"]))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	m.ctx = ctx
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	end, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}
	result := end.(Model)
	record := map[string]any{"environment": "Windows native terminal with controlled product DTOs", "width": result.width, "height": result.height, "decision_calls": control.decisionCalls, "answer": control.decisionAnswer, "journal_format": viewFileVersion}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if control.decisionCalls != 1 || control.decisionAnswer.RunID != "candidate-0" || control.decisionAnswer.Note != "中文第一行\n第二行" {
		t.Fatalf("native terminal input did not reach exact original question: %+v", control.decisionAnswer)
	}
}

// This wrapper records actual Bubble Tea key events and rendered frames. It does
// not generate keys, call a worker or expose a test-only production API.
type cpwTerminalModel struct {
	Model
	keys   []string
	frames []map[string]any
}

func (m cpwTerminalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.Model.Update(msg)
	m.Model = next.(Model)
	if key, ok := msg.(tea.KeyMsg); ok {
		m.keys = append(m.keys, key.String())
		run := ""
		if row := m.selectedRow(m.selected); row != nil && row.run != nil {
			run = row.run.RunID
		}
		m.frames = append(m.frames, map[string]any{"key": key.String(), "focus": m.projects[m.selected].focus, "candidate": run, "reader": m.detailExpanded || m.operationDetails || m.answer != nil && m.answer.details, "view": m.View()})
	}
	return m, cmd
}

func TestCPWWindowsTerminalInput(t *testing.T) {
	path := os.Getenv("STEERLINE_CPW_TERMINAL_CAPTURE")
	if path == "" {
		t.Fatal("explicit terminal acceptance requires STEERLINE_CPW_TERMINAL_CAPTURE")
	}
	m, control := ccFixture(t, 2)
	query := m.projects[0].client.(*testClient).query
	query.snapshot = m.projects[0].snapshot
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	m.ctx = ctx
	p := tea.NewProgram(cpwTerminalModel{Model: m}, tea.WithAltScreen(), tea.WithContext(ctx))
	end, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}
	result := end.(cpwTerminalModel)
	record := map[string]any{"environment": "Windows ConPTY with isolated product DTO fixture", "width": result.width, "height": result.height, "keys": result.keys, "frames": result.frames, "control_calls": control.calls, "decision_calls": control.decisionCalls, "answer": control.decisionAnswer}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if control.calls != 0 || control.decisionCalls != 1 || control.decisionAnswer.RunID != "candidate-0" || control.decisionAnswer.Note != "中文第一行\n第二行" {
		t.Fatalf("terminal input lost original target/text: %+v", control.decisionAnswer)
	}
	for _, key := range []string{"tab", "shift+tab", "enter", "f2", "f6", "f7", "f8", "esc"} {
		found := false
		for _, seen := range result.keys {
			if seen == key {
				found = true
			}
		}
		if !found {
			t.Fatalf("native input did not exercise %s", key)
		}
	}
	hiddenSubmit := false
	for _, frame := range result.frames {
		if frame["key"] == "f2" && frame["reader"] == true {
			hiddenSubmit = true
		}
	}
	if !hiddenSubmit {
		t.Fatal("native F2 was not exercised inside F8 reading")
	}
}
