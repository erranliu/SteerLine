package task

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestContractDigestAndReferenceBoundary(t *testing.T) {
	ref := FileReference{Path: "docs/spec/task.spec.md", Commit: strings.Repeat("a", 40), SHA256: strings.Repeat("b", 64)}
	contract := TaskContract{
		ContractID: "task-7", Repository: "https://example.test/repo.git", Remote: "origin", Branch: "main",
		SpecRef: ref, PlanRef: FileReference{Path: "docs/spec/task.plan.md", Commit: ref.Commit, SHA256: ref.SHA256},
		AcceptanceRefs: []FileReference{ref},
	}
	var err error
	contract.Digest, err = ContractDigest(contract)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateContract(contract); err != nil {
		t.Fatal(err)
	}
	contract.SpecRef.Path = "../outside.spec.md"
	contract.Digest, err = ContractDigest(contract)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateContract(contract); err == nil {
		t.Fatal("contract accepted a path outside the repository")
	}
	if _, err = ValidateContractJSON([]byte(`{"contract_id":"first","contract_id":"second"}`)); err == nil {
		t.Fatal("contract accepted a duplicate JSON key")
	}
}

func TestTaskStatusAndDeliveryEvidence(t *testing.T) {
	for _, id := range []string{"01", "-1", "9223372036854775808"} {
		var taskErr *Error
		if err := ValidateID(id); !errors.As(err, &taskErr) || taskErr.Code != "contract_mismatch" {
			t.Fatalf("task ID %q was not rejected with contract_mismatch: %v", id, err)
		}
	}
	at := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	record := TaskRecord{TaskID: "7", Status: "open", Version: 1}
	if err := SetStatus(&record, "in_progress", false, at); err != nil {
		t.Fatal(err)
	}
	if record.Version != 2 || !record.UpdatedAt.Equal(at) {
		t.Fatalf("task transition lost its version or timestamp: %+v", record)
	}
	if err := SetStatus(&record, "discarded", true, at); err == nil || record.Status != "in_progress" {
		t.Fatalf("merged task was discarded: %+v, %v", record, err)
	}
	merged := TaskDeliveryEvidence{
		PRInstance: "github", URL: "https://example.test/repo/pull/7", Repository: "example/repo", PRNumber: 7,
		BaseBranch: "main", State: "merged", Merged: true, MergedAt: at, MergeCommit: strings.Repeat("c", 40),
	}
	body, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	delivery := TaskDeliveryRecord{
		ProjectIdentity: "project", TaskID: "7", SelectedWorkflowRunID: "run-7", PRInstance: merged.PRInstance,
		PRRepository: merged.Repository, PRNumber: merged.PRNumber, PRURL: merged.URL,
		MergeCommit: merged.MergeCommit, MergedAt: at, RecordedAt: at, Evidence: body,
	}
	if err := ValidateDelivery(delivery, "main"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDelivery(delivery, "other"); err == nil {
		t.Fatal("delivery accepted evidence for a different target branch")
	}
}

func TestBatchValidationAndReadinessShareTaskRules(t *testing.T) {
	ref := FileReference{Path: "docs/spec/task.spec.md", Commit: strings.Repeat("a", 40), SHA256: strings.Repeat("b", 64)}
	contract := TaskContract{
		ContractID: "contract-2", Repository: "https://example.test/repo.git", Remote: "origin", Branch: "main",
		SpecRef: ref, PlanRef: FileReference{Path: "docs/spec/task.plan.md", Commit: ref.Commit, SHA256: ref.SHA256},
		AcceptanceRefs: []FileReference{ref},
	}
	contract.Digest, _ = ContractDigest(contract)
	batch := TaskBatchInput{SchemaVersion: BatchSchemaVersion, ProjectID: "project", Tasks: []TaskBatchTask{{LocalKey: "second", TaskID: 2, Title: "Second", Type: "task", Contract: contract}}, Dependencies: []TaskBatchEdge{}}
	encoded, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	decoded, canonical, err := DecodeBatch(encoded)
	if err != nil || len(canonical) == 0 {
		t.Fatalf("valid batch did not decode: %v", err)
	}
	graph := Snapshot{Authority: ProjectTaskAuthority{ProjectIdentity: "project"}, ByID: map[string]TaskRecord{}, RepositoryTaskIDs: map[string]bool{}, ContractIDs: map[string]bool{}, Deliveries: map[string]TaskDeliveryRecord{}}
	issues, cycle := ValidateBatchAgainstGraph(decoded, graph, []string{"task"}, nil)
	if len(issues) != 0 || len(cycle) != 0 {
		t.Fatalf("valid batch was rejected: %+v, %v", issues, cycle)
	}
	duplicate := decoded
	duplicate.Tasks = append(append([]TaskBatchTask(nil), decoded.Tasks...), TaskBatchTask{LocalKey: "other", TaskID: 2, Title: "Other", Type: "task", Contract: contract})
	issues, _ = ValidateBatchAgainstGraph(duplicate, graph, []string{"task"}, nil)
	if len(issues) == 0 {
		t.Fatal("duplicate task identity was accepted")
	}
	first := TaskRecord{TaskID: "1", Status: "closed"}
	second := TaskRecord{TaskID: "2", Status: "open"}
	graph.Tasks = []TaskRecord{first, second}
	graph.ByID = map[string]TaskRecord{"1": first, "2": second}
	graph.Edges = []TaskDependencyRecord{{PredecessorID: "1", SuccessorID: "2"}}
	if ready := ReadyTasks(graph); len(ready) != 0 {
		t.Fatalf("undelivered predecessor released task: %+v", ready)
	}
	graph.Deliveries["1"] = TaskDeliveryRecord{TaskID: "1"}
	if ready := ReadyTasks(graph); len(ready) != 1 || ready[0].TaskID != "2" {
		t.Fatalf("delivered predecessor did not release task: %+v", ready)
	}
}

func TestDeliveryGateRequiresSelectedWorker(t *testing.T) {
	record := TaskRecord{TaskID: "2", ContractID: "contract-2"}
	contract := TaskContract{ContractID: "contract-2", Digest: "fixed"}
	gate := DeliveryGate{WorkflowVersion: "worker-flow-v1", WorkflowTaskID: "2", WorkflowRunID: "run", SelectedExecutionID: "execution", ExecutionID: "execution", ExecutionWorkflowID: "run", PRURL: "https://example.test/pull/2", SessionID: "session", WorkspaceBranch: "branch", Phase: "waiting_pr_merge"}
	if url, err := DeliveryPR(record, contract, gate, "worker-flow-v1"); err != nil || url != gate.PRURL {
		t.Fatalf("selected worker was rejected: %q, %v", url, err)
	}
	gate.ExecutionWorkflowID = "other"
	if _, err := DeliveryPR(record, contract, gate, "worker-flow-v1"); err == nil {
		t.Fatal("execution from another workflow was accepted")
	}
}

func TestBatchPlanAndTaskClaimKeepFixedValues(t *testing.T) {
	at := time.Date(2026, 9, 30, 1, 2, 3, 0, time.UTC)
	goals := []string{"goal-a"}
	input := TaskBatchInput{ProjectID: "project", Tasks: []TaskBatchTask{{LocalKey: "first", TaskID: 7, Title: "First", Type: "task", GoalIDs: goals, Contract: TaskContract{ContractID: "contract-7"}}}}
	plan := PlanBatch(input, "batch", "command", "digest", at)
	if plan.Batch.TaskIDs["first"] != "7" || len(plan.Tasks) != 1 || plan.Tasks[0].Status != "open" || plan.Tasks[0].Version != 1 {
		t.Fatalf("batch plan lost a task identity: %+v", plan)
	}
	goals[0] = "changed"
	if plan.Tasks[0].GoalIDs[0] != "goal-a" {
		t.Fatal("batch plan shares mutable goal IDs with input")
	}
	claimed, err := Claim(plan.Tasks[0], "worker", at.Add(time.Minute))
	if err != nil || claimed.Status != "in_progress" || claimed.Claimant != "worker" || claimed.Version != 2 {
		t.Fatalf("claim did not advance the planned task: %+v, %v", claimed, err)
	}
	if plan.Tasks[0].Status != "open" {
		t.Fatal("claim mutated the sampled task record")
	}
	proof := TaskDeliveryEvidence{PRInstance: "github", URL: "https://example.test/repo/pull/7", Repository: "example/repo", PRNumber: 7, BaseBranch: "main", State: "merged", Merged: true, MergedAt: at, MergeCommit: strings.Repeat("c", 40)}
	proofJSON, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	delivery := TaskDeliveryRecord{ProjectIdentity: "project", TaskID: "7", SelectedWorkflowRunID: "run-7", PRInstance: proof.PRInstance, PRRepository: proof.Repository, PRNumber: proof.PRNumber, PRURL: proof.URL, MergeCommit: proof.MergeCommit, MergedAt: at, RecordedAt: at, Evidence: proofJSON}
	closed, err := CompleteDelivery(claimed, delivery, "main", at.Add(2*time.Minute))
	if err != nil || closed.Status != "closed" || closed.Version != 3 {
		t.Fatalf("delivery did not advance the claimed task: %+v, %v", closed, err)
	}
	if _, err := CompleteDelivery(plan.Tasks[0], delivery, "main", at); err == nil {
		t.Fatal("open task was closed without a claim")
	}
	delivery.TaskID = "other"
	if _, err := CompleteDelivery(claimed, delivery, "main", at); err == nil {
		t.Fatal("delivery for another task was accepted")
	}
	cleanup := []string{"workspace_cleanup:one"}
	if _, err := Discard(claimed, "stopped", "operator", "request", cleanup, true, at); err == nil {
		t.Fatal("merged task was discarded")
	}
	discarded, err := Discard(claimed, "stopped", "operator", "request", cleanup, false, at.Add(3*time.Minute))
	if err != nil || discarded.Status != "discarded" || discarded.Version != 3 || discarded.DiscardRequestID != "request" {
		t.Fatalf("discard did not freeze its task identity: %+v, %v", discarded, err)
	}
	cleanup[0] = "changed"
	if discarded.CleanupPending[0] != "workspace_cleanup:one" {
		t.Fatal("discard shares its cleanup list with the caller")
	}
	if _, err := Discard(discarded, "again", "operator", "second-request", nil, false, at.Add(4*time.Minute)); err == nil {
		t.Fatal("already discarded task accepted a second discard")
	}
}
