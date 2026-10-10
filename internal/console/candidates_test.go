package console

import (
	"fmt"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func ccFixture(t *testing.T, count int) (Model, *testControlClient) {
	t.Helper()
	now := time.Now().UTC()
	ref := testProject("cc")
	snap, first := decisionSnapshot(ref, now)
	snap.Tasks.Tasks = nil
	first.TaskID = "38"
	first.TaskStatus = "in_progress"
	first.SessionID = "session-0"
	first.ResourceStatus = "active"
	first.CandidateCreatedAt = now.Add(-time.Minute)
	rt := snap.Observation.Page
	rt.Runs = nil
	rt.Service.Capabilities = append(rt.Service.Capabilities, "start_candidate", "abandon_candidate", "recheck_environment", "open_terminal")
	for i := 0; i < count; i++ {
		r := *first
		d := *first.Decision
		r.Decision = &d
		r.RunID = fmt.Sprintf("candidate-%d", i)
		r.ExecutionID = fmt.Sprintf("execution-%d", i)
		r.SessionID = fmt.Sprintf("session-%d", i)
		r.AttemptID = fmt.Sprintf("attempt-%d", i)
		r.Decision.ID = fmt.Sprintf("decision-%d", i)
		r.ExecutionProfile = &api.ExecutionProfileSummary{ID: fmt.Sprintf("profile-%d", i), Name: fmt.Sprintf("方案%d", i), Host: "local", Agent: "codex", ImplementationDisplay: []string{fmt.Sprintf("model-%d", i)}}
		rt.Runs = append(rt.Runs, &r)
		snap.DecisionReasons[r.RunID] = "提交后核对原会话接收与工作轮次开始"
	}
	snap.NativeTasks = &api.TaskPage{GraphRevision: 7, Tasks: []api.TaskDetail{{TaskSummary: api.TaskSummary{TaskID: "38", Status: "in_progress", Version: 2}}}}
	ref.Presentation.Profiles = []api.ExecutionProfileSummary{{ID: "profile-new", Name: "新方案", Digest: "fixed-new"}}
	control := &testControlClient{}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snap}, control, now)
	m.projects[0].selectedKey = "task:38"
	m.profile = &viewProfile{dir: t.TempDir()}
	return m, control
}

func ccKey(m Model, k tea.KeyMsg) (Model, tea.Cmd) {
	next, cmd := m.Update(k)
	return next.(Model), cmd
}

func TestCC1(t *testing.T) {
	m, _ := ccFixture(t, 5)
	rows := m.rows(0)
	if len(rows) != 1 || rows[0].taskID != "38" {
		t.Fatalf("Task main rows: %+v", rows)
	}
	_, _, _, decisions, _, attention := projectCounts(m.projects[0], m.clock.Now())
	if decisions != 1 || attention != 1 {
		t.Fatal("Task counters counted candidates", decisions, attention)
	}
	m.detailExpanded = true
	text := strings.Join(renderDetail(m, m.projects[0], m.selectedRow(0), 120, 1000), "\n")
	for i := 0; i < 5; i++ {
		for _, want := range []string{fmt.Sprintf("candidate-%d", i), fmt.Sprintf("方案%d", i), fmt.Sprintf("session-%d", i)} {
			if !strings.Contains(text, want) {
				t.Fatal("snapshot fact missing", want)
			}
		}
	}
	if !strings.Contains(text, "指标缺口") || !strings.Contains(text, "资源 active") || !strings.Contains(text, "无硬名额") {
		t.Fatal(text)
	}
	t.Log(text)
}

func TestCC2(t *testing.T) {
	m, _ := ccFixture(t, 2)
	m, _ = m.refreshFinalReplies(0)
	oldKey, oldGeneration := m.projects[0].final.finalKey, m.projects[0].final.finalGeneration
	m.projects[0].final.bodyText = "A正文"
	m, _ = ccKey(m, runeKey('a'))
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("候选甲草稿")})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = ccKey(m, runeKey('}'))
	if m.selectedRow(0).run.RunID != "candidate-1" || m.projects[0].final.bodyText != "" {
		t.Fatal("candidate switch crossed final body")
	}
	next, _ := m.Update(finalRepliesMsg{projectKey: m.projects[0].ref.Key, identity: m.projects[0].ref.Identity, key: oldKey, generation: oldGeneration, page: api.ConversationPage{Events: []api.ConversationEvent{{EventID: 1}}}})
	m = next.(Model)
	if len(m.projects[0].final.finalEvents) != 0 {
		t.Fatal("late response crossed candidates")
	}
	runs := m.projects[0].snapshot.Observation.Page.Runs
	runs[0], runs[1] = runs[1], runs[0]
	if m.selectedRow(0).run.RunID != "candidate-1" {
		t.Fatal("reorder changed selection")
	}
	m, _ = ccKey(m, runeKey('{'))
	m, _ = ccKey(m, runeKey('a'))
	if string(m.answer.note) != "候选甲草稿" {
		t.Fatal("candidate draft lost")
	}
}

func TestCC3(t *testing.T) {
	m, c := ccFixture(t, 2)
	m, _ = ccKey(m, runeKey('b'))
	if m.profileChoice == nil {
		t.Fatal(m.status)
	}
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal(m.status)
	}
	var id string
	for key, a := range m.projects[0].actions {
		id = key
		if a.Payload.ExpectedTaskVersion != 2 || a.Payload.ExpectedGraphRevision != 7 || a.Payload.ExecutionProfileDigest != "fixed-new" {
			t.Fatal(a)
		}
	}
	m, _ = ccKey(m, runeKey('b'))
	m, duplicate := ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if duplicate != nil {
		t.Fatal("duplicate published replacement request")
	}
	m, _ = ccKey(m, runeKey('}'))
	next, _ := m.Update(cmd())
	m = next.(Model)
	if c.calls != 1 || c.request.RequestID != id || m.selectedRow(0).run.RunID != "candidate-1" {
		t.Fatal("submission was retargeted")
	}
	a := m.projects[0].actions[id]
	c.request.RepositoryIdentity = m.projects[0].ref.Identity
	if !controlRequestMatchesAction(&c.request, a, c.request.RepositoryIdentity) {
		t.Fatal("original request reconciliation failed")
	}
	next, _ = m.Update(actionReceiptMsg{projectKey: "cc", identity: c.request.RepositoryIdentity, requestID: id, request: &c.request})
	m = next.(Model)
	if m.projects[0].actions[id].Status != "submitted" || len(m.projects[0].actions) != 1 {
		t.Fatal("lost receipt replaced request")
	}
	next, _ = m.Update(actionReceiptMsg{projectKey: "cc", identity: c.request.RepositoryIdentity, requestID: id, receipt: &api.ControlReceipt{RequestID: id, Action: "start_candidate", InstanceID: a.TargetInstance, Status: "applied", RunID: "wrong-candidate"}})
	m = next.(Model)
	if m.projects[0].actions[id].Status != "indeterminate" || m.projects[0].actions[id].Payload != a.Payload || m.selectedRow(0).run.RunID != "candidate-1" {
		t.Fatal("wrong candidate receipt changed target")
	}
	m5, _ := ccFixture(t, 5)
	m5, _ = ccKey(m5, runeKey('b'))
	if m5.profileChoice != nil || !strings.Contains(m5.status, "已达5个") {
		t.Fatal(m5.status)
	}
	t.Logf("immutable candidate input: %+v; request %s recovered without resubmit", a.Payload, id)
}

func TestCC4(t *testing.T) {
	m, c := ccFixture(t, 2)
	m, _ = ccKey(m, runeKey('z'))
	if m.confirm == nil {
		t.Fatal(m.status)
	}
	text := strings.Join(confirmRequired(m, m.projects[0]), "\n")
	if !strings.Contains(text, "不可撤销") || !strings.Contains(text, "可能仍运行") {
		t.Fatal(text)
	}
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal(m.status)
	}
	m, _ = ccKey(m, runeKey('}'))
	next, _ := m.Update(cmd())
	m = next.(Model)
	a := m.projects[0].actions[c.request.RequestID]
	if a.Payload.WorkflowRunID != "candidate-0" || a.Payload.ExpectedCandidateVersion != 9 {
		t.Fatal(a)
	}
	next, _ = m.Update(actionReceiptMsg{projectKey: "cc", identity: m.projects[0].ref.Identity, requestID: a.RequestID, receipt: &api.ControlReceipt{RequestID: a.RequestID, Action: a.Action, InstanceID: a.TargetInstance, Status: "applying", Code: "merge_state_unknown", RunID: "candidate-0"}})
	m = next.(Model)
	if m.projects[0].actions[a.RequestID].Resolved || m.selectedRow(0).run.RunID != "candidate-1" {
		t.Fatal("pending verification reported exit or switched selection")
	}
	r := m.projects[0].snapshot.Observation.Page.Runs[0]
	r.Abandonment = &api.AbandonmentSummary{RequestedAt: m.clock.Now()}
	r.ResourceStatus = "unknown"
	r.PRHistory = []string{"https://example.invalid/historical/1"}
	m, _ = ccKey(m, runeKey('{'))
	m.detailExpanded = true
	text = strings.Join(renderDetail(m, m.projects[0], m.selectedRow(0), 120, 1000), "\n")
	for _, want := range []string{"历史/已放弃", "资源 unknown", "historical/1", "worker 停止独立核实"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = ccKey(m, runeKey('z'))
	if m.confirm != nil {
		t.Fatal("abandoned feedback/exit remained available")
	}
	m.projects[0].snapshot.Observation.Page.Runs[1].PR.State = "merged"
	m, _ = ccKey(m, runeKey('}'))
	m, _ = ccKey(m, runeKey('z'))
	if m.confirm != nil || !strings.Contains(m.status, "禁止放弃") {
		t.Fatal(m.status)
	}
	t.Log("exit/stop facts remain separate; exact original exit request; pending verification not exit; merged PR closes abandon")
}

func TestCandidateMergedConflictPresentation(t *testing.T) {
	m, _ := ccFixture(t, 2)
	s := &m.projects[0]
	runs := s.snapshot.Observation.Page.Runs
	runs[0].Abandonment = &api.AbandonmentSummary{RequestedAt: m.clock.Now()}
	runs[0].PR.State = "merged"
	text := strings.Join(renderCandidates(m, *s, m.selectedRow(0)), "\n")
	if !strings.Contains(text, "冲突：已放弃候选 PR 已合并") || !strings.Contains(text, "资源 active") {
		t.Fatal("abandoned merge conflict or live resource fact missing", text)
	}
	runs[0].Abandonment = nil
	runs[1].PR.State = "merged"
	text = strings.Join(renderCandidates(m, *s, m.selectedRow(0)), "\n")
	if !strings.Contains(text, "冲突：多个候选 PR 已合并") || strings.Contains(text, "获胜/唯一交付") {
		t.Fatal("multiple merges must remain a conflict without a winner", text)
	}
	for _, r := range runs {
		r.TaskStatus = "closed"
	}
	runs[0].DeliveryOutcome, runs[1].DeliveryOutcome = "loser", "winner"
	text = strings.Join(renderCandidates(m, *s, m.selectedRow(0)), "\n")
	if strings.Count(text, "获胜/唯一交付") != 1 || !strings.Contains(text, "冲突：非交付候选 PR 已合并") || !strings.Contains(text, "资源 active") {
		t.Fatal("late loser merge must preserve the sole delivery and actual resources", text)
	}
}
