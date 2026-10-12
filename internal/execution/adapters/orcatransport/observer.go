package orcatransport

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

type observer struct {
	events  chan b.LifecycleEvent
	done    chan error
	cancel  context.CancelFunc
	process b.ProcessIdentity
	once    sync.Once
}

func (o *observer) Events() <-chan b.LifecycleEvent { return o.events }
func (o *observer) Done() <-chan error              { return o.done }
func (o *observer) StopHandle() b.StopHandle        { return &observationHandle{owner: o} }

// Passive cancellation cannot await a completion event which the actor is acknowledging.
func (o *observer) Close(context.Context) error { o.once.Do(o.cancel); return nil }

type observationHandle struct{ owner *observer }

func (h *observationHandle) Identity() b.ProcessIdentity { return h.owner.process }
func (h *observationHandle) Kill()                       { _ = h.owner.Close(context.Background()) }
func (h *observationHandle) Close()                      { _ = h.owner.Close(context.Background()) }

func (l *Lifecycle) observe(s b.LifecycleSnapshot) b.LifecycleObserver {
	ctx, cancel := context.WithCancel(context.Background())
	o := &observer{events: make(chan b.LifecycleEvent, 4), done: make(chan error, 1), cancel: cancel, process: s.Ref.External}
	go func() {
		defer close(o.events)
		defer close(o.done)
		send := func(event b.LifecycleEvent) bool {
			event.Kind = "turn"
			if event.InputWait {
				event.Kind, event.Status, event.Title = "execution", "waiting", "Worker reply unavailable"
			} else if event.Fact.Kind == b.EventStarted {
				event.Status, event.Title = "started", "Worker input started"
			} else if event.Fact.Kind == b.EventCompleted {
				event.Status, event.Title = "completed", "Worker reply committed"
			} else {
				event.Kind, event.Status, event.Title = "execution", "observing", "Worker output observed"
			}
			event.Persisted = make(chan error, 1)
			event.Processed = make(chan error, 1)
			select {
			case o.events <- event:
			case <-ctx.Done():
				return false
			}
			select {
			case err := <-event.Persisted:
				if err != nil {
					o.done <- err
					return false
				}
			case <-ctx.Done():
				return false
			}
			select {
			case err := <-event.Processed:
				if err != nil {
					o.done <- err
					return false
				}
			case <-ctx.Done():
				return false
			}
			return true
		}
		started, waiting := false, false
		for {
			before, _ := json.Marshal(s.Implementation)
			receipt, _, err := l.observeOnce(ctx, s)
			if err != nil {
				if ctx.Err() == nil {
					o.done <- err
				}
				return
			}
			if len(receipt.Implementation) > 0 {
				s.Implementation = receipt.Implementation
			}
			committed := false
			if receipt.Diagnostic != "" && !waiting {
				if !send(b.LifecycleEvent{InputWait: true, Diagnostic: receipt.Diagnostic, Fact: b.EventFact{Kind: b.EventStatus, Source: "orca.terminal.wait", SourceIdentity: SessionID(s.Ref.External), SourceSegment: command(s), SourceEventID: command(s) + ":reply-missing", SessionID: SessionID(s.Ref.External)}, Implementation: receipt.Implementation}) {
					return
				}
				waiting = true
				committed = true
			}
			if receipt.State.TurnStarted && !started {
				if !send(b.LifecycleEvent{Fact: b.EventFact{Kind: b.EventStarted, Source: replySource, SourceIdentity: SessionID(s.Ref.External), SourceSegment: command(s), SourceEventID: command(s) + ":started", SessionID: SessionID(s.Ref.External), StartedTurnID: command(s), InputAccepted: true, Matched: true}, Implementation: receipt.Implementation}) {
					return
				}
				started = true
				committed = true
			}
			if evidence := receipt.TurnEvidence; evidence != nil && evidence.Completed {
				outcome := "failed"
				if evidence.Succeeded {
					outcome = "succeeded"
				}
				if !send(b.LifecycleEvent{Fact: b.EventFact{Kind: b.EventCompleted, Source: replySource, SourceIdentity: SessionID(s.Ref.External), SourceSegment: command(s), SourceEventID: evidence.FinalItemID, SessionID: SessionID(s.Ref.External), StartedTurnID: command(s), Completed: true, Matched: true, Outcome: outcome, LastMessage: evidence.LastMessage}, Implementation: receipt.Implementation}) {
					return
				}
				o.done <- nil
				return
			}
			after, _ := json.Marshal(receipt.Implementation)
			if !committed && string(before) != string(after) {
				state, stateErr := l.state(s)
				if stateErr != nil {
					o.done <- stateErr
					return
				}
				turn := turnFor(&state, command(s))
				if turn != nil && !send(b.LifecycleEvent{Fact: b.EventFact{Kind: b.EventStatus, Source: "orca.terminal.read", SourceIdentity: SessionID(s.Ref.External), SourceSegment: command(s), SourceEventID: command(s) + ":" + digest(after), SessionID: SessionID(s.Ref.External)}, Implementation: receipt.Implementation}) {
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
	return o
}
