package steerline

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Network I/O belongs to the Execution. Only handleAppServerLaunchMessage
// writes the service projection and SQLite, including the mandatory receipt
// barrier between thread/start and turn/start.
func (s *Service) launchAppServerTurn(ctx context.Context, config Config, run Run, execution ExecutionRecord, commandID, prompt, opID string) {
	timeout := positiveDuration(config.CommandTimeout, 30*time.Second)
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	persistThread := func() error {
		ack := make(chan error, 1)
		message := &appServerLaunchMessage{phase: "thread_ready", commandID: commandID, opID: opID, run: run, execution: execution, ack: ack}
		select {
		case s.workerMessages <- workerMessage{runID: run.RunID, launch: message}:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case err := <-ack:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	worker, events, turnID, err := startAppServerWorkerTurn(ctx, config, &run, &execution, commandID, prompt, persistThread)
	message := &appServerLaunchMessage{phase: "turn_result", commandID: commandID, opID: opID, run: run, execution: execution, worker: worker, events: events, turnID: turnID, err: err}
	select {
	case s.workerMessages <- workerMessage{runID: run.RunID, launch: message}:
	case <-s.runContext.Done():
		if worker != nil {
			_ = worker.detachLocalObservation(context.Background())
		}
	}
}

func (s *Service) handleAppServerLaunchMessage(runID string, message *appServerLaunchMessage) (retErr error) {
	if message == nil || message.commandID == "" || message.opID == "" {
		return errors.New("app-server launch message lacks its fixed command")
	}
	defer func() {
		if message.phase == "turn_result" && s.appServerLaunching[runID] == message.commandID {
			delete(s.appServerLaunching, runID)
		}
		if message.worker != nil && s.workers[runID] != message.worker {
			_ = message.worker.detachLocalObservation(context.Background())
		}
	}()
	if message.ack != nil {
		defer func() { message.ack <- retErr }()
	}
	var run *Run
	for _, candidate := range s.State.Runs {
		if candidate != nil && candidate.RunID == runID {
			run = candidate
			break
		}
	}
	if run == nil || run.Protocol != string(ProtocolLocalCodexAppServer) {
		return errors.New("app-server launch no longer belongs to a local execution")
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	c := fixed.config
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || execution.ExecutionID != message.execution.ExecutionID || execution.PendingCommandID != message.commandID || run.AttemptID != message.run.AttemptID || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return ErrWorkerTurnStale
	}
	if message.phase == "thread_ready" {
		if message.execution.Process == nil || !codexThreadID.MatchString(message.execution.SessionID) || message.execution.AppServerEndpoint == "" || message.execution.AppServerGeneration == "" {
			return errors.New("app-server thread receipt is incomplete")
		}
		if execution.Process != nil && *execution.Process != *message.execution.Process || execution.SessionID != "" && execution.SessionID != message.execution.SessionID {
			return errors.New("app-server thread receipt conflicts with its fixed execution")
		}
		ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendLocal, Protocol: ProtocolLocalCodexAppServer, Target: targetFor(c), Agent: agentFor(c), RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: deref(run.WorktreePath), WorkspaceBranch: deref(run.WorktreeBranch), BaseCommit: deref(run.BaseCommit), External: *message.execution.Process, Generation: message.execution.AppServerGeneration}
		if err := (executionRecords{root: c.RunDir}).saveBinding(ref); err != nil {
			return err
		}
		execution.Process = message.execution.Process
		execution.AppServerEndpoint = message.execution.AppServerEndpoint
		execution.AppServerGeneration = message.execution.AppServerGeneration
		execution.SessionID = message.execution.SessionID
		s.refreshRun(run)
		return s.save()
	}
	if message.phase != "turn_result" {
		return fmt.Errorf("unknown app-server launch phase %q", message.phase)
	}
	if message.err != nil {
		if execution.Process == nil && message.execution.Process != nil {
			execution.Process = message.execution.Process
			execution.AppServerEndpoint = message.execution.AppServerEndpoint
			execution.AppServerGeneration = message.execution.AppServerGeneration
			s.refreshRun(run)
		}
		if err := s.setOperationState(message.opID, "outcome_unknown", message.turnID, nil, message.err.Error()); err != nil {
			return err
		}
		execution.Status = "blocked"
		s.refreshRun(run)
		return s.waitWorkerFlow(run, "Original worker turn start is unverified: "+message.err.Error())
	}
	if message.worker == nil || !codexThreadID.MatchString(message.turnID) || execution.Process == nil || message.worker.proc.Identity() != *execution.Process || message.execution.SessionID != execution.SessionID || message.execution.AppServerGeneration != execution.AppServerGeneration {
		return errors.New("app-server turn result differs from its fixed thread/process")
	}
	execution.PendingTurnID = message.turnID
	execution.Status = "pending"
	s.refreshRun(run)
	if attempt := s.workflowAttempt(run, run.AttemptID); attempt != nil {
		if attempt.EventPath == "" {
			attempt.EventPath = message.events
			execution.LogPath = ptr(message.events)
			s.refreshRun(run)
		}
		attempt.Status, attempt.UpdatedAt = "dispatched", now()
	}
	if err := s.setOperationState(message.opID, "accepted", message.turnID, mustJSON(*execution.Process), ""); err != nil {
		return err
	}
	if err := s.save(); err != nil {
		return err
	}
	s.registerWorker(run, message.worker)
	s.note(run, "steer", "worker", "accepted", "original worker turn accepted", message.commandID, map[string]any{"session_id": execution.SessionID, "turn_number": execution.TurnNumber + 1})
	return nil
}
