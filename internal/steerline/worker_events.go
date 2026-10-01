package steerline

import (
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
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
			if err := s.recordConversationEvent(run, ev); err != nil {
				return err
			}
			s.recordActivity(run, ev.Activity)
		}
		return nil
	}
	mode, err := s.workerFlowRegistration(run)
	if err != nil {
		return err
	}
	fact := ev.Fact
	if ev.SourceGap || ev.OutputGap {
		fact.Kind, fact.Gap = executionapi.EventGap, true
	}
	if fact.Kind == executionapi.EventInputAccepted {
		fact.InputAccepted = true
	}
	if fact.Kind == executionapi.EventGap {
		fact.Gap = true
	}
	var operationIDs []string
	if ev.Diagnostic != "" && fact.Kind == "" && len(fact.Questions) == 0 && fact.ToolReply == nil {
		return errors.New(ev.Diagnostic)
	}
	if fact.SessionID != "" {
		if execution.SessionID != "" && fact.SessionID != execution.SessionID {
			return ErrWorkerTurnStale
		}
		execution.SessionID = fact.SessionID
		s.refreshRun(run)
	}
	if fact.StartedTurnID != "" && fact.Kind == executionapi.EventStarted {
		opID, err := s.recordAppServerTurnStarted(run, workflow, execution, fact.StartedTurnID)
		if err != nil {
			return err
		}
		if opID != "" {
			operationIDs = append(operationIDs, opID)
		}
		workflow, execution = s.workflowRecords(run)
	}
	if fact.QuestionRoute != nil || len(fact.Questions) != 0 {
		if fact.QuestionRoute == nil || len(fact.Questions) == 0 {
			return errors.New("worker question facts lack their fixed route")
		}
		if err := validateQuestionRouteForRun(run, execution, fact.QuestionRoute, fact.Questions, mode.QuestionCodecKey, mode.QuestionCodecVersion); err != nil {
			return err
		}
		routeID := fact.QuestionRoute.QuestionID
		seen, err := s.routedQuestionSeen(run, execution, fact.QuestionRoute, fact.Questions)
		if err != nil {
			return err
		}
		if !seen {
			if run.Decision != nil && (run.Decision.Status == "pending" || run.Decision.Status == "answer_saved") {
				execution.PendingQuestions = append(execution.PendingQuestions, PendingQuestion{Questions: cloneExecutionQuestions(fact.Questions), Route: fact.QuestionRoute.Clone()})
			} else {
				decision, err := WriteDecisionRequest(run, decisionRequestFromEventFact(fact))
				if err != nil {
					return err
				}
				s.setRunDecision(run, decision)
			}
		}
		if err := NoteWorkerQuestion(workflow, execution, fact.QuestionRoute.TurnID); err != nil {
			return err
		}
		s.refreshRun(run)
		s.note(run, "steer", "decision", "pending", "Execution is waiting for an answer", routeID, decisionMeta(run))
	}
	if reply := fact.ToolReply; reply != nil && run.Decision != nil && run.Decision.Request.QuestionRoute != nil {
		route := run.Decision.Request.QuestionRoute
		if reply.CallID == route.QuestionID && (reply.TurnID == "" || reply.TurnID == route.TurnID) && (reply.ThreadID == "" || reply.ThreadID == execution.SessionID) {
			if run.Decision.Status == "answer_saved" && reply.Status == "completed" && reply.Success != nil && *reply.Success && reply.MatchesAnswers(run.Decision.Answers) {
				confirmed := false
				for _, op := range s.domain.Operations {
					if op.IdempotencyKey == "worker-question-answer:"+execution.ExecutionID+":"+run.Decision.ID && (op.Status == "in_flight" || op.Status == "accepted" || op.Status == "outcome_unknown" || op.Status == "confirmed") {
						if op.Status != "confirmed" {
							if err := s.setOperationState(op.OperationID, "confirmed", route.QuestionID, mustJSON(map[string]any{"receipt_id": reply.CallID, "status": reply.Status, "success": true}), ""); err != nil {
								return err
							}
							operationIDs = append(operationIDs, op.OperationID)
						}
						confirmed = true
						break
					}
				}
				if !confirmed {
					return errors.New("execution reply completed without the original saved answer intent")
				}
				s.setDecisionStatus(run, "reply_accepted")
				if err := s.commitWorker(run, operationIDs...); err != nil {
					return err
				}
				operationIDs = nil
				workflow, execution = s.workflowRecords(run)
				if len(execution.PendingQuestions) > 0 {
					next := execution.PendingQuestions[0]
					execution.PendingQuestions = execution.PendingQuestions[1:]
					queued := executionapi.EventFact{Questions: cloneExecutionQuestions(next.Questions), QuestionRoute: ptr(next.Route.Clone())}
					decision, err := WriteDecisionRequest(run, decisionRequestFromEventFact(queued))
					if err != nil {
						return err
					}
					s.setRunDecision(run, decision)
				} else {
					if err := ContinueWorkerQuestion(workflow, execution, route.TurnID); err != nil {
						return err
					}
					s.refreshRun(run)
				}
			} else if reply.Status == "failed" || reply.Success != nil && !*reply.Success {
				execution.Error = ptr("The original answer was rejected by the execution")
				s.refreshRun(run)
			}
		}
	}
	if ev.Summary != "" {
		execution.LastEvent = ptr(ev.Summary)
		s.refreshRun(run)
	}
	if ev.Activity.Kind != "" {
		if err := s.recordConversationEvent(run, ev); err != nil {
			return err
		}
		s.recordActivity(run, ev.Activity)
		s.emitConsole(ev.Activity)
	} else if ev.Persisted != nil {
		return errors.New("worker source event lacks activity classification")
	}
	return s.commitWorker(run, operationIDs...)
}

func (s *Service) routedQuestionSeen(run *Run, execution *ExecutionRecord, route *executionapi.QuestionRoute, questions []executionapi.Question) (bool, error) {
	if run.Decision != nil && run.Decision.Request.QuestionRoute != nil && run.Decision.Request.QuestionRoute.QuestionID == route.QuestionID {
		if err := sameRoutedQuestion(route, questions, run.Decision.Request.QuestionRoute, run.Decision.Request.Questions); err != nil {
			return true, err
		}
		return true, nil
	}
	for _, pending := range execution.PendingQuestions {
		if pending.Route.QuestionID != route.QuestionID {
			continue
		}
		if err := sameRoutedQuestion(route, questions, &pending.Route, pending.Questions); err != nil {
			return true, err
		}
		return true, nil
	}
	for _, record := range s.domain.Decisions {
		if record.ExecutionID != execution.ExecutionID || record.QuestionID != route.QuestionID {
			continue
		}
		var request DecisionRequest
		if err := decode(record.Request, &request); err != nil {
			return false, err
		}
		if request.QuestionRoute == nil {
			return false, errors.New("reused question identity conflicts with a saved ordinary decision")
		}
		if err := sameRoutedQuestion(route, questions, request.QuestionRoute, request.Questions); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func sameRoutedQuestion(route *executionapi.QuestionRoute, questions []executionapi.Question, oldRoute *executionapi.QuestionRoute, oldQuestions []executionapi.Question) error {
	if route == nil || oldRoute == nil || route.Digest != oldRoute.Digest || digestValue(questions) != digestValue(oldQuestions) {
		return errors.New("conflicting duplicate question route or content")
	}
	return nil
}

func validateQuestionRouteForRun(run *Run, execution *ExecutionRecord, route *executionapi.QuestionRoute, questions []executionapi.Question, codecKey string, codecVersion int) error {
	if run == nil || execution == nil || route == nil {
		return errors.New("question route has no fixed execution")
	}
	want := executionapi.QuestionRouteIdentity{ProjectID: execution.ProjectIdentity, WorkflowRunID: run.RunID,
		ExecutionID: execution.ExecutionID, AttemptID: run.AttemptID, Generation: execution.AppServerGeneration,
		TurnID: route.TurnID, QuestionID: route.QuestionID}
	if route.TurnID != execution.CurrentTurnID && route.TurnID != execution.PendingTurnID {
		return ErrWorkerTurnStale
	}
	return validateQuestionRoute(route, questions, want, codecKey, codecVersion)
}

func validateQuestionRoute(route *executionapi.QuestionRoute, questions []executionapi.Question, want executionapi.QuestionRouteIdentity, codecKey string, codecVersion int) error {
	if route == nil {
		return errors.New("question route is missing")
	}
	if err := route.ValidateIdentity(want); err != nil {
		return err
	}
	if route.ImplementationKey != codecKey || route.CodecVersion != codecVersion {
		return errors.New("question route codec differs from the fixed execution implementation")
	}
	for _, question := range questions {
		if question.ExecutionID != want.ExecutionID || question.AttemptID != want.AttemptID || question.Generation != want.Generation || question.TurnID != want.TurnID {
			return errors.New("question differs from its fixed execution route")
		}
	}
	return nil
}
