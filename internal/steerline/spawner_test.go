package steerline

import (
	"bytes"
	"io"
	"testing"
)

func TestOversizedWorkerLinePreservesRawOutputAndLaterEvents(t *testing.T) {
	input := append(bytes.Repeat([]byte("x"), (4<<20)+1), '\n')
	input = append(input, []byte("{\"type\":\"thread.started\",\"thread_id\":\"after-large-line\"}\n")...)
	var raw bytes.Buffer
	events := make(chan WorkerEvent, 8)
	if err := streamWorkerEvents(io.TeeReader(bytes.NewReader(input), &raw), NewCodexNormalizer(), events); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw.Bytes(), input) {
		t.Fatal("raw output was truncated")
	}
	first := <-events
	if first.Activity.Kind != "unknown" || len(first.Raw) != (4<<20)+2 || first.Activity.Raw != nil {
		t.Fatalf("large malformed line was not retained without bloating activity: %+v", first.Activity)
	}
	select {
	case event := <-events:
		if event.Session != "after-large-line" {
			t.Fatalf("later event lost: %+v", event)
		}
	default:
		t.Fatal("later event was not decoded")
	}
}

func TestWorkerLineBeyondRawRecordLimitCreatesGap(t *testing.T) {
	input := append(bytes.Repeat([]byte("x"), JournalMaxRecord+1), '\n')
	input = append(input, []byte("{\"type\":\"thread.started\",\"thread_id\":\"after-gap\"}\n")...)
	var raw bytes.Buffer
	events := make(chan WorkerEvent, 4)
	if err := streamWorkerEvents(io.TeeReader(bytes.NewReader(input), &raw), NewCodexNormalizer(), events); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw.Bytes(), input) {
		t.Fatal("raw journal lost oversized bytes")
	}
	gap := <-events
	if !gap.SourceGap || gap.SourceOffset != 0 || gap.SourceLength != JournalMaxRecord+2 || gap.SourceSeq != 1 || len(gap.Raw) != 0 {
		t.Fatalf("oversized record had no bounded gap: %+v", gap)
	}
	next := <-events
	if next.Session != "after-gap" || next.SourceSeq != 2 || next.SourceOffset != gap.SourceLength {
		t.Fatalf("event after oversized gap was misplaced: %+v", next)
	}
}
