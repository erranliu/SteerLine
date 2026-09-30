package steerline

import (
	"context"
	"errors"
	"fmt"

	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

// The service is the only writer. Mode selection carries no Service reference.
func (s *Service) workerFlowMode(run *Run) (workerflow.Mode, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return 0, err
	}
	mode, err := workerflow.SelectMode(run.Protocol, fixed.config.hostName())
	if err != nil {
		return 0, nativeErr("unsupported_schema", err.Error())
	}
	return mode, nil
}

func (s *Service) prepareWorkerFlowByMode(ctx context.Context, task TaskRecord, run *Run, mode workerflow.Mode) error {
	switch mode {
	case workerflow.LocalCodex:
		return s.prepareCodexWorker(ctx, task, run)
	case workerflow.NativeOrca:
		return s.prepareNativeOrcaWorkerFlow(ctx, task, run)
	case workerflow.SupervisedOrca:
		return s.prepareSupervisedOrcaWorker(ctx, task, run)
	default:
		return nativeErr("unsupported_schema", "unknown worker mode")
	}
}

func (s *Service) startWorkerTurnByMode(ctx context.Context, run *Run, turnID, prompt, opID string, mode workerflow.Mode) error {
	switch mode {
	case workerflow.LocalCodex:
		return s.startCodexWorkerTurn(ctx, run, turnID, prompt, opID)
	case workerflow.NativeOrca:
		return s.startNativeOrcaWorkerTurn(ctx, run, turnID, prompt, opID)
	case workerflow.SupervisedOrca:
		return s.startSupervisedOrcaWorkerTurn(ctx, run, turnID, prompt, opID)
	default:
		return nativeErr("unsupported_schema", "unknown worker mode")
	}
}

func (s *Service) prepareCodexWorker(ctx context.Context, task TaskRecord, run *Run) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	if err := requireWorkerFlowCapabilities(ctx, fixed.config); err != nil {
		return err
	}
	return s.prepareWorkerWorkspace(ctx, task, run, false)
}

func (s *Service) prepareNativeOrcaWorkerFlow(ctx context.Context, task TaskRecord, run *Run) error {
	if err := s.prepareNativeOrcaWorker(ctx, run); err != nil {
		return s.nativeOrcaUncertain(run, "Native Orca worker preparation needs reconciliation: "+err.Error())
	}
	return s.startInitialWorkerFlowTurn(ctx, task, run)
}

func (s *Service) prepareSupervisedOrcaWorker(ctx context.Context, task TaskRecord, run *Run) error {
	return s.prepareWorkerWorkspace(ctx, task, run, true)
}

func (s *Service) startCodexWorkerTurn(ctx context.Context, run *Run, commandID, prompt, opID string) error {
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

func (s *Service) startNativeOrcaWorkerTurn(ctx context.Context, run *Run, turnID, prompt, opID string) error {
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

func (s *Service) startSupervisedOrcaWorkerTurn(ctx context.Context, run *Run, turnID, prompt, opID string) error {
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
