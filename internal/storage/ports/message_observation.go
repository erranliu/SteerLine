package ports

type MessageObservation struct {
	Project, CandidateID, RunID, AttemptID, SessionID, TurnID string
	Text, ObservedAt, Diagnostic                              string
}
