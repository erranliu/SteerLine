package model

type StageResult struct {
	CommandID            string `json:"command_id"`
	AttemptID            string `json:"attempt_id"`
	StepID               string `json:"step_id"`
	InputVersion         uint64 `json:"input_version"`
	TurnID               string `json:"turn_id"`
	SessionID            string `json:"session_id"`
	Succeeded            bool   `json:"succeeded"`
	NativeOutcome        string `json:"native_outcome,omitempty"`
	FinalVerified        bool   `json:"final_verified"`
	Final                string `json:"final,omitempty"`
	FinalNativeSessionID string `json:"final_native_session_id,omitempty"`
	FinalNativeTurnID    string `json:"final_native_turn_id,omitempty"`
	FinalDiagnostic      string `json:"final_diagnostic,omitempty"`
	FinalSource          string `json:"final_source,omitempty"`
	FinalItemID          string `json:"final_item_id,omitempty"`
	Failure              string `json:"failure,omitempty"`
	Outcome              string `json:"outcome,omitempty"`
}

func CloneStageResults(results []StageResult) []StageResult {
	return append([]StageResult(nil), results...)
}
