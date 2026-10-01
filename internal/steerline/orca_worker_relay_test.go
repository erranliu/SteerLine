package steerline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeOrcaDoneRequestBindsCurrentTurn(t *testing.T) {
	run := &Run{RunID: "60bc979788fbee3d491f49d2eeca10c1"}
	turn := &implementationTurnView{TurnID: "65986a5980b8c67c5588cb2fd04c0fa4", TaskID: "task-current", DispatchID: "ctx-current"}
	worktree := t.TempDir()
	path, err := nativeOrcaRelayPath(worktree, run.RunID, turn.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	request := nativeOrcaDoneRequest{WorkflowRunID: run.RunID, TurnID: turn.TurnID, TaskID: turn.TaskID, DispatchID: turn.DispatchID, DispatchCapability: "dcap_test-only", Outcome: "failed", Subject: "Could not finish", Body: "The worker reported an error."}
	write := func() {
		t.Helper()
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if got, err := readNativeOrcaDoneRequest(path, worktree, run, turn); err != nil || got == nil || got.Outcome != "failed" {
		t.Fatalf("current relay request rejected: %+v, %v", got, err)
	}
	request.DispatchCapability = ""
	write()
	if _, err := readNativeOrcaDoneRequest(path, worktree, run, turn); err == nil {
		t.Fatal("a relay request without the original Dispatch capability was accepted")
	}
	request.DispatchCapability = "dcap_test-only"
	request.DispatchID = "ctx-other"
	write()
	if _, err := readNativeOrcaDoneRequest(path, worktree, run, turn); err == nil {
		t.Fatal("another Dispatch's relay request was accepted")
	}
}
