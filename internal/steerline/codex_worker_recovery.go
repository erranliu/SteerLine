package steerline

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

func appServerAnswerEvidence(path string, ask codexworker.AskRouteData, answers map[string][]string) (bool, error) {
	return codexworker.AnswerEvidence(path, ask.ThreadID, ask.TurnID, ask.CallID, answers)
}

// A crash can happen after a native request is journaled but before its
// Decision reaches SQLite. Replay only requests not completed or persisted.
type appServerRecoveryResult struct {
	runID     string
	attemptID string
	execution ExecutionRecord
	worker    *Worker
	completed bool
	dead      bool
	err       error
}

func recoverAppServerExecution(ctx context.Context, config Config, run Run, execution ExecutionRecord) appServerRecoveryResult {
	timeout := positiveDuration(config.CommandTimeout, 30*time.Second)
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	commandID, turnID := execution.CurrentCommandID, execution.CurrentTurnID
	if execution.PendingCommandID != "" {
		commandID, turnID = execution.PendingCommandID, execution.PendingTurnID
	}
	binding, err := codexBinding(&execution)
	if err != nil {
		return appServerRecoveryResult{runID: run.RunID, attemptID: run.AttemptID, execution: execution, err: err}
	}
	native, err := codexBackend(config, &run).Recover(probeCtx, binding, commandID, turnID, knownCodexAsks(&run, &execution))
	worker := applyCodexRecovery(&run, &execution, native)
	result := appServerRecoveryResult{runID: run.RunID, attemptID: run.AttemptID, execution: execution, worker: worker, completed: native.Completed, dead: native.Dead, err: err}
	return result
}

// Probe every unsettled local Execution at once. Results are applied only by
// the service writer, so one slow or unavailable endpoint cannot serialize
// the identity checks for unrelated tasks.
func (s *Service) resumeAppServerWorkers(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	results := make(chan appServerRecoveryResult, len(s.runs))
	var probes sync.WaitGroup
	defer func() {
		cancel()
		probes.Wait()
		close(results)
		for result := range results {
			if result.worker != nil {
				_ = result.worker.detachLocalObservation(context.Background())
			}
		}
	}()
	count := 0
	for _, run := range s.runs {
		if run == nil || run.WorkflowVersion != WorkerFlowVersion || run.Protocol != string(executionapi.ProtocolLocalCodexAppServer) || s.workers[run.RunID] != nil || s.appServerLaunching[run.RunID] != "" || s.appServerRecovering[run.RunID] {
			continue
		}
		task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
		workflow, execution := s.workflowRecords(run)
		if task == nil || workflow == nil || execution == nil || task.Status != "in_progress" || workflow.Phase == "completed" || execution.PendingCommandID == "" && (execution.CurrentTurnID == "" || execution.CurrentTurnID == execution.LastStoppedTurnID) {
			continue
		}
		config, err := s.executionConfigForRun(run)
		if err != nil {
			return err
		}
		runCopy, executionCopy := *run, *execution
		if run.Decision != nil {
			decisionCopy := *run.Decision
			runCopy.Decision = &decisionCopy
		}
		executionCopy.PendingQuestions = clonePendingQuestions(execution.PendingQuestions)
		if s.State.Service.Ready && s.workerMessages != nil {
			if s.appServerRecovering == nil {
				s.appServerRecovering = make(map[string]bool)
			}
			s.appServerRecovering[run.RunID] = true
			s.forwarders.Add(1)
			go func() {
				defer s.forwarders.Done()
				result := recoverAppServerExecution(s.runContext, config, runCopy, executionCopy)
				select {
				case s.workerMessages <- workerMessage{runID: runCopy.RunID, recovery: &result}:
				case <-s.runContext.Done():
					if result.worker != nil {
						_ = result.worker.detachLocalObservation(context.Background())
					}
				}
			}()
			continue
		}
		count++
		probes.Add(1)
		go func() {
			defer probes.Done()
			results <- recoverAppServerExecution(ctx, config, runCopy, executionCopy)
		}()
	}
	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message := <-s.workerMessages:
			if err := s.handleWorkerMessage(message); err != nil {
				return err
			}
			i--
		case result := <-results:
			if err := s.applyAppServerRecovery(result); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) applyAppServerRecovery(result appServerRecoveryResult) error {
	defer func() {
		if result.worker != nil && s.workers[result.runID] != result.worker {
			_ = result.worker.detachLocalObservation(context.Background())
		}
	}()
	var run *Run
	for _, candidate := range s.runs {
		if candidate != nil && candidate.RunID == result.runID {
			run = candidate
			break
		}
	}
	if run == nil {
		return errors.New("app-server recovery lost its fixed workflow run")
	}
	if run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nil
	}
	if _, err := s.workerFlowContext(run); err != nil {
		return err
	}
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || execution.ExecutionID != result.execution.ExecutionID {
		return errors.New("app-server recovery changed the fixed execution")
	}
	if result.attemptID != "" && result.attemptID != run.AttemptID || execution.RecoveryVersion != result.execution.RecoveryVersion ||
		execution.PendingCommandID != result.execution.PendingCommandID || execution.CurrentCommandID != result.execution.CurrentCommandID || execution.CurrentTurnID != result.execution.CurrentTurnID {
		return nil // A late probe cannot change a new Attempt or stop the service.
	}
	if result.execution.Process != nil {
		if execution.Process != nil && *execution.Process != *result.execution.Process {
			return s.waitWorkerFlow(run, "Original app-server process identity changed during recovery")
		}
		binding, err := codexBinding(&result.execution)
		if err != nil {
			return s.waitWorkerFlow(run, "Original app-server binding failed verification")
		}
		if err := applyCodexBinding(execution, binding); err != nil {
			return s.waitWorkerFlow(run, "Original app-server binding could not be committed")
		}
		s.refreshRun(run)
	}
	if result.dead {
		if owned := s.ownedLocalProcesses[run.RunID]; owned != nil {
			if owned.Identity() != *execution.Process {
				return s.waitWorkerFlow(run, "Original managed process identity changed during recovery")
			}
			owned.Close()
			delete(s.ownedLocalProcesses, run.RunID)
		}
		if !workerTreeAbsent(*execution.Process) {
			execution.RecoveryState = string(workerflow.RecoveryUnverified)
			return s.waitWorkerFlow(run, "Original worker descendants may still write to the worktree")
		}
		execution.RecoveryState = string(workerflow.ClassifyRecovery(workerflow.RecoveryFacts{WorkerAbsent: true, WritersAbsent: true, Completed: result.completed}))
		if result.completed {
			if codexSessionID(execution) == "" {
				binding, bindingErr := codexBinding(&result.execution)
				if bindingErr != nil || applyCodexBinding(execution, binding) != nil {
					return s.waitWorkerFlow(run, "Original app-server thread identity could not be committed")
				}
			}
			if execution.PendingTurnID == "" {
				execution.PendingTurnID = result.execution.PendingTurnID
			}
			if err := s.finishWorkerFlowFor(run, nil); err != nil {
				return err
			}
			return s.commitWorker(run)
		}
		execution.PendingQuestions = nil
		execution.Status = "blocked"
		s.refreshRun(run)
		if run.Decision != nil && run.Decision.Status == "answer_saved" {
			return s.refreshSavedRecoveryDecision(run, false, "原 Codex worker 已退出，旧请求已失效。请确认带入新轮次的输入。")
		}
		if record := s.decisionRecord(run); record != nil && record.Status == "pending" &&
			record.RecoveryVersion == execution.RecoveryVersion && record.RecoveryState == execution.RecoveryState {
			return nil // Repeated probes preserve the displayed recovery decision.
		}
		oldDecisionID := ""
		priorQuestion := ""
		if run.Decision != nil && run.Decision.Status == "pending" {
			oldDecisionID = run.Decision.ID
			priorQuestion = run.Decision.Request.Question + "\n" + run.Decision.Request.Context
			s.setDecisionStatus(run, "superseded")
			if err := s.commitWorker(run); err != nil {
				return err
			}
			s.setRunDecision(run, nil)
		}
		if err := s.waitWorkerFlow(run, "原 Codex worker 已退出，原轮次没有可信完成结果。请核对当前工作区后手动继续。\n\n原问题："+priorQuestion); err != nil {
			return err
		}
		if record := s.decisionRecord(run); record != nil && oldDecisionID != "" {
			record.RecoveryOfDecision = oldDecisionID
			s.markWorkerDecisionWrite(record.DecisionID)
			return s.commitWorker(run)
		}
		return nil
	}
	if result.err != nil {
		execution.RecoveryState = string(workerflow.RecoveryUnverified)
		execution.Status = "blocked"
		return s.waitWorkerFlow(run, "Original app-server turn cannot be reconciled: "+result.err.Error())
	}
	if codexSessionID(execution) != "" && codexSessionID(execution) != codexSessionID(&result.execution) {
		return s.waitWorkerFlow(run, "Original app-server thread identity changed during recovery")
	}
	binding, bindingErr := codexBinding(&result.execution)
	if bindingErr != nil {
		return s.waitWorkerFlow(run, "Original app-server binding failed verification")
	}
	if bindingErr = applyCodexBinding(execution, binding); bindingErr != nil {
		return s.waitWorkerFlow(run, "Original app-server binding could not be committed")
	}
	execution.RecoveryState = string(workerflow.ClassifyRecovery(workerflow.RecoveryFacts{WorkerLive: true, QuestionPending: run.Decision != nil && run.Decision.Request.QuestionRoute != nil}))
	s.refreshRun(run)
	if execution.PendingCommandID != "" {
		execution.PendingTurnID = result.execution.PendingTurnID
		opID, err := s.recordWorkerTurnStarted(run, workflow, execution, execution.PendingTurnID)
		if err != nil {
			return s.waitWorkerFlow(run, "Original turn start could not be reconciled: "+err.Error())
		}
		if err := s.commitWorker(run, opID); err != nil {
			return err
		}
	}
	if err := s.reconcileAppServerReplyFromJournal(run, workflow, execution); err != nil {
		return s.waitWorkerFlow(run, "Original Codex answer cannot be reconciled: "+err.Error())
	}
	if run.Decision != nil && run.Decision.Status == "answer_saved" {
		if err := s.refreshSavedRecoveryDecision(run, run.Decision.Request.QuestionRoute != nil, "服务重开后保留了旧答案；请重新确认后才投递。 "); err != nil {
			return err
		}
	}
	workflow, execution = s.workflowRecords(run)
	if result.completed {
		return s.finishWorkerFlowFor(run, nil)
	}
	if run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute == nil {
		// A prior connection failure is no longer a reason to request post-turn
		// feedback. Keep the resolved record before exposing native questions.
		s.setDecisionStatus(run, "superseded")
		if err := s.commitWorker(run); err != nil {
			return err
		}
		workflow, execution = s.workflowRecords(run)
		s.setRunDecision(run, nil)
	}
	execution.Status, execution.Error = "active", nil
	s.refreshRun(run)
	if run.Decision == nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved" {
		workflow.Phase = "executing"
		s.refreshRun(run)
	}
	if err := s.commitWorker(run); err != nil {
		return err
	}
	s.registerWorker(run, result.worker)
	return nil
}

func (s *Service) reconcileAppServerReplyFromJournal(run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord) error {
	if run.Decision == nil || run.Decision.Status != "answer_saved" || run.Decision.Request.QuestionRoute == nil || !stableExecutionID(execution.CurrentCommandID) {
		return nil
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	context := workerSessionContext(run, execution)
	ask, err := codexworker.DecodeQuestionRoute(*run.Decision.Request.QuestionRoute, *context)
	if err != nil {
		return err
	}
	path := filepath.Join(fixed.config.RunDir, "runs", run.RunID, "turns", execution.CurrentCommandID, "appserver.jsonl")
	confirmed, err := appServerAnswerEvidence(path, ask, run.Decision.Answers)
	if err != nil || !confirmed {
		return err
	}
	key := "worker-question-answer:" + execution.ExecutionID + ":" + run.Decision.ID
	var operation *OperationRecord
	for i := range s.domain.Operations {
		if s.domain.Operations[i].IdempotencyKey == key {
			operation = &s.domain.Operations[i]
			break
		}
	}
	if operation == nil || operation.Status != "in_flight" && operation.Status != "accepted" && operation.Status != "outcome_unknown" && operation.Status != "confirmed" {
		return errors.New("matching Codex tool result has no fixed reply operation")
	}
	if operation.Status != "confirmed" {
		if err := s.stageWorkerOperation(run, operation.OperationID, "confirmed", ask.CallID, mustJSON(map[string]any{"tool_item_id": ask.CallID, "recovered": true}), ""); err != nil {
			return err
		}
	}
	s.setDecisionStatus(run, "reply_accepted")
	if err := s.commitWorker(run); err != nil {
		return err
	}
	workflow, execution = s.workflowRecords(run)
	if len(execution.PendingQuestions) > 0 {
		next := execution.PendingQuestions[0]
		execution.PendingQuestions = execution.PendingQuestions[1:]
		decision, err := appServerAskDecision(run, next.Questions, next.Route)
		if err != nil {
			return err
		}
		s.setRunDecision(run, decision)
	} else if workflow.Phase == "waiting_decision" {
		if err := ContinueWorkerQuestion(workflow, execution, execution.CurrentTurnID); err != nil {
			return err
		}
		s.refreshRun(run)
	}
	return s.commitWorker(run)
}

func clonePendingQuestions(values []PendingQuestion) []PendingQuestion {
	copy := make([]PendingQuestion, len(values))
	for i := range values {
		copy[i] = PendingQuestion{Questions: cloneExecutionQuestions(values[i].Questions), Route: values[i].Route.Clone()}
	}
	return copy
}
