package main

// L1 names stable rule roots explicitly. New owner tests belong to L4 go.rules
// until their value as a mandatory early guard is deliberately reviewed.
var taskRuleCases = []quickOwnerCase{
	{"quick-owner-task-model", "github.com/erranliu/SteerLine/internal/task/model", []string{"TestContractDigestAndReferenceBoundary", "TestBatchValidationAndReadinessShareTaskRules"}},
	{"quick-owner-task", "github.com/erranliu/SteerLine/internal/task", []string{"TestTM03TaskBatch", "TestTM03Result", "TestTM03Dependencies"}},
	{"quick-owner-runner-contract", "github.com/erranliu/SteerLine/internal/runnercontract", []string{"TestTakeoverWireIdentity"}},
}
