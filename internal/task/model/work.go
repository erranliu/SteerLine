package model

import "encoding/json"

// FixedWork carries only the task's fixed contract and sampled graph identity.
// Consumers can define work ports without importing the task application.
type FixedWork struct {
	MaterialDigest  string
	FrozenTestPlan  json.RawMessage
	ConfirmedDigest string
	Record          TaskRecord
	Contract        TaskContract
	Authority       GraphAuthority
}
