package orcaworker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDoneRequestBindsCurrentTurn(t *testing.T) {
	runID := "60bc979788fbee3d491f49d2eeca10c1"
	turn := &NativeTurnState{TurnID: "65986a5980b8c67c5588cb2fd04c0fa4", TaskID: "task-current", DispatchID: "ctx-current"}
	worktree := t.TempDir()
	path, err := RelayPath(worktree, runID, turn.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	request := DoneRequest{WorkflowRunID: runID, TurnID: turn.TurnID, TaskID: turn.TaskID, DispatchID: turn.DispatchID, DispatchCapability: "dcap_test-only", Outcome: "failed", Subject: "Could not finish", Body: "The worker reported an error."}
	withBOM := false
	write := func() {
		t.Helper()
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if withBOM {
			data = append([]byte{0xef, 0xbb, 0xbf}, data...)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if got, err := ReadDoneRequest(path, worktree, runID, turn.TurnID, turn.TaskID, turn.DispatchID); err != nil || got == nil || got.Outcome != "failed" {
		t.Fatalf("current relay request rejected: %+v, %v", got, err)
	}
	withBOM = true
	write()
	if got, err := ReadDoneRequest(path, worktree, runID, turn.TurnID, turn.TaskID, turn.DispatchID); err != nil || got == nil || *got != request {
		t.Fatalf("PowerShell UTF-8 BOM changed or blocked the bound relay: %+v, %v", got, err)
	}
	request.DispatchCapability = ""
	write()
	if _, err := ReadDoneRequest(path, worktree, runID, turn.TurnID, turn.TaskID, turn.DispatchID); err == nil {
		t.Fatal("a relay request without the original Dispatch capability was accepted")
	}
	request.DispatchCapability = "dcap_test-only"
	request.DispatchID = "ctx-other"
	write()
	if _, err := ReadDoneRequest(path, worktree, runID, turn.TurnID, turn.TaskID, turn.DispatchID); err == nil {
		t.Fatal("another Dispatch's relay request was accepted")
	}
}
