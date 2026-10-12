package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	app "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/console"
	codex "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestPS1InlineConsoleWiring(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	f := newLifecycleProject(t, "local")
	var profiles []any
	for i := 0; i < 2; i++ {
		payload, err := codex.EncodeProfileConfig(codex.ProfileConfig{Executable: os.Args[0], Model: fmt.Sprintf("fixture-%d", i)})
		require(t, err)
		profiles = append(profiles, map[string]any{"id": fmt.Sprintf("p%d", i), "name": fmt.Sprintf("方案%d", i), "host": "local", "agent": "codex", "implementation_configs": []backend.ImplementationConfigInput{{ImplementationKey: codex.ProfileCodecKey, CodecVersion: 2, Payload: payload}}})
	}
	require(t, os.WriteFile(app.GlobalExecutionConfigPath(), mustJSON(map[string]any{"version": 2, "default_execution_profile": "p0", "execution_profiles": profiles}), 0600))
	var err error
	f.config, err = app.LoadConfig(f.configPath)
	require(t, err)
	createIntegrationTask(t, ctx, f, 901)
	svc := startIntegrationService(t, ctx, f.config, &integrationLifecycleResolver{})
	defer svc.stop(t)
	client, err := app.OpenClient(ctx, f.configPath)
	require(t, err)
	defer client.Close()
	ref := client.Project()
	m, err := console.NewModel(ctx, []api.ProjectRef{ref}, console.Options{Clients: map[string]console.ConsoleClient{ref.Key: client}, ApplicationHome: app.HomeDir()})
	require(t, err)
	defer m.Close()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = next.(console.Model)
	var drive func(tea.Cmd)
	drive = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, child := range batch {
				drive(child)
			}
			return
		}
		next, follow := m.Update(msg)
		m = next.(console.Model)
		drive(follow)
	}
	step := func(key tea.KeyMsg) { next, cmd := m.Update(key); m = next.(console.Model); drive(cmd) }
	key := func(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }
	step(key('r'))
	step(key('s'))
	step(tea.KeyMsg{Type: tea.KeyEnter})
	var primary *api.CandidateSummary
	// Each candidate must finish both execute/report rounds before feedback.
	waitFor(t, 2*time.Minute, func() bool {
		page := client.ReadProject(ctx, "901", api.ProjectSnapshot{})
		if page.Observation.Page == nil {
			return false
		}
		for _, run := range page.Observation.Page.Runs {
			if run.SessionID != "" && run.Decision != nil {
				primary = run
				return true
			}
		}
		return false
	})
	step(key('r'))
	step(key('b'))
	step(tea.KeyMsg{Type: tea.KeyDown})
	step(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, 2*time.Minute, func() bool {
		page := client.ReadProject(ctx, "901", api.ProjectSnapshot{})
		if page.Observation.Page == nil || len(page.Observation.Page.Runs) != 2 {
			return false
		}
		for _, run := range page.Observation.Page.Runs {
			if run.SessionID == "" || run.Decision == nil {
				return false
			}
		}
		return true
	})
	step(key('r'))
	for _, want := range []string{"任务列表", "候选与执行", "运行与反馈"} {
		if !strings.Contains(m.View(), want) {
			t.Fatal("real app lost pane", want, m.View())
		}
	}
	batch, err := client.RecentActivity(ctx, primary.RunID, primary.ExecutionID, primary.AttemptID)
	require(t, err)
	if len(batch.Activity) > 10 || batch.Cursor.ExecutionID != primary.ExecutionID || batch.Cursor.AttemptID != primary.AttemptID {
		t.Fatal("recent activity lost original scope", batch)
	}
	step(key('a'))
	step(tea.KeyMsg{Type: tea.KeyTab})
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("原地中文\n第二行")})
	if !strings.Contains(m.View(), "任务列表") || !strings.Contains(m.View(), "▶ ┃ 你的答复") {
		t.Fatal("editor replaced main view", m.View())
	}
	step(tea.KeyMsg{Type: tea.KeyF7})
	step(key('a'))
	if strings.Contains(m.View(), "原地中文") {
		t.Fatal("draft crossed into sibling")
	}
	step(tea.KeyMsg{Type: tea.KeyF6})
	step(key('a'))
	if !strings.Contains(m.View(), "原地中文") {
		t.Fatal("original draft not restored", m.View())
	}
	// Explicitly refresh/review the same unsent draft after the second worker's
	// preparation settles; never retry a request whose outcome is unknown.
	step(tea.KeyMsg{Type: tea.KeyEsc})
	step(key('r'))
	step(key('a'))
	if !strings.Contains(m.View(), "原地中文") {
		t.Fatal("refresh lost original draft", m.View())
	}
	step(tea.KeyMsg{Type: tea.KeyF2})
	t.Logf("inline submit surface: %s", m.View())
	if strings.Contains(m.View(), "stale_revision") || strings.Contains(m.View(), "stale_instance") || strings.Contains(m.View(), "答案命令结果待确认") {
		t.Fatal("inline request not admitted", m.View())
	}
	// The original feedback turn must be observable under Full's process load;
	// keep this bounded independently of the five-minute overall fixture.
	waitFor(t, time.Minute, func() bool {
		page := client.ReadProject(ctx, "901", api.ProjectSnapshot{})
		if page.Observation.Page == nil {
			return false
		}
		for _, run := range page.Observation.Page.Runs {
			if run.RunID == primary.RunID && run.TurnNumber > primary.TurnNumber {
				return true
			}
		}
		return false
	})
	raw, err := json.Marshal(client.ReadProject(ctx, "901", api.ProjectSnapshot{}).Observation.Page.Runs)
	require(t, err)
	t.Logf("real app inline answer resumed the exact original candidate; two candidates retained: %s", raw)
}
