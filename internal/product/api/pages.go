package api

import "time"

// ProjectRef contains accepted display values and an immutable project identity.
// It cannot construct a backend, execute a tool, or open a database.
type ProjectRef struct {
	Key             string              `json:"key"`
	Name            string              `json:"name"`
	ConfigPath      string              `json:"config_path"`
	Identity        string              `json:"identity"`
	RunDir          string              `json:"run_dir"`
	ApplicationHome string              `json:"application_home"`
	ApplicationBin  string              `json:"application_bin"`
	Error           string              `json:"error"`
	Conflict        string              `json:"conflict"`
	Presentation    ProjectPresentation `json:"presentation"`
}

type ProjectPresentation struct {
	ServiceInstruction string                    `json:"service_instruction"`
	NativeWorkflow     bool                      `json:"native_workflow"`
	ConfigDigest       string                    `json:"config_digest"`
	TaskAuthority      string                    `json:"task_authority"`
	WorkflowProtocol   string                    `json:"workflow_protocol"`
	ShutdownGrace      string                    `json:"shutdown_grace"`
	DefaultProfileID   string                    `json:"default_profile_id"`
	Profiles           []ExecutionProfileSummary `json:"profiles"`
}

// ExecutionProfileSummary deliberately has no implementation envelope, command
// arguments, credentials, or mutable execution configuration.
type ExecutionProfileSummary struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Host                  string   `json:"host"`
	Agent                 string   `json:"agent"`
	Digest                string   `json:"digest"`
	AcceptedConfigVersion string   `json:"accepted_config_version"`
	UnavailableReason     string   `json:"unavailable_reason"`
	ImplementationDisplay []string `json:"implementation_display"`
	Details               []string `json:"details"`
}

type ProjectSnapshot struct {
	CandidateFreshnessLimit time.Duration       `json:"candidate_freshness_limit"`
	DecisionReasons         map[string]string   `json:"decision_reasons"`
	SelectedTaskID          string              `json:"selected_task_id"`
	FullReadAt              time.Time           `json:"full_read_at"`
	Connection              ServiceConnection   `json:"connection"`
	Observation             ObservationSnapshot `json:"observation"`
	Tasks                   *TaskCandidates     `json:"tasks"`
	TaskError               string              `json:"task_error"`
	NativeTasks             *TaskPage           `json:"native_tasks"`
	SelectedNative          *TaskDetail         `json:"selected_native"`
	NativeTaskError         string              `json:"native_task_error"`
	History                 *HistorySummary     `json:"history"`
	HistoryError            string              `json:"history_error"`
}

type ServiceConnection struct {
	Snapshot        ObservationSnapshot    `json:"snapshot"`
	Process         ProcessProbe           `json:"process"`
	State           ServiceConnectionState `json:"state"`
	IdentityMatched bool                   `json:"identity_matched"`
	ConfigMatched   bool                   `json:"config_matched"`
	Protocol        int                    `json:"protocol"`
	Capabilities    []string               `json:"capabilities"`
	CanControl      bool                   `json:"can_control"`
	CanStop         bool                   `json:"can_stop"`
	CanStart        bool                   `json:"can_start"`
	StartupID       string                 `json:"startup_id"`
	Diagnostic      string                 `json:"diagnostic"`
}

type ObservationSnapshot struct {
	Page           *ServicePage  `json:"page"`
	State          SnapshotState `json:"state"`
	SampledAt      time.Time     `json:"sampled_at"`
	HeartbeatAge   time.Duration `json:"heartbeat_age"`
	FreshnessLimit time.Duration `json:"freshness_limit"`
	ClockSkew      bool          `json:"clock_skew"`
	Error          string        `json:"error"`
}

// ServicePage has only display facts; domain rows and private actor state are
// never carried across the product boundary.
type ServicePage struct {
	Revision     uint64              `json:"revision"`
	UpdatedAt    time.Time           `json:"updated_at"`
	HeartbeatAt  time.Time           `json:"heartbeat_at"`
	Service      ServiceSummary      `json:"service"`
	Repository   RepositorySummary   `json:"repository"`
	CurrentTask  *string             `json:"current_task"`
	SlotOccupied bool                `json:"slot_occupied"`
	Runs         []*CandidateSummary `json:"runs"`
	Control      ControlSummary      `json:"control"`
}

type RepositorySummary struct{ Path, Remote, Branch, Identity string }
type ServiceSummary struct {
	InstanceID      string   `json:"instance_id"`
	Status          string   `json:"status"`
	ConfigDigest    string   `json:"config_digest"`
	Claimant        string   `json:"claimant"`
	Mode            string   `json:"mode"`
	PID             int      `json:"pid"`
	Error           *string  `json:"error"`
	StartupID       *string  `json:"startup_id"`
	Ready           bool     `json:"ready"`
	ControlProtocol int      `json:"control_protocol"`
	Capabilities    []string `json:"capabilities"`
}
type ControlSummary struct {
	DispatchMode   string           `json:"dispatch_mode"`
	DispatchPaused bool             `json:"dispatch_paused"`
	Revision       uint64           `json:"revision"`
	Operation      *PendingControl  `json:"operation"`
	Blockers       []ControlBlocker `json:"blockers"`
}
type PendingControl struct{ RequestID, Action, TaskID, Phase, RunID, AttemptID, DecisionID string }
type ControlBlocker struct{ RequestID, Action, TaskID, RunID, AttemptID, DecisionID, Reason string }

type TaskCandidates struct {
	Version            int             `json:"version"`
	InstanceID         string          `json:"instance_id"`
	RepositoryIdentity string          `json:"repository_identity"`
	QueriedAt          time.Time       `json:"queried_at"`
	LastSuccessAt      *time.Time      `json:"last_success_at"`
	Error              *string         `json:"error"`
	BlockReason        string          `json:"block_reason"`
	Tasks              []TaskCandidate `json:"tasks"`
}
type TaskCandidate struct{ TaskID, Title, Type, Summary string }

// CandidateSummary is the selected worker's page, not an execution record.
// Session identity and revisions are explicit so commands cannot guess a run.
type CandidateSummary struct {
	DeliveryOutcome    string                   `json:"delivery_outcome"`
	SessionID          string                   `json:"session_id"`
	SessionGeneration  string                   `json:"session_generation"`
	CurrentTurnID      string                   `json:"current_turn_id"`
	TurnNumber         uint64                   `json:"turn_number"`
	Abandonment        *AbandonmentSummary      `json:"abandonment"`
	ExecutionProfile   *ExecutionProfileSummary `json:"execution_profile"`
	PRHistory          []string                 `json:"pr_history"`
	CandidateCreatedAt time.Time                `json:"candidate_created_at"`
	CandidateEndedAt   *time.Time               `json:"candidate_ended_at"`
	CandidateTiming    string                   `json:"candidate_timing"`
	ResourceStatus     string                   `json:"resource_status"`
	EnvironmentBlocker *EnvironmentBlocker      `json:"environment_blocker"`
	RecoveryRequired   bool                     `json:"recovery_required"`
	RecoveryState      string                   `json:"recovery_state"`
	RecoveryVersion    uint64                   `json:"recovery_version"`
	TaskID             string                   `json:"task_id"`
	DomainTaskID       string                   `json:"domain_task_id"`
	WorkflowVersion    string                   `json:"workflow_version"`
	ExecutionID        string                   `json:"execution_id"`
	Title              string                   `json:"title"`
	RunID              string                   `json:"run_id"`
	AttemptID          string                   `json:"attempt_id"`
	ControlRevision    uint64                   `json:"control_revision"`
	Claimant           string                   `json:"claimant"`
	Host               string                   `json:"host"`
	Backend            string                   `json:"backend"`
	Protocol           string                   `json:"protocol"`
	Agent              string                   `json:"agent"`
	Target             TargetSummary            `json:"target"`
	TaskStatus         string                   `json:"task_status"`
	ExecutionStatus    string                   `json:"execution_status"`
	FinishTaskStatus   string                   `json:"finish_task_status"`
	Process            *WorkerLocation          `json:"process"`
	StartedAt          *time.Time               `json:"started_at"`
	EndedAt            *time.Time               `json:"ended_at"`
	WorktreePath       *string                  `json:"worktree_path"`
	WorktreeBranch     *string                  `json:"worktree_branch"`
	BaseCommit         *string                  `json:"base_commit"`
	LastEvent          *string                  `json:"last_event"`
	LogPath            *string                  `json:"log_path"`
	ResultPath         *string                  `json:"result_path"`
	Summary            *string                  `json:"summary"`
	Error              *string                  `json:"error"`
	PR                 PRSummary                `json:"pr"`
	Decision           *DecisionSummary         `json:"decision"`
}
type TargetSummary struct {
	ID string `json:"id"`
}
type WorkerLocation struct {
	Terminal string `json:"terminal"`
}
type AbandonmentSummary struct {
	Reason      string    `json:"reason"`
	RequestedAt time.Time `json:"requested_at"`
}
type EnvironmentBlocker struct {
	ID      string          `json:"id"`
	TurnID  string          `json:"turn_id"`
	Version uint64          `json:"version"`
	Fact    EnvironmentFact `json:"fact"`
}
type EnvironmentFact struct{ Reason, Detail, Confidence, Source string }
type PRSummary struct {
	URL          *string    `json:"url"`
	HeadRevision *string    `json:"head_revision"`
	MergeCommit  *string    `json:"merge_commit"`
	QueryError   *string    `json:"query_error"`
	State        string     `json:"state"`
	MergedAt     *time.Time `json:"merged_at"`
	QueriedAt    *time.Time `json:"queried_at"`
}

type DecisionOption struct{ ID, Label, Detail string }
type DecisionRequest struct {
	Question           string           `json:"question"`
	Context            string           `json:"context"`
	Recommended        string           `json:"recommended"`
	Options            []DecisionOption `json:"options"`
	AllowFreeText      bool             `json:"allow_free_text"`
	HasNativeQuestions bool             `json:"has_native_questions"`
}
type DecisionSummary struct {
	ID         string          `json:"id"`
	AttemptID  string          `json:"attempt_id"`
	Status     string          `json:"status"`
	Version    uint64          `json:"version"`
	Request    DecisionRequest `json:"request"`
	OptionID   string          `json:"option_id"`
	Note       string          `json:"note"`
	AnsweredBy string          `json:"answered_by"`
	AnsweredAt *time.Time      `json:"answered_at"`
	AppliedAt  *time.Time      `json:"applied_at"`
}
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
type DecisionInput struct {
	Answer      DecisionAnswer `json:"answer"`
	RequestedBy string         `json:"requested_by"`
	CommandID   string         `json:"command_id"`
}

// HistorySummary carries only the history needed for numbering, navigation and
// rendering. It excludes operations, implementation envelopes and fixed contracts.
type HistorySummary struct {
	WorkflowRuns []WorkflowSummary  `json:"workflow_runs"`
	Attempts     []AttemptSummary   `json:"attempts"`
	Decisions    []DecisionHistory  `json:"decisions"`
	Executions   []ExecutionSummary `json:"executions"`
}
type WorkflowSummary struct {
	WorkflowRunID    string                   `json:"workflow_run_id"`
	TaskID           string                   `json:"task_id"`
	CreatedAt        time.Time                `json:"created_at"`
	ExecutionProfile *ExecutionProfileSummary `json:"execution_profile"`
}
type AttemptSummary struct {
	ExecutionID   string    `json:"execution_id"`
	AttemptID     string    `json:"attempt_id"`
	AttemptNumber int       `json:"attempt_number"`
	CreatedAt     time.Time `json:"created_at"`
}
type DecisionHistory struct {
	WorkflowRunID    string           `json:"workflow_run_id"`
	DecisionID       string           `json:"decision_id"`
	ExecutionID      string           `json:"execution_id"`
	AttemptID        string           `json:"attempt_id"`
	Status           string           `json:"status"`
	Version          uint64           `json:"version"`
	CreatedAt        time.Time        `json:"created_at"`
	Request          *DecisionRequest `json:"request"`
	Answer           *DecisionAnswer  `json:"answer"`
	Diagnostic       string           `json:"diagnostic"`
	AnswerDiagnostic string           `json:"answer_diagnostic,omitempty"`
}
type ExecutionSummary struct {
	ExecutionID string    `json:"execution_id"`
	VerifiedAt  time.Time `json:"verified_at"`
	Details     []string  `json:"details"`
}

type TaskSummary struct {
	TaskID   string `json:"task_id"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Version  uint64 `json:"version"`
	Claimant string `json:"claimant"`
}
type DependencySummary struct {
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	Delivered bool   `json:"delivered"`
	Blocker   string `json:"blocker"`
}
type DeliverySummary struct {
	PRURL         string    `json:"pr_url"`
	WorkflowRunID string    `json:"workflow_run_id"`
	ExecutionID   string    `json:"execution_id"`
	MergedAt      time.Time `json:"merged_at"`
}
type TaskDetail struct {
	TaskSummary
	ProjectID         string              `json:"project_id"`
	Priority          int                 `json:"priority"`
	SpecPath          string              `json:"spec_path"`
	WorkflowRuns      []WorkflowSummary   `json:"workflow_runs"`
	Delivery          *DeliverySummary    `json:"delivery,omitempty"`
	Predecessors      []DependencySummary `json:"predecessors"`
	Successors        []string            `json:"successors"`
	DependencyReady   bool                `json:"dependency_ready"`
	UnmetPredecessors []DependencySummary `json:"unmet_predecessors"`
	StartupBlockers   []string            `json:"startup_blockers"`
}
type TaskCapabilities struct {
	DependencyReadiness string `json:"dependency_readiness"`
	DependencyBlockers  string `json:"dependency_blockers"`
	StartupBlockers     string `json:"startup_blockers"`
}
type TaskPage struct {
	Protocol         string           `json:"protocol"`
	ProjectID        string           `json:"project_id"`
	AuthorityVersion uint64           `json:"authority_version"`
	GraphRevision    uint64           `json:"graph_revision"`
	DatabaseRevision uint64           `json:"database_revision"`
	DatabaseID       string           `json:"database_id"`
	SampledAt        time.Time        `json:"sampled_at"`
	Capabilities     TaskCapabilities `json:"capabilities"`
	Tasks            []TaskDetail     `json:"tasks"`
	NextCursor       string           `json:"next_cursor,omitempty"`
	HasMore          bool             `json:"has_more"`
}

// CommandReceipt keeps the original request and target with the actual outcome.
// It has no persisted request payload, idempotency storage key or write access.
type CommandReceipt struct {
	ProjectIdentity string    `json:"project_identity"`
	CommandID       string    `json:"command_id"`
	Kind            string    `json:"kind"`
	TargetID        string    `json:"target_id"`
	ExpectedVersion uint64    `json:"expected_version"`
	RequestedBy     string    `json:"requested_by"`
	Status          string    `json:"status"`
	ResultCode      string    `json:"result_code,omitempty"`
	ResultError     string    `json:"result_error,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type LogBatch struct {
	Cursor      ActivityCursor  `json:"cursor"`
	Reset       bool            `json:"reset"`
	Activity    []ActivityEvent `json:"activity"`
	Diagnostics []string        `json:"diagnostics"`
}
type CapabilityFilter struct{ Backend, Protocol, TargetID, ExecutionID, AttemptID, Source, SourceIdentity string }

type RecoveryInspection struct {
	Version                int                `json:"version"`
	SampledAt              time.Time          `json:"sampled_at"`
	RunID                  string             `json:"run_id"`
	RunRevision            uint64             `json:"run_revision"`
	ServiceInstanceID      string             `json:"service_instance_id"`
	ServiceStatus          string             `json:"service_status"`
	DispatchMode           string             `json:"dispatch_mode"`
	DispatchPaused         bool               `json:"dispatch_paused"`
	SlotOccupied           bool               `json:"slot_occupied"`
	TaskID                 string             `json:"task_id"`
	DecisionID             string             `json:"decision_id"`
	DecisionStatus         string             `json:"decision_status"`
	ExecutionStatus        string             `json:"execution_status"`
	DeliveryStatus         string             `json:"delivery_status"`
	Backend                string             `json:"backend"`
	Protocol               string             `json:"protocol"`
	Target                 TargetSummary      `json:"target"`
	ProcessCheck           string             `json:"process_check"`
	Workspace              *WorkspaceSummary  `json:"workspace"`
	WorkspaceError         string             `json:"workspace_error"`
	NativeClaim            *TaskSummary       `json:"native_claim"`
	ClaimError             string             `json:"claim_error"`
	PR                     *RecoveryPRSummary `json:"pr"`
	PRError                string             `json:"pr_error"`
	Blockers               []ControlBlocker   `json:"blockers"`
	ConcurrentActionReason string             `json:"concurrent_action_reason"`
}
type WorkspaceSummary struct {
	Worktree  string    `json:"worktree"`
	Base      string    `json:"base"`
	Head      string    `json:"head"`
	Dirty     bool      `json:"dirty"`
	CheckedAt time.Time `json:"checked_at"`
}
type RecoveryPRSummary struct{ URL, State, Repository, BaseBranch string }

func CloneProjectRef(ref ProjectRef) ProjectRef {
	ref.Presentation.Profiles = append([]ExecutionProfileSummary(nil), ref.Presentation.Profiles...)
	for i := range ref.Presentation.Profiles {
		p := &ref.Presentation.Profiles[i]
		p.ImplementationDisplay = append([]string(nil), p.ImplementationDisplay...)
		p.Details = append([]string(nil), p.Details...)
	}
	return ref
}
