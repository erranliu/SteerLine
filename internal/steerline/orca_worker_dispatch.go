package steerline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func nativeOrcaAnswerText(answer DecisionAnswer, request DecisionRequest) string {
	if request.OrcaAsk == nil {
		return workerFeedbackText(answer, request)
	}
	return strings.Join(answer.Answers[request.OrcaAsk.MessageID], "\n\n")
}

func findNativeOrcaTask(ctx context.Context, c Config, runID, coordinator, spec string) (string, error) {
	return orcaworker.FindTask(ctx, orcaWorkerCaller(c), runID, coordinator, spec)
}

func findNativeOrcaReply(ctx context.Context, c Config, runID, dispatchID, questionID, answer string) (string, error) {
	return orcaworker.FindReply(ctx, orcaWorkerCaller(c), runID, dispatchID, questionID, answer)
}

func findNativeOrcaSend(ctx context.Context, c Config, runID, dispatchID, subject, body string) (string, error) {
	return orcaworker.FindSend(ctx, orcaWorkerCaller(c), runID, dispatchID, subject, body)
}

func (s *Service) startNativeOrcaTurn(ctx context.Context, run *Run, execution *ExecutionRecord, turnID, prompt string) error {
	purpose := "business"
	if execution.Environment != nil && execution.Environment.Preparation.TurnID == turnID {
		purpose = "readiness"
	}
	for _, other := range execution.OrcaNative.Turns {
		if other.TurnID != turnID && !other.Settled {
			return errors.New("original Orca Dispatch is active or unknown; reconcile before new input")
		}
	}
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	if fixedConfig.hostName() == "orca_supervised" {
		return s.startRemoteOrcaTurn(ctx, run, execution, turnID, prompt)
	}
	if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
		return err
	}
	if err := ensureNativeOrcaRelayIgnored(ctx, fixedConfig, execution.Workspace.Worktree); err != nil {
		return err
	}
	if len([]byte(prompt)) > 16<<10 || len([]rune(prompt)) > 12000 {
		return &executionapi.BackendError{Code: executionapi.BackendRejected, Diagnostic: "Orca native task spec exceeds the conservative Windows command-line bound"}
	}
	if execution.OrcaNative.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		return errors.New("Orca coordinator and Run are not prepared")
	}
	state := execution.OrcaNative
	var current *OrcaNativeTurn
	for i := range state.Turns {
		if state.Turns[i].TurnID == turnID {
			current = &state.Turns[i]
			break
		}
	}
	if current != nil && current.DispatchID != "" {
		return nil
	}
	intentAt := now()
	if purpose == "business" {
		if op := s.operationByKey("worker-turn:" + execution.ExecutionID + ":" + turnID); op != nil {
			intentAt = op.CreatedAt
		}
	}
	taskKey := "orca-native-task:" + execution.ExecutionID + ":" + turnID
	if current == nil {
		if op := s.operationByKey(taskKey); op != nil {
			id, err := findNativeOrcaTask(ctx, fixedConfig, execution.Target.OrchestrationRun, state.Coordinator.Terminal, prompt)
			if err != nil {
				return err
			}
			if id == "" {
				return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "original Orca Task creation has no unique matching Task"}
			}
			if purpose == "readiness" {
				intentAt = op.CreatedAt
			}
			state.Turns = append(state.Turns, OrcaNativeTurn{TurnID: turnID, TaskID: id, TaskRequestID: op.PlatformRequestID, Purpose: purpose, IntentAt: intentAt})
			execution.Target.ExternalTask = id
			s.refreshRun(run)
			if op.Status != "confirmed" {
				if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"task_id": id}), ""); err != nil {
					return err
				}
			} else if err := s.commitWorker(run); err != nil {
				return err
			}
		} else {
			taskOp, err := s.startWorkerOperation(run, "orca_native_task", taskKey, map[string]string{"turn_id": turnID, "spec": prompt, "run": execution.Target.OrchestrationRun})
			if err != nil {
				return err
			}
			receipt, err := orcaworker.CreateTask(ctx, orcaWorkerMetaCaller(fixedConfig), prompt, run.Title, execution.Target.OrchestrationRun, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
			if err != nil {
				var callErr *orcaworker.CallError
				if errors.As(err, &callErr) {
					_ = s.changeWorkerOperation(run, taskOp, "outcome_unknown", "", nil, err.Error())
				}
				return err
			}
			state.Turns = append(state.Turns, OrcaNativeTurn{TurnID: turnID, TaskID: receipt.ID, TaskRequestID: receipt.RequestID, Purpose: purpose, IntentAt: intentAt})
			execution.Target.ExternalTask = receipt.ID
			s.refreshRun(run)
			if err := s.changeWorkerOperation(run, taskOp, "confirmed", receipt.RequestID, receipt.Payload, ""); err != nil {
				return err
			}
		}
		current = &state.Turns[len(state.Turns)-1]
	}
	taskID := current.TaskID
	dispatchKey := "orca-native-dispatch:" + execution.ExecutionID + ":" + turnID
	if current.InjectionAttempts > 0 {
		dispatchKey += fmt.Sprintf(":retry:%d", current.InjectionAttempts)
	}
	if op := s.operationByKey(dispatchKey); op != nil && op.Status == "rejected" && execution.Environment != nil {
		if current.InjectionAttempts >= 2 || !now().Before(execution.Environment.Preparation.Deadline) {
			return errors.New("bounded Orca input rejection retry exhausted")
		}
		current.InjectionAttempts++
		dispatchKey = "orca-native-dispatch:" + execution.ExecutionID + ":" + turnID + fmt.Sprintf(":retry:%d", current.InjectionAttempts)
	}
	if op := s.operationByKey(dispatchKey); op != nil {
		id, err := orcaworker.RecoverDispatch(ctx, orcaWorkerMetaCaller(fixedConfig), taskID, execution.Target.OrchestrationRun, *execution.Process)
		if err != nil {
			return err
		}
		current.DispatchID, current.DispatchRequestID = id, op.PlatformRequestID
		s.bindNativeOrcaDispatch(run, execution, current)
		if op.Status != "accepted" && op.Status != "confirmed" {
			return s.changeWorkerOperation(run, op.OperationID, "accepted", op.PlatformRequestID, mustJSON(map[string]string{"dispatch_id": id}), "")
		}
		return s.commitWorker(run)
	}
	if execution.Environment != nil {
		fact, ready, err := orcaworker.InspectAgentInput(ctx, orcaWorkerMetaCaller(fixedConfig), workerOrcaTerminalIdentity(*execution.Process))
		if err != nil {
			return err
		}
		if !ready {
			if err := s.blockWorkerEnvironment(run, fact); err != nil {
				return err
			}
			return errors.New("original Codex input target is not verified")
		}
	}
	dispatchOp, err := s.startWorkerOperation(run, "orca_native_dispatch", dispatchKey, map[string]string{"task": taskID, "terminal": execution.Process.Terminal, "run": execution.Target.OrchestrationRun})
	if err != nil {
		return err
	}
	env, err := orcaworker.DispatchToAgent(ctx, orcaWorkerMetaCaller(fixedConfig), taskID, execution.Target.OrchestrationRun, state.Coordinator.Terminal, *execution.Process, execution.Workspace.Worktree, execution.Workspace.Branch, fixedConfig.agentName(), positiveDuration(fixedConfig.Orca.WaitTimeout, 2*time.Minute), execution.Environment != nil, state.Coordinator, fixedConfig.Repository)
	if err != nil {
		status := "outcome_unknown"
		if orcaworker.NoAgentDetected(env) {
			status = "rejected"
		}
		_ = s.changeWorkerOperation(run, dispatchOp, status, "", nil, err.Error())
		return workerOrcaBackendError(err)
	}
	receipt, err := orcaworker.DispatchReceipt(env, taskID, execution.Target.OrchestrationRun, state.Coordinator.RuntimeID)
	if err != nil {
		return err
	}
	current.DispatchID, current.DispatchRequestID = receipt.ID, receipt.RequestID
	s.bindNativeOrcaDispatch(run, execution, current)
	if err := s.changeWorkerOperation(run, dispatchOp, "accepted", receipt.RequestID, receipt.Payload, ""); err != nil {
		return err
	}
	if orcaworker.PromptAwaitingSubmission(receipt.Payload, execution.Process.Incarnation) {
		if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
			return err
		}
		submitted, err := orcaworker.SubmitPendingPaste(ctx, orcaWorkerMetaCaller(fixedConfig), workerOrcaTerminalIdentity(*execution.Process))
		if err != nil {
			return workerOrcaBackendError(err)
		}
		if submitted {
			s.note(run, "steer", "worker", "accepted", "submitted pending Orca prompt", execution.Target.Dispatch, nil)
		}
	}
	return nil
}

func (s *Service) bindNativeOrcaDispatch(run *Run, execution *ExecutionRecord, turn *OrcaNativeTurn) {
	execution.Target.ExternalTask = turn.TaskID
	s.refreshRun(run)
	execution.Target.Dispatch = turn.DispatchID
	s.refreshRun(run)
	execution.Process.Task, execution.Process.Dispatch, execution.Process.OrchestrationRun = turn.TaskID, turn.DispatchID, execution.Target.OrchestrationRun
	s.refreshRun(run)
	if attempt := s.workflowAttempt(run, run.AttemptID); attempt != nil {
		if turn.Purpose == "readiness" {
			return
		}
		attempt.ExternalBinding = execution.Target
		attempt.OutputSource, attempt.OutputCursor = "", ""
	}
}

func nativeOrcaSessionIdentity(execution *ExecutionRecord) (string, error) {
	if execution == nil || execution.Process == nil || (execution.Process.Kind != "orca-native" && execution.Process.Kind != "orca_supervised") || execution.Process.RuntimeID == "" || execution.Process.Terminal == "" || execution.Process.Incarnation == "" {
		return "", fmt.Errorf("native Orca session origin is incomplete")
	}
	if execution.Process.Kind == "orca_supervised" {
		return remote.SessionID(executionapi.ProcessIdentity(*execution.Process)), nil
	}
	return orcaworker.SessionID(*execution.Process), nil
}

func (s *Service) beginNativeOrcaTurn(run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	if turn.Purpose == "readiness" {
		return nil
	}
	if execution.CurrentTurnID == turn.TurnID {
		return nil
	}
	session, err := nativeOrcaSessionIdentity(execution)
	if err != nil {
		return err
	}
	if err := BeginWorkerTurn(workflow, execution, session, turn.TurnID); err != nil {
		return err
	}
	if attempt := s.workflowAttempt(run, run.AttemptID); attempt != nil {
		attempt.Status, attempt.UpdatedAt = "running", now()
	}
	execution.Status, execution.Error = "active", nil
	s.refreshRun(run)
	if run.Decision != nil && run.Decision.Status == "answer_saved" && orcaQuestionID(run.Decision) == "" {
		s.setDecisionStatus(run, "reply_accepted")
	}
	execution.TurnStartedAt = ptr(now())
	s.refreshRun(run)
	for _, op := range s.domain.Operations {
		if op.IdempotencyKey == "worker-turn:"+execution.ExecutionID+":"+turn.TurnID && op.Status != "confirmed" {
			if err := s.setOperationState(op.OperationID, "confirmed", turn.DispatchRequestID, mustJSON(*turn), ""); err != nil {
				return err
			}
			return s.commitWorker(run, op.OperationID)
		}
	}
	return nil
}

func (s *Service) applyNativeOrcaDecision(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, answer DecisionAnswer, commandID string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	turn := nativeOrcaCurrentTurn(execution)
	if turn != nil && turn.Purpose == "readiness" && run.Decision.Request.OrcaAsk == nil {
		for i := len(execution.OrcaNative.Turns) - 1; i >= 0; i-- {
			if execution.OrcaNative.Turns[i].Purpose == "business" {
				turn = &execution.OrcaNative.Turns[i]
				break
			}
		}
	}
	if ask := run.Decision.Request.OrcaAsk; ask != nil {
		for i := range execution.OrcaNative.Turns {
			if execution.OrcaNative.Turns[i].DispatchID == ask.DispatchID {
				turn = &execution.OrcaNative.Turns[i]
				break
			}
		}
	}
	if execution.Process == nil || (execution.Process.Kind != "orca-native" && execution.Process.Kind != "orca_supervised") || execution.OrcaNative == nil || turn == nil || turn.DispatchID == "" {
		return s.waitWorkerFlow(run, "Original Orca Dispatch identity is unavailable; feedback was retained")
	}
	if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
		execution.Error = ptr("Feedback retained: original Orca session is unavailable: " + err.Error())
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	if orcaQuestionID(run.Decision) != "" {
		if turn.DoneMessageID != "" || execution.LastStoppedTurnID == turn.TurnID {
			execution.Error = ptr("The original Orca question expired with its Dispatch; feedback was retained")
			s.refreshRun(run)
			return s.commitWorker(run)
		}
		key := "orca-native-reply:" + execution.ExecutionID + ":" + commandID
		for _, op := range s.domain.Operations {
			if op.IdempotencyKey == key {
				if op.Status != "confirmed" && op.Status != "accepted" {
					var messageID string
					var err error
					if fixedConfig.hostName() == "orca_supervised" {
						messageID, err = remoteClientForExecution(fixedConfig, execution).FindAnswer(ctx, remoteWorkerRef(remoteExecutionRef(run, execution)), orcaQuestionID(run.Decision), nativeOrcaAnswerText(answer, run.Decision.Request))
					} else {
						messageID, err = findNativeOrcaReply(ctx, fixedConfig, execution.Target.OrchestrationRun, turn.DispatchID, orcaQuestionID(run.Decision), nativeOrcaAnswerText(answer, run.Decision.Request))
					}
					if err != nil || messageID == "" {
						return s.nativeOrcaUncertain(run, "Orca reply outcome is unverified; the original question was not answered twice")
					}
					if err := s.setOperationState(op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"reply_message_id": messageID}), ""); err != nil {
						return err
					}
				}
				s.setDecisionStatus(run, "reply_accepted")
				if err := continueNativeOrcaQuestion(workflow, execution, turn); err != nil {
					return err
				}
				s.refreshRun(run)
				return s.commitWorker(run, op.OperationID)
			}
		}
		body := nativeOrcaAnswerText(answer, run.Decision.Request)
		opID, err := s.startWorkerOperation(run, "orca_native_reply", key, map[string]string{"question": orcaQuestionID(run.Decision), "answer": body, "dispatch": turn.DispatchID})
		if err != nil {
			return err
		}
		var receipt orcaworker.MutationReceipt
		if fixedConfig.hostName() == "orca_supervised" {
			receipt, err = remoteOrcaQuestionReply(ctx, fixedConfig, run, execution, orcaQuestionID(run.Decision), body)
		} else {
			receipt, err = orcaworker.ReplyQuestion(ctx, orcaWorkerMetaCaller(fixedConfig), orcaQuestionID(run.Decision), body, execution.Target.OrchestrationRun, execution.OrcaNative.Coordinator.Terminal, turn.DispatchID, execution.OrcaNative.Coordinator.RuntimeID)
		}
		if err != nil {
			var callErr *orcaworker.CallError
			if errors.As(err, &callErr) {
				_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
				return s.nativeOrcaUncertain(run, "Orca reply outcome needs reconciliation: "+err.Error())
			}
			return s.nativeOrcaUncertain(run, "Orca reply receipt lacks its original request identity")
		}
		if err := s.setOperationState(opID, "confirmed", receipt.RequestID, receipt.Payload, ""); err != nil {
			return err
		}
		s.setDecisionStatus(run, "reply_accepted")
		if err := continueNativeOrcaQuestion(workflow, execution, turn); err != nil {
			return err
		}
		s.refreshRun(run)
		return s.commitWorker(run, opID)
	}
	if turn.DoneMessageID == "" && execution.CurrentTurnID == turn.TurnID && execution.LastStoppedTurnID != turn.TurnID {
		return s.sendNativeOrcaFeedback(ctx, run, workflow, execution, turn, answer, commandID)
	}
	if turn.DoneMessageID == "" || execution.LastStoppedTurnID != turn.TurnID {
		execution.Error = ptr("Feedback retained until the original Orca Dispatch settles")
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	if op := s.operationByKey("orca-native-send:" + execution.ExecutionID + ":" + commandID); op != nil && op.Status != "rejected" {
		execution.Error = ptr("Feedback may have been accepted by the settled Orca Dispatch; it was not copied into a new Task")
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	for _, op := range s.domain.Operations {
		if op.IdempotencyKey == "worker-turn:"+execution.ExecutionID+":"+commandID {
			if op.Status == "accepted" || op.Status == "confirmed" {
				return nil
			}
			execution.Error = ptr("Feedback delivery remains unverified; no duplicate Orca Task was created")
			s.refreshRun(run)
			return s.commitWorker(run)
		}
	}
	return s.startWorkerFlowTurn(ctx, run, workerFeedbackText(answer, run.Decision.Request), commandID)
}

func (s *Service) sendNativeOrcaFeedback(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turn *OrcaNativeTurn, answer DecisionAnswer, commandID string) error {
	return s.sendNativeOrcaFeedbackToDecision(ctx, run, workflow, execution, turn, answer, commandID, run.Decision.Request, run.Decision.ID, true)
}

func continueNativeOrcaQuestion(workflow *WorkflowRunRecord, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	if turn.Purpose == "readiness" {
		workflow.Phase = "queued"
		if execution.Environment != nil && execution.Environment.Blocker != nil {
			workflow.Phase = "waiting_decision"
		}
		workflow.UpdatedAt = now()
		return nil
	}
	return ContinueWorkerQuestion(workflow, execution, turn.TurnID)
}

func (s *Service) sendNativeOrcaFeedbackToDecision(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turn *OrcaNativeTurn, answer DecisionAnswer, commandID string, request DecisionRequest, decisionID string, resumeQuestion bool) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	key := "orca-native-send:" + execution.ExecutionID + ":" + commandID
	name := "SteerLine feedback " + commandID
	body := workerFeedbackText(answer, request)
	accepted := func() error {
		for i := range s.domain.Decisions {
			record := &s.domain.Decisions[i]
			if record.DecisionID == decisionID && record.Status != "reply_accepted" {
				record.Status = "reply_accepted"
				record.Version++
				record.UpdatedAt = now()
				s.markWorkerDecisionWrite(decisionID)
			}
		}
		if resumeQuestion {
			if err := ContinueWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
				return err
			}
		}
		s.refreshRun(run)
		return nil
	}
	for _, op := range s.domain.Operations {
		if op.IdempotencyKey != key {
			continue
		}
		if op.Status == "confirmed" || op.Status == "accepted" {
			if err := accepted(); err != nil {
				return err
			}
			s.refreshRun(run)
			return s.commitWorker(run)
		}
		messageID, err := findNativeOrcaSend(ctx, fixedConfig, execution.Target.OrchestrationRun, turn.DispatchID, name, body)
		if err != nil || messageID == "" {
			execution.Error = ptr("Orca feedback send remains unverified; it was not copied to another Dispatch")
			s.refreshRun(run)
			return s.commitWorker(run)
		}
		if err := s.setOperationState(op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"message_id": messageID}), ""); err != nil {
			return err
		}
		if err := accepted(); err != nil {
			return err
		}
		s.refreshRun(run)
		return s.commitWorker(run, op.OperationID)
	}
	opID, err := s.startWorkerOperation(run, "orca_native_send", key, map[string]string{"dispatch": turn.DispatchID, "subject": name, "body": body})
	if err != nil {
		return err
	}
	receipt, err := orcaworker.SendFeedback(ctx, orcaWorkerMetaCaller(fixedConfig), name, body, turn.DispatchID, execution.Target.OrchestrationRun, execution.OrcaNative.Coordinator.Terminal, execution.OrcaNative.Coordinator.RuntimeID)
	if err != nil {
		var callErr *orcaworker.CallError
		if errors.As(err, &callErr) {
			if orcaworker.FeedbackDispatchInactive(err) {
				_ = s.changeWorkerOperation(run, opID, "rejected", "", nil, err.Error())
				execution.Error = ptr("Original Orca Dispatch is inactive; feedback remains saved until settlement is confirmed")
				s.refreshRun(run)
				return s.commitWorker(run)
			}
			_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
			return s.nativeOrcaUncertain(run, "Orca send receipt is unknown: "+err.Error())
		}
		return s.nativeOrcaUncertain(run, "Orca accepted feedback without an exact Dispatch receipt")
	}
	if err := s.setOperationState(opID, "confirmed", receipt.RequestID, receipt.Payload, ""); err != nil {
		return err
	}
	if err := accepted(); err != nil {
		return err
	}
	execution.LastEvent = ptr("Orca accepted feedback for the current Dispatch; agent consumption is unverified")
	s.refreshRun(run)
	return s.commitWorker(run, opID)
}
