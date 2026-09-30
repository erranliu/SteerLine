package contracts

import "time"

type ProbeIdentity struct {
	Execution     string `json:"execution"`
	Turn          string `json:"turn"`
	Nonce         string `json:"nonce"`
	ProfileDigest string `json:"profile_digest"`
	Runtime       string `json:"runtime"`
	Terminal      string `json:"terminal"`
	Incarnation   string `json:"incarnation"`
	Worktree      string `json:"worktree"`
}

type ProbeReceipt struct {
	ProbeIdentity
	InputSHA256 string `json:"input_sha256"`
	GitHead     string `json:"git_head"`
	Read        bool   `json:"read"`
	Write       bool   `json:"write"`
	Delete      bool   `json:"delete"`
}

type ProbeToolEvidence struct {
	SourceIdentity string `json:"source_identity"`
	CallID         string `json:"call_id"`
	MessageID      string `json:"message_id"`
	Command        string `json:"command"`
	Marker         string `json:"marker"`
	Verified       bool   `json:"verified"`
	FailureCode    string `json:"failure_code,omitempty"`
}

type ToolContext struct {
	SourceIdentity string `json:"source_identity,omitempty"`
	TaskBound      bool   `json:"task_bound,omitempty"`
	CallID         string `json:"call_id,omitempty"`
	Started        bool   `json:"started,omitempty"`
}

type HostFact struct {
	Reason      string `json:"reason"`
	Confidence  string `json:"confidence"`
	Source      string `json:"source"`
	Detail      string `json:"detail,omitempty"`
	Runtime     string `json:"runtime"`
	Terminal    string `json:"terminal"`
	Incarnation string `json:"incarnation"`
	Turn        string `json:"turn,omitempty"`
}

type WorkerEnvironment struct {
	OriginalTerminal   *ProcessIdentity      `json:"original_terminal,omitempty"`
	ProfileDigest      string                `json:"profile_digest"`
	LaunchCommand      string                `json:"launch_command,omitempty"`
	WriteRoots         []string              `json:"write_roots,omitempty"`
	Runtime            string                `json:"runtime,omitempty"`
	Preparation        *WorkerPreparation    `json:"preparation"`
	History            []WorkerPreparation   `json:"history,omitempty"`
	Blocker            *WorkerHostBlocker    `json:"blocker,omitempty"`
	DeferredDecisionID string                `json:"deferred_decision_id,omitempty"`
	DeferredFeedback   []EnvironmentFeedback `json:"deferred_feedback,omitempty"`
	Ready              bool                  `json:"ready"`
}

type EnvironmentFeedback struct {
	DecisionID string `json:"decision_id"`
	TurnID     string `json:"turn_id"`
	CommandID  string `json:"command_id"`
}

type WorkerPreparation struct {
	TurnID       string            `json:"turn_id"`
	Nonce        string            `json:"nonce"`
	StartedAt    time.Time         `json:"started_at"`
	Deadline     time.Time         `json:"deadline"`
	Probe        ProbeIdentity     `json:"probe"`
	Command      string            `json:"command,omitempty"`
	InputDigest  string            `json:"input_digest,omitempty"`
	ScriptDigest string            `json:"script_digest,omitempty"`
	Receipt      *ProbeReceipt     `json:"receipt,omitempty"`
	Evidence     ProbeToolEvidence `json:"evidence"`
	Verified     bool              `json:"verified"`
	Invalidated  bool              `json:"invalidated,omitempty"`
}

type WorkerHostBlocker struct {
	ID                 string    `json:"id"`
	Fact               HostFact  `json:"fact"`
	ObservedAt         time.Time `json:"observed_at"`
	TurnID             string    `json:"turn_id"`
	DecisionID         string    `json:"decision_id,omitempty"`
	RecheckOperationID string    `json:"recheck_operation_id,omitempty"`
}
