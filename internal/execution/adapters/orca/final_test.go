package orcaworker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestWorkerStageNativeFinalUsesOriginalSettledMaterial(t *testing.T) {
	b := outputTestBinding(t)
	root := t.TempDir()
	path := filepath.Join(root, "runs", b.WorkflowID, "turns", b.ExecutionID, "orca-output."+b.TurnID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(b)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	l := &ExecutionLifecycle{setup: executionbackend.LifecycleSetup{RunDirectory: root}, orca: ProfileConfig{Executable: b.Command.Executable, Args: b.Command.Args}}
	snapshot := executionbackend.LifecycleSnapshot{Ref: executionbackend.ExecutionRef{WorkspacePath: b.Workspace, External: b.Process}, Session: executionbackend.SessionContext{ProjectID: b.ProjectID, WorkflowRunID: b.WorkflowID, ExecutionID: b.ExecutionID, AttemptID: b.AttemptID}}
	turn := NativeTurnState{TurnID: b.TurnID, TaskID: b.Process.Task, DispatchID: b.Process.Dispatch, Outcome: "succeeded", LastMessage: "done summary is not the response"}
	if err := l.readNativeFinal(context.Background(), snapshot, &turn, &NativeExecutionState{}); err == nil || turn.FinalVerified {
		t.Fatal("worker_done became a final without settled material")
	}
	body := `{"outcome":"done","outputs":{"candidate_sha":"abc"},"summary":"original result"}`
	material := OutputMaterial{WorkflowRunID: b.WorkflowID, TurnID: b.TurnID, TaskID: b.Process.Task, DispatchID: b.Process.Dispatch, Body: body, Outcome: "completed"}
	if err := freezeSettledOutput(path, b, material); err != nil {
		t.Fatal(err)
	}
	if err := l.readNativeFinal(context.Background(), snapshot, &turn, &NativeExecutionState{}); err != nil || !turn.FinalVerified || turn.FinalText != body || turn.FinalSource != outputSource || turn.FinalNativeTurnID != b.Process.Dispatch || turn.FinalItemID == "" {
		t.Fatalf("original response changed: %+v %v", turn, err)
	}
	for _, field := range []string{"execution", "attempt", "terminal", "task", "dispatch", "workspace", "command"} {
		t.Run(field, func(t *testing.T) {
			changed, native := snapshot.Clone(), turn
			switch field {
			case "execution":
				changed.Session.ExecutionID = stableIDValue(9)
			case "attempt":
				changed.Session.AttemptID = stableIDValue(9)
			case "terminal":
				changed.Ref.External.Terminal = "other"
			case "task":
				native.TaskID = "other"
			case "dispatch":
				native.DispatchID = "other"
			case "workspace":
				changed.Ref.WorkspacePath = t.TempDir()
			case "command":
				native.TurnID = stableIDValue(9)
			}
			if err := l.readNativeFinal(context.Background(), changed, &native, &NativeExecutionState{}); err == nil {
				t.Fatal("changed original source was accepted")
			}
		})
	}
}
