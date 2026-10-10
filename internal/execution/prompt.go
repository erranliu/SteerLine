package execution

import (
	"encoding/json"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const implementPrompt = "Implement the following Spec and Plan."
const reportPrompt = "Format the preceding implementation result as JSON."
const publishPrompt = "Push and create or reuse a PR."
const publishReportPrompt = "Format the preceding publication result as JSON."

func renderWorkerPrompt(step string, facts any) (string, error) {
	text := ""
	switch step {
	case executionmodel.StepImplement:
		text = implementPrompt
	case executionmodel.StepReport:
		text = reportPrompt
	case executionmodel.StepPublish:
		text = publishPrompt
	case executionmodel.StepPublishReport:
		text = publishReportPrompt
	default:
		return "", errors.New("no automatic prompt for this step")
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		return "", err
	}
	if step == executionmodel.StepImplement {
		var input struct {
			Task struct {
				Spec       taskmodel.FileReference   `json:"spec"`
				Plan       taskmodel.FileReference   `json:"plan"`
				Common     []taskmodel.FileReference `json:"common_refs"`
				Acceptance []taskmodel.FileReference `json:"acceptance_refs"`
			} `json:"task"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return "", err
		}
		text += "\nSpec: " + input.Task.Spec.Path + "\nPlan: " + input.Task.Plan.Path
		seen := map[string]bool{input.Task.Spec.Path: true, input.Task.Plan.Path: true}
		for _, ref := range append(input.Task.Common, input.Task.Acceptance...) {
			if ref.Path != "" && !seen[ref.Path] {
				text += "\nReference: " + ref.Path
				seen[ref.Path] = true
			}
		}
	} else if string(raw) != "{}" && string(raw) != "null" {
		text += "\nDiagnostics:\n" + string(raw)
	}
	if step == executionmodel.StepReport || step == executionmodel.StepPublishReport {
		text += "\n" + stageResultInstruction(step)
	}
	return text, nil
}

func validateWorkerWorkspace(workspace executionmodel.WorkspaceEvidence) error {
	if workspace.Worktree == "" || workspace.Branch == "" || !commitHash.MatchString(workspace.Base) {
		return errors.New("worker requires its prepared workspace, assigned branch and fixed base")
	}
	return nil
}

func Prompt(task taskmodel.TaskRecord, contract taskmodel.TaskContract, workflowRunID string, policy executionbackend.PublicationPolicy) (string, error) {
	if task.TaskID == "" || task.Title == "" || contract.ContractID == "" || contract.SpecRef.Path == "" || contract.PlanRef.Path == "" || !executionbackend.StableExecutionID(workflowRunID) {
		return "", errors.New("worker instruction requires a fixed task, Spec, Plan and workflow run")
	}
	if err := policy.Validate(); err != nil {
		return "", err
	}
	facts, err := workerTaskFacts(task, contract)
	if err != nil {
		return "", err
	}
	return renderWorkerPrompt(executionmodel.StepImplement, struct {
		Task json.RawMessage `json:"task"`
	}{facts})
}

func workerTaskFacts(task taskmodel.TaskRecord, contract taskmodel.TaskContract) (json.RawMessage, error) {
	return json.Marshal(struct {
		TaskID     string                    `json:"task_id"`
		Title      string                    `json:"title"`
		Spec       taskmodel.FileReference   `json:"spec"`
		Plan       taskmodel.FileReference   `json:"plan"`
		Common     []taskmodel.FileReference `json:"common_refs,omitempty"`
		Acceptance []taskmodel.FileReference `json:"acceptance_refs,omitempty"`
	}{task.TaskID, task.Title, contract.SpecRef, contract.PlanRef, contract.CommonRefs, contract.AcceptanceRefs})
}
