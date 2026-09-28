package steerline

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestRealNativeStepPromptSingleIdentity reconstructs a saved isolated Step
// Input. It is opt-in and read-only; it never dispatches or replies to a worker.
func TestRealNativeStepPromptSingleIdentity(t *testing.T) {
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
	step := os.Getenv("STEERLINE_REAL_NATIVE_PROMPT_STEP")
	if step == "" {
		step = "publish_pr"
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
		if state.Attempts[i].ExecutionID == run.ExecutionID && state.Attempts[i].StepID == step {
			attempt = &state.Attempts[i]
		}
	}
	if workflow == nil || attempt == nil {
		t.Fatalf("original %s workflow or Attempt is missing", step)
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
	if len([]byte(spec)) > 16<<10 {
		t.Fatalf("saved native %s prompt exceeds 16 KiB: %d bytes", step, len([]byte(spec)))
	}
	matches := regexp.MustCompile(`FromBase64String\('([^']+)'\)`).FindAllStringSubmatch(spec, -1)
	if len(matches) != 1 {
		t.Fatalf("saved native %s prompt contains %d identity literals; want one", step, len(matches))
	}
	identityBytes, err := base64.StdEncoding.DecodeString(matches[0][1])
	if err != nil {
		t.Fatal(err)
	}
	var identity struct {
		ProjectIdentity string `json:"project_identity"`
		TaskID          string `json:"task_id"`
		WorkflowRunID   string `json:"workflow_run_id"`
		ExecutionID     string `json:"execution_id"`
		AttemptID       string `json:"attempt_id"`
		StepID          string `json:"step_id"`
	}
	if err = json.Unmarshal(identityBytes, &identity); err != nil {
		t.Fatal(err)
	}
	if identity.ProjectIdentity != input.ProjectIdentity || identity.TaskID != input.TaskID || identity.WorkflowRunID != input.WorkflowRunID || identity.ExecutionID != input.ExecutionID || identity.AttemptID != input.AttemptID || identity.StepID != step {
		t.Fatalf("native %s prompt identity differs from the saved Step Input: %+v", step, identity)
	}
	for _, duplicate := range []string{"$identitySHA256", "$identityExpected", "$projectIdentitySHA256"} {
		if strings.Contains(spec, duplicate) {
			t.Fatalf("native %s prompt still asks the worker to recopy %s", step, duplicate)
		}
	}
	t.Logf("saved native %s prompt contains one bound identity and fits 16 KiB: %d bytes", step, len([]byte(spec)))
}
