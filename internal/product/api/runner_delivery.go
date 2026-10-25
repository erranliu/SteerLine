package api

import "time"

// ExecutionWaitSummary contains only public display values, not owner handles.
type ExecutionWaitSummary struct {
	Contract      string     `json:"contract"`
	OperationID   string     `json:"operation_id"`
	Query         string     `json:"query"`
	Stage         string     `json:"stage"`
	ReasonCode    string     `json:"reason_code"`
	StartedAt     time.Time  `json:"started_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeadlineAt    time.Time  `json:"deadline_at"`
	CallsStarted  int        `json:"calls_started"`
	MaxCalls      int        `json:"max_calls"`
	NextAttemptAt *time.Time `json:"next_attempt_at"`
	Diagnostic    string     `json:"diagnostic"`
}

// RunnerDeliverySummary is a read-only display DTO. It has no owner bindings,
// stored lifecycle state or execution capabilities.
type RunnerDeliverySummary struct {
	ExecutionWait    *ExecutionWaitSummary  `json:"execution_wait"`
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
