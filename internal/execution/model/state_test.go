package model

import (
	"testing"
	"time"
)

func TestWorkerPRBindingKeepsOriginalURL(t *testing.T) {
	for _, prState := range []string{"open", "closed_unmerged"} {
		state := State{WorkflowRunID: "run", TaskID: "7", WorkflowVersion: Version, Status: "waiting_external", ExecutionID: "execution", ExecutionWorkflowRunID: "run", PRURL: "original", PRState: prState}
		if next, err := BindPR(state, "another", "open", false, time.Now()); err == nil || next != state {
			t.Fatal("a replacement PR changed the original binding")
		}
		if next, err := BindPR(state, "original", "open", false, time.Now()); err != nil || next.PRURL != state.PRURL {
			t.Fatalf("same PR could not be updated: %v", err)
		}
	}
}

