package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	queryapi "github.com/erranliu/SteerLine/internal/steerline/query"
)

func TestConversationSQLiteDedupesBodiesAndPagesUnicodeByRevision(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	text := strings.Repeat("SteerLine 你好 🙂 ", 500)
	first := conversationTestWrite("bytes:0:4", 1, "delta", []byte(text), []byte("raw-record-one"))
	checkpoint := conversationTestCheckpoint(1, `{"byte_offset":4}`)
	committed, err := store.CommitConversation(ctx, []ConversationWrite{first}, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if len(committed) != 1 || committed[0].Body == nil || committed[0].Body.Bytes != int64(len(text)) || committed[0].Body.Completeness != contracts.BodyPartial {
		t.Fatalf("body reference is incomplete: %+v", committed)
	}
	duplicate, err := store.CommitConversation(ctx, []ConversationWrite{first}, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if len(duplicate) != 1 || duplicate[0].EventID != committed[0].EventID || duplicate[0].Body.Revision != committed[0].Body.Revision {
		t.Fatalf("source replay was not idempotent: first=%+v replay=%+v", committed, duplicate)
	}

	query := conversationQueries(store, "project")
	var restored strings.Builder
	offset := int64(0)
	for {
		page, readErr := query.ReadMessageBody(ctx, *committed[0].Body, offset, 11)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !utf8.Valid(page.Data) {
			t.Fatal("body page contains invalid UTF-8")
		}
		restored.Write(page.Data)
		offset = page.NextOffset
		if page.Complete {
			if page.SHA256 != committed[0].Body.SHA256 || page.Completeness != contracts.BodyPartial {
				t.Fatalf("body page revision metadata changed: %+v", page)
			}
			break
		}
		if offset == 0 {
			t.Fatal("body paging did not advance")
		}
	}
	if restored.String() != text {
		t.Fatalf("multi-page Unicode body changed: got %d bytes, want %d", restored.Len(), len(text))
	}
	wholeDigest := sha256.Sum256([]byte(text))
	if hex.EncodeToString(wholeDigest[:]) != committed[0].Body.SHA256 {
		t.Fatal("stored body SHA-256 does not match the source text")
	}
	rawPage, err := query.ReadRaw(ctx, *committed[0].Raw, 0, 64)
	if err != nil || string(rawPage.Data) != "raw-record-one" || !rawPage.Complete {
		t.Fatalf("raw reference was not readable: page=%+v err=%v", rawPage, err)
	}

	delta := conversationTestWrite("bytes:4:8", 2, "delta", []byte(" tail"), []byte("raw-record-two"))
	updated, err := store.CommitConversation(ctx, []ConversationWrite{delta}, conversationTestCheckpoint(2, `{"byte_offset":8}`))
	if err != nil {
		t.Fatal(err)
	}
	if updated[0].Body.Revision != committed[0].Body.Revision+1 || updated[0].Body.Completeness != contracts.BodyPartial {
		t.Fatalf("delta was not stored as a new body revision: %+v", updated[0].Body)
	}
	repeatedText := conversationTestWrite("bytes:8:12", 3, "delta", []byte(" tail"), []byte("raw-record-three"))
	repeated, err := store.CommitConversation(ctx, []ConversationWrite{repeatedText}, conversationTestCheckpoint(3, `{"byte_offset":12}`))
	if err != nil {
		t.Fatal(err)
	}
	if repeated[0].Body.Revision != updated[0].Body.Revision+1 || repeated[0].Body.Bytes != updated[0].Body.Bytes+int64(len(" tail")) {
		t.Fatalf("a real repeated text event was removed by content dedupe: %+v", repeated[0].Body)
	}
	snapshot := conversationTestWrite("bytes:12:16", 4, "snapshot", []byte("replacement"), []byte("raw-record-four"))
	snapshot.Event.Type = contracts.ConversationMessageSnapshot
	replaced, err := store.CommitConversation(ctx, []ConversationWrite{snapshot}, conversationTestCheckpoint(4, `{"byte_offset":16}`))
	if err != nil {
		t.Fatal(err)
	}
	if replaced[0].Body.Revision != repeated[0].Body.Revision+1 {
		t.Fatalf("snapshot did not create a distinct revision: %+v", replaced[0].Body)
	}
	oldSnapshot := conversationTestWrite("bytes:1:3", 1, "snapshot", []byte("stale"), []byte("raw-stale"))
	oldSnapshot.Event.Type = contracts.ConversationMessageSnapshot
	conflict, err := store.CommitConversation(ctx, []ConversationWrite{oldSnapshot}, conversationTestCheckpoint(5, `{"byte_offset":20}`))
	if err != nil {
		t.Fatal(err)
	}
	if conflict[0].Diagnostic == "" || conflict[0].Body.SHA256 != replaced[0].Body.SHA256 {
		t.Fatalf("late older snapshot replaced the known body: %+v", conflict[0])
	}

	page, err := query.ReadConversation(ctx, ConversationFilter{ExecutionID: "execution", AttemptID: "attempt"}, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 2 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("conversation query did not return a bounded page: %+v", page)
	}
	if _, err = query.ReadConversation(ctx, ConversationFilter{ExecutionID: "other", AttemptID: "attempt"}, page.NextCursor, 2); err == nil {
		t.Fatal("query cursor crossed execution filters")
	}
	remainder, err := query.ReadConversation(ctx, ConversationFilter{ExecutionID: "execution", AttemptID: "attempt"}, page.NextCursor, 10)
	if err != nil || len(remainder.Events) == 0 {
		t.Fatalf("could not read the remaining conversation page: %+v %v", remainder, err)
	}
	tailCursor, err := queryapi.ConversationCursorAfter("project", ConversationFilter{ExecutionID: "execution", AttemptID: "attempt"}, remainder.Events[len(remainder.Events)-1].EventID)
	if err != nil {
		t.Fatal(err)
	}
	tail, err := query.ReadConversation(ctx, ConversationFilter{ExecutionID: "execution", AttemptID: "attempt"}, tailCursor, 10)
	if err != nil || len(tail.Events) != 0 {
		t.Fatalf("tail cursor did not resume after the latest event: %+v %v", tail, err)
	}
	if _, err = query.ReadRaw(ctx, ConversationRawRef{ExecutionID: "other", AttemptID: "attempt", ID: committed[0].Raw.ID}, 0, 10); err == nil {
		t.Fatal("raw reference crossed execution identity")
	}
}

func TestConversationCommitDoesNotAdvanceCheckpointOnInvalidBody(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	write := conversationTestWrite("bytes:0:4", 1, "complete", []byte{0xff}, []byte("raw-before-error"))
	if _, err = store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, `{"byte_offset":4}`)); err == nil {
		t.Fatal("invalid body unexpectedly committed")
	}
	page, err := conversationQueries(store, "project").ReadConversation(ctx, ConversationFilter{ExecutionID: "execution", AttemptID: "attempt"}, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 0 {
		t.Fatalf("event committed without its valid body: %+v", page.Events)
	}
	if _, err = store.ConversationCheckpoint(ctx, "project", "execution", "attempt", "fixture-journal", "segment-1"); err == nil {
		t.Fatal("checkpoint advanced after the event transaction failed")
	}
}

func TestConversationBodyAndRawCorruptionCannotReadAsComplete(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "steerline.db"))
	require(t, err)
	defer store.Close()
	write := conversationTestWrite("bytes:0:4", 1, "complete", []byte("hello 🙂"), []byte("raw-record"))
	events, err := store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, `{"byte_offset":4}`))
	require(t, err)
	query := conversationQueries(store, "project")
	if page, err := query.ReadMessageBody(ctx, *events[0].Body, 0, 64); err != nil || !page.Complete {
		t.Fatalf("intact body was unreadable: page=%+v err=%v", page, err)
	}
	_, err = store.db.ExecContext(ctx, `UPDATE conversation_bodies SET body=? WHERE project_identity=? AND execution_id=? AND attempt_id=? AND item_key=? AND revision=?`, []byte("HELLO 🙂"), "project", "execution", "attempt", events[0].Body.ItemKey, events[0].Body.Revision)
	require(t, err)
	if page, err := query.ReadMessageBody(ctx, *events[0].Body, 0, 64); err == nil || page.Complete {
		t.Fatalf("modified body was reported complete: page=%+v err=%v", page, err)
	}
	_, err = store.db.ExecContext(ctx, `UPDATE conversation_bodies SET body=zeroblob(?) WHERE project_identity=? AND execution_id=? AND attempt_id=? AND item_key=? AND revision=?`, maxConversationBodyBytes+1, "project", "execution", "attempt", events[0].Body.ItemKey, events[0].Body.Revision)
	require(t, err)
	if page, err := query.ReadMessageBody(ctx, *events[0].Body, 0, 64); err == nil || page.Complete {
		t.Fatalf("oversized corrupt body was reported complete: page=%+v err=%v", page, err)
	}
	_, err = store.db.ExecContext(ctx, `UPDATE conversation_raw SET payload=? WHERE project_identity=? AND execution_id=? AND attempt_id=? AND raw_id=?`, []byte("RAW-record"), "project", "execution", "attempt", events[0].Raw.ID)
	require(t, err)
	if page, err := query.ReadRaw(ctx, *events[0].Raw, 0, 64); err == nil || page.Complete {
		t.Fatalf("modified raw record was reported complete: page=%+v err=%v", page, err)
	}
	_, err = store.db.ExecContext(ctx, `UPDATE conversation_raw SET payload=zeroblob(?) WHERE project_identity=? AND execution_id=? AND attempt_id=? AND raw_id=?`, maxConversationBodyBytes+1, "project", "execution", "attempt", events[0].Raw.ID)
	require(t, err)
	if page, err := query.ReadRaw(ctx, *events[0].Raw, 0, 64); err == nil || page.Complete {
		t.Fatalf("oversized corrupt raw record was reported complete: page=%+v err=%v", page, err)
	}
	_, err = store.db.ExecContext(ctx, `DELETE FROM conversation_bodies WHERE project_identity=? AND execution_id=? AND attempt_id=? AND item_key=? AND revision=?`, "project", "execution", "attempt", events[0].Body.ItemKey, events[0].Body.Revision)
	require(t, err)
	if page, err := query.ReadMessageBody(ctx, *events[0].Body, 0, 64); err == nil || page.Complete {
		t.Fatalf("missing body was reported complete: page=%+v err=%v", page, err)
	}
}

func TestConversationLongUnicodeFinalIsExactAcrossPages(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "steerline.db"))
	require(t, err)
	defer store.Close()
	final := strings.Repeat("末尾 🙂 SteerLine\n", 900)
	write := conversationTestWrite("final", 1, "complete", []byte(final), []byte("final-source"))
	write.Event.Type = contracts.ConversationMessageComplete
	write.Event.Phase = contracts.MessagePhaseFinalAnswer
	events, err := store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, "final-cursor"))
	require(t, err)
	ref := events[0].Body
	if ref == nil || ref.Bytes <= 4096 || ref.Completeness != contracts.BodyComplete {
		t.Fatalf("final body was truncated or partial: %+v", ref)
	}
	query := conversationQueries(store, "project")
	var restored bytes.Buffer
	for offset := int64(0); ; {
		page, err := query.ReadMessageBody(ctx, *ref, offset, 257)
		require(t, err)
		if !utf8.Valid(page.Data) || page.NextOffset <= offset || page.Completeness != contracts.BodyComplete {
			t.Fatalf("Unicode final page is invalid: %+v", page)
		}
		restored.Write(page.Data)
		offset = page.NextOffset
		if page.Complete {
			break
		}
	}
	actual := sha256.Sum256(restored.Bytes())
	if restored.String() != final || hex.EncodeToString(actual[:]) != ref.SHA256 {
		t.Fatal("final pages did not reconstruct the exact source body")
	}
}

func TestConversationWriteFailureRollsBackContentAndCursor(t *testing.T) {
	ctx := context.Background()
	for _, table := range []string{"conversation_raw", "conversation_bodies", "conversation_current_messages", "conversation_events", "conversation_source_cursors"} {
		t.Run(table, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "steerline.db")
			store, err := OpenSQLiteStore(path)
			require(t, err)
			_, err = store.db.ExecContext(ctx, fmt.Sprintf(`CREATE TRIGGER block_conversation_write BEFORE INSERT ON %s BEGIN SELECT RAISE(ABORT, 'injected write failure'); END`, table))
			require(t, err)
			write := conversationTestWrite("bytes:0:4", 1, "complete", []byte("final"), []byte("raw-record"))
			if _, err = store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, `{"byte_offset":4}`)); err == nil {
				t.Fatal("injected failure unexpectedly committed")
			}
			require(t, store.Close())
			store, err = OpenSQLiteStore(path)
			require(t, err)
			defer store.Close()
			for _, relation := range []string{"conversation_raw", "conversation_bodies", "conversation_current_messages", "conversation_events", "conversation_source_cursors"} {
				var count int
				require(t, store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+relation).Scan(&count))
				if count != 0 {
					t.Fatalf("%s survived failed transaction at %s: %d", relation, table, count)
				}
			}
			_, err = store.db.ExecContext(ctx, `DROP TRIGGER block_conversation_write`)
			require(t, err)
			first, err := store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, `{"byte_offset":4}`))
			require(t, err)
			replayed, err := store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, `{"byte_offset":4}`))
			require(t, err)
			if len(first) != 1 || len(replayed) != 1 || first[0].EventID != replayed[0].EventID {
				t.Fatalf("recovery duplicated source event: first=%+v replay=%+v", first, replayed)
			}
		})
	}
}

func TestConversationSQLiteFullRollsBackContentAndCursor(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "steerline.db")
	store, err := OpenSQLiteStore(path)
	require(t, err)
	store.db.SetMaxIdleConns(1)
	var pages int
	require(t, store.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages))
	var pageLimit int
	require(t, store.db.QueryRowContext(ctx, fmt.Sprintf(`PRAGMA max_page_count=%d`, pages+1)).Scan(&pageLimit))
	if pageLimit != pages+1 {
		t.Fatalf("SQLite page limit was not applied: got %d, want %d", pageLimit, pages+1)
	}
	write := conversationTestWrite("bytes:0:4", 1, "complete", bytes.Repeat([]byte("a"), 256<<10), []byte("raw-record"))
	if _, err = store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, `{"byte_offset":4}`)); err == nil {
		t.Fatal("SQLite full condition unexpectedly committed")
	}
	var count int
	require(t, store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversation_events`).Scan(&count))
	if count != 0 {
		t.Fatal("event survived SQLite full condition")
	}
	if _, err = store.ConversationCheckpoint(ctx, "project", "execution", "attempt", "fixture-journal", "segment-1"); err == nil {
		t.Fatal("checkpoint advanced after SQLite full condition")
	}
	require(t, store.Close())
	store, err = OpenSQLiteStore(path)
	require(t, err)
	defer store.Close()
	events, err := store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, `{"byte_offset":4}`))
	require(t, err)
	if len(events) != 1 || events[0].Body == nil {
		t.Fatalf("recovered write lost body: %+v", events)
	}
}

func TestConversationLateOldAttemptCannotCrossNewAttempt(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "steerline.db"))
	require(t, err)
	defer store.Close()
	old := conversationTestWrite("source-event", 1, "complete", []byte("old attempt"), []byte("old source"))
	newer := conversationTestWrite("source-event", 1, "complete", []byte("new attempt"), []byte("new source"))
	newer.Event.AttemptID = "attempt-new"
	newer.Event.ItemKey = conversationItemKey(newer.Event, newer.Event.ItemID)
	newCheckpoint := conversationTestCheckpoint(1, "new-cursor")
	newCheckpoint.AttemptID = "attempt-new"
	newEvents, err := store.CommitConversation(ctx, []ConversationWrite{newer}, newCheckpoint)
	require(t, err)
	oldEvents, err := store.CommitConversation(ctx, []ConversationWrite{old}, conversationTestCheckpoint(1, "old-cursor"))
	require(t, err)
	if len(newEvents) != 1 || len(oldEvents) != 1 || newEvents[0].EventID == oldEvents[0].EventID || newEvents[0].Body.ItemKey == oldEvents[0].Body.ItemKey {
		t.Fatalf("late old event reused the new attempt identity: old=%+v new=%+v", oldEvents, newEvents)
	}
	query := conversationQueries(store, "project")
	for _, tc := range []struct {
		attempt string
		ref     ConversationBodyRef
		want    string
	}{{"attempt", *oldEvents[0].Body, "old attempt"}, {"attempt-new", *newEvents[0].Body, "new attempt"}} {
		page, err := query.ReadConversation(ctx, ConversationFilter{ExecutionID: "execution", AttemptID: tc.attempt}, "", 10)
		require(t, err)
		if len(page.Events) != 1 || page.Events[0].AttemptID != tc.attempt {
			t.Fatalf("attempt filter crossed identities: %+v", page.Events)
		}
		body, err := query.ReadMessageBody(ctx, tc.ref, 0, 100)
		require(t, err)
		if string(body.Data) != tc.want || !body.Complete {
			t.Fatalf("attempt body was overwritten: %+v", body)
		}
		checkpoint, err := store.ConversationCheckpoint(ctx, "project", "execution", tc.attempt, "fixture-journal", "segment-1")
		require(t, err)
		if checkpoint.LocalSeq != 1 {
			t.Fatalf("attempt cursor changed: %+v", checkpoint)
		}
	}
}

func TestMain(m *testing.M) {
	if stage := os.Getenv("STEERLINE_CONVERSATION_CRASH_STAGE"); stage != "" {
		err := conversationProcessExitAtBoundary(stage)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func conversationProcessExitAtBoundary(stage string) error {
	store, err := OpenSQLiteStore(os.Getenv("STEERLINE_CONVERSATION_CRASH_DB"))
	if err != nil {
		return err
	}
	if stage == "committed" {
		write := conversationTestWrite("bytes:0:4", 1, "complete", []byte("recovered final"), []byte("source"))
		_, err = store.CommitConversation(context.Background(), []ConversationWrite{write}, conversationTestCheckpoint(1, "cursor"))
		if err != nil {
			return err
		}
		os.Exit(27)
	}
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	steps := []struct {
		name  string
		query string
	}{
		{"raw", `INSERT INTO conversation_raw(project_identity,execution_id,attempt_id,raw_id,source_event_key,payload,sha256,byte_count,created_at) VALUES('project','execution','attempt','raw','source-event',X'61','digest',1,'now')`},
		{"body", `INSERT INTO conversation_bodies(project_identity,execution_id,attempt_id,item_key,revision,body,sha256,byte_count,source,completeness,created_at) VALUES('project','execution','attempt','item',1,X'61','digest',1,'source','complete','now')`},
		{"event", fmt.Sprintf(`INSERT INTO conversation_events(project_identity,execution_id,attempt_id,source_event_key,source,source_segment,event_type,received_at,payload_bytes,payload) VALUES('project','execution','attempt','source-event','%s','segment-1','message.complete','now',2,X'7b7d')`, "fixture-journal")},
		{"cursor", fmt.Sprintf(`INSERT INTO conversation_source_cursors(project_identity,execution_id,attempt_id,source,source_segment,local_seq,updated_at) VALUES('project','execution','attempt','%s','segment-1',1,'now')`, "fixture-journal")},
	}
	for _, step := range steps {
		_, err = tx.ExecContext(context.Background(), step.query)
		if err != nil {
			return err
		}
		if step.name == stage {
			os.Exit(27)
		}
	}
	return fmt.Errorf("unknown crash stage %q", stage)
}

func TestConversationProcessExitBeforeCommitRecoversAllBoundaries(t *testing.T) {
	ctx := context.Background()
	for _, stage := range []string{"raw", "body", "event", "cursor", "committed"} {
		t.Run(stage, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "steerline.db")
			cmd := exec.Command(os.Args[0], "-test.run=^$")
			cmd.Env = append(os.Environ(), "STEERLINE_CONVERSATION_CRASH_STAGE="+stage, "STEERLINE_CONVERSATION_CRASH_DB="+path)
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 27 {
				t.Fatalf("child did not exit at %s boundary: err=%v output=%s", stage, err, output)
			}
			store, err := OpenSQLiteStore(path)
			require(t, err)
			defer store.Close()
			want := 0
			if stage == "committed" {
				want = 1
			}
			for _, relation := range []string{"conversation_raw", "conversation_bodies", "conversation_events", "conversation_source_cursors"} {
				var count int
				require(t, store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+relation).Scan(&count))
				if count != want {
					t.Fatalf("%s has %d rows after process exit at %s, want %d", relation, count, stage, want)
				}
			}
			write := conversationTestWrite("bytes:0:4", 1, "complete", []byte("recovered final"), []byte("source"))
			first, err := store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, "cursor"))
			require(t, err)
			replayed, err := store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, "cursor"))
			require(t, err)
			if len(first) != 1 || len(replayed) != 1 || first[0].EventID != replayed[0].EventID {
				t.Fatalf("recovery duplicated the event: first=%+v replay=%+v", first, replayed)
			}
		})
	}
}

func TestConversationCommitRejectsChangedRawAtSameSourceIdentity(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "steerline.db"))
	require(t, err)
	defer store.Close()
	first := conversationTestWrite("bytes:0:4", 1, "complete", []byte("first"), []byte("original source bytes"))
	_, err = store.CommitConversation(ctx, []ConversationWrite{first}, conversationTestCheckpoint(1, `{"byte_offset":4}`))
	require(t, err)
	changed := first
	changed.Raw = []byte("rewritten source bytes")
	if _, err = store.CommitConversation(ctx, []ConversationWrite{changed}, conversationTestCheckpoint(2, `{"byte_offset":8}`)); err == nil {
		t.Fatal("rewritten bytes reused one source event identity")
	}
	checkpoint, err := store.ConversationCheckpoint(ctx, "project", "execution", "attempt", "fixture-journal", "segment-1")
	require(t, err)
	if checkpoint.LocalSeq != 1 {
		t.Fatal("source cursor advanced after conflicting raw bytes")
	}
	changed = first
	changed.Event.Type = contracts.ConversationGap
	changed.Body = nil
	changed.BodyMode = ""
	if _, err = store.CommitConversation(ctx, []ConversationWrite{changed}, conversationTestCheckpoint(2, `{"byte_offset":8}`)); err == nil {
		t.Fatal("changed event semantics reused one source identity")
	}
}

func TestConversationCheckpointPreflightRejectsUnanchoredState(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "steerline.db"))
	require(t, err)
	defer store.Close()
	write := conversationTestWrite("bytes:0:4", 1, "complete", []byte("body"), []byte("raw"))
	_, err = store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, `{"byte_offset":4}`))
	require(t, err)
	if err = store.ValidateConversationCheckpointFormat(ctx, "project", []string{"fixture-journal"}); err == nil || !strings.Contains(err.Error(), "unsupported conversation checkpoint") {
		t.Fatalf("unanchored old state was accepted: %v", err)
	}
}

func conversationTestWrite(sourceID string, seq uint64, mode string, body, raw []byte) ConversationWrite {
	event := ConversationEvent{
		Version: contracts.ConversationVersion, ProjectID: "project", TaskID: "task", WorkflowID: "workflow",
		ExecutionID: "execution", AttemptID: "attempt", Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess,
		Agent: "fixture", TargetID: "local-test", Source: "fixture-journal", SourceRole: contracts.ConversationSourceFallback, SourceSegment: "segment-1",
		SessionID: "session", SessionQuality: contracts.IdentityNative, TurnID: "turn", TurnQuality: contracts.IdentityDerived,
		ItemID: "same-item", ItemQuality: contracts.IdentityNative, SourceEventID: sourceID, SourceEventQuality: contracts.IdentityDerived,
		SourceSeq: seq, Type: contracts.ConversationMessageDelta, Phase: contracts.MessagePhaseUnknown, ReceivedAt: time.Now().UTC(),
		Evidence: map[string]string{"identity_rule": "test-byte-range"},
	}
	event.ItemKey = conversationItemKey(event, event.ItemID)
	return ConversationWrite{Event: event, Body: body, BodyMode: mode, Raw: raw, BodySource: "test-source"}
}

func conversationTestCheckpoint(seq uint64, cursor string) ConversationCheckpoint {
	return ConversationCheckpoint{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Source: "fixture-journal", SourceSegment: "segment-1", SourceIdentity: "local-test", Generation: "1", Cursor: cursor, LocalSeq: seq, UpdatedAt: time.Now().UTC()}
}
