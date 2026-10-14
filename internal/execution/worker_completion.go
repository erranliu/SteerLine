package execution

import (
	"context"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/execution/reliable"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (s *Service) finishWorkerFlowFor(run *reconstructiontypes.RunView, workerErr error) error {
	if s.candidateBusinessClosed(run) || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nil
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || workflow.WorkflowVersion != executionmodel.Version {
		return errors.New("finished worker lacks its fixed execution")
	}
	snapshot, err := s.lifecycleSnapshot(run, workflow, execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), reliable.ObservationTimeout(duration(fixed.config.CommandTimeout)))
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
	// A result receipt proves its original turn, not current session eligibility.
	// Preserve the shared qualification until its next probe; an omitted native
	// availability flag must not invalidate consent with a synthetic false fact.
	if execution.PendingCommandID == evidence.CommandID {
		opID, err := s.recordWorkerTurnStarted(run, workflow, execution, evidence.TurnID, evidence.SessionID)
		if err != nil {
			return s.waitWorkerFlow(run, "original turn start could not be reconciled: "+err.Error())
		}
		if err := s.commitWorker(run, opID); err != nil {
			return err
		}
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
	if record := s.decisionRecord(run); execution.RecoveryRequired && record != nil && record.Status == "pending" && record.RecoveryState == "unverified" &&
		run.Decision.Request.QuestionRoute == nil && len(run.Decision.Request.Options) == 0 && !s.hasUnknownWorkerOperation(run) {
		// This was an observation gap, not a business question. Its trusted
		// original completion must return to the normal PR/feedback resolution.
		s.setDecisionStatus(run, "superseded")
		if err := s.commitWorker(run); err != nil {
			return err
		}
		s.setRunDecision(run, nil)
	}
	pending := execution.PendingQuestions[:0]
	for _, q := range execution.PendingQuestions {
		if q.InteractionKind == "async" {
			pending = append(pending, q)
		}
	}
	execution.PendingQuestions = pending
	if run.Decision != nil && run.Decision.Request.QuestionRoute != nil && run.Decision.Request.InteractionKind != "async" {
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
	// An observer transport failure cannot change a subsequently verified native
	// result. Missing native evidence was handled above; only the adapter owns outcome.
	return s.acceptStageTurn(ctx, run, lifecycle, fixed, evidence.TurnEvidence)
}
