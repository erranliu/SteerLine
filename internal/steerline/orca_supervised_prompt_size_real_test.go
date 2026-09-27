package steerline

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// TestRealNativePublishPromptStillFitsBound reconstructs the exact saved
// publish Step Input from a completed isolated native run. It is opt-in and
// read-only; it never dispatches or replies to a worker.
func TestRealNativePublishPromptStillFitsBound(t *testing.T) {
	path := os.Getenv("STEERLINE_REAL_NATIVE_PROMPT_CONFIG")
	if path == "" {
		t.Skip("set STEERLINE_REAL_NATIVE_PROMPT_CONFIG to an isolated native acceptance config")
	}
	ctx := context.Background()
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := LoadDatabaseRuntime(c)
	if err != nil || state == nil || len(state.Runs) != 1 {
		t.Fatalf("read original native Run: state=%v error=%v", state, err)
	}
	run := state.Runs[0]
	task := nativeTaskRecord(state.Tasks, run.DomainTaskID)
	if task == nil {
		t.Fatal("original native task is missing")
	}
	var workflow *WorkflowRunRecord
	var attempt *StepAttemptRecord
	for i := range state.WorkflowRuns {
		if state.WorkflowRuns[i].WorkflowRunID == run.RunID {
			workflow = &state.WorkflowRuns[i]
		}
	}
	for i := range state.Attempts {
		if state.Attempts[i].ExecutionID == run.ExecutionID && state.Attempts[i].StepID == "publish_pr" {
			attempt = &state.Attempts[i]
		}
	}
	if workflow == nil || attempt == nil {
		t.Fatal("original publish workflow or Attempt is missing")
	}
	var operation *OperationRecord
	for i := range state.Operations {
		if state.Operations[i].OperationID == attempt.OperationID {
			operation = &state.Operations[i]
		}
	}
	if operation == nil || operation.Kind != "dispatch_step" {
		t.Fatal("original publish start Operation is missing")
	}
	input, err := decodeSupervisedStartInput(operation.Request)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Input NativeTaskWorkerContext `json:"input"`
	}
	if err := json.Unmarshal(operation.Request, &request); err != nil {
		t.Fatal(err)
	}
	pair, err := probeSupervisedOrcaTarget(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := supervisedStepTaskSpec(c, *task, *run, *workflow, *attempt, input, request.Input, pair, deref(run.BaseCommit))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("saved native publish prompt remains within 16 KiB: %d bytes", len([]byte(spec)))
}
