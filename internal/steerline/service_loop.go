package steerline

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/execution"
	"time"
)

// Scheduler is the host's sole business capability. Calls run serially;
// qualification and publication decisions remain in the execution actor.
type Scheduler interface {
	BeginActor(context.Context) error
	RecoverAdmission() (bool, error)
	Initialize(context.Context, context.CancelFunc) error
	InstanceID() string
	WaitSources()
	Signals() <-chan execution.Signal
	ReplayErrors() <-chan error
	HandleSignal(execution.Signal) error
	HandleReplayError(context.Context, error) error
	PollHistory(context.Context)
	TickDecision(context.Context) error
	TickTask(context.Context, context.CancelFunc) error
	TickControl(context.Context, context.CancelFunc) error
	TickPR(context.Context) error
	Heartbeat() error
	EndActor(*execution.StopAcceptance, error) error
}

func RunService(ctx context.Context, actor Scheduler, c execution.Config, mode string) (retErr error) {
	if mode != "manual" && mode != "auto" {
		return errors.New("dispatch mode must be manual or auto")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer func() { cancel(); actor.WaitSources() }()
	if err := actor.BeginActor(ctx); err != nil {
		return err
	}
	heartDone := make(chan struct{})
	acceptedStops := make(chan execution.StopAcceptance, 1)
	instanceID := actor.InstanceID()
	go func() {
		defer close(heartDone)
		heartbeat := time.NewTicker(duration(c.Heartbeat))
		defer heartbeat.Stop()
		stopTick := time.NewTicker(500 * time.Millisecond)
		defer stopTick.Stop()
		seen := map[string]bool{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopTick.C:
				request, _, checkedAt := findStopRequestCached(c, instanceID, seen)
				if request != nil {
					acceptedStops <- execution.StopAcceptance{Request: *request, At: checkedAt}
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
		var stop *execution.StopAcceptance
		select {
		case value := <-acceptedStops:
			stop = &value
		default:
		}
		retErr = actor.EndActor(stop, retErr)
	}()
	taskTick := time.NewTicker(duration(c.TaskPoll))
	defer taskTick.Stop()
	prTick := time.NewTicker(duration(c.PRPoll))
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
		case err := <-actor.ReplayErrors():
			if err = actor.HandleReplayError(ctx, err); err != nil {
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
