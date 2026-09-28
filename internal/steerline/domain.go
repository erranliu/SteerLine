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
	PRInstance  string    `json:"pr_instance"`
	URL         string    `json:"url"`
	Repository  string    `json:"repository"`
	PRNumber    int64     `json:"number"`
	BaseBranch  string    `json:"base_branch"`
	State       string    `json:"state"`
	Merged      bool      `json:"merged"`
	MergedAt    time.Time `json:"merged_at"`
	MergeCommit string    `json:"merge_commit"`
}

type WorkflowDefinition struct {
	ID              string `json:"id"`
	Version         string `json:"version"`
	ReviewRequired  bool   `json:"review_required"`
	MaxRepairRounds int    `json:"max_repair_rounds"`
}

type WorkflowRunRecord struct {
	ProjectIdentity  string                 `json:"project_identity"`
	WorkflowRunID    string                 `json:"workflow_run_id"`
	TaskID           string                 `json:"task_id"`
	WorkflowVersion  string                 `json:"workflow_version"`
	ConfigDigest     string                 `json:"config_digest,omitempty"`
	ExecutionProfile *FixedExecutionProfile `json:"execution_profile,omitempty"`
	Definition       WorkflowDefinition     `json:"workflow_definition"`
	ProjectSteps     ProjectSteps           `json:"project_steps,omitempty"`
	Runner           StepRunnerSettings     `json:"runner"`
	Phase            string                 `json:"phase"`
	Repository       string                 `json:"repository,omitempty"`
	Branch           string                 `json:"branch,omitempty"`
	SelectedResult   string                 `json:"selected_result_id,omitempty"`
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
	ProjectIdentity       string            `json:"project_identity"`
	ExecutionID           string            `json:"execution_id"`
	WorkflowRunID         string            `json:"workflow_run_id"`
	Agent                 string            `json:"agent,omitempty"`
	Backend               string            `json:"backend,omitempty"`
	Protocol              string            `json:"protocol,omitempty"`
	Process               *ProcessIdentity  `json:"process,omitempty"`
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
func AdvanceMinimalExecution(run *WorkflowRunRecord, exec *ExecutionRecord, result StepGateResult, requiredChecks []string) (string, error) {
	if run == nil || exec == nil {
		return "", errors.New("minimal-v1 workflow binding is incomplete")
	}
	nextRun, nextExecution := *run, *exec
	nextExecution.RepairPrerequisiteIDs = append([]string(nil), exec.RepairPrerequisiteIDs...)
	next, err := advanceMinimalExecution(&nextRun, &nextExecution, result, requiredChecks)
	if err == nil || next == "blocked" {
		*run, *exec = nextRun, nextExecution
	}
	return next, err
}

func advanceMinimalExecution(run *WorkflowRunRecord, exec *ExecutionRecord, result StepGateResult, requiredChecks []string) (string, error) {
	if run == nil || exec == nil || run.WorkflowVersion != MinimalWorkflowVersion || run.Definition.ID != MinimalWorkflowVersion || run.Definition.Version != MinimalWorkflowVersion {
		return "", errors.New("minimal-v1 workflow binding is incomplete")
	}
	if result.StepID == "" || result.StepID != exec.CurrentStep {
		return "", errors.New("step result does not match the active step")
	}
	if exec.Status != "active" {
		return "", errors.New("execution is not active")
	}
	if result.StepID != "publish_pr" && (result.PRURL != "" || result.PublishedHead != "") {
		return "", errors.New("publication evidence is only allowed in publish_pr")
	}
	if !exec.Workspace.Dirty && result.Workspace.Dirty {
		return "", errors.New("step result reports uncommitted workspace changes")
	}
	if result.Workspace.Repository != "" && exec.Workspace.Repository != "" && result.Workspace.Repository != exec.Workspace.Repository {
		return "", fmt.Errorf("%s workspace repository identity %q differs from execution identity %q", result.StepID, result.Workspace.Repository, exec.Workspace.Repository)
	}
	switch result.StepID {
	case "prepare":
		if !result.Succeeded {
			return "", errors.New("prepare step did not succeed")
		}
		if exec.Status != "active" {
			return "", errors.New("execution is not active")
		}
		if result.Workspace.Repository == "" || result.Workspace.Worktree == "" || !revisionPattern.MatchString(result.Workspace.Base) || result.Workspace.Head != result.Workspace.Base || result.Workspace.VerifiedAt.IsZero() || result.Workspace.Dirty {
			return "", errors.New("prepare requires verified repository, worktree, base revision and clean workspace evidence")
		}
		fixedBase := exec.Target.BaseRevision
		if fixedBase == "" {
			fixedBase = exec.Workspace.Base
		}
		if fixedBase != "" && result.Workspace.Base != fixedBase {
			return "", errors.New("prepare base revision differs from the fixed workflow base")
		}
		exec.Workspace = result.Workspace
		return "implement", nil
	case "implement":
		if result.FailureKind != "" {
			return "", errors.New("implement returned a failure receipt")
		}
		if !result.Succeeded {
			return "", errors.New("implement step did not succeed")
		}
		if !revisionPattern.MatchString(result.Revision) {
			return "", errors.New("implement requires a candidate commit revision")
		}
		if result.Workspace.Dirty {
			return "", errors.New("implement must return a clean committed workspace")
		}
		expectedInput := exec.Workspace.Head
		if expectedInput == "" {
			expectedInput = exec.Workspace.Base
		}
		if !revisionPattern.MatchString(exec.Workspace.Base) {
			return "", errors.New("implement preparation has no valid fixed base revision")
		}
		if result.InputRevision != expectedInput {
			return "", fmt.Errorf("implement input revision %q differs from prepared HEAD %q", result.InputRevision, expectedInput)
		}
		if result.Workspace.Repository != exec.Workspace.Repository {
			return "", fmt.Errorf("implement repository identity %q differs from prepared repository %q", result.Workspace.Repository, exec.Workspace.Repository)
		}
		if canonical(result.Workspace.Worktree) != canonical(exec.Workspace.Worktree) {
			return "", fmt.Errorf("implement worktree %q differs from prepared worktree %q", result.Workspace.Worktree, exec.Workspace.Worktree)
		}
		if result.Workspace.Base != exec.Workspace.Base {
			return "", fmt.Errorf("implement base revision %q differs from prepared base %q", result.Workspace.Base, exec.Workspace.Base)
		}
		if result.Workspace.Branch != exec.Workspace.Branch {
			return "", fmt.Errorf("implement branch %q differs from prepared branch %q", result.Workspace.Branch, exec.Workspace.Branch)
		}
		exec.CandidateRevision = result.Revision
		exec.VerifiedRevision = ""
		exec.ReviewedRevision = ""
		if exec.Workspace.Repository == "" {
			exec.Workspace.Repository = result.Workspace.Repository
		}
		if exec.Workspace.Worktree == "" {
			exec.Workspace.Worktree = result.Workspace.Worktree
		}
		if exec.Workspace.Base == "" {
			exec.Workspace.Base = result.Workspace.Base
		}
		exec.Workspace.Head = result.Revision
		exec.Workspace.Dirty = false
		if result.Workspace.VerifiedAt.IsZero() {
			return "", errors.New("implement requires a verified workspace receipt")
		}
		return "verify", nil
	case "verify":
		if result.FailureKind != "" {
			return "", fmt.Errorf("verification failed before required checks completed: %s", result.FailureKind)
		}
		if !revisionPattern.MatchString(exec.CandidateRevision) || result.Revision != exec.CandidateRevision {
			return "", errors.New("verify revision does not match candidate")
		}
		if result.Workspace.Dirty || result.Workspace.Head != exec.CandidateRevision {
			return "", errors.New("verify requires a clean workspace at the candidate revision")
		}
		if len(requiredChecks) == 0 {
			return "", errors.New("verify requires a nonempty fixed required-check list")
		}
		if exec.Protocol == string(ProtocolOrcaSupervised) {
			if err := validateSupervisedChecks(result.Checks, exec.CandidateRevision); err != nil {
				return "", err
			}
		}
		failed, err := validateChecks(requiredChecks, result.Checks, exec.CandidateRevision)
		if err != nil {
			return "", err
		}
		if failed {
			return repairOrBlock(run, exec, result, "required verification check failed")
		}
		if !result.Succeeded {
			return "", errors.New("verify step did not succeed")
		}
		if result.ResultID == "" {
			return "", errors.New("verify result must have a stable result id")
		}
		exec.VerifiedRevision = result.Revision
		return afterVerification(run, exec, result)
	case "review":
		if result.FailureKind != "" {
			return "", fmt.Errorf("review failed before a verdict: %s", result.FailureKind)
		}
		if !result.Succeeded && result.ReviewVerdict != "changes_requested" {
			return "", errors.New("review step did not succeed")
		}
		if !run.Definition.ReviewRequired {
			return "", errors.New("review was disabled in the pinned workflow definition")
		}
		if !revisionPattern.MatchString(exec.CandidateRevision) || exec.VerifiedRevision != exec.CandidateRevision || result.Revision != exec.CandidateRevision || result.Workspace.Dirty || result.Workspace.Head != exec.CandidateRevision {
			return "", errors.New("review requires evidence for the verified clean candidate")
		}
		if result.ReviewVerdict == "changes_requested" {
			return repairOrBlock(run, exec, result, "review requested changes")
		}
		if result.ReviewVerdict != "passed" {
			return "", errors.New("review verdict must be passed or changes_requested")
		}
		if result.ResultID == "" {
			return "", errors.New("review result must have a stable result id")
		}
		if result.Revision != exec.CandidateRevision {
			return "", errors.New("review revision does not match verified candidate")
		}
		if result.Workspace.Dirty || result.Workspace.Head != exec.CandidateRevision {
			return "", errors.New("review requires the verified clean revision")
		}
		exec.ReviewedRevision = result.Revision
		return "publish_pr", nil
	case "publish_pr":
		if !revisionPattern.MatchString(exec.CandidateRevision) {
			return "", errors.New("publish_pr requires a valid candidate revision")
		}
		if !result.Succeeded {
			return "", errors.New("publish_pr step did not succeed")
		}
		if exec.VerifiedRevision != exec.CandidateRevision {
			return "", errors.New("publish_pr requires successful verification of the current candidate")
		}
		if run.Definition.ReviewRequired && exec.ReviewedRevision != exec.CandidateRevision {
			return "", errors.New("publish_pr requires a passing review of the current candidate")
		}
		if result.Workspace.Dirty || result.Workspace.Head != exec.CandidateRevision {
			return "", errors.New("publish_pr requires the verified clean candidate revision")
		}
		if result.PublishedHead != exec.CandidateRevision {
			return "", errors.New("published PR head does not match verified candidate")
		}
		if result.Revision != exec.CandidateRevision {
			return "", errors.New("publish_pr revision differs from the verified candidate")
		}
		if result.PRURL == "" {
			return "", errors.New("publish_pr requires a verified PR URL")
		}
		run.Phase = "waiting_pr_merge"
		exec.Status = "succeeded"
		exec.CurrentStep = "waiting_pr_merge"
		return "waiting_pr_merge", nil
	default:
		return "", fmt.Errorf("unknown minimal-v1 step %q", result.StepID)
	}
}

func afterVerification(run *WorkflowRunRecord, exec *ExecutionRecord, result StepGateResult) (string, error) {
	exec.Workspace.Head = exec.CandidateRevision
	if run.Definition.ReviewRequired {
		return "review", nil
	}
	return "publish_pr", nil
}

func validateChecks(required []string, checks []CheckEvidence, revision string) (bool, error) {
	seen := make(map[string]CheckEvidence, len(checks))
	for _, check := range checks {
		if check.Name == "" {
			return false, errors.New("check receipt has an empty name")
		}
		if _, exists := seen[check.Name]; exists {
			return false, fmt.Errorf("duplicate check receipt %q", check.Name)
		}
		seen[check.Name] = check
	}
	failed := false
	for _, name := range required {
		check, ok := seen[name]
		if !ok || check.Status == "not_run" || check.Status == "unknown" {
			return false, fmt.Errorf("required check %q has no conclusive execution receipt", name)
		}
		if check.Revision != revision {
			return false, fmt.Errorf("required check %q was run against a different revision", name)
		}
		if check.Status == "failed" {
			failed = true
			continue
		}
		if check.Status != "passed" {
			return false, fmt.Errorf("required check %q has invalid status %q", name, check.Status)
		}
	}
	return failed, nil
}

func validateSupervisedChecks(checks []CheckEvidence, revision string) error {
	for _, check := range checks {
		if strings.TrimSpace(check.Command) == "" || strings.TrimSpace(check.ScriptVersion) == "" || strings.TrimSpace(check.EvidenceRef) == "" {
			return fmt.Errorf("check %q lacks command, script version or evidence reference", check.Name)
		}
		if check.ExitCode == nil || check.Passed == nil {
			return fmt.Errorf("check %q lacks an explicit exit code or passed value", check.Name)
		}
		if !revisionPattern.MatchString(check.ActualCommit) || !strings.EqualFold(check.ActualCommit, revision) {
			return fmt.Errorf("check %q actual commit differs from the pinned revision", check.Name)
		}
		switch check.Status {
		case "passed":
			if *check.ExitCode != 0 || !*check.Passed {
				return fmt.Errorf("check %q reports passed without exit code zero and passed=true", check.Name)
			}
		case "failed":
			if *check.ExitCode == 0 || *check.Passed {
				return fmt.Errorf("check %q reports failed without a nonzero exit code and passed=false", check.Name)
			}
		default:
			return fmt.Errorf("check %q has inconclusive status %q", check.Name, check.Status)
		}
	}
	return nil
}

func repairOrBlock(run *WorkflowRunRecord, exec *ExecutionRecord, result StepGateResult, reason string) (string, error) {
	if result.FailureKind != "" {
		return "", fmt.Errorf("automatic repair is disabled for %s failure", result.FailureKind)
	}
	if result.ResultID == "" {
		return "", errors.New("repair requires a stable failed-result ID")
	}
	if exec.RepairRounds >= run.Definition.MaxRepairRounds {
		run.Phase = "blocked"
		exec.Status = "blocked"
		return "blocked", fmt.Errorf("repair round limit reached: %s", reason)
	}
	exec.RepairRounds++
	exec.VerifiedRevision = ""
	exec.ReviewedRevision = ""
	exec.RepairPrerequisiteIDs = append(exec.RepairPrerequisiteIDs, result.PrerequisiteIDs...)
	exec.RepairPrerequisiteIDs = append(exec.RepairPrerequisiteIDs, result.ResultID)
	exec.CurrentStep = "implement"
	return "implement", nil
}

func SetWorkflowDecision(run *WorkflowRunRecord, exec *ExecutionRecord, attempt *StepAttemptRecord, status string) error {
	if run == nil || exec == nil || attempt == nil {
		return errors.New("decision scope requires workflow, execution and attempt")
	}
	if status != "pending" && status != "answer_saved" && status != "reply_accepted" && status != "applied" && status != "failed" && status != "superseded" {
		return fmt.Errorf("invalid decision status %q", status)
	}
	if exec.CurrentStep != attempt.StepID || attempt.Status != "running" && attempt.Status != "waiting_decision" {
		return errors.New("decision does not belong to the active step attempt")
	}
	if status == "pending" || status == "answer_saved" || status == "reply_accepted" {
		attempt.Status = "waiting_decision"
		exec.Status = "waiting_decision"
		run.Phase = "waiting_decision"
	}
	if status == "applied" {
		attempt.Status = "running"
		exec.Status = "active"
		run.Phase = "executing"
	}
	return nil
}

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
	case "planned", "dispatching", "dispatched", "running", "waiting_decision", "succeeded", "failed", "rejected", "stopping", "stopped", "outcome_unknown":
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
	case "pending", "active", "waiting_decision", "succeeded", "failed", "blocked", "stopped":
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

func MinimalStepSequence(def WorkflowDefinition) []string {
	steps := []string{"prepare", "implement", "verify"}
	if def.ReviewRequired {
		steps = append(steps, "review")
	}
	return append(steps, "publish_pr")
}

type DeliveryEvidence struct {
	PRURL              string
	PublishedHead      string
	MergeCommit        string
	TargetBranch       string
	Merged             bool
	ProviderTaskStatus string
	ObservedAt         time.Time
}

func SelectWorkflowResult(run *WorkflowRunRecord, executions []ExecutionRecord, selected string) error {
	if run == nil || len(executions) == 0 {
		return errors.New("workflow result selection requires an execution")
	}
	eligible := []ExecutionRecord{}
	for _, execution := range executions {
		if execution.WorkflowRunID == run.WorkflowRunID && execution.Status == "succeeded" && execution.CurrentStep == "waiting_pr_merge" {
			eligible = append(eligible, execution)
		}
	}
	if len(eligible) == 1 && selected == "" {
		run.SelectedResult = eligible[0].ExecutionID
		return nil
	}
	for _, execution := range eligible {
		if execution.ExecutionID == selected {
			run.SelectedResult = selected
			return nil
		}
	}
	return errors.New("workflow result must be explicitly selected from a successful execution")
}

func CompleteMergedDelivery(task *TaskRecord, run *WorkflowRunRecord, execution *ExecutionRecord, evidence DeliveryEvidence) error {
	if task == nil || run == nil || execution == nil {
		return errors.New("completion requires task, workflow run and selected execution")
	}
	if run.Phase != "waiting_pr_merge" || run.SelectedResult != execution.ExecutionID || execution.WorkflowRunID != run.WorkflowRunID {
		return errors.New("completion does not match the selected workflow result")
	}
	if !evidence.Merged || evidence.MergeCommit == "" || evidence.ProviderTaskStatus != "closed" || evidence.PRURL == "" || evidence.PublishedHead == "" || evidence.TargetBranch == "" || evidence.ObservedAt.IsZero() {
		return errors.New("delivery and task closure must be independently verified")
	}
	if execution.CandidateRevision != "" && evidence.PublishedHead != execution.CandidateRevision {
		return errors.New("merged PR head differs from the selected candidate")
	}
	task.Status = "closed"
	task.Version++
	task.UpdatedAt = now()
	run.Phase = "completed"
	run.UpdatedAt = now()
	execution.Status = "succeeded"
	execution.CurrentStep = "completed"
	execution.UpdatedAt = now()
	return nil
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

func AggregateWorkflowPhase(run *WorkflowRunRecord, executions []ExecutionRecord) {
	if run == nil {
		return
	}
	run.PendingDecisions = 0
	active, waitingPR, blocked := 0, 0, 0
	allSucceeded := len(executions) > 0
	for _, execution := range executions {
		switch execution.Status {
		case "active":
			active++
			allSucceeded = false
		case "waiting_decision":
			run.PendingDecisions++
			allSucceeded = false
		case "succeeded":
			if execution.CurrentStep == "waiting_pr_merge" {
				waitingPR++
			}
		case "pending":
			allSucceeded = false
		case "failed", "blocked", "stopped":
			blocked++
			allSucceeded = false
		default:
			allSucceeded = false
		}
	}
	switch {
	case active > 0:
		run.Phase = "executing"
	case run.PendingDecisions > 0:
		run.Phase = "waiting_decision"
	case waitingPR > 0:
		run.Phase = "waiting_pr_merge"
	case allSucceeded:
		run.Phase = "completing"
	case blocked > 0:
		run.Phase = "blocked"
	default:
		run.Phase = "queued"
	}
}
