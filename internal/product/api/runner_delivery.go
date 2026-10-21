package api

import "time"

// RunnerDeliverySummary is a read-only display DTO. It has no owner bindings,
// stored lifecycle state or execution capabilities.
type RunnerDeliverySummary struct {
	Available        bool                   `json:"available"`
	RunID            string                 `json:"run_id,omitempty"`
	Revision         uint64                 `json:"revision"`
	Submission       string                 `json:"submission"`
	Step             string                 `json:"step,omitempty"`
	Status           string                 `json:"status,omitempty"`
	Phase            string                 `json:"phase,omitempty"`
	Diagnostic       string                 `json:"diagnostic,omitempty"`
	NativeOutcome    string                 `json:"native_outcome,omitempty"`
	FinalVerified    bool                   `json:"final_verified"`
	ContentStatus    string                 `json:"content_status,omitempty"`
	PRState          string                 `json:"pr_state,omitempty"`
	PRCheckedAt      *time.Time             `json:"pr_checked_at,omitempty"`
	PRQueryError     string                 `json:"pr_query_error,omitempty"`
	StopStatus       string                 `json:"stop_status,omitempty"`
	Closed           bool                   `json:"closed"`
	ResumeRequired   bool                   `json:"resume_required"`
	NewInputsAllowed bool                   `json:"new_inputs_allowed"`
	Controls         RunnerDeliveryControls `json:"controls"`
	Commands         []RunnerCommandSummary `json:"commands"`
}

type RunnerDeliveryControls struct {
	CanReply     bool   `json:"can_reply"`
	CanResume    bool   `json:"can_resume"`
	CanStop      bool   `json:"can_stop"`
	CanMessage   bool   `json:"can_message"`
	CanInterrupt bool   `json:"can_interrupt"`
	CanClose     bool   `json:"can_close"`
	Reason       string `json:"reason"`
}

type RunnerCommandSummary struct {
	Command      string `json:"command"`
	RequestID    string `json:"request_id"`
	CommandID    string `json:"command_id,omitempty"`
	ReceiptState string `json:"receipt_state"`
	ActionStatus string `json:"action_status,omitempty"`
}
