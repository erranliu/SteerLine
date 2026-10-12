package model

import (
	"encoding/json"
)

const DevelopmentFlow = "development/v3"

const (
	StepImplement     = "implement.execute"
	StepReport        = "implement.report"
	StepPublish       = "publish"
	StepPublishReport = "publish.report"
	StepAwaitMerge    = "await_merge"
	StepDelivered     = "delivered"
)

// StageInput is the frozen business binding of a native input. Native transport
// purpose remains business/readiness; it is not the business step identifier.
type StageInput struct {
	FeedbackDecisionID string          `json:"feedback_decision_id,omitempty"`
	CommandID          string          `json:"command_id"`
	AttemptID          string          `json:"attempt_id"`
	StepID             string          `json:"step_id"`
	Version            uint64          `json:"version"`
	Prompt             string          `json:"prompt"`
	TaskFacts          json.RawMessage `json:"task_facts,omitempty"`
	SourceCommandID    string          `json:"source_command_id,omitempty"`
	SourceTurnID       string          `json:"source_turn_id,omitempty"`
	ReportToolPolicy   string          `json:"report_tool_policy,omitempty"`
}

type StageResult struct {
	CommandID            string `json:"command_id"`
	AttemptID            string `json:"attempt_id"`
	StepID               string `json:"step_id"`
	InputVersion         uint64 `json:"input_version"`
	TurnID               string `json:"turn_id"`
	SessionID            string `json:"session_id"`
	Succeeded            bool   `json:"succeeded"`
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

func ModelStep(step string) bool {
	return step == StepImplement || ReportStep(step) || step == StepPublish
}

func ReportStep(step string) bool { return step == StepReport || step == StepPublishReport }

func PublicationStep(step string) bool { return step == StepPublish || step == StepPublishReport }

func CloneStageResults(results []StageResult) []StageResult {
	return append([]StageResult(nil), results...)
}

func ValidFlowStatus(status string) bool {
	switch status {
	case "queued", "running", "waiting_input", "blocked", "reconciling", "waiting_external", "completed", "abandoned":
		return true
	}
	return false
}
