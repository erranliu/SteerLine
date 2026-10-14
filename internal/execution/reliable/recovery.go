package reliable

// RecoveryFacts contain verified worker evidence. A missing connection or a
// missing process alone does not establish the absence of workspace writers.
type RecoveryFacts struct {
	WorkerLive       bool
	WorkerAbsent     bool
	WritersAbsent    bool
	Completed        bool
	QuestionPending  bool
	OutcomeUncertain bool
}

type RecoveryDisposition string

const (
	RecoveryRunning     RecoveryDisposition = "running"
	RecoveryQuestion    RecoveryDisposition = "question"
	RecoveryCompleted   RecoveryDisposition = "completed"
	RecoveryInterrupted RecoveryDisposition = "interrupted"
	RecoveryUnverified  RecoveryDisposition = "unverified"
)

func ClassifyRecovery(f RecoveryFacts) RecoveryDisposition {
	if f.OutcomeUncertain || f.WorkerLive && f.WorkerAbsent {
		return RecoveryUnverified
	}
	if f.Completed {
		return RecoveryCompleted
	}
	if f.WorkerLive {
		if f.QuestionPending {
			return RecoveryQuestion
		}
		return RecoveryRunning
	}
	if f.WorkerAbsent && f.WritersAbsent {
		return RecoveryInterrupted
	}
	return RecoveryUnverified
}
