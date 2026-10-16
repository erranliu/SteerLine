package taskrunner

import (
	"context"
	"errors"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type settlementStore struct {
	ActorStore
	snapshot ports.RunnerSnapshot
}

func (s settlementStore) ReadRunnerSnapshot(context.Context, string) (ports.RunnerSnapshot, error) {
	return s.snapshot, nil
}

func TestRunnerShutdownDoesNotEquateServiceExitWithNativeSettlement(t *testing.T) {
	for _, tc := range []struct {
		name      string
		candidate *runnerstate.Candidate
		settled   bool
	}{
		{"empty_without_proof", nil, false},
		{"unknown_submission", &runnerstate.Candidate{SubmitState: "unknown"}, false},
		{"missing_projection", &runnerstate.Candidate{RunID: "original"}, false},
		{"native_running", &runnerstate.Candidate{RunID: "original", Projection: &runnerclient.Run{StopStatus: "running"}}, false},
		{"cached_stopped_without_proof", &runnerstate.Candidate{RunID: "original", Projection: &runnerclient.Run{StopStatus: "stopped"}}, false},
		{"stale_running_with_final_proof", &runnerstate.Candidate{RunID: "original", Projection: &runnerclient.Run{StopStatus: "running"}}, true},
		{"unknown_submission_with_final_proof", &runnerstate.Candidate{SubmitState: "unknown"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := ports.RunnerSnapshot{}
			if tc.candidate != nil {
				snapshot.Candidates = []runnerstate.Candidate{*tc.candidate}
			}
			calls := 0
			proofs := 0
			a := &Actor{options: ActorOptions{Store: settlementStore{snapshot: snapshot}, Tasks: &Service{Project: "original"}, ShutdownRunner: func(context.Context) error { calls++; return nil }, RunnerSettlement: func(context.Context) error {
				if calls != 1 {
					t.Fatal("settlement queried before service exit")
				}
				proofs++
				if !tc.settled {
					return errors.New("original native resources remain unverified")
				}
				return nil
			}}}
			if a.ResourceSettlementError() == nil {
				t.Fatal("shutdown was never confirmed")
			}
			if err := a.shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := a.shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			expectedProofs := 2
			if tc.settled {
				expectedProofs = 1
			}
			if calls != 1 || proofs != expectedProofs || (a.ResourceSettlementError() == nil) != tc.settled {
				t.Fatal("daemon exit replaced native evidence or shutdown was replayed", calls, a.ResourceSettlementError())
			}
		})
	}
}

func TestRunnerSettlementReadCanRecoverWithoutRepeatingStop(t *testing.T) {
	stops, reads := 0, 0
	a := &Actor{options: ActorOptions{
		ShutdownRunner: func(context.Context) error { stops++; return nil },
		RunnerSettlement: func(context.Context) error {
			reads++
			if reads == 1 {
				return context.DeadlineExceeded
			}
			return nil
		},
	}}
	if err := a.shutdown(context.Background()); err != nil || a.ResourceSettlementError() == nil {
		t.Fatal(err)
	}
	if err := a.shutdown(context.Background()); err != nil || a.ResourceSettlementError() != nil || stops != 1 || reads != 2 {
		t.Fatal(err, a.ResourceSettlementError(), stops, reads)
	}
}
