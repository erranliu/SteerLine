package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (l *ExecutionLifecycle) startTurn(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	input := request.Input.Turn
	if input == nil || request.Snapshot.PendingTurn == nil || input.CommandID != request.Snapshot.PendingTurn.CommandID || input.Prompt != request.Snapshot.PendingTurn.Prompt {
		return nativeReceipt(request, executionapi.OperationRejected), nil, errors.New("native Orca Dispatch input differs from the original pending turn")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	turn := nativeTurn(&state, input.CommandID)
	if turn == nil || turn.TaskID == "" || turn.DispatchID != "" || turn.Settled || !state.AgentBound || !state.RelayIgnored {
		return nativeReceipt(request, executionapi.OperationRejected), nil, errors.New("native Orca Dispatch lacks a committed Task, agent binding or relay path")
	}
	process := request.Snapshot.Ref.External
	if err := l.verifyAgent(ctx, request.Snapshot, process); err != nil {
		return receipt, nil, err
	}
	if state.Coordinator.Terminal == "" || request.Snapshot.Ref.Target.OrchestrationRun == "" {
		return receipt, nil, errors.New("native Orca Dispatch has no committed Run/coordinator")
	}
	identity := DispatchIdentity{TaskID: turn.TaskID, RunID: request.Snapshot.Ref.Target.OrchestrationRun,
		RuntimeID: process.RuntimeID, Terminal: process.Terminal, Incarnation: process.Incarnation, WorktreeID: process.WorktreeID}
	result, err := DispatchToAgent(ctx, l.client.MetaCall, turn.TaskID, request.Snapshot.Ref.Target.OrchestrationRun,
		state.Coordinator.Terminal, process, request.Snapshot.Workspace.Path, request.Snapshot.Workspace.Branch,
		"codex", positiveDuration(l.orca.WaitTimeout, 2*time.Minute), false, state.Coordinator, l.setup.RepositoryPath)
	if err != nil {
		if NoAgentDetected(result) {
			receipt.Disposition = executionapi.OperationRejected
		} else {
			receipt.Disposition = executionapi.OperationUnknown
		}
		receipt.Diagnostic = err.Error()
		return receipt, nil, nativeCallError(err)
	}
	created, err := DispatchReceipt(result, turn.TaskID, request.Snapshot.Ref.Target.OrchestrationRun, state.Coordinator.RuntimeID)
	if err != nil {
		receipt.Diagnostic = err.Error()
		return receipt, nil, err
	}
	identity.DispatchID = created.ID
	if err := VerifyRecoveredDispatch(ctx, l.client.MetaCall, identity); err != nil {
		receipt.Diagnostic = err.Error()
		return receipt, nil, err
	}
	turn.DispatchID, turn.DispatchRequestID = created.ID, created.RequestID
	turn.InputSubmissionPending = PromptAwaitingSubmission(created.Payload, process.Incarnation)
	process = l.boundProcess(request.Snapshot, request.Snapshot.Ref.Target, turn)
	target := request.Snapshot.Ref.Target
	target.ExternalTask, target.Dispatch = turn.TaskID, turn.DispatchID
	target.OrchestrationRun = request.Snapshot.Ref.Target.OrchestrationRun
	receipt.Target, receipt.Process = &target, &process
	receipt.RequestID, receipt.Disposition = created.RequestID, executionapi.OperationAccepted
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	receipt.State.SessionID, receipt.State.SessionGeneration = SessionID(process), process.Incarnation
	receipt.State.TurnID, receipt.State.InputAccepted = turn.TurnID, true
	receipt.State.ResourceID = turn.DispatchID
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, process, state)
	if err != nil {
		return receipt, nil, err
	}
	if turn.InputSubmissionPending {
		return receipt, nil, nil
	}
	return receipt, newNativeObserver(ctx, l, l.boundSnapshot(request.Snapshot, target, process, state)), nil
}

func (l *ExecutionLifecycle) reconcileStart(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	input := request.Input.Turn
	if input == nil {
		return receipt, nil, errors.New("native Orca Dispatch reconciliation lost its original input")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	turn := nativeTurn(&state, input.CommandID)
	if turn == nil || turn.TaskID == "" {
		return receipt, nil, errors.New("original Orca Task is not committed")
	}
	process := request.Snapshot.Ref.External
	if err := l.verifyAgent(ctx, request.Snapshot, process); err != nil {
		return receipt, nil, err
	}
	dispatchID, err := RecoverDispatch(ctx, l.client.MetaCall, turn.TaskID, request.Snapshot.Ref.Target.OrchestrationRun, process)
	if err != nil {
		return receipt, nil, nativeCallError(err)
	}
	turn.DispatchID = dispatchID
	turn.DispatchRequestID = request.Operation.ID
	turn.InputSubmissionPending = PromptAwaitingSubmission(savedNativePayload(request.Operation), process.Incarnation)
	if !turn.InputSubmissionPending {
		pending, inspectErr := InspectPendingPaste(ctx, l.client.MetaCall, workerTerminalIdentity(process))
		if inspectErr != nil {
			return receipt, nil, nativeCallError(inspectErr)
		}
		turn.InputSubmissionPending = pending
	}
	process = l.boundProcess(request.Snapshot, request.Snapshot.Ref.Target, turn)
	target := request.Snapshot.Ref.Target
	target.ExternalTask, target.Dispatch = turn.TaskID, turn.DispatchID
	receipt.Target, receipt.Process = &target, &process
	receipt.RequestID, receipt.Disposition = turn.DispatchRequestID, executionapi.OperationAccepted
	receipt.State.SessionID, receipt.State.SessionGeneration = SessionID(process), process.Incarnation
	receipt.State.TurnID, receipt.State.InputAccepted, receipt.State.ResourceID = turn.TurnID, true, turn.DispatchID
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, process, state)
	if err != nil || turn.InputSubmissionPending {
		return receipt, nil, err
	}
	return receipt, newNativeObserver(ctx, l, l.boundSnapshot(request.Snapshot, target, process, state)), nil
}

func (l *ExecutionLifecycle) submitInput(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	input := request.Input.Turn
	if input == nil {
		return nativeReceipt(request, executionapi.OperationRejected), nil, errors.New("native Orca input submission is missing its turn")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	turn := nativeTurn(&state, input.CommandID)
	process := request.Snapshot.Ref.External
	if turn == nil || turn.DispatchID == "" || !turn.InputSubmissionPending {
		return nativeReceipt(request, executionapi.OperationRejected), nil, errors.New("native Orca has no committed pending paste submission")
	}
	if err := l.verifyAgent(ctx, request.Snapshot, process); err != nil {
		return receipt, nil, err
	}
	_, err = SubmitPendingPaste(ctx, l.client.MetaCall, workerTerminalIdentity(process))
	if err != nil {
		receipt.Diagnostic = err.Error()
		return receipt, nil, nativeCallError(err)
	}
	turn.InputSubmissionPending = false
	process = l.boundProcess(request.Snapshot, request.Snapshot.Ref.Target, turn)
	receipt.Process = &process
	receipt.RequestID, receipt.Disposition = turn.DispatchRequestID, executionapi.OperationAccepted
	receipt.State.SessionID, receipt.State.SessionGeneration = SessionID(process), process.Incarnation
	receipt.State.TurnID, receipt.State.InputAccepted, receipt.State.ResourceID = turn.TurnID, true, turn.DispatchID
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, process, state)
	if err != nil {
		return receipt, nil, err
	}
	return receipt, newNativeObserver(ctx, l, l.boundSnapshot(request.Snapshot, request.Snapshot.Ref.Target, process, state)), nil
}

func (l *ExecutionLifecycle) reconcileInput(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	input := request.Input.Turn
	state, _, err := l.readState(request.Snapshot)
	if err != nil || input == nil {
		return receipt, nil, errors.Join(err, errors.New("native Orca input reconciliation lost its fixed turn"))
	}
	turn := nativeTurn(&state, input.CommandID)
	process := request.Snapshot.Ref.External
	if turn == nil || turn.DispatchID == "" {
		return receipt, nil, errors.New("native Orca input reconciliation has no original Dispatch")
	}
	if err := l.verifyAgent(ctx, request.Snapshot, process); err != nil {
		return receipt, nil, err
	}
	pending, err := InspectPendingPaste(ctx, l.client.MetaCall, workerTerminalIdentity(process))
	if err != nil {
		return receipt, nil, nativeCallError(err)
	}
	if pending {
		receipt.Diagnostic = "the original paste remains visible; an unknown Enter is never resent"
		return receipt, nil, nil
	}
	turn.InputSubmissionPending = false
	process = l.boundProcess(request.Snapshot, request.Snapshot.Ref.Target, turn)
	receipt.Process = &process
	receipt.RequestID, receipt.Disposition = turn.DispatchRequestID, executionapi.OperationConfirmed
	receipt.State.SessionID, receipt.State.SessionGeneration = SessionID(process), process.Incarnation
	receipt.State.TurnID, receipt.State.InputAccepted, receipt.State.ResourceID = turn.TurnID, true, turn.DispatchID
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, process, state)
	if err != nil {
		return receipt, nil, err
	}
	return receipt, newNativeObserver(ctx, l, l.boundSnapshot(request.Snapshot, request.Snapshot.Ref.Target, process, state)), nil
}

func (l *ExecutionLifecycle) reportCompletion(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	input := request.Input.Completion
	state, _, err := l.readState(request.Snapshot)
	if err != nil || input == nil {
		return receipt, errors.Join(err, errors.New("native Orca completion report lost its fixed input"))
	}
	turn := nativeTurn(&state, input.TurnID)
	process := request.Snapshot.Ref.External
	if turn == nil || turn.DispatchID == "" || process.Terminal == "" {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca completion report has no fixed Dispatch")
	}
	path, err := RelayPath(request.Snapshot.Workspace.Path, request.Snapshot.Session.WorkflowRunID, turn.TurnID)
	if err != nil {
		return receipt, err
	}
	relay, err := ReadDoneRequest(path, request.Snapshot.Workspace.Path, request.Snapshot.Session.WorkflowRunID, turn.TurnID, turn.TaskID, turn.DispatchID)
	if err != nil || relay == nil {
		return receipt, errors.Join(err, errors.New("native Orca relay request is unavailable"))
	}
	if relay.Outcome != input.Outcome || relay.Subject != input.Subject || relay.Body != input.Body {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca completion changed the saved relay body")
	}
	dispatch := DispatchIdentity{DispatchID: turn.DispatchID, TaskID: turn.TaskID, RunID: request.Snapshot.Ref.Target.OrchestrationRun,
		RuntimeID: process.RuntimeID, Terminal: process.Terminal, Incarnation: process.Incarnation, WorktreeID: process.WorktreeID}
	status, err := VerifyActiveDispatch(ctx, l.client.MetaCall, dispatch)
	if err != nil || status != "dispatched" {
		return receipt, errors.Join(err, errors.New("native Orca relay is not bound to a live Dispatch"))
	}
	report, err := SendDone(ctx, l.client.MetaCall, *relay, request.Snapshot.Ref.Target.OrchestrationRun, process.Terminal, process.RuntimeID)
	if err != nil {
		return receipt, nativeCallError(err)
	}
	turn.RelayReported = true
	receipt.Disposition, receipt.RequestID = executionapi.OperationAccepted, report.RequestID
	receipt.Operation.Receipt = append([]byte(nil), report.Payload...)
	receipt.State.ResourceID = report.ID
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, process, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileCompletion(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	input := request.Input.Completion
	state, _, err := l.readState(request.Snapshot)
	if err != nil || input == nil {
		return receipt, nil, errors.Join(err, errors.New("native Orca completion reconciliation lost its fixed input"))
	}
	turn := nativeTurn(&state, input.TurnID)
	if turn == nil {
		return receipt, nil, errors.New("native Orca completion reconciliation has no original turn")
	}
	id, err := FindDone(ctx, l.client.Call, request.Snapshot.Ref.Target.OrchestrationRun, request.Snapshot.Ref.External.Terminal,
		turn.TaskID, turn.DispatchID, input.Subject, input.Body, input.Outcome)
	if err != nil || id == "" {
		return receipt, nil, err
	}
	turn.RelayReported = true
	receipt.Disposition, receipt.RequestID = executionapi.OperationConfirmed, id
	receipt.State.ResourceID = id
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, request.Snapshot.Ref.External, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) deliverFeedback(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	input := request.Input.Feedback
	if input == nil {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca feedback is missing")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, err
	}
	if request.Snapshot.QuestionRoute == nil {
		turn := latestBusinessTurn(&state, request.Snapshot)
		if turn == nil || turn.DispatchID == "" || input.Message == "" {
			return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca status feedback has no active original Dispatch")
		}
		subject := "SteerLine feedback " + input.DecisionID
		sent, err := SendFeedback(ctx, l.client.MetaCall, subject, input.Message, turn.DispatchID,
			request.Snapshot.Ref.Target.OrchestrationRun, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
		if err != nil {
			return receipt, nativeCallError(err)
		}
		receipt.Disposition, receipt.RequestID = executionapi.OperationConfirmed, sent.RequestID
		receipt.State.InputAccepted = true
		receipt.Operation.Receipt = append([]byte(nil), sent.Payload...)
		return receipt, nil
	}
	if input.Answer == nil {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca question answer has no structured values")
	}
	turn, route, err := l.questionRoute(request.Snapshot, state)
	if err != nil {
		return receipt, err
	}
	body := strings.Join(input.Answer.Answers[route.MessageID], "\n\n")
	if body == "" {
		body = input.Message
	}
	if body == "" {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca structured answer is empty")
	}
	answer, err := ReplyQuestion(ctx, l.client.MetaCall, route.MessageID, body, request.Snapshot.Ref.Target.OrchestrationRun,
		state.Coordinator.Terminal, turn.DispatchID, state.Coordinator.RuntimeID)
	if err != nil {
		return receipt, nativeCallError(err)
	}
	receipt.Disposition, receipt.RequestID = executionapi.OperationConfirmed, answer.RequestID
	receipt.State.InputAccepted = true
	receipt.Operation.Receipt = append([]byte(nil), answer.Payload...)
	return receipt, nil
}

func (l *ExecutionLifecycle) reconcileFeedback(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	input := request.Input.Feedback
	if input == nil {
		return receipt, nil, errors.New("native Orca feedback reconciliation lost its input")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	if request.Snapshot.QuestionRoute == nil {
		turn := latestBusinessTurn(&state, request.Snapshot)
		if turn == nil || turn.DispatchID == "" {
			return receipt, nil, errors.New("native Orca status feedback reconciliation lost its Dispatch")
		}
		subject := "SteerLine feedback " + input.DecisionID
		id, err := FindSend(ctx, l.client.Call, request.Snapshot.Ref.Target.OrchestrationRun, turn.DispatchID, subject, input.Message)
		if err != nil || id == "" {
			return receipt, nil, err
		}
		receipt.Disposition, receipt.RequestID, receipt.State.InputAccepted = executionapi.OperationConfirmed, id, true
		return receipt, nil, nil
	}
	if input.Answer == nil {
		return receipt, nil, errors.New("native Orca answer reconciliation has no structured values")
	}
	turn, route, err := l.questionRoute(request.Snapshot, state)
	if err != nil {
		return receipt, nil, err
	}
	body := strings.Join(input.Answer.Answers[route.MessageID], "\n\n")
	if body == "" {
		body = input.Message
	}
	id, err := FindReply(ctx, l.client.Call, request.Snapshot.Ref.Target.OrchestrationRun, turn.DispatchID, route.MessageID, body)
	if err != nil || id == "" {
		return receipt, nil, err
	}
	receipt.Disposition, receipt.RequestID = executionapi.OperationConfirmed, id
	receipt.State.InputAccepted = true
	return receipt, nil, nil
}

func (l *ExecutionLifecycle) questionRoute(snapshot executionapi.LifecycleSnapshot, state NativeExecutionState) (*NativeTurnState, QuestionRouteData, error) {
	if snapshot.QuestionRoute == nil {
		return nil, QuestionRouteData{}, errors.New("native Orca question route is missing")
	}
	for i := range state.Turns {
		turn := &state.Turns[i]
		fixed := snapshot.Session
		fixed.TaskID, fixed.CurrentTurnID = turn.TaskID, turn.TurnID
		fixed.Generation = snapshot.Ref.External.Incarnation
		data, err := DecodeQuestionRoute(*snapshot.QuestionRoute, fixed, turn.TaskID, turn.DispatchID, snapshot.Ref.External.Terminal)
		if err == nil {
			return turn, data, nil
		}
	}
	return nil, QuestionRouteData{}, errors.New("native Orca question route differs from every saved Turn/Dispatch")
}

func (l *ExecutionLifecycle) acknowledge(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	id := request.Input.Ack.DeliveryID
	state, _, err := l.readState(request.Snapshot)
	if err != nil || id == "" || state.DeliveryID != id || state.Coordinator.Terminal == "" {
		return receipt, errors.Join(err, errors.New("native Orca acknowledgement differs from its saved delivery"))
	}
	ack, err := AckInbox(ctx, l.client.MetaCall, state.Coordinator.Terminal, request.Snapshot.Ref.Target.OrchestrationRun, id, state.Coordinator.RuntimeID)
	if err != nil {
		return receipt, nativeCallError(err)
	}
	state.DeliveryID = ""
	receipt.Disposition, receipt.RequestID = executionapi.OperationConfirmed, ack.RequestID
	receipt.Operation.Receipt = append([]byte(nil), ack.Payload...)
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, request.Snapshot.Ref.External, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileAck(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	id := request.Input.Ack.DeliveryID
	state, _, err := l.readState(request.Snapshot)
	if err != nil || id == "" || state.Coordinator.Terminal == "" {
		return receipt, nil, errors.Join(err, errors.New("native Orca acknowledgement reconciliation lost its binding"))
	}
	acknowledged, err := DeliveryAcknowledged(ctx, l.client.MetaCall, state.Coordinator.Terminal, state.Coordinator.RuntimeID,
		request.Snapshot.Ref.Target.OrchestrationRun, id)
	if err != nil || !acknowledged {
		return receipt, nil, err
	}
	state.DeliveryID = ""
	receipt.Disposition = executionapi.OperationConfirmed
	receipt.RequestID = id
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, request.Snapshot.Ref.External, state)
	return receipt, nil, err
}

func workerTerminalIdentity(identity executionapi.ProcessIdentity) TerminalIdentity {
	return TerminalIdentity{Kind: "orca-native", Handle: identity.Terminal, RuntimeID: identity.RuntimeID, Incarnation: identity.Incarnation}
}

func (l *ExecutionLifecycle) verifyAgent(ctx context.Context, snapshot executionapi.LifecycleSnapshot, identity executionapi.ProcessIdentity) error {
	if identity.Kind != "orca" || identity.Terminal == "" || identity.RuntimeID == "" || identity.Incarnation == "" || identity.WorktreeID != snapshot.Ref.Target.WorktreeID {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca agent binding is incomplete"}
	}
	return VerifyAgentTerminal(ctx, l.client.MetaCall, identity, snapshot.Workspace.Path, snapshot.Workspace.Branch, "codex")
}

func (l *ExecutionLifecycle) boundSnapshot(snapshot executionapi.LifecycleSnapshot, target executionapi.ExecutionTarget,
	process executionapi.ProcessIdentity, state NativeExecutionState) executionapi.LifecycleSnapshot {
	bound := snapshot.Clone()
	bound.Ref.Target, bound.Session.Target, bound.Ref.External = target, target, process
	if envelope, err := l.stateEnvelope(bound, target, process, state); err == nil {
		bound.Implementation = []executionapi.ImplementationDataEnvelope{envelope}
	}
	return bound
}

func (l *ExecutionLifecycle) cleanup(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	if len(request.Input.Cleanup.Resources) != 1 {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca cleanup must name exactly one original resource")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, err
	}
	identity := request.Input.Cleanup.Resources[0]
	role := "agent"
	if identity == state.Coordinator {
		role = "coordinator"
		if err := InboxDrained(ctx, l.client.MetaCall, state.Coordinator.Terminal, state.Coordinator.RuntimeID, request.Snapshot.Ref.Target.OrchestrationRun); err != nil {
			return receipt, err
		}
		state.CoordinatorClosed = true
	} else if identity == request.Snapshot.Ref.External {
		state.AgentClosed = true
	} else {
		return nativeReceipt(request, executionapi.OperationRejected), &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca cleanup differs from the fixed resource list"}
	}
	if identity.Terminal == "" || identity.RuntimeID == "" || identity.Incarnation == "" {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca cleanup lacks original terminal identity")
	}
	absent, err := InspectCleanupTerminal(ctx, l.client.MetaCall, identity, l.setup.RepositoryPath, role, false)
	if err != nil {
		return receipt, nativeCallError(err)
	}
	if !absent {
		closed, closeErr := CloseTerminal(ctx, l.client.MetaCall, identity.Terminal, identity.RuntimeID)
		if closeErr != nil {
			return receipt, nativeCallError(closeErr)
		}
		receipt.Operation.Receipt = append([]byte(nil), closed...)
	}
	receipt.Disposition = executionapi.OperationConfirmed
	receipt.RequestID = identity.Terminal
	receipt.State.ResourceID = identity.Terminal
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, request.Snapshot.Ref.External, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileCleanup(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	if len(request.Input.Cleanup.Resources) != 1 {
		return nativeReceipt(request, executionapi.OperationRejected), nil, errors.New("native Orca cleanup reconciliation must name one resource")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	identity := request.Input.Cleanup.Resources[0]
	role := "agent"
	if identity == state.Coordinator {
		role = "coordinator"
		if err := InboxDrained(ctx, l.client.MetaCall, state.Coordinator.Terminal, state.Coordinator.RuntimeID, request.Snapshot.Ref.Target.OrchestrationRun); err != nil {
			return receipt, nil, err
		}
	} else if identity != request.Snapshot.Ref.External {
		return nativeReceipt(request, executionapi.OperationRejected), nil, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca cleanup reconciliation changed its original terminal"}
	}
	absent, err := InspectCleanupTerminal(ctx, l.client.MetaCall, identity, l.setup.RepositoryPath, role, true)
	if err != nil || !absent {
		return receipt, nil, nativeCallError(err)
	}
	if role == "agent" {
		state.AgentClosed = true
	} else {
		state.CoordinatorClosed = true
	}
	receipt.Disposition, receipt.RequestID = executionapi.OperationConfirmed, identity.Terminal
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, request.Snapshot.Ref.External, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) StopLifecycle(ctx context.Context, request executionapi.LifecycleRequest, _ executionapi.StopHandle) (executionapi.LifecycleReceipt, error) {
	if err := request.Validate(); err != nil {
		return nativeReceipt(request, executionapi.OperationRejected), err
	}
	if request.Operation.Action == string(executionapi.LifecycleStopObservation) {
		return nativeReceipt(request, executionapi.OperationConfirmed), nil
	}
	if request.Operation.Action != string(executionapi.LifecycleStopExecution) || request.Snapshot.Ref.External.Terminal == "" {
		return nativeReceipt(request, executionapi.OperationRejected), executionapi.UnsupportedAdapterOperation(request.Operation.Action, "native Orca stop has no original terminal")
	}
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	if err := l.verifyAgent(ctx, request.Snapshot, request.Snapshot.Ref.External); err != nil {
		return receipt, err
	}
	closed, err := l.client.CloseTerminalChecked(ctx, request.Snapshot.Ref.External.Terminal, request.Snapshot.Ref.External.RuntimeID)
	if err != nil {
		return receipt, nativeCallError(err)
	}
	receipt.Disposition = executionapi.OperationStatus(closed.Disposition)
	receipt.Diagnostic = closed.Diagnostic
	receipt.RequestID = request.Snapshot.Ref.External.Terminal
	return receipt, nil
}

func (l *ExecutionLifecycle) CleanupLifecycle(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	return l.cleanup(ctx, request)
}

func (l *ExecutionLifecycle) VerifyManualRecovery(ctx context.Context, snapshot executionapi.LifecycleSnapshot) (executionapi.LifecycleRecoveryEvidence, error) {
	if err := l.validateSnapshot(snapshot); err != nil {
		return executionapi.LifecycleRecoveryEvidence{}, err
	}
	identity := snapshot.Ref.External
	if identity.Terminal == "" {
		return executionapi.LifecycleRecoveryEvidence{ProcessAbsent: true}, nil
	}
	terminal, err := l.client.ShowTerminal(ctx, identity.Terminal)
	if err != nil {
		return executionapi.LifecycleRecoveryEvidence{}, err
	}
	if terminal.RuntimeID != identity.RuntimeID || terminal.IncarnationID != identity.Incarnation || terminal.WorktreeID != identity.WorktreeID {
		return executionapi.LifecycleRecoveryEvidence{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca recovery terminal identity changed"}
	}
	if err := VerifyAgentTerminal(ctx, l.client.MetaCall, identity, snapshot.Workspace.Path, snapshot.Workspace.Branch, "codex"); err != nil {
		return executionapi.LifecycleRecoveryEvidence{}, err
	}
	return executionapi.LifecycleRecoveryEvidence{ManagedProcess: true, ProcessAbsent: terminal.Exited(), SessionAvailable: !terminal.Exited()}, nil
}

func (l *ExecutionLifecycle) BindRecovery(ctx context.Context, previous, next executionapi.LifecycleSnapshot) (executionapi.ImplementationDataEnvelope, error) {
	if err := l.validateSnapshot(previous); err != nil {
		return executionapi.ImplementationDataEnvelope{}, err
	}
	if err := sameNativeTarget(previous.Ref, next.Ref); err != nil {
		return executionapi.ImplementationDataEnvelope{}, err
	}
	identity := previous.Ref.External
	if identity.Terminal == "" {
		return executionapi.ImplementationDataEnvelope{}, errors.New("native Orca recovery has no original agent terminal")
	}
	if err := VerifyAgentTerminal(ctx, l.client.MetaCall, identity, previous.Workspace.Path, previous.Workspace.Branch, "codex"); err != nil {
		return executionapi.ImplementationDataEnvelope{}, err
	}
	state, found, err := l.readState(previous)
	if err != nil || !found {
		return executionapi.ImplementationDataEnvelope{}, errors.Join(err, errors.New("native Orca recovery lost its implementation binding"))
	}
	return NewExecutionStateEnvelope(ExecutionStateIdentity{ProjectID: next.Session.ProjectID, ExecutionID: next.Session.ExecutionID,
		AttemptID: next.Session.AttemptID, Generation: next.ResourceGeneration, ConfigDigest: next.ConfigurationDigest,
		Protocol: next.Ref.Protocol, Process: identity, CoordinatorTerminal: state.Coordinator.Terminal}, state)
}

var _ executionapi.LifecycleRecoveryPort = (*ExecutionLifecycle)(nil)

var _ = json.Valid
