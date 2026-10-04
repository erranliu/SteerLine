package console

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
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
	rt := snap.Observation.Runtime
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
		r.ExecutionProfile = &executionmodel.FixedExecutionProfile{ID: fmt.Sprintf("profile-%d", i), Name: fmt.Sprintf("方案%d", i), Host: "local", Agent: "codex", ImplementationDisplay: []string{fmt.Sprintf("model-%d", i)}}
		rt.Runs = append(rt.Runs, &r)
		snap.DecisionReasons[r.RunID] = "提交后核对原会话接收与工作轮次开始"
	}
	snap.NativeTasks = &contracts.NativeTaskPage{GraphRevision: 7, Tasks: []contracts.NativeTaskDetail{{TaskRecord: taskmodel.TaskRecord{TaskID: "38", Status: "in_progress", Version: 2}}}}
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
	m, c := ccFixture(t, 2)
	m, _ = m.refreshSelectedLogs(0)
	s := &m.projects[0]
	oldKey, oldGeneration := s.conversationKey, s.conversationGeneration
	s.bodyText = "A正文"
	s.view.AnchorItem = "A-item"
	s.view.AnchorRevision = 2
	s.detailScroll = 4
	m, _ = ccKey(m, runeKey('a'))
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("候选甲草稿")})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = ccKey(m, runeKey('}'))
	if m.selectedRow(0).run.RunID != "candidate-1" || m.projects[0].bodyText != "" || m.projects[0].view.AnchorItem != "" {
		t.Fatal("candidate switch crossed body/anchor")
	}
	if m.projects[0].conversationFilter.WorkflowID != "candidate-1" || m.projects[0].conversationFilter.ExecutionID != "execution-1" || m.projects[0].conversationFilter.SessionID != "session-1" {
		t.Fatal("canonical query lost precise session")
	}
	next, _ := m.Update(conversationMsg{projectKey: s.ref.Key, identity: s.ref.Identity, key: oldKey, generation: oldGeneration, page: contracts.ConversationPage{Events: []contracts.ConversationEvent{{EventID: 1}}}})
	m = next.(Model)
	if len(m.projects[0].conversationEvents) != 0 {
		t.Fatal("late source changed B")
	}
	// The selected ID is held even if a fresh page temporarily omits it.
	pinned := m.projects[0].view.Candidates["38"]
	previousRuns := m.projects[0].snapshot.Observation.Runtime.Runs
	m.projects[0].snapshot.Observation.Runtime.Runs = m.projects[0].snapshot.Observation.Runtime.Runs[:1]
	if m.selectedRow(0).run != nil || m.projects[0].view.Candidates["38"] != pinned {
		t.Fatal("missing candidate silently selected sibling")
	}
	page := m.projects[0].snapshot.NativeTasks
	m.projects[0].snapshot.NativeTasks = nil
	gap := strings.Join(renderDetail(m, m.projects[0], m.selectedRow(0), 120, 100), "\n")
	if !strings.Contains(gap, "暂不可读") {
		t.Fatal("combined canonical gap was not shown", gap)
	}
	m.projects[0].snapshot.NativeTasks = page
	m.projects[0].snapshot.Observation.Runtime.Runs = previousRuns
	runs := m.projects[0].snapshot.Observation.Runtime.Runs
	runs[0], runs[1] = runs[1], runs[0]
	if m.selectedRow(0).run.RunID != "candidate-1" {
		t.Fatal("reorder changed ID selection")
	}
	m, _ = ccKey(m, runeKey('{'))
	m, _ = ccKey(m, runeKey('a'))
	if string(m.answer.note) != "候选甲草稿" || m.projects[0].view.AnchorItem != "A-item" {
		t.Fatal("A draft/reading not restored")
	}
	r := m.selectedRow(0).run
	r.EnvironmentBlocker = &executionmodel.WorkerHostBlocker{ID: "blocker-A", Version: 2, TurnID: "turn-A", Fact: executionbackend.HostFact{Reason: "原终端不可输入"}}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	if c.decisionCalls != 0 || m.answer == nil || !strings.Contains(m.answer.error, "环境阻断") {
		t.Fatal("blocked draft was sent")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	updated, environmentCmd := m.createAction("recheck_environment", "")
	m = updated.(Model)
	if environmentCmd == nil {
		t.Fatal(m.status)
	}
	m, _ = ccKey(m, runeKey('}'))
	next, _ = m.Update(environmentCmd())
	m = next.(Model)
	var environment productapi.ControlPayload
	if err := json.Unmarshal(c.request.Payload, &environment); err != nil {
		t.Fatal(err)
	}
	if environment.Environment == nil || environment.Environment.RunID != "candidate-0" || environment.Environment.ExecutionID != "execution-0" || environment.Environment.BlockerID != "blocker-A" || environment.Environment.Version != 2 {
		t.Fatal("environment action retargeted after candidate switch", environment)
	}
	m, _ = ccKey(m, runeKey('{'))
	r.EnvironmentBlocker = nil
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if c.decisionCalls != 0 {
		t.Fatal("clear blocker auto sent")
	}
	if err := m.saveProjectView(0); err != nil {
		t.Fatal(err)
	}
	v, err := readViewFile(m.profile.path())
	if err != nil {
		t.Fatal(err)
	}
	if v.Projects["cc"].Candidates["38"] != "candidate-0" || v.Projects["cc"].Drafts[candidateIdentity(r)+"\x00"+r.Decision.ID].Note != "候选甲草稿" {
		t.Fatal("restart preferences lost identity")
	}
	// Reopen a real UI view profile, not merely decoding JSON in the same model.
	home := t.TempDir()
	profile, prefs, _, err := acquireViewProfile(home)
	if err != nil {
		t.Fatal(err)
	}
	prefs.Projects[m.projects[0].ref.Key] = v.Projects["cc"]
	prefs.ActiveProject = "cc"
	if err := writeViewFile(profile.path(), prefs); err != nil {
		t.Fatal(err)
	}
	profile.close()
	ref := m.projects[0].ref
	client := m.projects[0].client.(*testClient)
	client.query.snapshot = m.projects[0].snapshot
	reopened, err := NewModel(context.Background(), []ProjectRef{ref}, Options{Clients: map[string]api.ConsoleClient{ref.Key: client}, ApplicationHome: home, Clock: m.clock})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cmd := reopened.readProject(0)
	next, _ = reopened.Update(cmd())
	reopened = next.(Model)
	if reopened.selectedRow(0) == nil || reopened.selectedRow(0).run.RunID != "candidate-0" {
		t.Fatal("reopen changed selected candidate")
	}
	reopened, _ = ccKey(reopened, runeKey('a'))
	if reopened.answer == nil || string(reopened.answer.note) != "候选甲草稿" {
		t.Fatal("reopen lost draft")
	}
	// A native log on disk cannot fill a canonical database gap.
	path := filepath.Join(t.TempDir(), "raw.jsonl")
	os.WriteFile(path, []byte("forbidden raw fallback"), 0600)
	r.LogPath = &path
	m.projects[0].conversationEvents = nil
	m.projects[0].conversationError = "规范数据库正文缺失"
	text := strings.Join(allSessionLines(m.projects[0], m.selectedRow(0), 120), "\n")
	if strings.Contains(text, "forbidden raw fallback") || !strings.Contains(text, "规范数据库正文缺失") {
		t.Fatal(text)
	}
	t.Log("exact candidate draft/reading persisted; late event discarded; blocker preserved Decision and required explicit resubmit; raw file ignored")
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
	next, _ = m.Update(actionReceiptMsg{projectKey: "cc", identity: c.request.RepositoryIdentity, requestID: id, receipt: &productapi.ControlReceipt{RequestID: id, Action: "start_candidate", InstanceID: a.TargetInstance, Status: "applied", RunID: "wrong-candidate"}})
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
	text := renderStopConfirmation(m, m.projects[0], 100)
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
	next, _ = m.Update(actionReceiptMsg{projectKey: "cc", identity: m.projects[0].ref.Identity, requestID: a.RequestID, receipt: &productapi.ControlReceipt{RequestID: a.RequestID, Action: a.Action, InstanceID: a.TargetInstance, Status: "applying", Code: "merge_state_unknown", RunID: "candidate-0"}})
	m = next.(Model)
	if m.projects[0].actions[a.RequestID].Resolved || m.selectedRow(0).run.RunID != "candidate-1" {
		t.Fatal("pending verification reported exit or switched selection")
	}
	r := m.projects[0].snapshot.Observation.Runtime.Runs[0]
	r.Abandonment = &executionmodel.CandidateAbandonment{At: m.clock.Now()}
	r.ResourceStatus = "unknown"
	r.PRHistory = []string{"https://example.invalid/historical/1"}
	m, _ = ccKey(m, runeKey('{'))
	text = strings.Join(renderDetail(m, m.projects[0], m.selectedRow(0), 120, 1000), "\n")
	for _, want := range []string{"历史/已放弃", "资源 unknown", "historical/1", "worker 停止独立核实"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	m, _ = ccKey(m, runeKey('z'))
	if m.confirm != nil {
		t.Fatal("abandoned feedback/exit remained available")
	}
	m.projects[0].snapshot.Observation.Runtime.Runs[1].PR.State = "merged"
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
	runs := s.snapshot.Observation.Runtime.Runs
	runs[0].Abandonment = &executionmodel.CandidateAbandonment{At: m.clock.Now()}
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
	s.snapshot.Observation.Runtime.Deliveries = []executionmodel.TaskDeliveryRecord{{TaskID: "38", SelectedWorkflowRunID: runs[1].RunID}}
	text = strings.Join(renderCandidates(m, *s, m.selectedRow(0)), "\n")
	if strings.Count(text, "获胜/唯一交付") != 1 || !strings.Contains(text, "冲突：非交付候选 PR 已合并") || !strings.Contains(text, "资源 active") {
		t.Fatal("late loser merge must preserve the sole delivery and actual resources", text)
	}
}
