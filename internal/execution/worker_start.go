package execution

import (
	"context"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (s *Service) prepareWorkerFlow(ctx context.Context, task taskmodel.TaskRecord, run *reconstructiontypes.RunView) error {
	if s.candidateBusinessClosed(run) {
		return nil
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	if fixed.workflow.WorkflowVersion != executionmodel.Version || fixed.attempt.StepID != "worker" || run.WorkspaceIntent == nil {
		return nativeErr("inconsistent_delivery", "worker preparation has no fixed claim and workspace intent")
	}
	if fixed.execution.RecoveryRequired {
		return errors.New("existing worker execution requires a fresh manual recovery decision")
	}
	c := fixed.config

	if err := c.BindPublicationTarget(ctx, s.runtime.ProcessHost); err != nil {
		return err
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	report, err := lifecycle.PreflightLifecycle(ctx, lifecycleSetupForRun(run, fixed, s.runtime.ProcessHost, snapshot))
	if err != nil {
		return err
	}
	if err := validateLifecycleStartCapability(snapshot.Ref, report); err != nil {
		return err
	}
	if fixed.execution.Environment != nil {
		return s.advanceWorkerPreparation(ctx, run)
	}
	return s.prepareWorkerWorkspace(ctx, task, run)
}

func validateLifecycleStartCapability(ref executionbackend.ExecutionRef, report executionbackend.CapabilityReport) error {
	if report.Version <= 0 || report.Backend != ref.Backend || report.Protocol != ref.Protocol || report.Target != ref.Target || report.Agent != ref.Agent {
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "lifecycle preflight report differs from the fixed execution target"}
	}
	for _, capability := range report.Capabilities {
		if capability.Name == "start" && capability.Status == executionbackend.CapabilityVerified {
			return nil
		}
	}
	return &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: "lifecycle preflight did not verify start capability"}
}

type workerWorkspacePreparationInput struct {
	WorkflowRunID string                           `json:"workflow_run_id"`
	ExecutionID   string                           `json:"execution_id"`
	RemoteName    string                           `json:"remote_name,omitempty"`
	Workspace     executionbackend.WorkspaceIntent `json:"workspace"`
}

func workerWorkspaceRequest(run *reconstructiontypes.RunView) (workerWorkspacePreparationInput, error) {
	if run.WorkspaceIntent == nil {
		return workerWorkspacePreparationInput{}, nativeErr("inconsistent_delivery", "worker preparation has no fixed workspace intent")
	}
	return workerWorkspacePreparationInput{WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, Workspace: *run.WorkspaceIntent}, nil
}

func (s *Service) prepareWorkerWorkspace(ctx context.Context, task taskmodel.TaskRecord, run *reconstructiontypes.RunView) error {
	input, err := workerWorkspaceRequest(run)
	if err != nil {
		return err
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	candidate, pending, err := unresolvedLifecycleCandidate(snapshot)
	if err != nil {
		return err
	}
	if !pending || candidate.Action != executionbackend.LifecyclePrepareWorkspace {
		candidate, err = lifecycle.Next(ctx, snapshot.Clone())
		if err != nil {
			return err
		}
	}
	if err = candidate.Validate(); err != nil {
		return err
	}
	prepared := false
	if candidate.Done {
		if fixed.execution.Workspace.Worktree != "" {
			prepared = true
		} else if operation := s.operationByKey("worker-workspace:" + run.ExecutionID); operation != nil {
			_, resumeErr := s.resumeWorkerWorkspace(ctx, &task, run)
			return resumeErr
		} else {
			opID, startErr := s.startWorkerOperation(run, "prepare_workspace", "worker-workspace:"+run.ExecutionID, input)
			if startErr != nil {
				return startErr
			}
			if _, err = s.workerFlowContext(run); err != nil {
				return errors.New("worker claim disappeared after preparation intent was saved")
			}
			prepared, err = prepareLocalWorkerWorkspace(s, ctx, run, opID)
		}
	} else if candidate.Action == executionbackend.LifecyclePrepareWorkspace {
		workspaceIntent := executionbackend.WorkspaceIntent{Path: input.Workspace.Path, Branch: input.Workspace.Branch, BaseCommit: input.Workspace.BaseCommit}
		if candidate.Input.Workspace == nil || *candidate.Input.Workspace != workspaceIntent {
			return errors.New("lifecycle planner changed the saved workspace intent")
		}
		prepared, err = true, s.executeLifecycleStep(ctx, run, lifecycle, candidate)
	} else if fixed.execution.Workspace.Worktree != "" {
		prepared = true
	} else {
		return errors.New("lifecycle resource planning must prepare its workspace before other resources")
	}
	if !prepared || err != nil {
		if errors.Is(err, errLifecycleNeedsReconciliation) || errors.Is(err, errLifecycleEffectPending) {
			return nil
		}
		return err
	}
	if err := s.prepareLifecycleResources(ctx, run, lifecycle); err != nil {
		return err
	}
	if fixed.execution.Environment != nil {
		return nil
	}
	if run.Decision != nil || fixed.workflow.Status != "queued" {
		return nil
	}
	return s.startInitialWorkerFlowTurn(ctx, task, run)
}

func (s *Service) startInitialWorkerFlowTurn(ctx context.Context, task taskmodel.TaskRecord, run *reconstructiontypes.RunView) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	c := fixed.config
	contract, err := s.commandStore.TaskContract(ctx, c.identity, task)
	if err != nil {
		return s.waitWorkerFlow(run, "Fixed task contract is unavailable: "+err.Error())
	}
	prompt, err := Prompt(task, contract, run.RunID, c.PublicationPolicy)
	if err != nil {
		return s.waitWorkerFlow(run, err.Error())
	}
	return s.startWorkerFlowTurn(ctx, run, prompt, newID())
}

func (s *Service) startWorkerFlowTurn(ctx context.Context, run *reconstructiontypes.RunView, prompt, turnID string) error {
	if s.candidateBusinessClosed(run) {
		return nativeErr("candidate_inactive", "candidate no longer accepts business input")
	}
	if s.lifecycleEffectPending(run.RunID) {
		return nil
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	workflow, execution, attempt := fixed.workflow, fixed.execution, fixed.attempt
	readinessTurn := execution.Environment != nil && execution.Environment.Preparation != nil && execution.Environment.Preparation.TurnID == turnID
	if execution.Environment != nil && (!execution.Environment.Ready || execution.Environment.Blocker != nil) && !readinessTurn {
		return errors.New("business input requires verified readiness and no host blocker")
	}
	if execution.RecoveryRequired {
		return errors.New("worker turn requires a fresh manual recovery decision")
	}
	sessionID, _ := executionSessionFacts(execution)
	// Resource creation may have committed the preparation Command before the
	// original turn could be sent. A human recheck continues that same saved
	// preparation identity; it does not allocate another turn or bypass a gate.
	resumePreparation := readinessTurn && execution.PendingCommandID == turnID &&
		execution.PendingTurnID == "" && s.workers[run.RunID] == nil
	if workflow.WorkflowVersion != executionmodel.Version || !executionbackend.StableExecutionID(turnID) ||
		execution.PendingTurnID != "" || execution.PendingCommandID != "" && !resumePreparation || sessionID != "" && execution.CurrentTurnID != "" && execution.LastStoppedTurnID != execution.CurrentTurnID {
		return errors.New("worker turn cannot start without one idle original session")
	}
	if sessionID == "" && execution.TurnNumber != 0 && run.DomainAttemptNumber <= 1 {
		return errors.New("worker session was lost; replacement is not allowed")
	}
	if attempt.StepID != "worker" {
		return errors.New("worker turn has no fixed worker attempt")
	}
	c := fixed.config
	if err := verifyPublicationTargetForRun(ctx, c, s.runtime.Directory, s.runtime.ProcessHost, run); err != nil {
		return err
	}

	prompt += publicationTargetInstruction(c)
	if !readinessTurn {
		if workflow.FlowRef != executionmodel.DevelopmentFlow || !executionmodel.ModelStep(workflow.StepID) {
			return errors.New("unknown fixed flow or model step")
		}
		if stageInput(workflow, turnID) == nil {
			workflow.InputVersion++
			workflow.StageInputs = append(workflow.StageInputs, executionmodel.StageInput{CommandID: turnID, AttemptID: attempt.AttemptID, StepID: workflow.StepID, Version: workflow.InputVersion, Prompt: prompt, CandidateSHA: workflow.CandidateSHA})
		}
		workflow.Status = "queued"
	}
	execution.PendingTurnID, execution.PendingCommandID = "", turnID
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	return s.startWorkerLifecycle(ctx, run, lifecycle, fixed, turnID, prompt)
}
