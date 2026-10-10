package scheduler

import (
	"context"
	"errors"
	"time"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// Scheduler is the host's sole business capability. Calls run serially;
// qualification and publication decisions remain in the execution actor.
type Scheduler[T any] interface {
	BeginActor(context.Context) error
	RecoverAdmission() (bool, error)
	Initialize(context.Context, context.CancelFunc) error
	InstanceID() string
	WaitSources()
	Signals() <-chan T
	HistoryErrors() <-chan error
	HandleSignal(T) error
	HandleHistoryError(context.Context, error) error
	PollHistory(context.Context)
	TickDecision(context.Context) error
	TickTask(context.Context, context.CancelFunc) error
	TickControl(context.Context, context.CancelFunc) error
	TickPR(context.Context) error
	Heartbeat() error
	EndActor(*productapi.ControlRequest, *time.Time, error) error
}

func Run[T any](ctx context.Context, actor Scheduler[T], c Schedule, mode string) (retErr error) {
	if mode != "manual" && mode != "auto" {
		return errors.New("dispatch mode must be manual or auto")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer func() { cancel(); actor.WaitSources() }()
	if err := actor.BeginActor(ctx); err != nil {
		return err
	}
	heartDone := make(chan struct{})
	acceptedStops := make(chan stopAcceptance, 1)
	instanceID := actor.InstanceID()
	go func() {
		defer close(heartDone)
		heartbeat := time.NewTicker(c.Heartbeat)
		defer heartbeat.Stop()
		stopTick := time.NewTicker(500 * time.Millisecond)
		defer stopTick.Stop()
		seen := map[string]bool{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopTick.C:
				request, _, checkedAt := c.FindStop(instanceID, seen)
				if request != nil {
					acceptedStops <- stopAcceptance{Request: *request, At: checkedAt}
					cancel()
					return
				}
			case <-heartbeat.C:
				if actor.Heartbeat() != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-heartDone
		var stop *stopAcceptance
		select {
		case value := <-acceptedStops:
			stop = &value
		default:
		}
		var request *productapi.ControlRequest
		var acceptedAt *time.Time
		if stop != nil {
			request = &stop.Request
			acceptedAt = &stop.At
			if cancellationOnly(retErr) {
				retErr = nil // An accepted stop may cancel an in-flight actor poll.
			}
		}
		retErr = actor.EndActor(request, acceptedAt, retErr)
	}()
	taskTick := time.NewTicker(c.TaskPoll)
	defer taskTick.Stop()
	prTick := time.NewTicker(c.PRPoll)
	defer prTick.Stop()
	decisionTick := time.NewTicker(2 * time.Second)
	defer decisionTick.Stop()
	controlTick := time.NewTicker(500 * time.Millisecond)
	defer controlTick.Stop()
	historyTick := time.NewTicker(2 * time.Second)
	defer historyTick.Stop()
	for {
		settled, err := actor.RecoverAdmission()
		if err != nil {
			return err
		}
		if settled {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-controlTick.C:
		}
	}
	if err := actor.Initialize(ctx, cancel); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-historyTick.C:
			actor.PollHistory(ctx)
		case err := <-actor.HistoryErrors():
			if err = actor.HandleHistoryError(ctx, err); err != nil {
				return err
			}
		case message := <-actor.Signals():
			if err := actor.HandleSignal(message); err != nil {
				return err
			}
		case <-decisionTick.C:
			if err := actor.TickDecision(ctx); err != nil {
				return err
			}
		case <-taskTick.C:
			if err := actor.TickTask(ctx, cancel); err != nil {
				return err
			}
		case <-controlTick.C:
			if err := actor.TickControl(ctx, cancel); err != nil {
				return err
			}
		case <-prTick.C:
			if err := actor.TickPR(ctx); err != nil {
				return err
			}
		}
	}
}

// A joined persistence/business error must survive an accepted stop. Only the
// cancellation caused by that stop is a successful scheduler exit.
func cancellationOnly(err error) bool {
	if err == context.Canceled {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !cancellationOnly(child) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return cancellationOnly(wrapped.Unwrap())
	}
	return false
}

type Schedule struct {
	TaskPoll, PRPoll, Heartbeat time.Duration
	FindStop                    func(string, map[string]bool) (*productapi.ControlRequest, string, time.Time)
}
type stopAcceptance struct {
	Request productapi.ControlRequest
	At      time.Time
}
