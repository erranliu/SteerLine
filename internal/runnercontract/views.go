// Package runnercontract contains caller-owned values of the public CLI contract.
package runnercontract

import "time"

// These values are the public CLI contract, owned by the caller. There is no
// Go module dependency on the independently versioned Runner implementation.
type PRFact struct {
	URL            string     `json:"url"`
	Provider       string     `json:"provider"`
	Repository     string     `json:"repository"`
	Number         int        `json:"number"`
	BaseBranch     string     `json:"base_branch"`
	HeadRepository string     `json:"head_repository"`
	HeadBranch     string     `json:"head_branch"`
	HeadRevision   string     `json:"head_revision,omitempty"`
	State          string     `json:"state"`
	Merged         bool       `json:"merged"`
	MergedAt       *time.Time `json:"merged_at,omitempty"`
	MergeCommit    *string    `json:"merge_commit,omitempty"`
	CheckedAt      time.Time  `json:"checked_at"`
	QueryError     string     `json:"query_error,omitempty"`
}
type Final struct {
	CommandID     string `json:"command_id"`
	AttemptID     string `json:"attempt_id"`
	SessionID     string `json:"session_id"`
	TurnID        string `json:"turn_id"`
	NativeOutcome string `json:"native_outcome"` // unreported is absent provider outcome, not native success.
	Verified      bool   `json:"verified"`
	Source        string `json:"source,omitempty"`
	ItemID        string `json:"item_id,omitempty"`
	Text          string `json:"text,omitempty"`
	Diagnostic    string `json:"diagnostic,omitempty"`
}
type Execution struct {
	RequestedModel           string `json:"requested_model,omitempty"`
	RequestedReasoningEffort string `json:"requested_reasoning_effort,omitempty"`
	WorkspaceScope           string `json:"workspace_scope,omitempty"`
	TurnStarted              bool   `json:"turn_started"`
	AttemptID                string `json:"attempt_id"`
	Generation               string `json:"generation"`
	Backend                  string `json:"backend"`
	Protocol                 string `json:"protocol"`
	SessionID                string `json:"session_id"`
	TurnID                   string `json:"turn_id"`
	NativeOutcome            string `json:"native_outcome"`
	FinalAvailable           bool   `json:"final_available"`
	Workspace                string `json:"workspace"`
	Branch                   string `json:"branch"`
}
type Question struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Options []struct {
		ID     string `json:"id"`
		Label  string `json:"label"`
		Detail string `json:"detail"`
	} `json:"options"`
	AllowFreeText   bool       `json:"allow_free_text"`
	NativeQuestions []Question `json:"native_questions"`
}
type Controls struct {
	CanReply  bool   `json:"can_reply"`
	CanResume bool   `json:"can_resume"`
	CanStop   bool   `json:"can_stop"`
	Reason    string `json:"reason"`
}
type Run struct {
	Correlation      map[string]string `json:"correlation,omitempty"`
	Workflow         Workflow          `json:"workflow"`
	Audit            *Audit            `json:"audit,omitempty"`
	Materials        []Material        `json:"materials"`
	RunID            string            `json:"run_id"`
	Revision         uint64            `json:"revision"`
	Title            string            `json:"title"`
	InputDigest      string            `json:"input_digest"`
	ResourceDigest   string            `json:"resource_digest"`
	Status           string            `json:"status"`
	Phase            string            `json:"phase"`
	Step             string            `json:"step"`
	Diagnostic       string            `json:"diagnostic"`
	NewInputsAllowed bool              `json:"new_inputs_allowed"`
	ResumeRequired   bool              `json:"resume_required"`
	StopStatus       string            `json:"stop_status"`
	Execution        Execution         `json:"execution"`
	Question         *Question         `json:"question"`
	Controls         Controls          `json:"controls"`
	PR               *PRFact           `json:"pr"`
	Final            *Final            `json:"final"`
	Summary          string            `json:"summary"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type Material struct {
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	Commit       string `json:"commit,omitempty"`
	Digest       string `json:"digest"`
	SnapshotPath string `json:"snapshot_path"`
}

type Audit struct {
	Model           string        `json:"model"`
	ReasoningEffort string        `json:"reasoning_effort"`
	RepairCount     uint64        `json:"repair_count"`
	Results         []AuditResult `json:"results"`
}

type AuditResult struct {
	Step                  string    `json:"step"`
	SourceReportCommandID string    `json:"source_report_command_id"`
	CommandID             string    `json:"command_id"`
	TurnID                string    `json:"turn_id"`
	Verdict               string    `json:"verdict,omitempty"`
	Findings              []Finding `json:"findings,omitempty"`
	Failure               string    `json:"failure,omitempty"`
}

type Finding struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
}
type Command struct {
	CommandID     string  `json:"command_id"`
	RequestID     string  `json:"request_id"`
	RunID         *string `json:"run_id"`
	Command       string  `json:"command"`
	ReceiptStatus string  `json:"receipt_status"`
	ActionStatus  string  `json:"action_status"`
}
type Inspection struct {
	Command *Command `json:"command"`
	Run     *Run     `json:"run"`
}
type Event struct {
	Transition *WorkflowTransition `json:"transition,omitempty"`
	EventID    string              `json:"event_id"`
	RunID      string              `json:"run_id"`
	Revision   uint64              `json:"revision"`
	Type       string              `json:"type"`
	CommandID  string              `json:"command_id"`
	AttemptID  string              `json:"attempt_id"`
	Final      *Final              `json:"final"`
	CreatedAt  time.Time           `json:"created_at"`
}
type EventPage struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

type WorkflowState struct {
	Stage      string `json:"workflow_stage"`
	Status     string `json:"workflow_status"`
	Diagnostic string `json:"diagnostic,omitempty"`
}
type WorkflowTransition struct {
	From WorkflowState `json:"from"`
	To   WorkflowState `json:"to"`
}
