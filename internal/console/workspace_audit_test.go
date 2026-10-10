package console

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestCPWAuditClippedAnswerTargetCannotSend(t *testing.T) {
	m, c := ccFixture(t, 1)
	m.width, m.height = 40, 28
	task := "task-with-long-ID"
	m.selectedRow(0).run.TaskID = task
	m.projects[0].snapshot.NativeTasks.Tasks[0].TaskID = task
	m.projects[0].selectedKey = "task:" + task
	m, _ = ccKey(m, runeKey('a'))
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	if cmd != nil || c.decisionCalls != 0 || m.answer == nil {
		t.Fatal("submitted despite clipped original target")
	}
}

func TestCPWAuditRecoveryModeEndsWhenAnswerOpens(t *testing.T) {
	m, _ := ccFixture(t, 1)
	m, _ = ccKey(m, runeKey('u'))
	s := m.projects[0]
	r := m.selectedRow(0).run
	old := recoveryInspectionMsg{projectKey: s.ref.Key, identity: s.ref.Identity, runID: r.RunID, generation: s.recoveryGeneration, err: errors.New("old inspection failure")}
	m, _ = ccKey(m, runeKey('a'))
	if m.answer == nil || m.projects[0].recoveryActive {
		t.Fatal("answer retained inspection mode")
	}
	status := m.status
	next, _ := m.Update(old)
	m = next.(Model)
	if m.status != status || m.projects[0].recoveryError != "" {
		t.Fatal("old inspection displaced answer status")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	if m.projects[0].recoveryActive || strings.Contains(m.View(), "只读核验") {
		t.Fatal("submission returned to old report")
	}
}

func TestCPWAuditUnresolvedReceiptSurvivesNewTerminalReceipt(t *testing.T) {
	m, _ := ccFixture(t, 1)
	s := &m.projects[0]
	r := m.selectedRow(0).run
	s.actions["pending"] = &actionState{RequestID: "pending", TaskID: r.TaskID, RunID: r.RunID, Status: "indeterminate", Message: "原请求尚未核实", UpdatedAt: time.Now().Add(-time.Minute)}
	s.actions["new"] = &actionState{RequestID: "new", Action: "answer_decision", TaskID: r.TaskID, RunID: r.RunID, Status: "accepted", Message: "较新答案已保存", Resolved: true, UpdatedAt: time.Now(), DecisionDraftNote: "新预览"}
	text := strings.Join(receiptSummary(*s, m.selectedRow(0)), "\n")
	if !strings.Contains(text, "原请求尚未核实") {
		t.Fatal("terminal receipt hid unresolved request", text)
	}
	s.actions["new"].Status = "rejected"
	s.actions["failed"] = &actionState{RequestID: "failed", TaskID: r.TaskID, RunID: r.RunID, Status: "failed", Resolved: true, Message: "另一个已终态失败", UpdatedAt: time.Now().Add(time.Second)}
	text = strings.Join(receiptSummary(*s, m.selectedRow(0)), "\n")
	if !strings.Contains(text, "原请求尚未核实") {
		t.Fatal("resolved failures hid unresolved request", text)
	}
}

func TestCPWAuditIndexFailureInvalidatesFinalPreview(t *testing.T) {
	for _, kind := range []string{"error", "empty", "gap"} {
		t.Run(kind, func(t *testing.T) {
			m, _ := ccFixture(t, 1)
			r := m.selectedRow(0).run
			r.Decision = nil
			r.CurrentTurnID = "turn"
			r.FinishTaskStatus = "waiting_pr_merge"
			m, _ = m.refreshFinalReplies(0)
			e := finalFixture(m, 1, "turn", "final", "旧的成功预览")
			m.projects[0].client.(*testClient).query.bodies = map[string]string{"final": "旧的成功预览"}
			next, cmd := m.Update(finalResult(m, e))
			m = next.(Model)
			next, _ = m.Update(cmd())
			m = next.(Model)
			msg := finalResult(m)
			if kind == "error" {
				msg.err = errors.New("current index unavailable")
			}
			if kind == "gap" {
				gap := e
				gap.Type = api.ConversationGap
				gap.Diagnostic = "current final gap"
				msg = finalResult(m, gap)
			}
			next, _ = m.Update(msg)
			m = next.(Model)
			if m.projects[0].final.previewText != "" || strings.Contains(m.View(), "旧的成功预览") {
				t.Fatal("stale final remained visible after", kind)
			}
		})
	}
}

func TestCPWAuditIgnoredPreviewIsNotCachedAsCompleted(t *testing.T) {
	m, _ := ccFixture(t, 1)
	r := m.selectedRow(0).run
	r.CurrentTurnID = "turn"
	m, _ = m.refreshFinalReplies(0)
	e := finalFixture(m, 1, "turn", "final", "可重读的正文")
	m.projects[0].client.(*testClient).query.bodies = map[string]string{"final": "可重读的正文"}
	next, cmd := m.Update(finalResult(m, e))
	m = next.(Model)
	late := cmd()
	r.CurrentTurnID = "different-turn"
	next, _ = m.Update(late)
	m = next.(Model)
	r.CurrentTurnID = "turn"
	state := &m.projects[0]
	if state.final.readPreview(m.ctx, state.client, finalReplyRoute{state.ref.Key, state.ref.Identity}, e) == nil {
		t.Fatal("ignored response became a permanent cache hit")
	}
}

func TestCPWAuditF8PagingDoesNotOvershoot(t *testing.T) {
	for _, answer := range []bool{false, true} {
		m, _ := ccFixture(t, 1)
		m.width, m.height = 80, 24
		if answer {
			m, _ = ccKey(m, runeKey('a'))
			m.answer.request.Context = strings.Repeat("详细背景\n", 70)
		} else {
			m.projects[0].ref.Presentation.Profiles[0].Details = []string{strings.Repeat("详细方案\n", 70)}
			m, _ = ccKey(m, runeKey('b'))
		}
		m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF8})
		for i := 0; i < 40; i++ {
			m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyPgDown})
		}
		bottom := m.View()
		m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyPgUp})
		if m.View() == bottom {
			t.Fatal("upward paging remained stuck beyond end", answer)
		}
	}
}

func TestCPWAuditGapRemainsVisibleWithValidFinal(t *testing.T) {
	m, _ := ccFixture(t, 1)
	r := m.selectedRow(0).run
	r.Decision = nil
	r.CurrentTurnID = "turn"
	r.FinishTaskStatus = "waiting_pr_merge"
	m, _ = m.refreshFinalReplies(0)
	e := finalFixture(m, 1, "turn", "final", "已采集的规范回复")
	m.projects[0].client.(*testClient).query.bodies = map[string]string{"final": "已采集的规范回复"}
	gap := e
	gap.Type = api.ConversationGap
	gap.Diagnostic = "来源仍有缺口"
	next, cmd := m.Update(finalResult(m, gap, e))
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	text := strings.Join(feedbackLines(m, m.projects[0], m.selectedRow(0)), "\n")
	if !strings.Contains(text, "来源仍有缺口") || !strings.Contains(text, "已采集的规范回复") {
		t.Fatal("valid preview hid canonical gap", text)
	}
}
