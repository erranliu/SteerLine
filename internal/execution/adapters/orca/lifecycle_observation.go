package orcaworker

import (
	"context"
	"errors"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (l *ExecutionLifecycle) ObserveLifecycle(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if err := l.validateSnapshot(snapshot); err != nil {
		return executionbackend.LifecycleReceipt{}, nil, err
	}
	state, found, err := l.readState(snapshot)
	if err != nil || !found {
		return executionbackend.LifecycleReceipt{Disposition: executionbackend.OperationUnknown, Diagnostic: "native Orca observation has no saved implementation state"}, nil, err
	}
	turn := latestBusinessTurn(&state, snapshot)
	if turn == nil || turn.DispatchID == "" || snapshot.Ref.External.Terminal == "" {
		return executionbackend.LifecycleReceipt{Disposition: executionbackend.OperationUnknown, Diagnostic: "native Orca observation has no original Task/Dispatch"}, nil, nil
	}
	identity := snapshot.Ref.External
	receipt := executionbackend.LifecycleReceipt{Disposition: executionbackend.OperationUnknown,
		State: executionbackend.LifecycleStateFacts{SessionID: SessionID(identity), SessionGeneration: identity.Incarnation, TurnID: turn.TurnID}}
	if turn.DoneMessageID != "" {
		settled, settleErr := Settlement(ctx, l.client.MetaCall, nativeDispatchIdentity(snapshot, *turn), turn.Outcome)
		if settleErr == nil {
			turn.Settled, turn.Outcome = true, settled
			receipt.Disposition = executionbackend.OperationConfirmed
			receipt.State.TurnCompleted = true
			receipt.TurnEvidence = nativeTurnEvidence(snapshot, *turn)
			receipt.Implementation, err = l.targetEnvelope(snapshot, snapshot.Ref.Target, identity, state)
			return receipt, nil, err
		}
	}
	terminal, showErr := ShowTerminal(ctx, l.client.MetaCall, identity.Terminal)
	if showErr != nil {
		receipt.Diagnostic = showErr.Error()
		return receipt, nil, nativeCallError(showErr)
	}
	if terminal.RuntimeID != identity.RuntimeID || terminal.IncarnationID != identity.Incarnation || terminal.WorktreeID != identity.WorktreeID {
		return receipt, nil, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "original native Orca terminal or incarnation changed"}
	}
	if terminal.Exited() && turn.DoneMessageID == "" {
		receipt.Diagnostic = "original Orca terminal exited without a persisted worker_done message"
		return receipt, nil, nil
	}
	receipt.Disposition = executionbackend.OperationAccepted
	return receipt, newNativeObserver(ctx, l, snapshot), nil
}

func latestBusinessTurn(state *NativeExecutionState, snapshot executionbackend.LifecycleSnapshot) *NativeTurnState {
	// Before the start receipt commits, the center has a Command but no Turn.
	// This implementation owns the Command-to-native-Turn binding in its codec.
	if snapshot.Session.PendingCommandID != "" {
		if turn := nativeTurn(state, snapshot.Session.PendingCommandID); turn != nil {
			return turn
		}
	}
	// A pending original input takes precedence over the last completed turn.
	// Otherwise recovery would replay old settlement while a new Dispatch waits.
	if snapshot.Session.PendingTurnID != "" {
		if turn := nativeTurn(state, snapshot.Session.PendingTurnID); turn != nil {
			return turn
		}
	}
	if snapshot.Session.CurrentTurnID != "" {
		if turn := nativeTurn(state, snapshot.Session.CurrentTurnID); turn != nil {
			return turn
		}
	}
	for i := len(state.Turns) - 1; i >= 0; i-- {
		if state.Turns[i].Purpose == "business" {
			return &state.Turns[i]
		}
	}
	if len(state.Turns) > 0 {
		return &state.Turns[len(state.Turns)-1]
	}
	return nil
}

func nativeDispatchIdentity(snapshot executionbackend.LifecycleSnapshot, turn NativeTurnState) DispatchIdentity {
	identity := snapshot.Ref.External
	return DispatchIdentity{DispatchID: turn.DispatchID, TaskID: turn.TaskID, RunID: snapshot.Ref.Target.OrchestrationRun,
		RuntimeID: identity.RuntimeID, Terminal: identity.Terminal, Incarnation: identity.Incarnation, WorktreeID: identity.WorktreeID}
}

func nativeTurnEvidence(snapshot executionbackend.LifecycleSnapshot, turn NativeTurnState) *executionbackend.TurnEvidence {
	started := snapshot.Session.CurrentTurnID == turn.TurnID || snapshot.Session.LastStoppedTurnID == turn.TurnID
	if turn.Purpose == "readiness" && turn.DoneMessageID != "" {
		started = true
	}
	return &executionbackend.TurnEvidence{TurnID: turn.TurnID, CommandID: turn.TurnID, SessionID: SessionID(snapshot.Ref.External),
		Started: started, Completed: true, Succeeded: turn.Outcome == "succeeded", LastMessage: turn.LastMessage,
		Failure: map[bool]string{true: turn.LastMessage, false: ""}[turn.Outcome == "failed"], PRCandidates: append([]string(nil), turn.PRCandidates...)}
}

type nativeStopHandle struct {
	identity executionbackend.ProcessIdentity
}

func (handle *nativeStopHandle) Identity() executionbackend.ProcessIdentity { return handle.identity }

func (*nativeStopHandle) Kill() {}

func (*nativeStopHandle) Close() {}

type nativeLifecycleObserver struct {
	lifecycle *ExecutionLifecycle
	snapshot  executionbackend.LifecycleSnapshot
	state     NativeExecutionState
	turn      NativeTurnState
	stop      *nativeStopHandle
	cancel    context.CancelFunc
	events    chan executionbackend.LifecycleEvent
	done      chan error
	closed    chan struct{}
}

func newNativeObserver(parent context.Context, lifecycle *ExecutionLifecycle, snapshot executionbackend.LifecycleSnapshot) executionbackend.LifecycleObserver {
	state, found, err := lifecycle.readState(snapshot)
	turn := latestBusinessTurn(&state, snapshot)
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	observer := &nativeLifecycleObserver{lifecycle: lifecycle, snapshot: snapshot.Clone(), state: state,
		stop: &nativeStopHandle{identity: snapshot.Ref.External}, cancel: cancel,
		events: make(chan executionbackend.LifecycleEvent, 16), done: make(chan error, 1), closed: make(chan struct{})}
	if err != nil || !found || turn == nil || turn.DispatchID == "" {
		go func() {
			defer close(observer.events)
			defer close(observer.done)
			if err != nil {
				observer.done <- err
			} else {
				observer.done <- errors.New("native Orca observer has no fixed Dispatch")
			}
			close(observer.closed)
		}()
		return observer
	}
	observer.turn = *turn
	go observer.observe(ctx)
	return observer
}

func (o *nativeLifecycleObserver) observe(ctx context.Context) {
	defer close(o.events)
	defer close(o.done)
	defer close(o.closed)
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := o.processInbox(ctx); err != nil {
			select {
			case o.done <- err:
			default:
			}
			return
		}
		turn := nativeTurn(&o.state, o.turn.TurnID)
		if turn != nil && turn.DoneMessageID != "" {
			outcome, err := Settlement(ctx, o.lifecycle.client.MetaCall, nativeDispatchIdentity(o.snapshot, *turn), turn.Outcome)
			if err == nil {
				turn.Settled, turn.Outcome = true, outcome
				if !o.emitSettlement(ctx, *turn) {
					return
				}
				o.done <- nil
				return
			}
		}
		if turn != nil && turn.Started && !o.started() && !o.ensureStarted(ctx) {
			return
		}
		if turn == nil || turn.DoneMessageID == "" {
			terminal, err := ShowTerminal(ctx, o.lifecycle.client.MetaCall, o.snapshot.Ref.External.Terminal)
			if err == nil && terminal.Exited() {
				o.done <- errors.New("original Orca terminal exited before worker_done was committed")
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (o *nativeLifecycleObserver) processInbox(ctx context.Context) error {
	identity := o.state.Coordinator
	if identity.Terminal == "" {
		return errors.New("native Orca inbox has no original coordinator")
	}
	inbox, err := ReadDelivery(ctx, o.lifecycle.client.MetaCall, identity.Terminal, identity.RuntimeID, o.snapshot.Ref.Target.OrchestrationRun)
	if err != nil {
		return err
	}
	if len(inbox.Messages) == 0 {
		return nil
	}
	fixed := o.snapshot.Session
	fixed.TaskID, fixed.CurrentTurnID = o.turn.TaskID, o.turn.TurnID
	batch, err := DecodeInbox(inbox, o.turn.TaskID, o.turn.DispatchID, o.snapshot.Ref.External.Terminal, fixed)
	if err != nil {
		return err
	}
	o.state.DeliveryID = inbox.DeliveryID
	for index := range inbox.Messages {
		fact := batch.Facts[index]
		if fact.SourceEventID == "" {
			continue
		}
		if fact.Matched && !o.readinessTurn() && !o.ensureStarted(ctx) {
			return errors.New("native Orca turn start could not be committed before its first worker message")
		}
		turn := nativeTurn(&o.state, o.turn.TurnID)
		if turn == nil {
			return errors.New("native Orca inbox lost its current turn")
		}
		if fact.Kind == executionbackend.EventCompleted {
			turn.DoneMessageID, turn.Outcome, turn.LastMessage = fact.SourceEventID, fact.Outcome, fact.LastMessage
			turn.PRCandidates = append([]string(nil), fact.PRCandidates...)
		}
		seen := false
		for _, id := range o.state.SeenMessages {
			seen = seen || id == fact.SourceEventID
		}
		if !seen {
			o.state.SeenMessages = append(o.state.SeenMessages, fact.SourceEventID)
		}
		envelopes, err := o.lifecycle.targetEnvelope(o.snapshot, o.snapshot.Ref.Target, o.snapshot.Ref.External, o.state)
		if err != nil {
			return err
		}
		event := executionbackend.LifecycleEvent{Fact: fact, Implementation: envelopes, DeliveryID: inbox.DeliveryID,
			DeliveryFinal: index == len(batch.Facts)-1, Summary: nativeFactSummary(fact), Kind: "execution",
			Status: string(fact.Kind), Title: nativeFactTitle(fact), Persisted: make(chan error, 1), Processed: make(chan error, 1)}
		if !o.sendAndWait(ctx, event) {
			return errors.New("native Orca event was not committed before observation ended")
		}
		if event.DeliveryFinal {
			o.state.DeliveryID = ""
		}
	}
	return nil
}

func (o *nativeLifecycleObserver) started() bool {
	return o.snapshot.Session.CurrentTurnID == o.turn.TurnID
}

func (o *nativeLifecycleObserver) readinessTurn() bool { return o.turn.Purpose == "readiness" }

func (o *nativeLifecycleObserver) ensureStarted(ctx context.Context) bool {
	if o.started() {
		return true
	}
	event := executionbackend.LifecycleEvent{Fact: executionbackend.EventFact{Kind: executionbackend.EventStarted,
		StartedTurnID: o.turn.TurnID, SessionID: SessionID(o.snapshot.Ref.External)},
		Summary: "native Orca turn start observed", Kind: "execution", Status: "started", Title: "Worker turn started",
		Persisted: make(chan error, 1), Processed: make(chan error, 1)}
	if !o.sendAndWait(ctx, event) {
		return false
	}
	o.snapshot.Session.CurrentTurnID = o.turn.TurnID
	return true
}

func (o *nativeLifecycleObserver) emitSettlement(ctx context.Context, turn NativeTurnState) bool {
	if err := o.lifecycle.freezeOriginalOutput(ctx, o.snapshot, turn); err != nil {
		o.done <- err
		return false
	}
	envelopes, err := o.lifecycle.targetEnvelope(o.snapshot, o.snapshot.Ref.Target, o.snapshot.Ref.External, o.state)
	if err != nil {
		o.done <- err
		return false
	}
	fact := executionbackend.EventFact{Kind: executionbackend.EventStatus, Source: "native_orca_settlement", SourceEventID: "settled:" + turn.DispatchID, Matched: true,
		Outcome: turn.Outcome, Completed: true, LastMessage: turn.LastMessage, PRCandidates: append([]string(nil), turn.PRCandidates...)}
	event := executionbackend.LifecycleEvent{Fact: fact, Implementation: envelopes,
		Summary: "native Orca Dispatch settlement verified", Kind: "execution", Status: "settled", Title: "Worker dispatch settled",
		Persisted: make(chan error, 1), Processed: make(chan error, 1)}
	return o.sendAndWait(ctx, event)
}

func (o *nativeLifecycleObserver) sendAndWait(ctx context.Context, event executionbackend.LifecycleEvent) bool {
	event = event.Clone()
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

func nativeFactSummary(fact executionbackend.EventFact) string {
	if fact.LastMessage != "" {
		return fact.LastMessage
	}
	if fact.QuestionText != "" {
		return fact.QuestionText
	}
	return string(fact.Kind)
}

func nativeFactTitle(fact executionbackend.EventFact) string {
	switch fact.Kind {
	case executionbackend.EventQuestion:
		return "Orca worker question"
	case executionbackend.EventCompleted:
		return "Orca worker completion"
	case executionbackend.EventEscalation:
		return "Orca worker escalation"
	case executionbackend.EventStatus:
		return "Orca worker status"
	default:
		return "Orca worker event"
	}
}

func (o *nativeLifecycleObserver) Events() <-chan executionbackend.LifecycleEvent { return o.events }

func (o *nativeLifecycleObserver) Done() <-chan error { return o.done }

func (o *nativeLifecycleObserver) StopHandle() executionbackend.StopHandle { return o.stop }

func (o *nativeLifecycleObserver) Close(ctx context.Context) error {
	o.cancel()
	select {
	case <-o.closed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

var _ executionbackend.LifecycleObserver = (*nativeLifecycleObserver)(nil)
