package orcaworker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func outputTestBinding(t *testing.T) outputBinding {
	t.Helper()
	return outputBinding{Version: 1, ProjectID: "project", WorkflowID: stableIDValue(1), ExecutionID: stableIDValue(2), AttemptID: stableIDValue(3), TurnID: stableIDValue(4), Workspace: t.TempDir(), Command: executionbackend.Command{Executable: os.Args[0], Args: []string{"-test.run=^TestOrcaOutputCLIFixture$", "--", "__orca_output_fixture"}}, Process: executionbackend.ProcessIdentity{Kind: "orca", RuntimeID: "runtime", Terminal: "terminal", Incarnation: "inc", WorktreeID: "workspace", OrchestrationRun: "run", Task: "task", Dispatch: "dispatch"}}
}

func writeTestMaterial(t *testing.T, binding outputBinding, material OutputMaterial) string {
	t.Helper()
	relative, err := outputRelativePath(binding.WorkflowID, binding.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(binding.Workspace, relative)
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func settledTestResult(binding outputBinding, status string) Result {
	raw, _ := json.Marshal(map[string]any{"dispatch": map[string]string{"id": binding.Process.Dispatch, "runId": binding.Process.OrchestrationRun, "taskId": binding.Process.Task, "status": status}, "worker": map[string]string{"dispatchId": binding.Process.Dispatch, "agentTerminalHandle": binding.Process.Terminal}, "terminal": map[string]string{"handle": binding.Process.Terminal, "incarnationId": binding.Process.Incarnation, "worktreeId": binding.Process.WorktreeID}})
	return Result{RuntimeID: binding.Process.RuntimeID, Payload: raw}
}

func TestOutputMaterialRequiresOriginalSettledDispatch(t *testing.T) {
	binding := outputTestBinding(t)
	material := OutputMaterial{WorkflowRunID: binding.WorkflowID, TurnID: binding.TurnID, TaskID: "task", DispatchID: "dispatch", Body: "完整结果🙂\n{\"status\":\"done\"}"}
	path := writeTestMaterial(t, binding, material)
	status := "dispatched"
	call := func(context.Context, ...string) (Result, error) { return settledTestResult(binding, status), nil }
	if value, err := readOutputMaterial(context.Background(), call, binding); err != nil || value != nil {
		t.Fatalf("file alone became completion: %+v %v", value, err)
	}
	status = "completed"
	value, err := readOutputMaterial(context.Background(), call, binding)
	if err != nil || value == nil || value.Body != material.Body {
		t.Fatalf("complete material changed: %+v %v", value, err)
	}
	for _, field := range []string{"workflow", "turn", "task", "dispatch"} {
		t.Run(field, func(t *testing.T) {
			changed := material
			switch field {
			case "workflow":
				changed.WorkflowRunID = stableIDValue(8)
			case "turn":
				changed.TurnID = stableIDValue(8)
			case "task":
				changed.TaskID = "another-task"
			case "dispatch":
				changed.DispatchID = "another-dispatch"
			}
			writeTestMaterial(t, binding, changed)
			if _, err := readOutputMaterial(context.Background(), call, binding); err == nil {
				t.Fatal("mismatched original identity was accepted")
			}
		})
	}
	if err = os.WriteFile(path, []byte(`{"body":"truncated`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readOutputMaterial(context.Background(), call, binding); err == nil {
		t.Fatal("truncated output material was accepted")
	}
}

func TestOrcaConversationUsesExplicitMaterialAndReplay(t *testing.T) {
	binding := outputTestBinding(t)
	writeTestMaterial(t, binding, OutputMaterial{WorkflowRunID: binding.WorkflowID, TurnID: binding.TurnID, TaskID: "task", DispatchID: "dispatch", Body: "完整结果\n{\"status\":\"done\"}"})
	runDir := t.TempDir()
	dir := filepath.Join(runDir, "turns", binding.ExecutionID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(binding)
	if err := os.WriteFile(filepath.Join(dir, "orca-output."+binding.TurnID+".json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	base := executionmodel.ConversationEvent{ProjectID: binding.ProjectID, TaskID: "business-task", WorkflowID: binding.WorkflowID, ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID, Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV2, Agent: "configured-agent", TargetID: "target"}
	adapter := NewConversationAdapter()
	discovered, err := adapter.DiscoverHistory(context.Background(), conversationtypes.ConversationDiscovery{ProjectID: binding.ProjectID, Runs: []conversationtypes.ConversationRunSource{{Base: base, RunDirectory: runDir, Workspace: binding.Workspace}}})
	if err != nil || len(discovered.Sources) != 1 {
		t.Fatalf("discovery %v %+v", err, discovered)
	}
	request := conversationtypes.ConversationHistoryReadRequest{Source: discovered.Sources[0]}
	batch, err := adapter.ReadHistory(context.Background(), request, nil)
	if err != nil || !batch.Settled || len(batch.Writes) != 2 {
		t.Fatalf("material read %v %+v", err, batch)
	}
	for _, write := range batch.Writes {
		if err = write.Event.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if batch.Writes[0].Event.Evidence["phase_origin"] != "explicit_worker_output_material" || batch.Writes[0].BodyMode != "complete" || string(batch.Writes[0].Body) != "完整结果\n{\"status\":\"done\"}" {
		t.Fatal("source inferred provider phase or truncated explicit body")
	}
	request.Cursor, request.HasCheckpoint = batch.Cursor, true
	replay, err := adapter.ReadHistory(context.Background(), request, nil)
	if err != nil || len(replay.Writes) != 0 || !replay.Settled {
		t.Fatalf("replay duplicated original final: %v %+v", err, replay)
	}
	writeTestMaterial(t, binding, OutputMaterial{WorkflowRunID: binding.WorkflowID, TurnID: binding.TurnID, TaskID: "task", DispatchID: "dispatch", Body: "changed after settlement"})
	frozen, err := adapter.ReadHistory(context.Background(), request, nil)
	if err != nil || len(frozen.Writes) != 0 || !frozen.Settled {
		t.Fatal("workspace mutation changed the already-frozen original output")
	}
	// Released resources no longer have a live terminal; frozen evidence is sufficient.
	binding.Command.Executable = "missing-native-program"
	evidencePath := request.Source.BindingPath + ".settled"
	raw, err = os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "完整结果", "tampered-result", 1))
	if err = os.WriteFile(evidencePath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.ReadHistory(context.Background(), request, nil); err == nil {
		t.Fatal("tampered frozen evidence was accepted")
	}

}

func TestOrcaOutputCLIFixture(t *testing.T) {
	if !strings.Contains(strings.Join(os.Args, " "), "__orca_output_fixture") {
		return
	}
	binding := outputBinding{Process: executionbackend.ProcessIdentity{RuntimeID: "runtime", Terminal: "terminal", Incarnation: "inc", WorktreeID: "workspace", OrchestrationRun: "run", Task: "task", Dispatch: "dispatch"}}
	result := settledTestResult(binding, "completed")
	raw, _ := json.Marshal(map[string]any{"ok": true, "result": result.Payload, "_meta": map[string]string{"runtimeId": "runtime"}})
	fmt.Print(string(raw))
	os.Exit(0)
}
