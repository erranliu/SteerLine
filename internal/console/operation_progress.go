package console

// Operation progress is a display overlay; it never changes task or worker
// facts in the service snapshot or implies that a worker is already live.
func operationProgress(action *actionState) string {
	switch action.Status {
	case "submitting":
		return "提交中"
	case "submitted", "pending":
		return "等待受理"
	case "indeterminate":
		return "结果待核实"
	}
	switch action.Action {
	case "start_task", "start_candidate":
		return "启动中"
	case "stop_service", "stop_candidate":
		return "停止中"
	case "answer_decision":
		return "反馈处理中"
	case "recheck_environment":
		return "核验中"
	case "open_terminal":
		return "打开中"
	default:
		return "处理中"
	}
}

func taskOperation(state projectState, taskID string) *actionState {
	var latest *actionState
	for _, action := range state.actions {
		if action.Resolved || action.Status == "applied" || action.Status == "accepted" || action.Status == "rejected" || action.Status == "failed" {
			continue
		}
		if page := state.snapshot.Connection.Snapshot.Page; page != nil && action.TargetInstance != page.Service.InstanceID {
			continue
		}
		id := action.TaskID
		if action.EnvironmentTarget != nil {
			for _, run := range candidateRuns(state, "") {
				if run.RunID == action.EnvironmentTarget.RunID {
					id = run.TaskID
					break
				}
			}
		}
		if id != taskID {
			continue
		}
		if latest == nil || action.UpdatedAt.After(latest.UpdatedAt) || action.UpdatedAt.Equal(latest.UpdatedAt) && action.RequestID < latest.RequestID {
			latest = action
		}
	}
	return latest
}

func rowExecutionProgress(state projectState, row consoleRow) string {
	if action := rowOperation(state, row); action != nil && (action.Action == "start_task" || action.Action == "start_candidate") {
		// An additional candidate must not overwrite an existing sibling's status.
		if row.run == nil || firstText(action.RunID, action.RequestID) == row.run.RunID && row.run.ExecutionStatus == "pending" {
			return operationProgress(action)
		}
	}
	return rowExecutionStatus(row)
}

func rowOperation(state projectState, row consoleRow) *actionState {
	if row.run != nil {
		for _, a := range state.actions {
			if a.TaskID != row.taskID {
				continue
			}
			id := a.RunID
			if a.Action == "start_task" || a.Action == "start_candidate" {
				id = firstText(id, a.RequestID)
			}
			if id != row.run.RunID || a.Resolved || a.Status == "applied" || a.Status == "rejected" || a.Status == "failed" {
				continue
			}
			if page := state.snapshot.Connection.Snapshot.Page; page != nil && a.TargetInstance != page.Service.InstanceID {
				continue
			}
			return a
		}
	}
	action := taskOperation(state, row.taskID)
	if action == nil || row.run == nil {
		return action
	}
	runID := action.RunID
	if action.Action == "start_candidate" {
		runID = firstText(runID, action.RequestID)
	}
	if action.EnvironmentTarget != nil {
		runID = action.EnvironmentTarget.RunID
	}
	if runID != "" && runID != row.run.RunID {
		return nil
	}
	return action
}
