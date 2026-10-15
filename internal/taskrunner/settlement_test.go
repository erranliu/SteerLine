package taskrunner

import (
	"context"
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
		{"empty", nil, true},
		{"unknown_submission", &runnerstate.Candidate{SubmitState: "unknown"}, false},
		{"missing_projection", &runnerstate.Candidate{RunID: "original"}, false},
		{"native_running", &runnerstate.Candidate{RunID: "original", Projection: &runnerclient.Run{StopStatus: "running"}}, false},
		{"native_stopped", &runnerstate.Candidate{RunID: "original", Projection: &runnerclient.Run{StopStatus: "stopped"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := ports.RunnerSnapshot{}
			if tc.candidate != nil {
				snapshot.Candidates = []runnerstate.Candidate{*tc.candidate}
			}
			calls := 0
			a := &Actor{options: ActorOptions{Store: settlementStore{snapshot: snapshot}, Tasks: &Service{Project: "original"}, ShutdownRunner: func(context.Context) error { calls++; return nil }}}
			if a.ResourceSettlementError() == nil {
				t.Fatal("shutdown was never confirmed")
			}
			if err := a.shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := a.shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || (a.ResourceSettlementError() == nil) != tc.settled {
				t.Fatal("daemon exit replaced native evidence or shutdown was replayed", calls, a.ResourceSettlementError())
			}
		})
	}
}
