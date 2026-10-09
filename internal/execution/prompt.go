package execution

import (
	"encoding/json"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func Prompt(task taskmodel.TaskRecord, contract taskmodel.TaskContract, workflowRunID string, policy executionbackend.PublicationPolicy) (string, error) {
	if task.TaskID == "" || task.Title == "" || contract.ContractID == "" || contract.SpecRef.Path == "" || contract.PlanRef.Path == "" || !executionbackend.StableExecutionID(workflowRunID) {
		return "", errors.New("worker instruction requires a fixed task, Spec, Plan and workflow run")
	}
	if err := policy.Validate(); err != nil {
		return "", err
	}
	facts, err := json.Marshal(struct {
		TaskID     string                    `json:"task_id"`
		Title      string                    `json:"title"`
		Spec       taskmodel.FileReference   `json:"spec"`
		Plan       taskmodel.FileReference   `json:"plan"`
		Common     []taskmodel.FileReference `json:"common_refs,omitempty"`
		Acceptance []taskmodel.FileReference `json:"acceptance_refs,omitempty"`
	}{task.TaskID, task.Title, contract.SpecRef, contract.PlanRef, contract.CommonRefs, contract.AcceptanceRefs})
	if err != nil {
		return "", err
	}
	return "Current step: implement.execute. Follow the bound Spec/Plan and project instructions. Read references from their listed Git commits and check SHA-256. Complete implementation, required tests, self-review and a local candidate commit. Do not publish in this step. End with a normal explanation of work, checks and unresolved issues; no JSON is required.\nFixed facts:\n" + string(facts), nil
}
