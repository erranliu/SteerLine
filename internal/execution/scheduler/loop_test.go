package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

type lifecycleActor struct {
	cancel            context.CancelFunc
	begin, initialize bool
	ended, waited     bool
	mode              string
}

type stoppingActor struct {
	lifecycleActor
	failure error
	prior   error
	stop    *productapi.ControlRequest
}

func (a *stoppingActor) Initialize(context.Context, context.CancelFunc) error {
	a.initialize = true
	return nil
}

func (a *stoppingActor) TickTask(ctx context.Context, _ context.CancelFunc) error {
	<-ctx.Done()
	return errors.Join(ctx.Err(), a.failure)
}

func (a *stoppingActor) EndActor(stop *productapi.ControlRequest, _ *time.Time, prior error) error {
	a.ended, a.prior, a.stop = true, prior, stop
	return prior
}

func TestAcceptedStopDuringActorPollKeepsOnlyRealErrors(t *testing.T) {
	for _, joined := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancellation", true: "joined_failure"}[joined], func(t *testing.T) {
			a := &stoppingActor{}
			failure := errors.New("persistence failed")
			if joined {
				a.failure = failure
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := Run(ctx, a, Schedule{TaskPoll: time.Millisecond, PRPoll: time.Second, Heartbeat: time.Second,
				FindStop: func(string, map[string]bool) (*productapi.ControlRequest, string, time.Time) {
					return &productapi.ControlRequest{RequestID: "accepted-stop"}, "digest", time.Now()
				}}, "manual")
			if !a.ended || !a.waited || a.stop == nil {
				t.Fatal("accepted stop did not finish the actor")
			}
			if joined {
				if !errors.Is(err, failure) || !errors.Is(a.prior, failure) {
					t.Fatalf("stop hid a real error: %v", err)
				}
			} else if err != nil || a.prior != nil {
				t.Fatalf("accepted stop became a service failure: %v", err)
			}
		})
	}
}

func (a *lifecycleActor) BeginActor(context.Context) error { a.begin = true; return nil }
func (*lifecycleActor) RecoverAdmission() (bool, error)    { return true, nil }
func (a *lifecycleActor) Initialize(context.Context, context.CancelFunc) error {
	a.initialize = true
	a.cancel()
	return nil
}
func (*lifecycleActor) InstanceID() string                                    { return "fixture-instance" }
func (a *lifecycleActor) WaitSources()                                        { a.waited = true }
func (*lifecycleActor) Signals() <-chan struct{}                              { return make(chan struct{}) }
func (*lifecycleActor) HistoryErrors() <-chan error                           { return make(chan error) }
func (*lifecycleActor) HandleSignal(struct{}) error                           { return nil }
func (*lifecycleActor) HandleHistoryError(context.Context, error) error       { return nil }
func (*lifecycleActor) PollHistory(context.Context)                           {}
func (*lifecycleActor) TickDecision(context.Context) error                    { return nil }
func (*lifecycleActor) TickTask(context.Context, context.CancelFunc) error    { return nil }
func (*lifecycleActor) TickControl(context.Context, context.CancelFunc) error { return nil }
func (*lifecycleActor) TickPR(context.Context) error                          { return nil }
func (*lifecycleActor) Heartbeat() error                                      { return nil }
func (a *lifecycleActor) EndActor(*productapi.ControlRequest, *time.Time, error) error {
	a.ended = true
	return nil
}

func TestRunStopsActorAndWaitsForSourcesOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &lifecycleActor{cancel: cancel}
	err := Run(ctx, a, Schedule{TaskPoll: time.Second, PRPoll: time.Second, Heartbeat: time.Second}, "manual")
	if err != nil || !a.begin || !a.initialize || !a.ended || !a.waited {
		t.Fatalf("actor lifecycle was incomplete: actor=%+v err=%v", a, err)
	}
}

func TestRunRejectsUnknownDispatchModeBeforeStartingActor(t *testing.T) {
	a := &lifecycleActor{cancel: func() {}}
	if err := Run(context.Background(), a, Schedule{}, "automatic"); err == nil {
		t.Fatal("unknown dispatch mode was accepted")
	}
	if a.begin || a.initialize || a.ended {
		t.Fatalf("invalid mode reached actor lifecycle: %+v", a)
	}
}
