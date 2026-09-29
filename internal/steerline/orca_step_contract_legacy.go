package steerline

// Retained only for Orca platform probes and their fixed step envelopes.
// Production task claims accept worker-flow-v1 and never dispatch this contract.

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
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

func writeNativeMinimalPrompt(path string, input NativeTaskWorkerContext, contract StepContract) error {
	data, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return err
	}
	prompt := fmt.Sprintf("Execute only native task step %q. Read the fixed contract and follow project entry %q at version %q. Input is one JSON object with schema_version %q. Return one JSON object matching the supplied output schema, without Markdown or extra text. Do not claim or close tasks, update center state, dispatch another step, or merge a pull request.\n\n%s\n", input.CurrentStep, contract.Entry, contract.Version, NativeTaskInputSchema, string(data))
	prompt += "Pinned reference SHA-256 values cover the raw Git blob bytes at each reference's commit and path. Hash those bytes directly (for example, Python subprocess.check_output for git show COMMIT:PATH); do not hash the checked-out file or text passed through a shell, because line-ending conversion changes its bytes.\n"
	if input.CurrentStep == "implement" {
		prompt += "Before reporting success, commit the implementation on the assigned branch. Check that HEAD is the new candidate commit and its diff from the fixed base satisfies the task contract. Report the actual raw Git status in result.workspace.dirty; the center independently decides whether workspace changes affect the execution gate. Set result.revision and result.workspace.head to the exact candidate commit. Report succeeded=false if the implementation or commit is incomplete.\n"
	}
	if input.StepInput.Decision != nil {
		prompt += "After applying the supplied human answer, include result.applied_decision_id equal to decision.answer.decision_id as application evidence.\n"
	}
	return AtomicWrite(path, []byte(prompt))
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
