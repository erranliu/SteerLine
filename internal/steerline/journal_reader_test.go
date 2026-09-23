package steerline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeJournal(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestJournalWaitsForCompleteUTF8CRLFRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activity.jsonl")
	line := `{"v":1,"seq":1,"detail":"雪🙂"}`
	bytes := []byte(line + "\r\n")
	cut := strings.Index(line, "雪") + 1
	if err := os.WriteFile(path, bytes[:cut], 0600); err != nil {
		t.Fatal(err)
	}
	first, err := ReadJournalBatch(path, JournalCursor{}, JournalOptions{FromStart: true, Activity: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Lines) != 0 || first.Cursor.Offset != 0 || first.Cursor.ScanOffset != int64(cut) {
		t.Fatalf("partial record advanced or emitted early: lines=%q cursor=%+v", first.Lines, first.Cursor)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(bytes[cut:]); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := ReadJournalBatch(path, first.Cursor, JournalOptions{Activity: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Lines) != 1 || string(second.Lines[0]) != line {
		t.Fatalf("completed record not emitted exactly once: %q", second.Lines)
	}
	if second.Cursor.Offset != int64(len(bytes)) {
		t.Fatalf("offset=%d want actual CRLF byte count %d", second.Cursor.Offset, len(bytes))
	}
	third, err := ReadJournalBatch(path, second.Cursor, JournalOptions{Activity: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Lines) != 0 {
		t.Fatalf("record repeated: %q", third.Lines)
	}
}

func TestJournalResetsOnTruncationAndAtomicReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activity.jsonl")
	writeJournal(t, path, "old-record-one\nold-record-two\n")
	first, err := ReadJournalBatch(path, JournalCursor{}, JournalOptions{FromStart: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Lines) != 2 {
		t.Fatalf("initial lines=%q", first.Lines)
	}
	writeJournal(t, path, "new\n")
	truncated, err := ReadJournalBatch(path, first.Cursor, JournalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !truncated.Reset || len(truncated.Lines) != 1 || string(truncated.Lines[0]) != "new" || truncated.Cursor.Generation == first.Cursor.Generation {
		t.Fatalf("truncation did not start a clean generation: %+v", truncated)
	}
	equalTemp := filepath.Join(filepath.Dir(path), "equal.jsonl")
	writeJournal(t, equalTemp, "old\n")
	if err := os.Rename(equalTemp, path); err != nil {
		t.Fatal(err)
	}
	equalReplace, err := ReadJournalBatch(path, truncated.Cursor, JournalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !equalReplace.Reset || len(equalReplace.Lines) != 1 || string(equalReplace.Lines[0]) != "old" || equalReplace.Cursor.Generation == truncated.Cursor.Generation {
		t.Fatalf("same-size atomic replacement did not reset: %+v", equalReplace)
	}
	oldCursor := equalReplace.Cursor
	tmp := filepath.Join(filepath.Dir(path), "replacement.jsonl")
	writeJournal(t, tmp, "replacement-with-more-bytes\nsecond\n")
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	replaced, err := ReadJournalBatch(path, oldCursor, JournalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !replaced.Reset || len(replaced.Lines) != 2 || !strings.HasPrefix(string(replaced.Lines[0]), "replacement") || replaced.Cursor.Generation == oldCursor.Generation {
		t.Fatalf("atomic replacement did not reset: %+v", replaced)
	}
}

func TestJournalDeduplicatesSeqAndResetsAtRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activity.jsonl")
	rows := []string{
		`{"seq":3,"kind":"message"}`,
		`{"seq":3,"kind":"message duplicate"}`,
		`{"seq":4,"kind":"command"}`,
		`{"seq":2,"kind":"new generation"}`,
	}
	writeJournal(t, path, strings.Join(rows, "\n")+"\n")
	batch, err := ReadJournalBatch(path, JournalCursor{}, JournalOptions{FromStart: true, Activity: true})
	if err != nil {
		t.Fatal(err)
	}
	if !batch.Reset || batch.Cursor.Generation < 2 || len(batch.Lines) != 1 {
		t.Fatalf("seq rollback should reset the logical generation and discard prior batch records: %+v", batch)
	}
	var got struct {
		Seq uint64 `json:"seq"`
	}
	if err := json.Unmarshal(batch.Lines[0], &got); err != nil || got.Seq != 2 {
		t.Fatalf("rollback line=%q err=%v", batch.Lines[0], err)
	}
	if batch.Cursor.Offset != int64(len(strings.Join(rows, "\n")+"\n")) {
		t.Fatalf("rollback moved physical offset backwards: %+v", batch.Cursor)
	}
	next, err := ReadJournalBatch(path, batch.Cursor, JournalOptions{Activity: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Lines) != 0 {
		t.Fatalf("rollback prefix was revisited: %q", next.Lines)
	}
}

func TestJournalBoundsBatchesAndSkipsOversizeLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	var b strings.Builder
	b.WriteString(strings.Repeat("x", JournalMaxRecord+1))
	b.WriteByte('\n')
	for i := 0; i < 600; i++ {
		b.WriteString("row\n")
	}
	writeJournal(t, path, b.String())
	cursor := JournalCursor{}
	all := 0
	first := true
	for {
		batch, err := ReadJournalBatch(path, cursor, JournalOptions{FromStart: first})
		if err != nil {
			t.Fatal(err)
		}
		if len(batch.Lines) > JournalMaxRecords {
			t.Fatalf("oversized record count %d", len(batch.Lines))
		}
		if len(batch.Lines) > 1 {
			bytes := 0
			for _, line := range batch.Lines {
				bytes += len(line)
			}
			if bytes > JournalMaxBytes {
				t.Fatalf("batch exceeded byte bound: %d", bytes)
			}
		}
		if first && len(batch.Diagnostics) != 1 {
			t.Fatalf("oversize diagnostic missing: %+v", batch.Diagnostics)
		}
		all += len(batch.Lines)
		cursor, first = batch.Cursor, false
		if len(batch.Lines) == 0 {
			break
		}
	}
	if all != 600 {
		t.Fatalf("read %d valid lines, want 600", all)
	}
}

func TestJournalFromStartFalseSkipsOldPartialRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	writeJournal(t, path, "old-partial")
	first, err := ReadJournalBatch(path, JournalCursor{}, JournalOptions{FromStart: false})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("-tail\nnew-line\n")
	if err == nil {
		err = f.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	second, err := ReadJournalBatch(path, first.Cursor, JournalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Lines) != 1 || string(second.Lines[0]) != "new-line" {
		t.Fatalf("from-start=false must not emit half of an old partial line: %+v", second)
	}
}
