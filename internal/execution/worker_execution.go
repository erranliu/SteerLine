package execution

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/execution/reliable"
	"github.com/erranliu/SteerLine/internal/execution/workflow"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

const workerFlowStructuredFinalDiagnostic = "fixed development flow requires bound structured final evidence; this adapter has no verified staged-result source"

// WorkerFlowCapabilityError reports static adapter capability without running
// a native preflight or starting a worker. Product profile queries use it to
// avoid presenting a route as usable when the current staged flow cannot
// verify its final result.
func WorkerFlowCapabilityError(c Config, directory *ExecutionDirectory, processHost executionbackend.ManagedProcessHost) error {
	if c.WorkflowProtocol != executionmodel.Version {
		return nil
	}
	entry, err := registrationFor(c, directory)
	if err != nil {
		return err
	}
	setup, err := lifecycleSetupForConfig(c, entry, processHost)
	if err != nil {
		return err
	}
	lifecycle, err := entry.Lifecycle(setup.Clone())
	if err != nil {
		return err
	}
	if lifecycle == nil {
		return errors.New("execution adapter returned no lifecycle capability port")
	}
	if !lifecycle.PolicyCapabilities().StructuredFinalEvidence {
		return &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: workerFlowStructuredFinalDiagnostic}
	}
	return reliable.ValidatePreparationCapabilities(lifecycle.PolicyCapabilities().Preparation)
}

func requireWorkerFlowCapabilitiesWithResolver(ctx context.Context, c Config, directory *ExecutionDirectory, processHost executionbackend.ManagedProcessHost, resolver executionbackend.LifecycleResolver) error {
	if err := validateWorkflowConfig(c, directory); err != nil {
		return err
	}
	entry, err := registrationFor(c, directory)
	if err != nil {
		return err
	}
	setup, err := lifecycleSetupForConfig(c, entry, processHost)
	if err != nil {
		return err
	}
	lifecycle, probeErr := resolver.ResolveLifecycle(setup.Clone())
	if probeErr != nil {
		return probeErr
	}
	if !lifecycle.PolicyCapabilities().StructuredFinalEvidence {
		return &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: workerFlowStructuredFinalDiagnostic}
	}
	if lifecycle.PolicyCapabilities().ServiceStopBehavior != entry.ServiceStopBehavior {
		return errors.New("worker service-stop behavior differs from its registered declaration")
	}
	if err := reliable.ValidatePreparationCapabilities(lifecycle.PolicyCapabilities().Preparation); err != nil {
		return err
	}
	report, probeErr := lifecycle.PreflightLifecycle(ctx, setup.Clone())
	if probeErr != nil {
		return probeErr
	}
	return validateLifecycleStartCapability(setup.Snapshot.Ref, report)
}

// workerRecords binds one persisted run to its fixed profile and records.
// It is resolved for each operation; no mutable "current worker" slot owns it.
type workerRecords struct {
	run       *reconstructiontypes.RunView
	workflow  *executionmodel.WorkflowRunRecord
	execution *executionmodel.ExecutionRecord
	attempt   *executionmodel.StepAttemptRecord
	config    Config
}

func (s *Service) workerRecords(run *reconstructiontypes.RunView) (workerRecords, error) {
	if run == nil || run.RunID == "" || run.ExecutionID == "" || run.AttemptID == "" {
		return workerRecords{}, errors.New("worker flow has no fixed run identity")
	}
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil || workflow.WorkflowRunID != run.RunID ||
		execution.ExecutionID != run.ExecutionID || execution.WorkflowRunID != run.RunID || attempt.ExecutionID != execution.ExecutionID {
		return workerRecords{}, errors.New("worker flow has no matching persisted execution and attempt")
	}
	config, err := s.executionConfigForRun(run)
	if err != nil {
		return workerRecords{}, err
	}
	return workerRecords{run: run, workflow: workflow, execution: execution, attempt: attempt, config: copyWorkerConfig(config)}, nil
}

func copyWorkerConfig(c Config) Config {
	c.AgentLaunch = c.AgentLaunch.Clone()
	c.Git.Args = append([]string(nil), c.Git.Args...)
	c.ImplementationData = CloneImplementationData(c.ImplementationData)
	c.ImplementationDisplay = append([]string(nil), c.ImplementationDisplay...)
	c.PR.Query.Args = append([]string(nil), c.PR.Query.Args...)
	return c
}

func (s *Service) deliverResumableWorkerFeedback(ctx context.Context, run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord, answer executionmodel.DecisionAnswer, commandID string) error {
	sessionID, _ := executionSessionFacts(execution)
	if sessionID == "" {
		execution.Error = ptr("Feedback is saved, but the original worker session is not yet available for delivery")
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	if run.Decision.Request.QuestionRoute != nil && run.Decision.Request.InteractionKind != "async" {
		if s.signalChannel() != nil && s.workers[run.RunID] == nil {
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
	if liveTurn && executionmodel.ReportStep(s.workflowRecord(run).StepID) {
		return nil
	}
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
	if s.workers[run.RunID] != nil {
		return nil
	}
	if pending := execution.PendingCommandID; pending != "" {
		if stageInput(s.workflowRecord(run), pending) == nil || s.operationByKey("worker-turn:"+execution.ExecutionID+":"+pending) != nil || s.operationByKey("worker-session:"+execution.ExecutionID+":"+pending) != nil {
			return nil // Preserve input while its original native start is pending.
		}
		// A frozen next step may be waiting for explicit continuation before any
		// native submission. Let the accepted feedback replace that unsent input.
	}
	return s.startStageFeedback(ctx, run, answer, commandID)
}

// Only application record projection remains here; Workflow renders the text.
func workerFeedbackText(answer executionmodel.DecisionAnswer, request executionmodel.DecisionRequest) string {
	var async json.RawMessage
	if request.InteractionKind == "async" {
		async, _ = json.Marshal(map[string]any{"question_id": request.QuestionRoute.QuestionID, "source_turn_id": request.QuestionRoute.TurnID, "questions": request.Questions, "answers": answer.Answers})
	}
	label := ""
	for _, option := range request.Options {
		if option.ID == answer.OptionID {
			label = option.Label
			break
		}
	}
	return workflow.FeedbackText(answer.OptionID, label, answer.Note, async)
}
