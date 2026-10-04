package execution

import (
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
)

import (
	"context"
	"errors"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (s *Service) finishWorkerFlowFor(run *contracts.Run, workerErr error) error {
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || workflow.WorkflowVersion != executionmodel.Version {
		return errors.New("finished worker lacks its fixed execution")
	}
	if s.candidateBusinessClosed(run) {
		return s.finishRetiredCandidate(run, lifecycle, fixed)
	}
	snapshot, err := s.lifecycleSnapshot(run, workflow, execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), min(positiveDuration(fixed.config.CommandTimeout, 30*time.Second), 30*time.Second))
	defer cancel()
	receipt, observer, observeErr := lifecycle.ObserveLifecycle(ctx, snapshot)
	if observer != nil {
		worker := workerFromLifecycle(run.RunID, run.AttemptID, observer)
		if worker == nil {
			return errors.New("lifecycle observation returned no fixed stop handle")
		}
		if err := mergeLifecycleExecutionReceipt(execution, receipt); err != nil {
			_ = worker.detachLocalObservation(context.Background())
			return err
		}
		s.refreshRun(run)
		if err := s.commitWorker(run); err != nil {
			_ = worker.detachLocalObservation(context.Background())
			return err
		}
		s.registerWorker(run, worker)
		return nil
	}
	if observeErr != nil || receipt.TurnEvidence == nil {
		execution.Status = "blocked"
		s.refreshRun(run)
		reason := "the original execution returned no confirmed turn result"
		if observeErr != nil {
			reason = observeErr.Error()
		} else if receipt.Diagnostic != "" {
			reason = receipt.Diagnostic
		}
		return s.waitWorkerFlow(run, reason)
	}
	if err := mergeLifecycleExecutionReceipt(execution, receipt); err != nil {
		return s.waitWorkerFlow(run, "the original execution binding could not be reconciled: "+err.Error())
	}
	if execution.Environment != nil && execution.Environment.Preparation != nil && receipt.State.TurnID == execution.Environment.Preparation.TurnID && receipt.State.TurnCompleted {
		turn, turnErr := s.currentHostTurn(run)
		if turnErr != nil {
			return turnErr
		}
		if turn == nil || turn.TurnID != execution.Environment.Preparation.TurnID {
			return s.waitWorkerFlow(run, "the original preparation Dispatch receipt is unavailable")
		}
		if err := s.readHostOutput(context.Background(), run); err != nil {
			if s.store.Err() != nil {
				return s.store.Err()
			}
			return s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "preparation_output_unverified", Confidence: "unverified", Source: "host.worker-output", Detail: clip(err.Error(), 1000), Turn: turn.TurnID})
		}
		return s.finishWorkerPreparation(context.Background(), run, execution, turn, turn.Outcome)
	}
	evidence := workerTurnEvidence{TurnEvidence: *receipt.TurnEvidence}
	if !evidence.Started || evidence.SessionID == "" || evidence.TurnID == "" || !evidence.Completed {
		execution.Status = "blocked"
		s.refreshRun(run)
		return s.waitWorkerFlow(run, "the original execution did not provide a complete turn receipt")
	}
	if execution.SessionID != evidence.SessionID || execution.CurrentTurnID != evidence.TurnID && execution.PendingTurnID != evidence.TurnID ||
		execution.CurrentCommandID != evidence.CommandID && execution.PendingCommandID != evidence.CommandID {
		return s.waitWorkerFlow(run, "the original turn receipt differs from its fixed execution input")
	}
	if execution.CurrentTurnID == "" {
		if err := BeginWorkerTurn(workflow, execution, evidence.SessionID, evidence.TurnID); err != nil {
			return s.waitWorkerFlow(run, "original turn start could not be reconciled: "+err.Error())
		}
		s.refreshRun(run)
		for _, operation := range s.domain.Operations {
			if operation.IdempotencyKey == "worker-turn:"+run.ExecutionID+":"+evidence.CommandID && operation.Status != "confirmed" {
				requestID := operation.PlatformRequestID
				if requestID == "" {
					requestID = evidence.TurnID
				}
				if err := s.setOperationState(operation.OperationID, "confirmed", requestID, mustJSON(map[string]string{"session_id": evidence.SessionID, "turn_id": evidence.TurnID}), ""); err != nil {
					return err
				}
				if err := s.commitWorker(run, operation.OperationID); err != nil {
					return err
				}
				break
			}
		}
	}
	if !evidence.Completed {
		execution.Status = "blocked"
		s.refreshRun(run)
		return s.waitWorkerFlow(run, "the original worker exited without a confirmed completed turn; its output and session are retained")
	}
	if len(execution.PendingQuestions) > 0 {
		execution.PendingQuestions = nil
	}
	if run.Decision != nil && run.Decision.Request.QuestionRoute != nil {
		if run.Decision.Status == "pending" {
			s.setDecisionStatus(run, "superseded")
			if err := s.commitWorker(run); err != nil {
				return err
			}
			s.setRunDecision(run, nil)
		} else if run.Decision.Status == "answer_saved" {
			if err := StopWorkerTurn(workflow, execution, evidence.TurnID, true); err != nil {
				return err
			}
			execution.TurnEndedAt = ptr(now())
			execution.Status = "blocked"
			execution.Error = ptr("Original turn ended before the saved answer was confirmed; the answer and question remain available for reconciliation")
			s.refreshRun(run)
			return s.commitWorker(run)
		}
	}
	issue := ""
	if workerErr != nil {
		issue = workerErr.Error()
	}
	if evidence.Failure != "" {
		issue = strings.TrimSpace(issue + "; " + evidence.Failure)
	}
	if len(evidence.PRCandidates) == 1 {
		candidate := evidence.PRCandidates[0]
		workflow.PRCandidateURL = candidate
		info, queryErr := publicationClient(fixed.config).Query(context.Background(), candidate)
		if queryErr == nil {
			queryErr = BindWorkerPR(workflow, execution, info, candidate, fixed.config.PR.Repository, false)
		}
		if queryErr != nil {
			issue = strings.TrimSpace(issue + "; PR link is not verified: " + queryErr.Error())
		}
	} else if len(evidence.PRCandidates) > 1 {
		issue = strings.TrimSpace(issue + "; several PR links were reported: " + strings.Join(evidence.PRCandidates, ", "))
	}
	question := len(evidence.PRCandidates) > 1
	if run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.Question != "The worker stopped. What should it do next?" {
		question = true
	}
	turnOutcome := "failed"
	if evidence.Succeeded {
		turnOutcome = "succeeded"
	}
	if err := s.stopVerifiedWorkerTurn(run, workflow, execution, evidence.TurnID, turnOutcome, question); err != nil {
		return err
	}

	return s.resolveStoppedWorkerTurn(run, workflow, execution, stoppedWorkerResolution{TurnID: evidence.TurnID, LastMessage: evidence.LastMessage, Issue: issue, Question: question, RecordPRStop: true})
}
