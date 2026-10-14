package console

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestCandidatePendingStartSurvivesJournalReopen(t *testing.T) {
	m, c := ccFixture(t, 1)
	home := t.TempDir()
	profile, prefs, diagnostic, err := acquireViewProfile(home)
	if err != nil || diagnostic != "" {
		t.Fatal(diagnostic, err)
	}
	m.profile, m.prefs = profile, prefs
	defer m.Close()
	m, _ = ccKey(m, runeKey('b'))
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal(m.status)
	}
	id := m.selectedRow(0).run.RunID
	ref, client := m.projects[0].ref, m.projects[0].client
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewModel(context.Background(), []ProjectRef{ref}, Options{ApplicationHome: home, Clients: map[string]ConsoleClient{ref.Key: client}})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	a := reopened.projects[0].actions[id]
	if a == nil || a.Resolved || a.CandidateNumber != 2 || a.Status != "indeterminate" || reopened.projects[0].view.Outstanding[id] != "task:38" || c.calls != 0 {
		t.Fatal("reopened start lost its original identity or resubmitted", a)
	}
}

func TestCandidateConfirmationSelectsIndependentPendingHistory(t *testing.T) {
	m, c := ccFixture(t, 1)
	m.projects[0].final.bodyText = "P1 old reply"
	m.projects[0].recentActivity = []activitySummary{{Title: "P1 old activity"}}
	var ids []string
	var cmd tea.Cmd
	for number := 2; number <= 5; number++ {
		m, _ = ccKey(m, runeKey('b'))
		m, cmd = ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
		row := m.selectedRow(0)
		if cmd == nil || row == nil || row.run == nil || row.run.CandidateNumber != number || row.run.Decision != nil {
			t.Fatalf("confirmation %d did not select its own pending candidate: %s", number, m.status)
		}
		ids = append(ids, row.run.RunID)
		if m.projects[0].final.bodyText != "" || len(m.projects[0].recentActivity) != 0 || strings.Contains(m.View(), "P1 old") {
			t.Fatal("old candidate content remained visible")
		}
		next, _ := m.Update(cmd())
		m = next.(Model)
		if c.request.RequestID != ids[len(ids)-1] {
			t.Fatal("submission changed original candidate identity")
		}
	}
	m, _ = ccKey(m, runeKey('b'))
	if m.profileChoice != nil || !strings.Contains(m.status, "5个") {
		t.Fatalf("pending starts did not count toward limit: status=%q count=%d choice=%+v answer=%+v", m.status, len(candidateRuns(m.projects[0], "38")), m.profileChoice, m.answer)
	}
	// The first receipt may arrive after the latest candidate is selected.
	id := ids[0]
	a := m.projects[0].actions[id]
	next, _ := m.Update(actionReceiptMsg{projectKey: "cc", identity: m.projects[0].ref.Identity, requestID: id,
		receipt: &api.ControlReceipt{RequestID: id, InstanceID: a.TargetInstance, Action: "start_candidate", Status: "failed", RunID: id, Message: "startup probe failed"}})
	m = next.(Model)
	if m.selectedRow(0).run.RunID != ids[len(ids)-1] {
		t.Fatal("late receipt stole selection")
	}
	if len(candidateRuns(m.projects[0], "38")) != 5 {
		t.Fatal("failed candidate disappeared")
	}
	m, _ = ccKey(m, runeKey('b'))
	m, cmd = ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || m.selectedRow(0).run.CandidateNumber != 6 || len(candidateRuns(m.projects[0], "38")) != 6 {
		t.Fatal("failure did not free a slot while retaining history")
	}
	// A duplicate network result cannot create a seventh candidate.
	next, _ = m.Update(actionReceiptMsg{projectKey: "cc", identity: m.projects[0].ref.Identity, requestID: id,
		receipt: &api.ControlReceipt{RequestID: id, InstanceID: a.TargetInstance, Action: "start_candidate", Status: "failed", RunID: id}})
	m = next.(Model)
	if len(candidateRuns(m.projects[0], "38")) != 6 || c.calls != 4 {
		t.Fatal("receipt replay allocated or submitted a replacement")
	}
}
