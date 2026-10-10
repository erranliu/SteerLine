package model

import (
	"testing"
	"time"
)

func TestWorkerStageRecoveredNativeStartRequiresOriginalIntent(t *testing.T) {
	s := State{WorkflowRunID: "run", TaskID: "task", WorkflowVersion: Version, Status: "reconciling", ExecutionID: "execution", ExecutionWorkflowRunID: "run", SessionID: "session", PendingTurnID: "original", CurrentTurnID: "previous", LastStoppedTurnID: "previous"}
	if next, err := BeginTurn(s, "session", "original", time.Now()); err != nil || next.CurrentTurnID != "original" || next.Status != "running" {
		t.Fatalf("original reconciled start rejected: %+v %v", next, err)
	}
	for _, pending := range []string{"", "other"} {
		s.PendingTurnID = pending
		if _, err := BeginTurn(s, "session", "original", time.Now()); err == nil {
			t.Fatal("unknown outcome authorized an unrelated input")
		}
	}
}
