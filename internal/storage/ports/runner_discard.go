package ports

import (
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"time"
)

type RunnerDiscard struct {
	Project, RequestID              string
	Intent                          taskmodel.DiscardIntent
	AuthorityVersion, GraphRevision uint64
	CandidateRevisions              map[string]uint64
	VerificationStarted             time.Time
}
