package console

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func finalFixture(m Model, id int64, turn, item, text string) api.ConversationEvent {
	run := m.selectedRow(0).run
	digest := sha256.Sum256([]byte(text))
	return api.ConversationEvent{Version: api.ConversationVersion, EventID: id, ProjectID: m.projects[0].ref.Identity, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, Source: "canonical", SourceSegment: "segment", SessionID: run.SessionID, TurnID: turn, TurnQuality: api.IdentityNative, ItemKey: item, SourceRevision: 1, Type: api.ConversationMessageComplete, Phase: api.MessagePhaseFinalAnswer, Body: &api.ConversationBodyRef{ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, ItemKey: item, Revision: 1, SHA256: hex.EncodeToString(digest[:]), Bytes: int64(len(text)), Completeness: api.BodyComplete}}
}

func finalResult(m Model, events ...api.ConversationEvent) finalRepliesMsg {
	s := m.projects[0]
	return finalRepliesMsg{projectKey: s.ref.Key, identity: s.ref.Identity, key: s.finalKey, generation: s.finalGeneration, page: api.ConversationPage{Events: events}}
}

func TestPS1CompleteFinalAndHistoryAreCandidateBound(t *testing.T) {
	m, _ := ccFixture(t, 2)
	m.detailExpanded = true
	m.detailSection = "final"
	m, _ = m.refreshFinalReplies(0)
	text := strings.Repeat("中文完整正文🧭\n", 9000) + "回复末尾"
	query := m.projects[0].client.(*testClient).query
	query.bodies = map[string]string{"first": "历史回复", "second": text}
	first := finalFixture(m, 1, "turn-one", "first", "历史回复")
	second := finalFixture(m, 2, "turn-two", "second", text)
	next, cmd := m.Update(finalResult(m, first, second))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("latest final was not read")
	}
	late := cmd()
	next, _ = m.Update(late)
	m = next.(Model)
	if m.projects[0].bodyText != text || m.projects[0].bodyBusy || m.projects[0].bodyError != "" {
		t.Fatal("long final was not loaded in full", m.projects[0].bodyError)
	}
	m, cmd = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.projects[0].bodyText != "历史回复" {
		t.Fatal("history selection lost exact body")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF7})
	next, _ = m.Update(late)
	m = next.(Model)
	if m.projects[0].bodyText != "" || len(m.projects[0].finalEvents) != 0 {
		t.Fatal("late A body reached B")
	}
	foreign := second
	foreign.ExecutionID = "other"
	next, _ = m.Update(finalResult(m, foreign))
	m = next.(Model)
	if m.projects[0].finalError == "" || len(m.projects[0].finalEvents) != 0 {
		t.Fatal("foreign final accepted")
	}
}

func TestPS1FinalRejectsUnknownPhaseAndBadBody(t *testing.T) {
	m, _ := ccFixture(t, 1)
	m.detailExpanded = true
	m.detailSection = "final"
	m, _ = m.refreshFinalReplies(0)
	process := finalFixture(m, 1, "turn", "process", "过程")
	process.Phase = api.MessagePhaseCommentary
	unknown := finalFixture(m, 2, "turn", "unknown", "未知")
	unknown.TurnQuality = api.IdentityUnknown
	gap := process
	gap.Type = api.ConversationGap
	gap.Diagnostic = "原来源尚未采集"
	next, _ := m.Update(finalResult(m, process, unknown, gap))
	m = next.(Model)
	if len(m.projects[0].finalEvents) != 0 || !strings.Contains(m.projects[0].finalError, "尚未采集") {
		t.Fatal("process/unknown promoted or gap hidden")
	}
	final := finalFixture(m, 3, "turn", "final", "内容")
	query := m.projects[0].client.(*testClient).query
	query.bodies = map[string]string{"final": "内容"}
	query.badBody = true
	next, cmd := m.Update(finalResult(m, final))
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.projects[0].bodyText != "" || m.projects[0].bodyError == "" {
		t.Fatal("failed read showed complete final")
	}
}

func TestPS1JournalRestoresDraftAndOnlyQueriesOriginalRequest(t *testing.T) {
	home := t.TempDir()
	ref := testProject("journal")
	now := time.Now().UTC()
	snapshot, run := decisionSnapshot(ref, now)
	control := &testControlClient{}
	client := &testClient{query: &testQuerySource{snapshot: snapshot}, control: control, ref: ref}
	options := Options{ApplicationHome: home, Clock: &testClock{now: now}, Clients: map[string]ConsoleClient{ref.Key: client}}
	m, err := NewModel(context.Background(), []ProjectRef{ref}, options)
	if err != nil {
		t.Fatal(err)
	}
	m.projects[0].snapshot = snapshot
	m.projects[0].snapshotLoaded = true
	m.projects[0].selectedKey = "task:" + run.TaskID
	id := "0123456789abcdef0123456789abcdef"
	m.projects[0].view.Drafts = map[string]CandidateDraft{candidateIdentity(run) + "\x00" + run.Decision.ID: {OptionIndex: 1, Note: "跨重开保留的答案"}}
	m.projects[0].actions[id] = &actionState{RequestID: id, Action: "answer_decision", TaskID: run.TaskID, RunID: run.RunID, DecisionID: run.Decision.ID, DecisionIdentity: candidateIdentity(run), TargetInstance: snapshot.Observation.Page.Service.InstanceID, Status: "indeterminate", UpdatedAt: now, DecisionVersion: run.Decision.Version, DecisionRevision: run.ControlRevision}
	m.projects[0].detailScroll = 19
	m.projects[0].filter = 3
	m.detailExpanded = true
	if err = m.saveProjectView(0); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(m.profile.path())
	if err != nil {
		t.Fatal(err)
	}
	for _, retired := range []string{"selected_key", "candidate_reading", "panel", "body_offset", "anchor_group", "detail_scroll", "active_project"} {
		if strings.Contains(string(raw), retired) {
			t.Fatal("reading preference persisted", retired)
		}
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewModel(context.Background(), []ProjectRef{ref}, options)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	s := &reopened.projects[0]
	if reopened.detailExpanded || s.detailScroll != 0 || s.filter != 0 || s.selectedKey != "" || len(s.candidates) != 0 {
		t.Fatal("reading state restored")
	}
	if s.view.Drafts[candidateIdentity(run)+"\x00"+run.Decision.ID].Note != "跨重开保留的答案" || s.actions[id] == nil {
		t.Fatal("draft or original command lost")
	}
	cmd := reopened.pollAction(0, s.actions[id])
	reopened.Update(cmd())
	if control.calls != 0 || control.decisionCalls != 0 || control.receipts == 0 {
		t.Fatal("reopen sent another request")
	}
}

func TestPS1DamagedOrOldJournalKeepsFileAndBlocksExternalInput(t *testing.T) {
	for _, raw := range []string{"not json", `{"version":1,"projects":{}}`} {
		t.Run(raw, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, "ui", "views", "window")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "view.json")
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			ref := testProject("damaged")
			now := time.Now().UTC()
			snap := readySnapshot(ref, now)
			control := &testControlClient{}
			client := &testClient{query: &testQuerySource{snapshot: snap}, control: control, ref: ref}
			m, err := NewModel(context.Background(), []ProjectRef{ref}, Options{ApplicationHome: home, Clock: &testClock{now: now}, Clients: map[string]ConsoleClient{ref.Key: client}})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			m.projects[0].snapshot = snap
			m.projects[0].snapshotLoaded = true
			m.projects[0].selectedKey = "task:task-B"
			m, _ = ccKey(m, runeKey('s'))
			m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil || control.calls != 0 || m.prefsErr == "" {
				t.Fatal("damaged journal allowed new input")
			}
			kept, err := os.ReadFile(path)
			if err != nil || string(kept) != raw {
				t.Fatal("damaged file overwritten", err)
			}
		})
	}
}

func TestPS1BackgroundProjectBodyStaysWithOriginalProject(t *testing.T) {
	m, _ := ccFixture(t, 1)
	m.detailExpanded = true
	m.detailSection = "final"
	m, _ = m.refreshFinalReplies(0)
	event := finalFixture(m, 1, "turn", "body", "原项目正文")
	next, cmd := m.Update(finalResult(m, event))
	m = next.(Model)
	first := m.projects[0]
	second := first
	second.ref.Key = "other-project"
	second.ref.Identity = "other-identity"
	second.bodyRef = nil
	second.bodyText = ""
	m.projects = append(m.projects, second)
	m.selected = 1
	msg := bodyMsg{projectKey: first.ref.Key, identity: first.ref.Identity, key: first.finalKey, generation: m.projects[0].bodyGeneration, body: *event.Body, text: "原项目正文"}
	if cmd == nil {
		t.Fatal("body was not scheduled")
	}
	next, _ = m.Update(msg)
	m = next.(Model)
	if m.projects[0].bodyText != "原项目正文" || m.projects[1].bodyText != "" {
		t.Fatal("background response crossed projects")
	}
}
