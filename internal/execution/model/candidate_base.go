package model

// CandidateCommonBase preserves the original experiment base across every run,
// including abandoned candidates. It never claims the task again.
func CandidateCommonBase(workflows []WorkflowRunRecord, executions []ExecutionRecord, taskID string) (string, error) {
	base := ""
	for _, w := range workflows {
		if w.TaskID != taskID {
			continue
		}
		found := false
		for _, e := range executions {
			if e.WorkflowRunID != w.WorkflowRunID || e.ExecutionID != w.SelectedResult {
				continue
			}
			if !validDeliveryCommit(e.Workspace.Base) || base != "" && base != e.Workspace.Base {
				return "", &ResultError{Code: "contract_mismatch", Message: "candidate common base is missing or inconsistent"}
			}
			base, found = e.Workspace.Base, true
			break
		}
		if !found {
			return "", &ResultError{Code: "inconsistent_delivery", Message: "selected workflow result is missing"}
		}
	}
	if base == "" {
		return "", &ResultError{Code: "task_not_ready", Message: "additional candidate requires the original task claim"}
	}
	return base, nil
}
