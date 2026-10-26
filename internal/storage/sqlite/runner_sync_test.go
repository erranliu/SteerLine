package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func TestRunnerActorPublishesCandidatesBeforeAuxiliaryFailure(t *testing.T) {
	for _, slow := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "timeout"}[slow], func(t *testing.T) {
			ctx := context.Background()
			s, project, reserve := candidateStopFixture(t)
			candidate := reserve(strings.Repeat("b", 32), false)
			candidate.RunID, candidate.InputDigest, candidate.SubmitState = "run", "input", "accepted"
			candidate.Projection = &runnerclient.Run{RunID: "run", InputDigest: "input", Revision: 1, Status: "running"}
			candidate.Revision++
			require(t, s.SaveCandidate(ctx, candidate, 1, nil))
			cli := &blockedObservation{run: *candidate.Projection, immediate: true}
			cli.run.Revision = 2
			cli.run.Phase = "observing"
			svc := &taskrunner.Service{Project: project, Store: s, Bridge: &runnerbridge.Bridge{Store: s, Client: func(string) runnerbridge.CLI { return cli }}}
			actor, err := taskrunner.NewActor(taskrunner.ActorOptions{Tasks: svc, Store: s, Host: &runnerActorHost{}, Repository: hostcontract.Repository{Identity: project}, Mode: "manual", Heartbeat: time.Second, Control: func(context.Context, api.ControlRequest) (api.ControlReceipt, error) {
				return api.ControlReceipt{}, nil
			}})
			require(t, err)
			failure := errors.New("auxiliary unavailable")
			actor.ObserveAuxiliary = func(work context.Context) error {
				// The public runtime must already contain the new fact even while
				// this callback is still waiting on an unrelated resource.
				if len(actor.State.Runs) != 1 || actor.State.Runs[0].RunnerRevision != cli.run.Revision {
					t.Fatal("live candidate was not published before auxiliary work")
				}
				if slow {
					deadline, ok := work.Deadline()
					if !ok || time.Until(deadline) > 5*time.Second {
						t.Fatal("auxiliary work has no bounded budget")
					}
					<-work.Done()
					return work.Err()
				}
				return failure
			}
			started := time.Now()
			require(t, actor.TickDecision(ctx))
			if slow && time.Since(started) > 7*time.Second {
				t.Fatal("auxiliary timeout blocked the actor")
			}
			if actor.State.Service.Error == nil {
				t.Fatal("auxiliary diagnostic was lost")
			}
			// A later tick must continue observing, and clear a recovered error.
			actor.ObserveAuxiliary = func(context.Context) error { return nil }
			cli.run.Revision++
			require(t, actor.TickDecision(ctx))
			if actor.State.Runs[0].RunnerRevision != 3 || actor.State.Service.Error != nil {
				t.Fatal("auxiliary failure prevented subsequent candidate updates")
			}
		})
	}
}
