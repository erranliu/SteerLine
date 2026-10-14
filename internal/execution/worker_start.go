package execution

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/execution/reliable"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (s *Service) prepareWorkerFlow(ctx context.Context, task taskmodel.TaskRecord, run *reconstructiontypes.RunView) error {
	if s.candidateBusinessClosed(run) || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nil
	}
	fixed, err := s.workerRecords(run)
	if err != nil {
		return err
	}
	if fixed.workflow.WorkflowVersion != executionmodel.Version || fixed.attempt.StepID != "worker" || run.WorkspaceIntent == nil {
		return nativeErr("inconsistent_delivery", "worker preparation has no fixed claim and workspace intent")
	}
	if fixed.execution.RecoveryRequired {
		return errors.New("existing worker execution requires a fresh manual recovery decision")
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	if !lifecycle.PolicyCapabilities().StructuredFinalEvidence {
		return &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: "fixed development flow requires original bound final provenance before preparing or sending input"}
	}
	setup := lifecycleSetupForRun(run, fixed, s.runtime.ProcessHost, snapshot).Clone()
	if s.signalQueue != nil {
		return s.scheduleWorkerStartCheck(ctx, run, fixed.config, lifecycle, setup)
	}
	if err := checkWorkerStart(ctx, fixed.config, lifecycle, setup); err != nil {
		return s.failReservedStart(run, "Worker start preflight failed: "+err.Error())
	}
	return s.prepareWorkerFlowResources(ctx, run)
}

func (s *Service) prepareWorkerFlowResources(ctx context.Context, run *reconstructiontypes.RunView) error {
	fixed, err := s.workerRecords(run)
	if err != nil {
		return err
	}
	if fixed.execution.Environment != nil {
		return s.advanceWorkerPreparation(ctx, run)
	}
	task := nativeTaskRecord(s.domain.Tasks, run.TaskID)
	if task == nil {
		return nativeErr("inconsistent_delivery", "worker preparation lost its claimed task")
	}
	return s.prepareWorkerWorkspace(ctx, *task, run)
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
	candidate, pending, err := reliable.UnresolvedCandidate(snapshot)
	if err != nil {
		return err
	}
	if !pending || candidate.Action != executionbackend.LifecyclePrepareWorkspace {
		candidate, err = reliable.Next(ctx, lifecycle, snapshot.Clone())
		if err != nil {
			return err
		}
	}
	if err = candidate.Validate(); err != nil {
		return err
	}
	prepared := false
 if candidate.Done {
  if fixed.execution.Workspace.Worktree==""{return errors.New("lifecycle completed without verified original workspace; no local fallback is authorized")}
  prepared=true
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
	fixed, err := s.workerRecords(run)
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
	facts, err := workerTaskFacts(task, contract)
	if err != nil {
		return err
	}
	return s.startWorkerFlowTurn(ctx, run, prompt, newID(), facts)
}

func (s *Service) startWorkerFlowTurn(ctx context.Context, run *reconstructiontypes.RunView, prompt, turnID string, taskFacts ...json.RawMessage) error {
	if s.candidateBusinessClosed(run) || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nativeErr("candidate_inactive", "candidate no longer accepts business input")
	}
	if s.lifecycleEffectPending(run.RunID) {
		return nil
	}
	fixed, err := s.workerRecords(run)
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

	frozen := stageInput(workflow, turnID)
	if !readinessTurn && frozen != nil {
		if frozen.Prompt != prompt || frozen.StepID != workflow.StepID || frozen.Version != workflow.InputVersion {
			return errors.New("native input differs from the frozen stage prompt")
		}
	} else {
		if err := validateWorkerWorkspace(execution.Workspace); err != nil {
			return err
		}
		prompt += stagePublicationInstruction(workflow.StepID, c)
	}
	if !readinessTurn {
		if workflow.FlowRef != executionmodel.DevelopmentFlow || !executionmodel.ModelStep(workflow.StepID) {
			return errors.New("unknown fixed flow or model step")
		}
		if stageInput(workflow, turnID) == nil {
			workflow.InputVersion++
			input := executionmodel.StageInput{CommandID: turnID, AttemptID: attempt.AttemptID, StepID: workflow.StepID, Version: workflow.InputVersion, Prompt: prompt}
			if len(taskFacts) > 0 {
				input.TaskFacts = append(json.RawMessage(nil), taskFacts[0]...)
			}
			workflow.StageInputs = append(workflow.StageInputs, input)
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
