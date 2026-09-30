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

const WorkerFlowRuntimeSchema = 10

// ServiceCheckpoint contains only service-owned recovery state. Task and worker
// state is read from domain rows in the same transaction as this checkpoint.
type ServiceCheckpoint struct {
	SchemaVersion int               `json:"schema_version"`
	Revision      uint64            `json:"revision"`
	UpdatedAt     time.Time         `json:"updated_at"`
	HeartbeatAt   time.Time         `json:"heartbeat_at"`
	Service       ServiceState      `json:"service"`
	Repository    Repository        `json:"repository"`
	Operation     *ControlOperation `json:"operation,omitempty"`
	Blockers      []ControlBlocker  `json:"blockers,omitempty"`
	ReservedTask  *string           `json:"reserved_task,omitempty"`
}

// Runtime is a sampled client view plus the service actor's working domain rows.
// Only ServiceCheckpoint is serialized into the projection table.
type Runtime struct {
	SchemaVersion int                  `json:"schema_version"`
	Revision      uint64               `json:"revision"`
	UpdatedAt     time.Time            `json:"updated_at"`
	HeartbeatAt   time.Time            `json:"heartbeat_at"`
	Service       ServiceState         `json:"service"`
	Repository    Repository           `json:"repository"`
	Tasks         []TaskRecord         `json:"tasks,omitempty"`
	WorkflowRuns  []WorkflowRunRecord  `json:"workflow_runs,omitempty"`
	Executions    []ExecutionRecord    `json:"executions,omitempty"`
	Attempts      []StepAttemptRecord  `json:"attempts,omitempty"`
	Decisions     []DecisionRecord     `json:"decisions,omitempty"`
	Operations    []OperationRecord    `json:"operations,omitempty"`
	Deliveries    []TaskDeliveryRecord `json:"-"`
	CurrentTask   *string              `json:"current_task"`
	SlotOccupied  bool                 `json:"slot_occupied"`
	Runs          []*Run               `json:"runs"`
	Control       ControlState         `json:"control"`
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
// PID/Created/Executable; Orca terminal-v2 uses Terminal/RuntimeID/Incarnation;
// supervised Orca uses coordinator/remote runtime epochs and exact Run/Task/
// Dispatch/execution-host/worktree identity. Kind names the protocol family.
type ProcessIdentity struct {
	Kind                 string `json:"kind,omitempty"`
	PID                  int    `json:"pid,omitempty"`
	Created              string `json:"created,omitempty"`
	Executable           string `json:"executable,omitempty"`
	Terminal             string `json:"terminal,omitempty"`
	RuntimeID            string `json:"runtime_id,omitempty"`
	Incarnation          string `json:"incarnation,omitempty"`
	CoordinatorRuntimeID string `json:"coordinator_runtime_id,omitempty"`
	OrchestrationRun     string `json:"orchestration_run,omitempty"`
	Task                 string `json:"task,omitempty"`
	Dispatch             string `json:"dispatch,omitempty"`
	ExecutionHost        string `json:"execution_host,omitempty"`
	WorktreeID           string `json:"worktree_id,omitempty"`
}
type Run struct {
	RecoveryRequired     bool                    `json:"recovery_required,omitempty"`
	RecoveryState        string                  `json:"recovery_state,omitempty"`
	RecoveryVersion      uint64                  `json:"recovery_version,omitempty"`
	ManualStop           *ManualStopConfirmation `json:"manual_stop,omitempty"`
	TaskID               string                  `json:"task_id"`
	DomainTaskID         string                  `json:"domain_task_id,omitempty"`
	IdentityMapDigest    string                  `json:"identity_map_digest,omitempty"`
	DomainAttemptNumber  int                     `json:"domain_attempt_number,omitempty"`
	WorkflowVersion      string                  `json:"workflow_version,omitempty"`
	WorkflowConfigDigest string                  `json:"workflow_config_digest,omitempty"`
	ExecutionID          string                  `json:"execution_id,omitempty"`
	StepResultPath       *string                 `json:"step_result_path,omitempty"`
	Title                string                  `json:"title"`
	RunID                string                  `json:"run_id"`
	AttemptID            string                  `json:"attempt_id,omitempty"`
	ControlRevision      uint64                  `json:"control_revision,omitempty"`
	Claimant             string                  `json:"claimant"`
	Host                 string                  `json:"host,omitempty"`
	Backend              string                  `json:"backend,omitempty"`
	Protocol             string                  `json:"protocol,omitempty"`
	Agent                string                  `json:"agent,omitempty"`
	Target               ExecutionTarget         `json:"target,omitempty"`
	TaskStatus           string                  `json:"task_status,omitempty"`
	BeadsStatus          string                  `json:"beads_status,omitempty"`
	ExecutionStatus      string                  `json:"execution_status"`
	FinishTaskStatus     string                  `json:"finish_task_status"`
	SessionID            *string                 `json:"codex_session_id"`
	Process              *ProcessIdentity        `json:"process"`
	StartedAt            *time.Time              `json:"started_at"`
	EndedAt              *time.Time              `json:"ended_at"`
	WorktreePath         *string                 `json:"worktree_path"`
	WorktreeBranch       *string                 `json:"worktree_branch"`
	WorkspaceIntent      *WorkspaceIntent        `json:"workspace_intent"`
	Completion           *LegacyCompletionConfig `json:"completion,omitempty"`
	BeadsContext         *LegacyBeadsContext     `json:"beads_context,omitempty"`
	BaseCommit           *string                 `json:"base_commit"`
	PR                   PRState                 `json:"pr"`
	LastEvent            *string                 `json:"last_event"`
	LogPath              *string                 `json:"log_path"`
	ResultPath           *string                 `json:"result_path"`
	Summary              *string                 `json:"summary"`
	Error                *string                 `json:"error"`
	// Decision is set when the agent raised a structured question and the run is
	// paused as finish_task_status=waiting_decision until a human answers.
	Decision *DecisionState `json:"decision,omitempty"`
}

// LegacyBeadsContext is decoded only as a marker for rejecting old runtime
// projections before any native recovery or dispatch action.
type LegacyBeadsContext struct {
	Directory string `json:"beads_dir"`
	ProjectID string `json:"project_id"`
	Database  string `json:"database"`
	Backend   string `json:"backend"`
}

// A human-confirmed stop settles the resource uncertainty, not the outcome of
// any previously sent operation. It applies only to this exact attempt.
type ManualStopConfirmation struct {
	AttemptID   string    `json:"attempt_id"`
	ConfirmedAt time.Time `json:"confirmed_at"`
	ConfirmedBy string    `json:"confirmed_by,omitempty"`
	Basis       string    `json:"basis,omitempty"`
}

// DecisionOption is one choice an agent offers when it needs a human decision.
type DecisionOption struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Detail   string `json:"detail,omitempty"`
	Receiver string `json:"receiver,omitempty"`
	Action   string `json:"action,omitempty"`
}

// DecisionRequest is the structured question carried in an R-a result.
type DecisionRequest struct {
	Question      string            `json:"question"`
	Context       string            `json:"context,omitempty"`
	Options       []DecisionOption  `json:"options"`
	Recommended   string            `json:"recommended,omitempty"`
	AllowFreeText bool              `json:"allow_free_text,omitempty"`
	NativeAsk     *codexAskCall     `json:"native_ask,omitempty"`
	OrcaAsk       *orcaAskCall      `json:"orca_ask,omitempty"`
	HostTarget    *HostActionTarget `json:"host_target,omitempty"`
}

type HostActionTarget struct {
	ExecutionID string `json:"execution_id"`
	TurnID      string `json:"turn_id"`
	BlockerID   string `json:"blocker_id"`
}

// DecisionState is the runtime projection of one decision. SQLite owns the
// accepted request and answer; callers submit answers through CommandService.
type DecisionState struct {
	ID              string              `json:"id"`
	AttemptID       string              `json:"attempt_id,omitempty"`
	Version         uint64              `json:"version"`
	Request         DecisionRequest     `json:"request"`
	Status          string              `json:"status"`
	OptionID        string              `json:"option_id,omitempty"`
	Note            string              `json:"note,omitempty"`
	Answers         map[string][]string `json:"answers,omitempty"`
	SourceAttemptID string              `json:"source_attempt_id,omitempty"`
	RecoveryVersion uint64              `json:"recovery_version,omitempty"`
	AnsweredBy      string              `json:"answered_by,omitempty"`
	AnsweredAt      *time.Time          `json:"answered_at,omitempty"`
	AppliedAt       *time.Time          `json:"applied_at,omitempty"`
	// Decoded only to reject projections written by the old sidecar-answer
	// implementation. These paths are never read as answer authority.
	RequestPath string `json:"request_path,omitempty"`
	AnswerPath  string `json:"answer_path,omitempty"`
}

// DecisionAnswer is an immutable human answer submitted to CommandService.
type DecisionAnswer struct {
	DecisionID              string              `json:"decision_id"`
	RunID                   string              `json:"run_id"`
	OptionID                string              `json:"option_id,omitempty"`
	Note                    string              `json:"note,omitempty"`
	Answers                 map[string][]string `json:"answers,omitempty"`
	AnsweredBy              string              `json:"answered_by"`
	AnsweredAt              time.Time           `json:"answered_at"`
	ExpectedDecisionVersion uint64              `json:"expected_decision_version,omitempty"`
	ExpectedRunRevision     uint64              `json:"expected_run_revision,omitempty"`
	SourceAttemptID         string              `json:"source_attempt_id,omitempty"`
	ExpectedRecoveryVersion uint64              `json:"expected_recovery_version,omitempty"`
	ServiceInstanceID       string              `json:"service_instance_id,omitempty"`
}

// Intent is durable before Git creates anything; actual workspace fields stay null
// until creation and verification succeed.
type WorkspaceIntent struct {
	Path       string `json:"path"`
	Branch     string `json:"branch"`
	BaseCommit string `json:"base_commit"`
}
type PRState struct {
	URL          *string    `json:"url"`
	HeadRevision *string    `json:"head_revision,omitempty"`
	State        string     `json:"state"`
	MergedAt     *time.Time `json:"merged_at"`
	MergeCommit  *string    `json:"merge_commit"`
	QueriedAt    *time.Time `json:"queried_at"`
	QueryError   *string    `json:"query_error"`
}

func ptr[T any](v T) *T { return &v }
func now() time.Time    { return time.Now().UTC() }
