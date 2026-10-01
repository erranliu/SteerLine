package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
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

func (s *Service) validateNativeOrcaQuestionFact(run *Run, execution *ExecutionRecord, turn *OrcaNativeTurn, fact executionapi.EventFact) error {
	if run == nil || execution == nil || execution.Process == nil || turn == nil || fact.QuestionRoute == nil || len(fact.Questions) == 0 {
		return errors.New("inbox question lacks its fixed route or execution")
	}
	if fact.QuestionRoute.TurnID != turn.TurnID {
		return ErrWorkerTurnStale
	}
	mode, err := s.workerFlowRegistration(run)
	if err != nil {
		return err
	}
	want := executionapi.QuestionRouteIdentity{ProjectID: execution.ProjectIdentity, WorkflowRunID: run.RunID,
		ExecutionID: execution.ExecutionID, AttemptID: run.AttemptID, Generation: nativeOrcaGeneration(execution),
		TurnID: turn.TurnID, QuestionID: fact.QuestionRoute.QuestionID}
	return validateQuestionRoute(fact.QuestionRoute, fact.Questions, want, mode.QuestionCodecKey, mode.QuestionCodecVersion)
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

func (s *Service) pollNativeOrcaWorker(ctx context.Context, run *Run) (retErr error) {
	defer func() {
		if err := s.observeWorkerEnvironment(ctx, run); retErr == nil {
			retErr = err
		}
		if retErr == nil {
			retErr = s.flushEnvironmentFeedback(ctx, run)
		}
	}()
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
		delivery, err := readNativeOrcaDelivery(ctx, fixedConfig, run, execution, turn)
		if err != nil {
			if ctx.Err() != nil {
				return nil // Service shutdown is not a worker failure.
			}
			if execution.Environment == nil {
				return s.nativeOrcaUncertain(run, "Orca inbox is temporarily unavailable: "+err.Error())
			}
			execution.LastEvent = ptr("Orca inbox observation gap: " + clip(err.Error(), 500))
		}
		if len(delivery.Facts) > 0 {
			for _, fact := range delivery.Facts {
				if fact.SourceEventID == "" {
					continue
				}
				if fact.Kind == executionapi.EventQuestion {
					if err := s.validateNativeOrcaQuestionFact(run, execution, turn, fact); err != nil {
						return err
					}
				}
				if nativeOrcaSeen(state, fact.SourceEventID) {
					if fact.Kind == executionapi.EventQuestion && fact.QuestionRoute != nil {
						seen, err := s.routedQuestionSeen(run, execution, fact.QuestionRoute, fact.Questions)
						if err != nil {
							return err
						}
						if !seen {
							return errors.New("replayed inbox question identity has no matching persisted route")
						}
					}
					continue
				}
				if !fact.Matched {
					state.SeenMessages = append(state.SeenMessages, fact.SourceEventID)
					continue
				}
				switch fact.Kind {
				case executionapi.EventQuestion:
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					seen, err := s.routedQuestionSeen(run, execution, fact.QuestionRoute, fact.Questions)
					if err != nil {
						return err
					}
					if !seen {
						decision, err := WriteDecisionRequest(run, decisionRequestFromEventFact(fact))
						if err != nil {
							return err
						}
						s.setOrcaQuestion(run, decision)
					}
					if turn.Purpose != "readiness" {
						if err := NoteWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
							return err
						}
					} else {
						workflow.Phase = "waiting_decision"
					}
					s.refreshRun(run)
				case executionapi.EventCompleted:
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					turn.DoneMessageID, turn.Outcome = fact.SourceEventID, fact.Outcome
					if turn.Purpose != "readiness" {
						if err := s.bindNativeOrcaPRCandidates(ctx, run, workflow, execution, fact.PRCandidates); err != nil {
							execution.Error = ptr(clip(err.Error(), 2000))
							s.refreshRun(run)
						}
					}
				case executionapi.EventStatus:
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					if turn.Purpose != "readiness" {
						if err := s.bindNativeOrcaPRCandidates(ctx, run, workflow, execution, fact.PRCandidates); err != nil {
							execution.Error = ptr(clip(err.Error(), 2000))
							s.refreshRun(run)
						}
					}
				case executionapi.EventEscalation:
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					if run.Decision == nil || run.Decision.Request.HostTarget != nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved" {
						decision, err := WriteDecisionRequest(run, decisionRequestFromEventFact(fact))
						if err != nil {
							return err
						}
						s.setOrcaQuestion(run, decision)
					}
					if turn.Purpose != "readiness" {
						if err := NoteWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
							return err
						}
					} else {
						workflow.Phase = "waiting_decision"
					}
					s.refreshRun(run)
				}
				state.SeenMessages = append(state.SeenMessages, fact.SourceEventID)
			}
			state.DeliveryID = delivery.DeliveryID
			if turn.Purpose == "readiness" && execution.Environment != nil {
				if err := s.readNativeOrcaOutput(ctx, run, execution, turn); err != nil {
					if s.store.Err() != nil {
						return s.store.Err()
					}
					execution.LastEvent = ptr("准备工具证据观察缺口：" + clip(err.Error(), 500))
				}
				p := execution.Environment.Preparation
				if p.Receipt == nil {
					receipt, err := orcaworker.ReadProbe(p.Probe, p.InputDigest)
					if err != nil {
						if err := s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "probe_receipt_invalid", Confidence: "confirmed", Source: "readiness.receipt", Detail: err.Error(), Turn: turn.TurnID}); err != nil {
							return err
						}
					} else {
						p.Receipt = receipt
					}
				}
			}
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
	entry, err := s.workerFlowRegistration(run)
	if err != nil {
		return err
	}
	if entry.RelayResult != nil {
		handled, err := entry.RelayResult(s, ctx, run, execution, turn)
		if handled || err != nil {
			return err
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
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	workflow.PRCandidateURL = urls[0]
	info, err := publicationClient(fixed.config).Query(ctx, urls[0])
	if err != nil {
		return err
	}
	pendingQuestion := run.Decision != nil && run.Decision.Status == "pending" && orcaQuestionID(run.Decision) != ""
	if err := BindWorkerPR(workflow, execution, info, urls[0], fixed.config.PR.Repository, pendingQuestion); err != nil {
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
	ops, err := orcaOperationsFor(fixedConfig)
	if err != nil {
		return err
	}
	state := execution.OrcaNative
	key := "orca-native-ack:" + execution.ExecutionID + ":" + deliveryID
	op := s.operationByKey(key)
	if op != nil && op.Status == "confirmed" {
		return nil
	}
	if op != nil {
		// The ack response may have been lost after Orca advanced its FIFO.
		// Read the original Run before deciding whether another ack is needed.
		var absent bool
		var err error
		absent, err = ops.DeliveryAcknowledged(ctx, run, execution, deliveryID)
		if err != nil {
			return err
		}
		if absent {
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
	receipt, err := ops.Ack(ctx, run, execution, deliveryID)
	if err != nil {
		var outcome *executionapi.BackendError
		if errors.As(err, &outcome) && outcome.Code == executionapi.BackendOutcomeUnknown {
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
	result, resetCursor, err := orcaworker.ReadWorkerOutput(ctx, orcaWorkerMetaCaller(fixedConfig), turn.DispatchID, turn.OutputCursor, execution.OrcaNative.Coordinator.RuntimeID)
	if resetCursor {
		attempt.OutputCursor = ""
	}
	if err != nil {
		return err
	}
	cursor := result.NextCursor
	if cursor == "" {
		cursor = result.Cursor
	}
	if turn.Purpose != "readiness" {
		attempt.OutputSource, attempt.OutputCursor = result.Source, cursor
	}
	turn.OutputSource, turn.OutputCursor, turn.OutputComplete = result.Source, cursor, result.ContentComplete
	if turn.Purpose == "readiness" && execution.Environment != nil {
		p := execution.Environment.Preparation
		if resetCursor {
			p.Evidence = orcaworker.ProbeToolEvidence{}
		}
		p.Evidence, err = orcaworker.ObserveProbe(result, p.Command, p.Nonce, p.ScriptDigest, p.Evidence)
		if err != nil {
			return err
		}
		if p.Evidence.FailureCode != "" {
			return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: p.Evidence.FailureCode, Confidence: "confirmed", Source: "orca.transcript.tool-result", Turn: turn.TurnID})
		}
	}
	if turn.Purpose == "business" && execution.Environment != nil {
		if resetCursor {
			turn.ToolContext = orcaworker.ToolContext{}
		}
		var failed bool
		turn.ToolContext, failed, err = orcaworker.ObserveToolInitialization(result, turn.TaskID, turn.IntentAt.UnixMilli(), turn.ToolContext)
		if err != nil {
			return err
		}
		if failed {
			return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "tool_initialization_failed", Confidence: "confirmed", Source: "orca.transcript.tool-result", Turn: turn.TurnID})
		}
		if turn.ToolContext.Started && execution.CurrentTurnID != turn.TurnID {
			workflow := s.workflowRecord(run)
			if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
				return err
			}
		}
	}
	turn.OutputClipping, turn.OutputFallbackReason = append([]string(nil), result.Clipping...), result.FallbackReason
	content := result.DisplayContent()
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
	turn.Settled = true
	if execution.Environment != nil && execution.Environment.DeferredDecisionID != "" {
		for i := range s.domain.Decisions {
			record := &s.domain.Decisions[i]
			if record.DecisionID != execution.Environment.DeferredDecisionID || record.Status != "pending" {
				continue
			}
			var request DecisionRequest
			_ = json.Unmarshal(record.Request, &request)
			if request.QuestionRoute != nil && request.QuestionRoute.TurnID == turn.TurnID {
				record.Status = "superseded"
				record.Version++
				record.UpdatedAt = now()
				s.markWorkerDecisionWrite(record.DecisionID)
				execution.Environment.DeferredDecisionID = ""
			}
		}
	}
	if turn.Purpose == "readiness" {
		return s.finishOrcaPreparation(ctx, run, execution, turn, settlement.Outcome)
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
	entry, err := s.workerFlowRegistration(run)
	if err != nil {
		return err
	}
	if entry.ClearRelay != nil {
		entry.ClearRelay(execution, run, turn)
	}

	return nil
}
