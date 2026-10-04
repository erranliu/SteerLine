package api

type HostActionTarget struct {
	RunID       string `json:"run_id"`
	AttemptID   string `json:"attempt_id"`
	Version     uint64 `json:"version"`
	ExecutionID string `json:"execution_id"`
	TurnID      string `json:"turn_id"`
	BlockerID   string `json:"blocker_id"`
}
