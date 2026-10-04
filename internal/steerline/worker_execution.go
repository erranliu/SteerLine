package steerline

import (
	"context"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func requireWorkerFlowCapabilities(ctx context.Context, c Config) error {
	if c.ExecutionDirectory == nil {
		return errors.New("execution directory is required")
	}
	return requireWorkerFlowCapabilitiesWithResolver(ctx, c, c.ExecutionDirectory)
}

func requireWorkerFlowCapabilitiesWithResolver(ctx context.Context, c Config, resolver executionbackend.LifecycleResolver) error {
	if err := validateWorkflowConfig(&c, nil); err != nil {
		return err
	}
	entry, err := registrationFor(c)
	if err != nil {
		return err
	}
	setup, err := lifecycleSetupForConfig(c, entry)
	if err != nil {
		return err
	}
	lifecycle, probeErr := resolver.ResolveLifecycle(setup.Clone())
	if probeErr != nil {
		return probeErr
	}
	report, probeErr := lifecycle.PreflightLifecycle(ctx, setup.Clone())
	if probeErr != nil {
		return probeErr
	}
	return validateLifecycleStartCapability(setup.Snapshot.Ref, report)
}

// workerFlowContext binds one persisted run to its fixed profile and records.
// It is resolved for each operation; no mutable "current worker" slot owns it.
type workerFlowContext struct {
	run       *Run
	workflow  *executionmodel.WorkflowRunRecord
	execution *executionmodel.ExecutionRecord
	attempt   *executionmodel.StepAttemptRecord
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
	c.ImplementationData = cloneImplementationData(c.ImplementationData)
	c.ImplementationDisplay = append([]string(nil), c.ImplementationDisplay...)
	profiles := make([]ExecutionProfile, len(c.ExecutionProfiles))
	copy(profiles, c.ExecutionProfiles)
	for i := range profiles {
		profiles[i].ExecutionRequirements = profiles[i].ExecutionRequirements.Clone()
		profiles[i].Publication = clonePublicationSettings(profiles[i].Publication)
		if profiles[i].RequirementsEvaluation != nil {
			e := profiles[i].RequirementsEvaluation.Clone()
			profiles[i].RequirementsEvaluation = &e
		}
		profiles[i].ImplementationConfigs = cloneImplementationInputs(profiles[i].ImplementationConfigs)
		profiles[i].ImplementationData = cloneImplementationData(profiles[i].ImplementationData)
		profiles[i].ImplementationDisplay = append([]string(nil), profiles[i].ImplementationDisplay...)
	}
	c.ExecutionProfiles = profiles
	c.PR.Query.Args = append([]string(nil), c.PR.Query.Args...)
	c.PR.Close.Args = append([]string(nil), c.PR.Close.Args...)
	if c.workerEnv != nil {
		env := make(map[string]string, len(c.workerEnv))
		for key, value := range c.workerEnv {
			env[key] = value
		}
		c.workerEnv = env
	}
	return c
}

func (s *Service) deliverResumableWorkerFeedback(ctx context.Context, run *Run, execution *executionmodel.ExecutionRecord, answer executionmodel.DecisionAnswer, commandID string) error {
	sessionID, _ := executionSessionFacts(execution)
	if sessionID == "" {
		execution.Error = ptr("Feedback is saved, but the original worker session is not yet available for delivery")
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	if run.Decision.Request.QuestionRoute != nil {
		if s.workerMessages != nil && s.workers[run.RunID] == nil {
			return nil // Recovery must attach the original observer before replying.
		}
		return s.deliverLifecycleQuestionAnswer(ctx, run, execution, answer)
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
	priorStatus := s.operationByKey("worker-live-feedback:" + execution.ExecutionID + ":" + commandID)
	liveTurn := execution.CurrentTurnID != "" && execution.CurrentTurnID != execution.LastStoppedTurnID && execution.PendingCommandID == "" && (execution.Status == "active" || execution.Status == "running" || s.workers[run.RunID] != nil)
	if priorStatus != nil || liveTurn {
		lifecycle, _, err := s.lifecycleForRun(run)
		if err != nil {
			return err
		}
		if priorStatus == nil && !lifecycle.PolicyCapabilities().ActiveTurnFeedback {
			return nil // This implementation preserves input until the original turn stops.
		}
		return s.deliverLifecycleActiveFeedback(ctx, run, answer, commandID, run.Decision.Request, s.decisionRecord(run))
	}
	if execution.PendingCommandID != "" || s.workers[run.RunID] != nil {
		return nil
	} // Preserve input while the original start is pending.
	return s.startWorkerFlowTurn(ctx, run, workerFeedbackText(answer, run.Decision.Request), commandID)
}

func workerFeedbackText(answer executionmodel.DecisionAnswer, request executionmodel.DecisionRequest) string {
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
		if status != string(executionbackend.OperationRejected) {
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
