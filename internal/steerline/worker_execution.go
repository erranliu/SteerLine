package steerline

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func (s *Service) prepareCodexWorker(ctx context.Context, task TaskRecord, run *Run) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	if err := requireWorkerFlowCapabilities(ctx, fixed.config); err != nil {
		return err
	}
	return s.prepareWorkerWorkspace(ctx, task, run)
}

func (s *Service) prepareNativeOrcaWorkerFlow(ctx context.Context, task TaskRecord, run *Run) error {
	if execution := s.executionRecord(run); execution.Environment != nil {
		return s.advanceOrcaPreparation(ctx, run)
	}
	if err := s.prepareNativeOrcaWorker(ctx, run); err != nil {
		return s.nativeOrcaUncertain(run, "Native Orca worker preparation needs reconciliation: "+err.Error())
	}
	return s.startInitialWorkerFlowTurn(ctx, task, run)
}

func (s *Service) prepareSupervisedOrcaWorker(ctx context.Context, task TaskRecord, run *Run) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	if err := requireWorkerFlowCapabilities(ctx, fixed.config); err != nil {
		return err
	}
	return s.prepareWorkerWorkspace(ctx, task, run)
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
		ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalCodexAppServer, Target: targetFor(fixed.config), Agent: agentFor(fixed.config), RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: deref(run.WorktreePath), WorkspaceBranch: deref(run.WorktreeBranch), BaseCommit: deref(run.BaseCommit), External: *execution.Process, Generation: execution.AppServerGeneration}
		return s.commitWorkerBinding(run, ref)
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
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
		if ctx.Err() != nil {
			return nil
		}
		fixed.execution.Status = "blocked"
		s.refreshRun(run)
		if fixed.execution.Environment != nil {
			return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "business_dispatch_unverified", Confidence: "unverified", Source: "business.operation", Detail: clip(err.Error(), 1000)})
		}
		return s.nativeOrcaUncertain(run, "Native Orca Dispatch needs reconciliation: "+err.Error())
	}
	fixed.attempt.Status, fixed.attempt.UpdatedAt = "dispatched", now()
	fixed.execution.Status = "pending"
	s.refreshRun(run)
	current := nativeOrcaCurrentTurn(fixed.execution)
	if current == nil {
		return errors.New("native Orca turn has no accepted Dispatch identity")
	}
	if err := s.setOperationState(opID, "accepted", current.DispatchRequestID, mustJSON(*current), ""); err != nil {
		return err
	}
	s.note(run, "steer", "worker", "accepted", "native Orca Dispatch input accepted", current.DispatchID, map[string]any{"task_id": current.TaskID, "turn_id": turnID})
	return s.commitWorker(run, opID)
}

func (s *Service) startSupervisedOrcaWorkerTurn(ctx context.Context, run *Run, turnID, prompt, opID string) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	if err := s.startRemoteOrcaTurn(ctx, run, fixed.execution, turnID, prompt); err != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
		return s.nativeOrcaUncertain(run, "Remote Orca start requires reconciliation: "+err.Error())
	}
	turn := nativeOrcaCurrentTurn(fixed.execution)
	if turn == nil || turn.DispatchID == "" {
		return errors.New("remote worker has no accepted Dispatch")
	}
	fixed.attempt.Status, fixed.attempt.UpdatedAt = "dispatched", now()
	fixed.execution.Status = "pending"
	s.refreshRun(run)
	return s.changeWorkerOperation(run, opID, "accepted", turn.DispatchRequestID, mustJSON(*turn), "")
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
	return s.waitWorkerFlow(run, "Original worker turn start is unverified: "+startErr.Error(), opID)
}

func (s *Service) acceptStartedWorkerTurn(run *Run, worker *Worker, events, platformRequestID, opID, commandID string, local bool) (retErr error) {
	if worker == nil {
		return errors.New("accepted worker turn has no observer")
	}
	defer func() {
		if retErr != nil && worker.cancelObservation != nil {
			worker.cancelObservation()
		}
	}()
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
	s.note(run, "steer", "worker", "accepted", "original worker turn accepted", commandID, map[string]any{"session_id": execution.SessionID, "turn_number": execution.TurnNumber + 1})
	if err := s.commitWorker(run, opID); err != nil {
		return fmt.Errorf("worker started but its exact process receipt could not be saved: %w", err)
	}
	s.registerWorker(run, worker)
	return nil
}

func requireWorkerFlowCapabilities(ctx context.Context, c Config) error {
	if err := validateWorkflowConfig(&c, nil); err != nil {
		return err
	}
	entry, err := registrationFor(c)
	if err != nil {
		return err
	}
	return entry.Preflight(ctx, c)
}
func preflightRemoteWorkerFlow(ctx context.Context, c Config) error {
	if c.agentName() != "codex" || c.Model == "" || c.ReasoningEffort == "" || c.Sandbox != "workspace-write" || c.Profile != "" || c.Orca.Supervised == nil {
		return &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "remote Orca requires Codex, model/effort, workspace-write and a fixed supervised target"}
	}
	_, err := probeSupervisedOrcaTarget(ctx, c)
	return err

}
func preflightNativeWorkerFlow(ctx context.Context, c Config) error {
	if c.managedWindowsOrca() {
		if _, err := nativeOrcaFixedAgentCommand(c); err != nil {
			return &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: err.Error(), Cause: err}
		}
		runner := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
		if err := codexworker.ProbeManagedCLI(ctx, c.Codex, func(ctx context.Context, executable string, args []string) ([]byte, error) {
			return runner.Output(ctx, Command{Executable: executable, Args: args})
		}); err != nil {
			return err
		}
	}

	if c.Profile != "" || c.Sandbox != "workspace-write" {
		return &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "Orca requires workspace-write and no Codex profile"}
	}
	if err := orcaworker.CheckStatus(ctx, orcaWorkerCaller(c)); err != nil {
		return err
	}
	_, err := nativeOrcaRepo(ctx, c)
	return err

}
func preflightCodexWorkerFlow(ctx context.Context, c Config) error {
	if c.agentName() != "codex" {
		return &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "resumable worker flow requires Codex"}
	}
	if c.hostName() != "local" {
		return &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "this worker-flow host is not supported"}
	}
	if err := (codexworker.Config{Policy: c.ExecutionPolicy, Args: c.Codex.Args, Sandbox: c.Sandbox, WindowsSandbox: c.WindowsSandbox, ReasoningEffort: c.ReasoningEffort}).Validate(runtime.GOOS); err != nil {
		return err
	}
	command := c.Codex
	if err := resolveCommand(&command); err != nil {
		return err
	}
	c.Codex = command
	runner := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	err := codexworker.ProbeCapabilities(ctx, codexworker.ProbeConfig{
		Executable: command.Executable, Args: command.Args, Repository: c.Repository, RunDir: c.RunDir,
	}, codexworker.ProbePort{
		Output: func(ctx context.Context, executable string, args []string) ([]byte, error) {
			return runner.Output(ctx, Command{Executable: executable, Args: args})
		},
		HistoryAvailable: func(ctx context.Context) bool { return nativeCodexHistoryAvailable(ctx, c) },
		StartManaged: func(cmd *exec.Cmd) (func(), error) {
			owned, err := startProcess(cmd)
			if err != nil {
				return nil, err
			}
			return owned.Close, nil
		},
	})
	var unsupported *codexworker.ProbeError
	if errors.As(err, &unsupported) {
		return &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: unsupported.Diagnostic, Cause: unsupported.Cause}
	}
	return err

}

// workerFlowContext binds one persisted run to its fixed profile and records.
// It is resolved for each operation; no mutable "current worker" slot owns it.
type workerFlowContext struct {
	run       *Run
	workflow  *WorkflowRunRecord
	execution *ExecutionRecord
	attempt   *StepAttemptRecord
	config    Config
}

func (s *Service) workerFlowContext(run *Run) (workerFlowContext, error) {
	if run == nil || run.RunID == "" || run.ExecutionID == "" || run.AttemptID == "" {
		return workerFlowContext{}, errors.New("worker flow has no fixed run identity")
	}
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil || workflow.WorkflowRunID != run.RunID ||
		execution.ExecutionID != run.ExecutionID || execution.WorkflowRunID != run.RunID || attempt.ExecutionID != execution.ExecutionID {
		return workerFlowContext{}, errors.New("worker flow has no matching persisted execution and attempt")
	}
	config, err := s.executionConfigForRun(run)
	if err != nil {
		return workerFlowContext{}, err
	}
	return workerFlowContext{run: run, workflow: workflow, execution: execution, attempt: attempt, config: copyWorkerFlowConfig(config)}, nil
}

func copyWorkerFlowConfig(c Config) Config {
	c.ExecutionRequirements = c.ExecutionRequirements.Clone()
	if c.requirementsEvaluation != nil {
		e := c.requirementsEvaluation.Clone()
		c.requirementsEvaluation = &e
	}
	c.ExecutableTypes = append([]string(nil), c.ExecutableTypes...)
	c.Git.Args = append([]string(nil), c.Git.Args...)
	c.Codex.Args = append([]string(nil), c.Codex.Args...)
	c.Orca.Args = append([]string(nil), c.Orca.Args...)
	profiles := make([]ExecutionProfile, len(c.ExecutionProfiles))
	copy(profiles, c.ExecutionProfiles)
	for i := range profiles {
		profiles[i].ExecutionRequirements = profiles[i].ExecutionRequirements.Clone()
		profiles[i].Publication = clonePublicationSettings(profiles[i].Publication)
		if profiles[i].RequirementsEvaluation != nil {
			e := profiles[i].RequirementsEvaluation.Clone()
			profiles[i].RequirementsEvaluation = &e
		}
		if profiles[i].ConfiguredPublicationPolicy != nil {
			policy := *profiles[i].ConfiguredPublicationPolicy
			profiles[i].ConfiguredPublicationPolicy = &policy
		}
		profiles[i].Codex.Args = append([]string(nil), profiles[i].Codex.Args...)
		profiles[i].Orca.Args = append([]string(nil), profiles[i].Orca.Args...)
		if profiles[i].Orca.Supervised != nil {
			supervised := *profiles[i].Orca.Supervised
			profiles[i].Orca.Supervised = &supervised
		}
	}
	c.ExecutionProfiles = profiles
	c.PR.Query.Args = append([]string(nil), c.PR.Query.Args...)
	c.PR.Close.Args = append([]string(nil), c.PR.Close.Args...)
	if c.Orca.Supervised != nil {
		supervised := *c.Orca.Supervised
		c.Orca.Supervised = &supervised
	}
	if c.workerEnv != nil {
		env := make(map[string]string, len(c.workerEnv))
		for key, value := range c.workerEnv {
			env[key] = value
		}
		c.workerEnv = env
	}
	return c
}

func (s *Service) deliverResumableWorkerFeedback(ctx context.Context, run *Run, execution *ExecutionRecord, answer DecisionAnswer, commandID string) error {
	if execution.SessionID == "" || run.SessionID == nil || *run.SessionID != execution.SessionID {
		execution.Error = ptr("Feedback is saved, but the original worker session is not yet available for delivery")
		s.refreshRun(run)
		return s.commitWorker(run)
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
	for _, operation := range s.domain.Operations {
		if operation.IdempotencyKey != "worker-turn:"+execution.ExecutionID+":"+commandID {
			continue
		}
		if operation.Status == "confirmed" || operation.Status == "accepted" || operation.Status == "in_flight" {
			return nil // The original request may have started. Never resend.
		}
		execution.Error = ptr("Feedback delivery remains unverified at the original request ID; no duplicate message was sent")
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	return s.startWorkerFlowTurn(ctx, run, workerFeedbackText(answer, run.Decision.Request), commandID)
}

func workerFeedbackText(answer DecisionAnswer, request DecisionRequest) string {
	if request.HostTarget != nil && answer.OptionID == "" {
		return answer.Note
	}
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

func prepareLocalWorkerWorkspace(s *Service, ctx context.Context, run *Run, opID string) (bool, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return false, err
	}
	execution := fixed.execution
	preparation, err := prepareWorktree(ctx, fixed.config, run)
	if err != nil {
		status := string(preparation.Status)
		if status != string(executionapi.OperationRejected) {
			status = "outcome_unknown"
		}
		_ = s.changeWorkerOperation(run, opID, status, "", mustJSON(preparation), err.Error())
		execution.Status = "blocked"
		s.refreshRun(run)
		return false, s.waitWorkerFlow(run, "Workspace preparation needs reconciliation: "+err.Error())
	}
	s.refreshRun(run)
	workspace, err := s.inspectWorkflowWorkspace(ctx, run)
	if err != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
		execution.Status = "blocked"
		s.refreshRun(run)
		return false, s.waitWorkerFlow(run, "Workspace identity could not be verified: "+err.Error())
	}
	applyWorkflowWorkspaceEvidence(run, execution, workspace)
	if err = s.changeWorkerOperation(run, opID, "confirmed", "", mustJSON(workspace), ""); err != nil {
		return false, err
	}
	return true, nil
}
func remoteWorkerWorkspaceRequest(run *Run) (workerWorkspacePreparationInput, error) {
	input, err := workerWorkspaceRequest(run)
	if err != nil {
		return input, err
	}
	input.RemoteName, err = workerOrcaWorktreeName(run.TaskID, run.RunID)
	return input, err
}
func prepareRegisteredRemoteWorkspace(s *Service, ctx context.Context, run *Run, opID string) (bool, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return false, err
	}
	if err = s.prepareRemoteWorkerWorkspace(ctx, run, fixed.execution, opID); err != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
		fixed.execution.Status = "blocked"
		s.refreshRun(run)
		return false, s.waitWorkerFlow(run, "Workspace preparation needs reconciliation: "+err.Error())
	}
	if err = s.prepareRemoteOrcaCoordinator(ctx, run); err != nil {
		return false, s.waitWorkerFlow(run, "Original Orca terminal preparation is unverified: "+err.Error())
	}
	return true, nil
}
