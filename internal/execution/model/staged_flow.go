package model

import executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"

const DevelopmentFlow = "development/v1"

const (
	StepImplement  = "implement.execute"
	StepReport     = "implement.report"
	StepPublish    = "publish"
	StepAwaitMerge = "await_merge"
	StepDelivered  = "delivered"
)

// StageInput is the frozen business binding of a native input. Native transport
// purpose remains business/readiness; it is not the business step identifier.
type StageInput struct {
	CommandID        string         `json:"command_id"`
	AttemptID        string         `json:"attempt_id"`
	StepID           string         `json:"step_id"`
	Version          uint64         `json:"version"`
	Prompt           string         `json:"prompt"`
	SourceCommandID  string         `json:"source_command_id,omitempty"`
	SourceTurnID     string         `json:"source_turn_id,omitempty"`
	Snapshot         *StageSnapshot `json:"snapshot,omitempty"`
	CandidateSHA     string         `json:"candidate_sha,omitempty"`
	ReportToolPolicy string         `json:"report_tool_policy,omitempty"`
}

// StageSnapshot is evidence, not an independently writable state machine.
type StageSnapshot struct {
	Workspace executionbackend.WorkspaceInspection `json:"workspace"`
	PR        *PRInfo                              `json:"pr,omitempty"`
}

type StageResult struct {
	CommandID            string         `json:"command_id"`
	AttemptID            string         `json:"attempt_id"`
	StepID               string         `json:"step_id"`
	InputVersion         uint64         `json:"input_version"`
	TurnID               string         `json:"turn_id"`
	SessionID            string         `json:"session_id"`
	Succeeded            bool           `json:"succeeded"`
	Final                string         `json:"final,omitempty"`
	FinalNativeSessionID string         `json:"final_native_session_id,omitempty"`
	FinalNativeTurnID    string         `json:"final_native_turn_id,omitempty"`
	FinalSource          string         `json:"final_source,omitempty"`
	FinalItemID          string         `json:"final_item_id,omitempty"`
	Failure              string         `json:"failure,omitempty"`
	Snapshot             *StageSnapshot `json:"snapshot,omitempty"`
	Outcome              string         `json:"outcome,omitempty"`
}

func ModelStep(step string) bool {
	return step == StepImplement || step == StepReport || step == StepPublish
}

func CloneStageResults(results []StageResult) []StageResult {
	copyResults := append([]StageResult(nil), results...)
	for i := range copyResults {
		if results[i].Snapshot != nil {
			value := *results[i].Snapshot
			if value.PR != nil {
				pr := *value.PR
				value.PR = &pr
			}
			copyResults[i].Snapshot = &value
		}
	}
	return copyResults
}

func ValidFlowStatus(status string) bool {
	switch status {
	case "queued", "running", "waiting_input", "blocked", "reconciling", "waiting_external", "completed", "abandoned":
		return true
	}
	return false
}
