package ports

import "github.com/erranliu/SteerLine/internal/runnerbridge"

// RunnerSnapshot samples caller-owned task facts and public Run projections in
// one read transaction. It contains no Runner database or native state handle.
type RunnerSnapshot struct {
	Graph      TaskGraphSnapshot
	Candidates []runnerbridge.Candidate
	Checkpoint *ServiceCheckpoint
}
