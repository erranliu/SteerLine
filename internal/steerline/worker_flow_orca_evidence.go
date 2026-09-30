package steerline

import (
	"context"
	"encoding/json"
	"errors"
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
	env, err := workerOrcaCall(ctx, c, "orchestration", "check", "--terminal", execution.OrcaNative.Coordinator.Terminal, "--run", execution.Target.OrchestrationRun, "--json")
	if err != nil {
		return fact, err
	}
	if env.Meta.RuntimeID != execution.OrcaNative.Coordinator.RuntimeID {
		return fact, errors.New("Orca coordinator runtime changed")
	}
	var delivery struct {
		RunID          string             `json:"runId"`
		DeliveryID     string             `json:"deliveryId"`
		Messages       []OrcaInboxMessage `json:"messages"`
		ConnectionLost bool               `json:"connectionLost"`
	}
	if err := json.Unmarshal(env.Result, &delivery); err != nil || delivery.RunID != execution.Target.OrchestrationRun || delivery.ConnectionLost || len(delivery.Messages) > 0 && delivery.DeliveryID == "" {
		return fact, errors.Join(err, errors.New("Orca inbox delivery identity is unverified"))
	}
	fact.DeliveryID, fact.Messages = delivery.DeliveryID, delivery.Messages
	return fact, nil
}

func decodeNativeOrcaMessage(message OrcaInboxMessage, runID string, turn OrcaNativeTurn, agentTerminal string) (nativeOrcaMessageFact, bool, error) {
	var fact nativeOrcaMessageFact
	if !nativeOrcaMessageMatches(message, turn, agentTerminal) {
		return fact, false, nil
	}
	fact.Kind, fact.MessageID, fact.TaskID, fact.DispatchID = message.Type, message.ID, turn.TaskID, turn.DispatchID
	switch message.Type {
	case "question":
		question := nativeOrcaQuestion(message, runID, turn)
		fact.Question = &question
	case "worker_done":
		outcome, err := nativeOrcaDoneOutcome(message, turn)
		if err != nil {
			return fact, true, err
		}
		fact.Outcome = outcome
		fact.PRCandidates = workerPRClues(message.Body)
	case "status":
		fact.PRCandidates = workerPRClues(message.Body)
	case "escalation":
		question := DecisionRequest{Question: clip("Orca worker escalation: "+message.Subject, 1800), Context: clip(message.Body, 7000), Options: []DecisionOption{{ID: "continue", Label: "Continue current work"}}, AllowFreeText: true}
		fact.Question = &question
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
	env, err := workerOrcaCall(ctx, c, "orchestration", "worker-show", "--dispatch", turn.DispatchID, "--json")
	if err != nil {
		return fact, err
	}
	var result struct {
		Dispatch struct {
			ID     string `json:"id"`
			RunID  string `json:"runId"`
			TaskID string `json:"taskId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID          string `json:"dispatchId"`
			AgentTerminalHandle string `json:"agentTerminalHandle"`
		} `json:"worker"`
		Terminal struct {
			Handle        string `json:"handle"`
			IncarnationID string `json:"incarnationId"`
			WorktreeID    string `json:"worktreeId"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return fact, err
	}
	if env.Meta.RuntimeID != execution.OrcaNative.Coordinator.RuntimeID || result.Dispatch.ID != turn.DispatchID || result.Dispatch.TaskID != turn.TaskID || result.Dispatch.RunID != execution.Target.OrchestrationRun || result.Worker.DispatchID != turn.DispatchID || result.Worker.AgentTerminalHandle != execution.Process.Terminal || result.Terminal.Handle != execution.Process.Terminal || result.Terminal.IncarnationID != execution.Process.Incarnation || result.Terminal.WorktreeID != execution.Process.WorktreeID {
		return fact, errors.New("Orca settled Dispatch identity differs from the fixed worker")
	}
	if turn.Outcome == "succeeded" && result.Dispatch.Status != "completed" || turn.Outcome == "failed" && result.Dispatch.Status != "failed" {
		return fact, errors.New("Orca worker_done and Dispatch settlement disagree")
	}
	if turn.Outcome != "succeeded" && turn.Outcome != "failed" {
		return fact, errors.New("Orca worker_done has no verified outcome")
	}
	fact.Outcome, fact.DispatchID, fact.RuntimeID = turn.Outcome, turn.DispatchID, env.Meta.RuntimeID
	return fact, nil
}

func verifyNativeOrcaActiveDispatch(ctx context.Context, c Config, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	if execution == nil || execution.Process == nil || turn == nil {
		return errors.New("stopped-center Orca Dispatch has no fixed identity")
	}
	env, err := workerOrcaCall(ctx, c, "orchestration", "worker-show", "--dispatch", turn.DispatchID, "--json")
	if err != nil {
		return err
	}
	var current struct {
		Dispatch struct {
			ID     string `json:"id"`
			RunID  string `json:"runId"`
			TaskID string `json:"taskId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			AgentTerminalHandle string `json:"agentTerminalHandle"`
		} `json:"worker"`
	}
	if json.Unmarshal(env.Result, &current) != nil || env.Meta.RuntimeID != execution.Process.RuntimeID || current.Dispatch.ID != turn.DispatchID || current.Dispatch.RunID != execution.Target.OrchestrationRun || current.Dispatch.TaskID != turn.TaskID || current.Dispatch.Status != "dispatched" || current.Worker.AgentTerminalHandle != execution.Process.Terminal {
		return errors.New("stopped-center Orca Dispatch could not be verified as still active")
	}
	return nil
}
