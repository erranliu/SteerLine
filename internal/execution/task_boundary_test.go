package execution

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestTaskStatusAndDeliveryEvidence(t *testing.T) {
	at := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	merged := executionmodel.TaskDeliveryEvidence{
		PRInstance: "github", URL: "https://example.test/repo/pull/7", Repository: "example/repo", PRNumber: 7,
		BaseBranch: "main", State: "merged", Merged: true, MergedAt: at, MergeCommit: strings.Repeat("c", 40),
	}
	body, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	delivery := executionmodel.TaskDeliveryRecord{
		ProjectIdentity: "project", TaskID: "7", SelectedWorkflowRunID: "run-7", PRInstance: merged.PRInstance,
		PRRepository: merged.Repository, PRNumber: merged.PRNumber, PRURL: merged.URL,
		MergeCommit: merged.MergeCommit, MergedAt: at, RecordedAt: at, Evidence: body,
	}
	if err := executionmodel.ValidateDelivery(delivery, "main"); err != nil {
		t.Fatal(err)
	}
	if err := executionmodel.ValidateDelivery(delivery, "other"); err == nil {
		t.Fatal("delivery accepted evidence for a different target branch")
	}
}

func TestDeliveryGateRequiresSelectedWorker(t *testing.T) {
	record := taskmodel.TaskRecord{TaskID: "2", ContractID: "contract-2"}
	contract := taskmodel.TaskContract{ContractID: "contract-2", Digest: "fixed"}
	gate := executionmodel.DeliveryGate{WorkflowVersion: "worker-flow-v1", WorkflowTaskID: "2", WorkflowRunID: "run", SelectedExecutionID: "execution", ExecutionID: "execution", ExecutionWorkflowID: "run", PRURL: "https://example.test/pull/2", SessionID: "session", WorkspaceBranch: "branch", Phase: "waiting_pr_merge"}
	if url, err := executionmodel.DeliveryPR(record.TaskID, record.ContractID, contract.ContractID, contract.Digest, gate, "worker-flow-v1"); err != nil || url != gate.PRURL {
		t.Fatalf("selected worker was rejected: %q, %v", url, err)
	}
	gate.ExecutionWorkflowID = "other"
	if _, err := executionmodel.DeliveryPR(record.TaskID, record.ContractID, contract.ContractID, contract.Digest, gate, "worker-flow-v1"); err == nil {
		t.Fatal("execution from another workflow was accepted")
	}
}
