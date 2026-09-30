package steerline

import (
	"context"
	"errors"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

// Every service instance must revalidate unfinished executions before a saved
// answer or dispatch intent may write to an external worker. The gate is part
// of the Execution row, so an abrupt process exit has the same startup rule.
func (s *Service) adoptOrGateServiceExecutions() error {
	return s.markServiceRecoveryGateForPriorOwner()
}

func (s *Service) markServiceRecoveryGateForPriorOwner() error {
	for _, run := range s.runs {
		if run == nil || run.WorkflowVersion != WorkerFlowVersion || run.TaskStatus != "in_progress" {
			continue
		}
		workflow, execution := s.workflowRecords(run)
		if workflow == nil || execution == nil || workflow.Phase == "completed" || workflow.Phase == "abandoned" {
			continue
		}
		if execution.OwnerServiceInstanceID != s.State.Service.InstanceID {
			s.markOneServiceRecoveryGate(run)
			execution.OwnerServiceInstanceID = s.State.Service.InstanceID
			s.refreshRun(run)
			if err := s.commitWorker(run); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) markServiceRecoveryGate() error {
	for _, run := range s.runs {
		s.markOneServiceRecoveryGate(run)
		if run != nil && run.TaskStatus == "in_progress" {
			if err := s.commitWorker(run); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) markOneServiceRecoveryGate(run *Run) {
	if run == nil || run.WorkflowVersion != WorkerFlowVersion {
		return
	}
	task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
	workflow, execution := s.workflowRecords(run)
	if task == nil || workflow == nil || execution == nil || task.Status != "in_progress" || workflow.Phase == "completed" || workflow.Phase == "abandoned" {
		return
	}
	execution.RecoveryRequired = true
	execution.RecoveryVersion++
	execution.RecoveryState = "unverified"
	marked := now()
	execution.RecoveryMarkedAt = &marked
	execution.UpdatedAt = marked
	s.refreshRun(run)
}

func (s *Service) resumeConfiguredWork(ctx context.Context) error {
	if err := s.resumeWorkerFlow(ctx); err != nil {
		return err
	}
	for _, run := range s.runs {
		if run == nil || run.WorkflowVersion != WorkerFlowVersion || run.Protocol != string(ProtocolOrcaTerminalV2) {
			continue
		}
		workflow, execution := s.workflowRecords(run)
		if workflow == nil || execution == nil || workflow.Phase != "completed" || execution.OrcaNative == nil {
			continue
		}
		if err := s.finishNativeOrcaCleanup(ctx, run); err != nil {
			execution.LastEvent = ptr("Orca delivery completed; owned terminal cleanup needs reconciliation: " + err.Error())
			s.refreshRun(run)
			if saveErr := s.commitWorker(run); saveErr != nil {
				return saveErr
			}
		}
	}
	return nil
}

func (s *Service) resumeWorkerFlow(ctx context.Context) error {
	if err := s.resumeAppServerWorkers(ctx); err != nil {
		return err
	}
	for _, run := range s.runs {
		if run == nil || run.WorkflowVersion != WorkerFlowVersion || s.workers[run.RunID] != nil {
			continue
		}
		task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
		workflow, execution := s.workflowRecords(run)
		if task == nil || workflow == nil || execution == nil || task.Status != "in_progress" || workflow.Phase == "completed" {
			continue
		}
		adapter, err := s.workerFlowMode(run)
		if err != nil {
			return err
		}
		_, err = s.resumeWorkerFlowByMode(ctx, task, run, adapter)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) waitWorkerFlow(run *Run, reason string, operationIDs ...string) error {
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || workflow.WorkflowVersion != WorkerFlowVersion {
		return errors.New("worker flow wait has no fixed execution")
	}
	if workflow.Phase == "completed" || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nil
	}
	unresolvedNativeTurn := run.Protocol == string(ProtocolLocalCodexAppServer) && !execution.RecoveryRequired && (execution.PendingCommandID != "" || execution.CurrentTurnID != "" && execution.CurrentTurnID != execution.LastStoppedTurnID)
	if !unresolvedNativeTurn && (run.Decision == nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved") {
		contextText := reason
		if run.Protocol == string(ProtocolLocalCodexAppServer) && execution.CurrentTurnID != "" && execution.CurrentTurnID == execution.LastStoppedTurnID {
			contextText = "本轮已结束，提交后将在原 Codex 会话开始下一轮。\n\n" + reason
		}
		options := []DecisionOption{{ID: "continue", Label: "Continue current work"}}
		if execution.RecoveryRequired && (execution.RecoveryState == "interrupted" || execution.RecoveryState == "completed") && run.Protocol == string(ProtocolLocalCodexAppServer) && execution.Process != nil {
			options = []DecisionOption{{ID: "continue", Label: "原 Codex 会话继续", Detail: "在新的本地进程中核实并恢复原 thread"}, {ID: "new_session", Label: "新会话继续", Detail: "使用现有工作区和持久历史作为上下文"}}
		} else if execution.RecoveryRequired && execution.RecoveryState == "interrupted" && run.Protocol == string(ProtocolLocalCodexAppServer) {
			options = []DecisionOption{{ID: "continue", Label: "开始本地 Codex 执行", Detail: "先核实或准备原任务的工作区"}}
		}
		question := "The worker stopped. What should it do next?"
		if execution.RecoveryRequired && (execution.RecoveryState == "unverified" || execution.RecoveryState == "") {
			question = "Worker state is unverified. Review the original execution evidence."
			options = nil
		}
		request := DecisionRequest{Question: question, Context: clip(contextText, 7000), Options: options, AllowFreeText: true}
		decision, err := WriteDecisionRequest(run, request)
		if err != nil {
			return err
		}
		s.setRunDecision(run, decision)
		if execution.RecoveryRequired {
			if record := s.decisionRecord(run); record != nil {
				record.RecoveryVersion = execution.RecoveryVersion
				record.RecoveryState = execution.RecoveryState
			}
		}
	}
	workflow.Phase, workflow.UpdatedAt = "waiting_decision", now()
	if unresolvedNativeTurn && (run.Decision == nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved") {
		workflow.Phase = "executing"
		if execution.CurrentTurnID == "" || execution.CurrentTurnID == execution.LastStoppedTurnID {
			workflow.Phase = "queued"
		}
	}
	if execution.Status == "active" && run.Protocol != string(ProtocolLocalCodexAppServer) {
		execution.Status = "idle"
	}
	execution.UpdatedAt = now()
	execution.Error = ptr(clip(reason, 2000))
	s.refreshRun(run)
	if unresolvedNativeTurn && workflow.Phase != "waiting_decision" {
		s.note(run, "steer", "worker", "unknown", "original worker turn needs reconciliation", reason, nil)
	} else {
		s.note(run, "steer", "decision", "pending", "worker needs input", reason, decisionMeta(run))
	}
	return s.commitWorker(run, operationIDs...)
}

// A verified open PR does not require input, but the original session remains
// available if the user wants to request more work before merging it.
func (s *Service) offerWorkerPRFeedback(run *Run, url string) error {
	if run == nil || run.WorkflowVersion != WorkerFlowVersion || url == "" {
		return errors.New("optional PR feedback requires a fixed worker and PR")
	}
	if run.Decision != nil && run.Decision.Status == "pending" {
		return nil
	}
	decision, err := WriteDecisionRequest(run, DecisionRequest{
		Question: "The worker stopped. What should it do next?",
		Context:  "PR 已核实并等待合并。本轮已结束，提交反馈后将在原 Codex 会话开始下一轮。",
		Options:  []DecisionOption{{ID: "continue", Label: "Continue current work"}}, AllowFreeText: true,
	})
	if err != nil {
		return err
	}
	s.setRunDecision(run, decision)
	return nil
}

func (s *Service) prepareWorkerFlow(ctx context.Context, task TaskRecord, run *Run) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	if fixed.workflow.WorkflowVersion != WorkerFlowVersion || fixed.attempt.StepID != "worker" || run.WorkspaceIntent == nil {
		return nativeErr("inconsistent_delivery", "worker preparation has no fixed claim and workspace intent")
	}
	if fixed.execution.RecoveryRequired {
		return errors.New("existing worker execution requires a fresh manual recovery decision")
	}
	adapter, err := s.workerFlowMode(run)
	if err != nil {
		return err
	}
	return s.prepareWorkerFlowByMode(ctx, task, run, adapter)
}

func (s *Service) prepareWorkerWorkspace(ctx context.Context, task TaskRecord, run *Run, remote bool) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	workflow, execution, attempt, c := fixed.workflow, fixed.execution, fixed.attempt, fixed.config
	if run.WorkspaceIntent == nil {
		return nativeErr("inconsistent_delivery", "worker preparation has no fixed workspace intent")
	}
	remoteName := ""
	if remote {
		var nameErr error
		remoteName, nameErr = workerOrcaWorktreeName(run.TaskID, run.RunID)
		if nameErr != nil {
			return nameErr
		}
	}
	opID, err := s.startWorkerOperation(run, "prepare_workspace", "worker-workspace:"+execution.ExecutionID, struct {
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
	if remote {
		err = s.prepareRemoteWorkerWorkspace(ctx, run, execution, opID)
	} else {
		err = prepareWorktreeOnly(ctx, c, run)
	}
	if err != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
		execution.Status = "blocked"
		s.refreshRun(run)
		return s.waitWorkerFlow(run, "Workspace preparation needs reconciliation: "+err.Error())
	}
	if !remote {
		s.refreshRun(run)
		workspace, inspectErr := s.inspectWorkflowWorkspace(ctx, run)
		if inspectErr != nil {
			_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, inspectErr.Error())
			execution.Status = "blocked"
			s.refreshRun(run)
			return s.waitWorkerFlow(run, "Workspace identity could not be verified: "+inspectErr.Error())
		}
		applyWorkflowWorkspaceEvidence(run, execution, workspace)
		if err = s.changeWorkerOperation(run, opID, "confirmed", "", mustJSON(workspace), ""); err != nil {
			return err
		}
	}
	if remote {
		if err := s.prepareWorkerOrcaTerminal(ctx, run); err != nil {
			return s.waitWorkerFlow(run, "Original Orca terminal preparation is unverified: "+err.Error())
		}
	}
	return s.startInitialWorkerFlowTurn(ctx, task, run)
}

func (s *Service) startInitialWorkerFlowTurn(ctx context.Context, task TaskRecord, run *Run) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	c := fixed.config
	contract, err := nativeContractForTask(ctx, s.domainStore, c.identity, task)
	if err != nil {
		return s.waitWorkerFlow(run, "Fixed task contract is unavailable: "+err.Error())
	}
	prompt, err := workerflow.Prompt(task, contract, run.RunID, c.hostName() == "local")
	if err != nil {
		return s.waitWorkerFlow(run, err.Error())
	}
	return s.startWorkerFlowTurn(ctx, run, prompt, newID())
}

func (s *Service) startWorkerFlowTurn(ctx context.Context, run *Run, prompt, turnID string) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	workflow, execution, attempt := fixed.workflow, fixed.execution, fixed.attempt
	if execution.RecoveryRequired {
		return errors.New("worker turn requires a fresh manual recovery decision")
	}
	if workflow.WorkflowVersion != WorkerFlowVersion || !stableExecutionID(turnID) ||
		execution.PendingTurnID != "" || execution.PendingCommandID != "" || execution.SessionID != "" && execution.CurrentTurnID != "" && execution.LastStoppedTurnID != execution.CurrentTurnID {
		return errors.New("worker turn cannot start without one idle original session")
	}
	if execution.SessionID == "" && execution.TurnNumber != 0 && run.DomainAttemptNumber <= 1 {
		return errors.New("worker session was lost; replacement is not allowed")
	}
	if attempt.StepID != "worker" {
		return errors.New("worker turn has no fixed worker attempt")
	}
	adapter, err := s.workerFlowMode(run)
	if err != nil {
		return err
	}
	if adapter == workerflow.LocalCodex {
		prompt += localWorkerPublishInstruction(run.RunID)
	} else if adapter == workerflow.NativeOrca {
		prompt += nativeOrcaRelayInstruction(run.RunID, turnID)
	}
	execution.PendingTurnID, execution.PendingCommandID = adapter.PendingIdentity(turnID)
	opID, err := s.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+run.ExecutionID+":"+turnID, struct {
		ExecutionID string `json:"execution_id"`
		SessionID   string `json:"session_id,omitempty"`
		TurnID      string `json:"turn_id"`
		Prompt      string `json:"prompt"`
	}{run.ExecutionID, execution.SessionID, turnID, prompt})
	if err != nil {
		return err
	}
	if _, err := s.workerFlowContext(run); err != nil {
		return errors.New("worker turn disappeared after send intent was saved")
	}
	return s.startWorkerTurnByMode(ctx, run, turnID, prompt, opID, adapter)
}

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
	question := evidence.QuestionHint
	if len(evidence.PRCandidates) > 1 {
		question = true
	}
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

func (s *Service) pollWorkerFlowPR(ctx context.Context, run *Run, task *TaskRecord, workflow *WorkflowRunRecord) error {
	fixed, contextErr := s.workerFlowContext(run)
	if contextErr != nil {
		return contextErr
	}
	c := fixed.config
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
		_, err := (CommandService{Store: s.domainStore, Project: c.identity}).CompleteNativeTask(ctx, c, task.TaskID, task.Version, workflow.WorkflowRunID, s.prs)
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
	workflow.PRQueriedAt = &queried
	s.refreshRun(run)
	if err != nil {
		workflow.PRQueryError = ptr(clip(err.Error(), 1000))
		s.refreshRun(run)
		if workflow.PRURL == "" && workflow.Phase != "executing" {
			return s.waitWorkerFlow(run, "PR link could not be verified; the center will retry: "+err.Error())
		}
		return s.commitWorker(run)
	}
	execution := s.executionRecord(run)
	if execution == nil {
		return nativeErr("inconsistent_delivery", "linked PR has no selected worker execution")
	}
	if err := validateWorkerPRIdentity(info, url, c.PR.Repository, workflow.Branch, execution.Workspace.Branch); err != nil {
		workflow.PRQueryError = ptr("PR does not belong to the fixed task execution branch")
		s.refreshRun(run)
		if workflow.Phase != "executing" {
			return s.waitWorkerFlow(run, "PR link is for a different repository, base or worker branch")
		}
		return s.commitWorker(run)
	}
	pendingQuestion := run.Decision != nil && (run.Decision.Status == "answer_saved" || run.Decision.Status == "pending" && run.Decision.Request.Question != "The worker stopped. What should it do next?")
	previousPRURL := workflow.PRURL
	if err := BindWorkerPR(workflow, execution, info, url, c.PR.Repository, pendingQuestion); err != nil {
		return s.waitWorkerFlow(run, "Several PRs or conflicting PR identities need a user decision: "+err.Error())
	}
	if run.Decision != nil && run.Decision.Status == "pending" && !pendingQuestion && previousPRURL != info.URL && workflow.Phase != "waiting_decision" {
		s.setDecisionStatus(run, "superseded")
	}
	if workflow.Phase == "waiting_pr_merge" && execution.Status != "active" {
		if err := s.offerWorkerPRFeedback(run, info.URL); err != nil {
			return err
		}
	}
	workflow.PRURL, workflow.PRState, workflow.PRMergedAt, workflow.PRMergeCommit, workflow.PRQueryError = info.URL, info.State, info.MergedAt, info.MergeCommit, nil
	s.refreshRun(run)
	if err := s.commitWorker(run); err != nil {
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
	fixed, contextErr := s.workerFlowContext(run)
	if contextErr != nil {
		return contextErr
	}
	c := fixed.config
	if run == nil || run.WorkflowVersion != WorkerFlowVersion || run.Decision == nil || run.Decision.Status != "answer_saved" {
		return nil
	}
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || workflow.Phase != "waiting_decision" && workflow.Phase != "waiting_pr_merge" {
		return errors.New("saved feedback no longer belongs to a waiting worker flow")
	}
	config, err := s.executionConfigForRun(run)
	if err != nil {
		return err
	}
	route, err := workerFlowModeFor(config, run, execution)
	if err != nil {
		return err
	}
	answer, err := ReadDecisionAnswer(run)
	if err != nil {
		return err
	}
	command, err := s.domainStore.CommandByIdempotency(ctx, c.identity, decisionAnswerKey(answer))
	if err != nil || command.Kind != "answer_decision" || command.TargetID != answer.DecisionID || command.Status != "accepted" || !stableExecutionID(command.CommandID) {
		return errors.Join(err, errors.New("saved worker feedback has no fixed accepted command identity"))
	}
	if execution.RecoveryRequired && (answer.SourceAttemptID != run.AttemptID || answer.ExpectedRecoveryVersion != execution.RecoveryVersion ||
		execution.RecoveryMarkedAt == nil || !command.CreatedAt.After(*execution.RecoveryMarkedAt)) {
		return nil // Prior-service input cannot advance the feedback phase or send.
	}
	if execution.RecoveryRequired && (execution.RecoveryState == "unverified" || execution.RecoveryState == "") {
		return nil // Verify the original worker and pending operations first.
	}
	if execution.RecoveryRequired && s.hasUnknownWorkerOperation(run) {
		execution.RecoveryState = "unverified"
		return s.waitWorkerFlow(run, "An earlier external operation has an unknown outcome; no recovery input was sent")
	}
	if workflow.Phase == "waiting_pr_merge" {
		decision := s.decisionRecord(run)
		if decision == nil || s.store == nil || snapshotWorkflow(&s.committed, workflow.WorkflowRunID) == nil || recordFingerprint(*snapshotWorkflow(&s.committed, workflow.WorkflowRunID)) != recordFingerprint(*workflow) {
			return errors.New("worker feedback phase has unpublished or missing actor state")
		}
		if err := s.store.Err(); err != nil {
			return err
		}
		previousWorkflow := *workflow
		if err := BeginWorkerFeedbackDelivery(workflow, *execution, *decision, now()); err != nil {
			return err
		}
		s.refreshRun(run)
		if err := s.commitWorker(run); err != nil {
			*workflow = previousWorkflow
			s.refreshRun(run)
			return err
		}
		workflow, execution = s.workflowRecords(run)
		if workflow == nil || execution == nil || run.ControlRevision != workflow.ControlRevision || run.FinishTaskStatus != "waiting_decision" {
			return errors.New("committed worker feedback phase could not be projected")
		}
	}
	if execution.RecoveryRequired {
		if route == workerflow.NativeOrca && execution.RecoveryState == "relay_pending" {
			if answer.OptionID != "forward_result" {
				return errors.New("Orca result relay requires explicit forwarding approval")
			}
			s.setDecisionStatus(run, "applied")
			s.setRunDecision(run, nil)
			execution.RecoveryRequired = false
			execution.RecoveryState = "running"
			if err := s.commitWorker(run); err != nil {
				return err
			}
			turn := nativeOrcaCurrentTurn(execution)
			if turn == nil || turn.DispatchID == "" {
				return errors.New("original Orca Dispatch disappeared before relay")
			}
			if err := s.relayNativeOrcaDone(ctx, run, execution, turn); err != nil {
				return s.nativeOrcaUncertain(run, "Manual Orca result relay needs reconciliation: "+err.Error())
			}
			return nil // The original inbox will prove delivery on a later poll.
		}
		if route == workerflow.LocalCodex && execution.RecoveryState == "interrupted" && execution.Process == nil && execution.TurnNumber == 0 {
			return s.startManuallyRecoveredInitialCodex(ctx, run, answer, command.CommandID)
		}
		if route == workerflow.LocalCodex && (execution.RecoveryState == "interrupted" || execution.RecoveryState == "completed") {
			prompt := "The previous local Codex process stopped. Inspect the current worktree, Git status, existing work and persistent task history before continuing. Do not replay an earlier tool call blindly.\n\n" + workerFeedbackText(answer, run.Decision.Request)
			if err := s.beginLocalRecoveryAttempt(ctx, run, answer.OptionID); err != nil {
				return s.waitWorkerFlow(run, "手动继续条件未满足："+err.Error())
			}
			return s.startWorkerFlowTurn(ctx, run, prompt, command.CommandID)
		}
		if execution.RecoveryState != "live" && execution.RecoveryState != "running" && execution.RecoveryState != "question" {
			return nil
		}
		execution.RecoveryRequired = false
		if err := s.commitWorker(run); err != nil {
			return err
		}
	}
	if route == workerflow.NativeOrca {
		return s.applyNativeOrcaDecision(ctx, run, workflow, execution, answer, command.CommandID)
	}
	return s.deliverResumableWorkerFeedback(ctx, run, execution, answer, command.CommandID, route)
}
