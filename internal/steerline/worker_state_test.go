package steerline

import (
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"testing"
)

func TestWorkerFlowDeliveryGateUsesSessionAndPRWithoutStepResults(t *testing.T) {
	task := taskmodel.TaskRecord{TaskID: "8", ContractID: "contract-8"}
	contract := taskmodel.TaskContract{ContractID: "contract-8", Digest: "fixed-digest"}
	wf := executionmodel.WorkflowRunRecord{WorkflowRunID: "wf-8", TaskID: task.TaskID, WorkflowVersion: executionmodel.Version, Phase: "waiting_pr_merge", SelectedResult: "exec-8", PRURL: "https://git.example/o/r/pulls/8"}
	exec := executionmodel.ExecutionRecord{ExecutionID: "exec-8", CurrentAttemptID: "attempt-8", WorkflowRunID: wf.WorkflowRunID, Protocol: string(neutralProtocol), Workspace: executionmodel.WorkspaceEvidence{Branch: "fixture/task-8", Dirty: true}, Status: "idle"}
	setNeutralBinding(t, &exec, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	prURL, err := validateNativeDeliveryGates(task, contract, wf, exec)
	if err != nil || prURL != wf.PRURL {
		t.Fatalf("workflow requires an obsolete step result: prURL=%q error=%v", prURL, err)
	}
	removeImplementationEnvelope(&exec, neutralBindingKey)
	exec.SessionID, exec.SessionGeneration = "", ""
	if _, err := validateNativeDeliveryGates(task, contract, wf, exec); err == nil {
		t.Fatal("accepted delivery without a bound worker session")
	}
}
