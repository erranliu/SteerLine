package execution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
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
	first, err := ReadJournalBatch(path, productapi.JournalCursor{}, JournalOptions{FromStart: true})
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
	second, err := ReadJournalBatch(path, first.Cursor, JournalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Lines) != 1 || string(second.Lines[0]) != line {
		t.Fatalf("completed record not emitted exactly once: %q", second.Lines)
	}
	if second.Cursor.Offset != int64(len(bytes)) {
		t.Fatalf("offset=%d want actual CRLF byte count %d", second.Cursor.Offset, len(bytes))
	}
	third, err := ReadJournalBatch(path, second.Cursor, JournalOptions{})
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
	first, err := ReadJournalBatch(path, productapi.JournalCursor{}, JournalOptions{FromStart: true})
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

func TestJournalFramingDoesNotInterpretActivitySequence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	contents := "{\"seq\":3}\n{\"seq\":3}\n{\"seq\":4}\n{\"seq\":2}\n"
	writeJournal(t, path, contents)
	batch, err := ReadJournalBatch(path, productapi.JournalCursor{}, JournalOptions{FromStart: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Lines) != 4 || batch.Cursor.Generation != 1 || batch.Cursor.Offset != int64(len(contents)) || strings.Join([]string{string(batch.Lines[0]), string(batch.Lines[1]), string(batch.Lines[2]), string(batch.Lines[3])}, "\n")+"\n" != contents {
		t.Fatal("raw sequence fields filtered records or reset the physical cursor")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != contents {
		t.Fatal("journal reader rewrote its raw source")
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
	cursor := productapi.JournalCursor{}
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
	first, err := ReadJournalBatch(path, productapi.JournalCursor{}, JournalOptions{FromStart: false})
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

func TestJournalByteBudgetIsolatesLargeRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	want := []string{"first", strings.Repeat("x", JournalMaxBytes+20), "last"}
	writeJournal(t, path, strings.Join(want, "\n")+"\n")
	cursor := productapi.JournalCursor{}
	var got []string
	for i := 0; i < 10; i++ {
		batch, err := ReadJournalBatch(path, cursor, JournalOptions{FromStart: true})
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, line := range batch.Lines {
			n += len(line)
			got = append(got, string(line))
		}
		if n > JournalMaxBytes && len(batch.Lines) != 1 {
			t.Fatalf("oversize record must own its batch: records=%d bytes=%d", len(batch.Lines), n)
		}
		cursor = batch.Cursor
		if !batch.More {
			break
		}
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("batch boundaries lost records: got %d records", len(got))
	}
}

func TestJournalBoundsUnterminatedOversizeScan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	writeJournal(t, path, strings.Repeat("x", 3*JournalMaxRecord))
	batch, err := ReadJournalBatch(path, productapi.JournalCursor{}, JournalOptions{FromStart: true})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Cursor.ScanOffset > JournalMaxRecord+JournalMaxBytes+1 || !batch.More || !batch.Cursor.Dropping {
		t.Fatalf("oversize scan was not bounded: offset=%d more=%v dropping=%v", batch.Cursor.ScanOffset, batch.More, batch.Cursor.Dropping)
	}
}
