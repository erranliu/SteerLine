package api

import "time"

// LastMessage is a best-effort activity hint, not execution or recovery truth.
type LastMessage struct {
	RunID      string    `json:"run_id"`
	AttemptID  string    `json:"attempt_id"`
	SessionID  string    `json:"session_id"`
	TurnID     string    `json:"turn_id"`
	Kind       string    `json:"kind"`
	Text       string    `json:"text"`
	ObservedAt time.Time `json:"observed_at"`
}
type LastMessageView struct {
	RunID     string       `json:"run_id"`
	AttemptID string       `json:"attempt_id"`
	Message   *LastMessage `json:"message"`
}
