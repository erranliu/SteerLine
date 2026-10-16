package runnercontract

import (
	"encoding/json"
	"time"
)

// Caller-owned values of the proposed check/v1 public contract. Native policy
// and result production remain exclusively in Runner.
type CheckInput struct {
	Repository  Repository        `json:"repository"`
	Execution   ExecutionInput    `json:"execution"`
	Publication *Publication      `json:"publication,omitempty"`
	Correlation map[string]string `json:"correlation,omitempty"`
}
type CheckStop struct {
	CheckID          string `json:"check_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
}
type CheckReceipt struct {
	CommandID     string `json:"command_id"`
	CheckID       string `json:"check_id"`
	ReceiptStatus string `json:"receipt_status"`
	ActionStatus  string `json:"action_status"`
}
type CheckCommand struct {
	CommandID     string `json:"command_id"`
	RequestID     string `json:"request_id"`
	CheckID       string `json:"check_id"`
	Command       string `json:"command"`
	ReceiptStatus string `json:"receipt_status"`
	ActionStatus  string `json:"action_status"`
	Diagnostic    string `json:"diagnostic,omitempty"`
}
type CheckItem struct {
	ID       string          `json:"id"`
	Status   string          `json:"status"`
	Reason   string          `json:"reason,omitempty"`
	Identity string          `json:"identity,omitempty"`
	Evidence []CheckEvidence `json:"evidence"`
}
type CheckEvidence struct {
	Source string `json:"source"`
	Text   string `json:"text"`
}
type CheckResources struct {
	Status     string `json:"status"`
	Diagnostic string `json:"diagnostic,omitempty"`
}
type CheckControls struct {
	CanStop bool   `json:"can_stop"`
	Reason  string `json:"reason,omitempty"`
}
type Check struct {
	CheckID          string            `json:"check_id"`
	Revision         uint64            `json:"revision"`
	InputDigest      string            `json:"input_digest"`
	ResourceDigest   string            `json:"resource_digest"`
	Context          CheckInput        `json:"context"`
	ActualContext    map[string]string `json:"actual_context"`
	UpdatedAt        time.Time         `json:"updated_at"`
	Status           string            `json:"status"`
	CapabilityStatus string            `json:"capability_status"`
	RequiredChecks   []string          `json:"required_checks"`
	Items            []CheckItem       `json:"items"`
	Resources        CheckResources    `json:"resources"`
	Controls         CheckControls     `json:"controls"`
	PR               *PRFact           `json:"pr,omitempty"`
}
type CheckInspection struct {
	Command *CheckCommand `json:"command"`
	Check   *Check        `json:"check"`
}
type CheckSupport struct {
	Adapter     string   `json:"adapter"`
	TargetKind  string   `json:"target_kind"`
	AccessModes []string `json:"access_modes"`
}
type CheckCapabilities struct {
	Version          int             `json:"version"`
	Supported        []CheckSupport  `json:"supported"`
	InputSchema      json.RawMessage `json:"input_schema"`
	ReceiptSchema    json.RawMessage `json:"receipt_schema"`
	InspectionSchema json.RawMessage `json:"inspection_schema"`
}
