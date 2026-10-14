package model

import "time"

// CandidateParticipates depends only on the candidate's own workflow state.
func CandidateParticipates(status string) bool {
	return status != "" && status != "completed" && status != "abandoned"
}

// StopCandidate is the single first-wins declaration for manual stops and
// definite startup failures. Execution and resource facts remain untouched.
func StopCandidate(w *WorkflowRunRecord, reason, by string, at time.Time) bool {
	if w == nil || !CandidateParticipates(w.Status) {
		return false
	}
	w.Status, w.StartPending, w.UpdatedAt = "abandoned", false, at
	w.CandidateStoppedAt, w.CandidateStopReason, w.CandidateStoppedBy = &at, reason, by
	return true
}
