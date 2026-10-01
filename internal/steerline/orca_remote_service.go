package steerline

import (
	"context"
	"encoding/json"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func remoteExecutionRef(run *Run, execution *ExecutionRecord) executionapi.ExecutionRef {
	ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaSupervised, Agent: executionapi.AgentSpec{Name: execution.Agent}, RunID: run.RunID, AttemptID: run.AttemptID, Target: execution.Target, WorkspacePath: execution.Workspace.Worktree, WorkspaceBranch: execution.Workspace.Branch, BaseCommit: execution.Workspace.Base}
	if execution.Process != nil {
		ref.External = *execution.Process
	}
	return ref
}

func remoteClientForExecution(c Config, execution *ExecutionRecord) remote.Client {
	cfg := remoteWorkerConfig(c)
	if execution.OrcaNative != nil {
		cfg.ExpectedRemoteRuntime = execution.OrcaNative.RemoteRuntimeID
	}
	if execution.OrcaNative != nil && execution.OrcaNative.Coordinator.Terminal != "" {
		cfg.Settings.CoordinatorTerminal = execution.OrcaNative.Coordinator.Terminal
	}
	return remote.New(cfg)
}

func (s *Service) prepareRemoteOrcaCoordinator(ctx context.Context, run *Run) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	execution := fixed.execution
	pair, err := probeSupervisedOrcaTarget(ctx, fixed.config)
	if err != nil {
		return err
	}
	if execution.OrcaNative == nil {
		execution.OrcaNative = &OrcaNativeExecution{}
	}
	state := execution.OrcaNative
	if state.RemoteRuntimeID != "" && state.RemoteRuntimeID != pair.RemoteRuntime {
		return errors.New("original remote runtime changed")
	}
	state.RemoteRuntimeID = pair.RemoteRuntime
	client := remoteWorkerClient(fixed.config)
	if state.Coordinator.Terminal == "" {
		key := "orca-remote-coordinator:" + execution.ExecutionID
		op := s.operationByKey(key)
		handle := ""
		if op != nil {
			handle, err = client.FindCoordinator(ctx, "SteerLine coordinator "+run.RunID, pair.CenterRuntime)
			if err != nil {
				return err
			}
			if handle == "" {
				return errors.New("original remote coordinator creation has no unique terminal; no replacement created")
			}
		} else {
			if execution.RecoveryRequired {
				return errors.New("remote coordinator has no original creation intent")
			}
			id, intentErr := s.startWorkerOperation(run, "orca_remote_coordinator", key, map[string]string{"execution_id": execution.ExecutionID, "runtime": pair.CenterRuntime})
			if intentErr != nil {
				return intentErr
			}
			op = s.operationByKey(key)
			handle, err = client.CreateCoordinator(ctx, run.RunID, pair.CenterRuntime)
			if err != nil {
				_ = s.changeWorkerOperation(run, id, "outcome_unknown", "", nil, err.Error())
				return remoteWorkerError(err)
			}
		}
		identity, err := client.ShowCoordinator(ctx, handle, pair.CenterRuntime)
		if err != nil {
			return remoteWorkerError(err)
		}
		state.Coordinator = executionapi.ProcessIdentity(identity)
		execution.Target.CoordinatorTerminal = handle
		s.refreshRun(run)
		if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(state.Coordinator), ""); err != nil {
			return err
		}
	}
	if state.Coordinator.RuntimeID != pair.CenterRuntime {
		return errors.New("original remote coordinator runtime changed")
	}
	if execution.Target.OrchestrationRun == "" {
		client = remoteClientForExecution(fixed.config, execution)
		key := "orca-remote-run:" + execution.ExecutionID
		objective := "SteerLine worker " + execution.ExecutionID
		op := s.operationByKey(key)
		var receipt orcaworker.MutationReceipt
		if op != nil {
			id, err := client.FindRun(ctx, objective, state.Coordinator.RuntimeID, state.Coordinator.Terminal)
			if err != nil {
				return err
			}
			receipt = orcaworker.MutationReceipt{ID: id, RequestID: op.PlatformRequestID, Payload: mustJSON(map[string]string{"run_id": id, "proof": "original_coordinator_objective"})}
		} else {
			if execution.RecoveryRequired {
				return errors.New("remote Run has no original creation intent")
			}
			id, err := s.startWorkerOperation(run, "orca_remote_run", key, map[string]string{"execution_id": execution.ExecutionID, "objective": objective, "coordinator": state.Coordinator.Terminal})
			if err != nil {
				return err
			}
			op = s.operationByKey(key)
			receipt, err = orcaworker.CreateRun(ctx, client.CenterCall, objective, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
			if err != nil {
				_ = s.changeWorkerOperation(run, id, "outcome_unknown", "", nil, err.Error())
				return err
			}
		}
		execution.Target.OrchestrationRun, execution.Target.OrchestrationRunRequestID = receipt.ID, receipt.RequestID
		s.refreshRun(run)
		return s.changeWorkerOperation(run, op.OperationID, "confirmed", receipt.RequestID, receipt.Payload, "")
	}
	return nil
}

func (s *Service) startRemoteOrcaTurn(ctx context.Context, run *Run, execution *ExecutionRecord, turnID, prompt string) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	if execution.OrcaNative == nil || execution.OrcaNative.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		return errors.New("remote worker has no prepared coordinator/Run")
	}
	client := remoteClientForExecution(fixed.config, execution)
	state := execution.OrcaNative
	var turn *OrcaNativeTurn
	for i := range state.Turns {
		if state.Turns[i].TurnID == turnID {
			turn = &state.Turns[i]
			break
		}
	}
	if turn != nil && turn.DispatchID != "" {
		return s.verifyRemoteOrcaWorker(ctx, run, execution)
	}
	key := "orca-remote-task:" + execution.ExecutionID + ":" + turnID
	if turn == nil {
		op := s.operationByKey(key)
		var receipt orcaworker.MutationReceipt
		if op != nil {
			id, err := client.FindTask(ctx, execution.Target.OrchestrationRun, prompt, state.Coordinator.RuntimeID)
			if err != nil {
				return err
			}
			if id == "" {
				return errors.New("original remote Task is unverified; no duplicate Task created")
			}
			receipt = orcaworker.MutationReceipt{ID: id, RequestID: op.PlatformRequestID}
		} else {
			id, err := s.startWorkerOperation(run, "orca_remote_task", key, map[string]string{"execution_id": execution.ExecutionID, "turn_id": turnID, "spec": prompt})
			if err != nil {
				return err
			}
			op = s.operationByKey(key)
			receipt, err = orcaworker.CreateTask(ctx, client.CenterCall, prompt, run.Title, execution.Target.OrchestrationRun, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
			if err != nil {
				_ = s.changeWorkerOperation(run, id, "outcome_unknown", "", nil, err.Error())
				return err
			}
		}
		state.Turns = append(state.Turns, OrcaNativeTurn{Purpose: "business", IntentAt: now(), TurnID: turnID, TaskID: receipt.ID, TaskRequestID: receipt.RequestID})
		turn = &state.Turns[len(state.Turns)-1]
		if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", receipt.RequestID, mustJSON(*turn), ""); err != nil {
			return err
		}
	}
	key = "orca-remote-start:" + execution.ExecutionID + ":" + turnID
	op := s.operationByKey(key)
	var started remote.StartReceipt
	if op != nil {
		started, err = client.RecoverStart(ctx, execution.Target.OrchestrationRun, turn.TaskID, execution.Target.WorktreeID, state.Coordinator.RuntimeID)
	} else {
		id, intentErr := s.startWorkerOperation(run, "orca_remote_start", key, map[string]string{"execution_id": execution.ExecutionID, "task_id": turn.TaskID, "worktree_id": execution.Target.WorktreeID, "session_id": execution.SessionID})
		if intentErr != nil {
			return intentErr
		}
		op = s.operationByKey(key)
		var previous *executionapi.ProcessIdentity
		if execution.Process != nil {
			identity := executionapi.ProcessIdentity(*execution.Process)
			previous = &identity
		}
		started, err = client.StartTask(ctx, execution.Target.OrchestrationRun, turn.TaskID, execution.Target.WorktreeID, previous)
		if err != nil {
			_ = s.changeWorkerOperation(run, id, "outcome_unknown", started.PlatformRequestID, mustJSON(started), err.Error())
			return remoteWorkerError(err)
		}
	}
	if err != nil {
		return remoteWorkerError(err)
	}
	identity, err := client.BindStart(ctx, started, execution.Process, execution.Workspace.Worktree, execution.Workspace.Branch)
	if err != nil {
		return remoteWorkerError(err)
	}
	firstBinding := execution.Process == nil
	execution.Process = &identity
	turn.DispatchID, turn.DispatchRequestID = started.DispatchID, started.PlatformRequestID
	execution.Target.ExecutionHost = identity.ExecutionHost
	s.bindNativeOrcaDispatch(run, execution, turn)
	s.refreshRun(run)
	if err := s.setOperationState(op.OperationID, "accepted", started.PlatformRequestID, mustJSON(started), ""); err != nil {
		return err
	}
	// The immutable attempt binding records its original session. Later Dispatches
	// are written with their turn Operations and provider state in the same commit.
	if firstBinding {
		ref := remoteExecutionRef(run, execution)
		return s.commitWorkerAttached(run, workerCommitAttachment{Binding: &ref}, op.OperationID)
	}
	return s.commitWorker(run, op.OperationID)
}

func (s *Service) verifyRemoteOrcaWorker(ctx context.Context, run *Run, execution *ExecutionRecord) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	if execution.Process == nil || execution.Process.Kind != "orca_supervised" {
		return errors.New("original remote agent binding is incomplete")
	}
	client := remoteClientForExecution(fixed.config, execution)
	ref := remoteWorkerRef(remoteExecutionRef(run, execution))
	return remoteWorkerError(client.VerifyWorker(ctx, ref))
}

func (s *Service) resumeRemoteOrcaWorker(ctx context.Context, task *TaskRecord, run *Run) (bool, error) {
	workflow, execution := s.workflowRecords(run)
	if execution.Workspace.Worktree == "" {
		op := s.operationByKey("worker-workspace:" + execution.ExecutionID)
		if op == nil {
			if execution.RecoveryRequired {
				return true, s.waitWorkerFlow(run, "Remote workspace preparation has no original intent")
			}
			return true, s.prepareWorkerFlow(ctx, *task, run)
		}
		if err := s.recoverRemoteWorkerWorkspace(ctx, run, execution, op); err != nil {
			return true, s.nativeOrcaUncertain(run, err.Error())
		}
	}
	if execution.OrcaNative == nil || execution.OrcaNative.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		if err := s.prepareRemoteOrcaCoordinator(ctx, run); err != nil {
			return true, s.nativeOrcaUncertain(run, err.Error())
		}
	}
	turn := nativeOrcaCurrentTurn(execution)
	if execution.PendingTurnID != "" && (turn == nil || turn.DispatchID == "") {
		if execution.RecoveryRequired && s.operationByKey("orca-remote-start:"+execution.ExecutionID+":"+execution.PendingTurnID) == nil {
			return true, s.waitWorkerFlow(run, "Remote start was never confirmed; restart does not issue the original input")
		}
		op := s.operationByKey("worker-turn:" + execution.ExecutionID + ":" + execution.PendingTurnID)
		var input struct{ Prompt string }
		if op == nil || json.Unmarshal(op.Request, &input) != nil || input.Prompt == "" {
			return true, s.nativeOrcaUncertain(run, "Original remote input intent is missing")
		}
		if err := s.startRemoteOrcaTurn(ctx, run, execution, execution.PendingTurnID, input.Prompt); err != nil {
			return true, s.nativeOrcaUncertain(run, err.Error())
		}
		turn = nativeOrcaCurrentTurn(execution)
	}
	if turn == nil {
		if execution.RecoveryRequired {
			return true, s.waitWorkerFlow(run, "Original remote start is unverified")
		}
		return true, s.startInitialWorkerFlowTurn(ctx, *task, run)
	}
	if execution.RecoveryRequired {
		// Observe and replay the inbox before permitting fresh recovery input.
		if err := s.pollNativeOrcaWorker(ctx, run); err != nil {
			return true, err
		}
		if err := s.verifyRemoteOrcaWorker(ctx, run, execution); err != nil {
			execution.RecoveryState = "unverified"
			return true, s.nativeOrcaUncertain(run, err.Error())
		} else {
			execution.RecoveryState = "live"
		}
		s.refreshRun(run)
		if err := s.commitWorker(run); err != nil {
			return true, err
		}
		if run.Decision != nil && run.Decision.Status == "answer_saved" {
			return true, s.refreshSavedRecoveryDecision(run, run.Decision.Request.QuestionRoute != nil, "Remote input was retained after service restart; confirm it against the original Dispatch.")
		}
		return true, nil
	}
	if workflow.Phase == "queued" && execution.PendingTurnID == "" {
		return true, s.startInitialWorkerFlowTurn(ctx, *task, run)
	}
	return true, s.pollNativeOrcaWorker(ctx, run)
}

func (s *Service) finishRemoteOrcaCleanup(ctx context.Context, run *Run) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	execution := fixed.execution
	turn := nativeOrcaCurrentTurn(execution)
	if turn == nil || turn.DoneMessageID == "" || execution.PendingTurnID != "" || execution.LastStoppedTurnID != turn.TurnID || execution.OrcaNative.DeliveryID != "" {
		return nil
	}
	client := remoteClientForExecution(fixed.config, execution)
	ref := remoteWorkerRef(remoteExecutionRef(run, execution))
	key := "orca-remote-release:" + execution.ExecutionID + ":" + turn.DispatchID
	op := s.operationByKey(key)
	if op == nil || op.Status != "confirmed" {
		if op == nil {
			_, err = s.startWorkerOperation(run, "orca_remote_release", key, map[string]string{"execution_id": execution.ExecutionID, "dispatch": turn.DispatchID})
			if err != nil {
				return err
			}
			op = s.operationByKey(key)
		}
		receipt, err := client.Release(ctx, ref)
		if err != nil {
			_ = s.changeWorkerOperation(run, op.OperationID, "outcome_unknown", "", nil, err.Error())
			return err
		}
		if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", receipt.RequestID, receipt.Payload, ""); err != nil {
			return err
		}
	}
	key = "orca-remote-coordinator-close:" + execution.ExecutionID
	op = s.operationByKey(key)
	if op != nil && op.Status == "confirmed" {
		return nil
	}
	uncertain := op != nil
	if op == nil {
		_, err = s.startWorkerOperation(run, "orca_remote_coordinator_close", key, map[string]string{"execution_id": execution.ExecutionID, "coordinator": execution.OrcaNative.Coordinator.Terminal})
		if err != nil {
			return err
		}
		op = s.operationByKey(key)
	}
	receipt, err := client.CloseCoordinator(ctx, ref, executionapi.ProcessIdentity(execution.OrcaNative.Coordinator), uncertain)
	if err != nil {
		_ = s.changeWorkerOperation(run, op.OperationID, "outcome_unknown", "", nil, err.Error())
		return err
	}
	return s.changeWorkerOperation(run, op.OperationID, "confirmed", "", receipt, "")
}

func remoteOrcaQuestionReply(ctx context.Context, c Config, run *Run, execution *ExecutionRecord, question, body string) (orcaworker.MutationReceipt, error) {
	id, err := remoteClientForExecution(c, execution).Reply(ctx, remoteWorkerRef(remoteExecutionRef(run, execution)), question, body)
	return orcaworker.MutationReceipt{RequestID: id, Payload: mustJSON(map[string]string{"question": question, "answer": body})}, remoteWorkerError(err)
}
