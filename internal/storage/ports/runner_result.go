package ports

import (
	"time"

	"github.com/erranliu/SteerLine/internal/execution/model"
)

// RunnerPRObservation is local to one task operation, separate from the Run projection.
type RunnerPRObservation struct {
	CandidateID, RunID   string
	CandidateRevision    uint64
	StartedAt, CheckedAt time.Time
	Fact                 model.PRInfo
}

// RunnerResult binds a task conclusion to a fresh observation of its original PR.
type RunnerResult struct {
	Project, TaskID, CandidateID                                    string
	TaskVersion, CandidateRevision, AuthorityVersion, GraphRevision uint64
	VerificationStarted                                             time.Time
	PR                                                              RunnerPRObservation
}
