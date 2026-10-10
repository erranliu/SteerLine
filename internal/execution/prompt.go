package execution

import (
	"encoding/json"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const implementPrompt = "Current step: implement.execute. Follow the bound Spec/Plan and project instructions. Read references from their listed Git commits and check SHA-256. Use the supplied existing results and accepted feedback to complete unfinished implementation, required tests, self-review and a local candidate commit. Do not publish in this step. End with a normal explanation of work, checks and unresolved issues; no JSON is required."
const reportPrompt = "Current step: implement.report. Only summarize the supplied original execution, existing outputs and failures. Do not change code, retry commands or perform additional checks. Unknown evidence must remain unknown. Report generation does not make the source execution successful."
const publishPrompt = "Current step: publish. Publish the specified candidate SHA to the fixed target, create or reuse its matching PR. Do not modify the candidate code or create another commit. First query existing branch/PR results before completing unfinished actions. Return the actual PR URL or partial results, blockers and unresolved actions in this same turn."

func renderWorkerPrompt(step string, facts any) (string, error) {
	text := ""
	switch step {
	case executionmodel.StepImplement:
		text = implementPrompt
	case executionmodel.StepReport:
		text = reportPrompt
	case executionmodel.StepPublish:
		text = publishPrompt
	default:
		return "", errors.New("no automatic prompt for this step")
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		return "", err
	}
	text += "\nFacts:\n" + string(raw)
	if step != executionmodel.StepImplement {
		text += "\n" + stageResultContract
	}
	return text, nil
}

func workerWorkspaceInstruction(workspace executionmodel.WorkspaceEvidence) (string, error) {
	if workspace.Worktree == "" || workspace.Branch == "" || !commitHash.MatchString(workspace.Base) {
		return "", errors.New("worker instruction requires its prepared workspace, assigned branch and fixed base")
	}
	facts, err := json.Marshal(struct {
		Worktree string `json:"worktree"`
		Branch   string `json:"branch"`
		Base     string `json:"base"`
	}{workspace.Worktree, workspace.Branch, workspace.Base})
	if err != nil {
		return "", err
	}
	return "\nFixed workspace:\n" + string(facts) + "\n", nil
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
