package api

// NativeSessionTarget identifies the exact candidate visible when opening a
// native viewer. The application resolves executable and endpoint itself.
type NativeSessionTarget struct {
	RunID             string `json:"run_id"`
	ExecutionID       string `json:"execution_id"`
	AttemptID         string `json:"attempt_id"`
	SessionID         string `json:"session_id"`
	SessionGeneration string `json:"session_generation"`
}
