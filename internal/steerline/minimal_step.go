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
	"strings"
)

const (
	MinimalStepInputSchema  = "steerline/minimal-step-input/v1"
	MinimalStepOutputSchema = "steerline/minimal-step-result/v1"
	minimalStepOutputMax    = 2 << 20
)

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
	Task                  Task                         `json:"task"`
	Decision              *MinimalDecisionContinuation `json:"decision,omitempty"`
}

type MinimalDecisionContinuation struct {
	Request DecisionRequest `json:"request"`
	Answer  DecisionAnswer  `json:"answer"`
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
	steps := []StepContract{workflow.Steps.Implement, workflow.Steps.Verify, workflow.Steps.PublishPR}
	if workflow.Definition.ReviewRequired {
		steps = append(steps, workflow.Steps.Review)
	}
	for _, step := range steps {
		if strings.HasPrefix(step.Entry, "builtin:") {
			continue
		}
		output, err := gitRunner(c, c.Repository).Output(ctx, c.Git, "ls-tree", base, "--", filepath.ToSlash(step.Entry))
		if err != nil {
			return err
		}
		fields := strings.Fields(string(output))
		if len(fields) < 3 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" {
			return fmt.Errorf("step entry %q must be a committed regular file at the pinned base", step.Entry)
		}
	}
	return nil
}

func InitializeMinimalWorkflow(project string, c Config, task Task, source BeadsContext, run *Run) (TaskRecord, WorkflowRunRecord, ExecutionRecord, StepAttemptRecord, error) {
	workflow, err := c.minimalConfig()
	if err != nil {
		return TaskRecord{}, WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, err
	}
	if workflow == nil {
		return TaskRecord{}, WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, errors.New("minimal workflow is not configured")
	}
	if run == nil || run.DomainTaskID == "" || run.RunID == "" || run.Backend == "" || run.Protocol == "" {
		return TaskRecord{}, WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, errors.New("minimal workflow requires stable task, run, backend and protocol identities")
	}
	runner := StepRunnerSettings{Codex: c.Codex, Sandbox: c.Sandbox, Model: c.Model, ReasoningEffort: c.ReasoningEffort, Profile: c.Profile}
	runner.Codex.Args = append([]string(nil), c.Codex.Args...)
	definitionBytes, err := json.Marshal(struct {
		Workflow *MinimalWorkflowConfig
		Runner   StepRunnerSettings
	}{workflow, runner})
	if err != nil {
		return TaskRecord{}, WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, err
	}
	digest := operationDigest(definitionBytes)
	created := now()
	taskRecord := TaskRecord{ProjectIdentity: project, TaskID: run.DomainTaskID, ExternalProvider: "beads", ExternalProject: source.ProjectID, ExternalID: task.ID, IdentityMapDigest: run.IdentityMapDigest, Title: task.Title, Claimant: run.Claimant, Source: "beads", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
	wfr := WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: run.RunID, TaskID: taskRecord.TaskID, WorkflowVersion: MinimalWorkflowVersion, ConfigDigest: digest, Definition: workflow.Definition, ProjectSteps: workflow.Steps, Phase: "queued", Repository: c.Repository, Branch: c.Branch, CreatedAt: created, UpdatedAt: created}
	wfr.Runner = runner
	executionID := newID()
	run.ExecutionID = executionID
	run.WorkflowVersion = MinimalWorkflowVersion
	run.WorkflowConfigDigest = digest
	target := run.Target
	target.BaseRevision = deref(run.BaseCommit)
	if target.BaseRevision == "" && run.WorkspaceIntent != nil {
		target.BaseRevision = run.WorkspaceIntent.BaseCommit
	}
	target.Branch = c.Branch
	target.WorktreeID = deref(run.WorktreePath)
	execution := ExecutionRecord{ProjectIdentity: project, ExecutionID: executionID, WorkflowRunID: run.RunID, Agent: run.Agent, Backend: run.Backend, Protocol: run.Protocol, Target: target, Status: "pending", CurrentStep: "prepare", CreatedAt: created, UpdatedAt: created}
	attemptID := newID()
	run.AttemptID = attemptID
	run.DomainAttemptNumber = 1
	attempt := StepAttemptRecord{ProjectIdentity: project, AttemptID: attemptID, ExecutionID: executionID, StepID: "prepare", AttemptNumber: 1, Status: "planned", InputRevision: target.BaseRevision, ContractDigest: operationDigest([]byte("prepare/v1")), CreatedAt: created, UpdatedAt: created}
	return taskRecord, wfr, execution, attempt, nil
}

func workflowDigest(value MinimalWorkflowConfig) string {
	data, _ := json.Marshal(value)
	return operationDigest(data)
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
	for i := range s.State.Attempts {
		if s.State.Attempts[i].AttemptID == attemptID && s.State.Attempts[i].ExecutionID == run.ExecutionID {
			return &s.State.Attempts[i]
		}
	}
	return nil
}

func (s *Service) inspectMinimalWorkspace(ctx context.Context, run *Run) (WorkspaceEvidence, error) {
	if run == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || run.BaseCommit == nil {
		return WorkspaceEvidence{}, errors.New("minimal execution has no recorded workspace")
	}
	worktree := *run.WorktreePath
	if run.Backend != "local_process" || run.Protocol != "local_process" {
		return WorkspaceEvidence{}, UnsupportedAdapterOperation("InspectWorkspace", "local inspection cannot inspect another execution target")
	}
	if err := verifyRunWorkspace(ctx, s.Config, run); err != nil {
		return WorkspaceEvidence{}, err
	}
	git := gitRunner(s.Config, worktree)
	head, err := git.Output(ctx, s.Config.Git, "rev-parse", "HEAD")
	if err != nil {
		return WorkspaceEvidence{}, err
	}
	branch, err := git.Output(ctx, s.Config.Git, "branch", "--show-current")
	if err != nil {
		return WorkspaceEvidence{}, err
	}
	dirty, err := git.Output(ctx, s.Config.Git, "status", "--porcelain")
	if err != nil {
		return WorkspaceEvidence{}, err
	}
	actualHead := strings.TrimSpace(string(head))
	actualBranch := strings.TrimSpace(string(branch))
	if actualBranch != *run.WorktreeBranch {
		return WorkspaceEvidence{}, errors.New("minimal workspace branch changed")
	}
	if !revisionPattern.MatchString(actualHead) {
		return WorkspaceEvidence{}, errors.New("minimal workspace returned an invalid HEAD")
	}
	return WorkspaceEvidence{Repository: s.Config.identity, Worktree: canonical(worktree), Branch: actualBranch, Base: *run.BaseCommit, Head: actualHead, Dirty: strings.TrimSpace(string(dirty)) != "", VerifiedAt: now()}, nil
}

func minimalAttemptPaths(c Config, run *Run, attempt *StepAttemptRecord) (dir, events, stderr, result string) {
	dir = filepath.Join(c.RunDir, "runs", run.RunID, "steps", attempt.AttemptID)
	return dir, filepath.Join(dir, "events.jsonl"), filepath.Join(dir, "stderr.log"), filepath.Join(dir, "result.json")
}

func buildMinimalStepInput(project string, task Task, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, attempt *StepAttemptRecord, history ...StepAttemptRecord) MinimalStepInput {
	input := MinimalStepInput{SchemaVersion: 1, ProjectIdentity: project, TaskID: run.DomainTaskID, WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, AttemptID: attempt.AttemptID, StepID: attempt.StepID, InputRevision: attempt.InputRevision, CandidateRevision: execution.CandidateRevision, PrerequisiteResultIDs: append([]string(nil), attempt.PrerequisiteIDs...), WorkflowDefinition: workflow.Definition, ProjectSteps: workflow.ProjectSteps, Workspace: execution.Workspace, Task: task}
	for _, id := range attempt.PrerequisiteIDs {
		for _, prior := range history {
			if prior.ExecutionID == execution.ExecutionID && prior.ResultID == id && len(prior.Result) > 0 {
				input.PrerequisiteResults = append(input.PrerequisiteResults, append(json.RawMessage(nil), prior.Result...))
				break
			}
		}
	}
	if run.Decision != nil && (run.Decision.Status == "answer_saved" || run.Decision.Status == "reply_accepted") {
		answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: run.Decision.OptionID, Note: run.Decision.Note, AnsweredBy: run.Decision.AnsweredBy}
		if run.Decision.AnsweredAt != nil {
			answer.AnsweredAt = *run.Decision.AnsweredAt
		}
		input.Decision = &MinimalDecisionContinuation{Request: run.Decision.Request, Answer: answer}
	}
	return input
}

func writeMinimalPrompt(path string, input MinimalStepInput, contract StepContract) error {
	data, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return err
	}
	var prompt string
	if input.Decision != nil {
		prompt = fmt.Sprintf("A human answered the blocking decision. Continue the same workflow step using the supplied request and answer. Load and follow project entry %q at version %q. Input is the following JSON. Return one JSON object matching the supplied output schema, without Markdown or extra text. Do not dispatch another step or write center state.\n\n%s\n", contract.Entry, contract.Version, string(data))
	} else {
		prompt = fmt.Sprintf("Execute only step %q for this task. Load and follow project entry %q at version %q. Input is the following JSON. Return one JSON object matching the supplied output schema, without Markdown or extra text. Do not dispatch another step or write center state.\n\n%s\n", input.StepID, contract.Entry, contract.Version, string(data))
	}
	boundaries := map[string]string{
		"implement":  "Read the specification and project instructions. Implement and test the change, then commit a clean local candidate. Do not push, publish a PR, merge or close the task.",
		"review":     "Review the verified candidate without changing code or publishing. Return passed, changes_requested or blocked with concrete evidence.",
		"publish_pr": "Publish only the verified candidate and return its precise PR URL and published head. Do not merge or close the task.",
	}
	prompt = boundaries[input.StepID] + "\n" + prompt
	if input.Decision != nil {
		prompt += "\nAfter applying the provided answer, return result.applied_decision_id equal to decision.answer.decision_id as application evidence.\n"
	}
	return AtomicWrite(path, []byte(prompt))
}

func minimalStepEnv(c Config, source BeadsContext, run *Run, attempt *StepAttemptRecord, input MinimalStepInput) map[string]string {
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

func (s *Service) startMinimalStepWorker(ctx context.Context, task Task, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, attempt *StepAttemptRecord, source BeadsContext) (*Worker, error) {
	if run.WorktreePath == nil {
		return nil, errors.New("minimal step has no worktree")
	}
	contract := minimalStepContract(workflow.ProjectSteps, attempt.StepID)
	if err := validateMinimalEntry(*run.WorktreePath, contract); err != nil {
		return nil, err
	}
	input := buildMinimalStepInput(s.Config.identity, task, run, workflow, execution, attempt, s.State.Attempts...)
	dir, events, stderr, result := minimalAttemptPaths(s.Config, run, attempt)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	run.StepResultPath = ptr(result)
	env := minimalStepEnv(s.Config, source, run, attempt, input)
	workConfig := s.Config
	workConfig.Codex, workConfig.Sandbox, workConfig.Model, workConfig.ReasoningEffort, workConfig.Profile = workflow.Runner.Codex, workflow.Runner.Sandbox, workflow.Runner.Model, workflow.Runner.ReasoningEffort, workflow.Runner.Profile
	workConfig.Repository = *run.WorktreePath
	workConfig.workerEnv = env
	if contract.Provider == "command" {
		executable, args, err := commandStepArgs(workConfig, run, attempt, input, contract, result)
		if err != nil {
			return nil, err
		}
		payload, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		workerConfig := workConfig
		workerConfig.Repository = *run.WorktreePath
		return SpawnerFor(workerConfig).Start(ctx, SpawnSpec{Dir: workerConfig.Repository, EventsPath: events, StderrPath: stderr, OutputPath: result, Command: Command{Executable: executable, Args: args}, Stdin: strings.NewReader(string(payload)), Env: mergeStepEnv(env, contract.Environment), Timeout: contract.Timeout})
	}
	if contract.Provider != "agent_skill" {
		return nil, fmt.Errorf("unsupported step provider %q", contract.Provider)
	}
	schema := filepath.Join(dir, "step-result.schema.json")
	if err := AtomicWrite(schema, MinimalStepResultSchema); err != nil {
		return nil, err
	}
	promptPath := filepath.Join(dir, "prompt.txt")
	if err := writeMinimalPrompt(promptPath, input, contract); err != nil {
		return nil, err
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return nil, err
	}
	args := append([]string{}, workConfig.Codex.Args...)
	args = append(args, "exec", "--json", "--output-schema", schema, "--output-last-message", result, "--sandbox", workConfig.Sandbox)
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
	return SpawnerFor(workConfig).Start(ctx, SpawnSpec{Dir: workConfig.Repository, EventsPath: events, StderrPath: stderr, Command: Command{Executable: workConfig.Codex.Executable, Args: args}, Stdin: strings.NewReader(string(prompt)), Env: mergeStepEnv(env, contract.Environment), Timeout: contract.Timeout})
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

func (s *Service) prepareMinimalExecution(ctx context.Context, task Task, run *Run, source BeadsContext) error {
	workflow, execution := s.minimalRecords(run)
	attempt := s.minimalAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("minimal prepare records are incomplete")
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
	execution.Workspace = workspace
	execution.Target.WorktreeID = workspace.Worktree
	execution.Target.Branch = workspace.Branch
	execution.Target.BaseRevision = workspace.Base
	execution.Workspace = workspace
	execution.UpdatedAt = now()
	gate := StepGateResult{StepID: "prepare", Succeeded: true, ResultID: resultID, Workspace: workspace}
	next, err := AdvanceMinimalExecution(workflow, execution, gate, workflow.ProjectSteps.Verify.RequiredChecks)
	if err != nil {
		return s.blockMinimalPreparation(run, attempt, opID, err)
	}
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
	return s.startMinimalStep(ctx, task, run, source, s.minimalAttempt(run, newAttempt.AttemptID))
}

func (s *Service) finishMinimalStep(exitErr error) error {
	run := s.active
	if run == nil {
		return errors.New("minimal step finished without an active run")
	}
	workflow, execution := s.minimalRecords(run)
	attempt := s.minimalAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("minimal step finished without normalized records")
	}
	s.worker = nil
	s.active = nil
	s.State.CurrentTask = nil
	s.State.SlotOccupied = false
	run.EndedAt = ptr(now())
	if exitErr != nil {
		unknown := attempt.StepID == "publish_pr"
		return s.failMinimalStep(run, workflow, execution, attempt, fmt.Errorf("worker exited: %w", exitErr), unknown)
	}
	envelope, err := readMinimalStepResult(deref(run.StepResultPath))
	if err != nil {
		return s.failMinimalStep(run, workflow, execution, attempt, fmt.Errorf("invalid step result: %w", err), attempt.StepID == "publish_pr")
	}
	if envelope.ProjectIdentity != s.Config.identity || envelope.TaskID != run.DomainTaskID || envelope.WorkflowRunID != run.RunID || envelope.ExecutionID != execution.ExecutionID || envelope.AttemptID != attempt.AttemptID || envelope.Result.StepID != attempt.StepID {
		return s.failMinimalStep(run, workflow, execution, attempt, errors.New("step result identity does not match the dispatched attempt"), attempt.StepID == "publish_pr")
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
	if run.Decision != nil && run.Decision.Status == "reply_accepted" && envelope.Result.AppliedDecisionID != run.Decision.ID {
		return s.failMinimalStep(run, workflow, execution, attempt, errors.New("step result lacks matching decision application evidence"), attempt.StepID == "publish_pr")
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
	result := envelope.Result
	if result.InputRevision != attempt.InputRevision {
		return s.failMinimalStep(run, workflow, execution, attempt, errors.New("result input revision differs from the fixed attempt input"), attempt.StepID == "publish_pr")
	}
	if result.StepID == "implement" {
		if result.Revision != workspace.Head || workspace.Dirty {
			return s.failMinimalStep(run, workflow, execution, attempt, errors.New("implement must leave a clean workspace at its candidate commit"), false)
		}
		if _, err = gitRunner(s.Config, *run.WorktreePath).Output(context.Background(), s.Config.Git, "merge-base", "--is-ancestor", workspace.Base, workspace.Head); err != nil {
			return s.failMinimalStep(run, workflow, execution, attempt, fmt.Errorf("candidate commit is not based on the pinned workspace base: %w", err), false)
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
	if attempt.Status != "running" {
		return s.failMinimalStep(run, workflow, execution, attempt, fmt.Errorf("finished attempt has unexpected status %q", attempt.Status), false)
	}
	next, gateErr := AdvanceMinimalExecution(workflow, execution, result, workflow.ProjectSteps.Verify.RequiredChecks)
	if gateErr != nil {
		return s.failMinimalStep(run, workflow, execution, attempt, gateErr, false)
	}
	if err = TransitionStepAttempt(attempt, "succeeded"); err != nil {
		return err
	}
	if err = s.recordMinimalDecisionApplied(run, result); err != nil {
		return err
	}
	if next != "waiting_pr_merge" {
		execution.CurrentStep = next
	}
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
	execution.UpdatedAt = now()
	workflow.Phase = "blocked"
	workflow.UpdatedAt = now()
	run.FinishTaskStatus = "blocked"
	run.EndedAt = ptr(now())
	run.Error = ptr(clip(cause.Error(), 2000))
	run.Process = nil
	run.StepResultPath = nil
	s.State.CurrentTask = nil
	s.State.SlotOccupied = false
	s.note(run, "steer", "step_result", status, "minimal-v1 step blocked", attempt.StepID, map[string]any{"attempt_id": attempt.AttemptID, "outcome_unknown": unknown, "reason": cause.Error()})
	s.closeActivity(run)
	return errors.Join(cause, s.save())
}

func validateMinimalWorkspaceReceipt(receipt, actual WorkspaceEvidence) error {
	if receipt.Repository != actual.Repository || canonical(receipt.Worktree) != actual.Worktree || receipt.Branch != actual.Branch || receipt.Base != actual.Base || receipt.Head != actual.Head || receipt.Dirty != actual.Dirty {
		return errors.New("step workspace receipt differs from center inspection")
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
	if workflow == nil || execution == nil || run.Decision == nil || workflow.Phase != "waiting_decision" || execution.Status != "waiting_decision" || run.Decision.Status != "answer_saved" {
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
	task, err := s.beads.Show(ctx, run.TaskID)
	if err != nil {
		return err
	}
	if task.Status != "in_progress" || task.Assignee != run.Claimant {
		return errors.New("task ownership changed before decision continuation")
	}
	source, err := s.beads.Context(ctx)
	if err != nil {
		return err
	}
	if run.BeadsContext == nil || !source.Same(*run.BeadsContext) {
		return errors.New("Beads identity changed before minimal decision resume")
	}
	resumedAttempt := s.minimalAttempt(run, resumed.AttemptID)
	if err = s.startMinimalStep(ctx, task, run, source, resumedAttempt); err != nil {
		run.Decision.Status = "failed"
		run.Decision.Version++
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
	if result.AppliedDecisionID != run.Decision.ID {
		return errors.New("decision application evidence does not match the pending reply")
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

func (s *Service) syncMinimalDecision(run *Run) error {
	if run == nil || run.Decision == nil {
		return errors.New("minimal decision record is required")
	}
	request, err := json.Marshal(run.Decision.Request)
	if err != nil {
		return err
	}
	var answer []byte
	if run.Decision.Status == "answer_saved" || run.Decision.Status == "reply_accepted" || run.Decision.Status == "applied" || run.Decision.Status == "failed" {
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
	if status == "resuming" {
		status = "answer_saved"
	}
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
	_ = s.setOperationState(operationID, "outcome_unknown", "", nil, cause.Error())
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

func (s *Service) startMinimalStep(ctx context.Context, task Task, run *Run, source BeadsContext, attempt *StepAttemptRecord) error {
	workflow, execution := s.minimalRecords(run)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("minimal step records are incomplete")
	}
	if attempt.Status != "planned" {
		return fmt.Errorf("step attempt %s is not planned", attempt.AttemptID)
	}
	contractBytes, err := json.Marshal(minimalStepContract(workflow.ProjectSteps, attempt.StepID))
	if err != nil {
		return err
	}
	if workflow.ConfigDigest == "" || workflow.ConfigDigest != run.WorkflowConfigDigest || operationDigest(contractBytes) != attempt.ContractDigest {
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
	workspace, err := s.inspectMinimalWorkspace(ctx, run)
	if err != nil {
		return err
	}
	expectedRevision := execution.CandidateRevision
	allowDirtyResume := run.Decision != nil && run.Decision.Status == "answer_saved" && attempt.StepID == "implement"
	if attempt.StepID == "implement" {
		expectedRevision = execution.Workspace.Head
		if expectedRevision == "" {
			expectedRevision = execution.Workspace.Base
		}
	}
	if workspace.Head != expectedRevision || (workspace.Dirty && !allowDirtyResume) {
		return fmt.Errorf("step %s preflight requires clean revision %s, found head=%s dirty=%t", attempt.StepID, expectedRevision, workspace.Head, workspace.Dirty)
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
	opID, err := s.minimalOperation(run, attempt, "dispatch_step", "step:"+execution.ExecutionID+":"+attempt.AttemptID, map[string]any{"task_id": run.DomainTaskID, "workflow_run_id": run.RunID, "execution_id": execution.ExecutionID, "attempt_id": attempt.AttemptID, "step_id": attempt.StepID, "input": input, "contract_digest": attempt.ContractDigest})
	if err != nil {
		return err
	}
	workflow, execution = s.minimalRecords(run)
	attempt = s.minimalAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("minimal step records disappeared after operation persistence")
	}
	worker, err := s.startMinimalStepWorker(ctx, task, run, workflow, execution, attempt, source)
	if err != nil {
		_ = s.setOperationState(opID, "rejected", "", nil, err.Error())
		_ = TransitionStepAttempt(attempt, "rejected")
		execution.Status = "failed"
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
	identity := worker.proc.Identity()
	run.Process = &identity
	run.StartedAt = ptr(now())
	run.ExecutionStatus = "running"
	execution.Process = &identity
	execution.Status = "active"
	execution.CurrentStep = attempt.StepID
	execution.UpdatedAt = now()
	if err = TransitionStepAttempt(attempt, "dispatched"); err == nil {
		err = TransitionStepAttempt(attempt, "running")
	}
	if err != nil {
		return err
	}
	if err = s.setOperationState(opID, "confirmed", "", mustJSON(identity), ""); err != nil {
		return err
	}
	s.State.CurrentTask = ptr(run.TaskID)
	s.State.SlotOccupied = true
	s.note(run, "steer", "step_start", "started", "minimal-v1 step started", attempt.StepID, map[string]any{"step": attempt.StepID, "attempt_id": attempt.AttemptID, "backend": run.Backend, "protocol": run.Protocol})
	if input.Decision != nil && run.Decision.Status == "answer_saved" {
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
	return nil
}

func (s *Service) resumeMinimalSteps(ctx context.Context) error {
	if s.worker != nil {
		return nil
	}
	if s.State.Control.Operation != nil || len(s.State.Control.Blockers) != 0 {
		return nil
	}
	for _, run := range s.State.Runs {
		if run == nil || run.WorkflowVersion != MinimalWorkflowVersion {
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
		for _, other := range s.State.Runs {
			if other != nil && other.RunID != run.RunID && (other.ExecutionStatus == "unknown" || other.ExecutionStatus == "pending" || other.ExecutionStatus == "running" || other.FinishTaskStatus == "waiting_decision") {
				return errors.New("another execution still holds the execution slot")
			}
		}
		task, err := s.beads.Show(ctx, run.TaskID)
		if err != nil {
			return err
		}
		if task.Status != "in_progress" || task.Assignee != run.Claimant {
			return errors.New("task ownership changed before planned step dispatch")
		}
		source, err := s.beads.Context(ctx)
		if err != nil {
			return err
		}
		if run.BeadsContext == nil || !source.Same(*run.BeadsContext) {
			return errors.New("Beads identity changed before minimal step resume")
		}
		if current.StepID == "prepare" {
			return s.prepareMinimalExecution(ctx, task, run, source)
		}
		return s.startMinimalStep(ctx, task, run, source, current)
	}
	return nil
}

func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }
