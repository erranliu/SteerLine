package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (l *ExecutionLifecycle) startTurn(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	input := request.Input.Turn
	if input == nil || request.Snapshot.PendingTurn == nil || input.CommandID != request.Snapshot.PendingTurn.CommandID || input.Prompt != request.Snapshot.PendingTurn.Prompt {
		return nativeReceipt(request, executionbackend.OperationRejected), nil, errors.New("native Orca Dispatch input differs from the original pending turn")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	turn := nativeTurn(&state, input.CommandID)
	if turn == nil || turn.TaskID == "" || turn.DispatchID != "" || turn.Settled || state.Coordinator.Terminal == "" || request.Snapshot.Ref.Target.OrchestrationRun == "" {
		return nativeReceipt(request, executionbackend.OperationRejected), nil, errors.New("native Orca start lacks its original Task/Run/coordinator")
	}
	result, started, process, err := l.startNativeWorker(ctx, request.Snapshot, state, *turn, request.Operation.ID)
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	if err != nil {
		recovered, recoverErr := l.recoverFailedStart(ctx, request, state, started, result, err)
		return recovered, nil, nativeCallError(recoverErr)
	}
	created := MutationReceipt{ID: started.DispatchID, RequestID: nativeRequestID(request.Operation.ID), Payload: result.Payload}
	turn.DispatchID, turn.DispatchRequestID = created.ID, created.RequestID
	turn.InputSubmissionPending = false
	state.AgentBound = true
	turn.Started = started.TurnStart == "observed"
	process.Task, process.Dispatch, process.OrchestrationRun = turn.TaskID, turn.DispatchID, request.Snapshot.Ref.Target.OrchestrationRun
	target := request.Snapshot.Ref.Target
	target.ExternalTask, target.Dispatch = turn.TaskID, turn.DispatchID
	target.OrchestrationRun = request.Snapshot.Ref.Target.OrchestrationRun
	receipt.Target, receipt.Process = &target, &process
	receipt.RequestID, receipt.Disposition = created.RequestID, executionbackend.OperationAccepted
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	receipt.State.SessionID, receipt.State.SessionGeneration = SessionID(process), process.Incarnation
	receipt.State.TurnID, receipt.State.InputAccepted = turn.TurnID, true
	receipt.State.TurnStarted = turn.Started
	receipt.State.ResourceID = turn.DispatchID
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, process, state)
	if err != nil {
		return receipt, nil, err
	}
	if err := l.saveOutputBinding(request.Snapshot, process, *turn); err != nil {
		return receipt, nil, err
	}
	return receipt, newNativeObserver(ctx, l, l.boundSnapshot(request.Snapshot, target, process, state)), nil
}

func (l *ExecutionLifecycle) reconcileStart(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	input := request.Input.Turn
	if input == nil {
		return receipt, nil, errors.New("original worker input is missing")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	turn := nativeTurn(&state, input.CommandID)
	if turn == nil || turn.TaskID == "" {
		return receipt, nil, errors.New("original Orca Task is not committed")
	}
	if state.FailedStartReleased || state.PartialWorker != nil {
		identity := partialWorker{RunID: request.Snapshot.Ref.Target.OrchestrationRun, TaskID: turn.TaskID,
			DispatchID: request.Snapshot.Ref.Target.Dispatch, RuntimeID: state.Coordinator.RuntimeID}
		if state.PartialWorker != nil {
			identity = *state.PartialWorker
		}
		if identity.RunID != request.Snapshot.Ref.Target.OrchestrationRun || identity.TaskID != turn.TaskID ||
			identity.DispatchID == "" || identity.DispatchID != request.Snapshot.Ref.Target.Dispatch || identity.RuntimeID != state.Coordinator.RuntimeID {
			return receipt, nil, errors.New("failed start lost its original resource identity")
		}
		shown, proof, observeErr := observeOriginalWorkerResource(ctx, l.client.MetaCall, identity.identity(request.Snapshot.Ref.Target.WorktreeID))
		// Keep the saved native effects without nesting the whole prior lifecycle
		// receipt on every unavailable observation.
		receipt.Operation.Receipt = nil
		var prior executionbackend.LifecycleReceipt
		if json.Unmarshal(request.Operation.Receipt, &prior) == nil && prior.Operation.ID == request.Operation.ID {
			receipt.Operation.Receipt = prior.Operation.Receipt
		}
		receipt.RequestID = nativeRequestID(request.Operation.ID)
		if observeErr != nil {
			return receipt, nil, observeErr
		}
		if shown.Resource.ReleaseState == "released" {
			effects := map[string]json.RawMessage{"release_observation": proof}
			if len(receipt.Operation.Receipt) > 0 {
				effects["original_start"] = receipt.Operation.Receipt
			}
			receipt.Operation.Receipt, _ = json.Marshal(effects)
			receipt.Disposition = executionbackend.OperationRejected
			receipt.State.Stopped, receipt.State.WorkerAbsent, receipt.State.WritersAbsent = true, true, true
			receipt.Diagnostic = "Original Orca worker start failed and its exact resource was released; handle this execution manually."
			return receipt, nil, nil
		}
		if state.FailedStartReleased {
			return receipt, nil, errors.New("original failed-start release is no longer verifiable")
		}
	}
	original := request.Snapshot.Clone()
	if request.Operation.Ref != nil {
		original.Ref = *request.Operation.Ref
	}
	result, started, process, err := l.startNativeWorker(ctx, original, state, *turn, request.Operation.ID)
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	receipt.RequestID = nativeRequestID(request.Operation.ID)
	if err != nil {
		recovered, recoverErr := l.recoverFailedStart(ctx, request, state, started, result, err)
		return recovered, nil, recoverErr
	}
	id, run := started.DispatchID, started.RunID
	turn.Started = started.TurnStart == "observed"
	process.OrchestrationRun = run
	turn.DispatchID = id
	state.AgentBound = true
	target := request.Snapshot.Ref.Target
	target.ExternalTask, target.Dispatch = turn.TaskID, id
	receipt.Target, receipt.Process = &target, &process
	receipt.RequestID = nativeRequestID(request.Operation.ID)
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.State.SessionID, receipt.State.SessionGeneration = SessionID(process), process.Incarnation
	receipt.State.TurnID, receipt.State.InputAccepted, receipt.State.ResourceID = turn.TurnID, true, id
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, process, state)
	if err != nil {
		return receipt, nil, err
	}
	if err = l.saveOutputBinding(request.Snapshot, process, *turn); err != nil {
		return receipt, nil, err
	}
	return receipt, newNativeObserver(ctx, l, l.boundSnapshot(request.Snapshot, target, process, state)), nil
}

func (l *ExecutionLifecycle) deliverFeedback(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	input := request.Input.Feedback
	if input == nil {
		return nativeReceipt(request, executionbackend.OperationRejected), errors.New("native Orca feedback is missing")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, err
	}
	if request.Snapshot.QuestionRoute == nil {
		turn := latestBusinessTurn(&state, request.Snapshot)
		if turn == nil || turn.DispatchID == "" || input.Message == "" {
			return nativeReceipt(request, executionbackend.OperationRejected), errors.New("native Orca status feedback has no active original Dispatch")
		}
		subject := "SteerLine feedback " + input.DecisionID
		sent, err := SendFeedback(ctx, l.client.MetaCall, subject, input.Message, turn.DispatchID,
			request.Snapshot.Ref.Target.OrchestrationRun, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
		if err != nil {
			return receipt, nativeCallError(err)
		}
		receipt.Disposition, receipt.RequestID = executionbackend.OperationConfirmed, sent.RequestID
		receipt.State.InputAccepted = true
		receipt.Operation.Receipt = append([]byte(nil), sent.Payload...)
		return receipt, nil
	}
	if input.Answer == nil {
		return nativeReceipt(request, executionbackend.OperationRejected), errors.New("native Orca question answer has no structured values")
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
		return nativeReceipt(request, executionbackend.OperationRejected), errors.New("native Orca structured answer is empty")
	}
	answer, err := ReplyQuestion(ctx, l.client.MetaCall, route.MessageID, body, request.Snapshot.Ref.Target.OrchestrationRun,
		state.Coordinator.Terminal, turn.DispatchID, state.Coordinator.RuntimeID)
	if err != nil {
		return receipt, nativeCallError(err)
	}
	receipt.Disposition, receipt.RequestID = executionbackend.OperationConfirmed, answer.RequestID
	receipt.State.InputAccepted = true
	receipt.Operation.Receipt = append([]byte(nil), answer.Payload...)
	return receipt, nil
}

func (l *ExecutionLifecycle) reconcileFeedback(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
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
		receipt.Disposition, receipt.RequestID, receipt.State.InputAccepted = executionbackend.OperationConfirmed, id, true
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
	receipt.Disposition, receipt.RequestID = executionbackend.OperationConfirmed, id
	receipt.State.InputAccepted = true
	return receipt, nil, nil
}

func (l *ExecutionLifecycle) questionRoute(snapshot executionbackend.LifecycleSnapshot, state NativeExecutionState) (*NativeTurnState, QuestionRouteData, error) {
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

func (l *ExecutionLifecycle) acknowledge(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
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
	receipt.Disposition, receipt.RequestID = executionbackend.OperationConfirmed, ack.RequestID
	receipt.Operation.Receipt = append([]byte(nil), ack.Payload...)
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, request.Snapshot.Ref.External, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileAck(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
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
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.RequestID = id
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, request.Snapshot.Ref.External, state)
	return receipt, nil, err
}

func workerTerminalIdentity(identity executionbackend.ProcessIdentity) TerminalIdentity {
	return TerminalIdentity{Kind: "orca-native", Handle: identity.Terminal, RuntimeID: identity.RuntimeID, Incarnation: identity.Incarnation}
}

func (l *ExecutionLifecycle) verifyAgent(ctx context.Context, snapshot executionbackend.LifecycleSnapshot, identity executionbackend.ProcessIdentity) error {
	if identity.Kind != "orca" || identity.Terminal == "" || identity.RuntimeID == "" || identity.Incarnation == "" || identity.WorktreeID != snapshot.Ref.Target.WorktreeID {
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "native Orca agent binding is incomplete"}
	}
	return VerifyAgentTerminal(ctx, l.client.MetaCall, identity, snapshot.Workspace.Path, snapshot.Workspace.Branch, snapshot.Ref.Agent.Name)
}

func (l *ExecutionLifecycle) boundSnapshot(snapshot executionbackend.LifecycleSnapshot, target executionbackend.ExecutionTarget,
	process executionbackend.ProcessIdentity, state NativeExecutionState) executionbackend.LifecycleSnapshot {
	bound := snapshot.Clone()
	bound.Ref.Target, bound.Session.Target, bound.Ref.External = target, target, process
	if envelope, err := l.stateEnvelope(bound, target, process, state); err == nil {
		bound.Implementation = []executionbackend.ImplementationDataEnvelope{envelope}
	}
	return bound
}

func (l *ExecutionLifecycle) cleanup(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	if len(request.Input.Cleanup.Resources) != 1 {
		return nativeReceipt(request, executionbackend.OperationRejected), errors.New("native Orca cleanup must name exactly one original resource")
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
		return l.releaseOriginalWorker(ctx, request, state)
	} else {
		return nativeReceipt(request, executionbackend.OperationRejected), &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "native Orca cleanup differs from the fixed resource list"}
	}
	if identity.Terminal == "" || identity.RuntimeID == "" || identity.Incarnation == "" {
		return nativeReceipt(request, executionbackend.OperationRejected), errors.New("native Orca cleanup lacks original terminal identity")
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
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.RequestID = identity.Terminal
	receipt.State.ResourceID = identity.Terminal
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, request.Snapshot.Ref.External, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileCleanup(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	if len(request.Input.Cleanup.Resources) != 1 {
		return nativeReceipt(request, executionbackend.OperationRejected), nil, errors.New("native Orca cleanup reconciliation must name one resource")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	identity := request.Input.Cleanup.Resources[0]
	role := "coordinator"
	if identity == request.Snapshot.Ref.External {
		released, proof, err := originalResourceReleased(ctx, l.client.MetaCall, identity)
		if err != nil {
			return receipt, nil, err
		}
		if !released {
			receipt.Diagnostic = "original worker release is not yet proven; preserve the original mutation and native recovery instructions"
			return receipt, nil, nil
		}
		state.AgentClosed = true
		receipt.Disposition, receipt.RequestID = executionbackend.OperationConfirmed, nativeRequestID(request.Operation.ID)
		receipt.Operation.Receipt = append([]byte(nil), proof...)
		receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, identity, state)
		return receipt, nil, err
	}
	if identity != state.Coordinator {
		return nativeReceipt(request, executionbackend.OperationRejected), nil, errors.New("cleanup reconciliation changed its original coordinator")
	}
	if err := InboxDrained(ctx, l.client.MetaCall, state.Coordinator.Terminal, state.Coordinator.RuntimeID, request.Snapshot.Ref.Target.OrchestrationRun); err != nil {
		return receipt, nil, err
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
	receipt.Disposition, receipt.RequestID = executionbackend.OperationConfirmed, identity.Terminal
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, request.Snapshot.Ref.External, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) StopLifecycle(ctx context.Context, request executionbackend.LifecycleRequest, _ executionbackend.StopHandle) (executionbackend.LifecycleReceipt, error) {
	if err := request.Validate(); err != nil {
		return nativeReceipt(request, executionbackend.OperationRejected), err
	}
	if request.Operation.Action == string(executionbackend.LifecycleStopObservation) {
		return nativeReceipt(request, executionbackend.OperationConfirmed), nil
	}
	if request.Operation.Action != string(executionbackend.LifecycleStopExecution) {
		return nativeReceipt(request, executionbackend.OperationRejected), executionbackend.UnsupportedAdapterOperation(request.Operation.Action, "native Orca stop has no original terminal")
	}
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	if request.Operation.Ref == nil {
		return receipt, errors.New("native stop lacks its saved resource identity")
	}
	identity := request.Operation.Ref.External
	if identity.Terminal == "" {
		state, _, err := l.readState(request.Snapshot)
		if err != nil || state.PartialWorker == nil {
			return receipt, errors.Join(err, errors.New("native Orca stop has no original worker"))
		}
		if state.PartialWorker.DispatchID != request.Operation.Ref.Target.Dispatch || state.PartialWorker.TaskID != request.Operation.Ref.Target.ExternalTask || state.PartialWorker.RunID != request.Operation.Ref.Target.OrchestrationRun {
			return receipt, errors.New("partial stop changed its original resource")
		}
		identity = state.PartialWorker.identity(request.Operation.Ref.Target.WorktreeID)
	}
	shown, raw, observeErr := observeOriginalWorkerResource(ctx, l.client.MetaCall, identity)
	receipt.Operation.Receipt = append([]byte(nil), raw...)
	if observeErr != nil {
		return receipt, observeErr
	}
	if shown.Resource.ReleaseState == "released" {
		receipt.Disposition = executionbackend.OperationConfirmed
		receipt.State.Stopped, receipt.State.WorkerAbsent, receipt.State.WritersAbsent = true, true, true
		return receipt, nil
	}
	if identity.Terminal != "" && shown.Worker.State != "succeeded" && shown.Worker.State != "failed" && shown.Worker.State != "stopped" {
		if err := l.verifyAgent(ctx, request.Snapshot, identity); err != nil {
			return receipt, err
		}
	}
	result, proof, err := workerResourceAction(ctx, l.client.MetaCall, identity, "worker-stop", request.Operation.ID)
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	receipt.RequestID = nativeRequestID(request.Operation.ID)
	if err != nil {
		return receipt, err
	}
	if proof.State == "stopped" {
		receipt.Disposition = executionbackend.OperationConfirmed
		receipt.State.Stopped = true
	}
	if proof.AlreadySettled {
		state, _, stateErr := l.readState(request.Snapshot)
		if stateErr != nil {
			return receipt, stateErr
		}
		for _, turn := range state.Turns {
			if turn.DispatchID == identity.Dispatch {
				if freezeErr := l.freezeOriginalOutput(ctx, request.Snapshot, turn); freezeErr != nil {
					receipt.Diagnostic = "canonical output unavailable: " + freezeErr.Error()
				}
				break
			}
		}
		released, releaseProof, releaseErr := workerResourceAction(ctx, l.client.MetaCall, identity, "worker-release", request.Operation.ID+":settled-release")
		receipt.Operation.Receipt, _ = json.Marshal(map[string]json.RawMessage{"stop": result.Payload, "release": released.Payload})
		if releaseErr != nil {
			return receipt, releaseErr
		}
		if workerReleased(releaseProof) {
			receipt.Disposition = executionbackend.OperationConfirmed
			receipt.State.Stopped, receipt.State.WorkerAbsent, receipt.State.WritersAbsent = true, true, true
		} else {
			receipt.Diagnostic += "; original settled worker release remains " + releaseProof.State + "; " + releaseProof.LastError + "; recovery=" + string(releaseProof.Recovery)
		}
	}

	return receipt, nil
}

func (l *ExecutionLifecycle) CleanupLifecycle(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	return l.cleanup(ctx, request)
}

func (l *ExecutionLifecycle) VerifyManualRecovery(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleRecoveryEvidence, error) {
	if err := l.validateSnapshot(snapshot); err != nil {
		return executionbackend.LifecycleRecoveryEvidence{}, err
	}
	identity := snapshot.Ref.External
	if identity.Terminal == "" {
		state, _, err := l.readState(snapshot)
		if err != nil {
			return executionbackend.LifecycleRecoveryEvidence{}, err
		}
		if state.PartialWorker != nil {
			released, _, err := originalResourceReleased(ctx, l.client.MetaCall, state.PartialWorker.identity(snapshot.Ref.Target.WorktreeID))
			return executionbackend.LifecycleRecoveryEvidence{ManagedProcess: true, ProcessAbsent: released}, err
		}
		for _, op := range snapshot.Operations {
			if op.Action == string(executionbackend.LifecycleStartTurn) && op.Status != executionbackend.OperationConfirmed && op.Status != executionbackend.OperationRejected {
				return executionbackend.LifecycleRecoveryEvidence{ManagedProcess: true}, errors.New("original native worker-start is unresolved; absence cannot be proven without its saved request receipt")
			}
		}
		return executionbackend.LifecycleRecoveryEvidence{ProcessAbsent: true}, nil
	}
	terminal, err := l.client.ShowTerminal(ctx, identity.Terminal)
	if err != nil {
		return executionbackend.LifecycleRecoveryEvidence{}, err
	}
	if terminal.RuntimeID != identity.RuntimeID || terminal.IncarnationID != identity.Incarnation || terminal.WorktreeID != identity.WorktreeID {
		return executionbackend.LifecycleRecoveryEvidence{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "native Orca recovery terminal identity changed"}
	}
	if err := VerifyAgentTerminal(ctx, l.client.MetaCall, identity, snapshot.Workspace.Path, snapshot.Workspace.Branch, snapshot.Ref.Agent.Name); err != nil {
		return executionbackend.LifecycleRecoveryEvidence{}, err
	}
	return executionbackend.LifecycleRecoveryEvidence{ManagedProcess: true, ProcessAbsent: terminal.Exited(), SessionAvailable: !terminal.Exited()}, nil
}

func (l *ExecutionLifecycle) BindRecovery(ctx context.Context, previous, next executionbackend.LifecycleSnapshot) (executionbackend.ImplementationDataEnvelope, error) {
	if err := l.validateSnapshot(previous); err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	if err := sameNativeTarget(previous.Ref, next.Ref); err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	identity := previous.Ref.External
	if identity.Terminal == "" {
		return executionbackend.ImplementationDataEnvelope{}, errors.New("native Orca recovery has no original agent terminal")
	}
	if err := VerifyAgentTerminal(ctx, l.client.MetaCall, identity, previous.Workspace.Path, previous.Workspace.Branch, previous.Ref.Agent.Name); err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	state, found, err := l.readState(previous)
	if err != nil || !found {
		return executionbackend.ImplementationDataEnvelope{}, errors.Join(err, errors.New("native Orca recovery lost its implementation binding"))
	}
	return NewExecutionStateEnvelope(ExecutionStateIdentity{ProjectID: next.Session.ProjectID, ExecutionID: next.Session.ExecutionID,
		AttemptID: next.Session.AttemptID, Generation: next.ResourceGeneration, ConfigDigest: next.ConfigurationDigest,
		Protocol: next.Ref.Protocol, Process: identity, CoordinatorTerminal: state.Coordinator.Terminal}, state)
}

var _ executionbackend.LifecycleRecoveryPort = (*ExecutionLifecycle)(nil)

var _ = json.Valid
