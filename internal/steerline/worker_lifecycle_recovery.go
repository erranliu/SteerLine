package steerline

import (
	"context"
	"errors"
	"sync"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

// A crash can happen after a native request is journaled but before its
// Decision reaches SQLite. Replay only requests not completed or persisted.
type appServerRecoveryResult struct {
	runID     string
	attemptID string
	request   executionapi.LifecycleRequest
	receipt   executionapi.LifecycleReceipt
	execution ExecutionRecord
	worker    *Worker
	completed bool
	dead      bool
	err       error
}

func recoverAppServerExecution(ctx context.Context, lifecycle executionapi.LifecyclePort, request executionapi.LifecycleRequest, execution ExecutionRecord) appServerRecoveryResult {
	receipt, observer, err := lifecycle.Reconcile(ctx, request.Clone())
	result := appServerRecoveryResult{
		runID: request.Snapshot.Session.WorkflowRunID, attemptID: request.Snapshot.Session.AttemptID,
		request: request.Clone(), receipt: receipt.Clone(), execution: execution, completed: receipt.State.TurnCompleted,
		dead: receipt.State.WorkerAbsent, err: err,
	}
	if validationErr := receipt.ValidateFor(request); validationErr != nil && err == nil {
		result.err = validationErr
	}
	if mergeErr := mergeLifecycleExecutionReceipt(&result.execution, receipt); mergeErr != nil && result.err == nil {
		result.err = mergeErr
	}
	if observer != nil {
		result.worker = workerFromLifecycle(result.runID, result.attemptID, observer)
	}
	if result.err == nil && result.worker == nil && !result.dead && !result.completed &&
		(receipt.Disposition == executionapi.OperationUnknown || receipt.Disposition == executionapi.OperationRejected) {
		result.err = &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: receipt.Diagnostic}
	}
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
		if run == nil || run.WorkflowVersion != WorkerFlowVersion || s.workers[run.RunID] != nil || s.lifecycleEffectPending(run.RunID) || s.appServerRecovering[run.RunID] {
			continue
		}
		task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
		workflow, execution := s.workflowRecords(run)
		if task == nil || workflow == nil || execution == nil || s.candidateBusinessClosed(run) || execution.PendingCommandID == "" && (execution.CurrentTurnID == "" || execution.CurrentTurnID == execution.LastStoppedTurnID) {
			continue
		}
		lifecycle, fixed, err := s.lifecycleForRun(run)
		if err != nil {
			continue
		}
		if lifecycle == nil || lifecycle.PolicyCapabilities().ResumeActiveObservation {
			continue
		}
		request, requestErr := s.originalLifecycleRequest(run, fixed)
		if requestErr != nil {
			if err := s.waitWorkerFlow(run, "Original app-server operation cannot be reconciled: "+requestErr.Error()); err != nil {
				return err
			}
			continue
		}
		runID, executionCopy := run.RunID, *execution
		executionCopy.PendingQuestions = clonePendingQuestions(execution.PendingQuestions)
		if s.State.Service.Ready && s.workerMessages != nil {
			if s.appServerRecovering == nil {
				s.appServerRecovering = make(map[string]bool)
			}
			s.appServerRecovering[run.RunID] = true
			s.forwarders.Add(1)
			go func() {
				defer s.forwarders.Done()
				result := recoverAppServerExecution(s.runContext, lifecycle, request, executionCopy)
				select {
				case s.workerMessages <- workerMessage{runID: runID, recovery: &result}:
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
			results <- recoverAppServerExecution(ctx, lifecycle, request, executionCopy)
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
	_, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || execution.ExecutionID != result.execution.ExecutionID {
		return errors.New("app-server recovery changed the fixed execution")
	}
	if err := result.receipt.ValidateFor(result.request); err != nil {
		return err
	}
	if err := s.validateLifecycleResultBinding(run, result.request); err != nil {
		return nil // A late read-only probe cannot overwrite a committed binding.
	}
	if result.request.Snapshot.Session.ExecutionID != execution.ExecutionID || result.request.Snapshot.ResourceGeneration != execution.Generation ||
		result.attemptID != "" && result.attemptID != run.AttemptID || execution.RecoveryVersion != result.execution.RecoveryVersion ||
		execution.PendingCommandID != result.execution.PendingCommandID || execution.CurrentCommandID != result.execution.CurrentCommandID || execution.CurrentTurnID != result.execution.CurrentTurnID {
		return nil // A late probe cannot change a new Attempt or stop the service.
	}
	if err := mergeLifecycleExecutionReceipt(execution, result.receipt); err != nil {
		return s.waitWorkerFlow(run, "Original app-server binding failed verification")
	}
	s.refreshRun(run)
	if result.dead {
		if owned := s.ownedProcesses[run.RunID]; owned != nil {
			if owned.Identity() != *execution.Process {
				return s.waitWorkerFlow(run, "Original managed process identity changed during recovery")
			}
			owned.Close()
			delete(s.ownedProcesses, run.RunID)
		}
		if !workerTreeAbsent(*execution.Process) {
			execution.RecoveryState = string(workerflow.RecoveryUnverified)
			return s.waitWorkerFlow(run, "Original worker descendants may still write to the worktree")
		}
		execution.RecoveryState = string(workerflow.ClassifyRecovery(workerflow.RecoveryFacts{WorkerAbsent: true, WritersAbsent: true, Completed: result.completed}))
		if result.completed {
			if execution.SessionID == "" || execution.SessionGeneration == "" {
				return s.waitWorkerFlow(run, "Original app-server session identity could not be committed")
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
			return s.refreshSavedRecoveryDecision(run, false, "原 worker 已退出，旧请求已失效。请确认带入新轮次的输入。")
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
		if err := s.waitWorkerFlow(run, "原 worker 已退出，原轮次没有可信完成结果。请核对当前工作区后手动继续。\n\n原问题："+priorQuestion); err != nil {
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
	if execution.SessionID != "" && execution.SessionID != result.execution.SessionID {
		return s.waitWorkerFlow(run, "Original app-server thread identity changed during recovery")
	}
	execution.SessionID, execution.SessionGeneration = result.execution.SessionID, result.execution.SessionGeneration
	for _, envelope := range result.execution.ImplementationData {
		if err := putImplementationEnvelope(execution, envelope); err != nil {
			return s.waitWorkerFlow(run, "Original app-server binding could not be committed")
		}
	}
	if result.execution.Process != nil {
		process := *result.execution.Process
		execution.Process = &process
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
	if err := s.reconcileSavedLifecycleQuestionAnswer(context.Background(), run); err != nil {
		return s.waitWorkerFlow(run, "Original worker answer cannot be reconciled: "+err.Error())
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

func clonePendingQuestions(values []PendingQuestion) []PendingQuestion {
	copy := make([]PendingQuestion, len(values))
	for i := range values {
		copy[i] = PendingQuestion{Questions: cloneExecutionQuestions(values[i].Questions), Route: values[i].Route.Clone()}
	}
	return copy
}
