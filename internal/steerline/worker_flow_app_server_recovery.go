package steerline

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
)

func appServerAnswerEvidence(path string, ask codexAskCall, answers map[string][]string) (bool, error) {
	return codexworker.AnswerEvidence(path, ask.ThreadID, ask.TurnID, ask.CallID, answers)
}

// A crash can happen after a native request is journaled but before its
// Decision reaches SQLite. Replay only requests not completed or persisted.
func appServerPendingAskEvents(path string, run *Run, execution *ExecutionRecord, turnID string) ([]codexapp.Envelope, error) {
	journal, err := codexworker.ReadAskJournal(path, execution.SessionID, turnID, func(event codexapp.Envelope) error {
		_, err := parseCodexAskCall(event, execution.SessionID, turnID)
		return err
	})
	if err != nil {
		return nil, err
	}
	var replay []codexapp.Envelope
	for _, event := range journal.Calls {
		call, _ := parseCodexAskCall(event, execution.SessionID, turnID)
		if journal.Completed[call.CallID] || run.Decision != nil && run.Decision.Request.NativeAsk != nil && run.Decision.Request.NativeAsk.CallID == call.CallID {
			continue
		}
		known := false
		for _, pending := range execution.PendingAsks {
			if pending.CallID == call.CallID {
				known = true
				break
			}
		}
		if !known {
			replay = append(replay, event)
		}
	}
	return replay, nil
}

type appServerRecoveryResult struct {
	runID     string
	execution ExecutionRecord
	worker    *Worker
	completed bool
	err       error
}

func recoverAppServerExecution(ctx context.Context, config Config, run Run, execution ExecutionRecord) appServerRecoveryResult {
	timeout := positiveDuration(config.CommandTimeout, 30*time.Second)
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	worker, completed, err := reattachAppServerWorker(probeCtx, config, &run, &execution)
	return appServerRecoveryResult{runID: run.RunID, execution: execution, worker: worker, completed: completed, err: err}
}

// Probe every unsettled local Execution at once. Results are applied only by
// the service writer, so one slow or unavailable endpoint cannot serialize
// the identity checks for unrelated tasks.
func (s *Service) resumeAppServerWorkers(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	results := make(chan appServerRecoveryResult, len(s.State.Runs))
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
	for _, run := range s.State.Runs {
		if run == nil || run.WorkflowVersion != WorkerFlowVersion || run.Protocol != string(ProtocolLocalCodexAppServer) || s.workers[run.RunID] != nil || s.appServerLaunching[run.RunID] != "" || s.appServerRecovering[run.RunID] {
			continue
		}
		task := nativeTaskRecord(s.State.Tasks, run.DomainTaskID)
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
		executionCopy.PendingAsks = append([]codexAskCall(nil), execution.PendingAsks...)
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
	for _, candidate := range s.State.Runs {
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
	if execution.PendingCommandID != result.execution.PendingCommandID || execution.CurrentCommandID != result.execution.CurrentCommandID || execution.CurrentTurnID != result.execution.CurrentTurnID {
		return ErrWorkerTurnStale
	}
	if result.execution.Process != nil {
		if execution.Process != nil && *execution.Process != *result.execution.Process {
			return s.waitWorkerFlow(run, "Original app-server process identity changed during recovery")
		}
		execution.Process = result.execution.Process
		execution.AppServerEndpoint = result.execution.AppServerEndpoint
		execution.AppServerGeneration = result.execution.AppServerGeneration
		s.refreshRun(run)
	}
	if result.err != nil {
		execution.Status = "blocked"
		return s.waitWorkerFlow(run, "Original app-server turn cannot be reconciled: "+result.err.Error())
	}
	if execution.SessionID != "" && execution.SessionID != result.execution.SessionID {
		return s.waitWorkerFlow(run, "Original app-server thread identity changed during recovery")
	}
	execution.SessionID = result.execution.SessionID
	s.refreshRun(run)
	if execution.PendingCommandID != "" {
		execution.PendingTurnID = result.execution.PendingTurnID
		opID, err := s.recordAppServerTurnStarted(run, workflow, execution, execution.PendingTurnID)
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
	workflow, execution = s.workflowRecords(run)
	if result.completed {
		return s.finishWorkerFlowFor(run, nil)
	}
	if run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.NativeAsk == nil {
		// A prior connection failure is no longer a reason to request post-turn
		// feedback. Keep the resolved record before exposing native questions.
		s.setDecisionStatus(run, "superseded")
		if err := s.save(); err != nil {
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
	if err := s.save(); err != nil {
		return err
	}
	s.registerWorker(run, result.worker)
	return nil
}

func (s *Service) reconcileAppServerReplyFromJournal(run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord) error {
	if run.Decision == nil || run.Decision.Status != "answer_saved" || run.Decision.Request.NativeAsk == nil || !stableExecutionID(execution.CurrentCommandID) {
		return nil
	}
	ask := *run.Decision.Request.NativeAsk
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	path := filepath.Join(fixed.config.RunDir, "runs", run.RunID, "turns", execution.CurrentCommandID, "appserver.jsonl")
	confirmed, err := appServerAnswerEvidence(path, ask, run.Decision.Answers)
	if err != nil || !confirmed {
		return err
	}
	key := "appserver-answer:" + execution.ExecutionID + ":" + run.Decision.ID
	var operation *OperationRecord
	for i := range s.State.Operations {
		if s.State.Operations[i].IdempotencyKey == key {
			operation = &s.State.Operations[i]
			break
		}
	}
	if operation == nil || operation.Status != "in_flight" && operation.Status != "accepted" && operation.Status != "outcome_unknown" && operation.Status != "confirmed" {
		return errors.New("matching Codex tool result has no fixed reply operation")
	}
	if operation.Status != "confirmed" {
		if err := s.setOperationState(operation.OperationID, "confirmed", ask.CallID, mustJSON(map[string]any{"tool_item_id": ask.CallID, "recovered": true}), ""); err != nil {
			return err
		}
	}
	s.setDecisionStatus(run, "reply_accepted")
	if err := s.save(); err != nil {
		return err
	}
	workflow, execution = s.workflowRecords(run)
	if len(execution.PendingAsks) > 0 {
		next := execution.PendingAsks[0]
		execution.PendingAsks = execution.PendingAsks[1:]
		decision, err := appServerAskDecision(run, next)
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
	return s.save()
}
