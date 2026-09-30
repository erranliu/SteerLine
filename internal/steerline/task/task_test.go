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
