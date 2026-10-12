package model

import (
	"errors"
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

func TestTwoTurnsKeepOriginalSessionAndPR(t *testing.T) {
	at := time.Date(2026, 9, 30, 1, 2, 3, 0, time.UTC)
	state := State{WorkflowRunID: "run", TaskID: "7", WorkflowVersion: Version, Status: "queued", ExecutionID: "execution", ExecutionWorkflowRunID: "run"}
	var err error
	state, err = BeginTurn(state, "session", "turn-1", at)
	if err != nil {
		t.Fatal(err)
	}
	state, err = BindPR(state, "pr-7", "open", false, at)
	if err != nil || state.Status != "running" {
		t.Fatalf("early PR interrupted active turn: %+v %v", state, err)
	}
	state, err = NoteQuestion(state, "turn-1", at)
	if err != nil || state.Status != "waiting_input" {
		t.Fatalf("question lost precedence: %+v %v", state, err)
	}
	state, err = ContinueQuestion(state, "turn-1", at)
	if err != nil || state.Status != "running" {
		t.Fatalf("answer changed the original turn: %+v %v", state, err)
	}
	state, err = StopTurn(state, "turn-1", false, at)
	if err != nil || state.Status != "blocked" {
		t.Fatalf("native stop incorrectly decided business completion: %+v %v", state, err)
	}
	state.Status = "queued"
	state, err = BeginTurn(state, "session", "turn-2", at.Add(time.Minute))
	if err != nil || state.SessionID != "session" || state.TurnNumber != 2 {
		t.Fatalf("continuation replaced the session: %+v %v", state, err)
	}
	state, err = StopTurn(state, "turn-2", false, at)
	if err != nil {
		t.Fatal(err)
	}
	state, err = Complete(state, "7", "closed", Delivery{TaskID: "7", WorkflowRunID: "run", PRURL: "pr-7", MergeCommit: "merge", MergedAt: at}, at)
	if err != nil || state.Status != "completed" {
		t.Fatalf("committed delivery did not complete flow: %+v %v", state, err)
	}
}

func TestLateTurnCannotChangeCurrentState(t *testing.T) {
	at := time.Date(2026, 9, 30, 1, 2, 3, 0, time.UTC)
	state := State{WorkflowRunID: "run", WorkflowVersion: Version, Status: "queued", ExecutionID: "execution", ExecutionWorkflowRunID: "run"}
	state, _ = BeginTurn(state, "session", "turn-1", at)
	state, _ = StopTurn(state, "turn-1", false, at)
	state.Status = "queued"
	state, _ = BeginTurn(state, "session", "turn-2", at)
	next, err := NoteQuestion(state, "turn-1", at)
	if !errors.Is(err, ErrTurnStale) || next != state {
		t.Fatalf("late question changed the newer turn: %+v %v", next, err)
	}
	if _, err := BeginTurn(state, "another-session", "turn-3", at); err == nil {
		t.Fatal("replacement session was accepted")
	}
}
