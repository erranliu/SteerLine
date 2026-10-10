package console

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

type nativeViewerTestClient struct {
	ConsoleClient
	target api.NativeSessionTarget
	calls  int
	err    error
}

func (c *nativeViewerTestClient) OpenNativeSession(_ context.Context, target api.NativeSessionTarget) error {
	c.target = target
	c.calls++
	return c.err
}

func TestNativeSessionShortcutOpensSelectedCandidateAndReportsFailure(t *testing.T) {
	now := time.Now()
	ref := testProject("native")
	snapshot, run := decisionSnapshot(ref, now)
	run.SessionID, run.SessionGeneration = "original-session", "original-generation"
	run.CurrentTurnID = "original-turn"
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.projects[0].selectedKey = "task:" + run.TaskID
	client := &nativeViewerTestClient{ConsoleClient: m.projects[0].client}
	m.projects[0].client = client
	updated, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("shortcut did not open native viewer")
	}
	if _, duplicate := m.openNativeSession(); duplicate != nil {
		t.Fatal("double key launched another in-flight viewer")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if client.calls != 1 || client.target.RunID != run.RunID || client.target.SessionID != run.SessionID || m.projects[0].nativeSessionOpening || !strings.Contains(m.status, "已打开 Codex CLI") || len(m.projects[0].actions) != 0 {
		t.Fatalf("viewer changed task controls or lost its candidate: %+v / %s", client.target, m.status)
	}
	client.err = errors.New("original server exited")
	updated, cmd = m.openNativeSession()
	updated, _ = updated.(Model).Update(cmd())
	if !strings.Contains(updated.(Model).status, "original server exited") {
		t.Fatal("failure hidden")
	}
	run.CurrentTurnID = ""
	if _, cmd := updated.(Model).openNativeSession(); cmd != nil {
		t.Fatal("empty native history opened")
	}
	run.CurrentTurnID = "original-turn"
	run.Host = "orca"
	if _, cmd := updated.(Model).openNativeSession(); cmd != nil {
		t.Fatal("nonlocal task opened")
	}
	run.Host = "orca_supervised"
	run.Agent = "claude"
	client.err = nil
	updated, cmd = updated.(Model).openNativeSession()
	if cmd == nil {
		t.Fatal("Orca candidate could not open its native viewer")
	}
	updated, _ = updated.(Model).Update(cmd())
	if !strings.Contains(updated.(Model).status, "已打开 Orca") {
		t.Fatal("Orca viewer reported Codex")
	}
}
