package reconstruction

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type Repository struct {
	Path     string `json:"path"`
	Remote   string `json:"remote"`
	Branch   string `json:"branch"`
	Identity string `json:"identity"`
}

// Runtime combines a sampled service checkpoint, task graph and public Runner facts.
type Runtime struct {
	SchemaVersion int                                 `json:"schema_version"`
	Revision      uint64                              `json:"revision"`
	UpdatedAt     time.Time                           `json:"updated_at"`
	HeartbeatAt   time.Time                           `json:"heartbeat_at"`
	Service       ServiceState                        `json:"service"`
	Repository    Repository                          `json:"repository"`
	Tasks         []taskmodel.TaskRecord              `json:"tasks,omitempty"`
	Deliveries    []executionmodel.TaskDeliveryRecord `json:"-"`
	CurrentTask   *string                             `json:"current_task"`
	SlotOccupied  bool                                `json:"slot_occupied"`
	Runs          []*RunView                          `json:"runs"`
	Control       ControlState                        `json:"control"`
}

type ServiceState struct {
	RunnerBuild         *runnercontract.BuildPin          `json:"runner_build,omitempty"`
	Mode                string                            `json:"mode"`
	InstanceID          string                            `json:"instance_id"`
	PID                 int                               `json:"pid"`
	Status              string                            `json:"status"`
	Claimant            string                            `json:"claimant"`
	Error               *string                           `json:"error"`
	ProcessIdentity     *executionbackend.ProcessIdentity `json:"process_identity"`
	StartupID           *string                           `json:"startup_id"`
	ConfigDigest        string                            `json:"config_digest"`
	Ready               bool                              `json:"ready"`
	ControlProtocol     int                               `json:"control_protocol"`
	Capabilities        []string                          `json:"capabilities"`
	HeartbeatIntervalMS int64                             `json:"heartbeat_interval_ms"`
}

type ControlState struct {
	DispatchMode   string            `json:"dispatch_mode"`
	DispatchPaused bool              `json:"dispatch_paused"`
	Revision       uint64            `json:"revision"`
	Operation      *ControlOperation `json:"operation"`
	Blockers       []ControlBlocker  `json:"blockers"`
}

type ControlOperation struct {
	RequestID     string                         `json:"request_id"`
	Action        string                         `json:"action"`
	TaskID        string                         `json:"task_id,omitempty"`
	Payload       json.RawMessage                `json:"payload,omitempty"`
	Phase         string                         `json:"phase"`
	RunID         string                         `json:"run_id,omitempty"`
	AttemptID     string                         `json:"attempt_id,omitempty"`
	DecisionID    string                         `json:"decision_id,omitempty"`
	RequestDigest string                         `json:"request_digest"`
	Result        *executionmodel.ControlReceipt `json:"result,omitempty"`
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

// RunView projects caller participation and the last public Runner observation.
type RunView struct {
	RunnerRevision           uint64 `json:"runner_revision"`
	InputDigest              string `json:"input_digest"`
	RunnerWorkflow           *runnercontract.Workflow
	Audit                    *runnercontract.Audit
	Materials                []runnercontract.Material
	RequestedModel           string
	RequestedReasoningEffort string

	Participation    string                       `json:"participation"`
	SubmissionStatus string                       `json:"submission_status"`
	StopRequested    bool                         `json:"stop_requested"`
	FlowRef          string                       `json:"flow_ref"`
	WorkflowStage    string                       `json:"workflow_stage"`
	WorkflowStatus   string                       `json:"workflow_status"`
	StageDiagnostic  string                       `json:"stage_diagnostic,omitempty"`
	StageResults     []executionmodel.StageResult `json:"stage_results,omitempty"`
	ReportToolPolicy string                       `json:"report_tool_policy,omitempty"`

	SessionID           string                                `json:"session_id,omitempty"`
	SessionGeneration   string                                `json:"session_generation,omitempty"`
	TurnNumber          uint64                                `json:"turn_number"`
	CurrentTurnID       string                                `json:"current_turn_id,omitempty"`
	ExecutionProfile    *executionmodel.FixedExecutionProfile `json:"execution_profile,omitempty"`
	PRHistory           []string                              `json:"pr_history,omitempty"`
	CandidateNumber     int                                   `json:"candidate_number,omitempty"`
	CandidateCreatedAt  time.Time                             `json:"candidate_created_at"`
	CandidateStopReason string                                `json:"candidate_stop_reason,omitempty"`
	CandidateStoppedBy  string                                `json:"candidate_stopped_by,omitempty"`
	CandidateEndedAt    *time.Time                            `json:"candidate_ended_at,omitempty"`
	CandidateTiming     string                                `json:"candidate_timing"`
	ResourceStatus      string                                `json:"resource_status"`
	EnvironmentBlocker  *executionmodel.WorkerHostBlocker     `json:"environment_blocker,omitempty"`
	RecoveryRequired    bool                                  `json:"recovery_required,omitempty"`
	RecoveryState       string                                `json:"recovery_state,omitempty"`
	RecoveryVersion     uint64                                `json:"recovery_version,omitempty"`
	TaskID              string                                `json:"task_id"`
	DomainTaskID        string                                `json:"domain_task_id,omitempty"`
	WorkflowVersion     string                                `json:"workflow_version,omitempty"`
	ExecutionID         string                                `json:"execution_id,omitempty"`
	Title               string                                `json:"title"`
	RunID               string                                `json:"run_id"`
	AttemptID           string                                `json:"attempt_id,omitempty"`
	ControlRevision     uint64                                `json:"control_revision,omitempty"`
	Claimant            string                                `json:"claimant"`
	Host                string                                `json:"host,omitempty"`
	Backend             string                                `json:"backend,omitempty"`
	Protocol            string                                `json:"protocol,omitempty"`
	Agent               string                                `json:"agent,omitempty"`
	Target              executionbackend.ExecutionTarget      `json:"target,omitempty"`
	TaskStatus          string                                `json:"task_status,omitempty"`
	ExecutionStatus     string                                `json:"execution_status"`
	Process             *executionbackend.ProcessIdentity     `json:"process"`
	StartedAt           *time.Time                            `json:"started_at"`
	EndedAt             *time.Time                            `json:"ended_at"`
	WorktreePath        *string                               `json:"worktree_path"`
	WorktreeBranch      *string                               `json:"worktree_branch"`
	BaseCommit          *string                               `json:"base_commit"`
	PR                  executionmodel.PRState                `json:"pr"`
	LastEvent           *string                               `json:"last_event"`
	LogPath             *string                               `json:"log_path"`
	ResultPath          *string                               `json:"result_path"`
	Summary             *string                               `json:"summary"`
	Error               *string                               `json:"error"`
	Decision            *executionmodel.DecisionState         `json:"decision,omitempty"`
}
