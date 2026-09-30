package steerline

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

func (s *Service) eventWorkerFlowFor(run *Run, ev WorkerEvent) (resultErr error) {
	defer func() { ackWorkerEventPersistence(ev, resultErr) }()
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || workflow.WorkflowVersion != WorkerFlowVersion {
		return errors.New("worker event has no fixed worker-flow execution")
	}
	if ev.RunID != "" && ev.RunID != run.RunID || ev.AttemptID != "" && ev.AttemptID != run.AttemptID {
		return ErrWorkerTurnStale
	}
	if workflow.Phase == "completed" || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		if ev.Activity.Kind != "" {
			err := s.recordConversationEvent(run, ev)
			if err != nil {
				return err
			}
			s.recordActivity(run, ev.Activity)
		}
		return nil
	}
	mode, err := s.workerFlowMode(run)
	if err != nil {
		return err
	}
	fact, err := decodeWorkerFlowEvent(mode, ev, execution, run.Decision != nil && run.Decision.Request.NativeAsk != nil)
	if err != nil {
		return err
	}
	var operationIDs []string
	if fact.SessionID != "" {
		execution.SessionID = fact.SessionID
		s.refreshRun(run)
	}
	if fact.StartedTurnID != "" && fact.Question == nil && mode == workerflow.LocalCodex {
		opID, err := s.recordAppServerTurnStarted(run, workflow, execution, fact.StartedTurnID)
		if err != nil {
			return err
		}
		operationIDs = append(operationIDs, opID)
	} else if fact.StartedTurnID != "" {
		turnID := fact.StartedTurnID
		commandID := turnID
		if err := BeginWorkerTurn(workflow, execution, execution.SessionID, turnID); err != nil {
			return err
		}
		if attempt := s.workflowAttempt(run, run.AttemptID); attempt != nil {
			attempt.Status, attempt.UpdatedAt = "running", now()
		}
		if run.Decision != nil && run.Decision.Status == "answer_saved" {
			s.setDecisionStatus(run, "reply_accepted")
		}
		for _, op := range s.domain.Operations {
			if op.IdempotencyKey == "worker-turn:"+run.ExecutionID+":"+commandID {
				requestID := op.PlatformRequestID
				if requestID == "" {
					requestID = turnID
				}
				if err := s.setOperationState(op.OperationID, "confirmed", requestID, mustJSON(map[string]string{"session_id": execution.SessionID, "turn_id": turnID}), ""); err != nil {
					return err
				}
				operationIDs = append(operationIDs, op.OperationID)
				break
			}
		}
		execution.Status, execution.Error = "active", nil
		execution.TurnStartedAt = ptr(now())
		s.refreshRun(run)
		execution.TurnEndedAt = nil
		s.refreshRun(run)
	}
	if fact.Question != nil {
		call := *fact.Question
		if run.Decision != nil && run.Decision.Request.NativeAsk != nil && run.Decision.Request.NativeAsk.CallID == call.CallID {
			// Replayed notification for the original request.
		} else {
			seen := false
			for _, pending := range execution.PendingAsks {
				if pending.CallID == call.CallID {
					seen = true
					break
				}
			}
			if !seen {
				if run.Decision != nil && (run.Decision.Status == "pending" || run.Decision.Status == "answer_saved") {
					execution.PendingAsks = append(execution.PendingAsks, call)
				} else {
					decision, err := appServerAskDecision(run, call)
					if err != nil {
						return err
					}
					s.setRunDecision(run, decision)
				}
			}
		}
		if err := NoteWorkerQuestion(workflow, execution, execution.CurrentTurnID); err != nil {
			return err
		}
		s.refreshRun(run)
		s.note(run, "steer", "decision", "pending", "Codex is waiting for an answer in the current turn", call.CallID, decisionMeta(run))
	}
	if fact.ToolReply != nil && run.Decision != nil && run.Decision.Request.NativeAsk != nil {
		reply := fact.ToolReply
		ask := run.Decision.Request.NativeAsk
		if reply.CallID == ask.CallID {
			if run.Decision.Status == "answer_saved" && reply.Status == "completed" && reply.Success != nil && *reply.Success && reply.MatchesAnswers(run.Decision.Answers) {
				confirmed := false
				for _, op := range s.domain.Operations {
					if op.IdempotencyKey == "appserver-answer:"+execution.ExecutionID+":"+run.Decision.ID && (op.Status == "in_flight" || op.Status == "accepted" || op.Status == "outcome_unknown" || op.Status == "confirmed") {
						if op.Status != "confirmed" {
							if err := s.setOperationState(op.OperationID, "confirmed", ask.CallID, mustJSON(map[string]any{"tool_item_id": reply.CallID, "native_status": reply.Status, "success": true}), ""); err != nil {
								return err
							}
							operationIDs = append(operationIDs, op.OperationID)
						}
						confirmed = true
						break
					}
				}
				if !confirmed {
					return errors.New("Codex tool completed without the original saved reply intent")
				}
				s.setDecisionStatus(run, "reply_accepted")
				if err := s.commitWorker(run, operationIDs...); err != nil {
					return err
				}
				operationIDs = nil
				workflow, execution = s.workflowRecords(run)
				if len(execution.PendingAsks) > 0 {
					next := execution.PendingAsks[0]
					execution.PendingAsks = execution.PendingAsks[1:]
					decision, err := appServerAskDecision(run, next)
					if err != nil {
						return err
					}
					s.setRunDecision(run, decision)
				} else {
					if err := ContinueWorkerQuestion(workflow, execution, execution.CurrentTurnID); err != nil {
						return err
					}
					s.refreshRun(run)
				}
			} else if reply.Status == "failed" || reply.Success != nil && !*reply.Success {
				execution.Error = ptr("Codex rejected the saved answer for its original tool call")
				s.refreshRun(run)
			}
		}
	}
	execution.LastEvent = ptr(ev.Summary)
	s.refreshRun(run)
	if ev.Activity.Kind != "" {
		if err := s.recordConversationEvent(run, ev); err != nil {
			return err
		}
		s.recordActivity(run, ev.Activity)
		s.emitConsole(ev.Activity)
	} else if ev.Persisted != nil {
		err := errors.New("worker source event lacks activity classification")
		return err
	}
	return s.commitWorker(run, operationIDs...)
}
