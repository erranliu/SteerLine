package runnercontract

// DeliveryObservation is a bounded, read-only caller projection of one public
// Run snapshot and independently observed command receipts. It is not a CLI
// lifecycle state, persisted authority or permission to send input.
type DeliveryObservation struct {
	Available        bool                 `json:"available"`
	RunID            string               `json:"run_id,omitempty"`
	Revision         uint64               `json:"revision"`
	Submission       string               `json:"submission"`
	Step             string               `json:"step,omitempty"`
	Status           string               `json:"status,omitempty"`
	Phase            string               `json:"phase,omitempty"`
	Diagnostic       string               `json:"diagnostic,omitempty"`
	NativeOutcome    string               `json:"native_outcome,omitempty"`
	FinalVerified    bool                 `json:"final_verified"`
	ContentStatus    string               `json:"content_status,omitempty"`
	PRState          string               `json:"pr_state,omitempty"`
	PRQueryError     string               `json:"pr_query_error,omitempty"`
	StopStatus       string               `json:"stop_status,omitempty"`
	Closed           bool                 `json:"closed"`
	ResumeRequired   bool                 `json:"resume_required"`
	NewInputsAllowed bool                 `json:"new_inputs_allowed"`
	Controls         Controls             `json:"controls"`
	Commands         []CommandObservation `json:"commands"`
}

type CommandObservation struct {
	Command      string `json:"command"`
	RequestID    string `json:"request_id"`
	CommandID    string `json:"command_id,omitempty"`
	ReceiptState string `json:"receipt_state"`
	ActionStatus string `json:"action_status,omitempty"`
}
