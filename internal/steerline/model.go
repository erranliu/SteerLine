package steerline

import (
	"encoding/json"
	"time"
)

type Repository struct {
	Path     string `json:"path"`
	Remote   string `json:"remote"`
	Branch   string `json:"branch"`
	Identity string `json:"identity"`
}

const CurrentRuntimeSchema = 5

type Runtime struct {
	SchemaVersion int          `json:"schema_version"`
	UpdatedAt     time.Time    `json:"updated_at"`
	HeartbeatAt   time.Time    `json:"heartbeat_at"`
	Service       ServiceState `json:"service"`
	Repository    Repository   `json:"repository"`
	CurrentTask   *string      `json:"current_task"`
	SlotOccupied  bool         `json:"slot_occupied"`
	Runs          []*Run       `json:"runs"`
	Control       ControlState `json:"control"`
}
type ServiceState struct {
	Mode                string           `json:"mode"`
	InstanceID          string           `json:"instance_id"`
	PID                 int              `json:"pid"`
	Status              string           `json:"status"`
	Claimant            string           `json:"claimant"`
	Error               *string          `json:"error"`
	ProcessIdentity     *ProcessIdentity `json:"process_identity"`
	StartupID           *string          `json:"startup_id"`
	ConfigDigest        string           `json:"config_digest"`
	Ready               bool             `json:"ready"`
	ControlProtocol     int              `json:"control_protocol"`
	Capabilities        []string         `json:"capabilities"`
	HeartbeatIntervalMS int64            `json:"heartbeat_interval_ms"`
}

type ControlState struct {
	DispatchMode   string            `json:"dispatch_mode"`
	DispatchPaused bool              `json:"dispatch_paused"`
	Revision       uint64            `json:"revision"`
	Operation      *ControlOperation `json:"operation"`
	Blockers       []ControlBlocker  `json:"blockers"`
}

type ControlOperation struct {
	RequestID     string          `json:"request_id"`
	Action        string          `json:"action"`
	TaskID        string          `json:"task_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Phase         string          `json:"phase"`
	RunID         string          `json:"run_id,omitempty"`
	AttemptID     string          `json:"attempt_id,omitempty"`
	DecisionID    string          `json:"decision_id,omitempty"`
	RequestDigest string          `json:"request_digest"`
	Result        *ControlReceipt `json:"result,omitempty"`
}

type ControlBlocker struct {
	RequestID  string `json:"request_id"`
	Action     string `json:"action"`
	TaskID     string `json:"task_id,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	AttemptID  string `json:"attempt_id,omitempty"`
	DecisionID string `json:"decision_id,omitempty"`
	Reason     string `json:"reason"`
}

type TaskSnapshot struct {
	Version            int             `json:"version"`
	InstanceID         string          `json:"instance_id"`
	RepositoryIdentity string          `json:"repository_identity"`
	QueriedAt          time.Time       `json:"queried_at"`
	LastSuccessAt      *time.Time      `json:"last_success_at"`
	Error              *string         `json:"error"`
	BlockReason        string          `json:"block_reason,omitempty"`
	Tasks              []TaskCandidate `json:"tasks"`
}
type TaskCandidate struct {
	TaskID  string `json:"task_id"`
	Title   string `json:"title"`
	Type    string `json:"type"`
	Summary string `json:"summary,omitempty"`
}

// ProcessIdentity is the host-neutral identity of a running worker. OS hosts use
// PID/Created/Executable; the Orca host uses Terminal/RuntimeID/Incarnation.
// Kind is "os" for local runs and "orca" when hosted by Orca.
type ProcessIdentity struct {
	Kind        string `json:"kind,omitempty"`
	PID         int    `json:"pid,omitempty"`
	Created     string `json:"created,omitempty"`
	Executable  string `json:"executable,omitempty"`
	Terminal    string `json:"terminal,omitempty"`
	RuntimeID   string `json:"runtime_id,omitempty"`
	Incarnation string `json:"incarnation,omitempty"`
}
type Run struct {
	TaskID           string            `json:"task_id"`
	Title            string            `json:"title"`
	RunID            string            `json:"run_id"`
	AttemptID        string            `json:"attempt_id,omitempty"`
	ControlRevision  uint64            `json:"control_revision,omitempty"`
	Claimant         string            `json:"claimant"`
	Host             string            `json:"host,omitempty"`
	BeadsStatus      string            `json:"beads_status"`
	ExecutionStatus  string            `json:"execution_status"`
	FinishTaskStatus string            `json:"finish_task_status"`
	SessionID        *string           `json:"codex_session_id"`
	Process          *ProcessIdentity  `json:"process"`
	StartedAt        *time.Time        `json:"started_at"`
	EndedAt          *time.Time        `json:"ended_at"`
	WorktreePath     *string           `json:"worktree_path"`
	WorktreeBranch   *string           `json:"worktree_branch"`
	WorkspaceIntent  *WorkspaceIntent  `json:"workspace_intent"`
	Completion       *CompletionConfig `json:"completion"`
	BeadsContext     *BeadsContext     `json:"beads_context"`
	BaseCommit       *string           `json:"base_commit"`
	PR               PRState           `json:"pr"`
	LastEvent        *string           `json:"last_event"`
	LogPath          *string           `json:"log_path"`
	ResultPath       *string           `json:"result_path"`
	Summary          *string           `json:"summary"`
	Error            *string           `json:"error"`
	// Decision is set when the agent raised a structured question and the run is
	// paused as finish_task_status=waiting_decision until a human answers.
	Decision *DecisionState `json:"decision,omitempty"`
}

// DecisionOption is one choice an agent offers when it needs a human decision.
type DecisionOption struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
}

// DecisionRequest is the structured question carried in an R-a result.
type DecisionRequest struct {
	Question      string           `json:"question"`
	Context       string           `json:"context,omitempty"`
	Options       []DecisionOption `json:"options"`
	Recommended   string           `json:"recommended,omitempty"`
	AllowFreeText bool             `json:"allow_free_text,omitempty"`
}

// DecisionState is the durable scheduler record of one decision. RequestPath and
// AnswerPath are absolute so the CLI/viewer can answer without the service.
// Status moves pending -> resuming -> applied; a restart never auto-resumes.
type DecisionState struct {
	ID          string          `json:"id"`
	Request     DecisionRequest `json:"request"`
	Status      string          `json:"status"`
	OptionID    string          `json:"option_id,omitempty"`
	Note        string          `json:"note,omitempty"`
	AnsweredBy  string          `json:"answered_by,omitempty"`
	AnsweredAt  *time.Time      `json:"answered_at,omitempty"`
	AppliedAt   *time.Time      `json:"applied_at,omitempty"`
	RequestPath string          `json:"request_path"`
	AnswerPath  string          `json:"answer_path"`
}

// DecisionAnswer is written by a human (via `steerline answer` or the viewer).
// The service is the only component that applies it.
type DecisionAnswer struct {
	DecisionID string    `json:"decision_id"`
	RunID      string    `json:"run_id"`
	OptionID   string    `json:"option_id,omitempty"`
	Note       string    `json:"note,omitempty"`
	AnsweredBy string    `json:"answered_by"`
	AnsweredAt time.Time `json:"answered_at"`
}

// Intent is durable before Git creates anything; actual workspace fields stay null
// until creation and verification succeed.
type WorkspaceIntent struct {
	Path       string `json:"path"`
	Branch     string `json:"branch"`
	BaseCommit string `json:"base_commit"`
}
type PRState struct {
	URL         *string    `json:"url"`
	State       string     `json:"state"`
	MergedAt    *time.Time `json:"merged_at"`
	MergeCommit *string    `json:"merge_commit"`
	QueriedAt   *time.Time `json:"queried_at"`
	QueryError  *string    `json:"query_error"`
}
type Result struct {
	TaskID       string  `json:"task_id"`
	RunID        string  `json:"run_id"`
	Outcome      string  `json:"outcome"`
	PRURL        *string `json:"pr_url"`
	WorktreePath *string `json:"worktree_path"`
	BaseCommit   *string `json:"base_commit"`
	Summary      *string `json:"summary"`
	Error        *string `json:"error"`
	// Decision is present only when Outcome is "needs_decision".
	Decision *DecisionRequest `json:"decision"`
}

func ptr[T any](v T) *T { return &v }
func now() time.Time    { return time.Now().UTC() }
