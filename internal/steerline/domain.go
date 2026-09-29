package steerline

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Domain entities are the durable center model. Runtime/Run below remains a
// read model for the existing CLI and console.
// WorkerFlowVersion is the persisted protocol for a single resumable worker session.
const WorkerFlowVersion = "worker-flow-v1"

type TaskRecord struct {
	ProjectIdentity   string     `json:"project_identity"`
	TaskID            string     `json:"task_id"`
	ExternalProvider  string     `json:"external_provider,omitempty"`
	ExternalProject   string     `json:"external_project,omitempty"`
	ExternalID        string     `json:"external_id,omitempty"`
	IdentityMapDigest string     `json:"identity_map_digest,omitempty"`
	BatchID           string     `json:"batch_id,omitempty"`
	Title             string     `json:"title"`
	Type              string     `json:"type,omitempty"`
	Priority          int        `json:"priority,omitempty"`
	GoalIDs           []string   `json:"goal_ids,omitempty"`
	ContractID        string     `json:"contract_id,omitempty"`
	Claimant          string     `json:"claimant,omitempty"`
	Source            string     `json:"source"`
	Status            string     `json:"status"`
	Version           uint64     `json:"version"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DiscardedAt       *time.Time `json:"discarded_at,omitempty"`
	DiscardReason     string     `json:"discard_reason,omitempty"`
	DiscardedBy       string     `json:"discarded_by,omitempty"`
	DiscardRequestID  string     `json:"discard_request_id,omitempty"`
	CleanupPending    []string   `json:"cleanup_pending,omitempty"`
}

// FileReference pins a contract document to an immutable commit and the
// SHA-256 of the committed blob bytes.
type FileReference struct {
	Path   string `json:"path"`
	Commit string `json:"commit"`
	SHA256 string `json:"sha256"`
}

type TaskContract struct {
	ContractID     string          `json:"contract_id"`
	Repository     string          `json:"repository"`
	Remote         string          `json:"remote"`
	Branch         string          `json:"branch"`
	SpecRef        FileReference   `json:"spec_ref"`
	PlanRef        FileReference   `json:"plan_ref"`
	CommonRefs     []FileReference `json:"common_refs"`
	AcceptanceRefs []FileReference `json:"acceptance_refs"`
	Digest         string          `json:"digest"`
}

type ProjectTaskAuthority struct {
	ProjectIdentity  string    `json:"project_id"`
	Authority        string    `json:"authority"`
	AuthorityVersion uint64    `json:"authority_version"`
	GraphRevision    uint64    `json:"graph_revision"`
	Repository       string    `json:"repository"`
	Remote           string    `json:"remote"`
	Branch           string    `json:"branch"`
	DispatchMode     string    `json:"dispatch_mode"`
	DispatchPaused   bool      `json:"dispatch_paused"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type TaskBatchRecord struct {
	BatchID         string            `json:"batch_id"`
	ProjectIdentity string            `json:"project_id"`
	SchemaVersion   string            `json:"schema_version"`
	PayloadDigest   string            `json:"payload_digest"`
	CreatedAt       time.Time         `json:"created_at"`
	CommandID       string            `json:"command_id"`
	TaskIDs         map[string]string `json:"task_ids"`
}

type TaskDependencyRecord struct {
	ProjectIdentity string `json:"project_id"`
	PredecessorID   string `json:"predecessor_id"`
	SuccessorID     string `json:"successor_id"`
}

type TaskDeliveryRecord struct {
	ProjectIdentity       string          `json:"project_id"`
	TaskID                string          `json:"task_id"`
	SelectedWorkflowRunID string          `json:"selected_workflow_run_id"`
	PRInstance            string          `json:"pr_instance"`
	PRRepository          string          `json:"pr_repository"`
	PRNumber              int64           `json:"pr_number"`
	PRURL                 string          `json:"pr_url"`
	MergeCommit           string          `json:"merge_commit"`
	MergedAt              time.Time       `json:"merged_at"`
	RecordedAt            time.Time       `json:"recorded_at"`
	Evidence              json.RawMessage `json:"evidence,omitempty"`
}

type TaskDeliveryEvidence struct {
	PRInstance         string           `json:"pr_instance"`
	URL                string           `json:"url"`
	Repository         string           `json:"repository"`
	PRNumber           int64            `json:"number"`
	BaseBranch         string           `json:"base_branch"`
	State              string           `json:"state"`
	Merged             bool             `json:"merged"`
	MergedAt           time.Time        `json:"merged_at"`
	MergeCommit        string           `json:"merge_commit"`
	ExistingAcceptance *json.RawMessage `json:"existing_acceptance,omitempty"`
}

type WorkflowRunRecord struct {
	ProjectIdentity  string                 `json:"project_identity"`
	WorkflowRunID    string                 `json:"workflow_run_id"`
	TaskID           string                 `json:"task_id"`
	WorkflowVersion  string                 `json:"workflow_version"`
	ConfigDigest     string                 `json:"config_digest,omitempty"`
	ExecutionProfile *FixedExecutionProfile `json:"execution_profile,omitempty"`
	Runner           StepRunnerSettings     `json:"runner"`
	Phase            string                 `json:"phase"`
	Repository       string                 `json:"repository,omitempty"`
	Branch           string                 `json:"branch,omitempty"`
	SelectedResult   string                 `json:"selected_result_id,omitempty"`
	PRURL            string                 `json:"pr_url,omitempty"`
	PRCandidateURL   string                 `json:"pr_candidate_url,omitempty"`
	PRState          string                 `json:"pr_state,omitempty"`
	PendingDecisions int                    `json:"pending_decisions,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type StepRunnerSettings struct {
	Codex           Command `json:"codex"`
	Sandbox         string  `json:"sandbox"`
	Model           string  `json:"model,omitempty"`
	ReasoningEffort string  `json:"reasoning_effort,omitempty"`
	Profile         string  `json:"profile,omitempty"`
}

type ExecutionRecord struct {
	ProjectIdentity       string               `json:"project_identity"`
	ExecutionID           string               `json:"execution_id"`
	WorkflowRunID         string               `json:"workflow_run_id"`
	Agent                 string               `json:"agent,omitempty"`
	Backend               string               `json:"backend,omitempty"`
	Protocol              string               `json:"protocol,omitempty"`
	Process               *ProcessIdentity     `json:"process,omitempty"`
	OrcaNative            *OrcaNativeExecution `json:"orca_native,omitempty"`
	SessionID             string               `json:"session_id,omitempty"`
	AppServerEndpoint     string               `json:"app_server_endpoint,omitempty"`
	AppServerGeneration   string               `json:"app_server_generation,omitempty"`
	PendingCommandID      string               `json:"pending_command_id,omitempty"`
	CurrentCommandID      string               `json:"current_command_id,omitempty"`
	PendingAsks           []codexAskCall       `json:"pending_asks,omitempty"`
	PendingTurnID         string               `json:"pending_turn_id,omitempty"`
	CurrentTurnID         string               `json:"current_turn_id,omitempty"`
	TurnNumber            uint64               `json:"turn_number,omitempty"`
	LastStoppedTurnID     string               `json:"last_stopped_turn_id,omitempty"`
	Target                ExecutionTarget      `json:"target"`
	Workspace             WorkspaceEvidence    `json:"workspace"`
	Status                string               `json:"status"`
	CurrentStep           string               `json:"current_step,omitempty"`
	CandidateRevision     string               `json:"candidate_revision,omitempty"`
	VerifiedRevision      string               `json:"verified_revision,omitempty"`
	ReviewedRevision      string               `json:"reviewed_revision,omitempty"`
	RepairPrerequisiteIDs []string             `json:"repair_prerequisite_result_ids,omitempty"`
	RepairRounds          int                  `json:"repair_rounds"`
	CreatedAt             time.Time            `json:"created_at"`
	UpdatedAt             time.Time            `json:"updated_at"`
}

// OrcaNativeExecution stores provider identities and observation positions.
// WorkflowRun, Decision and Operation remain the authorities for business state.
type OrcaNativeExecution struct {
	RepositoryID string           `json:"repository_id"`
	Coordinator  ProcessIdentity  `json:"coordinator"`
	Turns        []OrcaNativeTurn `json:"turns,omitempty"`
	DeliveryID   string           `json:"delivery_id,omitempty"`
	SeenMessages []string         `json:"seen_messages,omitempty"`
}

type OrcaNativeTurn struct {
	TurnID               string   `json:"turn_id"`
	TaskID               string   `json:"task_id"`
	DispatchID           string   `json:"dispatch_id"`
	TaskRequestID        string   `json:"task_request_id,omitempty"`
	DispatchRequestID    string   `json:"dispatch_request_id,omitempty"`
	DoneMessageID        string   `json:"done_message_id,omitempty"`
	Outcome              string   `json:"outcome,omitempty"`
	OutputSource         string   `json:"output_source,omitempty"`
	OutputCursor         string   `json:"output_cursor,omitempty"`
	OutputComplete       bool     `json:"output_complete,omitempty"`
	OutputClipping       []string `json:"output_clipping,omitempty"`
	OutputFallbackReason string   `json:"output_fallback_reason,omitempty"`
}

type ExecutionTarget struct {
	ID                        string `json:"id,omitempty"`
	ConnectionID              string `json:"connection_id,omitempty"`
	WorktreeID                string `json:"worktree_id,omitempty"`
	Branch                    string `json:"branch,omitempty"`
	BaseRevision              string `json:"base_revision,omitempty"`
	ExternalRuntime           string `json:"external_runtime,omitempty"`
	OrchestrationRun          string `json:"orchestration_run,omitempty"`
	OrchestrationRunRequestID string `json:"orchestration_run_request_id,omitempty"`
	CoordinatorTerminal       string `json:"coordinator_terminal,omitempty"`
	ExternalTask              string `json:"external_task,omitempty"`
	Dispatch                  string `json:"dispatch,omitempty"`
	ExecutionHost             string `json:"execution_host,omitempty"`
}

type WorkspaceEvidence struct {
	Repository string    `json:"repository,omitempty"`
	Worktree   string    `json:"worktree,omitempty"`
	Branch     string    `json:"branch,omitempty"`
	Base       string    `json:"base,omitempty"`
	Head       string    `json:"head,omitempty"`
	Dirty      bool      `json:"dirty,omitempty"`
	VerifiedAt time.Time `json:"verified_at,omitempty"`
}

type StepAttemptRecord struct {
	ProjectIdentity      string          `json:"project_identity"`
	AttemptID            string          `json:"attempt_id"`
	ExecutionID          string          `json:"execution_id"`
	StepID               string          `json:"step_id"`
	AttemptNumber        int             `json:"attempt_number"`
	OperationID          string          `json:"operation_id,omitempty"`
	EventPath            string          `json:"event_path,omitempty"`
	ResultPath           string          `json:"result_path,omitempty"`
	OutputSource         string          `json:"output_source,omitempty"`
	OutputCursor         string          `json:"output_cursor,omitempty"`
	Status               string          `json:"status"`
	InputRevision        string          `json:"input_revision,omitempty"`
	ContractDigest       string          `json:"contract_digest,omitempty"`
	PrerequisiteIDs      []string        `json:"prerequisite_result_ids,omitempty"`
	ResultID             string          `json:"result_id,omitempty"`
	Result               json.RawMessage `json:"result,omitempty"`
	ExternalBinding      ExecutionTarget `json:"external_binding,omitempty"`
	NativeLauncherPath   string          `json:"native_launcher_path,omitempty"`
	NativeLauncherDigest string          `json:"native_launcher_digest,omitempty"`
	Legacy               bool            `json:"legacy,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

type DecisionRecord struct {
	ProjectIdentity string    `json:"project_identity"`
	DecisionID      string    `json:"decision_id"`
	Scope           string    `json:"scope"`
	WorkflowRunID   string    `json:"workflow_run_id,omitempty"`
	ExecutionID     string    `json:"execution_id,omitempty"`
	AttemptID       string    `json:"attempt_id,omitempty"`
	QuestionID      string    `json:"question_id"`
	Request         []byte    `json:"request,omitempty"`
	Answer          []byte    `json:"answer,omitempty"`
	AnsweredBy      string    `json:"answered_by,omitempty"`
	Status          string    `json:"status"`
	Version         uint64    `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type DomainEvent struct {
	ProjectIdentity string    `json:"project_identity"`
	EventID         int64     `json:"event_id,omitempty"`
	TaskID          string    `json:"task_id,omitempty"`
	WorkflowRunID   string    `json:"workflow_run_id,omitempty"`
	ExecutionID     string    `json:"execution_id,omitempty"`
	AttemptID       string    `json:"attempt_id,omitempty"`
	ExternalSource  string    `json:"external_source,omitempty"`
	ExternalID      string    `json:"external_id,omitempty"`
	Type            string    `json:"type"`
	ReceivedAt      time.Time `json:"received_at"`
	Payload         []byte    `json:"payload,omitempty"`
	PayloadBytes    int       `json:"payload_bytes,omitempty"`
	PayloadDigest   string    `json:"payload_digest,omitempty"`
	PayloadOmitted  bool      `json:"payload_omitted,omitempty"`
}

type OperationRecord struct {
	ProjectIdentity   string    `json:"project_identity"`
	OperationID       string    `json:"operation_id"`
	IdempotencyKey    string    `json:"idempotency_key"`
	Kind              string    `json:"kind"`
	Status            string    `json:"status"`
	PayloadDigest     string    `json:"payload_digest"`
	Request           []byte    `json:"request,omitempty"`
	PlatformRequestID string    `json:"platform_request_id,omitempty"`
	Receipt           []byte    `json:"receipt,omitempty"`
	UnknownReason     string    `json:"unknown_reason,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type CommandRecord struct {
	ProjectIdentity string    `json:"project_identity"`
	CommandID       string    `json:"command_id"`
	Kind            string    `json:"kind"`
	TargetID        string    `json:"target_id"`
	ExpectedVersion uint64    `json:"expected_version"`
	IdempotencyKey  string    `json:"idempotency_key"`
	RequestedBy     string    `json:"requested_by"`
	Payload         []byte    `json:"payload"`
	Status          string    `json:"status"`
	Result          []byte    `json:"result,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

var revisionPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func validStepAttemptStatus(v string) bool {
	switch v {
	case "planned", "dispatching", "dispatched", "running", "waiting_decision", "succeeded", "failed", "rejected", "stopping", "stopped", "outcome_unknown", "abandoned":
		return true
	}
	return false
}

func validTaskStatus(v string) bool {
	switch v {
	case "open", "in_progress", "discarded", "closed":
		return true
	}
	return false
}
func validWorkflowPhase(v string) bool {
	switch v {
	case "queued", "executing", "waiting_decision", "waiting_pr_merge", "completing", "completed", "blocked", "abandoned":
		return true
	}
	return false
}
func validExecutionStatus(v string) bool {
	switch v {
	case "pending", "active", "idle", "waiting_decision", "succeeded", "failed", "blocked", "stopped", "abandoned":
		return true
	}
	return false
}
func validDecisionStatus(v string) bool {
	switch v {
	case "pending", "answer_saved", "resuming", "reply_accepted", "applied", "failed", "superseded":
		return true
	}
	return false
}
func validOperationStatus(v string) bool {
	switch v {
	case "pending", "in_flight", "accepted", "confirmed", "rejected", "outcome_unknown":
		return true
	}
	return false
}

func SetTaskStatus(task *TaskRecord, next string, mergedPR bool) error {
	if task == nil {
		return errors.New("task is required")
	}
	if !validTaskStatus(next) {
		return fmt.Errorf("invalid task status %q", next)
	}
	allowed := map[string]map[string]bool{"open": {"in_progress": true, "discarded": true}, "in_progress": {"discarded": true, "closed": true}, "discarded": {}, "closed": {}}
	if task.Status == next {
		return nil
	}
	if !allowed[task.Status][next] {
		return fmt.Errorf("task transition %s -> %s is not allowed", task.Status, next)
	}
	if next == "discarded" && mergedPR {
		return errors.New("task with a merged pull request cannot be discarded")
	}
	task.Status = next
	task.Version++
	task.UpdatedAt = now()
	return nil
}
