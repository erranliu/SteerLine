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
	if err := c.bindPublicationTarget(ctx); err != nil {
		return err
	}
	adapter, err := s.workerFlowMode(run)
	if err != nil {
		return err
	}
	return s.prepareWorkerFlowByMode(ctx, task, run, adapter)
}

func (s *Service) prepareWorkerWorkspace(ctx context.Context, task TaskRecord, run *Run, remote bool) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	workflow, execution, attempt, c := fixed.workflow, fixed.execution, fixed.attempt, fixed.config
	if run.WorkspaceIntent == nil {
		return nativeErr("inconsistent_delivery", "worker preparation has no fixed workspace intent")
	}
	remoteName := ""
	if remote {
		var nameErr error
		remoteName, nameErr = workerOrcaWorktreeName(run.TaskID, run.RunID)
		if nameErr != nil {
			return nameErr
		}
	}
	opID, err := s.startWorkerOperation(run, "prepare_workspace", "worker-workspace:"+execution.ExecutionID, struct {
		WorkflowRunID string                       `json:"workflow_run_id"`
		ExecutionID   string                       `json:"execution_id"`
		RemoteName    string                       `json:"remote_name,omitempty"`
		Workspace     executionapi.WorkspaceIntent `json:"workspace"`
	}{workflow.WorkflowRunID, execution.ExecutionID, remoteName, *run.WorkspaceIntent})
	if err != nil {
		return err
	}
	workflow, execution = s.workflowRecords(run)
	attempt = s.workflowAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("worker claim disappeared after preparation intent was saved")
	}
	if remote {
		err = s.prepareRemoteWorkerWorkspace(ctx, run, execution, opID)
	} else {
		preparation, prepareErr := prepareWorktree(ctx, c, run)
		err = prepareErr
		if err != nil {
			status := string(preparation.Status)
			if status != string(executionapi.OperationRejected) {
				status = "outcome_unknown"
			}
			_ = s.changeWorkerOperation(run, opID, status, "", mustJSON(preparation), err.Error())
			execution.Status = "blocked"
			s.refreshRun(run)
			return s.waitWorkerFlow(run, "Workspace preparation needs reconciliation: "+err.Error())
		}
	}
	if err != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
		execution.Status = "blocked"
		s.refreshRun(run)
		return s.waitWorkerFlow(run, "Workspace preparation needs reconciliation: "+err.Error())
	}
	if !remote {
		s.refreshRun(run)
		workspace, inspectErr := s.inspectWorkflowWorkspace(ctx, run)
		if inspectErr != nil {
			_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, inspectErr.Error())
			execution.Status = "blocked"
			s.refreshRun(run)
			return s.waitWorkerFlow(run, "Workspace identity could not be verified: "+inspectErr.Error())
		}
		applyWorkflowWorkspaceEvidence(run, execution, workspace)
		if err = s.changeWorkerOperation(run, opID, "confirmed", "", mustJSON(workspace), ""); err != nil {
			return err
		}
	}
	if remote {
		if err := s.prepareRemoteOrcaCoordinator(ctx, run); err != nil {
			return s.waitWorkerFlow(run, "Original Orca terminal preparation is unverified: "+err.Error())
		}
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
	if workflow.WorkflowVersion != WorkerFlowVersion || !stableExecutionID(turnID) ||
		execution.PendingTurnID != "" || execution.PendingCommandID != "" || execution.SessionID != "" && execution.CurrentTurnID != "" && execution.LastStoppedTurnID != execution.CurrentTurnID {
		return errors.New("worker turn cannot start without one idle original session")
	}
	if execution.SessionID == "" && execution.TurnNumber != 0 && run.DomainAttemptNumber <= 1 {
		return errors.New("worker session was lost; replacement is not allowed")
	}
	if attempt.StepID != "worker" {
		return errors.New("worker turn has no fixed worker attempt")
	}
	adapter, err := s.workerFlowMode(run)
	if err != nil {
		return err
	}
	c := fixed.config
	if err := verifyPublicationTargetForRun(ctx, c, run); err != nil {
		return err
	}
	prompt += publicationTargetInstruction(c)
	if fixed.config.PublicationPolicy.Publisher == executionapi.PublisherHostService {
		prompt += publication.WorkerInstruction(run.RunID)
	}
	if adapter == workerflow.NativeOrca {
		prompt += "\nOrca Run, Task and Dispatch IDs are separate from the fixed SteerLine workflow_run_id.\n"
		prompt += nativeOrcaRelayInstruction(run.RunID, turnID)
	}
	execution.PendingTurnID, execution.PendingCommandID = adapter.PendingIdentity(turnID)
	opID, err := s.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+run.ExecutionID+":"+turnID, struct {
		ExecutionID string `json:"execution_id"`
		SessionID   string `json:"session_id,omitempty"`
		TurnID      string `json:"turn_id"`
		Prompt      string `json:"prompt"`
	}{run.ExecutionID, execution.SessionID, turnID, prompt})
	if err != nil {
		return err
	}
	if _, err := s.workerFlowContext(run); err != nil {
		return errors.New("worker turn disappeared after send intent was saved")
	}
	return s.startWorkerTurnByMode(ctx, run, turnID, prompt, opID, adapter)
}
