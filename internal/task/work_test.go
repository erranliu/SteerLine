package task

import (
	"errors"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"testing"
	"time"
)

func TestTaskStatusAndDeliveryEvidence(t *testing.T) {
	for _, id := range []string{"01", "-1", "9223372036854775808"} {
		var taskErr *taskmodel.Error
		if err := taskmodel.ValidateID(id); !errors.As(err, &taskErr) || taskErr.Code != "contract_mismatch" {
			t.Fatalf("task ID %q was not rejected with contract_mismatch: %v", id, err)
		}
	}
	at := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	record := taskmodel.TaskRecord{TaskID: "7", Status: "open", Version: 1}
	if err := taskmodel.SetStatus(&record, "in_progress", at); err != nil {
		t.Fatal(err)
	}
	if record.Version != 2 || !record.UpdatedAt.Equal(at) {
		t.Fatalf("task transition lost its version or timestamp: %+v", record)
	}
}

func TestTM03Result(t *testing.T) {
	at := time.Date(2026, 9, 30, 1, 2, 3, 0, time.UTC)
	goals := []string{"goal-a"}
	input := taskmodel.TaskBatchInput{ProjectID: "project", Tasks: []taskmodel.TaskBatchTask{{LocalKey: "first", TaskID: 7, Title: "First", Type: "task", GoalIDs: goals, Contract: taskmodel.TaskContract{ContractID: "contract-7"}}}}
	plan := taskmodel.PlanBatch(input, "batch", "command", "digest", at)
	if plan.Batch.TaskIDs["first"] != "7" || len(plan.Tasks) != 1 || plan.Tasks[0].Status != "open" || plan.Tasks[0].Version != 1 {
		t.Fatalf("batch plan lost a task identity: %+v", plan)
	}
	goals[0] = "changed"
	if plan.Tasks[0].GoalIDs[0] != "goal-a" {
		t.Fatal("batch plan shares mutable goal IDs with input")
	}
	claimed, err := taskmodel.Claim(plan.Tasks[0], "worker", at.Add(time.Minute))
	if err != nil || claimed.Status != "in_progress" || claimed.Claimant != "worker" || claimed.Version != 2 {
		t.Fatalf("claim did not advance the planned task: %+v, %v", claimed, err)
	}
	if plan.Tasks[0].Status != "open" {
		t.Fatal("claim mutated the sampled task record")
	}
	resultInput := taskmodel.Result{ProjectIdentity: claimed.ProjectIdentity, TaskID: claimed.TaskID, ContractID: claimed.ContractID, ContractDigest: "digest", Claimant: claimed.Claimant, ExpectedTaskVersion: claimed.Version, ExpectedSetVersion: "set", ResultID: "result", RequestID: "request", Conclusion: "delivered"}
	contract := taskmodel.TaskContract{ContractID: claimed.ContractID, Digest: "digest"}
	declaration, err := (Service{}).Declare(claimed, contract, resultInput, at.Add(2*time.Minute))
	closed := declaration.After
	if err != nil || closed.Status != "closed" || closed.Version != 3 {
		t.Fatalf("delivery did not advance the claimed task: %+v, %v", closed, err)
	}
	if _, err := (Service{}).Declare(plan.Tasks[0], contract, resultInput, at); err == nil {
		t.Fatal("open task was closed without a claim")
	}
	resultInput.TaskID = "other"
	if _, err := (Service{}).Declare(claimed, contract, resultInput, at); err == nil {
		t.Fatal("delivery for another task was accepted")
	}
	cleanup := []string{"workspace_cleanup:one"}
	discarded, err := taskmodel.Discard(claimed, "stopped", "operator", "request", cleanup, at.Add(3*time.Minute))
	if err != nil || discarded.Status != "discarded" || discarded.Version != 3 || discarded.DiscardRequestID != "request" {
		t.Fatalf("discard did not freeze its task identity: %+v, %v", discarded, err)
	}
	cleanup[0] = "changed"
	if discarded.CleanupPending[0] != "workspace_cleanup:one" {
		t.Fatal("discard shares its cleanup list with the caller")
	}
	if _, err := taskmodel.Discard(discarded, "again", "operator", "second-request", nil, at.Add(4*time.Minute)); err == nil {
		t.Fatal("already discarded task accepted a second discard")
	}
}
