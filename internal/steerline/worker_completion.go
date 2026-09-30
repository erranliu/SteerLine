package steerline

import (
	"context"
	"errors"
	"strings"
)

func (s *Service) finishWorkerFlowFor(run *Run, workerErr error) error {
	fixed, contextErr := s.workerFlowContext(run)
	if contextErr != nil {
		return contextErr
	}
	c := fixed.config
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || workflow.WorkflowVersion != WorkerFlowVersion {
		return errors.New("finished worker lacks its fixed execution")
	}
	if task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID); task != nil && (task.Status == "closed" || task.Status == "discarded") {
		return nil // A late process exit cannot reopen a terminal Task.
	}
	mode, err := s.workerFlowMode(run)
	if err != nil {
		return err
	}
	evidence, readErr := readWorkerTurnEvidence(fixed, mode)
	if readErr != nil {
		execution.Status = "blocked"
		s.refreshRun(run)
		return s.waitWorkerFlow(run, readErr.Error())
	}
	turnID := evidence.TurnID
	if execution.CurrentTurnID == "" {
		if err := BeginWorkerTurn(workflow, execution, evidence.SessionID, turnID); err != nil {
			return s.waitWorkerFlow(run, "Original turn start could not be reconciled: "+err.Error())
		}
		s.refreshRun(run)
		for _, operation := range s.domain.Operations {
			if operation.IdempotencyKey == "worker-turn:"+run.ExecutionID+":"+evidence.CommandID && operation.Status != "confirmed" {
				requestID := operation.PlatformRequestID
				if requestID == "" {
					requestID = turnID
				}
				if err := s.setOperationState(operation.OperationID, "confirmed", requestID, mustJSON(map[string]string{"session_id": evidence.SessionID, "turn_id": turnID}), ""); err != nil {
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
		return s.waitWorkerFlow(run, "The original worker process exited without a confirmed completed turn; its output and session are retained")
	}
	if evidence.LocalCodex {
		execution.PendingAsks = nil // Requests from a terminal turn cannot enter a later turn.
	}
	if evidence.LocalCodex && run.Decision != nil && run.Decision.Request.NativeAsk != nil {
		if run.Decision.Status == "pending" {
			s.setDecisionStatus(run, "superseded")
			if err := s.commitWorker(run); err != nil {
				return err
			}
			workflow, execution = s.workflowRecords(run)
			s.setRunDecision(run, nil)
		} else if run.Decision.Status == "answer_saved" {
			if err := StopWorkerTurn(workflow, execution, turnID, true); err != nil {
				return err
			}
			execution.TurnEndedAt = ptr(now())
			execution.Status = "blocked"
			s.refreshRun(run)
			execution.Error = ptr("Native turn ended before the original answer delivery was confirmed; the saved answer and request are retained")
			s.refreshRun(run)
			return s.commitWorker(run)
		}
	}
	if run.Decision != nil && run.Decision.Status == "answer_saved" {
		answer, answerErr := ReadDecisionAnswer(run)
		if answerErr == nil {
			command, commandErr := s.domainStore.CommandByIdempotency(context.Background(), c.identity, decisionAnswerKey(answer))
			if commandErr == nil && command.Kind == "answer_decision" && (command.CommandID == turnID || evidence.LocalCodex && command.CommandID == execution.CurrentCommandID) {
				s.setDecisionStatus(run, "reply_accepted")
			}
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
		info, err := s.prs.Query(context.Background(), candidate)
		if err == nil {
			err = BindWorkerPR(workflow, execution, info, candidate, c.PR.Repository, false)
		}
		if err != nil {
			issue = strings.TrimSpace(issue + "; PR link is not verified: " + err.Error())
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
	if err := s.stopVerifiedWorkerTurn(run, workflow, execution, turnID, turnOutcome, question); err != nil {
		return err
	}
	if evidence.PublishRequested {
		runID := run.RunID
		_, publishErr := s.publishLocalWorkerRun(context.Background(), run, workflow, execution, turnID)
		run = s.localWorkerRun(runID)
		workflow, execution = s.workflowRecords(run)
		if run == nil || workflow == nil || execution == nil {
			return errors.New("fixed worker execution disappeared during host publication")
		}
		if publishErr != nil {
			issue = strings.TrimSpace(issue + "; host publication failed: " + publishErr.Error())
		}
	}
	return s.resolveStoppedWorkerTurn(run, workflow, execution, stoppedWorkerResolution{TurnID: turnID, LastMessage: evidence.LastMessage, Issue: issue, Question: question, RecordPRStop: true})
}
