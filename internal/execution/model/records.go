package model

import (
	"encoding/json"
	"errors"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type WorkflowRunRecord struct {
	CandidateStoppedAt  *time.Time             `json:"candidate_stopped_at,omitempty"`
	CandidateStopReason string                 `json:"candidate_stop_reason,omitempty"`
	CandidateStoppedBy  string                 `json:"candidate_stopped_by,omitempty"`
	StartPending        bool                   `json:"start_pending,omitempty"`
	CandidateNumber     int                    `json:"candidate_number,omitempty"`
	ControlRevision     uint64                 `json:"-"`
	PRHistory           []string               `json:"pr_history,omitempty"`
	ProjectIdentity     string                 `json:"project_identity"`
	WorkflowRunID       string                 `json:"workflow_run_id"`
	TaskID              string                 `json:"task_id"`
	WorkflowVersion     string                 `json:"workflow_version"`
	ConfigDigest        string                 `json:"config_digest,omitempty"`
	ExecutionProfile    *FixedExecutionProfile `json:"execution_profile,omitempty"`
	Status              string                 `json:"status"`
	FlowRef             string                 `json:"flow_ref"`
	StepID              string                 `json:"step_id"`
	InputVersion        uint64                 `json:"input_version"`
	StageInputs         []StageInput           `json:"stage_inputs,omitempty"`
	StageResults        []StageResult          `json:"stage_results,omitempty"`
	ReportRetry         uint8                  `json:"report_retry,omitempty"`
	StageDiagnostic     string                 `json:"stage_diagnostic,omitempty"`
	Repository          string                 `json:"repository,omitempty"`
	Branch              string                 `json:"branch,omitempty"`
	SelectedResult      string                 `json:"selected_result_id,omitempty"`
	PRURL               string                 `json:"pr_url,omitempty"`
	PRCandidateURL      string                 `json:"pr_candidate_url,omitempty"`
	PRState             string                 `json:"pr_state,omitempty"`
	PRQueriedAt         *time.Time             `json:"pr_queried_at,omitempty"`
	PRQueryError        *string                `json:"pr_query_error,omitempty"`
	PRMergedAt          *time.Time             `json:"pr_merged_at,omitempty"`
	PRMergeCommit       *string                `json:"pr_merge_commit,omitempty"`
	PendingDecisions    int                    `json:"pending_decisions,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

type ExecutionRecord struct {
	ResourceStopOperationID string     `json:"resource_stop_operation_id,omitempty"`
	ResourceStoppedAt       *time.Time `json:"resource_stopped_at,omitempty"`

	ImplementationData       []executionbackend.ImplementationDataEnvelope `json:"implementation_data,omitempty"`
	PolicyResources          *PolicyResources                              `json:"policy_resources,omitempty"`
	RecoveryRequired         bool                                          `json:"recovery_required,omitempty"`
	RecoveryVersion          uint64                                        `json:"recovery_version,omitempty"`
	RecoveryState            string                                        `json:"recovery_state,omitempty"`
	RecoverySessionAvailable *bool                                         `json:"recovery_session_available,omitempty"`
	RecoveryMarkedAt         *time.Time                                    `json:"recovery_marked_at,omitempty"`
	WorkspaceIntent          *executionbackend.WorkspaceIntent             `json:"workspace_intent,omitempty"`
	TurnStartedAt            *time.Time                                    `json:"turn_started_at,omitempty"`
	TurnEndedAt              *time.Time                                    `json:"turn_ended_at,omitempty"`
	Generation               string                                        `json:"generation,omitempty"`
	SessionID                string                                        `json:"session_id,omitempty"`
	SessionGeneration        string                                        `json:"session_generation,omitempty"`
	PendingDeliveryID        string                                        `json:"pending_delivery_id,omitempty"`
	ConfigurationDigest      string                                        `json:"configuration_digest,omitempty"`
	LastTurnOutcome          string                                        `json:"last_turn_outcome,omitempty"`
	LogPath                  *string                                       `json:"log_path,omitempty"`
	LastEvent                *string                                       `json:"last_event,omitempty"`
	Summary                  *string                                       `json:"summary,omitempty"`
	Error                    *string                                       `json:"error,omitempty"`
	ManualStop               *ManualStopConfirmation                       `json:"manual_stop,omitempty"`
	ProjectIdentity          string                                        `json:"project_identity"`
	ExecutionID              string                                        `json:"execution_id"`
	CurrentAttemptID         string                                        `json:"current_attempt_id,omitempty"`
	OwnerServiceInstanceID   string                                        `json:"owner_service_instance_id,omitempty"`
	WorkflowRunID            string                                        `json:"workflow_run_id"`
	Agent                    string                                        `json:"agent,omitempty"`
	Backend                  string                                        `json:"backend,omitempty"`
	Protocol                 string                                        `json:"protocol,omitempty"`
	Process                  *executionbackend.ProcessIdentity             `json:"process,omitempty"`
	Environment              *WorkerEnvironment                            `json:"environment,omitempty"`
	DecisionID               string                                        `json:"decision_id,omitempty"`
	PendingCommandID         string                                        `json:"pending_command_id,omitempty"`
	CurrentCommandID         string                                        `json:"current_command_id,omitempty"`
	PendingQuestions         []PendingQuestion                             `json:"pending_questions,omitempty"`
	PendingTurnID            string                                        `json:"pending_turn_id,omitempty"`
	CurrentTurnID            string                                        `json:"current_turn_id,omitempty"`
	TurnNumber               uint64                                        `json:"turn_number,omitempty"`
	LastStoppedTurnID        string                                        `json:"last_stopped_turn_id,omitempty"`
	Target                   executionbackend.ExecutionTarget              `json:"target"`
	Workspace                WorkspaceEvidence                             `json:"workspace"`
	Status                   string                                        `json:"status"`
	CurrentStep              string                                        `json:"current_step,omitempty"`
	CreatedAt                time.Time                                     `json:"created_at"`
	UpdatedAt                time.Time                                     `json:"updated_at"`
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
	if err := decodeJSON(data, &decoded); err != nil {
		return err
	}
	*e = ExecutionRecord(decoded)
	return nil
}

type WorkspaceEvidence struct {
	InspectionScope string    `json:"inspection_scope,omitempty"`
	Repository      string    `json:"repository,omitempty"`
	Worktree        string    `json:"worktree,omitempty"`
	Branch          string    `json:"branch,omitempty"`
	Base            string    `json:"base,omitempty"`
	Head            string    `json:"head,omitempty"`
	Dirty           bool      `json:"dirty,omitempty"`
	VerifiedAt      time.Time `json:"verified_at,omitempty"`
}

type StepAttemptRecord struct {
	WorkerProcess        *executionbackend.ProcessIdentity `json:"worker_process,omitempty"`
	ProjectIdentity      string                            `json:"project_identity"`
	AttemptID            string                            `json:"attempt_id"`
	ExecutionID          string                            `json:"execution_id"`
	StepID               string                            `json:"step_id"`
	AttemptNumber        int                               `json:"attempt_number"`
	OperationID          string                            `json:"operation_id,omitempty"`
	EventPath            string                            `json:"event_path,omitempty"`
	ResultPath           string                            `json:"result_path,omitempty"`
	OutputSource         string                            `json:"output_source,omitempty"`
	OutputCursor         string                            `json:"output_cursor,omitempty"`
	Status               string                            `json:"status"`
	InputRevision        string                            `json:"input_revision,omitempty"`
	ContractDigest       string                            `json:"contract_digest,omitempty"`
	Result               json.RawMessage                   `json:"result,omitempty"`
	ExternalBinding      executionbackend.ExecutionTarget  `json:"external_binding,omitempty"`
	NativeLauncherPath   string                            `json:"native_launcher_path,omitempty"`
	NativeLauncherDigest string                            `json:"native_launcher_digest,omitempty"`
	CreatedAt            time.Time                         `json:"created_at"`
	UpdatedAt            time.Time                         `json:"updated_at"`
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

type OperationRecord struct {
	ProjectIdentity   string                                       `json:"project_identity"`
	OperationID       string                                       `json:"operation_id"`
	IdempotencyKey    string                                       `json:"idempotency_key"`
	Kind              string                                       `json:"kind"`
	Status            string                                       `json:"status"`
	PayloadDigest     string                                       `json:"payload_digest"`
	Request           []byte                                       `json:"request,omitempty"`
	LifecycleIdentity *executionbackend.LifecycleOperationIdentity `json:"lifecycle_identity,omitempty"`
	PlatformRequestID string                                       `json:"platform_request_id,omitempty"`
	Receipt           []byte                                       `json:"receipt,omitempty"`
	UnknownReason     string                                       `json:"unknown_reason,omitempty"`
	CreatedAt         time.Time                                    `json:"created_at"`
	UpdatedAt         time.Time                                    `json:"updated_at"`
}
