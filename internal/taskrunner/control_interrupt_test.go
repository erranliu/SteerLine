package taskrunner

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCandidateStopReliabilityInterruptsOnlyWork(t *testing.T) {
	actor := &Actor{wakeup: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, done := make(chan struct{}), make(chan error, 1)
	go func() {
		done <- actor.RunWork(ctx, func(work context.Context) error { close(entered); <-work.Done(); return work.Err() })
	}()
	<-entered
	actor.NotifyControl()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("external wait retained actor")
	}
	if ctx.Err() != nil {
		t.Fatal("notification cancelled lifecycle")
	}
	select {
	case <-actor.ControlWakeup():
	default:
		t.Fatal("durable controls were not signalled")
	}
	failure := errors.New("storage failure")
	err := actor.RunWork(ctx, func(work context.Context) error { return errors.Join(work.Err(), failure) })
	if !errors.Is(err, failure) {
		t.Fatal("interrupt hid storage error", err)
	}
}
