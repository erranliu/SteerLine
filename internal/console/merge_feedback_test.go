package console

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func mergeFeedbackFixture(t *testing.T, count int) (Model, *testControlClient) {
	t.Helper()
	m, c := ccFixture(t, count)
	r := m.selectedRow(0).run
	r.FinishTaskStatus, r.ExecutionStatus, r.ResourceStatus = "waiting_pr_merge", "succeeded", "idle"
	url := "https://github.com/erranliu/SteerlineTest/pull/156"
	r.PR.URL, r.PR.State = &url, "open"
	r.Decision.Request.Question = "The worker stopped. What should it do next?"
	m.projects[0].final.previewText = "已提交证据并创建 PR"
	event := finalFixture(m, 1, r.CurrentTurnID, "final", m.projects[0].final.previewText)
	m.projects[0].final.previewEvent = &event
	return m, c
}

func TestWaitingPRMergeOptionalFeedbackSurfaces(t *testing.T) {
	m, _ := mergeFeedbackFixture(t, 1)
	s := m.projects[0]
	row := m.selectedRow(0)
	if got := candidateHumanStatus(s, row.run); got != "待合并" {
		t.Fatal(got)
	}
	if got := taskAttention(s, row.taskID); got != "待合并 P1" {
		t.Fatal(got)
	}
	if headline := candidateHeadline(s, row); !strings.Contains(headline, "待交互 0") || !strings.Contains(headline, "待合并 1") {
		t.Fatal(headline)
	}
	_, _, running, decision, merge, attention := projectCounts(s, m.clock.Now())
	if running != 0 || decision != 0 || merge != 1 || attention != 0 || !filterAllows(4, rowRun, row.run) || filterAllows(3, rowRun, row.run) || filterAllows(5, rowRun, row.run) {
		t.Fatalf("inconsistent counts/filters: %d %d %d %d", running, decision, merge, attention)
	}
	for name, lines := range map[string][]string{
		"overview": candidateOverviewLines(m, s, row, 160),
		"feedback": feedbackLines(m, s, row),
		"detail":   candidateDetailLines(m, s, row),
	} {
		text := strings.Join(lines, "\n")
		if !strings.Contains(text, "合并") || !strings.Contains(text, "可选反馈") {
			t.Fatalf("%s lost merge action or optional entry: %s", name, text)
		}
		if strings.Contains(text, "待交互") || strings.Contains(text, "待回答") || strings.Contains(text, "a/Enter 回答") {
			t.Fatalf("%s promoted optional feedback: %s", name, text)
		}
	}
	feedback := strings.Join(feedbackLines(m, s, row), "\n")
	if !strings.Contains(feedback, "最终回复预览") || !strings.Contains(feedback, "#156") || strings.Contains(feedback, "The worker stopped") {
		t.Fatal(feedback)
	}
	for _, size := range [][2]int{{160, 48}, {80, 30}, {50, 24}} {
		m.width, m.height = size[0], size[1]
		m.projects[0].focus = 1
		view := m.View()
		if !strings.Contains(view, "待合并") || strings.Contains(view, "待回答") || strings.Contains(view, "a/Enter 回复") {
			t.Fatalf("wrong primary state at %v: %s", size, view)
		}
	}
}

func TestWaitingPRMergeFeedbackRemainsExplicit(t *testing.T) {
	m, c := mergeFeedbackFixture(t, 1)
	r := m.selectedRow(0).run
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.answer != nil || m.projects[0].focus != 1 {
		t.Fatal("Enter opened optional feedback as a required answer")
	}
	m, _ = ccKey(m, runeKey('a'))
	if m.answer == nil || m.answer.runID != r.RunID || m.answer.decisionID != r.Decision.ID {
		t.Fatal("explicit feedback lost original identity")
	}
	if !strings.Contains(m.View(), "可选反馈") || r.FinishTaskStatus != "waiting_pr_merge" || r.Decision.Status != "pending" {
		t.Fatal("opening optional feedback rewrote business state or hid its purpose")
	}
	m, cmd = ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	if cmd == nil {
		t.Fatal(m.status)
	}
	m, _ = ccKey(m, runeKey('a'))
	if m.answer != nil {
		t.Fatal("submitted feedback allowed a duplicate draft")
	}
	m.Update(cmd())
	if c.decisionCalls != 1 || c.decisionAnswer.RunID != r.RunID || c.decisionAnswer.DecisionID != r.Decision.ID {
		t.Fatal("feedback was not submitted against the original decision")
	}
}

func TestWaitingPRMergeDoesNotHideSiblingDecision(t *testing.T) {
	m, _ := mergeFeedbackFixture(t, 2)
	s := m.projects[0]
	row := m.selectedRow(0)
	headline := candidateHeadline(s, row)
	if !strings.Contains(headline, "待交互 1") || !strings.Contains(headline, "待合并 1") || taskAttention(s, row.taskID) != "待回答 P2" {
		t.Fatal(headline, taskAttention(s, row.taskID))
	}
	_, _, _, decision, merge, _ := projectCounts(s, m.clock.Now())
	if decision != 1 || merge != 1 || !filterAllows(3, rowRun, candidateRuns(s, row.taskID)[1]) {
		t.Fatal("task-level aggregation hid the sibling decision")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF7})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.answer == nil || m.answer.runID != "candidate-1" {
		t.Fatal("Enter no longer opens a required sibling decision")
	}
}

func TestWaitingPRMergePreservesRealBlockers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*api.CandidateSummary)
	}{
		{"unknown execution", func(r *api.CandidateSummary) { r.ExecutionStatus = "unknown" }},
		{"failed execution", func(r *api.CandidateSummary) { r.ExecutionStatus = "failed" }},
		{"unverified recovery", func(r *api.CandidateSummary) { r.RecoveryRequired, r.RecoveryState = true, "unverified" }},
		{"interrupted recovery", func(r *api.CandidateSummary) { r.RecoveryRequired, r.RecoveryState = true, "interrupted" }},
		{"worker error", func(r *api.CandidateSummary) { message := "worker error"; r.Error = &message }},
		{"PR query failure", func(r *api.CandidateSummary) { message := "provider offline"; r.PR.QueryError = &message }},
		{"environment blocker", func(r *api.CandidateSummary) {
			r.EnvironmentBlocker = &api.EnvironmentBlocker{Fact: api.EnvironmentFact{Reason: "原会话不可用"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := mergeFeedbackFixture(t, 1)
			row := m.selectedRow(0)
			tc.change(row.run)
			s := m.projects[0]
			if candidateHumanStatus(s, row.run) == "待合并" || !runNeedsAttention(row.run) || !filterAllows(5, rowRun, row.run) {
				t.Fatal("merge phase concealed a real blocker")
			}
			if !strings.Contains(taskAttention(s, row.taskID), "核验") && !strings.Contains(taskAttention(s, row.taskID), "受阻") {
				t.Fatal(taskAttention(s, row.taskID))
			}
			if strings.Contains(nextAction(m, s, row), "人工合并") || strings.Contains(candidateHeadline(s, row), "待交互 1") {
				t.Fatal("optional feedback hid diagnostics or became a required question")
			}
		})
	}
}

func TestWaitingPRMergeTerminalCandidatesDoNotRequestInput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*api.CandidateSummary)
	}{
		{"closed task", func(r *api.CandidateSummary) { r.TaskStatus = "closed" }},
		{"discarded task", func(r *api.CandidateSummary) { r.TaskStatus = "discarded" }},
		{"completed workflow", func(r *api.CandidateSummary) { r.FinishTaskStatus = "completed" }},
		{"abandoned candidate", func(r *api.CandidateSummary) { r.Abandonment = &api.AbandonmentSummary{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := mergeFeedbackFixture(t, 1)
			r := m.selectedRow(0).run
			tc.change(r)
			_, _, _, decision, merge, _ := projectCounts(m.projects[0], m.clock.Now())
			if decision != 0 || merge != 0 || filterAllows(3, rowRun, r) || filterAllows(4, rowRun, r) || taskAttention(m.projects[0], r.TaskID) != "" {
				t.Fatal("stale optional question resurrected a terminal candidate")
			}
			if strings.Contains(candidateHeadline(m.projects[0], m.selectedRow(0)), "待交互 1") {
				t.Fatal("terminal candidate counted as waiting for input")
			}
		})
	}
}

func TestWaitingPRMergePhaseDeterminesWhetherFeedbackIsRequired(t *testing.T) {
	m, _ := mergeFeedbackFixture(t, 1)
	r := m.selectedRow(0).run
	decision := r.Decision
	r.RecoveryRequired, r.RecoveryState = true, "completed"
	if candidateHumanStatus(m.projects[0], r) != "待合并" || waitingForDecision(r) {
		t.Fatal("a verified completed recovery hid the merge-waiting phase")
	}
	// Keep the same question and identity: changing the business phase, rather
	// than matching its English wording, makes further input mandatory.
	r.FinishTaskStatus = "waiting_decision"
	if !waitingForDecision(r) || waitingForMerge(r) || candidateHumanStatus(m.projects[0], r) != "待交互" {
		t.Fatal("workflow transition did not restore required interaction")
	}
	if !filterAllows(3, rowRun, r) || filterAllows(4, rowRun, r) || !strings.Contains(nextAction(m, m.projects[0], m.selectedRow(0)), "回答") {
		t.Fatal("filters/actions did not follow the workflow phase")
	}
	if r.Decision != decision || decision.Status != "pending" {
		t.Fatal("presentation rewrote the persisted decision")
	}
}
