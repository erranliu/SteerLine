package execution

import (
	"context"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (s *Service) recordWorkerTurnStarted(run *reconstructiontypes.RunView, workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord, turnID string, sessionFacts ...string) (string, error) {
	commandID := execution.PendingCommandID
	fresh := commandID != ""
	if !fresh && execution.CurrentTurnID == turnID && execution.LastStoppedTurnID != turnID {
		commandID = execution.CurrentCommandID
	}
	if !executionbackend.StableExecutionID(commandID) || turnID == "" || fresh && execution.PendingTurnID != turnID {
		return "", errors.New("turn start lacks its fixed input command or verified receipt")
	}
	var operation *executionmodel.OperationRecord
	for i := range s.domain.Operations {
		if s.domain.Operations[i].IdempotencyKey == "worker-turn:"+run.ExecutionID+":"+commandID {
			operation = &s.domain.Operations[i]
			break
		}
	}
	if operation == nil {
		return "", errors.New("turn start has no saved input operation")
	}
	sessionID, _ := executionSessionFacts(execution)
	if len(sessionFacts) > 0 && sessionFacts[0] != "" {
		sessionID = sessionFacts[0]
	}
	observedWorkflow := *workflow
	if !executionmodel.CandidateParticipates(workflow.Status) {
		observedWorkflow.Status = "queued"
	}
	if err := BeginWorkerTurn(&observedWorkflow, execution, sessionID, turnID); err != nil {
		return "", err
	}
	if executionmodel.CandidateParticipates(workflow.Status) {
		*workflow = observedWorkflow
	}
	if executionmodel.CandidateParticipates(workflow.Status) && execution.Environment != nil && execution.Environment.Blocker != nil {
		workflow.Status = "waiting_input"
	}
	execution.CurrentCommandID, execution.PendingCommandID = commandID, ""
	if fresh && executionmodel.CandidateParticipates(workflow.Status) {
		if attempt := s.workflowAttempt(run, run.AttemptID); attempt != nil {
			attempt.Status, attempt.UpdatedAt = "running", now()
		}
		if run.Decision != nil && run.Decision.Status == "answer_saved" && (run.Decision.Request.QuestionRoute == nil || run.Decision.Request.InteractionKind == "async" && stageInput(workflow, commandID) != nil && stageInput(workflow, commandID).FeedbackDecisionID == run.Decision.ID) {
			async := run.Decision.Request.InteractionKind == "async"
			s.setDecisionStatus(run, "reply_accepted")
			if async && len(execution.PendingQuestions) > 0 {
				next := execution.PendingQuestions[0]
				execution.PendingQuestions = execution.PendingQuestions[1:]
				decision, err := WriteDecisionRequest(run, DecisionRequestFromEventFact(executionbackend.EventFact{InteractionKind: next.InteractionKind, Questions: next.Questions, QuestionRoute: &next.Route}))
				if err != nil {
					return "", err
				}
				s.setRunDecision(run, decision)
			}
		}

		execution.Status, execution.Error = "active", nil
		s.refreshRun(run)
		execution.TurnStartedAt, execution.TurnEndedAt = ptr(now()), nil
		s.refreshRun(run)
	}
	if operation.Status == "confirmed" {
		return operation.OperationID, nil
	}
	// A started event confirms the original input; it does not replace the
	// accepted native receipt or its request identity with a synthetic summary.
	receipt, requestID := operation.Receipt, operation.PlatformRequestID
	if len(receipt) == 0 {
		receipt = mustJSON(map[string]string{"session_id": sessionID, "turn_id": turnID})
	}
	if requestID == "" {
		requestID = turnID
	}
	return operation.OperationID, TransitionOperation(operation, "confirmed", requestID, receipt, "")
}

func (s *Service) eventWorkerFlowFor(run *reconstructiontypes.RunView, ev WorkerEvent) (resultErr error) {
	defer func() { ackWorkerEventPersistence(ev, resultErr) }()
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || workflow.WorkflowVersion != executionmodel.Version {
		return errors.New("worker event has no fixed worker-flow execution")
	}
	if ev.RunID != "" && ev.RunID != run.RunID || ev.AttemptID != "" && ev.AttemptID != run.AttemptID {
		return executionmodel.ErrTurnStale
	}
	participates := !s.candidateBusinessClosed(run)
	frozen := workflow.Status == "completed"
	advance := participates && run.TaskStatus != "closed" && run.TaskStatus != "discarded"
	mode, err := s.workerFlowRegistration(run)
	if err != nil {
		return err
	}
	fact := ev.Fact
	if ev.SourceGap || ev.OutputGap {
		fact.Kind, fact.Gap = executionbackend.EventGap, true
	}
	if fact.Kind == executionbackend.EventInputAccepted {
		fact.InputAccepted = true
	}
	if fact.Kind == executionbackend.EventGap {
		fact.Gap = true
	}
	var operationIDs []string
	if ev.Diagnostic != "" && fact.Kind == "" && len(fact.Questions) == 0 && fact.ToolReply == nil {
		return errors.New(ev.Diagnostic)
	}
	if fact.SessionID != "" {
		sessionID, _ := executionSessionFacts(execution)
		if sessionID != "" && fact.SessionID != sessionID || sessionID == "" && execution.Process != nil {
			return executionmodel.ErrTurnStale
		}
	}
	if frozen && fact.StartedTurnID != "" && fact.StartedTurnID != execution.CurrentTurnID {
		return executionmodel.ErrTurnStale
	}
	if !frozen && fact.StartedTurnID != "" && fact.Kind == executionbackend.EventStarted {
		opID, err := s.recordWorkerTurnStarted(run, workflow, execution, fact.StartedTurnID, fact.SessionID)
		if err != nil {
			return err
		}
		if opID != "" {
			operationIDs = append(operationIDs, opID)
		}
		workflow, execution = s.workflowRecords(run)
	}
	if !advance && (fact.QuestionRoute != nil || len(fact.Questions) != 0) {
		if fact.QuestionRoute == nil || len(fact.Questions) == 0 {
			return errors.New("worker question facts lack their fixed route")
		}
		if err := validateQuestionRouteForRun(run, execution, fact.QuestionRoute, fact.Questions, mode.QuestionCodecKey, mode.QuestionCodecVersion); err != nil {
			return err
		}
	}
	if advance && (fact.QuestionRoute != nil || len(fact.Questions) != 0) {
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
				execution.PendingQuestions = append(execution.PendingQuestions, executionmodel.PendingQuestion{InteractionKind: fact.InteractionKind, Questions: cloneExecutionQuestions(fact.Questions), Route: fact.QuestionRoute.Clone()})
			} else {
				decision, err := WriteDecisionRequest(run, DecisionRequestFromEventFact(fact))
				if err != nil {
					return err
				}
				s.setRunDecision(run, decision)
			}
		}
		if fact.InteractionKind != "async" {
			if err := NoteWorkerQuestion(workflow, execution, fact.QuestionRoute.TurnID); err != nil {
				return err
			}
		}
		s.refreshRun(run)
		title := "Execution is waiting for an answer"
		if fact.InteractionKind == "async" {
			title = "Execution has an asynchronous question; work may continue"
		}
		s.note(run, "steer", "decision", "pending", title, routeID, decisionMeta(run))
	}
	if advance && fact.Kind == executionbackend.EventEscalation && fact.Matched && fact.SourceEventID != "" && execution.CurrentTurnID != "" && execution.CurrentTurnID != execution.LastStoppedTurnID &&
		(run.Decision == nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved") &&
		(execution.Environment == nil || execution.Environment.Blocker == nil && (execution.Environment.Preparation == nil || execution.Environment.Preparation.TurnID != execution.CurrentTurnID)) {
		request := DecisionRequestFromEventFact(fact)
		if request.Question == "" {
			return errors.New("matched guidance event lacks its question")
		}
		request.Options = []executionbackend.DecisionOption{{ID: "continue", Label: "Continue current work"}}
		request.AllowFreeText = true
		decision, err := WriteDecisionRequest(run, request)
		if err != nil {
			return err
		}
		s.setRunDecision(run, decision)
		if err := NoteWorkerQuestion(workflow, execution, execution.CurrentTurnID); err != nil {
			return err
		}
		s.refreshRun(run)
	}
	if reply := fact.ToolReply; advance && reply != nil && run.Decision != nil && run.Decision.Request.QuestionRoute != nil {
		route := run.Decision.Request.QuestionRoute
		sessionID, _ := executionSessionFacts(execution)
		if reply.CallID == route.QuestionID && (reply.TurnID == "" || reply.TurnID == route.TurnID) && (reply.ThreadID == "" || reply.ThreadID == sessionID) {
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
				if execution.RecoveryRequired {
					// A late exact reply can resolve the original send uncertainty
					// while observation remains attached. Revalidate through the
					// worker so other unknown actions still retain their gate.
					lifecycle, fixed, err := s.lifecycleForRun(run)
					if err != nil {
						return err
					}
					snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, execution, fixed.attempt, nil)
					if err != nil {
						return err
					}
					probeCtx, probeCancel := context.WithTimeout(context.Background(), min(positiveDuration(fixed.config.CommandTimeout, 30*time.Second), 10*time.Second))
					evidence, err := lifecycle.VerifyManualRecovery(probeCtx, snapshot)
					probeCancel()
					if err == nil {
						s.setWorkerRecoveryEvidence(run, evidence)
					}
				}
				if err := s.commitWorker(run, operationIDs...); err != nil {
					return err
				}
				operationIDs = nil
				workflow, execution = s.workflowRecords(run)
				if len(execution.PendingQuestions) > 0 {
					next := execution.PendingQuestions[0]
					execution.PendingQuestions = execution.PendingQuestions[1:]
					queued := executionbackend.EventFact{InteractionKind: next.InteractionKind, Questions: cloneExecutionQuestions(next.Questions), QuestionRoute: ptr(next.Route.Clone())}
					decision, err := WriteDecisionRequest(run, DecisionRequestFromEventFact(queued))
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
	if !frozen && ev.Summary != "" {
		execution.LastEvent = ptr(ev.Summary)
		s.refreshRun(run)
	}
	for _, envelope := range ev.Implementation {
		if err := envelope.ValidateResourceIdentity(execution.ProjectIdentity, execution.ExecutionID, execution.CurrentAttemptID, execution.Generation, execution.ConfigurationDigest); err != nil {
			return err
		}
		if frozen {
			continue
		}
		if err := putImplementationEnvelope(execution, envelope); err != nil {
			return err
		}
	}
	if !frozen && ev.InputWait && ev.Diagnostic != "" {
		execution.Error = ptr(clip("Original worker observation is unverified: "+ev.Diagnostic, 2000))
		s.refreshRun(run)
	}
	if !frozen && ev.DeliveryFinal && ev.DeliveryID != "" {
		if execution.PendingDeliveryID != "" && execution.PendingDeliveryID != ev.DeliveryID {
			return executionmodel.ErrTurnStale
		}
		execution.PendingDeliveryID = ev.DeliveryID
		s.refreshRun(run)
	}
	var completionCandidate *executionbackend.LifecycleCandidate
	// An ended turn may still lack a verified ordinary final. Preserve the
	// stopped fact without a stage result or another input; observation continues.
	if !frozen && fact.Kind == executionbackend.EventStatus && fact.Completed && fact.Matched && fact.StartedTurnID != "" {
		if fact.StartedTurnID != execution.CurrentTurnID {
			return executionmodel.ErrTurnStale
		}
		if execution.LastStoppedTurnID != execution.CurrentTurnID {
			original := *workflow
			observed := *workflow
			if !participates {
				observed.Status = "running"
			}
			if err := s.stopVerifiedWorkerTurn(run, &observed, execution, fact.StartedTurnID, "unknown", false); err != nil {
				return err
			}
			if participates {
				workflow.Status, workflow.StageDiagnostic = "reconciling", "Native turn ended; original final reply is not yet verified"
			} else {
				*workflow = original
			}
			s.refreshRun(run)
		}
	}
	if ev.Candidate != nil {
		candidate := ev.Candidate.Clone()
		if err := candidate.Validate(); err != nil || candidate.Action != executionbackend.LifecycleReportCompletion || candidate.Input.Completion == nil {
			return errors.Join(err, errors.New("worker observer proposed an unsupported follow-up action"))
		}
		if candidate.Input.Completion.TurnID != execution.PendingTurnID && candidate.Input.Completion.TurnID != execution.CurrentTurnID {
			return executionmodel.ErrTurnStale
		}
		completionCandidate = &candidate
		if !participates && !frozen {
			execution.Summary = ptr(candidate.Input.Completion.Body)
			execution.LastTurnOutcome = candidate.Input.Completion.Outcome
			if execution.LastStoppedTurnID != candidate.Input.Completion.TurnID || execution.TurnEndedAt == nil {
				execution.TurnEndedAt = ptr(now())
			}
			execution.Status, execution.LastStoppedTurnID = "idle", candidate.Input.Completion.TurnID
		}
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
	if err := s.commitWorker(run, operationIDs...); err != nil {
		return err
	}
	if !advance {
		return nil
	}
	if completionCandidate != nil {
		lifecycle, _, err := s.lifecycleForRun(run)
		if err != nil {
			return err
		}
		if err := s.executeLifecycleStep(context.Background(), run, lifecycle, *completionCandidate); err != nil {
			if errors.Is(err, errLifecycleEffectPending) || errors.Is(err, errLifecycleNeedsReconciliation) {
				return nil
			}
			return err
		}
	}
	if !frozen && ev.DeliveryFinal && ev.DeliveryID != "" {
		return s.acknowledgeLifecycleDelivery(run, ev.DeliveryID)
	}
	return nil
}

func (s *Service) routedQuestionSeen(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord, route *executionbackend.ImplementationDataEnvelope, questions []executionbackend.Question) (bool, error) {
	if run.Decision != nil && sameQuestionOrigin(run.Decision.Request.QuestionRoute, route) {
		if err := sameRoutedQuestion(route, questions, run.Decision.Request.QuestionRoute, run.Decision.Request.Questions); err != nil {
			return true, err
		}
		return true, nil
	}
	for _, pending := range execution.PendingQuestions {
		if !sameQuestionOrigin(&pending.Route, route) {
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
		var request executionmodel.DecisionRequest
		if err := decode(record.Request, &request); err != nil {
			return false, err
		}
		if !sameQuestionOrigin(request.QuestionRoute, route) {
			continue
		}
		if err := sameRoutedQuestion(route, questions, request.QuestionRoute, request.Questions); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func sameQuestionOrigin(a, b *executionbackend.ImplementationDataEnvelope) bool {
	return a != nil && b != nil && a.ProjectID == b.ProjectID && a.WorkflowRunID == b.WorkflowRunID &&
		a.ExecutionID == b.ExecutionID && a.AttemptID == b.AttemptID && a.Generation == b.Generation &&
		a.TurnID == b.TurnID && a.QuestionID == b.QuestionID
}

func sameRoutedQuestion(route *executionbackend.ImplementationDataEnvelope, questions []executionbackend.Question, oldRoute *executionbackend.ImplementationDataEnvelope, oldQuestions []executionbackend.Question) error {
	if route == nil || oldRoute == nil || route.Digest != oldRoute.Digest || digestValue(questions) != digestValue(oldQuestions) {
		return errors.New("conflicting duplicate question route or content")
	}
	return nil
}

func validateQuestionRouteForRun(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord, route *executionbackend.ImplementationDataEnvelope, questions []executionbackend.Question, codecKey string, codecVersion int) error {
	if run == nil || execution == nil || route == nil {
		return errors.New("question route has no fixed execution")
	}
	_, generation := executionSessionFacts(execution)
	want := executionbackend.QuestionRouteIdentity{ProjectID: execution.ProjectIdentity, WorkflowRunID: run.RunID,
		ExecutionID: execution.ExecutionID, AttemptID: run.AttemptID, Generation: generation,
		TurnID: route.TurnID, QuestionID: route.QuestionID}
	if route.TurnID != execution.CurrentTurnID && route.TurnID != execution.PendingTurnID {
		return executionmodel.ErrTurnStale
	}
	return validateQuestionRoute(route, questions, want, codecKey, codecVersion)
}

func validateQuestionRoute(route *executionbackend.ImplementationDataEnvelope, questions []executionbackend.Question, want executionbackend.QuestionRouteIdentity, codecKey string, codecVersion int) error {
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
