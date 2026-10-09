package execution

import (
	"context"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"sync"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// A crash can happen after a native request is journaled but before its
// Decision reaches SQLite. Replay only requests not completed or persisted.
type workerRecoveryResult struct {
	runID     string
	attemptID string
	request   executionbackend.LifecycleRequest
	receipt   executionbackend.LifecycleReceipt
	execution executionmodel.ExecutionRecord
	worker    *Worker
	completed bool
	dead      bool
	err       error
}

func recoverWorkerExecution(ctx context.Context, lifecycle executionbackend.LifecyclePort, request executionbackend.LifecycleRequest, execution executionmodel.ExecutionRecord) workerRecoveryResult {
	receipt, observer, err := lifecycle.Reconcile(ctx, request.Clone())
	result := workerRecoveryResult{
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
		(receipt.Disposition == executionbackend.OperationUnknown || receipt.Disposition == executionbackend.OperationRejected) {
		result.err = &executionbackend.BackendError{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: receipt.Diagnostic}
	}
	return result
}

// Probe every unsettled local Execution at once. Results are applied only by
// the service writer, so one slow or unavailable endpoint cannot serialize
// the identity checks for unrelated tasks.
func (s *Service) resumeWorkerObservers(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	results := make(chan workerRecoveryResult, len(s.runs))
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
		if run == nil || run.WorkflowVersion != executionmodel.Version || s.workers[run.RunID] != nil || s.lifecycleEffectPending(run.RunID) || s.workerRecovering[run.RunID] {
			continue
		}
		task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
		workflow, execution := s.workflowRecords(run)
		if task == nil || workflow == nil || execution == nil || s.candidateBusinessClosed(run) || execution.PendingCommandID == "" && (execution.CurrentTurnID == "" || execution.CurrentTurnID == execution.LastStoppedTurnID) {
			continue
		}
		lifecycle, fixed, err := s.lifecycleForRun(run)
		// The next frozen flow input is committed before any native operation.
		// With no original operation there is no external outcome to reconcile.
		// Same-instance dispatch consumes this queue; restart retains its manual
		// recovery gate and cannot automatically submit the saved input.
		if execution.PendingCommandID != "" && stageInput(workflow, execution.PendingCommandID) != nil &&
			s.operationByKey("worker-turn:"+execution.ExecutionID+":"+execution.PendingCommandID) == nil &&
			s.operationByKey("worker-session:"+execution.ExecutionID+":"+execution.PendingCommandID) == nil {
			continue
		}
		if err != nil {
			continue
		}
		if lifecycle == nil || lifecycle.PolicyCapabilities().ResumeActiveObservation {
			continue
		}
		if handled, err := s.reconcileWorkerStops(ctx, run, lifecycle); err != nil {
			return err
		} else if handled {
			continue
		}
		request, requestErr := s.originalLifecycleRequest(run, fixed)
		if requestErr != nil {
			if err := s.waitWorkerFlow(run, "Original worker operation cannot be reconciled: "+requestErr.Error()); err != nil {
				return err
			}
			continue
		}
		runID, executionCopy := run.RunID, *execution
		executionCopy.PendingQuestions = clonePendingQuestions(execution.PendingQuestions)
		if s.State.Service.Ready && s.signalQueue != nil {
			if s.workerRecovering == nil {
				s.workerRecovering = make(map[string]bool)
			}
			s.workerRecovering[run.RunID] = true
			s.forwarders.Add(1)
			go func() {
				defer s.forwarders.Done()
				result := recoverWorkerExecution(s.runContext, lifecycle, request, executionCopy)
				select {
				case s.signalQueue <- Signal{runID: runID, recovery: &result}:
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
			results <- recoverWorkerExecution(ctx, lifecycle, request, executionCopy)
		}()
	}
	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message := <-s.signalQueue:
			if err := s.handleWorkerMessage(message); err != nil {
				return err
			}
			i--
		case result := <-results:
			if err := s.applyWorkerRecovery(result); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) applyWorkerRecovery(result workerRecoveryResult) error {
	defer func() {
		if result.worker != nil && s.workers[result.runID] != result.worker {
			_ = result.worker.detachLocalObservation(context.Background())
		}
	}()
	if result.err != nil && s.runContext != nil && s.runContext.Err() != nil {
		return nil
	}

	var run *reconstructiontypes.RunView
	for _, candidate := range s.runs {
		if candidate != nil && candidate.RunID == result.runID {
			run = candidate
			break
		}
	}
	if run == nil {
		return errors.New("worker recovery lost its fixed workflow run")
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
		return errors.New("worker recovery changed the fixed execution")
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
		return s.waitWorkerFlow(run, "Original worker binding failed verification")
	}
	// Persist proof of the original input before aggregating uncertainty. Tree
	// exit alone never confirms a start/send; native acceptance/completion can.
	if result.receipt.Disposition == executionbackend.OperationConfirmed && (result.receipt.State.InputAccepted || result.receipt.State.TurnStarted || result.receipt.State.TurnCompleted) {
		if op := s.operationByKey(result.request.Operation.IdempotencyKey); op != nil && op.Status != "confirmed" {
			if err := s.setOperationState(op.OperationID, "confirmed", result.receipt.State.TurnID, mustJSON(result.receipt), result.receipt.Diagnostic); err != nil {
				return err
			}
			if err := s.commitWorker(run, op.OperationID); err != nil {
				return err
			}
		}
	}
	s.refreshRun(run)
	if result.dead {
		if !result.receipt.State.WritersAbsent {
			execution.RecoveryState = string(executionmodel.RecoveryUnverified)
			return s.waitWorkerFlow(run, "Original worker writers may still modify the worktree")
		}
		s.setWorkerRecoveryFacts(run, executionmodel.RecoveryFacts{WorkerAbsent: true, WritersAbsent: true, Completed: result.completed}, result.receipt.State.SessionAvailable)
		if execution.RecoveryState == string(executionmodel.RecoveryUnverified) {
			execution.Status = "blocked"
			return s.waitWorkerFlow(run, "An earlier external operation has an unknown outcome; reconcile its original request before continuing")
		}
		if result.completed {
			if execution.SessionID == "" || execution.SessionGeneration == "" {
				return s.waitWorkerFlow(run, "Original worker session identity could not be committed")
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
		execution.RecoveryState = string(executionmodel.RecoveryUnverified)
		execution.Status = "blocked"
		return s.waitWorkerFlow(run, "Original worker turn cannot be reconciled: "+result.err.Error())
	}
	if execution.SessionID != "" && execution.SessionID != result.execution.SessionID {
		return s.waitWorkerFlow(run, "Original worker session identity changed during recovery")
	}
	execution.SessionID, execution.SessionGeneration = result.execution.SessionID, result.execution.SessionGeneration
	for _, envelope := range result.execution.ImplementationData {
		if err := putImplementationEnvelope(execution, envelope); err != nil {
			return s.waitWorkerFlow(run, "Original worker binding could not be committed")
		}
	}
	if result.execution.Process != nil {
		process := *result.execution.Process
		execution.Process = &process
	}
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
	s.setWorkerRecoveryFacts(run, executionmodel.RecoveryFacts{WorkerLive: true, QuestionPending: run.Decision != nil && run.Decision.Request.QuestionRoute != nil}, result.receipt.State.SessionAvailable)
	s.refreshRun(run)
	if run.Decision != nil && run.Decision.Status == "answer_saved" {
		if err := s.refreshSavedRecoveryDecision(run, run.Decision.Request.QuestionRoute != nil, "服务重开后保留了旧答案；请重新确认后才投递。 "); err != nil {
			return err
		}
	}
	workflow, execution = s.workflowRecords(run)
	if result.completed {
		return s.finishWorkerFlowFor(run, nil)
	}
	if execution.RecoveryState == "unverified" {
		execution.Status = "blocked"
		if err := s.waitWorkerFlow(run, "An earlier external operation has an unknown outcome; observation continues without sending input"); err != nil {
			return err
		}
		s.registerWorker(run, result.worker)
		return nil
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
		workflow.Status = "running"
		s.refreshRun(run)
	}
	if err := s.commitWorker(run); err != nil {
		return err
	}
	s.registerWorker(run, result.worker)
	return nil
}

func clonePendingQuestions(values []executionmodel.PendingQuestion) []executionmodel.PendingQuestion {
	copy := make([]executionmodel.PendingQuestion, len(values))
	for i := range values {
		copy[i] = executionmodel.PendingQuestion{Questions: cloneExecutionQuestions(values[i].Questions), Route: values[i].Route.Clone()}
	}
	return copy
}
