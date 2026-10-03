package steerline

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

func TestConversationLiveSourceFailureKeepsGapAndMapperPosition(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runID := newID()
	path := filepath.Join(root, "runs", runID, "events.jsonl")
	require(t, os.MkdirAll(filepath.Dir(path), 0700))
	firstRaw := []byte(`{"kind":"session","session":"source-session"}` + "\n")
	require(t, os.WriteFile(path, firstRaw, 0600))
	file, fileID, err := openJournal(path)
	require(t, err)
	require(t, file.Close())
	store, err := openSQLiteFixture(filepath.Join(root, "steerline.db"))
	require(t, err)
	defer store.Close()
	executionID, taskID := newID(), "501"
	run := &Run{TaskID: taskID, RunID: runID, AttemptID: runID, ExecutionID: executionID, DomainTaskID: taskID, WorkflowVersion: "minimal-v1", Backend: string(executionapi.BackendLocal), Protocol: string(executionapi.ProtocolLocalProcess), Agent: "fixture", Target: executionapi.ExecutionTarget{ID: "local"}, LogPath: &path}
	port := neutralJournalPort{}
	s := &Service{Config: Config{Persistence: sqlite.Factory{}, RunDir: root, identity: "project"}, conversationSources: port, domainStore: store, commandStore: store, conversationStore: store, State: Runtime{}, runs: []*Run{run}}
	sourceName := port.JournalSource(executionapi.BackendLocal, executionapi.ProtocolLocalProcess)
	segment := port.JournalSegment(executionapi.BackendLocal, executionapi.ProtocolLocalProcess, fileID)
	first := WorkerEvent{RunID: runID, AttemptID: runID, Source: sourceName, SourceSegment: segment, SourceIdentity: fileID, SourceSeq: 1, SourceOffset: 0, SourceLength: int64(len(firstRaw)), Raw: firstRaw}
	require(t, s.recordConversationEvent(run, first))
	secondRaw := []byte(`{"kind":"turn"}` + "\n")
	second := first
	second.Raw, second.SourceSeq, second.SourceOffset, second.SourceLength = secondRaw, 2, int64(len(firstRaw)), int64(len(secondRaw))
	if err = s.recordConversationEvent(run, second); err == nil {
		t.Fatal("source beyond the durable file was committed")
	}
	mapper, err := store.ConversationMapperSnapshot(ctx, "project", executionID, runID, sourceName, segment)
	require(t, err)
	if mapper.Turn != nil {
		t.Fatal("failed source read advanced mapper state derived from committed events")
	}
	cp, err := store.ConversationCheckpoint(ctx, "project", executionID, runID, sourceName, segment)
	require(t, err)
	if cp.LocalSeq != 1 {
		t.Fatal("source checkpoint advanced after a failed read")
	}
	page, err := NewConversationQueryService(store, "project").ReadConversation(ctx, ConversationFilter{ExecutionID: executionID, AttemptID: runID}, "", 10)
	require(t, err)
	if len(page.Events) != 2 || page.Events[1].Type != ConversationGap || page.Events[1].Status != "source_unavailable" || page.Events[1].Raw == nil {
		t.Fatalf("failed source was not visible with raw evidence: %+v", page.Events)
	}
}

func TestConversationChangedLiveBytesCannotBindOrCheckpoint(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runID := newID()
	executionID, taskID := newID(), "502"
	path := filepath.Join(root, "runs", runID, "events.jsonl")
	require(t, os.MkdirAll(filepath.Dir(path), 0700))
	actualSession := "00000000-0000-0000-0000-000000000001"
	forgedSession := "00000000-0000-0000-0000-000000000002"
	actual := []byte(`{"kind":"session","session":"` + actualSession + `"}` + "\n")
	forged := []byte(`{"kind":"session","session":"` + forgedSession + `"}` + "\n")
	if len(actual) != len(forged) {
		t.Fatal("same-length source rewrite fixture is invalid")
	}
	require(t, os.WriteFile(path, actual, 0600))
	file, fileID, err := openJournal(path)
	require(t, err)
	require(t, file.Close())
	port := neutralJournalPort{}
	sourceName := port.JournalSource(executionapi.BackendLocal, executionapi.ProtocolLocalProcess)
	segment := port.JournalSegment(executionapi.BackendLocal, executionapi.ProtocolLocalProcess, fileID)
	store, err := openSQLiteFixture(filepath.Join(root, "steerline.db"))
	require(t, err)
	defer store.Close()
	run := &Run{TaskID: taskID, RunID: runID, AttemptID: runID, ExecutionID: executionID, DomainTaskID: taskID, WorkflowVersion: "minimal-v1", Backend: string(executionapi.BackendLocal), Protocol: string(executionapi.ProtocolLocalProcess), Agent: "fixture", Target: executionapi.ExecutionTarget{ID: "local"}, WorktreePath: &root, LogPath: &path}
	service := &Service{Config: Config{Persistence: sqlite.Factory{}, RunDir: root, identity: "project"}, conversationSources: port, domainStore: store, commandStore: store, conversationStore: store}
	source := WorkerEvent{RunID: runID, AttemptID: runID, Source: sourceName, SourceSegment: segment, SourceIdentity: fileID, SourceSeq: 1, SourceOffset: 0, SourceLength: int64(len(forged)), Raw: forged, Session: forgedSession}
	if err = service.recordConversationEvent(run, source); err == nil || !strings.Contains(err.Error(), "source bytes changed") {
		t.Fatalf("rewritten source bytes were accepted: %v", err)
	}
	if _, err = store.ConversationCheckpoint(ctx, "project", executionID, runID, sourceName, segment); err == nil {
		t.Fatal("rewritten source advanced the checkpoint")
	}
	page, err := NewConversationQueryService(store, "project").ReadConversation(ctx, ConversationFilter{ExecutionID: executionID, AttemptID: runID, Source: sourceName}, "", 10)
	require(t, err)
	if len(page.Events) != 1 || page.Events[0].Type != ConversationGap || page.Events[0].Status != "source_unavailable" {
		t.Fatalf("rewritten source did not leave a visible gap: %+v", page.Events)
	}
}

func TestConversationJournalReplayIsBoundedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	runID := "0123456789abcdef0123456789abcdef"
	attemptID := "abcdef0123456789abcdef0123456789"
	executionID, taskID := newID(), "504"
	projectRoot := t.TempDir()
	runRoot := filepath.Join(projectRoot, "runs", runID)
	if err := os.MkdirAll(runRoot, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runRoot, "events.jsonl")
	lines := []string{
		`{"kind":"session","session":"00000000-0000-0000-0000-000000000001"}`,
		`{"kind":"turn"}`,
		`{"kind":"message","id":"item-a","text":"` + strings.Repeat("长文 🙂 ", 1000) + `"}`,
		`{"kind":"completed"}`,
	}
	contents := strings.Join(lines, "\r\n") + "\r\n"
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	sourceFile, fileID, err := openJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = sourceFile.Close(); err != nil {
		t.Fatal(err)
	}
	require(t, AtomicWrite(filepath.Join(runRoot, "fixture-source.json"), mustJSON(neutralJournalMarker{ProjectID: "project", WorkflowID: runID, ExecutionID: executionID, AttemptID: attemptID, SourceIdentity: fileID})))

	store, err := openSQLiteFixture(filepath.Join(projectRoot, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	serviceConfig := Config{Persistence: sqlite.Factory{}, RunDir: projectRoot, identity: "project"}
	service := &Service{Config: serviceConfig, conversationSources: neutralJournalPort{}, domainStore: store, commandStore: store, conversationStore: store, State: Runtime{}, domain: DomainSnapshot{Attempts: []StepAttemptRecord{{ProjectIdentity: "project", AttemptID: attemptID, ExecutionID: executionID, StepID: "implement", EventPath: path, Status: "succeeded"}}}, runs: []*Run{{TaskID: taskID, RunID: runID, AttemptID: attemptID, ExecutionID: executionID, DomainTaskID: taskID, WorkflowVersion: "minimal-v1", Backend: string(executionapi.BackendLocal), Protocol: string(executionapi.ProtocolLocalProcess), Agent: "fixture", Target: executionapi.ExecutionTarget{ID: "local-test"}, WorktreePath: &projectRoot, LogPath: &path, EndedAt: ptr(time.Now().UTC())}}}
	oldRunID := newID()
	oldRoot := filepath.Join(projectRoot, "runs", oldRunID)
	if err = os.MkdirAll(oldRoot, 0700); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(oldRoot, "events.jsonl")
	if err = os.WriteFile(oldPath, []byte(`{"kind":"session","session":"old-development-session"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	service.State.Runs = append(service.State.Runs, &Run{TaskID: "505", RunID: oldRunID, AttemptID: oldRunID, ExecutionID: newID(), DomainTaskID: "505", WorkflowVersion: "minimal-v1", Backend: string(executionapi.BackendLocal), Protocol: string(executionapi.ProtocolLocalProcess), Agent: "fixture", Target: executionapi.ExecutionTarget{ID: "local-test"}, LogPath: &oldPath, EndedAt: ptr(time.Now().UTC())})
	sources := service.conversationJournalSources()
	if len(sources) != 1 {
		t.Fatalf("expected one validated local source, got %+v", sources)
	}
	if err = service.replayConversationJournal(ctx, sources[0]); err != nil {
		t.Fatal(err)
	}
	query := NewConversationQueryService(store, "project")
	page, err := query.ReadConversation(ctx, ConversationFilter{ExecutionID: executionID, AttemptID: attemptID}, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 4 {
		t.Fatalf("replay returned %d events, want 4: %+v", len(page.Events), page.Events)
	}
	var bodyRef *ConversationBodyRef
	for i := range page.Events {
		if page.Events[i].Type == ConversationMessageComplete {
			bodyRef = page.Events[i].Body
			if page.Events[i].SessionID != "00000000-0000-0000-0000-000000000001" || page.Events[i].TurnQuality != IdentityDerived {
				t.Fatalf("replayed event identity was not reconstructed from source boundaries: %+v", page.Events[i])
			}
		}
	}
	if bodyRef == nil || bodyRef.Bytes <= activityDetailMax {
		t.Fatalf("replay lost long message content: %+v", bodyRef)
	}
	if err = service.replayConversationJournal(ctx, sources[0]); err != nil {
		t.Fatal(err)
	}
	page, err = query.ReadConversation(ctx, ConversationFilter{ExecutionID: executionID, AttemptID: attemptID}, "", 10)
	if err != nil || len(page.Events) != 4 {
		t.Fatalf("replay was not idempotent: count=%d err=%v", len(page.Events), err)
	}
	journalSource := service.conversationSources.JournalSource(executionapi.BackendLocal, executionapi.ProtocolLocalProcess)
	checkpoint, err := store.ConversationCheckpoint(ctx, "project", executionID, attemptID, journalSource, sources[0].FileID)
	if err == nil {
		// Checkpoints are addressed by the full source segment, not only the file ID.
		t.Fatalf("checkpoint lookup unexpectedly accepted a partial segment: %+v", checkpoint)
	}
	segment := service.conversationSources.JournalSegment(executionapi.BackendLocal, executionapi.ProtocolLocalProcess, sources[0].FileID)
	checkpoint, err = store.ConversationCheckpoint(ctx, "project", executionID, attemptID, journalSource, segment)
	if err != nil || checkpoint.LocalSeq != 4 {
		t.Fatalf("source cursor did not reach the last committed record: checkpoint=%+v err=%v", checkpoint, err)
	}
	position, err := contracts.DecodeConversationJournalPosition(checkpoint)
	if err != nil || position.ByteOffset != int64(len(contents)) {
		t.Fatalf("committed byte cursor was not restorable: %+v err=%v", position, err)
	}
	unchanged, err := ReadJournalBatch(path, journalCursorOf(position, path), JournalOptions{FromStart: true, KeepCR: true, MaxRecords: 256, MaxBytes: JournalMaxBytes})
	if err != nil || unchanged.Reset || len(unchanged.Records) != 0 {
		t.Fatalf("unchanged source was rescanned or reset: %+v err=%v", unchanged, err)
	}
	rewritten := bytes.Replace([]byte(contents), []byte("00000000-0000-0000-0000-000000000001"), []byte("00000000-0000-0000-0000-000000000002"), 1)
	if len(rewritten) != len(contents) {
		t.Fatal("test replacement changed file size")
	}
	require(t, os.WriteFile(path, rewritten, 0600))
	sources = service.conversationJournalSources()
	if len(sources) != 1 {
		t.Fatal("in-place source replacement escaped the bound file")
	}
	require(t, service.replayConversationJournal(ctx, sources[0]))
	changed, err := query.ReadConversation(ctx, ConversationFilter{ExecutionID: executionID, AttemptID: attemptID}, "", 20)
	require(t, err)
	if len(changed.Events) != 9 || changed.Events[4].Type != ConversationGap || changed.Events[4].Status != "source_changed" || changed.Events[5].SessionID != "00000000-0000-0000-0000-000000000002" || changed.Events[2].Body == nil {
		t.Fatalf("in-place source rewrite hid its gap or damaged old history: %+v", changed.Events)
	}
}

func conversationTestWrite(sourceID string, seq uint64, mode string, body, raw []byte) ConversationWrite {
	event := ConversationEvent{
		Version: ConversationVersion, ProjectID: "project", TaskID: "task", WorkflowID: "workflow",
		ExecutionID: "execution", AttemptID: "attempt", Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess,
		Agent: "fixture", TargetID: "local-test", Source: neutralJournalSource, SourceRole: contracts.ConversationSourceFallback, SourceSegment: "segment-1",
		SessionID: "session", SessionQuality: IdentityNative, TurnID: "turn", TurnQuality: IdentityDerived,
		ItemID: "same-item", ItemQuality: IdentityNative, SourceEventID: sourceID, SourceEventQuality: IdentityDerived,
		SourceSeq: seq, Type: ConversationMessageDelta, Phase: MessagePhaseUnknown, ReceivedAt: time.Now().UTC(),
		Evidence: map[string]string{"identity_rule": "test-byte-range"},
	}
	event.ItemKey = conversationItemKey(event, event.ItemID)
	return ConversationWrite{Event: event, Body: body, BodyMode: mode, Raw: raw, BodySource: "test-source"}
}

func conversationTestCheckpoint(seq uint64, cursor string) ConversationCheckpoint {
	return ConversationCheckpoint{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Source: neutralJournalSource, SourceSegment: "segment-1", SourceIdentity: "local-test", Generation: "1", Cursor: cursor, LocalSeq: seq, UpdatedAt: time.Now().UTC()}
}
