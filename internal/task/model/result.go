package model

import (
	"time"
)

// Result is a task conclusion. Provider proof stays with the execution result
// linked by ResultID; callers cannot close a task through a success boolean.
type Result struct {
	ProjectIdentity       string `json:"project_id"`
	TaskID                string `json:"task_id"`
	ContractID            string `json:"contract_id"`
	ContractDigest        string `json:"contract_digest"`
	Claimant              string `json:"claimant"`
	ExpectedTaskVersion   uint64 `json:"expected_task_version"`
	ExpectedGraphRevision uint64 `json:"expected_graph_revision"`
	ExpectedSetVersion    string `json:"expected_set_version"`
	ResultID              string `json:"result_id"`
	RequestID             string `json:"request_id"`
	Conclusion            string `json:"conclusion"`
}

type ResultDeclaration struct {
	Input  Result
	Before TaskRecord
	After  TaskRecord
}

type ResultReceipt struct {
	Input         Result `json:"input"`
	TaskVersion   uint64 `json:"task_version"`
	GraphRevision uint64 `json:"graph_revision"`
	Status        string `json:"status"`
}

func DeclareResult(record TaskRecord, contract TaskContract, input Result, at time.Time) (ResultDeclaration, error) {
	if input.ProjectIdentity != record.ProjectIdentity || input.TaskID != record.TaskID || input.Claimant == "" || input.Claimant != record.Claimant || input.ContractID != record.ContractID || input.ContractID != contract.ContractID || input.ContractDigest == "" || input.ContractDigest != contract.Digest {
		return ResultDeclaration{}, newError("contract_mismatch", "result must preserve the claimed task and fixed contract")
	}
	if record.Version != input.ExpectedTaskVersion || record.Status != "in_progress" {
		return ResultDeclaration{}, newError("stale_version", "result requires the original in-progress task version")
	}
	if input.ResultID == "" || input.RequestID == "" || input.ExpectedSetVersion == "" || input.Conclusion != "delivered" || at.IsZero() {
		return ResultDeclaration{}, newError("task_not_ready", "only a linked verified delivery may declare task closure")
	}
	next := record
	next.Status, next.Version, next.UpdatedAt = "closed", record.Version+1, at
	return ResultDeclaration{Input: input, Before: record, After: next}, nil
}
