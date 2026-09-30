package steerline

import (
	"context"
	"errors"
	"fmt"
)

// workerFlowAdapter is selected from the persisted execution protocol.
// Its methods run on the service writer; background observers only send facts.
type workerFlowAdapter interface {
	prepare(context.Context, TaskRecord, *Run) error
	resume(context.Context, *TaskRecord, *Run) (bool, error)
	preparePrompt(*Run, string) string
	stageIntent(*ExecutionRecord, string)
	startTurn(context.Context, *Run, string, string, string) error
	sendFeedback(context.Context, *Run, *WorkflowRunRecord, *ExecutionRecord, DecisionAnswer, string) error
}

type codexAppWorkerAdapter struct{ service *Service }
type nativeOrcaWorkerAdapter struct{ service *Service }
type supervisedOrcaWorkerAdapter struct{ service *Service }

func (a codexAppWorkerAdapter) prepare(ctx context.Context, task TaskRecord, run *Run) error {
	fixed, err := a.service.workerFlowContext(run)
	if err != nil {
		return err
	}
	if err := requireWorkerFlowCapabilities(ctx, fixed.config); err != nil {
		return err
	}
	return a.service.prepareWorkerWorkspace(ctx, task, run, false)
}

func (a nativeOrcaWorkerAdapter) prepare(ctx context.Context, task TaskRecord, run *Run) error {
	if err := a.service.prepareNativeOrcaWorker(ctx, run); err != nil {
		return a.service.nativeOrcaUncertain(run, "Native Orca worker preparation needs reconciliation: "+err.Error())
	}
	return a.service.startInitialWorkerFlowTurn(ctx, task, run)
}

func (a supervisedOrcaWorkerAdapter) prepare(ctx context.Context, task TaskRecord, run *Run) error {
	return a.service.prepareWorkerWorkspace(ctx, task, run, true)
}

func (s *Service) adapterForWorkerFlow(run *Run) (workerFlowAdapter, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return nil, err
	}
	switch {
	case run.Protocol == string(ProtocolLocalCodexAppServer) && fixed.config.hostName() == "local":
		return codexAppWorkerAdapter{service: s}, nil
	case run.Protocol == string(ProtocolOrcaTerminalV2) && fixed.config.hostName() == "orca":
		return nativeOrcaWorkerAdapter{service: s}, nil
	case run.Protocol == string(ProtocolOrcaSupervised) && fixed.config.hostName() == "orca_supervised":
		return supervisedOrcaWorkerAdapter{service: s}, nil
	default:
		return nil, nativeErr("unsupported_schema", "fixed worker protocol does not match its execution host")
	}
}

func (codexAppWorkerAdapter) preparePrompt(run *Run, prompt string) string {
	return prompt + localWorkerPublishInstruction(run.RunID)
}
func (nativeOrcaWorkerAdapter) preparePrompt(_ *Run, prompt string) string     { return prompt }
func (supervisedOrcaWorkerAdapter) preparePrompt(_ *Run, prompt string) string { return prompt }

func (codexAppWorkerAdapter) stageIntent(execution *ExecutionRecord, commandID string) {
	execution.PendingTurnID = "" // A Command ID is not a native turn receipt.
	execution.PendingCommandID = commandID
}
func (nativeOrcaWorkerAdapter) stageIntent(execution *ExecutionRecord, turnID string) {
	execution.PendingTurnID = turnID
}
func (supervisedOrcaWorkerAdapter) stageIntent(execution *ExecutionRecord, turnID string) {
	execution.PendingTurnID = turnID
}

func (a codexAppWorkerAdapter) startTurn(ctx context.Context, run *Run, commandID, prompt, opID string) error {
	s := a.service
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	if s.workerMessages != nil {
		copyRun, copyExecution, config := *run, *fixed.execution, fixed.config
		if s.appServerLaunching == nil {
			s.appServerLaunching = make(map[string]string)
		}
		s.appServerLaunching[run.RunID] = commandID
		s.forwarders.Add(1)
		go func() {
			defer s.forwarders.Done()
			s.launchAppServerTurn(s.runContext, config, copyRun, copyExecution, commandID, prompt, opID)
		}()
		return nil
	}
	persistThread := func() error {
		execution := fixed.execution
		if execution.Process == nil || execution.AppServerGeneration == "" {
			return errors.New("app-server thread has no verified process generation")
		}
		ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendLocal, Protocol: ProtocolLocalCodexAppServer, Target: targetFor(fixed.config), Agent: agentFor(fixed.config), RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: deref(run.WorktreePath), WorkspaceBranch: deref(run.WorktreeBranch), BaseCommit: deref(run.BaseCommit), External: *execution.Process, Generation: execution.AppServerGeneration}
		if err := (executionRecords{root: fixed.config.RunDir}).saveBinding(ref); err != nil {
			return err
		}
		return s.save()
	}
	worker, events, platformRequestID, err := startAppServerWorkerTurn(ctx, fixed.config, run, fixed.execution, commandID, prompt, persistThread)
	if err != nil {
		return s.failStartedWorkerTurn(run, opID, platformRequestID, err)
	}
	return s.acceptStartedWorkerTurn(run, worker, events, platformRequestID, opID, commandID, true)
}

func (a nativeOrcaWorkerAdapter) startTurn(ctx context.Context, run *Run, turnID, prompt, opID string) error {
	s := a.service
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	if err := s.startNativeOrcaTurn(ctx, run, fixed.execution, turnID, prompt); err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		fixed.execution.Status = "blocked"
		s.refreshRun(run)
		return s.nativeOrcaUncertain(run, "Native Orca Dispatch needs reconciliation: "+err.Error())
	}
	fixed.attempt.Status, fixed.attempt.UpdatedAt = "dispatched", now()
	fixed.execution.Status = "pending"
	s.refreshRun(run)
	current := nativeOrcaCurrentTurn(fixed.execution)
	if current == nil {
		return errors.New("native Orca turn has no accepted Dispatch identity")
	}
	if err := s.changeOperation(opID, "accepted", current.DispatchRequestID, mustJSON(*current), ""); err != nil {
		return err
	}
	s.note(run, "steer", "worker", "accepted", "native Orca Dispatch input accepted", current.DispatchID, map[string]any{"task_id": current.TaskID, "turn_id": turnID})
	return s.save()
}

func (a supervisedOrcaWorkerAdapter) startTurn(ctx context.Context, run *Run, turnID, prompt, opID string) error {
	s := a.service
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	worker, events, platformRequestID, err := startWorkerOrcaTurn(ctx, fixed.config, run, fixed.execution, turnID, prompt)
	if err != nil {
		return s.failStartedWorkerTurn(run, opID, platformRequestID, err)
	}
	return s.acceptStartedWorkerTurn(run, worker, events, platformRequestID, opID, turnID, false)
}

func (s *Service) failStartedWorkerTurn(run *Run, opID, platformRequestID string, startErr error) error {
	if err := s.setOperationState(opID, "outcome_unknown", platformRequestID, nil, startErr.Error()); err != nil {
		return err
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	fixed.execution.Status = "blocked"
	s.refreshRun(run)
	return s.waitWorkerFlow(run, "Original worker turn start is unverified: "+startErr.Error())
}

func (s *Service) acceptStartedWorkerTurn(run *Run, worker *Worker, events, platformRequestID, opID, commandID string, local bool) error {
	if worker == nil {
		return errors.New("accepted worker turn has no observer")
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	execution, attempt := fixed.execution, fixed.attempt
	if local {
		execution.PendingTurnID = platformRequestID
	}
	identity := worker.proc.Identity()
	execution.Process = &identity
	execution.Status = "pending"
	s.refreshRun(run)
	if attempt.EventPath == "" {
		attempt.EventPath = events
		execution.LogPath = ptr(events)
		s.refreshRun(run)
	}
	attempt.Status, attempt.UpdatedAt = "dispatched", now()
	if err := s.setOperationState(opID, "accepted", platformRequestID, mustJSON(identity), ""); err != nil {
		return err
	}
	if err := s.save(); err != nil {
		return fmt.Errorf("worker started but its exact process receipt could not be saved: %w", err)
	}
	s.registerWorker(run, worker)
	s.note(run, "steer", "worker", "accepted", "original worker turn accepted", commandID, map[string]any{"session_id": execution.SessionID, "turn_number": execution.TurnNumber + 1})
	return nil
}

func (a nativeOrcaWorkerAdapter) sendFeedback(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, answer DecisionAnswer, commandID string) error {
	return a.service.applyNativeOrcaDecision(ctx, run, workflow, execution, answer, commandID)
}
func (a codexAppWorkerAdapter) sendFeedback(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, answer DecisionAnswer, commandID string) error {
	return a.service.sendOriginalWorkerFeedback(ctx, run, workflow, execution, answer, commandID)
}
func (a supervisedOrcaWorkerAdapter) sendFeedback(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, answer DecisionAnswer, commandID string) error {
	return a.service.sendOriginalWorkerFeedback(ctx, run, workflow, execution, answer, commandID)
}

func (s *Service) sendOriginalWorkerFeedback(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, answer DecisionAnswer, commandID string) error {
	if execution.SessionID == "" || run.SessionID == nil || *run.SessionID != execution.SessionID {
		execution.Error = ptr("Feedback is saved, but the original worker session is not yet available for delivery")
		s.refreshRun(run)
		return s.save()
	}
	if run.Decision.Request.NativeAsk != nil {
		if s.workerMessages != nil && s.workers[run.RunID] == nil {
			return nil // Recovery must attach the original observer before replying.
		}
		return s.replyAppServerAsk(ctx, run, execution, answer)
	}
	if execution.Status == "active" || s.workers[run.RunID] != nil {
		return nil // Preserve the answer until the original turn stops.
	}
	for _, operation := range s.State.Operations {
		if operation.IdempotencyKey != "worker-turn:"+execution.ExecutionID+":"+commandID {
			continue
		}
		if operation.Status == "confirmed" || operation.Status == "accepted" || operation.Status == "in_flight" {
			return nil // The original request may have started. Never resend.
		}
		execution.Error = ptr("Feedback delivery remains unverified at the original request ID; no duplicate message was sent")
		s.refreshRun(run)
		return s.save()
	}
	return s.startWorkerFlowTurn(ctx, run, workerFeedbackText(answer, run.Decision.Request), commandID)
}
