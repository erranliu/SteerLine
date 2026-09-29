package steerline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) resumeConfiguredWork(ctx context.Context) error {
	return s.resumeWorkerFlow(ctx)
}

func (s *Service) resumeWorkerFlow(ctx context.Context) error {
	for _, run := range s.State.Runs {
		if run == nil || run.WorkflowVersion != WorkerFlowVersion || s.workers[run.RunID] != nil {
			continue
		}
		task := nativeTaskRecord(s.State.Tasks, run.DomainTaskID)
		workflow, execution := s.workflowRecords(run)
		if task == nil || workflow == nil || execution == nil || task.Status != "in_progress" || workflow.Phase == "completed" {
			continue
		}
		if err := s.useFixedRunProfile(run); err != nil {
			return err
		}
		reconciled, err := s.reconcileLocalWorkerPublication(ctx, run)
		if err != nil {
			return s.waitWorkerFlow(run, "Host publication needs reconciliation: "+err.Error())
		}
		if reconciled {
			continue
		}
		if execution.Process != nil && execution.Process.Kind == "orca" && (execution.PendingTurnID != "" || execution.CurrentTurnID != execution.LastStoppedTurnID) {
			turnID := execution.PendingTurnID
			if turnID == "" {
				turnID = execution.CurrentTurnID
			}
			events := filepath.Join(orcaTurnPath(s.Config, run.RunID, turnID), "events.jsonl")
			if exited, err := readOrcaTurnExit(events, turnID); err != nil {
				return s.waitWorkerFlow(run, "Original Orca turn exit evidence is invalid: "+err.Error())
			} else if exited != nil {
				s.active, s.worker = run, nil
				finishErr := s.finishWorkerFlow(nil)
				s.parkWorker()
				return finishErr
			}
			term, err := showWorkerOrcaTerminal(ctx, s.Config, execution.Process.Terminal)
			if err != nil {
				return s.waitWorkerFlow(run, "Original Orca terminal is unavailable: "+err.Error())
			}
			if err := verifyWorkerOrcaTerminal(term, run, execution.Process); err != nil {
				return s.waitWorkerFlow(run, err.Error())
			}
			worker, err := resumeWorkerOrcaTurn(s.Config, run, turnID)
			if err != nil {
				return s.waitWorkerFlow(run, "Original Orca turn journal cannot be reattached: "+err.Error())
			}
			s.registerWorker(run, worker)
			return nil
		}
		if execution.Process != nil && (execution.PendingTurnID != "" || execution.CurrentTurnID != execution.LastStoppedTurnID) {
			switch inspectProcess(execution.Process) {
			case "matching process still exists":
				// The center has no right to restart a live original process.
				return nil
			case "process absent":
				s.active, s.worker = run, nil
				err := s.finishWorkerFlow(nil)
				s.parkWorker()
				return err
			default:
				execution.Status = "blocked"
				return s.waitWorkerFlow(run, "Original worker process identity cannot be verified; its session and files are retained")
			}
		}
		if execution.PendingTurnID != "" {
			// A saved start may have succeeded without its process receipt.
			// A complete journal can reconcile it without another send.
			events := filepath.Join(s.Config.RunDir, "runs", run.RunID, "turns", execution.PendingTurnID, "events.jsonl")
			if outcome, err := readWorkerTurnOutcome(events); err == nil && outcome.Completed && outcome.SessionID != "" {
				s.active, s.worker = run, nil
				finishErr := s.finishWorkerFlow(nil)
				s.parkWorker()
				return finishErr
			}
			if workflow.Phase != "waiting_decision" {
				return s.waitWorkerFlow(run, "Worker turn start has no confirmed process receipt; original request remains unverified")
			}
			return nil
		}
		if workflow.Phase != "queued" {
			continue
		}
		var preparation *OperationRecord
		for i := range s.State.Operations {
			if s.State.Operations[i].IdempotencyKey == "worker-workspace:"+execution.ExecutionID {
				preparation = &s.State.Operations[i]
				break
			}
		}
		if preparation == nil {
			return s.prepareWorkerFlow(ctx, *task, run)
		}
		if run.WorkspaceIntent == nil {
			return s.waitWorkerFlow(run, "Saved workspace preparation has no fixed worktree identity")
		}
		if s.Config.hostName() == "orca_supervised" {
			if execution.Workspace.Worktree == "" {
				if err := s.recoverRemoteWorkerWorkspace(ctx, run, execution, preparation); err != nil {
					return s.waitWorkerFlow(run, "Original remote worktree cannot be reconciled: "+err.Error())
				}
			} else {
				run.WorktreePath, run.WorktreeBranch, run.BaseCommit = ptr(execution.Workspace.Worktree), ptr(execution.Workspace.Branch), ptr(execution.Workspace.Base)
				if preparation.Status != "confirmed" {
					if err := s.changeOperation(preparation.OperationID, "confirmed", preparation.PlatformRequestID, mustJSON(execution.Workspace), ""); err != nil {
						return err
					}
				}
			}
		} else {
			if _, err := os.Lstat(run.WorkspaceIntent.Path); os.IsNotExist(err) && preparation.Status != "confirmed" {
				if err := prepareWorktreeOnly(ctx, s.Config, run); err != nil {
					return s.waitWorkerFlow(run, "Original workspace preparation cannot be reconciled: "+err.Error())
				}
			} else if err != nil {
				return s.waitWorkerFlow(run, "Original workspace cannot be inspected: "+err.Error())
			}
			run.WorktreePath, run.WorktreeBranch, run.BaseCommit = ptr(run.WorkspaceIntent.Path), ptr(run.WorkspaceIntent.Branch), ptr(run.WorkspaceIntent.BaseCommit)
			workspace, err := s.inspectWorkflowWorkspace(ctx, run)
			if err != nil {
				return s.waitWorkerFlow(run, "Original workspace identity is unresolved: "+err.Error())
			}
			applyWorkflowWorkspaceEvidence(run, execution, workspace)
			if preparation.Status != "confirmed" {
				if err := s.changeOperation(preparation.OperationID, "confirmed", "", mustJSON(workspace), ""); err != nil {
					return err
				}
			}
		}
		if s.Config.hostName() == "orca" || s.Config.hostName() == "orca_supervised" {
			if err := s.prepareWorkerOrcaTerminal(ctx, run); err != nil {
				return s.waitWorkerFlow(run, "Original Orca terminal cannot be reconciled: "+err.Error())
			}
		}
		return s.startInitialWorkerFlowTurn(ctx, *task, run)
	}
	return nil
}

func (s *Service) waitWorkerFlow(run *Run, reason string) error {
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || workflow.WorkflowVersion != WorkerFlowVersion {
		return errors.New("worker flow wait has no fixed execution")
	}
	if workflow.Phase == "completed" || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nil
	}
	if run.Decision == nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved" {
		request := DecisionRequest{Question: "The worker stopped. What should it do next?", Context: clip(reason, 7000), Options: []DecisionOption{{ID: "continue", Label: "Continue current work"}}, AllowFreeText: true}
		decision, err := WriteDecisionRequest(run, request)
		if err != nil {
			return err
		}
		run.Decision = decision
	}
	workflow.Phase, workflow.UpdatedAt = "waiting_decision", now()
	if execution.Status == "active" {
		execution.Status = "idle"
	}
	execution.UpdatedAt = now()
	run.FinishTaskStatus = "waiting_decision"
	run.Error = ptr(clip(reason, 2000))
	s.note(run, "steer", "decision", "pending", "worker needs input", reason, decisionMeta(run))
	return s.save()
}

// A verified open PR does not require input, but the original session remains
// available if the user wants to request more work before merging it.
func offerWorkerPRFeedback(run *Run, url string) error {
	if run == nil || run.WorkflowVersion != WorkerFlowVersion || url == "" {
		return errors.New("optional PR feedback requires a fixed worker and PR")
	}
	if run.Decision != nil && run.Decision.Status == "pending" {
		return nil
	}
	decision, err := WriteDecisionRequest(run, DecisionRequest{
		Question: "The worker stopped. What should it do next?",
		Context:  "The verified PR is awaiting merge. You may send further instructions to the original worker session.",
		Options:  []DecisionOption{{ID: "continue", Label: "Continue current work"}}, AllowFreeText: true,
	})
	if err != nil {
		return err
	}
	run.Decision = decision
	return nil
}

func (s *Service) prepareWorkerFlow(ctx context.Context, task TaskRecord, run *Run) error {
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil || workflow.WorkflowVersion != WorkerFlowVersion || attempt.StepID != "worker" || run.WorkspaceIntent == nil {
		return nativeErr("inconsistent_delivery", "worker preparation has no fixed claim and workspace intent")
	}
	if s.Config.hostName() != "local" && s.Config.hostName() != "orca" && s.Config.hostName() != "orca_supervised" {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "same-session worker continuation is unavailable for this target"}
	}
	remoteName := ""
	if s.Config.hostName() == "orca_supervised" {
		var nameErr error
		remoteName, nameErr = workerOrcaWorktreeName(run.RunID)
		if nameErr != nil {
			return nameErr
		}
	}
	opID, err := s.startOperation("prepare_workspace", "worker-workspace:"+execution.ExecutionID, struct {
		WorkflowRunID string          `json:"workflow_run_id"`
		ExecutionID   string          `json:"execution_id"`
		RemoteName    string          `json:"remote_name,omitempty"`
		Workspace     WorkspaceIntent `json:"workspace"`
	}{workflow.WorkflowRunID, execution.ExecutionID, remoteName, *run.WorkspaceIntent})
	if err != nil {
		return err
	}
	workflow, execution = s.workflowRecords(run)
	attempt = s.workflowAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("worker claim disappeared after preparation intent was saved")
	}
	if s.Config.hostName() == "orca_supervised" {
		err = s.prepareRemoteWorkerWorkspace(ctx, run, execution, opID)
	} else {
		err = prepareWorktreeOnly(ctx, s.Config, run)
	}
	if err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		run.ExecutionStatus = "unknown"
		execution.Status = "blocked"
		return s.waitWorkerFlow(run, "Workspace preparation needs reconciliation: "+err.Error())
	}
	if s.Config.hostName() != "orca_supervised" {
		run.WorktreePath, run.WorktreeBranch, run.BaseCommit = ptr(run.WorkspaceIntent.Path), ptr(run.WorkspaceIntent.Branch), ptr(run.WorkspaceIntent.BaseCommit)
		workspace, inspectErr := s.inspectWorkflowWorkspace(ctx, run)
		if inspectErr != nil {
			_ = s.changeOperation(opID, "outcome_unknown", "", nil, inspectErr.Error())
			run.ExecutionStatus = "unknown"
			execution.Status = "blocked"
			return s.waitWorkerFlow(run, "Workspace identity could not be verified: "+inspectErr.Error())
		}
		applyWorkflowWorkspaceEvidence(run, execution, workspace)
		if err = s.changeOperation(opID, "confirmed", "", mustJSON(workspace), ""); err != nil {
			return err
		}
	}
	if s.Config.hostName() == "orca" || s.Config.hostName() == "orca_supervised" {
		if err := s.prepareWorkerOrcaTerminal(ctx, run); err != nil {
			return s.waitWorkerFlow(run, "Original Orca terminal preparation is unverified: "+err.Error())
		}
	}
	return s.startInitialWorkerFlowTurn(ctx, task, run)
}

func (s *Service) startInitialWorkerFlowTurn(ctx context.Context, task TaskRecord, run *Run) error {
	contract, err := nativeContractForTask(ctx, s.domainStore, s.Config.identity, task)
	if err != nil {
		return s.waitWorkerFlow(run, "Fixed task contract is unavailable: "+err.Error())
	}
	prompt, err := workerFlowPrompt(task, contract, s.Config.hostName() == "local")
	if err != nil {
		return s.waitWorkerFlow(run, err.Error())
	}
	return s.startWorkerFlowTurn(ctx, run, prompt, newID())
}

func (s *Service) startWorkerFlowTurn(ctx context.Context, run *Run, prompt, turnID string) error {
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil || workflow.WorkflowVersion != WorkerFlowVersion || !stableExecutionID(turnID) ||
		execution.PendingTurnID != "" || execution.SessionID != "" && execution.CurrentTurnID != "" && execution.LastStoppedTurnID != execution.CurrentTurnID {
		return errors.New("worker turn cannot start without one idle original session")
	}
	if execution.SessionID == "" && execution.TurnNumber != 0 {
		return errors.New("worker session was lost; replacement is not allowed")
	}
	if s.Config.hostName() == "local" {
		prompt += localWorkerPublishInstruction(run.RunID)
	}
	execution.PendingTurnID = turnID
	opID, err := s.startOperation("start_worker_turn", "worker-turn:"+run.ExecutionID+":"+turnID, struct {
		ExecutionID string `json:"execution_id"`
		SessionID   string `json:"session_id,omitempty"`
		TurnID      string `json:"turn_id"`
		Prompt      string `json:"prompt"`
	}{run.ExecutionID, execution.SessionID, turnID, prompt})
	if err != nil {
		return err
	}
	workflow, execution = s.workflowRecords(run)
	attempt = s.workflowAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("worker turn disappeared after send intent was saved")
	}
	var worker *Worker
	var events, platformRequestID string
	if s.Config.hostName() == "local" {
		worker, events, err = startLocalWorkerTurn(ctx, s.Config, run, execution, turnID, prompt)
		platformRequestID = turnID
	} else {
		worker, events, platformRequestID, err = startWorkerOrcaTurn(ctx, s.Config, run, execution, turnID, prompt)
	}
	if err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", platformRequestID, nil, err.Error())
		run.ExecutionStatus = "unknown"
		return s.waitWorkerFlow(run, "Original worker turn start is unverified: "+err.Error())
	}
	identity := worker.proc.Identity()
	run.Process, execution.Process = &identity, &identity
	run.ExecutionStatus = "pending"
	if attempt.EventPath == "" {
		attempt.EventPath = events
		run.LogPath = ptr(events)
	}
	attempt.Status = "dispatched"
	attempt.UpdatedAt = now()
	if err := s.setOperationState(opID, "accepted", platformRequestID, mustJSON(identity), ""); err != nil {
		return err
	}
	if err := s.save(); err != nil {
		return fmt.Errorf("worker started but its exact process receipt could not be saved: %w", err)
	}
	s.registerWorker(run, worker)
	s.note(run, "steer", "worker", "accepted", "original worker turn accepted", turnID, map[string]any{"session_id": execution.SessionID, "turn_number": execution.TurnNumber + 1})
	return nil
}

func (s *Service) eventWorkerFlow(ev WorkerEvent) error {
	run := s.active
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
			ackWorkerEventPersistence(ev, err)
			if err != nil {
				return err
			}
			s.recordActivity(run, ev.Activity)
		} else {
			ackWorkerEventPersistence(ev, nil)
		}
		return nil
	}
	if ev.Session != "" {
		if !codexThreadID.MatchString(ev.Session) || execution.SessionID != "" && execution.SessionID != ev.Session {
			return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker event changed the original Codex session"}
		}
		execution.SessionID = ev.Session
		run.SessionID = ptr(ev.Session)
	}
	if ev.Activity.Kind == "turn" && ev.Activity.Status == "started" {
		turnID := execution.PendingTurnID
		if turnID == "" {
			turnID = execution.CurrentTurnID
		}
		if err := BeginWorkerTurn(workflow, execution, execution.SessionID, turnID); err != nil {
			return err
		}
		if attempt := s.workflowAttempt(run, run.AttemptID); attempt != nil {
			attempt.Status, attempt.UpdatedAt = "running", now()
		}
		if run.Decision != nil && run.Decision.Status == "answer_saved" {
			run.Decision.Status = "reply_accepted"
			run.Decision.Version++
		}
		for _, op := range s.State.Operations {
			if op.IdempotencyKey == "worker-turn:"+run.ExecutionID+":"+turnID {
				requestID := op.PlatformRequestID
				if requestID == "" {
					requestID = turnID
				}
				if err := s.setOperationState(op.OperationID, "confirmed", requestID, mustJSON(map[string]string{"session_id": execution.SessionID, "turn_id": turnID}), ""); err != nil {
					return err
				}
				break
			}
		}
		run.ExecutionStatus, run.FinishTaskStatus, run.Error = "running", "pending", nil
		run.StartedAt = ptr(now())
	}
	run.LastEvent = ptr(ev.Summary)
	if ev.Activity.Kind != "" {
		if err := s.recordConversationEvent(run, ev); err != nil {
			ackWorkerEventPersistence(ev, err)
			return err
		}
		ackWorkerEventPersistence(ev, nil)
		s.recordActivity(run, ev.Activity)
		s.emitConsole(ev.Activity)
	} else if ev.Persisted != nil {
		err := errors.New("worker source event lacks activity classification")
		ackWorkerEventPersistence(ev, err)
		return err
	}
	return s.save()
}

func (s *Service) finishWorkerFlow(workerErr error) error {
	run := s.active
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || workflow.WorkflowVersion != WorkerFlowVersion {
		return errors.New("finished worker lacks its fixed execution")
	}
	if task := nativeTaskRecord(s.State.Tasks, run.DomainTaskID); task != nil && (task.Status == "closed" || task.Status == "discarded") {
		return nil // A late process exit cannot reopen a terminal Task.
	}
	turnID := execution.CurrentTurnID
	if turnID == "" {
		turnID = execution.PendingTurnID
	}
	if !stableExecutionID(turnID) {
		return s.waitWorkerFlow(run, "The original worker exited without a saved turn identity")
	}
	events := filepath.Join(s.Config.RunDir, "runs", run.RunID, "turns", turnID, "events.jsonl")
	outcome, readErr := readWorkerTurnOutcome(events)
	if readErr != nil || !outcome.Started || outcome.SessionID == "" || execution.SessionID != "" && execution.SessionID != outcome.SessionID {
		reason := "The original worker exited, but its turn output is insufficient to determine the result"
		if readErr != nil {
			reason += ": " + readErr.Error()
		}
		execution.Status = "idle"
		run.ExecutionStatus = "unknown"
		return s.waitWorkerFlow(run, reason)
	}
	if execution.CurrentTurnID == "" {
		if err := BeginWorkerTurn(workflow, execution, outcome.SessionID, turnID); err != nil {
			return s.waitWorkerFlow(run, "Original turn start could not be reconciled: "+err.Error())
		}
		run.SessionID = ptr(outcome.SessionID)
		for _, operation := range s.State.Operations {
			if operation.IdempotencyKey == "worker-turn:"+run.ExecutionID+":"+turnID && operation.Status != "confirmed" {
				requestID := operation.PlatformRequestID
				if requestID == "" {
					requestID = turnID
				}
				if err := s.setOperationState(operation.OperationID, "confirmed", requestID, mustJSON(map[string]string{"session_id": outcome.SessionID, "turn_id": turnID}), ""); err != nil {
					return err
				}
				break
			}
		}
	}
	if !outcome.Completed {
		execution.Status = "idle"
		run.ExecutionStatus = "unknown"
		return s.waitWorkerFlow(run, "The original worker process exited without a confirmed completed turn; its output and session are retained")
	}
	if run.Decision != nil && run.Decision.Status == "answer_saved" {
		answer, answerErr := ReadDecisionAnswer(run)
		if answerErr == nil {
			command, commandErr := s.domainStore.CommandByIdempotency(context.Background(), s.Config.identity, decisionAnswerKey(answer))
			if commandErr == nil && command.CommandID == turnID && command.Kind == "answer_decision" {
				run.Decision.Status = "reply_accepted"
				run.Decision.Version++
			}
		}
	}
	issue := ""
	if workerErr != nil {
		issue = workerErr.Error()
	}
	if outcome.Failure != "" {
		issue = strings.TrimSpace(issue + "; " + outcome.Failure)
	}
	if len(outcome.PRCandidates) == 1 {
		candidate := outcome.PRCandidates[0]
		workflow.PRCandidateURL = candidate
		info, err := s.prs.Query(context.Background(), candidate)
		if err == nil {
			err = BindWorkerPR(workflow, execution, info, candidate, s.Config.PR.Repository, false)
		}
		if err != nil {
			issue = strings.TrimSpace(issue + "; PR link is not verified: " + err.Error())
		}
	} else if len(outcome.PRCandidates) > 1 {
		issue = strings.TrimSpace(issue + "; several PR links were reported: " + strings.Join(outcome.PRCandidates, ", "))
	}
	question := strings.HasSuffix(strings.TrimSpace(outcome.LastMessage), "?") || strings.HasSuffix(strings.TrimSpace(outcome.LastMessage), "？")
	if len(outcome.PRCandidates) > 1 {
		question = true
	}
	if run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.Question != "The worker stopped. What should it do next?" {
		question = true
	}
	if err := StopWorkerTurn(workflow, execution, turnID, question); err != nil {
		return err
	}
	run.EndedAt = ptr(now())
	run.ExecutionStatus = "succeeded"
	if workflow.Phase == "waiting_pr_merge" {
		run.FinishTaskStatus = "waiting_pr_merge"
	} else {
		run.FinishTaskStatus = "waiting_decision"
	}
	if s.Config.hostName() == "local" && localWorkerRequestedPublish(outcome.LastMessage, run.RunID) {
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
	if workflow.Phase == "waiting_pr_merge" {
		run.FinishTaskStatus = "waiting_pr_merge"
		if workflow.PRURL != "" {
			run.PR.URL, run.PR.State = ptr(workflow.PRURL), workflow.PRState
		}
		if err := offerWorkerPRFeedback(run, workflow.PRURL); err != nil {
			return err
		}
		if issue != "" {
			run.Error = ptr(clip(issue, 2000))
		}
		s.note(run, "steer", "worker", "stopped", "worker turn ended with a verified PR", workflow.PRURL, map[string]any{"turn_id": turnID})
		return s.save()
	}
	context := strings.TrimSpace(outcome.LastMessage)
	if context == "" {
		context = "The worker stopped without a final reply."
	}
	if issue != "" {
		context += "\n\nStop reason: " + issue
	}
	if question {
		decision, err := WriteDecisionRequest(run, DecisionRequest{Question: clip(strings.TrimSpace(outcome.LastMessage), 1800), Context: clip(context, 7000), Options: []DecisionOption{{ID: "continue", Label: "Continue current work"}}, AllowFreeText: true})
		if err == nil {
			run.Decision = decision
		}
	}
	return s.waitWorkerFlow(run, context)
}

func (s *Service) pollWorkerFlowPR(ctx context.Context, run *Run, task *TaskRecord, workflow *WorkflowRunRecord) error {
	if run == nil || task == nil || workflow == nil || workflow.WorkflowVersion != WorkerFlowVersion {
		return nil
	}
	if task.Status == "closed" {
		return nil
	}
	if task.Status == "discarded" {
		if workflow.PRURL == "" {
			return nil
		}
		_, err := (CommandService{Store: s.domainStore, Project: s.Config.identity}).CompleteNativeTask(ctx, s.Config, task.TaskID, task.Version, workflow.WorkflowRunID, s.prs)
		if isNativeErrorCode(err, "task_discarded") {
			return nil
		}
		return err
	}
	url := workflow.PRURL
	if url == "" {
		url = workflow.PRCandidateURL
	}
	if url == "" {
		return nil
	}
	queried := now()
	info, err := s.prs.Query(ctx, url)
	run.PR.QueriedAt = &queried
	if err != nil {
		run.PR.QueryError = ptr(clip(err.Error(), 1000))
		if workflow.PRURL == "" && workflow.Phase != "executing" {
			return s.waitWorkerFlow(run, "PR link could not be verified; the center will retry: "+err.Error())
		}
		return s.save()
	}
	execution := s.executionRecord(run)
	if execution == nil {
		return nativeErr("inconsistent_delivery", "linked PR has no selected worker execution")
	}
	if err := validateWorkerPRIdentity(info, url, s.Config.PR.Repository, workflow.Branch, execution.Workspace.Branch); err != nil {
		run.PR.QueryError = ptr("PR does not belong to the fixed task execution branch")
		if workflow.Phase != "executing" {
			return s.waitWorkerFlow(run, "PR link is for a different repository, base or worker branch")
		}
		return s.save()
	}
	pendingQuestion := run.Decision != nil && (run.Decision.Status == "answer_saved" || run.Decision.Status == "pending" && run.Decision.Request.Question != "The worker stopped. What should it do next?")
	previousPRURL := workflow.PRURL
	if err := BindWorkerPR(workflow, execution, info, url, s.Config.PR.Repository, pendingQuestion); err != nil {
		return s.waitWorkerFlow(run, "Several PRs or conflicting PR identities need a user decision: "+err.Error())
	}
	if run.Decision != nil && run.Decision.Status == "pending" && !pendingQuestion && previousPRURL != info.URL && workflow.Phase != "waiting_decision" {
		run.Decision.Status = "superseded"
		run.Decision.Version++
	}
	if workflow.Phase == "waiting_pr_merge" && execution.Status != "active" {
		if err := offerWorkerPRFeedback(run, info.URL); err != nil {
			return err
		}
	}
	run.PR.URL, run.PR.State, run.PR.MergedAt, run.PR.MergeCommit, run.PR.QueryError = ptr(info.URL), info.State, info.MergedAt, info.MergeCommit, nil
	switch workflow.Phase {
	case "waiting_pr_merge":
		run.FinishTaskStatus = "waiting_pr_merge"
	case "waiting_decision":
		run.FinishTaskStatus = "waiting_decision"
	default:
		run.FinishTaskStatus = "pending"
	}
	if err := s.save(); err != nil {
		return err
	}
	if info.State == "merged" {
		return s.nativeCompleteMergedTask(ctx, run)
	}
	if info.State == "closed_unmerged" && execution.Status != "active" {
		return s.waitWorkerFlow(run, "PR closed without merge. Send feedback to the original worker session.")
	}
	return nil
}

func (s *Service) applyWorkerFlowDecision(ctx context.Context, run *Run) error {
	if run == nil || run.WorkflowVersion != WorkerFlowVersion || run.Decision == nil || run.Decision.Status != "answer_saved" {
		return nil
	}
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || workflow.Phase != "waiting_decision" && workflow.Phase != "waiting_pr_merge" {
		return errors.New("saved feedback no longer belongs to a waiting worker flow")
	}
	if workflow.Phase == "waiting_pr_merge" {
		workflow.Phase, workflow.UpdatedAt = "waiting_decision", now()
		run.FinishTaskStatus = "waiting_decision"
		if err := s.save(); err != nil {
			return err
		}
	}
	answer, err := ReadDecisionAnswer(run)
	if err != nil {
		return err
	}
	command, err := s.domainStore.CommandByIdempotency(ctx, s.Config.identity, decisionAnswerKey(answer))
	if err != nil || command.Kind != "answer_decision" || command.TargetID != answer.DecisionID || command.Status != "accepted" || !stableExecutionID(command.CommandID) {
		return errors.Join(err, errors.New("saved worker feedback has no fixed accepted command identity"))
	}
	if execution.SessionID == "" || run.SessionID == nil || *run.SessionID != execution.SessionID {
		run.Error = ptr("Feedback is saved, but the original worker session is not yet available for delivery")
		return s.save()
	}
	if execution.Status == "active" || s.workers[run.RunID] != nil {
		return nil // Preserve the answer until the original turn stops.
	}
	for _, operation := range s.State.Operations {
		if operation.IdempotencyKey != "worker-turn:"+execution.ExecutionID+":"+command.CommandID {
			continue
		}
		if operation.Status == "confirmed" || operation.Status == "accepted" || operation.Status == "in_flight" {
			return nil // The original request may have started. Never resend.
		}
		run.Error = ptr("Feedback delivery remains unverified at the original request ID; no duplicate message was sent")
		return s.save()
	}
	return s.startWorkerFlowTurn(ctx, run, workerFeedbackText(answer, run.Decision.Request), command.CommandID)
}

func workerFeedbackText(answer DecisionAnswer, request DecisionRequest) string {
	if answer.OptionID == "" {
		return answer.Note
	}
	label := answer.OptionID
	for _, option := range request.Options {
		if option.ID == answer.OptionID {
			label = option.Label
			break
		}
	}
	message := "Selected option: " + label
	if answer.Note != "" {
		message += "\n\nAdditional instructions:\n" + answer.Note
	}
	return message
}
