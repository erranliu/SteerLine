package console

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

// This stub implements only the two read ports accepted by finalReplyReader.
type finalReaderQueries struct {
	pages   map[string]api.ConversationPage
	bodies  map[string]string
	filters []api.ConversationFilter
	cursors []string
	limits  []int
	badBody bool
}

func (q *finalReaderQueries) Conversation(_ context.Context, filter api.ConversationFilter, cursor string) (api.ConversationPage, error) {
	q.filters = append(q.filters, filter)
	q.cursors = append(q.cursors, cursor)
	return q.pages[cursor], nil
}

func (q *finalReaderQueries) Body(_ context.Context, ref api.ConversationBodyRef, offset int64, limit int) (api.ConversationBodyPage, error) {
	q.limits = append(q.limits, limit)
	text := q.bodies[ref.ItemKey]
	end := min(int(offset)+limit, len(text))
	for !utf8.ValidString(text[int(offset):end]) {
		end--
	}
	page := api.ConversationBodyPage{ItemKey: ref.ItemKey, Revision: ref.Revision, Offset: offset, NextOffset: int64(end), TotalBytes: ref.Bytes, SHA256: ref.SHA256, Completeness: ref.Completeness, Complete: end == len(text), Data: []byte(text[int(offset):end])}
	if q.badBody {
		page.Revision++
	}
	return page, nil
}

func readerFinal(text, item, attempt, turn string) api.ConversationEvent {
	digest := sha256.Sum256([]byte(text))
	return api.ConversationEvent{Version: api.ConversationVersion, EventID: 1, ProjectID: "project", WorkflowID: "run", ExecutionID: "execution", AttemptID: attempt, SessionID: "session", TurnID: turn, TurnQuality: api.IdentityNative, Source: "canonical", ItemKey: item, SourceRevision: 1, Type: api.ConversationMessageComplete, Phase: api.MessagePhaseFinalAnswer, Body: &api.ConversationBodyRef{ExecutionID: "execution", AttemptID: attempt, ItemKey: item, Revision: 1, Bytes: int64(len(text)), SHA256: hex.EncodeToString(digest[:]), Completeness: api.BodyComplete}}
}

func readerFilter() api.ConversationFilter {
	return api.ConversationFilter{WorkflowID: "run", ExecutionID: "execution", SessionID: "session", FinalsOnly: true}
}

func TestFinalReaderCycleOwnsEmptyAndPinnedSelection(t *testing.T) {
	ctx, route := context.Background(), finalReplyRoute{"project-key", "project"}
	var reader finalReplyReader
	if cmd, cycled := reader.cycle(ctx, nil, route, -1); cmd != nil || cycled || reader.finalPinned || reader.bodyGeneration != 0 {
		t.Fatal("empty history changed state or required a host guard")
	}
	first := readerFinal("first body", "first", "attempt", "first-turn")
	last := readerFinal("last body", "last", "attempt", "last-turn")
	q := &finalReaderQueries{pages: map[string]api.ConversationPage{"": {Events: []api.ConversationEvent{first, last}}}, bodies: map[string]string{"first": "first body", "last": "last body"}}
	_, cmd := reader.refresh(ctx, q, time.Now(), route, "candidate", readerFilter(), true, "", "")
	reader.acceptReplies(ctx, q, route, true, cmd().(finalRepliesMsg))
	cmd, cycled := reader.cycle(ctx, q, route, -1)
	if !cycled || cmd == nil || !reader.finalPinned || reader.finalSelected != conversationItemKey(first) {
		t.Fatal("history cycle lost the selected item/pin")
	}
	reader.acceptBody(route.identity, cmd().(bodyMsg))
	if reader.bodyText != "first body" || reader.bodyError != "" {
		t.Fatal("history cycle read another item")
	}
}

func TestFinalReaderBindingRejectsAllLateReads(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	route := finalReplyRoute{"project-key", "project"}
	event := readerFinal("original body", "item", "attempt", "turn")
	q := &finalReaderQueries{pages: map[string]api.ConversationPage{"": {Events: []api.ConversationEvent{event}}}, bodies: map[string]string{"item": "original body"}}
	var reader finalReplyReader
	changed, cmd := reader.refresh(ctx, q, now, route, "candidate-a", readerFilter(), true, "attempt", "turn")
	if !changed || cmd == nil {
		t.Fatal("binding did not start the first index read")
	}
	index := cmd().(finalRepliesMsg)
	bodyCmd, selected := reader.acceptReplies(ctx, q, route, true, index)
	if !selected || bodyCmd == nil {
		t.Fatal("index did not select a body")
	}
	previewCmd := reader.readPreview(ctx, q, route, event)
	// Pending commands keep their original query/filter/body even after rebinding.
	changed, _ = reader.refresh(ctx, q, now, route, "candidate-b", readerFilter(), false, "attempt-b", "turn-b")
	if !changed || len(reader.finalEvents) != 0 || reader.bodyRef != nil || reader.previewEvent != nil || reader.finalPinned {
		t.Fatal("old state survived rebinding")
	}
	if cmd, changed := reader.acceptReplies(ctx, q, route, true, index); cmd != nil || changed {
		t.Fatal("late index was accepted")
	}
	reader.acceptBody(route.identity, bodyCmd().(bodyMsg))
	reader.acceptPreview(route.identity, finalPreviewTarget{"candidate-b", "attempt-b", "turn-b"}, previewCmd().(finalPreviewMsg))
	if reader.bodyText != "" || reader.previewText != "" || len(reader.finalEvents) != 0 {
		t.Fatal("late reads crossed the candidate binding")
	}
	if q.filters[0].AttemptID != "" || q.filters[0].TurnID != "" {
		t.Fatal("old full-history command captured the new scoped request")
	}
}

func TestFinalReaderHistoryPagingRevisionAndBodyValidation(t *testing.T) {
	ctx, route := context.Background(), finalReplyRoute{"project-key", "project"}
	text := strings.Repeat("完整正文\n", 15000)
	event := readerFinal(text, "historical", "old-attempt", "old-turn")
	event.SourceRevision = 3
	older := event
	older.SourceRevision = 2
	q := &finalReaderQueries{pages: map[string]api.ConversationPage{"": {Events: []api.ConversationEvent{event}, HasMore: true, NextCursor: "next"}, "next": {Events: []api.ConversationEvent{older}}}, bodies: map[string]string{"historical": text}}
	var reader finalReplyReader
	_, cmd := reader.refresh(ctx, q, time.Now(), route, "candidate", readerFilter(), true, "current-attempt", "current-turn")
	index := cmd().(finalRepliesMsg)
	if index.err != nil || len(q.cursors) != 2 || q.cursors[1] != "next" || q.filters[1].AttemptID != "" || q.filters[1].TurnID != "" {
		t.Fatal("history was filtered or pagination lost", index.err)
	}
	bodyCmd, changed := reader.acceptReplies(ctx, q, route, true, index)
	if !changed || len(reader.finalEvents) != 1 || reader.finalEvents[0].SourceRevision != 3 {
		t.Fatal("old revision replaced the original item")
	}
	msg := bodyCmd().(bodyMsg)
	foreign := msg
	foreign.body.Revision++
	reader.acceptBody(route.identity, foreign)
	if !reader.bodyBusy || reader.bodyText != "" {
		t.Fatal("wrong body reference was accepted")
	}
	reader.acceptBody(route.identity, msg)
	if reader.bodyError != "" || reader.bodyBusy || reader.bodyText != text || len(q.limits) < 2 {
		t.Fatal("full body was not validated across bounded pages", reader.bodyError)
	}
	q.badBody = true
	bad := reader.selectBody(ctx, q, route, event)().(bodyMsg)
	reader.acceptBody(route.identity, bad)
	if reader.bodyText != "" || reader.bodyError == "" {
		t.Fatal("mismatched body metadata became a completed body")
	}
	q.pages["next"] = api.ConversationPage{HasMore: true, NextCursor: "next"}
	stuck := reader.read(ctx, q, time.Now(), route, true, "", "", false)().(finalRepliesMsg)
	if stuck.err == nil {
		t.Fatal("nonadvancing history cursor was accepted")
	}
}

func TestFinalReaderPreviewIsolationAndIndexGap(t *testing.T) {
	ctx, route := context.Background(), finalReplyRoute{"project-key", "project"}
	text := strings.Repeat("line of final text\n", 500)
	event := readerFinal(text, "item", "attempt", "turn")
	q := &finalReaderQueries{pages: map[string]api.ConversationPage{"": {Events: []api.ConversationEvent{event}}}, bodies: map[string]string{"item": text}}
	var reader finalReplyReader
	_, cmd := reader.refresh(ctx, q, time.Now(), route, "candidate", readerFilter(), false, "attempt", "turn")
	previewCmd, _ := reader.acceptReplies(ctx, q, route, false, cmd().(finalRepliesMsg))
	msg := previewCmd().(finalPreviewMsg)
	reader.acceptPreview(route.identity, finalPreviewTarget{"candidate", "attempt", "other-turn"}, msg)
	if reader.previewReady || reader.previewText != "" || reader.previewError == "" {
		t.Fatal("stale turn was cached")
	}
	previewCmd = reader.readPreview(ctx, q, route, event)
	if previewCmd == nil {
		t.Fatal("rejected preview could not be read again")
	}
	reader.acceptPreview(route.identity, finalPreviewTarget{"candidate", "attempt", "turn"}, previewCmd().(finalPreviewMsg))
	if !reader.previewReady || len([]rune(reader.previewText)) > 800 || len(strings.Split(reader.previewText, "\n")) > 6 || q.limits[0] != 4096 || reader.bodyRef != nil {
		t.Fatal("preview changed limits or populated full-body state")
	}
	gap := event
	gap.Type, gap.Diagnostic = api.ConversationGap, "original source unavailable"
	reader.acceptReplies(ctx, q, route, false, finalRepliesMsg{projectKey: route.projectKey, identity: route.identity, key: reader.finalKey, generation: reader.finalGeneration, page: api.ConversationPage{Events: []api.ConversationEvent{gap}}})
	if reader.previewText != "" || reader.previewReady || !strings.Contains(reader.finalError, gap.Diagnostic) {
		t.Fatal("gap retained a stale success preview")
	}
}

type finalStageClient struct {
	ConsoleClient
	queries *finalReaderQueries
}

func (c finalStageClient) Conversation(ctx context.Context, filter api.ConversationFilter, cursor string) (api.ConversationPage, error) {
	return c.queries.Conversation(ctx, filter, cursor)
}

func (c finalStageClient) Body(ctx context.Context, ref api.ConversationBodyRef, offset int64, limit int) (api.ConversationBodyPage, error) {
	return c.queries.Body(ctx, ref, offset, limit)
}

func TestFinalStageQueryPreservesReportAndHistoryScopes(t *testing.T) {
	for _, mode := range []string{"report", "report-without-implementation", "history", "publish-report"} {
		t.Run(mode, func(t *testing.T) {
			m, _ := ccFixture(t, 1)
			run := m.selectedRow(0).run
			run.CurrentTurnID, run.StepID = "report-turn", "implement.report"
			run.StageResults = []api.StageResultSummary{{StepID: "implement.execute", TurnID: "old-implementation"}, {StepID: "implement.execute", TurnID: "original-implementation"}}
			if mode == "publish-report" {
				run.StepID = "publish.report"
				run.StageResults = append(run.StageResults, api.StageResultSummary{StepID: "publish", TurnID: "original-publication"})
			}
			if mode == "report-without-implementation" {
				run.StageResults = nil
			}
			if mode == "history" {
				m.detailExpanded, m.detailSection = true, "final"
			}
			q := &finalReaderQueries{}
			m.projects[0].client = finalStageClient{m.projects[0].client, q}
			_, cmd := m.refreshFinalReplies(0)
			cmd()
			wantTurn, wantAttempt := "original-implementation", run.AttemptID
			if mode == "report-without-implementation" {
				wantTurn = "report-turn"
			}
			if mode == "publish-report" {
				wantTurn = "original-publication"
			}
			if mode == "history" {
				wantTurn, wantAttempt = "", ""
			}
			if len(q.filters) != 1 || q.filters[0].TurnID != wantTurn || q.filters[0].AttemptID != wantAttempt || q.filters[0].ExecutionID != run.ExecutionID || !q.filters[0].FinalsOnly {
				t.Fatal("host changed the stage query", q.filters)
			}
		})
	}
}
