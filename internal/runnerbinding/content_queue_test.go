package runnerbinding

import (
	"testing"
	"time"
)

func TestContentQueueFIFOSettlementAndWaitingProjectRemoval(t *testing.T) {
	q := NewContentQueue(1)
	now := time.Now().UTC()
	q.UpdateContent("first", "roadmap:parse", now, false, false)
	q.UpdateContent("second", "roadmap:documents", now.Add(time.Second), false, false)
	if q.AcquireContent("second", "roadmap:documents") || !q.AcquireContent("first", "roadmap:parse") {
		t.Fatal("FIFO bypass")
	}
	// Removing a failed project's waiting work cannot release its active/unknown input.
	q.RemoveWaiting("first")
	if q.AcquireContent("second", "roadmap:documents") {
		t.Fatal("unknown active slot released")
	}
	q.UpdateContent("first", "roadmap:parse", now, true, true)
	if !q.AcquireContent("second", "roadmap:documents") {
		t.Fatal("settled slot retained")
	}
	q.UpdateContent("third", "waiting", now.Add(2*time.Second), false, false)
	q.RemoveWaiting("third")
	q.UpdateContent("second", "roadmap:documents", now, true, true)
	if q.AcquireContent("third", "waiting") {
		t.Fatal("removed waiting request dispatched")
	}
}
