package scheduler

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
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
	entered chan struct{}
}

func (a *stoppingActor) Initialize(context.Context, context.CancelFunc) error {
	a.initialize = true
	return nil
}

func (a *stoppingActor) TickTask(ctx context.Context, _ context.CancelFunc) error {
	close(a.entered)
	<-ctx.Done()
	return errors.Join(ctx.Err(), a.failure)
}

func (a *stoppingActor) EndActor(prior error) error {
	a.ended, a.prior = true, prior
	return prior
}

func TestCancellationDuringActorPollKeepsOnlyRealErrors(t *testing.T) {
	for _, joined := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancellation", true: "joined_failure"}[joined], func(t *testing.T) {
			a := &stoppingActor{entered: make(chan struct{})}
			failure := errors.New("persistence failed")
			if joined {
				a.failure = failure
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			go func() {
				select {
				case <-a.entered:
					cancel()
				case <-ctx.Done():
				}
			}()
			err := Run(ctx, a, Schedule{TaskPoll: time.Millisecond, PRPoll: time.Second, Heartbeat: time.Second}, "manual")
			if !a.ended || !a.waited {
				t.Fatal("cancellation did not finish the actor")
			}
			if joined {
				if !errors.Is(err, failure) || !errors.Is(a.prior, failure) {
					t.Fatalf("stop hid a real error: %v", err)
				}
			} else if err != nil || a.prior != nil {
				t.Fatalf("cancellation became a service failure: %v", err)
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
func (a *lifecycleActor) EndActor(error) error {
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

type deliveryPriorityActor struct {
	lifecycleActor
	stop, delivery, ordinary chan struct{}
	order                    []string
}

func (*deliveryPriorityActor) Initialize(context.Context, context.CancelFunc) error { return nil }
func (a *deliveryPriorityActor) ControlWakeup() <-chan struct{}                     { return a.stop }
func (a *deliveryPriorityActor) DeliveryWakeup() <-chan struct{}                    { return a.delivery }
func (a *deliveryPriorityActor) Signals() <-chan struct{}                           { return a.ordinary }
func (*deliveryPriorityActor) RunWork(ctx context.Context, run func(context.Context) error) error {
	return run(ctx)
}
func (a *deliveryPriorityActor) TickControl(context.Context, context.CancelFunc) error {
	if len(a.order) == 0 {
		a.order = append(a.order, "startup")
		a.stop <- struct{}{}
		a.delivery <- struct{}{}
		a.ordinary <- struct{}{}
		return nil
	}
	a.order = append(a.order, "stop")
	return nil
}
func (a *deliveryPriorityActor) TickDelivery(context.Context) error {
	a.order = append(a.order, "delivery")
	return nil
}
func (a *deliveryPriorityActor) HandleSignal(struct{}) error {
	a.order = append(a.order, "ordinary")
	a.cancel()
	return nil
}

func TestRunReadyDeliveryFollowsStopBeforeOrdinaryWork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	a := &deliveryPriorityActor{lifecycleActor: lifecycleActor{cancel: cancel}, stop: make(chan struct{}, 1), delivery: make(chan struct{}, 1), ordinary: make(chan struct{}, 1)}
	if err := Run(ctx, a, Schedule{TaskPoll: time.Hour, PRPoll: time.Hour, Heartbeat: time.Hour}, "manual"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.order, []string{"startup", "stop", "delivery", "ordinary"}) {
		t.Fatal("ready delivery did not follow urgent stop ahead of ordinary work", a.order)
	}
}
