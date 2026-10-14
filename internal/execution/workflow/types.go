package workflow

import "encoding/json"

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

// StageResult contains normalized business source facts. Native provenance
// is validated by reliable and retained only in the actor's persisted record.
type StageResult struct {
	CommandID, AttemptID, StepID             string
	InputVersion                             uint64
	TurnID                                   string // opaque source correlation, never decoded
	Succeeded, FinalVerified                 bool
	Final, Failure, FinalDiagnostic, Outcome string
}

func ModelStep(step string) bool {
	return step == StepImplement || ReportStep(step) || step == StepPublish
}

func ReportStep(step string) bool { return step == StepReport || step == StepPublishReport }

func PublicationStep(step string) bool { return step == StepPublish || step == StepPublishReport }
