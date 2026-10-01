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
	Delivery             func(context.Context, *ExecutionRecord) (nativeOrcaDeliveryFact, error)
	Settlement           func(context.Context, *ExecutionRecord, *OrcaNativeTurn) (nativeOrcaSettlement, error)
	VerifyActive         func(context.Context, *ExecutionRecord, *OrcaNativeTurn) error
	DeliveryAcknowledged func(context.Context, *Run, *ExecutionRecord, string) (bool, error)
	Ack                  func(context.Context, *Run, *ExecutionRecord, string) (orcaworker.MutationReceipt, error)
	FindAnswer           func(context.Context, *Run, *ExecutionRecord, *OrcaNativeTurn, string, string) (string, error)
	Reply                func(context.Context, *Run, *ExecutionRecord, *OrcaNativeTurn, string, string) (orcaworker.MutationReceipt, error)
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

func nativeOrcaOperations(c Config) orcaExecutionOperations {
	return orcaExecutionOperations{
		Delivery: func(ctx context.Context, e *ExecutionRecord) (nativeOrcaDeliveryFact, error) {
			d, err := orcaworker.ReadDelivery(ctx, orcaWorkerMetaCaller(c), e.OrcaNative.Coordinator.Terminal, e.OrcaNative.Coordinator.RuntimeID, e.Target.OrchestrationRun)
			if err != nil {
				return nativeOrcaDeliveryFact{}, err
			}
			return nativeOrcaDeliveryFact{DeliveryID: d.DeliveryID, Messages: d.Messages}, nil
		},
		Settlement: func(ctx context.Context, e *ExecutionRecord, t *OrcaNativeTurn) (nativeOrcaSettlement, error) {
			value, err := orcaworker.Settlement(ctx, orcaWorkerMetaCaller(c), orcaworker.DispatchIdentity{DispatchID: t.DispatchID, TaskID: t.TaskID, RunID: e.Target.OrchestrationRun, RuntimeID: e.OrcaNative.Coordinator.RuntimeID, Terminal: e.Process.Terminal, Incarnation: e.Process.Incarnation, WorktreeID: e.Process.WorktreeID}, t.Outcome)
			if err != nil {
				return nativeOrcaSettlement{}, err
			}
			return nativeOrcaSettlement{Outcome: value, DispatchID: t.DispatchID, RuntimeID: e.OrcaNative.Coordinator.RuntimeID}, nil
		},
		VerifyActive: func(ctx context.Context, e *ExecutionRecord, t *OrcaNativeTurn) error {
			return orcaworker.VerifyLiveDispatch(ctx, orcaWorkerMetaCaller(c), orcaworker.DispatchIdentity{DispatchID: t.DispatchID, TaskID: t.TaskID, RunID: e.Target.OrchestrationRun, RuntimeID: e.Process.RuntimeID, Terminal: e.Process.Terminal})
		},
		DeliveryAcknowledged: func(ctx context.Context, _ *Run, e *ExecutionRecord, id string) (bool, error) {
			return orcaworker.DeliveryAcknowledged(ctx, orcaWorkerMetaCaller(c), e.OrcaNative.Coordinator.Terminal, e.OrcaNative.Coordinator.RuntimeID, e.Target.OrchestrationRun, id)
		},
		Ack: func(ctx context.Context, _ *Run, e *ExecutionRecord, id string) (orcaworker.MutationReceipt, error) {
			receipt, err := orcaworker.AckInbox(ctx, orcaWorkerMetaCaller(c), e.OrcaNative.Coordinator.Terminal, e.Target.OrchestrationRun, id, e.OrcaNative.Coordinator.RuntimeID)
			var call *orcaworker.CallError
			if errors.As(err, &call) {
				err = &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: err.Error(), Cause: err}
			}
			return receipt, err
		},
		FindAnswer: func(ctx context.Context, _ *Run, e *ExecutionRecord, t *OrcaNativeTurn, id, body string) (string, error) {
			return orcaworker.FindReply(ctx, orcaWorkerCaller(c), e.Target.OrchestrationRun, t.DispatchID, id, body)
		},
		Reply: func(ctx context.Context, _ *Run, e *ExecutionRecord, t *OrcaNativeTurn, id, body string) (orcaworker.MutationReceipt, error) {
			return orcaworker.ReplyQuestion(ctx, orcaWorkerMetaCaller(c), id, body, e.Target.OrchestrationRun, e.OrcaNative.Coordinator.Terminal, t.DispatchID, e.OrcaNative.Coordinator.RuntimeID)
		},
	}
}

func remoteOrcaOperations(c Config) orcaExecutionOperations {
	return orcaExecutionOperations{
		Delivery: func(ctx context.Context, e *ExecutionRecord) (nativeOrcaDeliveryFact, error) {
			ref := executionapi.ExecutionRef{RunID: e.WorkflowRunID, Target: e.Target, External: *e.Process}
			d, err := remoteClientForExecution(c, e).Check(ctx, remoteWorkerRef(ref))
			return nativeOrcaDeliveryFact{DeliveryID: d.ID, Messages: d.Messages}, remoteWorkerError(err)
		},
		Settlement: func(ctx context.Context, e *ExecutionRecord, t *OrcaNativeTurn) (nativeOrcaSettlement, error) {
			ref := executionapi.ExecutionRef{Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaSupervised, Target: e.Target, External: *e.Process}
			value, err := remoteClientForExecution(c, e).Settlement(ctx, remoteWorkerRef(ref), t.Outcome)
			return nativeOrcaSettlement{Outcome: value, DispatchID: t.DispatchID, RuntimeID: e.Process.CoordinatorRuntimeID}, remoteWorkerError(err)
		},
		VerifyActive: func(ctx context.Context, e *ExecutionRecord, _ *OrcaNativeTurn) error {
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
		FindAnswer: func(ctx context.Context, r *Run, e *ExecutionRecord, _ *OrcaNativeTurn, id, body string) (string, error) {
			return remoteClientForExecution(c, e).FindAnswer(ctx, remoteWorkerRef(remoteExecutionRef(r, e)), id, body)
		},
		Reply: func(ctx context.Context, r *Run, e *ExecutionRecord, _ *OrcaNativeTurn, id, body string) (orcaworker.MutationReceipt, error) {
			return remoteOrcaQuestionReply(ctx, c, r, e, id, body)
		},
	}
}
