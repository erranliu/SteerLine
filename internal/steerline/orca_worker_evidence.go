package steerline

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

type nativeOrcaMessageFact struct {
	Kind         string
	MessageID    string
	TaskID       string
	DispatchID   string
	Question     *DecisionRequest
	Outcome      string
	PRCandidates []string
}

type nativeOrcaDeliveryFact struct {
	DeliveryID string
	Messages   []OrcaInboxMessage
}

func readNativeOrcaDelivery(ctx context.Context, c Config, execution *ExecutionRecord) (nativeOrcaDeliveryFact, error) {
	var fact nativeOrcaDeliveryFact
	if execution == nil || execution.OrcaNative == nil || execution.OrcaNative.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		return fact, errors.New("native Orca inbox has no bound coordinator/Run")
	}
	delivery, err := orcaworker.CheckInbox(ctx, orcaWorkerMetaCaller(c), execution.OrcaNative.Coordinator.Terminal, execution.Target.OrchestrationRun)
	if err != nil {
		return fact, err
	}
	if delivery.RuntimeID != execution.OrcaNative.Coordinator.RuntimeID {
		return fact, errors.New("Orca coordinator runtime changed")
	}
	if delivery.RunID != execution.Target.OrchestrationRun || delivery.ConnectionLost || len(delivery.Messages) > 0 && delivery.DeliveryID == "" {
		return fact, errors.New("Orca inbox delivery identity is unverified")
	}
	fact.DeliveryID, fact.Messages = delivery.DeliveryID, delivery.Messages
	return fact, nil
}

func decodeNativeOrcaMessage(message OrcaInboxMessage, runID string, turn OrcaNativeTurn, agentTerminal string) (nativeOrcaMessageFact, bool, error) {
	wire, matched, err := orcaworker.DecodeMessage(message, turn.TaskID, turn.DispatchID, agentTerminal)
	if err != nil || !matched {
		return nativeOrcaMessageFact{}, matched, err
	}
	fact := nativeOrcaMessageFact{Kind: wire.Kind, MessageID: wire.MessageID, TaskID: wire.TaskID, DispatchID: wire.DispatchID, Outcome: wire.Outcome}
	switch wire.Kind {
	case "question":
		request := DecisionRequest{Question: clip(wire.Question.Question, 1800), Context: clip(message.Body, 7000), AllowFreeText: true}
		for _, option := range wire.Question.Options {
			request.Options = append(request.Options, executionapi.DecisionOption{ID: option.ID, Label: option.Label, Detail: option.Detail})
		}
		request.OrcaAsk = &orcaAskCall{RunID: runID, TaskID: wire.TaskID, DispatchID: wire.DispatchID, MessageID: wire.MessageID, Questions: []executionapi.Question{{ID: wire.MessageID, Header: "Orca", Question: request.Question, Options: request.Options, AllowFreeText: true}}}
		fact.Question = &request
	case "worker_done", "status":
		fact.PRCandidates = wire.PRCandidates
	case "escalation":
		request := DecisionRequest{Question: clip("Orca worker escalation: "+message.Subject, 1800), Context: clip(message.Body, 7000), Options: []executionapi.DecisionOption{{ID: "continue", Label: "Continue current work"}}, AllowFreeText: true}
		fact.Question = &request
	}
	return fact, true, nil
}

type nativeOrcaSettlement struct {
	Outcome    string
	DispatchID string
	RuntimeID  string
}

func readNativeOrcaSettlement(ctx context.Context, c Config, execution *ExecutionRecord, turn *OrcaNativeTurn) (nativeOrcaSettlement, error) {
	var fact nativeOrcaSettlement
	if execution == nil || execution.OrcaNative == nil || execution.Process == nil || turn == nil {
		return fact, errors.New("Orca settlement has no fixed execution identity")
	}
	status, err := orcaworker.VerifySettledDispatch(ctx, orcaWorkerMetaCaller(c), orcaworker.DispatchIdentity{
		DispatchID: turn.DispatchID, TaskID: turn.TaskID, RunID: execution.Target.OrchestrationRun,
		RuntimeID: execution.OrcaNative.Coordinator.RuntimeID, Terminal: execution.Process.Terminal,
		Incarnation: execution.Process.Incarnation, WorktreeID: execution.Process.WorktreeID,
	})
	if err != nil {
		return fact, err
	}
	if turn.Outcome == "succeeded" && status != "completed" || turn.Outcome == "failed" && status != "failed" {
		return fact, errors.New("Orca worker_done and Dispatch settlement disagree")
	}
	if turn.Outcome != "succeeded" && turn.Outcome != "failed" {
		return fact, errors.New("Orca worker_done has no verified outcome")
	}
	fact.Outcome, fact.DispatchID, fact.RuntimeID = turn.Outcome, turn.DispatchID, execution.OrcaNative.Coordinator.RuntimeID
	return fact, nil
}

func verifyNativeOrcaActiveDispatch(ctx context.Context, c Config, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	if execution == nil || execution.Process == nil || turn == nil {
		return errors.New("stopped-center Orca Dispatch has no fixed identity")
	}
	status, err := orcaworker.VerifyActiveDispatch(ctx, orcaWorkerMetaCaller(c), orcaworker.DispatchIdentity{
		DispatchID: turn.DispatchID, TaskID: turn.TaskID, RunID: execution.Target.OrchestrationRun,
		RuntimeID: execution.Process.RuntimeID, Terminal: execution.Process.Terminal,
	})
	if errors.Is(err, orcaworker.ErrActiveDispatchIdentity) {
		return errors.New("stopped-center Orca Dispatch could not be verified as still active")
	}
	if err != nil {
		return err
	}
	if status != "dispatched" {
		return errors.New("stopped-center Orca Dispatch could not be verified as still active")
	}
	return nil
}
