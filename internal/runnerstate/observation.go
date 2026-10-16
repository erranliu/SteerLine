package runnerstate

// ObservedStatus preserves the last public Runner fact. Before an observation
// exists, only the caller's submission state is known. Participation is separate.
func ObservedStatus(candidate Candidate) string {
	if candidate.Projection != nil {
		return candidate.Projection.Status
	}
	return candidate.SubmitState
}
