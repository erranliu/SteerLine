package steerline

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

// Consumer-sized platform operations. The service retains intents, commits,
// decisions and delivery; platform calls do not write center business state.
type orcaExecutionOperations struct {
	Delivery             func(context.Context, *Run, *ExecutionRecord, *implementationTurnView) (executionapi.EventBatch, error)
	Settlement           func(context.Context, *ExecutionRecord, *implementationTurnView) (nativeOrcaSettlement, error)
	VerifyActive         func(context.Context, *ExecutionRecord, *implementationTurnView) error
	DeliveryAcknowledged func(context.Context, *Run, *ExecutionRecord, string) (bool, error)
	Ack                  func(context.Context, *Run, *ExecutionRecord, string) (orcaworker.MutationReceipt, error)
	FindAnswer           func(context.Context, *Run, *ExecutionRecord, *implementationTurnView, string, string) (string, error)
	Reply                func(context.Context, *Run, *ExecutionRecord, *implementationTurnView, string, string) (orcaworker.MutationReceipt, error)
}

func orcaOperationsFor(c Config) (orcaExecutionOperations, error) {
	entry, err := registrationFor(c)
	if err != nil {
		return orcaExecutionOperations{}, err
	}
	if entry.OrcaOperations == nil {
		return orcaExecutionOperations{}, unsupported("Orca operations")
	}
	ops := entry.OrcaOperations(c)
	if ops.Delivery == nil || ops.Settlement == nil || ops.VerifyActive == nil || ops.DeliveryAcknowledged == nil || ops.Ack == nil || ops.FindAnswer == nil || ops.Reply == nil {
		return orcaExecutionOperations{}, unsupported("incomplete registered Orca operations")
	}
	return ops, nil
}

func readNativeOrcaDelivery(ctx context.Context, c Config, run *Run, execution *ExecutionRecord, turn *implementationTurnView) (executionapi.EventBatch, error) {
	state, stateErr := decodeOrcaExecutionState(execution)
	if stateErr != nil {
		return executionapi.EventBatch{}, stateErr
	}
	if run == nil || execution == nil || state == nil || state.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		return executionapi.EventBatch{}, errors.New("inbox has no bound coordinator/Run")
	}
	if execution.Process == nil || turn == nil {
		return executionapi.EventBatch{}, errors.New("inbox has no fixed process or turn identity")
	}
	ops, err := orcaOperationsFor(c)
	if err != nil {
		return executionapi.EventBatch{}, err
	}
	return ops.Delivery(ctx, run, execution, turn)
}

func decodeNativeOrcaInbox(inbox orcaworker.Inbox, run *Run, execution *ExecutionRecord, turn *implementationTurnView) (executionapi.EventBatch, error) {
	if run == nil || execution == nil || execution.Process == nil || turn == nil {
		return executionapi.EventBatch{}, errors.New("inbox decoding has no fixed execution, recipient or turn")
	}
	fixed := workerSessionContext(run, execution)
	fixed.TaskID = turn.TaskID
	fixed.CurrentTurnID = turn.TurnID
	fixed.Generation = nativeOrcaGeneration(execution)
	return orcaworker.DecodeInbox(inbox, turn.TaskID, turn.DispatchID, execution.Process.Terminal, *fixed)
}

func nativeOrcaGeneration(execution *ExecutionRecord) string {
	if execution == nil {
		return ""
	}
	if state, err := decodeOrcaExecutionState(execution); err == nil && state != nil && state.Coordinator.RuntimeID != "" {
		return state.Coordinator.RuntimeID
	}
	if execution.Process != nil && execution.Process.Incarnation != "" {
		return execution.Process.Incarnation
	}
	return ""
}

func nativeOrcaOperations(c Config) orcaExecutionOperations {
	return orcaExecutionOperations{
		Delivery: func(ctx context.Context, run *Run, e *ExecutionRecord, turn *implementationTurnView) (executionapi.EventBatch, error) {
			state, err := decodeOrcaExecutionState(e)
			if err != nil || state == nil {
				return executionapi.EventBatch{}, errors.New("Orca delivery has no verified coordinator binding")
			}
			d, err := orcaworker.ReadDelivery(ctx, orcaWorkerMetaCaller(c), state.Coordinator.Terminal, state.Coordinator.RuntimeID, e.Target.OrchestrationRun)
			if err != nil {
				return executionapi.EventBatch{}, err
			}
			return decodeNativeOrcaInbox(d, run, e, turn)
		},
		Settlement: func(ctx context.Context, e *ExecutionRecord, t *implementationTurnView) (nativeOrcaSettlement, error) {
			state, err := decodeOrcaExecutionState(e)
			if err != nil || state == nil {
				return nativeOrcaSettlement{}, errors.New("Orca settlement has no verified coordinator binding")
			}
			value, err := orcaworker.Settlement(ctx, orcaWorkerMetaCaller(c), orcaworker.DispatchIdentity{DispatchID: t.DispatchID, TaskID: t.TaskID, RunID: e.Target.OrchestrationRun, RuntimeID: state.Coordinator.RuntimeID, Terminal: e.Process.Terminal, Incarnation: e.Process.Incarnation, WorktreeID: e.Process.WorktreeID}, t.Outcome)
			if err != nil {
				return nativeOrcaSettlement{}, err
			}
			return nativeOrcaSettlement{Outcome: value, DispatchID: t.DispatchID, RuntimeID: state.Coordinator.RuntimeID}, nil
		},
		VerifyActive: func(ctx context.Context, e *ExecutionRecord, t *implementationTurnView) error {
			return orcaworker.VerifyLiveDispatch(ctx, orcaWorkerMetaCaller(c), orcaworker.DispatchIdentity{DispatchID: t.DispatchID, TaskID: t.TaskID, RunID: e.Target.OrchestrationRun, RuntimeID: e.Process.RuntimeID, Terminal: e.Process.Terminal})
		},
		DeliveryAcknowledged: func(ctx context.Context, _ *Run, e *ExecutionRecord, id string) (bool, error) {
			state, err := decodeOrcaExecutionState(e)
			if err != nil || state == nil {
				return false, errors.New("Orca delivery acknowledgement has no verified coordinator binding")
			}
			return orcaworker.DeliveryAcknowledged(ctx, orcaWorkerMetaCaller(c), state.Coordinator.Terminal, state.Coordinator.RuntimeID, e.Target.OrchestrationRun, id)
		},
		Ack: func(ctx context.Context, _ *Run, e *ExecutionRecord, id string) (orcaworker.MutationReceipt, error) {
			state, err := decodeOrcaExecutionState(e)
			if err != nil || state == nil {
				return orcaworker.MutationReceipt{}, errors.New("Orca inbox acknowledgement has no verified coordinator binding")
			}
			receipt, err := orcaworker.AckInbox(ctx, orcaWorkerMetaCaller(c), state.Coordinator.Terminal, e.Target.OrchestrationRun, id, state.Coordinator.RuntimeID)
			var call *orcaworker.CallError
			if errors.As(err, &call) {
				err = &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: err.Error(), Cause: err}
			}
			return receipt, err
		},
		FindAnswer: func(ctx context.Context, _ *Run, e *ExecutionRecord, t *implementationTurnView, id, body string) (string, error) {
			return orcaworker.FindReply(ctx, orcaWorkerCaller(c), e.Target.OrchestrationRun, t.DispatchID, id, body)
		},
		Reply: func(ctx context.Context, _ *Run, e *ExecutionRecord, t *implementationTurnView, id, body string) (orcaworker.MutationReceipt, error) {
			state, err := decodeOrcaExecutionState(e)
			if err != nil || state == nil {
				return orcaworker.MutationReceipt{}, errors.New("Orca question reply has no verified coordinator binding")
			}
			return orcaworker.ReplyQuestion(ctx, orcaWorkerMetaCaller(c), id, body, e.Target.OrchestrationRun, state.Coordinator.Terminal, t.DispatchID, state.Coordinator.RuntimeID)
		},
	}
}

func remoteOrcaOperations(c Config) orcaExecutionOperations {
	return orcaExecutionOperations{
		Delivery: func(ctx context.Context, run *Run, e *ExecutionRecord, turn *implementationTurnView) (executionapi.EventBatch, error) {
			ref := executionapi.ExecutionRef{RunID: e.WorkflowRunID, Target: e.Target, External: *e.Process}
			d, err := remoteClientForExecution(c, e).Check(ctx, remoteWorkerRef(ref))
			if err != nil {
				return executionapi.EventBatch{}, remoteWorkerError(err)
			}
			return decodeNativeOrcaInbox(orcaworker.Inbox{RunID: e.Target.OrchestrationRun, DeliveryID: d.ID, Messages: d.Messages}, run, e, turn)
		},
		Settlement: func(ctx context.Context, e *ExecutionRecord, t *implementationTurnView) (nativeOrcaSettlement, error) {
			ref := executionapi.ExecutionRef{Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaSupervised, Target: e.Target, External: *e.Process}
			value, err := remoteClientForExecution(c, e).Settlement(ctx, remoteWorkerRef(ref), t.Outcome)
			return nativeOrcaSettlement{Outcome: value, DispatchID: t.DispatchID, RuntimeID: e.Process.CoordinatorRuntimeID}, remoteWorkerError(err)
		},
		VerifyActive: func(ctx context.Context, e *ExecutionRecord, _ *implementationTurnView) error {
			value, err := remote.NewBackend(remoteWorkerConfig(c)).Observe(ctx, executionapi.ExecutionRef{Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaSupervised, Target: e.Target, External: *e.Process})
			if err != nil {
				return err
			}
			if value.Liveness != "live" {
				return errors.New("remote Dispatch liveness is unverified")
			}
			return nil
		},
		DeliveryAcknowledged: func(ctx context.Context, r *Run, e *ExecutionRecord, id string) (bool, error) {
			return remoteClientForExecution(c, e).DeliveryAcknowledged(ctx, remoteWorkerRef(remoteExecutionRef(r, e)), id)
		},
		Ack: func(ctx context.Context, r *Run, e *ExecutionRecord, id string) (orcaworker.MutationReceipt, error) {
			request, err := remoteClientForExecution(c, e).Ack(ctx, remoteWorkerRef(remoteExecutionRef(r, e)), id)
			if err != nil {
				err = &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: err.Error(), Cause: err}
			}
			return orcaworker.MutationReceipt{RequestID: request, Payload: mustJSON(map[string]string{"acknowledged": id})}, err
		},
		FindAnswer: func(ctx context.Context, r *Run, e *ExecutionRecord, _ *implementationTurnView, id, body string) (string, error) {
			return remoteClientForExecution(c, e).FindAnswer(ctx, remoteWorkerRef(remoteExecutionRef(r, e)), id, body)
		},
		Reply: func(ctx context.Context, r *Run, e *ExecutionRecord, _ *implementationTurnView, id, body string) (orcaworker.MutationReceipt, error) {
			return remoteOrcaQuestionReply(ctx, c, r, e, id, body)
		},
	}
}
