package sqlite

func validStepAttemptStatus(v string) bool {
	switch v {
	case "planned", "dispatching", "dispatched", "running", "waiting_decision", "succeeded", "failed", "rejected", "stopping", "stopped", "outcome_unknown", "abandoned":
		return true
	}
	return false
}

func validWorkflowPhase(v string) bool {
	switch v {
	case "queued", "running", "waiting_input", "waiting_external", "reconciling", "completed", "blocked", "abandoned", "executing", "waiting_decision", "waiting_pr_merge", "completing":
		return true
	}
	return false
}
func validExecutionStatus(v string) bool {
	switch v {
	case "pending", "active", "idle", "waiting_decision", "succeeded", "failed", "blocked", "stopped", "abandoned":
		return true
	}
	return false
}
func validDecisionStatus(v string) bool {
	switch v {
	case "pending", "answer_saved", "resuming", "reply_accepted", "applied", "failed", "superseded":
		return true
	}
	return false
}
func validOperationStatus(v string) bool {
	switch v {
	case "pending", "in_flight", "accepted", "confirmed", "rejected", "outcome_unknown":
		return true
	}
	return false
}
