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
	select {
	case event := <-events:
		if event.Session != "after-large-line" {
			t.Fatalf("later event lost: %+v", event)
		}
	default:
		t.Fatal("later event was not decoded")
	}
}
