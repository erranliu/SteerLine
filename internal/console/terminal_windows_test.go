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
