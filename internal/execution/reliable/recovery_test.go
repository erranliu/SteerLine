package reliable

import (
	"testing"
)

func TestClassifyRecovery(t *testing.T) {
	for _, tc := range []struct {
		name string
		fact RecoveryFacts
		want RecoveryDisposition
	}{
		{"live turn", RecoveryFacts{WorkerLive: true}, RecoveryRunning},
		{"live question", RecoveryFacts{WorkerLive: true, QuestionPending: true}, RecoveryQuestion},
		{"completed after exit", RecoveryFacts{WorkerAbsent: true, WritersAbsent: true, Completed: true}, RecoveryCompleted},
		{"dead incomplete", RecoveryFacts{WorkerAbsent: true, WritersAbsent: true}, RecoveryInterrupted},
		{"dead descendants unknown", RecoveryFacts{WorkerAbsent: true}, RecoveryUnverified},
		{"unreachable", RecoveryFacts{}, RecoveryUnverified},
		{"unknown external outcome", RecoveryFacts{WorkerAbsent: true, WritersAbsent: true, OutcomeUncertain: true}, RecoveryUnverified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyRecovery(tc.fact); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
