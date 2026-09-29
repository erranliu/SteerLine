package steerline

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Domain entities are the durable center model. Runtime/Run below remains a
// compatibility read model for the existing CLI and viewer.
const MinimalWorkflowVersion = "minimal-v1"

// WorkerFlowVersion is the persisted protocol for a single resumable worker
// session. It is intentionally distinct from the unpublished minimal-v1 format.
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

type WorkflowDefinition struct {
	ID              string `json:"id"`
	Version         string `json:"version"`
	ReviewRequired  bool   `json:"review_required"`
	MaxRepairRounds int    `json:"max_repair_rounds"`
}

type WorkflowRunRecord struct {
	ProjectIdentity      string                 `json:"project_identity"`
	WorkflowRunID        string                 `json:"workflow_run_id"`
	TaskID               string                 `json:"task_id"`
	WorkflowVersion      string                 `json:"workflow_version"`
	ConfigDigest         string                 `json:"config_digest,omitempty"`
	ExecutionProfile     *FixedExecutionProfile `json:"execution_profile,omitempty"`
	Definition           WorkflowDefinition     `json:"workflow_definition"`
	ProjectSteps         ProjectSteps           `json:"project_steps,omitempty"`
	WorkspaceDirtyIgnore WorkspaceDirtyIgnore   `json:"workspace_dirty_ignore,omitempty"`
	Runner               StepRunnerSettings     `json:"runner"`
	Phase                string                 `json:"phase"`
	Repository           string                 `json:"repository,omitempty"`
	Branch               string                 `json:"branch,omitempty"`
	SelectedResult       string                 `json:"selected_result_id,omitempty"`
	PRURL                string                 `json:"pr_url,omitempty"`
	PRCandidateURL       string                 `json:"pr_candidate_url,omitempty"`
	PRState              string                 `json:"pr_state,omitempty"`
	PendingDecisions     int                    `json:"pending_decisions,omitempty"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
}

type StepRunnerSettings struct {
	Codex           Command `json:"codex"`
	Sandbox         string  `json:"sandbox"`
	Model           string  `json:"model,omitempty"`
	ReasoningEffort string  `json:"reasoning_effort,omitempty"`
	Profile         string  `json:"profile,omitempty"`
}

type ExecutionRecord struct {
	ProjectIdentity       string            `json:"project_identity"`
	ExecutionID           string            `json:"execution_id"`
	WorkflowRunID         string            `json:"workflow_run_id"`
	Agent                 string            `json:"agent,omitempty"`
	Backend               string            `json:"backend,omitempty"`
	Protocol              string            `json:"protocol,omitempty"`
	Process               *ProcessIdentity  `json:"process,omitempty"`
	SessionID             string            `json:"session_id,omitempty"`
	PendingTurnID         string            `json:"pending_turn_id,omitempty"`
	CurrentTurnID         string            `json:"current_turn_id,omitempty"`
	TurnNumber            uint64            `json:"turn_number,omitempty"`
	LastStoppedTurnID     string            `json:"last_stopped_turn_id,omitempty"`
	Target                ExecutionTarget   `json:"target"`
	Workspace             WorkspaceEvidence `json:"workspace"`
	Status                string            `json:"status"`
	CurrentStep           string            `json:"current_step,omitempty"`
	CandidateRevision     string            `json:"candidate_revision,omitempty"`
	VerifiedRevision      string            `json:"verified_revision,omitempty"`
	ReviewedRevision      string            `json:"reviewed_revision,omitempty"`
	RepairPrerequisiteIDs []string          `json:"repair_prerequisite_result_ids,omitempty"`
	RepairRounds          int               `json:"repair_rounds"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
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

type StepContract struct {
	Provider       string            `json:"provider"`
	Entry          string            `json:"entry"`
	Version        string            `json:"version"`
	InputSchema    string            `json:"input_schema"`
	OutputSchema   string            `json:"output_schema"`
	ExecutionSite  string            `json:"execution_site"`
	Timeout        time.Duration     `json:"timeout"`
	Command        []string          `json:"command,omitempty"`
	Environment    map[string]string `json:"environment,omitempty"`
	RequiredChecks []string          `json:"required_checks,omitempty"`
}

func (c *StepContract) UnmarshalJSON(data []byte) error {
	type rawContract struct {
		Provider       string            `json:"provider"`
		Entry          string            `json:"entry"`
		Version        string            `json:"version"`
		InputSchema    string            `json:"input_schema"`
		OutputSchema   string            `json:"output_schema"`
		ExecutionSite  string            `json:"execution_site"`
		Timeout        json.RawMessage   `json:"timeout"`
		Command        []string          `json:"command,omitempty"`
		Environment    map[string]string `json:"environment,omitempty"`
		RequiredChecks []string          `json:"required_checks,omitempty"`
	}
	var raw rawContract
	if err := decode(data, &raw); err != nil {
		return err
	}
	timeout := time.Duration(0)
	if len(raw.Timeout) > 0 {
		var text string
		if json.Unmarshal(raw.Timeout, &text) == nil {
			parsed, err := time.ParseDuration(text)
			if err != nil {
				return err
			}
			timeout = parsed
		} else {
			var number int64
			if err := json.Unmarshal(raw.Timeout, &number); err != nil {
				return errors.New("step timeout must be a duration string or nanoseconds")
			}
			timeout = time.Duration(number)
		}
	}
	*c = StepContract{Provider: raw.Provider, Entry: raw.Entry, Version: raw.Version, InputSchema: raw.InputSchema, OutputSchema: raw.OutputSchema, ExecutionSite: raw.ExecutionSite, Timeout: timeout, Command: raw.Command, Environment: raw.Environment, RequiredChecks: raw.RequiredChecks}
	return nil
}

type ProjectSteps struct {
	Implement StepContract `json:"implement"`
	Verify    StepContract `json:"verify"`
	Review    StepContract `json:"review"`
	PublishPR StepContract `json:"publish_pr"`
}

type MinimalWorkflowConfig struct {
	Definition WorkflowDefinition `json:"workflow_definition"`
	Steps      ProjectSteps       `json:"project_steps"`
}

func (c MinimalWorkflowConfig) Validate() error {
	if c.Definition.ID != MinimalWorkflowVersion || c.Definition.Version != MinimalWorkflowVersion {
		return errors.New("workflow_definition must pin minimal-v1")
	}
	if c.Definition.MaxRepairRounds < 0 {
		return errors.New("max_repair_rounds cannot be negative")
	}
	for id, step := range map[string]StepContract{
		"implement": c.Steps.Implement, "verify": c.Steps.Verify,
		"publish_pr": c.Steps.PublishPR,
	} {
		if err := validateStepContract(id, step); err != nil {
			return err
		}
		if step.InputSchema != MinimalStepInputSchema && step.InputSchema != NativeTaskInputSchema || step.OutputSchema != MinimalStepOutputSchema {
			return fmt.Errorf("%s must use the minimal-v1 input/output schemas", id)
		}
	}
	if c.Definition.ReviewRequired {
		if err := validateStepContract("review", c.Steps.Review); err != nil {
			return err
		}
		if c.Steps.Review.InputSchema != MinimalStepInputSchema && c.Steps.Review.InputSchema != NativeTaskInputSchema || c.Steps.Review.OutputSchema != MinimalStepOutputSchema {
			return errors.New("review must use the minimal-v1 input/output schemas")
		}
	}
	if len(c.Steps.Verify.RequiredChecks) == 0 {
		return errors.New("verify contract requires a fixed nonempty required_checks list")
	}
	if c.Steps.Verify.Provider != "command" {
		return errors.New("verify requires a deterministic command provider")
	}
	seen := map[string]bool{}
	for _, check := range c.Steps.Verify.RequiredChecks {
		if strings.TrimSpace(check) == "" || seen[check] {
			return errors.New("verify required_checks must contain unique nonempty names")
		}
		seen[check] = true
	}
	return nil
}

func DefaultMinimalWorkflowDefinition(reviewRequired bool) WorkflowDefinition {
	return WorkflowDefinition{ID: MinimalWorkflowVersion, Version: MinimalWorkflowVersion, ReviewRequired: reviewRequired, MaxRepairRounds: 2}
}

func validateStepContract(id string, c StepContract) error {
	if c.Provider != "agent_skill" && c.Provider != "command" {
		return fmt.Errorf("%s provider must be agent_skill or command", id)
	}
	if c.Entry == "" || c.Version == "" || c.InputSchema == "" || c.OutputSchema == "" || c.ExecutionSite == "" {
		return fmt.Errorf("%s contract is incomplete", id)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("%s timeout must be positive", id)
	}
	entry := strings.ReplaceAll(c.Entry, "\\", "/")
	if !strings.HasPrefix(entry, "builtin:") && (filepath.IsAbs(c.Entry) || strings.HasPrefix(entry, "/") || strings.Contains("/"+entry+"/", "/../") || entry == "..") {
		return fmt.Errorf("%s entry must be a repository-relative or builtin entry", id)
	}
	if c.ExecutionSite != "local" && c.ExecutionSite != "target" {
		return fmt.Errorf("%s execution_site must be local or target", id)
	}
	if c.Provider == "command" && len(c.Command) == 0 {
		return fmt.Errorf("%s command must be an executable and argument array", id)
	}
	if c.Provider == "command" && strings.TrimSpace(c.Command[0]) == "" {
		return fmt.Errorf("%s command executable is empty", id)
	}
	if c.Provider == "agent_skill" && len(c.Command) != 0 {
		return fmt.Errorf("%s agent_skill cannot carry a command", id)
	}
	if c.Provider == "agent_skill" && strings.HasPrefix(c.Entry, "builtin:") && (c.Entry != "builtin:"+id || (id != "implement" && id != "review") || c.Version != MinimalWorkflowVersion) {
		return fmt.Errorf("%s has no built-in agent entry %q; configure a repository entry", id, c.Entry)
	}
	return nil
}

var revisionPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

type StepGateResult struct {
	StepID                     string            `json:"step_id"`
	Succeeded                  bool              `json:"succeeded"`
	InputRevision              string            `json:"input_revision,omitempty"`
	Revision                   string            `json:"revision,omitempty"`
	BaseAncestor               *bool             `json:"base_ancestor,omitempty"`
	Workspace                  WorkspaceEvidence `json:"workspace,omitempty"`
	Checks                     []CheckEvidence   `json:"checks,omitempty"`
	ReviewVerdict              string            `json:"review_verdict,omitempty"`
	PRURL                      string            `json:"pr_url,omitempty"`
	PublishedHead              string            `json:"published_head,omitempty"`
	ResultID                   string            `json:"result_id,omitempty"`
	FailureKind                string            `json:"failure_kind,omitempty"`
	PrerequisiteIDs            []string          `json:"prerequisite_result_ids,omitempty"`
	Decision                   *DecisionRequest  `json:"decision,omitempty"`
	AppliedDecisionID          string            `json:"applied_decision_id,omitempty"`
	AppliedDecisionAttemptID   string            `json:"applied_decision_attempt_id,omitempty"`
	AppliedDecisionExecutionID string            `json:"applied_decision_execution_id,omitempty"`
	AppliedAnswerSHA256        string            `json:"applied_answer_sha256,omitempty"`
}

type CheckEvidence struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	Revision      string `json:"revision"`
	Command       string `json:"command"`
	ScriptVersion string `json:"script_version"`
	ExitCode      *int   `json:"exit_code"`
	Passed        *bool  `json:"passed"`
	EvidenceRef   string `json:"evidence_ref"`
	ActualCommit  string `json:"actual_commit"`
}

// AdvanceMinimalExecution validates one committed step result and computes the
// next center-owned step. It never treats a worker's success as task completion.
var allowedAttemptTransitions = map[string]map[string]bool{
	"planned":          {"dispatching": true, "failed": true},
	"dispatching":      {"dispatched": true, "rejected": true, "outcome_unknown": true},
	"dispatched":       {"running": true, "failed": true, "outcome_unknown": true},
	"running":          {"waiting_decision": true, "succeeded": true, "failed": true, "stopping": true, "outcome_unknown": true},
	"waiting_decision": {"running": true, "succeeded": true, "failed": true, "outcome_unknown": true, "stopping": true},
	"stopping":         {"stopped": true, "running": true, "outcome_unknown": true},
	"outcome_unknown":  {"dispatched": true, "running": true, "succeeded": true, "failed": true, "stopped": true},
}

func TransitionStepAttempt(attempt *StepAttemptRecord, next string) error {
	if attempt == nil {
		return errors.New("step attempt is required")
	}
	if !validStepAttemptStatus(next) {
		return fmt.Errorf("invalid step attempt status %q", next)
	}
	if attempt.Status == next {
		return nil
	}
	if !allowedAttemptTransitions[attempt.Status][next] {
		return fmt.Errorf("step attempt transition %s -> %s is not allowed", attempt.Status, next)
	}
	attempt.Status = next
	attempt.UpdatedAt = now()
	return nil
}

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
