package execution

import (
	"encoding/json"
	"errors"
	workflow "github.com/erranliu/SteerLine/internal/execution/workflow"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

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
	return workflow.RenderPrompt(executionmodel.StepImplement, struct {
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
