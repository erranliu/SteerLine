package workflow

import (
	"encoding/json"
	"errors"
)

type FileReference struct {
	Path string `json:"path"`
}

const ImplementPrompt = "Implement the following Spec and Plan."
const reportPrompt = "Format the preceding implementation result as JSON. Do not change files, run checks or publish."
const publishPrompt = "Push and create or reuse a PR using the current checked-out branch. Read its exact name with git branch --show-current and use that same name for the remote branch and PR head. Do not reconstruct the branch name from the task ID, workflow run ID, workspace path or previous messages."
const PublishReportPrompt = "Format the preceding publication result as JSON. Do not change files, run checks or publish."

func RenderPrompt(step string, facts any) (string, error) {
	text := ""
	switch step {
	case StepImplement:
		text = ImplementPrompt
	case StepReport:
		text = reportPrompt
	case StepPublish:
		text = publishPrompt
	case StepPublishReport:
		text = PublishReportPrompt
	default:
		return "", errors.New("no automatic prompt for this step")
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		return "", err
	}
	if step == StepImplement {
		var input struct {
			Task struct {
				Spec       FileReference   `json:"spec"`
				Plan       FileReference   `json:"plan"`
				Common     []FileReference `json:"common_refs"`
				Acceptance []FileReference `json:"acceptance_refs"`
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
	if step == StepReport || PublicationStep(step) {
		text += "\n" + ResultInstruction(step)
	}
	return text, nil
}
