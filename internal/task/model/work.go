package model

import "encoding/json"

// FixedWork carries only the task's fixed contract and sampled graph identity.
// Consumers can define work ports without importing the task application.
type FixedWork struct {
	FrozenTestPlan   json.RawMessage
	ConfirmedDigest  string
	MaterialPlan     string
	MaterialInputs   string
	MaterialOutput   string
	MaterialRevision uint64
	Record           TaskRecord
	Contract         TaskContract
	Authority        GraphAuthority
}

// ExecutionSnapshot is fixed once with the first candidate reservation.
type ExecutionSnapshot struct {
	Contract TaskContract    `json:"contract"`
	Base     string          `json:"base"`
	TestPlan json.RawMessage `json:"test_plan"`
}
