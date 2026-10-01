package steerline

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/publication"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

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
	c := fixed.config
	if err := s.checkCurrentExecutionRequirements(ctx, fixed); err != nil {
		return err
	}
	if err := c.bindPublicationTarget(ctx); err != nil {
		return err
	}
	adapter, err := s.workerFlowRegistration(run)
	if err != nil {
		return err
	}
	return adapter.Prepare(s, ctx, task, run)
}

type workerWorkspacePreparationInput struct {
	WorkflowRunID string                       `json:"workflow_run_id"`
	ExecutionID   string                       `json:"execution_id"`
	RemoteName    string                       `json:"remote_name,omitempty"`
	Workspace     executionapi.WorkspaceIntent `json:"workspace"`
}

func workerWorkspaceRequest(run *Run) (workerWorkspacePreparationInput, error) {
	if run.WorkspaceIntent == nil {
		return workerWorkspacePreparationInput{}, nativeErr("inconsistent_delivery", "worker preparation has no fixed workspace intent")
	}
	return workerWorkspacePreparationInput{WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, Workspace: *run.WorkspaceIntent}, nil
}
func (s *Service) prepareWorkerWorkspace(ctx context.Context, task TaskRecord, run *Run) error {
	entry, err := s.workerFlowRegistration(run)
	if err != nil {
		return err
	}
	if entry.WorkspaceRequest == nil || entry.PrepareWorkspace == nil {
		return unsupported("workspace preparation")
	}
	input, err := entry.WorkspaceRequest(run)
	if err != nil {
		return err
	}
	opID, err := s.startWorkerOperation(run, "prepare_workspace", "worker-workspace:"+run.ExecutionID, input)
	if err != nil {
		return err
	}
	if _, err = s.workerFlowContext(run); err != nil {
		return errors.New("worker claim disappeared after preparation intent was saved")
	}
	prepared, err := entry.PrepareWorkspace(s, ctx, run, opID)
	if !prepared || err != nil {
		return err
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
	prompt, err := workerflow.Prompt(task, contract, run.RunID, c.PublicationPolicy)
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
	if execution.Environment != nil && (!execution.Environment.Ready || execution.Environment.Blocker != nil) {
		return errors.New("business input requires verified readiness and no host blocker")
	}
	if execution.RecoveryRequired {
		return errors.New("worker turn requires a fresh manual recovery decision")
	}
	sessionID, _ := executionSessionFacts(execution)
	if workflow.WorkflowVersion != WorkerFlowVersion || !stableExecutionID(turnID) ||
		execution.PendingTurnID != "" || execution.PendingCommandID != "" || sessionID != "" && execution.CurrentTurnID != "" && execution.LastStoppedTurnID != execution.CurrentTurnID {
		return errors.New("worker turn cannot start without one idle original session")
	}
	if sessionID == "" && execution.TurnNumber != 0 && run.DomainAttemptNumber <= 1 {
		return errors.New("worker session was lost; replacement is not allowed")
	}
	if attempt.StepID != "worker" {
		return errors.New("worker turn has no fixed worker attempt")
	}
	adapter, err := s.workerFlowRegistration(run)
	if err != nil {
		return err
	}
	c := fixed.config
	if err := verifyPublicationTargetForRun(ctx, c, run); err != nil {
		return err
	}
	if err := s.checkCurrentExecutionRequirements(ctx, fixed); err != nil {
		return err
	}
	prompt += publicationTargetInstruction(c)
	if fixed.config.PublicationPolicy.Publisher == executionapi.PublisherHostService {
		prompt += publication.WorkerInstruction(run.RunID)
	}
	if adapter.TurnInstruction != nil {
		prompt += adapter.TurnInstruction(run.RunID, turnID)
	}
	execution.PendingTurnID, execution.PendingCommandID = adapter.PendingIdentity(turnID)
	opID, err := s.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+run.ExecutionID+":"+turnID, struct {
		ExecutionID string `json:"execution_id"`
		SessionID   string `json:"session_id,omitempty"`
		TurnID      string `json:"turn_id"`
		Prompt      string `json:"prompt"`
	}{run.ExecutionID, sessionID, turnID, prompt})
	if err != nil {
		return err
	}
	if _, err := s.workerFlowContext(run); err != nil {
		return errors.New("worker turn disappeared after send intent was saved")
	}
	return adapter.Start(s, ctx, run, turnID, prompt, opID)
}
