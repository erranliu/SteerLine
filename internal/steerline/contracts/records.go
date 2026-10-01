package contracts

import (
	"encoding/json"
	"errors"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/task"
)

type TaskRecord = task.TaskRecord
type FileReference = task.FileReference
type TaskContract = task.TaskContract
type ProjectTaskAuthority = task.ProjectTaskAuthority
type TaskBatchRecord = task.TaskBatchRecord
type TaskDependencyRecord = task.TaskDependencyRecord
type TaskDeliveryRecord = task.TaskDeliveryRecord
type TaskDeliveryEvidence = task.TaskDeliveryEvidence

type WorkflowRunRecord struct {
	ControlRevision  uint64                 `json:"-"`
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
	PRQueriedAt      *time.Time             `json:"pr_queried_at,omitempty"`
	PRQueryError     *string                `json:"pr_query_error,omitempty"`
	PRMergedAt       *time.Time             `json:"pr_merged_at,omitempty"`
	PRMergeCommit    *string                `json:"pr_merge_commit,omitempty"`
	PendingDecisions int                    `json:"pending_decisions,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type StepRunnerSettings struct {
	Codex           Command `json:"codex"`
	Sandbox         string  `json:"sandbox"`
	WindowsSandbox  string  `json:"windows_sandbox,omitempty"`
	Model           string  `json:"model,omitempty"`
	ReasoningEffort string  `json:"reasoning_effort,omitempty"`
	Profile         string  `json:"profile,omitempty"`
}

type ExecutionRecord struct {
	CurrentRequirementsError      *string                              `json:"current_requirements_error,omitempty"`
	CurrentRequirementsEvaluation *executionapi.RequirementsEvaluation `json:"current_requirements_evaluation,omitempty"`
	PolicyResources               *PolicyResources                     `json:"policy_resources,omitempty"`
	RecoveryRequired              bool                                 `json:"recovery_required,omitempty"`
	RecoveryVersion               uint64                               `json:"recovery_version,omitempty"`
	RecoveryState                 string                               `json:"recovery_state,omitempty"`
	RecoveryMarkedAt              *time.Time                           `json:"recovery_marked_at,omitempty"`
	WorkspaceIntent               *executionapi.WorkspaceIntent        `json:"workspace_intent,omitempty"`
	TurnStartedAt                 *time.Time                           `json:"turn_started_at,omitempty"`
	TurnEndedAt                   *time.Time                           `json:"turn_ended_at,omitempty"`
	LastTurnOutcome               string                               `json:"last_turn_outcome,omitempty"`
	LogPath                       *string                              `json:"log_path,omitempty"`
	LastEvent                     *string                              `json:"last_event,omitempty"`
	Summary                       *string                              `json:"summary,omitempty"`
	Error                         *string                              `json:"error,omitempty"`
	ManualStop                    *ManualStopConfirmation              `json:"manual_stop,omitempty"`
	ProjectIdentity               string                               `json:"project_identity"`
	ExecutionID                   string                               `json:"execution_id"`
	CurrentAttemptID              string                               `json:"current_attempt_id,omitempty"`
	OwnerServiceInstanceID        string                               `json:"owner_service_instance_id,omitempty"`
	WorkflowRunID                 string                               `json:"workflow_run_id"`
	Agent                         string                               `json:"agent,omitempty"`
	Backend                       string                               `json:"backend,omitempty"`
	Protocol                      string                               `json:"protocol,omitempty"`
	Process                       *executionapi.ProcessIdentity        `json:"process,omitempty"`
	OrcaNative                    *OrcaNativeExecution                 `json:"orca_native,omitempty"`
	Environment                   *WorkerEnvironment                   `json:"environment,omitempty"`
	SessionID                     string                               `json:"session_id,omitempty"`
	DecisionID                    string                               `json:"decision_id,omitempty"`
	AppServerEndpoint             string                               `json:"app_server_endpoint,omitempty"`
	AppServerGeneration           string                               `json:"app_server_generation,omitempty"`
	PendingCommandID              string                               `json:"pending_command_id,omitempty"`
	CurrentCommandID              string                               `json:"current_command_id,omitempty"`
	PendingQuestions              []PendingQuestion                    `json:"pending_questions,omitempty"`
	PendingTurnID                 string                               `json:"pending_turn_id,omitempty"`
	CurrentTurnID                 string                               `json:"current_turn_id,omitempty"`
	TurnNumber                    uint64                               `json:"turn_number,omitempty"`
	LastStoppedTurnID             string                               `json:"last_stopped_turn_id,omitempty"`
	Target                        executionapi.ExecutionTarget         `json:"target"`
	Workspace                     WorkspaceEvidence                    `json:"workspace"`
	Status                        string                               `json:"status"`
	CurrentStep                   string                               `json:"current_step,omitempty"`
	CreatedAt                     time.Time                            `json:"created_at"`
	UpdatedAt                     time.Time                            `json:"updated_at"`
}

func (e *ExecutionRecord) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, retired := fields["pending_asks"]; retired {
		return errors.New("execution record contains retired native question data; preserve it for manual recovery")
	}
	type executionRecordJSON ExecutionRecord
	var decoded executionRecordJSON
	if err := DecodeJSON(data, &decoded); err != nil {
		return err
	}
	*e = ExecutionRecord(decoded)
	return nil
}

// OrcaNativeExecution stores provider identities and observation positions.
// WorkflowRun, Decision and Operation remain the authorities for business state.
type OrcaNativeExecution struct {
	RemoteRuntimeID string                       `json:"remote_runtime_id,omitempty"`
	RepositoryID    string                       `json:"repository_id"`
	Coordinator     executionapi.ProcessIdentity `json:"coordinator"`
	Turns           []OrcaNativeTurn             `json:"turns,omitempty"`
	DeliveryID      string                       `json:"delivery_id,omitempty"`
	SeenMessages    []string                     `json:"seen_messages,omitempty"`
}

type OrcaNativeTurn struct {
	ToolContext          ToolContext `json:"tool_context,omitempty"`
	InjectionAttempts    int         `json:"injection_attempts,omitempty"`
	Purpose              string      `json:"purpose,omitempty"`
	IntentAt             time.Time   `json:"intent_at"`
	Settled              bool        `json:"settled,omitempty"`
	TurnID               string      `json:"turn_id"`
	TaskID               string      `json:"task_id"`
	DispatchID           string      `json:"dispatch_id"`
	TaskRequestID        string      `json:"task_request_id,omitempty"`
	DispatchRequestID    string      `json:"dispatch_request_id,omitempty"`
	DoneMessageID        string      `json:"done_message_id,omitempty"`
	Outcome              string      `json:"outcome,omitempty"`
	OutputSource         string      `json:"output_source,omitempty"`
	OutputCursor         string      `json:"output_cursor,omitempty"`
	OutputComplete       bool        `json:"output_complete,omitempty"`
	OutputClipping       []string    `json:"output_clipping,omitempty"`
	OutputFallbackReason string      `json:"output_fallback_reason,omitempty"`
	OutputDigest         string      `json:"output_digest,omitempty"`
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
	WorkerProcess        *executionapi.ProcessIdentity `json:"worker_process,omitempty"`
	WorkerSessionID      string                        `json:"worker_session_id,omitempty"`
	ProjectIdentity      string                        `json:"project_identity"`
	AttemptID            string                        `json:"attempt_id"`
	ExecutionID          string                        `json:"execution_id"`
	StepID               string                        `json:"step_id"`
	AttemptNumber        int                           `json:"attempt_number"`
	OperationID          string                        `json:"operation_id,omitempty"`
	EventPath            string                        `json:"event_path,omitempty"`
	ResultPath           string                        `json:"result_path,omitempty"`
	OutputSource         string                        `json:"output_source,omitempty"`
	OutputCursor         string                        `json:"output_cursor,omitempty"`
	Status               string                        `json:"status"`
	InputRevision        string                        `json:"input_revision,omitempty"`
	ContractDigest       string                        `json:"contract_digest,omitempty"`
	Result               json.RawMessage               `json:"result,omitempty"`
	ExternalBinding      executionapi.ExecutionTarget  `json:"external_binding,omitempty"`
	NativeLauncherPath   string                        `json:"native_launcher_path,omitempty"`
	NativeLauncherDigest string                        `json:"native_launcher_digest,omitempty"`
	CreatedAt            time.Time                     `json:"created_at"`
	UpdatedAt            time.Time                     `json:"updated_at"`
}

type DecisionRecord struct {
	RecoveryVersion    uint64    `json:"recovery_version,omitempty"`
	RecoveryState      string    `json:"recovery_state,omitempty"`
	RecoveryOfDecision string    `json:"recovery_of_decision,omitempty"`
	ProjectIdentity    string    `json:"project_identity"`
	DecisionID         string    `json:"decision_id"`
	Scope              string    `json:"scope"`
	WorkflowRunID      string    `json:"workflow_run_id,omitempty"`
	ExecutionID        string    `json:"execution_id,omitempty"`
	AttemptID          string    `json:"attempt_id,omitempty"`
	QuestionID         string    `json:"question_id"`
	Request            []byte    `json:"request,omitempty"`
	Answer             []byte    `json:"answer,omitempty"`
	AnsweredBy         string    `json:"answered_by,omitempty"`
	Status             string    `json:"status"`
	Version            uint64    `json:"version"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
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
