package scheduler

import (
	"context"
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
func (*lifecycleActor) ReplayErrors() <-chan error                            { return make(chan error) }
func (*lifecycleActor) HandleSignal(struct{}) error                           { return nil }
func (*lifecycleActor) HandleReplayError(context.Context, error) error        { return nil }
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
