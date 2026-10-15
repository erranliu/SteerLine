package ports

import "time"

// RunnerResult binds a task conclusion to a newly verified public PR fact in
// the caller's candidate projection. It contains no native execution records.
type RunnerResult struct {
	Project, TaskID, CandidateID                                    string
	TaskVersion, CandidateRevision, AuthorityVersion, GraphRevision uint64
	VerificationStarted                                             time.Time
}
