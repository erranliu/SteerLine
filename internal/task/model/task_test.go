package model

import (
	"encoding/json"
	"strings"
	"testing"
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
	graph := Snapshot{Authority: GraphAuthority{ProjectIdentity: "project"}, ByID: map[string]TaskRecord{}, RepositoryTaskIDs: map[string]bool{}, ContractIDs: map[string]bool{}, Deliveries: map[string]DeliveryFact{}}
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
	graph.Deliveries["1"] = DeliveryFact{TaskID: "1"}
	if ready := ReadyTasks(graph); len(ready) != 1 || ready[0].TaskID != "2" {
		t.Fatalf("delivered predecessor did not release task: %+v", ready)
	}
}
