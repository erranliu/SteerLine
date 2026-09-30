package steerline

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func nativeOrcaCurrentTurn(execution *ExecutionRecord) *OrcaNativeTurn {
	if execution == nil || execution.OrcaNative == nil || len(execution.OrcaNative.Turns) == 0 {
		return nil
	}
	return &execution.OrcaNative.Turns[len(execution.OrcaNative.Turns)-1]
}

func nativeOrcaSeen(state *OrcaNativeExecution, id string) bool {
	for _, seen := range state.SeenMessages {
		if seen == id {
			return true
		}
	}
	return false
}

func (s *Service) nativeOrcaUncertain(run *Run, reason string) error {
	if run == nil {
		return errors.New(reason)
	}
	execution := s.executionRecord(run)
	diagnostic := clip(reason, 2000)
	if execution.Status == "blocked" && execution.Error != nil && *execution.Error == diagnostic {
		return nil
	}
	execution.Error = ptr(diagnostic)
	s.refreshRun(run)
	execution.Status = "blocked"
	s.refreshRun(run)
	s.note(run, "steer", "worker", "unverified", "Orca observation needs reconciliation", reason, nil)
	return s.commitWorker(run)
}

func (s *Service) pollNativeOrcaWorker(ctx context.Context, run *Run) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || execution.OrcaNative == nil {
		return errors.New("native Orca poll has no fixed execution")
	}
	turn := nativeOrcaCurrentTurn(execution)
	if turn == nil || turn.DispatchID == "" {
		return s.nativeOrcaUncertain(run, "Orca Task has no confirmed Dispatch; original request must be reconciled")
	}
	if execution.LastStoppedTurnID == turn.TurnID && execution.OrcaNative.DeliveryID == "" {
		return nil
	}
	state := execution.OrcaNative
	if state.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		return errors.New("native Orca inbox has no bound coordinator/Run")
	}
	if turn.DoneMessageID == "" {
		delivery, err := readNativeOrcaDelivery(ctx, fixedConfig, execution)
		if err != nil {
			if ctx.Err() != nil {
				return nil // Service shutdown is not a worker failure.
			}
			return s.nativeOrcaUncertain(run, "Orca inbox is temporarily unavailable: "+err.Error())
		}
		if len(delivery.Messages) > 0 {
			for _, message := range delivery.Messages {
				if message.ID == "" || nativeOrcaSeen(state, message.ID) {
					continue
				}
				fact, matched, err := decodeNativeOrcaMessage(message, execution.Target.OrchestrationRun, *turn, execution.Process.Terminal)
				if err != nil {
					return err
				}
				if !matched {
					state.SeenMessages = append(state.SeenMessages, message.ID)
					continue
				}
				switch fact.Kind {
				case "question":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					if run.Decision == nil || orcaQuestionID(run.Decision) != fact.MessageID {
						decision, err := WriteDecisionRequest(run, *fact.Question)
						if err != nil {
							return err
						}
						s.setRunDecision(run, decision)
					}
					if err := NoteWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
						return err
					}
					s.refreshRun(run)
				case "worker_done":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					turn.DoneMessageID, turn.Outcome = fact.MessageID, fact.Outcome
					if err := s.bindNativeOrcaPRCandidates(ctx, run, workflow, execution, fact.PRCandidates); err != nil {
						execution.Error = ptr(clip(err.Error(), 2000))
						s.refreshRun(run)
					}
				case "status":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					if err := s.bindNativeOrcaPRCandidates(ctx, run, workflow, execution, fact.PRCandidates); err != nil {
						execution.Error = ptr(clip(err.Error(), 2000))
						s.refreshRun(run)
					}
				case "escalation":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					if run.Decision == nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved" {
						decision, err := WriteDecisionRequest(run, *fact.Question)
						if err != nil {
							return err
						}
						s.setRunDecision(run, decision)
					}
					if err := NoteWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
						return err
					}
					s.refreshRun(run)
				}
				state.SeenMessages = append(state.SeenMessages, message.ID)
			}
			state.DeliveryID = delivery.DeliveryID
			if err := s.commitWorker(run); err != nil {
				return err
			} // Entire Delivery precedes acknowledgement.
		}
	}
	if state.DeliveryID != "" {
		if err := s.ackNativeOrcaDelivery(ctx, run, execution, state.DeliveryID); err != nil {
			return s.nativeOrcaUncertain(run, "Orca Delivery acknowledgement needs reconciliation: "+err.Error())
		}
		state.DeliveryID = ""
		if err := s.commitWorker(run); err != nil {
			return err
		}
	}
	if turn.DoneMessageID == "" {
		if execution.RecoveryRequired {
			path, pathErr := nativeOrcaRelayPath(execution.Workspace.Worktree, run.RunID, turn.TurnID)
			if pathErr != nil {
				return s.nativeOrcaUncertain(run, "Orca result relay path cannot be verified: "+pathErr.Error())
			}
			if _, statErr := os.Stat(path); statErr == nil {
				request, readErr := readNativeOrcaDoneRequest(path, execution.Workspace.Worktree, run, turn)
				if readErr != nil || request == nil {
					return s.nativeOrcaUncertain(run, "Orca result relay request cannot be verified")
				}
				execution.RecoveryState = "relay_pending"
				if run.Decision == nil || len(run.Decision.Request.Options) != 1 || run.Decision.Request.Options[0].ID != "forward_result" {
					oldDecisionID := ""
					contextText := "原 worker 已写入结果请求。请核对原 Dispatch、结果与当前工作区后决定是否转发。"
					if run.Decision != nil && (run.Decision.Status == "pending" || run.Decision.Status == "answer_saved") {
						oldDecisionID = run.Decision.ID
						contextText += "\n\n原待答输入：" + run.Decision.Request.Question + "\n" + run.Decision.Request.Context
						s.setDecisionStatus(run, "superseded")
						if err := s.commitWorker(run); err != nil {
							return err
						}
						s.setRunDecision(run, nil)
					}
					decision, err := WriteDecisionRequest(run, DecisionRequest{Question: "Forward the original Orca worker result?", Context: clip(contextText, 7000), Options: []DecisionOption{{ID: "forward_result", Label: "转发原结果"}}, AllowFreeText: true})
					if err != nil {
						return err
					}
					s.setRunDecision(run, decision)
					if record := s.decisionRecord(run); record != nil {
						record.RecoveryOfDecision = oldDecisionID
					}
				}
				workflow.Phase, workflow.UpdatedAt = "waiting_decision", now()
				s.refreshRun(run)
				return s.commitWorker(run)
			} else if !os.IsNotExist(statErr) {
				return s.nativeOrcaUncertain(run, "Orca result relay file cannot be inspected: "+statErr.Error())
			}
			return nil // Restart observation never sends a new worker_done.
		}
		if err := s.relayNativeOrcaDone(ctx, run, execution, turn); err != nil {
			if ctx.Err() != nil {
				return nil // Service shutdown is not a worker result failure.
			}
			return s.nativeOrcaUncertain(run, "Orca worker result relay needs reconciliation: "+err.Error())
		}
	}
	if err := s.readNativeOrcaOutput(ctx, run, execution, turn); err != nil {
		if persistenceErr := s.store.Err(); persistenceErr != nil {
			return persistenceErr
		}
		execution.Error = ptr(clip("Orca output is bounded or unavailable: "+err.Error(), 2000))
		s.refreshRun(run)
		if commitErr := s.commitWorker(run); commitErr != nil {
			return commitErr
		}
	}
	if turn.DoneMessageID != "" {
		return s.finishNativeOrcaTurn(ctx, run, workflow, execution, turn)
	}
	if execution.Status == "blocked" && execution.Error != nil && strings.Contains(*execution.Error, "context canceled") {
		if err := verifyNativeOrcaActiveDispatch(ctx, fixedConfig, execution, turn); err != nil {
			return err
		}
		execution.Status, execution.Error = "pending", nil
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	if workflow.Phase == "executing" && run.ExecutionStatus == "unknown" {
		execution.Status = "active"
		execution.Error = nil
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	return nil
}

func (s *Service) bindNativeOrcaPRCandidates(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, urls []string) error {
	if len(urls) == 0 {
		return nil
	}
	if len(urls) != 1 {
		return errors.New("worker reported multiple PR links")
	}
	workflow.PRCandidateURL = urls[0]
	info, err := s.prs.Query(ctx, urls[0])
	if err != nil {
		return err
	}
	pendingQuestion := run.Decision != nil && run.Decision.Status == "pending" && orcaQuestionID(run.Decision) != ""
	if err := BindWorkerPR(workflow, execution, info, urls[0], s.projectExecutionConfig().PR.Repository, pendingQuestion); err != nil {
		return err
	}
	workflow.PRURL, workflow.PRState = info.URL, info.State
	s.refreshRun(run)
	return nil
}

func (s *Service) ackNativeOrcaDelivery(ctx context.Context, run *Run, execution *ExecutionRecord, deliveryID string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	state := execution.OrcaNative
	key := "orca-native-ack:" + execution.ExecutionID + ":" + deliveryID
	op := s.operationByKey(key)
	if op != nil && op.Status == "confirmed" {
		return nil
	}
	if op != nil {
		// The ack response may have been lost after Orca advanced its FIFO.
		// Read the original Run before deciding whether another ack is needed.
		current, err := orcaworker.CheckInbox(ctx, orcaWorkerMetaCaller(fixedConfig), state.Coordinator.Terminal, execution.Target.OrchestrationRun)
		if err != nil {
			return err
		}
		if current.RuntimeID != state.Coordinator.RuntimeID || current.RunID != execution.Target.OrchestrationRun {
			return errors.New("Orca Delivery recovery changed coordinator or Run")
		}
		if current.DeliveryID != deliveryID {
			return s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"delivery_id": deliveryID, "proof": "absent_from_original_run_fifo"}), "")
		}
	}
	opID := ""
	if op == nil {
		var err error
		opID, err = s.startWorkerOperation(run, "orca_native_ack", key, map[string]string{"delivery": deliveryID, "run": execution.Target.OrchestrationRun, "coordinator": state.Coordinator.Terminal})
		if err != nil {
			return err
		}
	} else {
		opID = op.OperationID
	}
	receipt, err := orcaworker.AckInbox(ctx, orcaWorkerMetaCaller(fixedConfig), state.Coordinator.Terminal, execution.Target.OrchestrationRun, deliveryID, state.Coordinator.RuntimeID)
	if err != nil {
		var callErr *orcaworker.CallError
		if errors.As(err, &callErr) {
			_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
		}
		return err
	}
	return s.changeWorkerOperation(run, opID, "confirmed", receipt.RequestID, receipt.Payload, "")
}

func (s *Service) readNativeOrcaOutput(ctx context.Context, run *Run, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	attempt := s.workflowAttempt(run, run.AttemptID)
	if attempt == nil {
		return errors.New("worker output has no attempt")
	}
	result, resetCursor, err := orcaworker.ReadWorkerOutput(ctx, orcaWorkerMetaCaller(fixedConfig), turn.DispatchID, attempt.OutputCursor, execution.OrcaNative.Coordinator.RuntimeID)
	if resetCursor {
		attempt.OutputCursor = ""
	}
	if err != nil {
		return err
	}
	attempt.OutputSource = result.Source
	attempt.OutputCursor = result.NextCursor
	if attempt.OutputCursor == "" {
		attempt.OutputCursor = result.Cursor
	}
	turn.OutputSource, turn.OutputCursor, turn.OutputComplete = result.Source, attempt.OutputCursor, result.ContentComplete
	turn.OutputClipping, turn.OutputFallbackReason = append([]string(nil), result.Clipping...), result.FallbackReason
	content := strings.Join(result.Terminal.Tail, "\n")
	if result.Source == "transcript" && len(result.Transcript) > 0 {
		content = string(result.Transcript)
	}
	if content != "" {
		digest := digestValue(struct{ Source, Content string }{result.Source, content})
		if digest != turn.OutputDigest {
			turn.OutputDigest = digest
			status := "bounded"
			if result.ContentComplete && len(result.Clipping) == 0 {
				status = "complete"
			}
			s.note(run, "worker", "output", status, "Orca worker output", clip(content, 7000), map[string]any{"source": result.Source, "fallback_reason": result.FallbackReason, "clipping": result.Clipping, "dispatch_id": turn.DispatchID})
		}
	}
	if result.SourceChanged || len(result.Clipping) > 0 || !result.ContentComplete {
		execution.LastEvent = ptr("Orca output is bounded (" + result.Source + "; " + result.FallbackReason + ")")
		s.refreshRun(run)
	}
	return s.commitWorker(run)
}

func (s *Service) finishNativeOrcaTurn(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	settlement, err := readNativeOrcaSettlement(ctx, fixed.config, execution, turn)
	if err != nil {
		return s.nativeOrcaUncertain(run, "Orca worker_done arrived but Dispatch settlement is unverified: "+err.Error())
	}
	if run.Decision != nil && orcaQuestionID(run.Decision) != "" && run.Decision.Status == "answer_saved" {
		return s.nativeOrcaUncertain(run, "Orca Dispatch settled while the original question reply remains unverified")
	}
	pendingQuestion := run.Decision != nil && run.Decision.Status == "pending" && orcaQuestionID(run.Decision) != ""
	if err := s.stopVerifiedWorkerTurn(run, workflow, execution, turn.TurnID, settlement.Outcome, pendingQuestion); err != nil {
		return err
	}
	if pendingQuestion {
		s.setDecisionStatus(run, "superseded")
		if err := s.commitWorker(run); err != nil {
			return err
		}
		s.setRunDecision(run, nil)
	}
	if err := s.resolveStoppedWorkerTurn(run, workflow, execution, stoppedWorkerResolution{TurnID: turn.TurnID, WaitReason: "The original Orca Dispatch settled (" + settlement.Outcome + "). Send feedback to continue the same agent session."}); err != nil {
		return err
	}
	removeNativeOrcaRelayRequest(execution.Workspace.Worktree, run.RunID, turn.TurnID)
	return nil
}
