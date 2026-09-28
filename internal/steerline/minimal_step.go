package steerline

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	MinimalStepInputSchema  = "steerline/minimal-step-input/v1"
	MinimalStepOutputSchema = "steerline/minimal-step-result/v1"
	NativeTaskInputSchema   = "native-task/v1"
	minimalStepOutputMax    = 2 << 20
)

type NativeTaskWorkerContext struct {
	SchemaVersion  string           `json:"schema_version"`
	ProjectID      string           `json:"project_id"`
	TaskID         string           `json:"task_id"`
	Contract       TaskContract     `json:"contract"`
	ContractDigest string           `json:"contract_digest"`
	WorkflowRunID  string           `json:"workflow_run_id"`
	ExecutionID    string           `json:"execution_id"`
	AttemptID      string           `json:"attempt_id"`
	Repository     string           `json:"repository"`
	Branch         string           `json:"branch"`
	BaseCommit     string           `json:"base_commit"`
	CurrentStep    string           `json:"current_step"`
	StepInput      MinimalStepInput `json:"step_input"`
}

type MinimalStepInput struct {
	SchemaVersion         int                          `json:"schema_version"`
	ProjectIdentity       string                       `json:"project_identity"`
	TaskID                string                       `json:"task_id"`
	WorkflowRunID         string                       `json:"workflow_run_id"`
	ExecutionID           string                       `json:"execution_id"`
	AttemptID             string                       `json:"attempt_id"`
	StepID                string                       `json:"step_id"`
	InputRevision         string                       `json:"input_revision,omitempty"`
	CandidateRevision     string                       `json:"candidate_revision,omitempty"`
	PrerequisiteResultIDs []string                     `json:"prerequisite_result_ids,omitempty"`
	PrerequisiteResults   []json.RawMessage            `json:"prerequisite_results,omitempty"`
	WorkflowDefinition    WorkflowDefinition           `json:"workflow_definition"`
	ProjectSteps          ProjectSteps                 `json:"project_steps"`
	Workspace             WorkspaceEvidence            `json:"workspace"`
	Task                  TaskRecord                   `json:"task"`
	Decision              *MinimalDecisionContinuation `json:"decision,omitempty"`
}

type MinimalDecisionContinuation struct {
	Request      DecisionRequest `json:"request"`
	Answer       DecisionAnswer  `json:"answer"`
	AttemptID    string          `json:"attempt_id"`
	ExecutionID  string          `json:"execution_id"`
	AnswerSHA256 string          `json:"answer_sha256"`
}

type MinimalStepResultEnvelope struct {
	SchemaVersion   int            `json:"schema_version"`
	ProjectIdentity string         `json:"project_identity"`
	TaskID          string         `json:"task_id"`
	WorkflowRunID   string         `json:"workflow_run_id"`
	ExecutionID     string         `json:"execution_id"`
	AttemptID       string         `json:"attempt_id"`
	Result          StepGateResult `json:"result"`
}

//go:embed minimal-step-result.schema.json
var MinimalStepResultSchema []byte

func (c Config) minimalConfig() (*MinimalWorkflowConfig, error) {
	if c.WorkflowDefinition == nil && c.ProjectSteps == nil {
		return nil, nil
	}
	if c.WorkflowDefinition == nil || c.ProjectSteps == nil {
		return nil, errors.New("minimal-v1 requires workflow_definition and project_steps")
	}
	value := &MinimalWorkflowConfig{Definition: *c.WorkflowDefinition, Steps: *c.ProjectSteps}
	if err := value.Validate(); err != nil {
		return nil, err
	}
	return value, nil
}

func (c Config) validateNativeWorkflow() (*MinimalWorkflowConfig, error) {
	workflow, err := c.minimalConfig()
	if err != nil {
		return nil, err
	}
	if workflow == nil {
		return nil, errors.New("native projects require workflow_definition and project_steps")
	}
	executionSite := "local"
	if c.hostName() == "orca" {
		if runtime.GOOS != "windows" || !strings.EqualFold(filepath.Base(c.Orca.Shell), "cmd.exe") {
			return nil, UnsupportedAdapterOperation("native-task/v1", "Orca native project steps require the supported Windows terminal v2 cmd.exe target")
		}
		executionSite = "target"
	} else if c.hostName() == "orca_supervised" {
		executionSite = "target"
	} else if c.hostName() != "local" {
		return nil, UnsupportedAdapterOperation("native-task/v1", "configured execution host has no native project-step adapter")
	}
	steps := []StepContract{workflow.Steps.Implement, workflow.Steps.Verify, workflow.Steps.PublishPR}
	if workflow.Definition.ReviewRequired {
		steps = append(steps, workflow.Steps.Review)
	}
	for _, step := range steps {
		if step.InputSchema != NativeTaskInputSchema {
			return nil, fmt.Errorf("native project_steps must use %s input context", NativeTaskInputSchema)
		}
		if step.ExecutionSite != executionSite {
			return nil, fmt.Errorf("native %s worker requires every project step to declare execution_site=%s", c.hostName(), executionSite)
		}
		for name := range step.Environment {
			upper := strings.ToUpper(name)
			if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(name) {
				return nil, fmt.Errorf("native project_steps has invalid environment variable name %q", name)
			}
			if strings.HasPrefix(upper, "BEADS_") || strings.HasPrefix(upper, "BD_") || upper == "BEADS_DIR" || upper == "BEADS_ACTOR" {
				return nil, fmt.Errorf("native project_steps cannot inject Beads environment variable %q", name)
			}
			if upper == "STEERLINE_WORKFLOW_RUN_ID" || upper == "STEERLINE_EXECUTION_ID" || upper == "STEERLINE_ATTEMPT_ID" || upper == "STEERLINE_STEP_ID" {
				return nil, fmt.Errorf("native project_steps cannot override execution identity environment variable %q", name)
			}
			if c.hostName() == "orca" || c.hostName() == "orca_supervised" {
				return nil, errors.New("remote native project steps cannot pin environment values in an unverified terminal command")
			}
		}
	}
	return workflow, nil
}

func (c Config) validateMinimalPreflight(workflow MinimalWorkflowConfig) error {
	contracts := map[string]StepContract{"implement": workflow.Steps.Implement, "verify": workflow.Steps.Verify, "publish_pr": workflow.Steps.PublishPR}
	if workflow.Definition.ReviewRequired {
		contracts["review"] = workflow.Steps.Review
	}
	for name, contract := range contracts {
		if err := validateMinimalEntry(c.Repository, contract); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if contract.Provider == "agent_skill" {
			if c.agentName() != "codex" {
				return UnsupportedAdapterOperation("Preflight", "local agent_skill steps currently require agent=codex")
			}
			if strings.TrimSpace(c.Codex.Executable) == "" {
				return fmt.Errorf("%s requires a configured Codex executable", name)
			}
			if _, err := exec.LookPath(c.Codex.Executable); err != nil {
				return fmt.Errorf("%s Codex preflight: %w", name, err)
			}
			if !strings.HasPrefix(contract.Entry, "builtin:") {
				entry := filepath.Join(c.Repository, filepath.FromSlash(contract.Entry))
				info, err := os.Stat(entry)
				if err != nil {
					return fmt.Errorf("%s skill entry %q: %w", name, contract.Entry, err)
				}
				if info.IsDir() {
					return fmt.Errorf("%s skill entry must be a file", name)
				}
			}
		}
		if contract.Provider == "command" {
			executable := contract.Command[0]
			if strings.ContainsAny(executable, `/\\`) {
				if !filepath.IsAbs(executable) {
					executable = filepath.Join(c.Repository, executable)
					if !inside(c.Repository, executable) {
						return fmt.Errorf("%s command executable escapes the repository", name)
					}
				}
			}
			resolved, err := exec.LookPath(executable)
			if err != nil {
				return fmt.Errorf("%s command preflight: %w", name, err)
			}
			ext := strings.ToLower(filepath.Ext(resolved))
			if ext == ".cmd" || ext == ".bat" || ext == ".ps1" {
				return fmt.Errorf("%s command must be a native executable, not a shell shim", name)
			}
		}
	}
	return nil
}

func validateMinimalEntry(root string, contract StepContract) error {
	if strings.HasPrefix(contract.Entry, "builtin:") {
		return nil
	}
	entry, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(contract.Entry)))
	if err != nil {
		return err
	}
	if !inside(root, entry) {
		return errors.New("step entry resolves outside its repository")
	}
	info, err := os.Stat(entry)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("step entry must be a regular file")
	}
	return nil
}

func (c Config) validateMinimalEntriesAtBase(ctx context.Context, workflow MinimalWorkflowConfig, base string) error {
	steps := []struct {
		name     string
		contract StepContract
	}{{"implement", workflow.Steps.Implement}, {"verify", workflow.Steps.Verify}, {"publish_pr", workflow.Steps.PublishPR}}
	if workflow.Definition.ReviewRequired {
		steps = append(steps, struct {
			name     string
			contract StepContract
		}{"review", workflow.Steps.Review})
	}
	for _, step := range steps {
		if strings.HasPrefix(step.contract.Entry, "builtin:") {
			continue
		}
		entry := filepath.ToSlash(step.contract.Entry)
		output, err := gitRunner(c, c.Repository).Output(ctx, c.Git, "ls-tree", base, "--", entry)
		if err != nil {
			return err
		}
		fields := strings.Fields(string(output))
		if len(fields) < 3 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" {
			return fmt.Errorf("step entry %q must be a committed regular file at the pinned base", step.contract.Entry)
		}
		content, err := gitRunner(c, c.Repository).Output(ctx, c.Git, "show", base+":"+entry)
		if err != nil {
			return fmt.Errorf("step entry %q cannot be read at the pinned base: %w", step.contract.Entry, err)
		}
		if err = rejectLegacyStepDependencies(step.name, content); err != nil {
			return err
		}
	}
	return nil
}

func nextStepAttemptNumber(attempts []StepAttemptRecord, executionID, stepID string) int {
	next := 1
	for _, attempt := range attempts {
		if attempt.ExecutionID == executionID && attempt.StepID == stepID && attempt.AttemptNumber >= next {
			next = attempt.AttemptNumber + 1
		}
	}
	return next
}

func (s *Service) minimalRecords(run *Run) (*WorkflowRunRecord, *ExecutionRecord) {
	if run == nil {
		return nil, nil
	}
	var workflow *WorkflowRunRecord
	for i := range s.State.WorkflowRuns {
		if s.State.WorkflowRuns[i].WorkflowRunID == run.RunID {
			workflow = &s.State.WorkflowRuns[i]
			break
		}
	}
	var execution *ExecutionRecord
	for i := range s.State.Executions {
		if s.State.Executions[i].ExecutionID == run.ExecutionID && s.State.Executions[i].WorkflowRunID == run.RunID {
			execution = &s.State.Executions[i]
			break
		}
	}
	return workflow, execution
}

func (s *Service) minimalAttempt(run *Run, attemptID string) *StepAttemptRecord {
	if run == nil {
		return nil
	}
	for i := range s.State.Attempts {
		if s.State.Attempts[i].AttemptID == attemptID && s.State.Attempts[i].ExecutionID == run.ExecutionID {
			return &s.State.Attempts[i]
		}
	}
	return nil
}

func (s *Service) inspectMinimalWorkspace(ctx context.Context, run *Run) (WorkspaceEvidence, error) {
	if run == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || run.BaseCommit == nil || run.WorkspaceIntent == nil {
		return WorkspaceEvidence{}, errors.New("minimal execution has no recorded workspace")
	}
	c, configErr := s.executionConfigForRun(run)
	if configErr != nil {
		return WorkspaceEvidence{}, configErr
	}
	if run.Backend == string(BackendOrca) && run.Protocol == string(ProtocolOrcaSupervised) {
		if err := restoreSupervisedTarget(run, s.State.Attempts, targetFor(c), executionRecords{root: c.RunDir}); err != nil {
			return WorkspaceEvidence{}, err
		}
		records := executionRecords{root: c.RunDir}
		bindingAttemptID := run.AttemptID
		ref, err := records.loadBinding(run.RunID, bindingAttemptID)
		if os.IsNotExist(err) {
			// A newly planned step has no binding yet. Reuse the most recent
			// settled attempt's exact Dispatch only for read-only workspace facts.
			var latest StepAttemptRecord
			for _, candidate := range s.State.Attempts {
				if candidate.ExecutionID == run.ExecutionID && candidate.ExternalBinding.WorktreeID == run.Target.WorktreeID && candidate.ExternalBinding.Dispatch != "" && (latest.UpdatedAt.IsZero() || candidate.UpdatedAt.After(latest.UpdatedAt)) {
					latest = candidate
				}
			}
			if latest.AttemptID != "" {
				bindingAttemptID = latest.AttemptID
				ref, err = records.loadBinding(run.RunID, bindingAttemptID)
			}
		}
		if err != nil {
			return WorkspaceEvidence{}, err
		}
		inspection, err := (orcaSupervisedBackend{config: c}).InspectWorkspace(ctx, ref)
		if err != nil {
			return WorkspaceEvidence{}, err
		}
		if execution := s.executionRecord(run); execution != nil {
			inspection.Dirty = execution.Workspace.Dirty
		}
		return WorkspaceEvidence{Repository: inspection.Repository, Worktree: canonical(inspection.Worktree), Branch: inspection.Branch, Base: inspection.Base, Head: inspection.Head, Dirty: inspection.Dirty, VerifiedAt: inspection.CheckedAt}, nil
	}
	worktree := *run.WorktreePath
	if canonical(worktree) != canonical(run.WorkspaceIntent.Path) || *run.WorktreeBranch != run.WorkspaceIntent.Branch || *run.BaseCommit != run.WorkspaceIntent.BaseCommit {
		return WorkspaceEvidence{}, nativeErr("authority_mismatch", "minimal workspace differs from its fixed durable intent")
	}
	backend := backendFor(c)
	kind, protocol := backend.Identity()
	if string(kind) != run.Backend || string(protocol) != run.Protocol || run.Target.ID != targetFor(c).ID {
		return WorkspaceEvidence{}, UnsupportedAdapterOperation("InspectWorkspace", "configured target differs from the native execution identity")
	}
	inspection, err := backend.InspectWorkspace(ctx, ExecutionRef{Version: ExecutionRecordVersion, Backend: kind, Protocol: protocol, Target: targetFor(c), RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: worktree, WorkspaceBranch: *run.WorktreeBranch, BaseCommit: *run.BaseCommit})
	if err != nil {
		return WorkspaceEvidence{}, err
	}
	if canonical(inspection.Worktree) != canonical(worktree) || inspection.Branch != *run.WorktreeBranch || inspection.Base != *run.BaseCommit || !revisionPattern.MatchString(inspection.Head) {
		return WorkspaceEvidence{}, nativeErr("contract_mismatch", "native target workspace inspection differs from its fixed identity")
	}
	return WorkspaceEvidence{Repository: c.identity, Worktree: canonical(worktree), Branch: inspection.Branch, Base: inspection.Base, Head: inspection.Head, Dirty: inspection.Dirty, VerifiedAt: inspection.CheckedAt}, nil
}

func restoreSupervisedTarget(run *Run, attempts []StepAttemptRecord, fixedTarget ExecutionTarget, records executionRecords) error {
	if run.Target.ID != "" && run.Target.ID != fixedTarget.ID {
		return nativeErr("authority_mismatch", fmt.Sprintf("supervised execution target ID %q differs from configured target %q", run.Target.ID, fixedTarget.ID))
	}
	if run.Target.ConnectionID != "" && run.Target.ConnectionID != fixedTarget.ConnectionID || run.Target.ExternalRuntime != "" && run.Target.ExternalRuntime != fixedTarget.ExternalRuntime {
		return nativeErr("authority_mismatch", "runtime target connection differs from the configured supervised target")
	}
	var recovered *ExecutionTarget
	for i := range attempts {
		attempt := &attempts[i]
		binding := attempt.ExternalBinding
		if attempt.ExecutionID != run.ExecutionID || attempt.StepID != "prepare" {
			continue
		}
		if binding.WorktreeID == "" || binding.OrchestrationRun == "" || binding.Dispatch == "" {
			ref, err := records.loadBinding(run.RunID, attempt.AttemptID)
			if err != nil {
				continue
			}
			binding = ref.Target
		}
		if binding.ID == "" || binding.WorktreeID == "" || binding.OrchestrationRun == "" || binding.Dispatch == "" {
			continue
		}
		if binding.ID != fixedTarget.ID || binding.ConnectionID != fixedTarget.ConnectionID || binding.ExternalRuntime != fixedTarget.ExternalRuntime {
			return nativeErr("authority_mismatch", "persisted prepare binding differs from the configured supervised target")
		}
		if recovered != nil && (recovered.WorktreeID != binding.WorktreeID || recovered.OrchestrationRun != binding.OrchestrationRun || recovered.Dispatch != binding.Dispatch) {
			return nativeErr("authority_mismatch", "persisted prepare bindings disagree on the fixed supervised worktree")
		}
		copy := binding
		recovered = &copy
	}
	if recovered == nil {
		return UnsupportedAdapterOperation("InspectWorkspace", "supervised execution lacks its exact fixed prepare binding")
	}
	if run.Target.WorktreeID != "" && run.Target.WorktreeID != recovered.WorktreeID ||
		run.Target.OrchestrationRun != "" && run.Target.OrchestrationRun != recovered.OrchestrationRun {
		return nativeErr("authority_mismatch", "runtime target conflicts with its fixed prepare binding")
	}
	if run.Target.WorktreeID != "" && run.Target.OrchestrationRun != "" {
		return nil
	}
	run.Target = *recovered
	return nil
}

func applyMinimalWorkspaceEvidence(run *Run, execution *ExecutionRecord, workspace WorkspaceEvidence) {
	applyMinimalWorkspaceTarget(run, execution, workspace)
	execution.Workspace = workspace
	execution.UpdatedAt = now()
}

func applyMinimalWorkspaceTarget(run *Run, execution *ExecutionRecord, workspace WorkspaceEvidence) {
	if run.Protocol == string(ProtocolOrcaSupervised) {
		execution.Target = run.Target
	} else {
		execution.Target.WorktreeID = workspace.Worktree
	}
	execution.Target.Branch = workspace.Branch
	execution.Target.BaseRevision = workspace.Base
	run.Target = execution.Target
}

func minimalAttemptPaths(c Config, run *Run, attempt *StepAttemptRecord) (dir, events, stderr, result string) {
	dir = filepath.Join(c.RunDir, "runs", run.RunID, "steps", attempt.AttemptID)
	return dir, filepath.Join(dir, "events.jsonl"), filepath.Join(dir, "stderr.log"), filepath.Join(dir, "result.json")
}

func buildMinimalStepInput(project string, task TaskRecord, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, attempt *StepAttemptRecord, history ...StepAttemptRecord) MinimalStepInput {
	input := MinimalStepInput{SchemaVersion: 1, ProjectIdentity: project, TaskID: run.DomainTaskID, WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, AttemptID: attempt.AttemptID, StepID: attempt.StepID, InputRevision: attempt.InputRevision, CandidateRevision: execution.CandidateRevision, PrerequisiteResultIDs: append([]string(nil), attempt.PrerequisiteIDs...), WorkflowDefinition: workflow.Definition, ProjectSteps: workflow.ProjectSteps, Workspace: execution.Workspace, Task: task}
	for _, id := range attempt.PrerequisiteIDs {
		for _, prior := range history {
			if prior.ExecutionID == execution.ExecutionID && prior.ResultID == id && len(prior.Result) > 0 {
				input.PrerequisiteResults = append(input.PrerequisiteResults, append(json.RawMessage(nil), prior.Result...))
				break
			}
		}
	}
	if run.Decision != nil && (run.Decision.Status == "answer_saved" || run.Decision.Status == "resuming" || run.Decision.Status == "reply_accepted") {
		answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: run.Decision.OptionID, Note: run.Decision.Note, AnsweredBy: run.Decision.AnsweredBy}
		if run.Decision.AnsweredAt != nil {
			answer.AnsweredAt = *run.Decision.AnsweredAt
		}
		input.Decision = &MinimalDecisionContinuation{Request: run.Decision.Request, Answer: answer, AttemptID: run.Decision.AttemptID, ExecutionID: run.ExecutionID, AnswerSHA256: decisionAnswerDigest(answer)}
	}
	return input
}

func writeNativeMinimalPrompt(path string, input NativeTaskWorkerContext, contract StepContract) error {
	data, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return err
	}
	prompt := fmt.Sprintf("Execute only native task step %q. Read the fixed contract and follow project entry %q at version %q. Input is one JSON object with schema_version %q. Return one JSON object matching the supplied output schema, without Markdown or extra text. Do not claim or close tasks, update center state, dispatch another step, or merge a pull request.\n\n%s\n", input.CurrentStep, contract.Entry, contract.Version, NativeTaskInputSchema, string(data))
	prompt += "Pinned reference SHA-256 values cover the raw Git blob bytes at each reference's commit and path. Hash those bytes directly (for example, Python subprocess.check_output for git show COMMIT:PATH); do not hash the checked-out file or text passed through a shell, because line-ending conversion changes its bytes.\n"
	if input.CurrentStep == "implement" {
		prompt += "Before reporting success, commit the implementation on the assigned branch. Check that HEAD is the new candidate commit, the worktree is clean, and its diff from the fixed base satisfies the task contract. Set result.revision and result.workspace.head to that exact commit and result.workspace.dirty to false. If any check fails, report succeeded=false; never report success for uncommitted changes.\n"
	}
	if input.StepInput.Decision != nil {
		prompt += "After applying the supplied human answer, include result.applied_decision_id equal to decision.answer.decision_id as application evidence.\n"
	}
	return AtomicWrite(path, []byte(prompt))
}

func (s *Service) minimalWorkerInput(ctx context.Context, task TaskRecord, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, attempt *StepAttemptRecord, input MinimalStepInput) (any, error) {
	remotePrepare := s.Config.hostName() == "orca_supervised" && attempt != nil && attempt.StepID == "prepare"
	if run == nil || workflow == nil || execution == nil || attempt == nil || run.BaseCommit == nil || (!remotePrepare && (run.WorktreePath == nil || run.WorktreeBranch == nil)) {
		return nil, nativeErr("contract_mismatch", "native worker context requires a fixed task, workflow, execution, attempt and workspace")
	}
	taskRecord := nativeTaskRecord(s.State.Tasks, task.TaskID)
	if taskRecord == nil || taskRecord.Status != "in_progress" || taskRecord.ContractID == "" {
		return nil, nativeErr("stale_version", "native task is no longer in progress or its contract is missing")
	}
	contract, err := nativeContractForTask(ctx, s.domainStore, s.Config.identity, *taskRecord)
	if err != nil {
		return nil, err
	}
	authority, err := s.domainStore.nativeAuthority(ctx, s.Config.identity)
	if err != nil {
		return nil, err
	}
	if contract.Repository != authority.Repository || contract.Remote != authority.Remote || contract.Branch != authority.Branch {
		return nil, nativeErr("authority_mismatch", "native task contract differs from the registered repository target")
	}
	workspace := WorkspaceEvidence{Repository: s.Config.identity, Branch: s.Config.Branch, Base: *run.BaseCommit, Head: *run.BaseCommit}
	if !remotePrepare {
		workspace, err = s.inspectMinimalWorkspace(ctx, run)
		if err != nil {
			return nil, fmt.Errorf("native execution workspace could not be verified: %w", err)
		}
		if canonical(workspace.Worktree) != canonical(*run.WorktreePath) || workspace.Branch != *run.WorktreeBranch || workspace.Base != *run.BaseCommit || workspace.Base != execution.Workspace.Base {
			return nil, nativeErr("contract_mismatch", "native execution workspace identity differs from its claimed base or fixed branch")
		}
	} else if execution.Workspace.Base != workspace.Base || execution.Target.WorktreeID != "" {
		return nil, nativeErr("contract_mismatch", "native remote prepare has a conflicting base or premature worktree identity")
	}
	contractConfig := s.Config
	if s.Config.hostName() != "orca_supervised" {
		contractConfig.Repository = workspace.Worktree
	}
	if verifyErr := VerifyTaskContractFiles(ctx, contractConfig, contract); verifyErr != nil {
		return nil, fmt.Errorf("native execution contract verification failed: %w", verifyErr)
	}
	return NativeTaskWorkerContext{SchemaVersion: NativeTaskInputSchema, ProjectID: s.Config.identity, TaskID: taskRecord.TaskID, Contract: contract, ContractDigest: contract.Digest, WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, AttemptID: attempt.AttemptID, Repository: contract.Repository, Branch: workspace.Branch, BaseCommit: workspace.Base, CurrentStep: attempt.StepID, StepInput: input}, nil
}

func minimalStepEnv(c Config, run *Run, attempt *StepAttemptRecord) map[string]string {
	env := map[string]string{"STEERLINE_WORKFLOW_RUN_ID": run.RunID, "STEERLINE_EXECUTION_ID": run.ExecutionID, "STEERLINE_ATTEMPT_ID": attempt.AttemptID, "STEERLINE_STEP_ID": attempt.StepID}
	return env
}

func commandStepArgs(c Config, run *Run, attempt *StepAttemptRecord, input MinimalStepInput, contract StepContract, resultPath string) (string, []string, error) {
	if len(contract.Command) == 0 {
		return "", nil, errors.New("command step has no executable")
	}
	worktree := deref(run.WorktreePath)
	replace := strings.NewReplacer("{task_id}", run.DomainTaskID, "{workflow_run_id}", run.RunID, "{execution_id}", run.ExecutionID, "{attempt_id}", attempt.AttemptID, "{step_id}", attempt.StepID, "{repository}", c.identity, "{worktree}", worktree, "{input_revision}", input.InputRevision, "{candidate_revision}", input.CandidateRevision, "{step_result_path}", resultPath)
	args := make([]string, len(contract.Command))
	for i, value := range contract.Command {
		args[i] = replace.Replace(value)
	}
	executable := args[0]
	if strings.ContainsAny(executable, `/\\`) {
		if !filepath.IsAbs(executable) {
			executable = filepath.Join(worktree, executable)
			if !inside(worktree, executable) {
				return "", nil, errors.New("step command executable escapes the worktree")
			}
		}
	}
	resolved, err := exec.LookPath(executable)
	if err != nil {
		return "", nil, err
	}
	ext := strings.ToLower(filepath.Ext(resolved))
	if ext == ".cmd" || ext == ".bat" || ext == ".ps1" {
		return "", nil, fmt.Errorf("step command must be a native executable, not a shell shim: %s", resolved)
	}
	return resolved, args[1:], nil
}

func prepareWorktreeOnly(ctx context.Context, c Config, run *Run) error {
	intent := run.WorkspaceIntent
	if intent == nil {
		return errors.New("missing durable workspace intent")
	}
	if err := c.prepareWorktreeRoot(); err != nil {
		return err
	}
	if canonical(filepath.Dir(intent.Path)) != canonical(c.WorktreeRoot) {
		return errors.New("workspace intent path no longer matches worktree_root")
	}
	if _, err := os.Lstat(intent.Path); !os.IsNotExist(err) {
		return fmt.Errorf("worktree path already exists or is inaccessible: %s", intent.Path)
	}
	if err := os.MkdirAll(c.WorktreeRoot, 0700); err != nil {
		return err
	}
	git := gitRunner(c, c.Repository)
	if _, err := git.Output(ctx, c.Git, "check-ref-format", "refs/heads/"+intent.Branch); err != nil {
		return err
	}
	refs, err := git.Output(ctx, c.Git, "for-each-ref", "--format=%(refname)", "refs/heads/"+intent.Branch)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(refs)) != "" {
		return errors.New("worktree branch already exists")
	}
	if err = os.Mkdir(intent.Path, 0700); err != nil {
		return err
	}
	if _, err = git.Output(ctx, c.Git, "worktree", "add", "--no-track", "-b", intent.Branch, "--", intent.Path, intent.BaseCommit); err != nil {
		return err
	}
	return verifyWorkspace(ctx, c, intent, true)
}

func minimalStepContract(steps ProjectSteps, step string) StepContract {
	switch step {
	case "implement":
		return steps.Implement
	case "verify":
		return steps.Verify
	case "review":
		return steps.Review
	case "publish_pr":
		return steps.PublishPR
	}
	return StepContract{}
}

func (s *Service) startMinimalStepWorker(ctx context.Context, task TaskRecord, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, attempt *StepAttemptRecord, workerInput any) (*Worker, error) {
	if run.WorktreePath == nil {
		return nil, errors.New("minimal step has no worktree")
	}
	contract := minimalStepContract(workflow.ProjectSteps, attempt.StepID)
	if err := validateMinimalEntry(*run.WorktreePath, contract); err != nil {
		return nil, err
	}
	input := buildMinimalStepInput(s.Config.identity, task, run, workflow, execution, attempt, s.State.Attempts...)
	if contract.InputSchema != NativeTaskInputSchema {
		return nil, errors.New("native step contract does not use native-task/v1")
	}
	if _, ok := workerInput.(NativeTaskWorkerContext); !ok {
		return nil, errors.New("native step worker input is missing its native-task/v1 context")
	}
	dir, events, stderr, result := minimalAttemptPaths(s.Config, run, attempt)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	run.StepResultPath = ptr(result)
	env := minimalStepEnv(s.Config, run, attempt)
	unsetEnv := []string{"BEADS_*", "BD_*"}
	workConfig := s.Config
	workConfig.Codex, workConfig.Sandbox, workConfig.Model, workConfig.ReasoningEffort, workConfig.Profile = workflow.Runner.Codex, workflow.Runner.Sandbox, workflow.Runner.Model, workflow.Runner.ReasoningEffort, workflow.Runner.Profile
	workConfig.Repository = *run.WorktreePath
	workConfig.workerEnv = env
	if workConfig.hostName() == "orca" {
		return s.startOrcaMinimalStepWorker(ctx, workConfig, task, run, workflow, execution, attempt, workerInput, contract, dir, result, input, mergeStepEnv(env, contract.Environment))
	}
	if contract.Provider == "command" {
		executable, args, err := commandStepArgs(workConfig, run, attempt, input, contract, result)
		if err != nil {
			return nil, err
		}
		payload, err := json.Marshal(workerInput)
		if err != nil {
			return nil, err
		}
		workerConfig := workConfig
		workerConfig.Repository = *run.WorktreePath
		return SpawnerFor(workerConfig).Start(ctx, SpawnSpec{RunID: run.RunID, AttemptID: attempt.AttemptID, Dir: workerConfig.Repository, EventsPath: events, StderrPath: stderr, OutputPath: result, Command: Command{Executable: executable, Args: args}, Stdin: strings.NewReader(string(payload)), Env: mergeStepEnv(env, contract.Environment), UnsetEnv: unsetEnv, Timeout: contract.Timeout})
	}
	if contract.Provider != "agent_skill" {
		return nil, fmt.Errorf("unsupported step provider %q", contract.Provider)
	}
	schema := filepath.Join(dir, "step-result.schema.json")
	if err := AtomicWrite(schema, MinimalStepResultSchema); err != nil {
		return nil, err
	}
	promptPath := filepath.Join(dir, "prompt.txt")
	nativeInput, ok := workerInput.(NativeTaskWorkerContext)
	if !ok {
		return nil, nativeErr("contract_mismatch", "native worker input is missing native-task/v1 context")
	}
	if err := writeNativeMinimalPrompt(promptPath, nativeInput, contract); err != nil {
		return nil, err
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return nil, err
	}
	args := append([]string{}, workConfig.Codex.Args...)
	args = append(args, "exec", "--json", "--output-schema", schema, "--output-last-message", result, "--sandbox", workConfig.Sandbox)
	gitWriteArgs, err := codexLinkedWorktreeWriteArgs(ctx, workConfig, workConfig.Repository)
	if err != nil {
		return nil, err
	}
	args = append(args, gitWriteArgs...)
	if workConfig.Model != "" {
		args = append(args, "--model", workConfig.Model)
	}
	if workConfig.ReasoningEffort != "" {
		args = append(args, "--config", "model_reasoning_effort="+workConfig.ReasoningEffort)
	}
	if workConfig.Profile != "" {
		args = append(args, "--profile", workConfig.Profile)
	}
	args = append(args, "-")
	spec := SpawnSpec{RunID: run.RunID, AttemptID: attempt.AttemptID, ProjectID: input.ProjectIdentity, WorkflowID: input.WorkflowRunID, ExecutionID: input.ExecutionID, Agent: workConfig.agentName(), TargetID: run.Target.ID, ConversationJSONL: true, ConversationReader: codexReaderCommand(workConfig), Dir: workConfig.Repository, EventsPath: events, StderrPath: stderr, Command: Command{Executable: workConfig.Codex.Executable, Args: args}, Stdin: strings.NewReader(string(prompt)), Env: mergeStepEnv(env, contract.Environment), UnsetEnv: unsetEnv, Timeout: contract.Timeout}
	return SpawnerFor(workConfig).Start(ctx, spec)
}

func mergeStepEnv(base, extra map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func readMinimalStepResult(path string) (MinimalStepResultEnvelope, error) {
	file, err := os.Open(path)
	if err != nil {
		return MinimalStepResultEnvelope{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, minimalStepOutputMax+1))
	if err != nil {
		return MinimalStepResultEnvelope{}, err
	}
	if len(data) > minimalStepOutputMax {
		return MinimalStepResultEnvelope{}, errors.New("minimal step output exceeds 2 MiB")
	}
	var result MinimalStepResultEnvelope
	if err = decode(data, &result); err != nil {
		return result, err
	}
	var fields struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err = json.Unmarshal(data, &fields); err != nil {
		return result, err
	}
	if value := strings.TrimSpace(string(fields.Result["succeeded"])); value != "true" && value != "false" {
		return result, errors.New("minimal step result requires an explicit succeeded boolean")
	}
	if result.SchemaVersion != 1 || result.Result.StepID == "" || result.Result.ResultID == "" {
		return result, errors.New("minimal step result identity or result ID is incomplete")
	}
	return result, nil
}

func (s *Service) setTaskStatus(taskID, status string) error {
	for i := range s.State.Tasks {
		task := &s.State.Tasks[i]
		if task.TaskID == taskID {
			return SetTaskStatus(task, status, false)
		}
	}
	return fmt.Errorf("task %s not found in SQLite model", taskID)
}

func (s *Service) minimalOperation(run *Run, attempt *StepAttemptRecord, kind, key string, payload any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	for _, existing := range s.State.Operations {
		if existing.IdempotencyKey == key {
			return "", fmt.Errorf("step operation %s already exists; reconcile it before dispatch", existing.OperationID)
		}
	}
	id := newID()
	op := OperationRecord{ProjectIdentity: s.Config.identity, OperationID: id, IdempotencyKey: key, Kind: kind, Status: "in_flight", PayloadDigest: operationDigest(data), Request: data, CreatedAt: now(), UpdatedAt: now()}
	s.State.Operations = append(s.State.Operations, op)
	if attempt != nil {
		attempt.OperationID = id
		if attempt.Status == "planned" {
			if err = TransitionStepAttempt(attempt, "dispatching"); err != nil {
				return "", err
			}
		}
	}
	if err = s.save(); err != nil {
		return "", err
	}
	return id, nil
}

// ensureSupervisedRun records the Run-create intent before the remote mutation.
// A lost response stays outcome_unknown; the caller must not create another Run.
func (s *Service) ensureSupervisedRun(ctx context.Context, run *Run) error {
	if run.Target.OrchestrationRun != "" {
		return nil
	}
	objective := "SteerLine workflow " + run.RunID + " on " + s.Config.Orca.Supervised.RepositorySelector
	opID, err := s.startOperation("create_orca_run", "orca-run:"+run.RunID, map[string]string{
		"workflow_run_id": run.RunID,
		"objective":       objective,
	})
	if err != nil {
		return err
	}
	runID, requestID, coordinator, createErr := (orcaSupervisedBackend{config: s.Config}).createOrchestrationRun(ctx, run.RunID, objective)
	if coordinator != "" {
		run.Target.CoordinatorTerminal = coordinator
	}
	if runID != "" {
		run.Target.OrchestrationRun = runID
		run.Target.OrchestrationRunRequestID = requestID
	}
	for i := range s.State.Executions {
		if s.State.Executions[i].ExecutionID == run.ExecutionID {
			s.State.Executions[i].Target = run.Target
			break
		}
	}
	status := "confirmed"
	reason := ""
	if createErr != nil {
		status, reason = "outcome_unknown", createErr.Error()
	}
	receipt := mustJSON(map[string]string{"run_id": runID, "platform_request_id": requestID, "coordinator_terminal": coordinator})
	if err := s.changeOperation(opID, status, requestID, receipt, reason); err != nil {
		return errors.Join(createErr, err)
	}
	if createErr == nil && coordinator != "" {
		coordinatorConfig := *s.Config.Orca.Supervised
		coordinatorConfig.CoordinatorTerminal = coordinator
		s.Config.Orca.Supervised = &coordinatorConfig
	}
	return createErr
}

func (s *Service) prepareMinimalExecution(ctx context.Context, task TaskRecord, run *Run) error {
	workflow, execution := s.minimalRecords(run)
	attempt := s.minimalAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("minimal prepare records are incomplete")
	}
	if s.Config.hostName() == "orca_supervised" {
		return s.startMinimalStep(ctx, task, run, attempt)
	}
	opID, err := s.minimalOperation(run, attempt, "prepare_workspace", "prepare:"+run.ExecutionID+":"+attempt.AttemptID, map[string]any{"task_id": run.DomainTaskID, "workflow_run_id": run.RunID, "execution_id": run.ExecutionID, "attempt_id": attempt.AttemptID, "workspace_intent": run.WorkspaceIntent, "backend": run.Backend, "protocol": run.Protocol})
	if err != nil {
		return err
	}
	workflow, execution = s.minimalRecords(run)
	attempt = s.minimalAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("minimal prepare records disappeared after operation persistence")
	}
	if err = prepareWorktreeOnly(ctx, s.Config, run); err != nil {
		return s.blockMinimalPreparation(run, attempt, opID, err)
	}
	run.WorktreePath = ptr(run.WorkspaceIntent.Path)
	run.WorktreeBranch = ptr(run.WorkspaceIntent.Branch)
	run.BaseCommit = ptr(run.WorkspaceIntent.BaseCommit)
	workspace, err := s.inspectMinimalWorkspace(ctx, run)
	if err != nil {
		return s.blockMinimalPreparation(run, attempt, opID, err)
	}
	if attempt.Status == "dispatching" {
		if err = TransitionStepAttempt(attempt, "dispatched"); err == nil {
			err = TransitionStepAttempt(attempt, "running")
		}
	}
	if err == nil {
		err = TransitionStepAttempt(attempt, "succeeded")
	}
	if err != nil {
		return err
	}
	resultID := "prepare:" + attempt.AttemptID
	attempt.ResultID = resultID
	attempt.Result = mustJSON(StepGateResult{StepID: "prepare", Succeeded: true, ResultID: resultID, Workspace: workspace})
	attempt.UpdatedAt = now()
	applyMinimalWorkspaceTarget(run, execution, workspace)
	gate := StepGateResult{StepID: "prepare", Succeeded: true, ResultID: resultID, Workspace: workspace}
	next, err := AdvanceMinimalExecution(workflow, execution, gate, workflow.ProjectSteps.Verify.RequiredChecks)
	if err != nil {
		return s.blockMinimalPreparation(run, attempt, opID, err)
	}
	applyMinimalWorkspaceEvidence(run, execution, workspace)
	execution.CurrentStep = next
	workflow.Phase = "executing"
	workflow.UpdatedAt = now()
	if err = s.setOperationState(opID, "confirmed", "", mustJSON(workspace), ""); err != nil {
		return err
	}
	s.note(run, "steer", "prepare", "completed", "minimal-v1 workspace ready", workspace.Worktree, map[string]any{"base_revision": workspace.Base, "head": workspace.Head})
	newAttempt := s.makeMinimalAttempt(run, execution, workflow, next, []string{resultID})
	s.State.Attempts = append(s.State.Attempts, newAttempt)
	run.AttemptID = newAttempt.AttemptID
	run.DomainAttemptNumber = newAttempt.AttemptNumber
	run.ExecutionStatus = "pending"
	run.Process = nil
	run.StartedAt = nil
	run.StepResultPath = nil
	if err = s.save(); err != nil {
		return err
	}
	return s.startMinimalStep(ctx, task, run, s.minimalAttempt(run, newAttempt.AttemptID))
}

func (s *Service) finishMinimalStep(exitErr error) error {
	run := s.active
	if run == nil {
		return errors.New("minimal step finished without an active run")
	}
	page, err := (QueryService{Store: s.domainStore, Project: s.Config.identity}).ShowNativeTask(context.Background(), run.DomainTaskID)
	if err != nil {
		return err
	}
	if len(page.Tasks) != 1 {
		return nativeErr("task_not_found", "native task disappeared while its worker finished")
	}
	taskRecord := page.Tasks[0].TaskRecord
	for i := range s.State.Tasks {
		if s.State.Tasks[i].TaskID == taskRecord.TaskID {
			s.State.Tasks[i] = taskRecord
			break
		}
	}
	if taskRecord.Status == "discarded" {
		if err := s.reconcileNativeCleanup(context.Background()); err != nil {
			return err
		}
		if s.active == nil {
			return nil
		}
		run.ExecutionStatus = "unknown"
		run.FinishTaskStatus = "blocked"
		run.Error = ptr("task was discarded while this step was active; its result was retained as late history and cleanup remains pending")
		s.State.SlotOccupied, s.State.CurrentTask = true, ptr(run.TaskID)
		s.note(run, "steer", "discard_cleanup", "unknown", "late step result was not accepted after discard", run.AttemptID, nil)
		return s.save()
	}
	workflow, execution := s.minimalRecords(run)
	attempt := s.minimalAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("minimal step finished without normalized records")
	}
	uncertain := false
	if exitErr != nil {
		var backendErr *BackendError
		uncertain = errors.As(exitErr, &backendErr) && (backendErr.Code == BackendOutcomeUnknown || backendErr.Code == BackendUnreachable || backendErr.Code == BackendIdentityMismatch)
	}
	if !uncertain && attempt.StepID != "publish_pr" {
		execution.Process, run.Process = nil, nil
	}
	if s.worker != nil && s.workers[run.RunID] == s.worker {
		delete(s.workers, run.RunID)
	}
	s.worker = nil
	s.active = nil
	s.State.CurrentTask = nil
	s.State.SlotOccupied = false
	run.EndedAt = ptr(now())
	if exitErr != nil {
		unknown := uncertain || attempt.StepID == "publish_pr"
		var backendErr *BackendError
		if run.Protocol == string(ProtocolOrcaSupervised) && errors.As(exitErr, &backendErr) && backendErr.Code == BackendUnsupported {
			unknown = true
		}
		if !unknown && run.Decision != nil && run.Decision.Status == "reply_accepted" {
			if err := s.failMinimalDecisionApplication(run, "resumed step terminated without application evidence: "+exitErr.Error()); err != nil {
				return err
			}
		}
		return s.failMinimalStep(run, workflow, execution, attempt, fmt.Errorf("worker exited: %w", exitErr), unknown)
	}
	envelope, err := readMinimalStepResult(deref(run.StepResultPath))
	if err != nil {
		if run.Decision != nil && run.Decision.Status == "reply_accepted" {
			if failErr := s.failMinimalDecisionApplication(run, "resumed step produced no verifiable result: "+err.Error()); failErr != nil {
				return failErr
			}
		}
		return s.failMinimalStep(run, workflow, execution, attempt, fmt.Errorf("invalid step result: %w", err), attempt.StepID == "publish_pr")
	}
	if envelope.ProjectIdentity != s.Config.identity || envelope.TaskID != run.DomainTaskID || envelope.WorkflowRunID != run.RunID || envelope.ExecutionID != execution.ExecutionID || envelope.AttemptID != attempt.AttemptID || envelope.Result.StepID != attempt.StepID {
		if run.Decision != nil && run.Decision.Status == "reply_accepted" {
			if err := s.failMinimalDecisionApplication(run, "resumed step result identity did not match"); err != nil {
				return err
			}
		}
		return s.failMinimalStep(run, workflow, execution, attempt, errors.New("step result identity does not match the dispatched attempt"), attempt.StepID == "publish_pr")
	}
	if persistErr := s.recordMinimalExecutionResult(run, envelope, deref(run.StepResultPath)); persistErr != nil {
		s.setError(fmt.Errorf("structured step result conversation persistence failed: %w", persistErr))
	}
	if run.Protocol == string(ProtocolOrcaSupervised) && (envelope.Result.BaseAncestor == nil || !*envelope.Result.BaseAncestor) {
		return s.failMinimalStep(run, workflow, execution, attempt, errors.New("supervised result lacks positive fixed-base ancestry evidence"), attempt.StepID == "publish_pr")
	}
	if err = confirmMinimalAttemptRan(attempt); err != nil {
		return s.failMinimalStep(run, workflow, execution, attempt, err, false)
	}
	for _, old := range s.State.Attempts {
		if old.ExecutionID == attempt.ExecutionID && old.ResultID == envelope.Result.ResultID && old.AttemptID != attempt.AttemptID {
			return s.failMinimalStep(run, workflow, execution, attempt, fmt.Errorf("duplicate step result ID %q", envelope.Result.ResultID), false)
		}
	}
	if len(attempt.PrerequisiteIDs) > 0 && !sameStrings(attempt.PrerequisiteIDs, envelope.Result.PrerequisiteIDs) {
		return s.failMinimalStep(run, workflow, execution, attempt, errors.New("step result prerequisite IDs differ from the fixed attempt input"), false)
	}
	if envelope.Result.StepID != "publish_pr" && (envelope.Result.PRURL != "" || envelope.Result.PublishedHead != "") {
		return s.failMinimalStep(run, workflow, execution, attempt, errors.New("step published before publish_pr; manual reconciliation required"), true)
	}
	if run.Decision != nil && run.Decision.Status == "reply_accepted" && !minimalDecisionApplicationMatches(run, envelope.Result) {
		if err = s.failMinimalDecisionApplication(run, "step result lacks decision/attempt/execution/answer-bound application evidence"); err != nil {
			return err
		}
		return s.failMinimalStep(run, workflow, execution, attempt, errors.New("step result lacks decision/attempt/execution/answer-bound application evidence"), attempt.StepID == "publish_pr")
	}
	if err = s.recordMinimalDecisionApplied(run, envelope.Result); err != nil {
		return err
	}
	if envelope.Result.Decision != nil {
		if err = s.recordMinimalDecisionApplied(run, envelope.Result); err != nil {
			return err
		}
		if err = s.waitMinimalDecision(run, workflow, execution, attempt, envelope.Result); err != nil {
			return s.failMinimalStep(run, workflow, execution, attempt, err, false)
		}
		return nil
	}
	workspace, err := s.inspectMinimalWorkspace(context.Background(), run)
	if err != nil {
		return s.failMinimalStep(run, workflow, execution, attempt, fmt.Errorf("workspace inspection failed after step: %w", err), attempt.StepID == "publish_pr")
	}
	if err = validateMinimalWorkspaceReceipt(envelope.Result.Workspace, workspace); err != nil {
		return s.failMinimalStep(run, workflow, execution, attempt, err, attempt.StepID == "publish_pr")
	}
	if run.Protocol == string(ProtocolOrcaSupervised) {
		if err = restoreSupervisedTarget(run, s.State.Attempts, targetFor(s.Config), executionRecords{root: s.Config.RunDir}); err != nil {
			return s.failMinimalStep(run, workflow, execution, attempt, err, attempt.StepID == "publish_pr")
		}
		attempt.ExternalBinding = run.Target
	}
	applyMinimalWorkspaceTarget(run, execution, workspace)
	result := envelope.Result
	if result.InputRevision != attempt.InputRevision {
		return s.failMinimalStep(run, workflow, execution, attempt, errors.New("result input revision differs from the fixed attempt input"), attempt.StepID == "publish_pr")
	}
	if result.StepID == "implement" {
		if result.Revision != workspace.Head || workspace.Dirty {
			return s.failMinimalStep(run, workflow, execution, attempt, errors.New("implement must leave a clean workspace at its candidate commit"), false)
		}
		if run.Protocol != string(ProtocolOrcaSupervised) {
			if _, err = gitRunner(s.Config, *run.WorktreePath).Output(context.Background(), s.Config.Git, "merge-base", "--is-ancestor", workspace.Base, workspace.Head); err != nil {
				return s.failMinimalStep(run, workflow, execution, attempt, fmt.Errorf("candidate commit is not based on the pinned workspace base: %w", err), false)
			}
		}
	}
	var publishedInfo *PRInfo
	if result.StepID == "publish_pr" {
		if _, err = s.Config.PR.PRNumber(result.PRURL); err != nil {
			return s.failMinimalStep(run, workflow, execution, attempt, fmt.Errorf("publish_pr returned an invalid PR URL: %w", err), true)
		}
		info, queryErr := s.prs.Query(context.Background(), result.PRURL)
		if queryErr != nil {
			return s.failMinimalStep(run, workflow, execution, attempt, fmt.Errorf("PR publication requires reconciliation: %w", queryErr), true)
		}
		if info.BaseBranch != s.Config.Branch || info.HeadRevision != execution.CandidateRevision || result.PublishedHead != info.HeadRevision {
			return s.failMinimalStep(run, workflow, execution, attempt, errors.New("published PR target or head does not match the verified candidate"), true)
		}
		publishedInfo = &info
	}
	result.Workspace = workspace
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return err
	}
	attempt.ResultID = result.ResultID
	attempt.Result = resultBytes
	next, gateErr := AdvanceMinimalExecution(workflow, execution, result, workflow.ProjectSteps.Verify.RequiredChecks)
	if gateErr != nil {
		return s.failMinimalStep(run, workflow, execution, attempt, gateErr, false)
	}
	applyMinimalWorkspaceEvidence(run, execution, workspace)
	if err = TransitionStepAttempt(attempt, "succeeded"); err != nil {
		return err
	}
	if err = s.recordMinimalDecisionApplied(run, result); err != nil {
		return err
	}
	if next != "waiting_pr_merge" {
		execution.CurrentStep = next
	}
	execution.Process, run.Process = nil, nil
	workflow.UpdatedAt = now()
	execution.UpdatedAt = now()
	run.ExecutionStatus = "pending"
	run.Process = nil
	run.StartedAt = nil
	run.Error = nil
	run.StepResultPath = nil
	run.FinishTaskStatus = "pending"
	s.note(run, "steer", "step_result", "completed", "minimal-v1 step accepted", attempt.StepID, map[string]any{"attempt_id": attempt.AttemptID, "result_id": result.ResultID, "revision": result.Revision, "next_step": next})
	if next == "waiting_pr_merge" {
		run.ExecutionStatus = "succeeded"
		run.FinishTaskStatus = "waiting_pr_merge"
		run.PR.URL = ptr(result.PRURL)
		run.PR.State = "unknown"
		run.PR.QueryError = nil
		workflow.SelectedResult = execution.ExecutionID
		if publishedInfo != nil {
			run.PR.State = publishedInfo.State
			run.PR.MergedAt = publishedInfo.MergedAt
			run.PR.MergeCommit = publishedInfo.MergeCommit
		}
		run.Error = nil
		s.closeActivity(run)
		return s.save()
	}
	if next == "blocked" {
		return s.failMinimalStep(run, workflow, execution, attempt, errors.New("workflow reached a blocked gate"), false)
	}
	prerequisites := []string{result.ResultID}
	if next == "implement" && execution.RepairRounds > 0 {
		prerequisites = append([]string(nil), execution.RepairPrerequisiteIDs...)
	}
	newAttempt := s.makeMinimalAttempt(run, execution, workflow, next, prerequisites)
	s.State.Attempts = append(s.State.Attempts, newAttempt)
	run.AttemptID = newAttempt.AttemptID
	run.DomainAttemptNumber = newAttempt.AttemptNumber
	return s.save()
}

// A settled failed step is a task outcome, not a service failure. Keep the
// cause visible to direct callers while the service loop continues polling.
type settledMinimalStepFailure struct{ cause error }

func (e *settledMinimalStepFailure) Error() string { return e.cause.Error() }
func (e *settledMinimalStepFailure) Unwrap() error { return e.cause }

// preservedMinimalStepUnknown means the uncertain outcome and its original
// execution binding have already been saved. It blocks that Run, not the
// service's observers for other independent Runs.
type preservedMinimalStepUnknown struct{ cause error }

func (e *preservedMinimalStepUnknown) Error() string { return e.cause.Error() }
func (e *preservedMinimalStepUnknown) Unwrap() error { return e.cause }

func (s *Service) failMinimalStep(run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, attempt *StepAttemptRecord, cause error, unknown bool) error {
	status := "failed"
	if unknown {
		status = "outcome_unknown"
		execution.Status = "blocked"
		run.ExecutionStatus = "unknown"
	} else {
		execution.Status = "failed"
		run.ExecutionStatus = "failed"
	}
	if attempt.Status == "running" || attempt.Status == "dispatched" || attempt.Status == "dispatching" {
		_ = TransitionStepAttempt(attempt, status)
	}
	if !unknown {
		execution.Process, run.Process = nil, nil
	}
	execution.UpdatedAt = now()
	workflow.Phase = "blocked"
	workflow.UpdatedAt = now()
	run.FinishTaskStatus = "blocked"
	if unknown {
		run.EndedAt = nil
	} else {
		run.EndedAt = ptr(now())
	}
	run.Error = ptr(clip(cause.Error(), 2000))
	if !unknown {
		run.Process = nil
	}
	run.StepResultPath = nil
	if unknown {
		s.State.CurrentTask = ptr(run.TaskID)
		s.State.SlotOccupied = true
	} else {
		s.State.CurrentTask = nil
		s.State.SlotOccupied = false
	}
	s.note(run, "steer", "step_result", status, "minimal-v1 step blocked", attempt.StepID, map[string]any{"attempt_id": attempt.AttemptID, "outcome_unknown": unknown, "reason": cause.Error()})
	s.closeActivity(run)
	if err := s.save(); err != nil {
		return errors.Join(cause, err)
	}
	if unknown {
		return &preservedMinimalStepUnknown{cause: cause}
	}
	return &settledMinimalStepFailure{cause: cause}
}

func validateMinimalWorkspaceReceipt(receipt, actual WorkspaceEvidence) error {
	if receipt.Repository != actual.Repository {
		return fmt.Errorf("step workspace repository %q differs from center inspection %q", receipt.Repository, actual.Repository)
	}
	if canonical(receipt.Worktree) != canonical(actual.Worktree) {
		return fmt.Errorf("step workspace path %q differs from center inspection %q", receipt.Worktree, actual.Worktree)
	}
	if receipt.Branch != actual.Branch {
		return fmt.Errorf("step workspace branch %q differs from center inspection %q", receipt.Branch, actual.Branch)
	}
	if receipt.Base != actual.Base {
		return fmt.Errorf("step workspace base %q differs from center inspection %q", receipt.Base, actual.Base)
	}
	if receipt.Head != actual.Head {
		return fmt.Errorf("step workspace HEAD %q differs from center inspection %q", receipt.Head, actual.Head)
	}
	if receipt.Dirty != actual.Dirty {
		return fmt.Errorf("step workspace dirty=%t differs from center inspection dirty=%t", receipt.Dirty, actual.Dirty)
	}
	return nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func confirmMinimalAttemptRan(attempt *StepAttemptRecord) error {
	if attempt == nil {
		return errors.New("minimal step attempt is missing")
	}
	if attempt.Status == "dispatching" {
		if err := TransitionStepAttempt(attempt, "dispatched"); err != nil {
			return err
		}
	}
	if attempt.Status == "dispatched" {
		if err := TransitionStepAttempt(attempt, "running"); err != nil {
			return err
		}
	}
	if attempt.Status != "running" {
		return fmt.Errorf("finished attempt has unexpected status %q", attempt.Status)
	}
	return nil
}

func (s *Service) waitMinimalDecision(run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, attempt *StepAttemptRecord, result StepGateResult) error {
	if result.Decision == nil {
		return errors.New("decision request is missing")
	}
	if err := validateDecisionRequest(result.Decision); err != nil {
		return err
	}
	workspace, err := s.inspectMinimalWorkspace(context.Background(), run)
	if err != nil {
		return fmt.Errorf("cannot checkpoint workspace before waiting for decision: %w", err)
	}
	if workspace.Repository != execution.Workspace.Repository || workspace.Worktree != execution.Workspace.Worktree || workspace.Base != execution.Workspace.Base || workspace.Branch != execution.Workspace.Branch {
		return errors.New("workspace identity changed before waiting for decision")
	}
	if attempt.StepID != "implement" && (workspace.Dirty || workspace.Head != execution.CandidateRevision) {
		return errors.New("step changed the verified workspace before requesting a decision")
	}
	execution.Workspace = workspace
	state, err := WriteDecisionRequest(run, *result.Decision)
	if err != nil {
		return err
	}
	state.AttemptID = attempt.AttemptID
	attempt.ResultID = result.ResultID
	payload, _ := json.Marshal(result)
	attempt.Result = payload
	if err = SetWorkflowDecision(workflow, execution, attempt, "pending"); err != nil {
		return err
	}
	run.Decision = state
	run.FinishTaskStatus = "waiting_decision"
	run.ExecutionStatus = "succeeded"
	run.Error = nil
	run.Process = nil
	run.EndedAt = ptr(now())
	run.StepResultPath = nil
	request, _ := json.Marshal(state.Request)
	s.State.Decisions = append(s.State.Decisions, DecisionRecord{ProjectIdentity: s.Config.identity, DecisionID: state.ID, Scope: "execution", WorkflowRunID: run.RunID, ExecutionID: execution.ExecutionID, AttemptID: attempt.AttemptID, QuestionID: state.ID, Request: request, Status: "pending", Version: state.Version, CreatedAt: now(), UpdatedAt: now()})
	s.note(run, "steer", "decision", "pending", "decision requested", state.Request.Question, decisionMeta(run))
	s.closeActivity(run)
	return s.save()
}

func (s *Service) resumeMinimalAfterDecision(ctx context.Context, run *Run, answer DecisionAnswer) error {
	workflow, execution := s.minimalRecords(run)
	if workflow == nil || execution == nil || run.Decision == nil || workflow.Phase != "waiting_decision" || execution.Status != "waiting_decision" || run.Decision.Status != "resuming" {
		return errors.New("minimal decision is not durably answerable")
	}
	if answer.DecisionID != run.Decision.ID || answer.RunID != run.RunID {
		return errors.New("decision answer identity does not match the waiting workflow")
	}
	if answer.OptionID != run.Decision.OptionID || answer.Note != run.Decision.Note || answer.AnsweredBy != run.Decision.AnsweredBy {
		return errors.New("decision answer differs from the persisted command result")
	}
	var waiting *StepAttemptRecord
	for i := range s.State.Attempts {
		attempt := &s.State.Attempts[i]
		if attempt.ExecutionID == execution.ExecutionID && attempt.AttemptID == run.Decision.AttemptID {
			waiting = attempt
			break
		}
	}
	if waiting == nil || waiting.StepID != execution.CurrentStep || waiting.Status != "waiting_decision" {
		return errors.New("minimal decision no longer matches its step attempt")
	}
	workspace, err := s.inspectMinimalWorkspace(ctx, run)
	if err != nil {
		return fmt.Errorf("cannot inspect workspace before decision resume: %w", err)
	}
	if workspace.Repository != execution.Workspace.Repository || workspace.Worktree != execution.Workspace.Worktree || workspace.Base != execution.Workspace.Base || workspace.Branch != execution.Workspace.Branch {
		return errors.New("workspace identity changed while waiting for decision")
	}
	if waiting.StepID != "implement" && (workspace.Dirty || workspace.Head != execution.Workspace.Head) {
		return errors.New("non-implement workspace changed while waiting for decision")
	}
	execution.Workspace = workspace
	prerequisites := append([]string(nil), waiting.PrerequisiteIDs...)
	if waiting.ResultID != "" {
		prerequisites = append(prerequisites, waiting.ResultID)
	}
	if err := TransitionStepAttempt(waiting, "succeeded"); err != nil {
		return err
	}
	resumed := s.makeMinimalAttempt(run, execution, workflow, waiting.StepID, prerequisites)
	s.State.Attempts = append(s.State.Attempts, resumed)
	run.AttemptID = resumed.AttemptID
	run.DomainAttemptNumber = resumed.AttemptNumber
	run.ExecutionStatus = "pending"
	run.FinishTaskStatus = "pending"
	run.EndedAt = nil
	run.Error = nil
	run.StepResultPath = nil
	execution.Status = "pending"
	workflow.Phase = "executing"
	workflow.UpdatedAt = now()
	execution.UpdatedAt = now()
	if err := s.save(); err != nil {
		return err
	}
	page, err := (QueryService{Store: s.domainStore, Project: s.Config.identity}).ShowNativeTask(ctx, run.DomainTaskID)
	if err != nil {
		return err
	}
	if len(page.Tasks) != 1 || page.Tasks[0].Status != "in_progress" || page.Tasks[0].Claimant != run.Claimant {
		return nativeErr("stale_version", "native task status or claimant changed before decision continuation")
	}
	task := page.Tasks[0].TaskRecord
	resumedAttempt := s.minimalAttempt(run, resumed.AttemptID)
	if err = s.startMinimalStep(ctx, task, run, resumedAttempt); err != nil {
		if run.ExecutionStatus != "unknown" {
			run.Decision.Status = "failed"
			run.Decision.Version++
		}
		_ = s.syncMinimalDecision(run)
		_ = s.save()
		return err
	}
	workflow, execution = s.minimalRecords(run)
	resumedAttempt = s.minimalAttempt(run, resumed.AttemptID)
	if workflow == nil || execution == nil || resumedAttempt == nil {
		return errors.New("minimal decision resume records disappeared after worker start")
	}
	return nil
}

func (s *Service) recordMinimalDecisionApplied(run *Run, result StepGateResult) error {
	if run.Decision == nil || run.Decision.Status != "reply_accepted" {
		return nil
	}
	if !minimalDecisionApplicationMatches(run, result) {
		return errors.New("decision application evidence does not match the fixed decision, attempt, execution and answer")
	}
	run.Decision.Status = "applied"
	run.Decision.Version++
	run.Decision.AppliedAt = ptr(now())
	if err := s.syncMinimalDecision(run); err != nil {
		return err
	}
	s.note(run, "steer", "decision", "applied", "decision applied", decisionAnswerSummary(run), nil)
	return nil
}

func (s *Service) failMinimalDecisionApplication(run *Run, reason string) error {
	if run == nil || run.Decision == nil || run.Decision.Status != "reply_accepted" {
		return nil
	}
	run.Decision.Status = "failed"
	run.Decision.Version++
	if err := s.syncMinimalDecision(run); err != nil {
		return err
	}
	s.note(run, "steer", "decision", "failed", "decision application was not verified", reason, decisionAnswerMeta(run))
	return nil
}

func minimalDecisionApplicationMatches(run *Run, result StepGateResult) bool {
	if run == nil || run.Decision == nil || run.Decision.Status != "reply_accepted" {
		return false
	}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: run.Decision.OptionID, Note: run.Decision.Note, AnsweredBy: run.Decision.AnsweredBy}
	return result.AppliedDecisionID == run.Decision.ID && result.AppliedDecisionAttemptID == run.Decision.AttemptID && result.AppliedDecisionExecutionID == run.ExecutionID && result.AppliedAnswerSHA256 == decisionAnswerDigest(answer)
}

func (s *Service) syncMinimalDecision(run *Run) error {
	if run == nil || run.Decision == nil {
		return errors.New("minimal decision record is required")
	}
	request, err := json.Marshal(run.Decision.Request)
	if err != nil {
		return err
	}
	var answer []byte
	if run.Decision.Status == "answer_saved" || run.Decision.Status == "resuming" || run.Decision.Status == "reply_accepted" || run.Decision.Status == "applied" || run.Decision.Status == "failed" {
		value := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: run.Decision.OptionID, Note: run.Decision.Note, AnsweredBy: run.Decision.AnsweredBy}
		if run.Decision.AnsweredAt != nil {
			value.AnsweredAt = *run.Decision.AnsweredAt
		}
		answer, err = json.Marshal(value)
		if err != nil {
			return err
		}
	}
	status := run.Decision.Status
	created := now()
	for i := range s.State.Decisions {
		if s.State.Decisions[i].DecisionID == run.Decision.ID {
			created = s.State.Decisions[i].CreatedAt
			s.State.Decisions[i] = DecisionRecord{ProjectIdentity: s.Config.identity, DecisionID: run.Decision.ID, Scope: "execution", WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.Decision.AttemptID, QuestionID: run.Decision.ID, Request: request, Answer: answer, AnsweredBy: run.Decision.AnsweredBy, Status: status, Version: run.Decision.Version, CreatedAt: created, UpdatedAt: now()}
			return nil
		}
	}
	s.State.Decisions = append(s.State.Decisions, DecisionRecord{ProjectIdentity: s.Config.identity, DecisionID: run.Decision.ID, Scope: "execution", WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.Decision.AttemptID, QuestionID: run.Decision.ID, Request: request, Answer: answer, AnsweredBy: run.Decision.AnsweredBy, Status: status, Version: run.Decision.Version, CreatedAt: created, UpdatedAt: now()})
	return nil
}

func (s *Service) blockMinimalPreparation(run *Run, attempt *StepAttemptRecord, operationID string, cause error) error {
	return s.blockMinimalPreparationWithReceipt(run, attempt, operationID, "", nil, cause)
}

func (s *Service) blockMinimalPreparationWithReceipt(run *Run, attempt *StepAttemptRecord, operationID, platformRequestID string, receipt []byte, cause error) error {
	if attempt.Status == "running" || attempt.Status == "dispatched" || attempt.Status == "dispatching" {
		_ = TransitionStepAttempt(attempt, "outcome_unknown")
	}
	if execution := s.executionRecord(run); execution != nil {
		execution.Status = "blocked"
		execution.UpdatedAt = now()
	}
	if workflow, _ := s.minimalRecords(run); workflow != nil {
		workflow.Phase = "blocked"
		workflow.UpdatedAt = now()
	}
	run.ExecutionStatus = "unknown"
	run.FinishTaskStatus = "blocked"
	run.Error = ptr(clip("workspace preparation outcome requires reconciliation: "+cause.Error(), 2000))
	s.State.CurrentTask = ptr(run.TaskID)
	s.State.SlotOccupied = true
	_ = s.setOperationState(operationID, "outcome_unknown", platformRequestID, receipt, cause.Error())
	s.note(run, "steer", "prepare", "unknown", "minimal-v1 preparation requires reconciliation", cause.Error(), nil)
	return errors.Join(cause, s.save())
}

func (s *Service) executionRecord(run *Run) *ExecutionRecord {
	_, execution := s.minimalRecords(run)
	return execution
}

func (s *Service) makeMinimalAttempt(run *Run, execution *ExecutionRecord, workflow *WorkflowRunRecord, step string, prerequisites []string) StepAttemptRecord {
	number := nextStepAttemptNumber(s.State.Attempts, execution.ExecutionID, step)
	contract := minimalStepContract(workflow.ProjectSteps, step)
	data, _ := json.Marshal(contract)
	inputRevision := execution.Workspace.Head
	if inputRevision == "" {
		inputRevision = execution.Workspace.Base
	}
	if step != "implement" {
		inputRevision = execution.CandidateRevision
	}
	return StepAttemptRecord{ProjectIdentity: s.Config.identity, AttemptID: newID(), ExecutionID: execution.ExecutionID, StepID: step, AttemptNumber: number, Status: "planned", InputRevision: inputRevision, ContractDigest: operationDigest(data), PrerequisiteIDs: append([]string(nil), prerequisites...), CreatedAt: now(), UpdatedAt: now()}
}

func (s *Service) startMinimalStep(ctx context.Context, task TaskRecord, run *Run, attempt *StepAttemptRecord) error {
	workflow, execution := s.minimalRecords(run)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("minimal step records are incomplete")
	}
	if attempt.Status != "planned" {
		return fmt.Errorf("step attempt %s is not planned", attempt.AttemptID)
	}
	if err := s.requireSettledSupervisedReleases(run); err != nil {
		return err
	}
	var err error
	contractBytes := []byte("prepare/v1")
	if attempt.StepID != "prepare" {
		contractBytes, err = json.Marshal(minimalStepContract(workflow.ProjectSteps, attempt.StepID))
		if err != nil {
			return err
		}
	}
	attemptContractMatches := operationDigest(contractBytes) == attempt.ContractDigest
	if attempt.StepID == "prepare" {
		taskRecord := nativeTaskRecord(s.State.Tasks, run.DomainTaskID)
		if taskRecord == nil {
			return nativeErr("contract_mismatch", "native prepare has no fixed task contract")
		}
		taskContract, contractErr := nativeContractForTask(ctx, s.domainStore, s.Config.identity, *taskRecord)
		if contractErr != nil {
			return contractErr
		}
		attemptContractMatches = attempt.ContractDigest == taskContract.Digest && taskRecord.ContractID == taskContract.ContractID
	}
	if workflow.ConfigDigest == "" || workflow.ConfigDigest != run.WorkflowConfigDigest || !attemptContractMatches {
		return errors.New("step contract differs from the pinned workflow or attempt")
	}
	if execution.CurrentStep != attempt.StepID {
		return fmt.Errorf("planned attempt %s for %s does not match execution current step %s", attempt.AttemptID, attempt.StepID, execution.CurrentStep)
	}
	if attempt.StepID == "review" || attempt.StepID == "publish_pr" {
		if !revisionPattern.MatchString(execution.CandidateRevision) || execution.VerifiedRevision != execution.CandidateRevision {
			return errors.New("step requires verification of the pinned candidate before dispatch")
		}
		if attempt.StepID == "publish_pr" && workflow.Definition.ReviewRequired && execution.ReviewedRevision != execution.CandidateRevision {
			return errors.New("publish_pr requires review of the pinned candidate before dispatch")
		}
	}
	workspace := WorkspaceEvidence{}
	remotePrepare := s.Config.hostName() == "orca_supervised" && attempt.StepID == "prepare"
	if !remotePrepare {
		workspace, err = s.inspectMinimalWorkspace(ctx, run)
		if err != nil {
			return err
		}
		expectedRevision := execution.CandidateRevision
		allowDirtyResume := run.Decision != nil && (run.Decision.Status == "answer_saved" || run.Decision.Status == "resuming") && attempt.StepID == "implement"
		if attempt.StepID == "implement" {
			expectedRevision = execution.Workspace.Head
			if expectedRevision == "" {
				expectedRevision = execution.Workspace.Base
			}
		}
		if workspace.Head != expectedRevision || (workspace.Dirty && !allowDirtyResume) {
			return fmt.Errorf("step %s preflight requires clean revision %s, found head=%s dirty=%t", attempt.StepID, expectedRevision, workspace.Head, workspace.Dirty)
		}
	}
	dir, events, _, resultPath := minimalAttemptPaths(s.Config, run, attempt)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	attempt.EventPath = events
	attempt.ResultPath = resultPath
	run.StepResultPath = ptr(resultPath)
	run.AttemptID = attempt.AttemptID
	run.DomainAttemptNumber = attempt.AttemptNumber
	run.EndedAt = nil
	run.Error = nil
	run.FinishTaskStatus = "pending"
	input := buildMinimalStepInput(s.Config.identity, task, run, workflow, execution, attempt, s.State.Attempts...)
	if len(input.PrerequisiteResults) != len(attempt.PrerequisiteIDs) {
		return errors.New("step is missing its pinned prerequisite result bodies")
	}
	workerInput, err := s.minimalWorkerInput(ctx, task, run, workflow, execution, attempt, input)
	if err != nil {
		return err
	}
	execution.CurrentStep = attempt.StepID
	execution.UpdatedAt = now()
	opID, err := s.minimalOperation(run, attempt, "dispatch_step", "step:"+execution.ExecutionID+":"+attempt.AttemptID, map[string]any{"task_id": run.DomainTaskID, "workflow_run_id": run.RunID, "execution_id": execution.ExecutionID, "attempt_id": attempt.AttemptID, "step_id": attempt.StepID, "input": workerInput, "contract_digest": attempt.ContractDigest})
	if err != nil {
		return err
	}
	workflow, execution = s.minimalRecords(run)
	attempt = s.minimalAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("minimal step records disappeared after operation persistence")
	}
	if attempt.Status != "dispatching" {
		return fmt.Errorf("minimal step attempt %s changed to %q after dispatch intent persistence", attempt.AttemptID, attempt.Status)
	}
	var worker *Worker
	var ref ExecutionRef
	platformRequestID := ""
	if s.Config.hostName() == "orca_supervised" {
		if createErr := s.ensureSupervisedRun(ctx, run); createErr != nil {
			receipt := mustJSON(map[string]any{"target": run.Target, "operation": "create_orca_run"})
			blockErr := s.blockMinimalPreparationWithReceipt(run, attempt, opID, run.Target.OrchestrationRunRequestID, receipt, createErr)
			return errors.Join(blockErr, createErr)
		}
	}
	if s.Config.hostName() == "orca_supervised" {
		started, startErr := (orcaSupervisedBackend{config: s.Config}).StartMinimalStep(ctx, task, *run, *workflow, *execution, *attempt, input, workerInput, resultPath)
		platformRequestID = started.PlatformRequestID
		if startErr != nil {
			var backendErr *BackendError
			if errors.As(startErr, &backendErr) && (backendErr.Code == BackendOutcomeUnknown || backendErr.Code == BackendUnreachable || backendErr.Code == BackendIdentityMismatch || backendErr.Code == BackendProtocolInvalid) {
				if started.Run.RunID == run.RunID {
					*run = started.Run
				}
				if started.Ref.External.Kind == "orca_supervised" {
					run.Process = &started.Ref.External
					execution.Process = &started.Ref.External
					execution.Target = started.Ref.Target
					attempt.ExternalBinding = started.Ref.Target
					if bindErr := (executionRecords{root: s.Config.RunDir}).saveBinding(started.Ref); bindErr != nil {
						startErr = errors.Join(startErr, bindErr)
					}
				}
				receipt := mustJSON(map[string]any{"ref": started.Ref, "target": run.Target, "residual_resources": started.ResidualResources})
				blockErr := s.blockMinimalPreparationWithReceipt(run, attempt, opID, platformRequestID, receipt, startErr)
				return errors.Join(blockErr, startErr)
			}
			err = startErr
		} else if started.Disposition != "accepted" || started.Worker == nil {
			return s.blockMinimalPreparation(run, attempt, opID, errors.New("supervised worker start returned no accepted worker"))
		} else {
			*run = started.Run
			worker, ref = started.Worker, started.Ref
			verifiedHead := attempt.InputRevision
			if attempt.StepID == "prepare" {
				verifiedHead = deref(run.BaseCommit)
			}
			workspace = WorkspaceEvidence{Repository: s.Config.identity, Worktree: canonical(deref(run.WorktreePath)), Branch: deref(run.WorktreeBranch), Base: deref(run.BaseCommit), Head: verifiedHead, Dirty: false, VerifiedAt: now()}
			execution.Target = ref.Target
			execution.Workspace = workspace
			attempt.ExternalBinding = ref.Target
		}
	} else {
		worker, err = s.startMinimalStepWorker(ctx, task, run, workflow, execution, attempt, workerInput)
		workflow, execution = s.minimalRecords(run)
		attempt = s.minimalAttempt(run, run.AttemptID)
		if workflow == nil || execution == nil || attempt == nil {
			return nativeErr("inconsistent_delivery", "minimal step records disappeared after worker admission")
		}
		if err != nil {
			var backendErr *BackendError
			if errors.As(err, &backendErr) && (backendErr.Code == BackendOutcomeUnknown || backendErr.Code == BackendUnreachable || backendErr.Code == BackendIdentityMismatch) {
				_ = s.setOperationState(opID, "outcome_unknown", "", nil, err.Error())
				if attempt.Status == "dispatching" {
					_ = TransitionStepAttempt(attempt, "outcome_unknown")
				}
				execution.Status = "blocked"
				execution.CurrentStep = attempt.StepID
				execution.UpdatedAt = now()
				workflow.Phase = "blocked"
				run.ExecutionStatus = "unknown"
				run.FinishTaskStatus = "blocked"
				run.EndedAt = nil
				run.Error = ptr(clip("step start outcome requires reconciliation: "+err.Error(), 2000))
				s.State.CurrentTask = ptr(run.TaskID)
				s.State.SlotOccupied = true
				s.note(run, "steer", "step_start", "unknown", "native step start requires reconciliation", attempt.AttemptID, map[string]any{"step": attempt.StepID, "backend": run.Backend, "protocol": run.Protocol})
				return errors.Join(err, s.save())
			}
		} else {
			identity := worker.proc.Identity()
			generation := identity.Created
			if identity.Kind == "orca" {
				generation = identity.Incarnation
			}
			ref = ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendKind(run.Backend), Protocol: ExecutionProtocol(run.Protocol), Target: targetFor(s.Config), Agent: agentFor(s.Config), RunID: run.RunID, AttemptID: attempt.AttemptID, WorkspacePath: deref(run.WorktreePath), WorkspaceBranch: deref(run.WorktreeBranch), BaseCommit: deref(run.BaseCommit), External: identity, Generation: generation}
		}
	}
	if err != nil {
		_ = s.setOperationState(opID, "rejected", platformRequestID, nil, err.Error())
		_ = TransitionStepAttempt(attempt, "rejected")
		execution.Status = "failed"
		execution.CurrentStep = attempt.StepID
		execution.UpdatedAt = now()
		workflow.Phase = "blocked"
		run.ExecutionStatus = "failed"
		run.FinishTaskStatus = "blocked"
		run.EndedAt = ptr(now())
		run.Error = ptr(clip("step dispatch failed: "+err.Error(), 2000))
		s.State.CurrentTask = nil
		s.State.SlotOccupied = false
		s.note(run, "steer", "error", "failed", "minimal-v1 step dispatch failed", err.Error(), map[string]any{"step": attempt.StepID})
		if saveErr := s.save(); saveErr != nil {
			return saveErr
		}
		return err
	}
	s.worker = worker
	s.active = run
	s.registerWorker(run, worker)
	identity := worker.proc.Identity()
	run.Process = &identity
	run.StartedAt = ptr(now())
	run.ExecutionStatus = "running"
	if s.Config.hostName() == "orca_supervised" {
		run.ExecutionStatus = "pending"
	}
	execution.Process = &identity
	execution.Status = "active"
	execution.CurrentStep = attempt.StepID
	execution.UpdatedAt = now()
	if err = TransitionStepAttempt(attempt, "dispatched"); err == nil && s.Config.hostName() != "orca_supervised" {
		err = TransitionStepAttempt(attempt, "running")
	}
	if err != nil {
		return err
	}
	if err = (executionRecords{root: s.Config.RunDir}).saveBinding(ref); err != nil {
		run.ExecutionStatus = "unknown"
		s.blocked(run, "Step started but its binding could not be persisted: "+err.Error())
		s.State.SlotOccupied, s.State.CurrentTask = true, ptr(run.TaskID)
		return errors.Join(err, s.save())
	}
	if err = s.setOperationState(opID, "confirmed", platformRequestID, mustJSON(identity), ""); err != nil {
		return err
	}
	s.State.CurrentTask = ptr(run.TaskID)
	s.State.SlotOccupied = true
	startStatus, startTitle := "started", "minimal-v1 step started"
	if s.Config.hostName() == "orca_supervised" {
		startStatus, startTitle = "info", "minimal-v1 step input accepted"
	}
	s.note(run, "steer", "step_start", startStatus, startTitle, attempt.StepID, map[string]any{"step": attempt.StepID, "attempt_id": attempt.AttemptID, "backend": run.Backend, "protocol": run.Protocol})
	if input.Decision != nil && (run.Decision.Status == "answer_saved" || run.Decision.Status == "resuming") {
		run.Decision.Status = "reply_accepted"
		run.Decision.Version++
		if err = s.syncMinimalDecision(run); err != nil {
			return err
		}
		s.note(run, "steer", "decision", "reply_accepted", "decision reply accepted", decisionAnswerSummary(run), nil)
	}
	if err = s.save(); err != nil {
		return err
	}
	if run.Process != nil && run.Process.Kind == "orca" && s.Config.hostName() == "orca" {
		// Terminal creation and its exact binding are durable before asking Orca
		// to show the tab. UI focus is independent of worker admission.
		if focusErr := (orcaClient{cfg: s.Config}).terminalSwitch(context.Background(), run.Process.Terminal); focusErr != nil {
			s.note(run, "steer", "step_start", "info", "Orca terminal started but could not be shown automatically", focusErr.Error(), map[string]any{"terminal": run.Process.Terminal})
			_ = s.save()
		}
	}
	return nil
}

func (s *Service) resumeMinimalSteps(ctx context.Context) error {
	if s.State.Control.Operation != nil {
		return nil
	}
	for _, run := range s.State.Runs {
		if run == nil || run.WorkflowVersion != MinimalWorkflowVersion {
			continue
		}
		if s.workers[run.RunID] != nil {
			continue
		}
		blocked := false
		for _, blocker := range s.State.Control.Blockers {
			if blocker.RunID == run.RunID || blocker.TaskID == run.TaskID {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		workflow, execution := s.minimalRecords(run)
		if workflow == nil || execution == nil || workflow.Phase != "executing" || run.FinishTaskStatus == "blocked" {
			continue
		}
		var current *StepAttemptRecord
		for i := range s.State.Attempts {
			attempt := &s.State.Attempts[i]
			if attempt.ExecutionID == execution.ExecutionID && attempt.StepID == execution.CurrentStep && attempt.Status == "planned" && (current == nil || attempt.AttemptNumber > current.AttemptNumber) {
				current = attempt
			}
		}
		if current == nil {
			continue
		}
		if err := s.useFixedRunProfile(run); err != nil {
			return err
		}
		task := nativeTaskRecord(s.State.Tasks, run.DomainTaskID)
		if task == nil || task.Status != "in_progress" || task.Claimant != run.Claimant {
			return nativeErr("stale_version", "native task status or claimant changed before planned step dispatch")
		}
		if current.StepID == "prepare" {
			return s.prepareMinimalExecution(ctx, *task, run)
		}
		return s.startMinimalStep(ctx, *task, run, current)
	}
	return nil
}

func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }
