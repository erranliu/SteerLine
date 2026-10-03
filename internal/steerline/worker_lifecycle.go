package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

var errLifecycleNeedsReconciliation = errors.New("lifecycle operation is waiting for read-only reconciliation")

// acceptStartedWorkerTurn commits a generic, already-accepted observer handle.
// It remains the shared persistence boundary used by receipt commit-failure tests.
func (s *Service) acceptStartedWorkerTurn(run *Run, worker *Worker, events, platformRequestID, operationID, commandID string, local bool) (retErr error) {
	if worker == nil || worker.proc == nil {
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
	if local {
		fixed.execution.PendingTurnID = platformRequestID
	}
	identity := worker.proc.Identity()
	fixed.execution.Process = &identity
	fixed.execution.Status = "pending"
	s.refreshRun(run)
	if events != "" {
		fixed.attempt.EventPath = events
		fixed.execution.LogPath = ptr(events)
		s.refreshRun(run)
	}
	fixed.attempt.Status, fixed.attempt.UpdatedAt = "dispatched", now()
	if err := s.setOperationState(operationID, "accepted", platformRequestID, mustJSON(identity), ""); err != nil {
		return err
	}
	sessionID, _ := executionSessionFacts(fixed.execution)
	s.note(run, "steer", "worker", "accepted", "original worker turn accepted", commandID,
		map[string]any{"session_id": sessionID, "turn_number": fixed.execution.TurnNumber + 1})
	if err := s.commitWorker(run, operationID); err != nil {
		return fmt.Errorf("worker started but its exact process receipt could not be saved: %w", err)
	}
	s.registerWorker(run, worker)
	return nil
}

func lifecycleSetupForRun(run *Run, fixed workerFlowContext, snapshot executionapi.LifecycleSnapshot) executionapi.LifecycleSetup {
	blocked := append([]string(nil), fixed.config.publicationSettings().AuthenticationRefs...)
	blocked = append(blocked, "BEADS_*", "BD_*", "GH_TOKEN", "GITHUB_TOKEN", "GITEA_TOKEN", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_*", "GIT_CONFIG_VALUE_*")
	return executionapi.LifecycleSetup{
		Snapshot: snapshot.Clone(), ProfileData: cloneImplementationData(fixed.config.ImplementationData),
		ProfileID: fixed.config.selectedProfileID, AcceptedConfigVersion: fixed.config.acceptedConfigVersion(),
		Policy: fixed.config.ExecutionPolicy, ProcessHost: fixed.config.ProcessHost, RepositoryPath: fixed.config.Repository,
		RunDirectory: fixed.config.RunDir, CommandTimeout: duration(fixed.config.CommandTimeout),
		AttemptNumber: run.DomainAttemptNumber, BlockedEnvironment: blocked,
		GitExecutable: fixed.config.Git.Executable, GitArgs: append([]string(nil), fixed.config.Git.Args...),
		RemoteName: fixed.config.Remote, TargetBranch: fixed.config.Branch,
	}
}

func lifecycleSetupForConfig(c Config, entry ExecutionRegistration) (executionapi.LifecycleSetup, error) {
	if c.identity == "" {
		if err := c.ResolveIdentity(context.Background()); err != nil {
			return executionapi.LifecycleSetup{}, err
		}
	}
	ref := executionapi.ExecutionRef{
		Version: executionapi.ExecutionRecordVersion, Backend: entry.Key.Backend, Protocol: entry.Key.Protocol,
		Target: entry.Target(c), Agent: agentFor(c), WorkspacePath: c.Repository,
	}
	session := executionapi.SessionContext{
		ProjectID: c.identity, TaskID: "capability-probe", WorkflowRunID: "00000000000000000000000000000001",
		ExecutionID: "00000000000000000000000000000002", AttemptID: "00000000000000000000000000000003",
		Agent: agentFor(c), Target: ref.Target,
	}
	snapshot := executionapi.LifecycleSnapshot{
		Ref: ref, Session: session, ConfigurationDigest: c.fixedProfileDigest,
		ResourceGeneration: "capability-probe",
	}
	blocked := append([]string(nil), c.publicationSettings().AuthenticationRefs...)
	blocked = append(blocked, "BEADS_*", "BD_*", "GH_TOKEN", "GITHUB_TOKEN", "GITEA_TOKEN", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_*", "GIT_CONFIG_VALUE_*")
	return executionapi.LifecycleSetup{
		Snapshot: snapshot, ProfileData: cloneImplementationData(c.ImplementationData),
		ProfileID: c.selectedProfileID, AcceptedConfigVersion: c.acceptedConfigVersion(),
		Policy: c.ExecutionPolicy, ProcessHost: c.ProcessHost,
		RepositoryPath: c.Repository, RunDirectory: c.RunDir, CommandTimeout: duration(c.CommandTimeout),
		AttemptNumber: 1, BlockedEnvironment: blocked, GitExecutable: c.Git.Executable,
		GitArgs: append([]string(nil), c.Git.Args...), RemoteName: c.Remote, TargetBranch: c.Branch,
	}, nil
}

func (s *Service) lifecycleForRun(run *Run) (executionapi.LifecyclePort, workerFlowContext, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return nil, workerFlowContext{}, err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return nil, workerFlowContext{}, err
	}
	port, err := s.lifecycleResolver.ResolveLifecycle(lifecycleSetupForRun(run, fixed, snapshot).Clone())
	if err != nil {
		return nil, workerFlowContext{}, err
	}
	return port, fixed, nil
}

func (s *Service) resumeLifecycleWorkerFlow(ctx context.Context, task *TaskRecord, run *Run) (bool, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return false, err
	}
	execution := fixed.execution
	if execution.Environment != nil {
		if err := s.observeWorkerEnvironment(ctx, run); err != nil {
			return true, err
		}
	}
	if execution.PendingDeliveryID != "" {
		if err := s.acknowledgeLifecycleDelivery(run, execution.PendingDeliveryID); err != nil {
			return true, err
		}
		fixed, err = s.workerFlowContext(run)
		if err != nil {
			return true, err
		}
		execution = fixed.execution
	}
	for _, operation := range s.domain.Operations {
		if !strings.HasPrefix(operation.IdempotencyKey, "worker-delivery-ack:"+execution.ExecutionID+":") ||
			operation.Kind != string(executionapi.LifecycleAcknowledge) || operation.Status != "in_flight" && operation.Status != "outcome_unknown" && operation.Status != "accepted" {
			continue
		}
		var input executionapi.AcknowledgeInput
		if err := json.Unmarshal(operation.Request, &input); err != nil || input.DeliveryID == "" {
			return true, errors.New("saved Delivery acknowledgement identity is invalid")
		}
		if err := s.acknowledgeLifecycleDelivery(run, input.DeliveryID); err != nil {
			return true, err
		}
		for _, current := range s.domain.Operations {
			if current.OperationID == operation.OperationID && current.Status != "confirmed" {
				return true, nil
			}
		}
		break
	}
	if lifecycle, _, resolveErr := s.lifecycleForRun(run); resolveErr != nil {
		return true, resolveErr
	} else if capabilities := lifecycle.PolicyCapabilities(); capabilities.ResumeActiveObservation || capabilities.SeparateTurnStart {
		handled, reconcileErr := s.reconcilePendingLifecycleSteps(ctx, run, lifecycle)
		if reconcileErr != nil {
			return true, reconcileErr
		}
		if handled || s.workers[run.RunID] != nil {
			return true, nil
		}
	}
	if execution.Environment != nil && !execution.RecoveryRequired {
		if execution.Environment.Blocker != nil {
			return true, s.observeWorkerEnvironment(ctx, run)
		}
		if execution.PendingTurnID != "" || execution.PendingCommandID != "" || execution.CurrentTurnID != "" && execution.CurrentTurnID != execution.LastStoppedTurnID {
			lifecycle, _, lifecycleErr := s.lifecycleForRun(run)
			if lifecycleErr != nil {
				return true, lifecycleErr
			}
			return true, s.resumeLifecycleObservation(ctx, run, lifecycle)
		}
		if !execution.Environment.Ready || execution.CurrentTurnID != "" && execution.CurrentTurnID == execution.LastStoppedTurnID && execution.PendingCommandID == "" && execution.PendingTurnID == "" &&
			(run.Decision == nil || run.Decision.Status == "reply_accepted" || run.Decision.Status == "applied") {
			return true, s.advanceWorkerPreparation(ctx, run)
		}
	}
	if !execution.RecoveryRequired && (execution.PendingCommandID != "" || execution.CurrentTurnID != "" && execution.CurrentTurnID != execution.LastStoppedTurnID) {
		if lifecycle, _, lifecycleErr := s.lifecycleForRun(run); lifecycleErr == nil && lifecycle.PolicyCapabilities().ResumeActiveObservation {
			return true, s.resumeLifecycleObservation(ctx, run, lifecycle)
		}
		return true, nil // The original local app-server process is handled by its recovery owner.
	}
	reconciled, err := s.reconcileLocalWorkerPublication(ctx, run)
	if err != nil {
		return true, s.waitWorkerFlow(run, "Host publication needs reconciliation: "+err.Error())
	}
	if reconciled {
		return true, nil
	}
	if execution.RecoveryRequired {
		lifecycle, fixed, lifecycleErr := s.lifecycleForRun(run)
		if lifecycleErr != nil {
			return true, lifecycleErr
		}
		snapshot, snapshotErr := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
		if snapshotErr != nil {
			return true, snapshotErr
		}
		evidence, verifyErr := lifecycle.VerifyManualRecovery(ctx, snapshot)
		if verifyErr != nil {
			execution.RecoveryState = "unverified"
			return true, s.waitWorkerFlow(run, "Original worker identity cannot be verified: "+verifyErr.Error())
		}
		switch {
		case evidence.ManagedProcess && !evidence.ProcessAbsent:
			execution.RecoveryState = "live"
		case evidence.ProcessAbsent && execution.CurrentTurnID != "" && execution.CurrentTurnID == execution.LastStoppedTurnID:
			execution.RecoveryState = "completed"
		case evidence.ProcessAbsent || !evidence.SessionAvailable:
			execution.RecoveryState = "interrupted"
		default:
			execution.RecoveryState = "unverified"
		}
		s.refreshRun(run)
		if err := s.commitWorker(run); err != nil {
			return true, err
		}
		if evidence.ManagedProcess && !evidence.ProcessAbsent && lifecycle.PolicyCapabilities().ResumeActiveObservation {
			return true, s.resumeLifecycleObservation(ctx, run, lifecycle)
		}
		if run.Decision != nil && run.Decision.Status == "answer_saved" {
			return true, s.refreshSavedRecoveryDecision(run, false, "服务重开后请确认旧答案；原轮次不会自动继续。")
		}
		if execution.RecoveryState == "completed" && run.Decision != nil && run.Decision.Status == "pending" {
			if record := s.decisionRecord(run); record != nil && (record.RecoveryVersion != execution.RecoveryVersion || record.RecoveryState != execution.RecoveryState) {
				oldID := record.DecisionID
				s.setDecisionStatus(run, "superseded")
				if err := s.commitWorker(run); err != nil {
					return true, err
				}
				s.setRunDecision(run, nil)
				if err := s.waitWorkerFlow(run, "原 worker 已退出，原轮次已结束。核对当前成果后选择原 thread 或明确使用新会话继续。"); err != nil {
					return true, err
				}
				if next := s.decisionRecord(run); next != nil {
					next.RecoveryOfDecision = oldID
					s.markWorkerDecisionWrite(next.DecisionID)
					return true, s.commitWorker(run)
				}
			}
		}
		if execution.Process == nil && !evidence.SessionAvailable && execution.PendingCommandID == "" && execution.CurrentTurnID == "" {
			return true, s.waitWorkerFlow(run, "服务重开后，原 worker 尚未开始执行。请核对工作区并手动继续。")
		}
		return true, nil
	}
	if fixed.workflow.Phase != "queued" {
		return true, nil
	}
	return false, s.prepareWorkerFlow(ctx, *task, run)
}

func (s *Service) resumeLifecycleObservation(ctx context.Context, run *Run, lifecycle executionapi.LifecyclePort) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	receipt, observer, observeErr := lifecycle.ObserveLifecycle(ctx, snapshot)
	if observeErr != nil {
		return s.waitWorkerFlow(run, "Original worker observation needs reconciliation: "+observeErr.Error())
	}
	if observer != nil {
		if err := mergeLifecycleExecutionReceipt(fixed.execution, receipt); err != nil {
			_ = observer.Close(context.Background())
			return err
		}
		if err := s.commitWorker(run); err != nil {
			_ = observer.Close(context.Background())
			return err
		}
		s.registerWorker(run, workerFromLifecycle(run.RunID, run.AttemptID, observer))
		return nil
	}
	if receipt.TurnEvidence != nil && receipt.TurnEvidence.Completed {
		return s.finishWorkerFlowFor(run, nil)
	}
	reason := receipt.Diagnostic
	if reason == "" {
		reason = "Original worker observation did not confirm an active or completed turn"
	}
	return s.waitWorkerFlow(run, reason)
}

func (s *Service) reconcilePendingLifecycleSteps(ctx context.Context, run *Run, lifecycle executionapi.LifecyclePort) (bool, error) {
	for step := 0; step < 8; step++ {
		fixed, err := s.workerFlowContext(run)
		if err != nil {
			return true, err
		}
		snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
		if err != nil {
			return true, err
		}
		candidate, found, err := unresolvedLifecycleCandidate(snapshot)
		if err != nil {
			return true, err
		}
		if !found {
			return false, nil
		}
		switch candidate.Action {
		case executionapi.LifecyclePrepareWorkspace, executionapi.LifecycleCreateResource,
			executionapi.LifecycleStartTurn, executionapi.LifecycleSubmitInput, executionapi.LifecycleReportCompletion, executionapi.LifecycleHostAction:
			if err := s.executeLifecycleStep(ctx, run, lifecycle, candidate); err != nil {
				if errors.Is(err, errLifecycleNeedsReconciliation) {
					return true, nil
				}
				return true, err
			}
			if s.workers[run.RunID] != nil {
				return true, nil
			}
		default:
			return false, nil
		}
	}
	return true, errors.New("lifecycle recovery exceeded the bounded original-operation list")
}

func lifecycleSnapshot(run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, attempt *StepAttemptRecord, pending *executionapi.TurnInput) (executionapi.LifecycleSnapshot, error) {
	if run == nil || workflow == nil || execution == nil || attempt == nil || run.RunID != workflow.WorkflowRunID ||
		run.ExecutionID != execution.ExecutionID || execution.WorkflowRunID != workflow.WorkflowRunID || attempt.ExecutionID != execution.ExecutionID {
		return executionapi.LifecycleSnapshot{}, errors.New("lifecycle snapshot has no fixed execution records")
	}
	ref := executionapi.ExecutionRef{
		Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendKind(execution.Backend),
		Protocol: executionapi.ExecutionProtocol(execution.Protocol), Target: execution.Target,
		Agent: executionapi.AgentSpec{Name: execution.Agent}, RunID: run.RunID, AttemptID: run.AttemptID,
		WorkspacePath: execution.Workspace.Worktree, WorkspaceBranch: execution.Workspace.Branch,
		BaseCommit: execution.Workspace.Base, Generation: execution.Generation,
	}
	if execution.Process != nil {
		ref.External = *execution.Process
	}
	session := executionapi.SessionContext{
		ProjectID: execution.ProjectIdentity, TaskID: run.TaskID, WorkflowRunID: run.RunID,
		ExecutionID: execution.ExecutionID, AttemptID: run.AttemptID,
		Agent: executionapi.AgentSpec{Name: execution.Agent}, Target: execution.Target,
		SessionID: execution.SessionID, Generation: execution.SessionGeneration, CurrentTurnID: execution.CurrentTurnID,
		PendingTurnID: execution.PendingTurnID, LastStoppedTurnID: execution.LastStoppedTurnID,
		CurrentCommandID: execution.CurrentCommandID,
		PendingCommandID: execution.PendingCommandID,
	}
	snapshot := executionapi.LifecycleSnapshot{
		Ref: ref, Session: session, TaskTitle: run.Title, WorkflowPhase: workflow.Phase, TaskDelivered: run.TaskStatus == "closed",
		ConfigurationDigest: execution.ConfigurationDigest,
		ResourceGeneration:  execution.Generation,
		Implementation:      cloneImplementationData(execution.ImplementationData),
		WorkspaceDirty:      execution.Workspace.Dirty,
		Workspace:           executionapi.WorkspaceIntent{Path: execution.Workspace.Worktree, Branch: execution.Workspace.Branch, BaseCommit: execution.Workspace.Base},
		OutputCursor:        attempt.OutputCursor, PendingTurn: pending,
		PendingDeliveryID: execution.PendingDeliveryID,
	}
	if execution.PolicyResources != nil {
		resource := execution.PolicyResources.ImplementationData.Clone()
		snapshot.PolicyResources = &resource
	}
	if execution.Environment != nil && execution.Environment.Preparation != nil && execution.Environment.Preparation.Probe.Nonce != "" {
		probe := execution.Environment.Preparation.Probe
		snapshot.PreparationProbe = &probe
	}
	if execution.Environment != nil && execution.Environment.OriginalTerminal != nil {
		identity := *execution.Environment.OriginalTerminal
		snapshot.OriginalHost = &identity
		if snapshot.Ref.External.Kind == "" {
			snapshot.Ref.External = identity
		}
	}
	if run.Decision != nil && run.Decision.Request.HostTarget != nil && execution.Environment != nil && execution.Environment.Blocker != nil {
		target := run.Decision.Request.HostTarget
		blocker := execution.Environment.Blocker
		if target.ExecutionID == execution.ExecutionID && target.TurnID == blocker.TurnID && target.BlockerID == blocker.ID && run.Decision.ID == blocker.DecisionID {
			snapshot.HostAuthorization = &executionapi.HostActionAuthorization{ExecutionID: execution.ExecutionID, AttemptID: run.AttemptID, TurnID: target.TurnID, DecisionID: run.Decision.ID, BlockerID: target.BlockerID, DecisionVersion: run.Decision.Version}
		}
	}
	if run.WorkspaceIntent != nil {
		snapshot.WorkspacePlan = *run.WorkspaceIntent
		if snapshot.Ref.BaseCommit == "" {
			snapshot.Ref.BaseCommit = run.WorkspaceIntent.BaseCommit
		}
	}
	if run.Decision != nil && run.Decision.Request.QuestionRoute != nil {
		snapshot.KnownQuestionIDs = append(snapshot.KnownQuestionIDs, run.Decision.Request.QuestionRoute.QuestionID)
		route := run.Decision.Request.QuestionRoute.Clone()
		snapshot.QuestionRoute = &route
	}
	for _, question := range execution.PendingQuestions {
		snapshot.KnownQuestionIDs = append(snapshot.KnownQuestionIDs, question.Route.QuestionID)
	}
	return snapshot.Clone(), nil
}

func (s *Service) lifecycleSnapshot(run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, attempt *StepAttemptRecord, pending *executionapi.TurnInput) (executionapi.LifecycleSnapshot, error) {
	snapshot, err := lifecycleSnapshot(run, workflow, execution, attempt, pending)
	if err != nil {
		return executionapi.LifecycleSnapshot{}, err
	}
	for _, record := range s.domain.Operations {
		if !strings.Contains(record.IdempotencyKey, execution.ExecutionID) {
			continue
		}
		if !strings.Contains(record.IdempotencyKey, "worker-lifecycle:") &&
			!strings.HasPrefix(record.IdempotencyKey, "worker-session:") &&
			!strings.HasPrefix(record.IdempotencyKey, "worker-turn:") &&
			!strings.HasPrefix(record.IdempotencyKey, "worker-input:") &&
			!strings.HasPrefix(record.IdempotencyKey, "worker-question-answer:") &&
			!strings.HasPrefix(record.IdempotencyKey, "worker-delivery-ack:") {
			continue
		}
		action := executionapi.LifecycleAction(record.Kind)
		switch record.Kind {
		case "prepare_worker_session":
			action = executionapi.LifecyclePrepareSession
		case "start_worker_turn":
			action = executionapi.LifecycleStartTurn
		case "submit_worker_input":
			action = executionapi.LifecycleSubmitInput
		case string(executionapi.LifecycleReportCompletion):
			action = executionapi.LifecycleReportCompletion
		case "reply_execution_question":
			action = executionapi.LifecycleDeliverFeedback
		case "worker_delivery_ack":
			action = executionapi.LifecycleAcknowledge
		}
		if !knownLifecycleAction(action) {
			continue
		}
		if identity := record.LifecycleIdentity; identity == nil || identity.ExecutionID != execution.ExecutionID || identity.Ref.AttemptID != run.AttemptID {
			continue
		}
		attemptID := run.AttemptID
		operation := executionapi.Operation{
			Version: executionapi.ExecutionRecordVersion, ID: record.OperationID, RunID: run.RunID, AttemptID: attemptID,
			Backend: executionapi.BackendKind(execution.Backend), Protocol: executionapi.ExecutionProtocol(execution.Protocol), Target: execution.Target,
			Action: string(action), IdempotencyKey: record.IdempotencyKey, Request: append([]byte(nil), record.Request...), Receipt: append([]byte(nil), record.Receipt...), PayloadDigest: record.PayloadDigest,
			Status: executionapi.OperationStatus(record.Status), StartedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
			Ref: &snapshot.Ref,
		}
		bindOriginalLifecycleIdentity(&operation, record)
		snapshot.Operations = append(snapshot.Operations, operation)
	}
	return snapshot.Clone(), nil
}

func knownLifecycleAction(action executionapi.LifecycleAction) bool {
	switch action {
	case executionapi.LifecyclePrepareWorkspace, executionapi.LifecycleCreateResource, executionapi.LifecyclePrepareSession,
		executionapi.LifecycleStartTurn, executionapi.LifecycleSubmitInput, executionapi.LifecycleReportCompletion, executionapi.LifecycleDeliverFeedback,
		executionapi.LifecycleAcknowledge, executionapi.LifecycleStopObservation, executionapi.LifecycleStopExecution, executionapi.LifecycleCleanup, executionapi.LifecycleHostAction:
		return true
	default:
		return false
	}
}

func unresolvedLifecycleCandidate(snapshot executionapi.LifecycleSnapshot) (executionapi.LifecycleCandidate, bool, error) {
	for _, operation := range snapshot.Operations {
		if operation.Status != executionapi.OperationInFlight && operation.Status != executionapi.OperationUnknown {
			continue
		}
		candidate, err := executionapi.CandidateForOperation(operation)
		return candidate, err == nil, err
	}
	return executionapi.LifecycleCandidate{}, false, nil
}

func lifecycleRequest(snapshot executionapi.LifecycleSnapshot, operation OperationRecord, action executionapi.LifecycleAction, input executionapi.LifecycleInput) executionapi.LifecycleRequest {
	request := executionapi.LifecycleRequest{
		Snapshot: snapshot.Clone(),
		Operation: executionapi.Operation{
			Version: executionapi.ExecutionRecordVersion, ID: operation.OperationID,
			RunID: snapshot.Session.WorkflowRunID, AttemptID: snapshot.Session.AttemptID,
			Backend: snapshot.Ref.Backend, Protocol: snapshot.Ref.Protocol, Target: snapshot.Ref.Target,
			Action: string(action), PayloadDigest: operation.PayloadDigest,
			IdempotencyKey: operation.IdempotencyKey, Request: append([]byte(nil), operation.Request...), Receipt: append([]byte(nil), operation.Receipt...),
			Status: executionapi.OperationStatus(operation.Status), StartedAt: operation.CreatedAt,
			UpdatedAt: operation.UpdatedAt, Ref: &snapshot.Ref,
		},
		Input: input.Clone(),
	}
	bindOriginalLifecycleIdentity(&request.Operation, operation)
	return request
}

func bindOriginalLifecycleIdentity(operation *executionapi.Operation, record OperationRecord) {
	if record.LifecycleIdentity == nil {
		return
	}
	identity := *record.LifecycleIdentity
	operation.LifecycleIdentity = &identity
	ref := identity.Ref
	operation.Ref, operation.RunID, operation.AttemptID = &ref, ref.RunID, ref.AttemptID
	operation.Backend, operation.Protocol, operation.Target = ref.Backend, ref.Protocol, ref.Target
}

func (s *Service) validateLifecycleResultBinding(run *Run, request executionapi.LifecycleRequest) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	current, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	if current.Session.ProjectID != request.Snapshot.Session.ProjectID || current.Session.ExecutionID != request.Snapshot.Session.ExecutionID ||
		current.Session.AttemptID != request.Snapshot.Session.AttemptID || current.ConfigurationDigest != request.Snapshot.ConfigurationDigest ||
		current.ResourceGeneration != request.Snapshot.ResourceGeneration ||
		executionapi.LifecycleBindingDigest(current) != executionapi.LifecycleBindingDigest(request.Snapshot) {
		return ErrWorkerTurnStale
	}
	return nil
}

func lifecycleTurnInput(commandID, prompt string, linkedArgs []string) executionapi.TurnInput {
	return executionapi.TurnInput{CommandID: commandID, Prompt: prompt, Purpose: "business", LinkedWorktreeArgs: append([]string(nil), linkedArgs...)}
}

func (s *Service) originalLifecycleRequest(run *Run, fixed workerFlowContext) (executionapi.LifecycleRequest, error) {
	commandID := fixed.execution.CurrentCommandID
	if fixed.execution.PendingCommandID != "" {
		commandID = fixed.execution.PendingCommandID
	}
	if commandID == "" {
		return executionapi.LifecycleRequest{}, errors.New("fixed execution has no original command ID")
	}
	turnKey := "worker-turn:" + fixed.execution.ExecutionID + ":" + commandID
	sessionKey := "worker-session:" + fixed.execution.ExecutionID + ":" + commandID
	operation := s.operationByKey(turnKey)
	action := executionapi.LifecycleStartTurn
	if operation == nil {
		operation = s.operationByKey(sessionKey)
		action = executionapi.LifecyclePrepareSession
	}
	if operation == nil {
		return executionapi.LifecycleRequest{}, errors.New("fixed worker input has no durable lifecycle operation")
	}
	var input executionapi.TurnInput
	if err := json.Unmarshal(operation.Request, &input); err != nil {
		return executionapi.LifecycleRequest{}, err
	}
	if input.CommandID != commandID || input.Prompt == "" {
		return executionapi.LifecycleRequest{}, errors.New("fixed lifecycle operation has a damaged or changed input")
	}
	if err := validateSavedLifecycleOperation(operation, action, executionapi.LifecycleInput{Turn: &input}); err != nil {
		return executionapi.LifecycleRequest{}, err
	}
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	snapshot, err := s.lifecycleSnapshot(run, workflow, execution, attempt, &input)
	if err != nil {
		return executionapi.LifecycleRequest{}, err
	}
	return lifecycleRequest(snapshot, *operation, action, executionapi.LifecycleInput{Turn: &input}), nil
}

func mergeLifecycleExecutionReceipt(execution *ExecutionRecord, receipt executionapi.LifecycleReceipt) error {
	if execution == nil {
		return errors.New("lifecycle receipt has no fixed execution record")
	}
	// A rejected receipt must not leave even a partial private candidate which
	// a later diagnostic commit could accidentally publish.
	var candidate ExecutionRecord
	if err := json.Unmarshal(mustJSON(execution), &candidate); err != nil {
		return err
	}
	if err := mergeLifecycleExecutionCandidate(&candidate, receipt); err != nil {
		return err
	}
	// Preparation coordination keeps these actor-owned pointers while applying
	// several receipts. Publish the valid candidate into them only now.
	if execution.Environment != nil && candidate.Environment != nil {
		if execution.Environment.Preparation != nil && candidate.Environment.Preparation != nil {
			*execution.Environment.Preparation = *candidate.Environment.Preparation
			candidate.Environment.Preparation = execution.Environment.Preparation
		}
		*execution.Environment = *candidate.Environment
		candidate.Environment = execution.Environment
	}
	*execution = candidate
	return nil
}

func mergeLifecycleExecutionCandidate(execution *ExecutionRecord, receipt executionapi.LifecycleReceipt) error {
	if receipt.Target != nil {
		current, next := execution.Target, *receipt.Target
		if next.ID != current.ID || next.ConnectionID != current.ConnectionID || next.ExternalRuntime != current.ExternalRuntime {
			return ErrWorkerTurnStale
		}
		for _, field := range [][2]string{{current.WorktreeID, next.WorktreeID},
			{current.CoordinatorTerminal, next.CoordinatorTerminal}, {current.OrchestrationRun, next.OrchestrationRun}, {current.ExecutionHost, next.ExecutionHost}} {
			if field[0] != "" && field[0] != field[1] {
				return ErrWorkerTurnStale
			}
		}
		if current.WorktreeID != "" && (current.Branch != next.Branch || current.BaseRevision != next.BaseRevision) {
			return ErrWorkerTurnStale
		}
		execution.Target = next
	}
	if receipt.Workspace != nil {
		if execution.Workspace.Base != "" && execution.Workspace.Base != receipt.Workspace.Base {
			return ErrWorkerTurnStale
		}
		workspace := receipt.Workspace
		if !execution.Workspace.VerifiedAt.IsZero() && execution.Workspace.Worktree != "" && (canonical(execution.Workspace.Worktree) != canonical(workspace.Worktree) || execution.Workspace.Branch != workspace.Branch) {
			return ErrWorkerTurnStale
		}
		if workspace.Worktree == "" || workspace.Branch == "" || workspace.Base == "" || workspace.Head == "" {
			return errors.New("lifecycle workspace receipt is incomplete")
		}
		execution.Workspace = WorkspaceEvidence{Repository: workspace.Repository, Worktree: workspace.Worktree, Branch: workspace.Branch,
			Base: workspace.Base, Head: workspace.Head, Dirty: workspace.Dirty, VerifiedAt: workspace.CheckedAt}
	}
	if receipt.State.WorktreeID != "" {
		if execution.Target.WorktreeID != "" && execution.Target.WorktreeID != receipt.State.WorktreeID {
			return ErrWorkerTurnStale
		}
		execution.Target.WorktreeID = receipt.State.WorktreeID
	}
	if receipt.OriginalHost != nil && execution.Environment != nil {
		original := *receipt.OriginalHost
		if execution.Environment.OriginalTerminal != nil && !sameExecutionSessionProcess(*execution.Environment.OriginalTerminal, original) {
			return ErrWorkerTurnStale
		}
		execution.Environment.OriginalTerminal = &original
		execution.Environment.Runtime = original.RuntimeID
	}
	if receipt.Process != nil {
		if execution.Process != nil && *execution.Process != *receipt.Process && !sameExecutionSessionProcess(*execution.Process, *receipt.Process) {
			return ErrWorkerTurnStale
		}
		process := *receipt.Process
		execution.Process = &process
		if execution.Environment != nil {
			if execution.Environment.OriginalTerminal != nil && !sameExecutionSessionProcess(*execution.Environment.OriginalTerminal, process) {
				return ErrWorkerTurnStale
			}
			execution.Environment.OriginalTerminal = &process
			execution.Environment.Runtime = process.RuntimeID
		}
	}
	if receipt.State.SessionID != "" {
		if execution.SessionID != "" && execution.SessionID != receipt.State.SessionID {
			return ErrWorkerTurnStale
		}
		execution.SessionID = receipt.State.SessionID
	}
	if receipt.State.SessionGeneration != "" {
		if execution.SessionGeneration != "" && execution.SessionGeneration != receipt.State.SessionGeneration {
			return ErrWorkerTurnStale
		}
		execution.SessionGeneration = receipt.State.SessionGeneration
	}
	if receipt.State.TurnID != "" {
		if execution.CurrentTurnID == "" && execution.PendingTurnID != "" && execution.PendingTurnID != receipt.State.TurnID {
			return ErrWorkerTurnStale
		}
		if execution.CurrentTurnID == "" {
			execution.PendingTurnID = receipt.State.TurnID
		}
	}
	for _, envelope := range receipt.Implementation {
		if err := envelope.ValidateResourceIdentity(execution.ProjectIdentity, execution.ExecutionID, execution.CurrentAttemptID, execution.Generation, execution.ConfigurationDigest); err != nil {
			return err
		}
		if err := putImplementationEnvelope(execution, envelope); err != nil {
			return err
		}
	}
	if execution.Environment != nil && execution.Environment.OriginalTerminal != nil && execution.PolicyResources != nil && receipt.State.LaunchCommand != "" {
		execution.Environment.LaunchCommand = receipt.State.LaunchCommand
		execution.Environment.WriteRoots = append([]string(nil), execution.PolicyResources.WriteRoots...)
	}
	return nil
}

func sameExecutionSessionProcess(a, b executionapi.ProcessIdentity) bool {
	return a.Kind == b.Kind && a.PID == b.PID && a.Created == b.Created && a.Executable == b.Executable &&
		a.Terminal == b.Terminal && a.RuntimeID == b.RuntimeID && a.Incarnation == b.Incarnation &&
		a.CoordinatorRuntimeID == b.CoordinatorRuntimeID && a.ExecutionHost == b.ExecutionHost && a.WorktreeID == b.WorktreeID
}

func (s *Service) startWorkerLifecycle(ctx context.Context, run *Run, lifecycle executionapi.LifecyclePort, fixed workerFlowContext, commandID, prompt string) error {
	if fixed.workflow.ExecutionProfile == nil {
		return errors.New("worker lifecycle has no fixed accepted execution profile")
	}
	if fixed.execution.ConfigurationDigest != "" && fixed.execution.ConfigurationDigest != fixed.workflow.ExecutionProfile.Digest {
		return errors.New("worker lifecycle configuration identity differs from the accepted profile")
	}
	input := lifecycleTurnInput(commandID, prompt, nil)
	if fixed.execution.Environment != nil && fixed.execution.Environment.Preparation != nil && fixed.execution.Environment.Preparation.TurnID == commandID {
		input.Purpose = "readiness"
	}
	policyCapabilities := lifecycle.PolicyCapabilities()
	if policyCapabilities.LinkedWorkspaceWriteRoots {
		args, err := linkedWorktreeWriteArgs(ctx, fixed.config, fixed.execution.Workspace.Worktree)
		if err != nil {
			return err
		}
		input.LinkedWorktreeArgs = args
		if err := bindPolicyResources(ctx, fixed.config, fixed.execution, fixed.execution.Workspace.Worktree, input.LinkedWorktreeArgs); err != nil {
			return err
		}
	}
	// Resource creation may precede the start operation. Preserve the complete
	// original input (including its write grant) before any such mutation.
	inputKey := "worker-input-value:" + fixed.execution.ExecutionID + ":" + commandID
	if saved := s.operationByKey(inputKey); saved != nil {
		if saved.Kind != "worker_input_value" || (saved.Status != "confirmed" && saved.Status != "in_flight") || saved.LifecycleIdentity == nil || saved.LifecycleIdentity.ExecutionID != run.ExecutionID || saved.LifecycleIdentity.Ref.AttemptID != run.AttemptID || saved.LifecycleIdentity.ConfigurationDigest != fixed.execution.ConfigurationDigest || saved.LifecycleIdentity.ResourceGeneration != fixed.execution.Generation || saved.PayloadDigest != operationDigest(saved.Request) || !bytes.Equal(saved.Request, mustJSON(&input)) {
			return errors.New("pending worker input differs from its original committed value")
		}
		if saved.Status == "in_flight" {
			if err := s.changeWorkerOperation(run, saved.OperationID, "confirmed", commandID, mustJSON(&input), ""); err != nil {
				return err
			}
		}
	} else {
		inputID, err := s.startWorkerOperation(run, "worker_input_value", inputKey, &input)
		if err != nil {
			return err
		}
		if err := s.changeWorkerOperation(run, inputID, "confirmed", commandID, mustJSON(&input), ""); err != nil {
			return err
		}
	}

	var candidate executionapi.LifecycleCandidate
	var snapshot executionapi.LifecycleSnapshot
	var err error
	for step := 0; step < 16; step++ {
		fixed, err = s.workerFlowContext(run)
		if err != nil {
			return err
		}
		snapshot, err = s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, &input)
		if err != nil {
			return err
		}
		var found bool
		candidate, found, err = unresolvedLifecycleCandidate(snapshot)
		if err != nil {
			return err
		}
		if !found {
			candidate, err = lifecycle.Next(ctx, snapshot.Clone())
			if err != nil {
				return err
			}
		}
		if err = candidate.Validate(); err != nil {
			return err
		}
		if candidate.Done {
			if s.workers[run.RunID] != nil {
				return nil
			}
			if fixed.execution.Environment != nil && fixed.execution.Environment.Preparation != nil && fixed.execution.Environment.Preparation.TurnID == commandID {
				return nil
			}
			return errors.New("lifecycle planner completed without a startable turn")
		}
		resourceStep := candidate.Action == executionapi.LifecycleCreateResource
		turnStep := policyCapabilities.SeparateTurnStart && (candidate.Action == executionapi.LifecycleStartTurn || candidate.Action == executionapi.LifecycleSubmitInput)
		if !resourceStep && !turnStep {
			break
		}
		if candidate.Input.Turn != nil && (candidate.Input.Turn.CommandID != input.CommandID || candidate.Input.Turn.Prompt != input.Prompt || !sameLifecycleStrings(candidate.Input.Turn.LinkedWorktreeArgs, input.LinkedWorktreeArgs)) {
			return errors.New("lifecycle planner changed the saved business input or resource grant")
		}
		if err := s.executeLifecycleStep(ctx, run, lifecycle, candidate); err != nil {
			if errors.Is(err, errLifecycleNeedsReconciliation) {
				return nil
			}
			return err
		}
		if s.workers[run.RunID] != nil {
			return nil
		}
	}
	if candidate.Input.Turn == nil || candidate.Input.Turn.CommandID != input.CommandID || candidate.Input.Turn.Prompt != input.Prompt ||
		!sameLifecycleStrings(candidate.Input.Turn.LinkedWorktreeArgs, input.LinkedWorktreeArgs) {
		return errors.New("lifecycle planner changed the saved business input or resource grant")
	}
	switch candidate.Action {
	case executionapi.LifecyclePrepareSession:
		key := "worker-session:" + fixed.execution.ExecutionID + ":" + commandID
		prior := s.operationByKey(key)
		opID, err := s.startWorkerOperation(run, "prepare_worker_session", key, candidate.Input.Turn)
		if err != nil {
			return err
		}
		operation := s.operationByKey(key)
		if operation == nil || operation.OperationID != opID {
			return errors.New("app-server session preparation lost its saved operation")
		}
		if err = validateSavedLifecycleOperation(operation, candidate.Action, candidate.Input); err != nil {
			return err
		}
		request := lifecycleRequest(snapshot, *operation, candidate.Action, candidate.Input)
		if err = request.Validate(); err != nil {
			return err
		}
		if s.workerMessages != nil {
			if s.appServerLaunching == nil {
				s.appServerLaunching = make(map[string]string)
			}
			s.appServerLaunching[run.RunID] = commandID
			ctx = s.runContext
			s.forwarders.Add(1)
			go func() {
				defer s.forwarders.Done()
				s.launchAppServerLifecycle(ctx, run.RunID, lifecycle, request, prior != nil)
			}()
			return nil
		}
		var receipt executionapi.LifecycleReceipt
		var observer executionapi.LifecycleObserver
		var executeErr error
		if prior != nil {
			receipt, observer, executeErr = lifecycle.Reconcile(ctx, request.Clone())
		} else {
			receipt, observer, executeErr = lifecycle.Execute(ctx, request.Clone())
		}
		startRequest, proceed, applyErr := s.applyAppServerSessionReceipt(run, request, receipt, executeErr)
		if applyErr != nil {
			if observer != nil {
				_ = observer.Close(context.Background())
			}
			return applyErr
		}
		if !proceed {
			return nil
		}
		startReceipt, startObserver, startErr := lifecycle.Execute(ctx, startRequest.Clone())
		return s.applyAppServerTurnReceipt(run, startRequest, startReceipt, startObserver, startErr)
	case executionapi.LifecycleStartTurn:
		key := "worker-turn:" + fixed.execution.ExecutionID + ":" + commandID
		prior := s.operationByKey(key)
		opID, err := s.startWorkerOperation(run, "start_worker_turn", key, candidate.Input.Turn)
		if err != nil {
			return err
		}
		operation := s.operationByKey(key)
		if operation == nil || operation.OperationID != opID {
			return errors.New("app-server turn start lost its saved operation")
		}
		if err = validateSavedLifecycleOperation(operation, candidate.Action, candidate.Input); err != nil {
			return err
		}
		request := lifecycleRequest(snapshot, *operation, candidate.Action, candidate.Input)
		if err = request.Validate(); err != nil {
			return err
		}
		if s.workerMessages != nil {
			if s.appServerLaunching == nil {
				s.appServerLaunching = make(map[string]string)
			}
			s.appServerLaunching[run.RunID] = commandID
			ctx = s.runContext
			s.forwarders.Add(1)
			go func() {
				defer s.forwarders.Done()
				s.launchAppServerLifecycle(ctx, run.RunID, lifecycle, request, prior != nil)
			}()
			return nil
		}
		var receipt executionapi.LifecycleReceipt
		var observer executionapi.LifecycleObserver
		var executeErr error
		if prior != nil {
			receipt, observer, executeErr = lifecycle.Reconcile(ctx, request.Clone())
		} else {
			receipt, observer, executeErr = lifecycle.Execute(ctx, request.Clone())
		}
		return s.applyAppServerTurnReceipt(run, request, receipt, observer, executeErr)
	default:
		return errors.New("lifecycle planner proposed an action outside the authorized worker turn")
	}
}

func (s *Service) interruptOwnedLocalTurn(runID string, owned executionapi.StopHandle) {
	if owned == nil {
		return
	}
	var run *Run
	for _, candidate := range s.runs {
		if candidate != nil && candidate.RunID == runID {
			run = candidate
			break
		}
	}
	if run == nil || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return
	}
	execution := s.executionRecord(run)
	if execution == nil || execution.Process == nil || *execution.Process != owned.Identity() || execution.CurrentTurnID == "" || execution.CurrentTurnID == execution.LastStoppedTurnID {
		return
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return
	}
	key := "worker-stop-observation:" + execution.ExecutionID + ":" + execution.CurrentTurnID
	if operation := s.operationByKey(key); operation != nil {
		// Even an unknown stop is reconciled later; shutdown never resends its interrupt.
		return
	}
	input := executionapi.LifecycleInput{Stop: &executionapi.StopInput{TurnID: execution.CurrentTurnID}}
	opID, err := s.startWorkerOperation(run, "stop_worker_observation", key, input.Stop)
	if err != nil {
		return
	}
	operation := s.operationByKey(key)
	if operation == nil || operation.OperationID != opID || validateSavedLifecycleOperation(operation, executionapi.LifecycleStopObservation, input) != nil {
		return
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, execution, fixed.attempt, nil)
	if err != nil {
		return
	}
	request := lifecycleRequest(snapshot, *operation, executionapi.LifecycleStopObservation, input)
	receipt, err := lifecycle.StopLifecycle(context.Background(), request, owned)
	if receipt.ValidateFor(request) != nil {
		return
	}
	status := string(receipt.Disposition)
	if err != nil || status != "accepted" && status != "confirmed" {
		status = "outcome_unknown"
	}
	if s.setOperationState(opID, status, receipt.RequestID, mustJSON(receipt), receipt.Diagnostic) != nil {
		return
	}
	_ = s.commitWorker(run, opID)
}

func validateSavedLifecycleOperation(operation *OperationRecord, action executionapi.LifecycleAction, input executionapi.LifecycleInput) error {
	if operation == nil {
		return errors.New("lifecycle operation is missing")
	}
	if err := input.Validate(action); err != nil {
		return err
	}
	var value any
	switch action {
	case executionapi.LifecycleCreateResource:
		value = input.Resource
	case executionapi.LifecyclePrepareSession, executionapi.LifecycleStartTurn, executionapi.LifecycleSubmitInput:
		value = input.Turn
	case executionapi.LifecycleReportCompletion:
		value = input.Completion
	case executionapi.LifecycleDeliverFeedback:
		value = input.Feedback
	case executionapi.LifecycleAcknowledge:
		value = input.Ack
	case executionapi.LifecycleStopExecution, executionapi.LifecycleStopObservation:
		value = input.Stop
	case executionapi.LifecycleCleanup:
		value = input.Cleanup
	case executionapi.LifecycleHostAction:
		value = input.Host
	case executionapi.LifecyclePrepareWorkspace:
		value = input.Workspace
	default:
		return errors.New("unsupported lifecycle operation action")
	}
	wire, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(operation.Request, wire) || operation.PayloadDigest != operationDigest(wire) {
		return errors.New("lifecycle operation input differs from its original durable intent")
	}
	return nil
}

func lifecycleInputValue(action executionapi.LifecycleAction, input executionapi.LifecycleInput) (any, error) {
	if err := input.Validate(action); err != nil {
		return nil, err
	}
	switch action {
	case executionapi.LifecyclePrepareWorkspace:
		return input.Workspace, nil
	case executionapi.LifecycleCreateResource:
		return input.Resource, nil
	case executionapi.LifecyclePrepareSession, executionapi.LifecycleStartTurn, executionapi.LifecycleSubmitInput:
		return input.Turn, nil
	case executionapi.LifecycleReportCompletion:
		return input.Completion, nil
	case executionapi.LifecycleDeliverFeedback:
		return input.Feedback, nil
	case executionapi.LifecycleAcknowledge:
		return input.Ack, nil
	case executionapi.LifecycleStopObservation, executionapi.LifecycleStopExecution:
		return input.Stop, nil
	case executionapi.LifecycleCleanup:
		return input.Cleanup, nil
	case executionapi.LifecycleHostAction:
		return input.Host, nil
	default:
		return nil, errors.New("unsupported lifecycle operation action")
	}
}

func lifecycleOperationKey(executionID string, action executionapi.LifecycleAction, input executionapi.LifecycleInput) (string, any, error) {
	value, err := lifecycleInputValue(action, input)
	if err != nil {
		return "", nil, err
	}
	wire, err := json.Marshal(value)
	if err != nil {
		return "", nil, err
	}
	return "worker-lifecycle:" + executionID + ":" + string(action) + ":" + operationDigest(wire), value, nil
}

func (s *Service) executeLifecycleStep(ctx context.Context, run *Run, lifecycle executionapi.LifecyclePort, candidate executionapi.LifecycleCandidate) error {
	if err := candidate.Validate(); err != nil || candidate.Done {
		return errors.Join(err, errors.New("lifecycle step has no executable action"))
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	key, value, err := lifecycleOperationKey(fixed.execution.ExecutionID, candidate.Action, candidate.Input)
	if err != nil {
		return err
	}
	kind := string(candidate.Action)
	if candidate.Action == executionapi.LifecycleStartTurn {
		key, kind, value = "worker-turn:"+fixed.execution.ExecutionID+":"+candidate.Input.Turn.CommandID, "start_worker_turn", candidate.Input.Turn
	} else if candidate.Action == executionapi.LifecycleSubmitInput {
		key, kind, value = "worker-input:"+fixed.execution.ExecutionID+":"+candidate.Input.Turn.CommandID, "submit_worker_input", candidate.Input.Turn
	}
	prior := s.operationByKey(key)
	var opID string
	if prior != nil && prior.Status != "pending" {
		opID = prior.OperationID
	} else {
		opID, err = s.startWorkerOperation(run, kind, key, value)
		if err != nil {
			return err
		}
	}
	operation := s.operationByKey(key)
	if operation == nil || operation.OperationID != opID {
		return errors.New("lifecycle intent was not retained under its original identity")
	}
	if operation.Status == "confirmed" {
		return nil
	}
	if prior != nil && operation.Status == "rejected" {
		return errors.New("original lifecycle operation was rejected; a new user-authorized intent is required")
	}
	input := candidate.Input.Clone()
	if err := validateSavedLifecycleOperation(operation, candidate.Action, input); err != nil {
		return err
	}
	if candidate.Action == executionapi.LifecycleCreateResource &&
		lifecycle.PolicyCapabilities().LinkedWorkspaceWriteRoots && fixed.execution.PolicyResources == nil {
		var linkedArgs []string
		if fixed.config.ExecutionPolicy.Filesystem.WriteScope == "workspace" {
			linkedArgs, err = linkedWorktreeWriteArgs(ctx, fixed.config, fixed.execution.Workspace.Worktree)
			if err != nil {
				return err
			}
		}
		if err = bindPolicyResources(ctx, fixed.config, fixed.execution, fixed.execution.Workspace.Worktree, linkedArgs); err != nil {
			return err
		}
		if err = s.commitWorker(run); err != nil {
			return fmt.Errorf("worker policy resources could not be committed before agent launch: %w", err)
		}
		fixed, err = s.workerFlowContext(run)
		if err != nil {
			return err
		}
	}
	var pending *executionapi.TurnInput
	if candidate.Input.Turn != nil && (candidate.Action == executionapi.LifecycleStartTurn || candidate.Action == executionapi.LifecycleSubmitInput) {
		pending = candidate.Input.Clone().Turn
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, pending)
	if err != nil {
		return err
	}
	request := lifecycleRequest(snapshot, *operation, candidate.Action, input)
	if err := request.Validate(); err != nil {
		return err
	}
	var receipt executionapi.LifecycleReceipt
	var observer executionapi.LifecycleObserver
	var callErr error
	reconciled := false
	if prior != nil && (operation.Status == "pending" || operation.Status == "in_flight" || operation.Status == "accepted" || operation.Status == "outcome_unknown") {
		receipt, observer, callErr = lifecycle.Reconcile(ctx, request.Clone())
		reconciled = true
	} else {
		receipt, observer, callErr = lifecycle.Execute(ctx, request.Clone())
	}
	hadUnknownMutation := !reconciled && (receipt.Disposition == executionapi.OperationUnknown || callErr != nil && receipt.Disposition != executionapi.OperationRejected)
	if hadUnknownMutation {
		if observer != nil {
			_ = observer.Close(context.Background())
			observer = nil
		}
		reconciledReceipt, reconciledObserver, reconcileErr := lifecycle.Reconcile(ctx, request.Clone())
		if validationErr := reconciledReceipt.ValidateFor(request); validationErr != nil {
			return validationErr
		}
		if reconcileErr == nil && reconciledReceipt.Disposition != executionapi.OperationUnknown {
			receipt, observer, callErr = reconciledReceipt, reconciledObserver, nil
		} else {
			if reconciledObserver != nil {
				_ = reconciledObserver.Close(context.Background())
			}
			if reconcileErr != nil {
				callErr = errors.Join(callErr, reconcileErr)
			}
		}
	}
	if observer != nil && candidate.Action != executionapi.LifecycleStartTurn && candidate.Action != executionapi.LifecycleSubmitInput {
		_ = observer.Close(context.Background())
		return errors.New("resource lifecycle step unexpectedly returned an observer")
	}
	if err := receipt.ValidateFor(request); err != nil {
		return err
	}
	previousProcess := fixed.execution.Process
	if err := mergeLifecycleExecutionReceipt(fixed.execution, receipt); err != nil {
		return errors.Join(s.waitWorkerFlow(run, "lifecycle receipt changed the fixed resource identity: "+err.Error()), err)
	}
	if receipt.Process != nil && fixed.execution.PolicyResources == nil && lifecycle.PolicyCapabilities().LinkedWorkspaceWriteRoots {
		var linkedArgs []string
		if fixed.config.ExecutionPolicy.Filesystem.WriteScope == "workspace" {
			linkedArgs, err = linkedWorktreeWriteArgs(ctx, fixed.config, fixed.execution.Workspace.Worktree)
			if err != nil {
				return err
			}
		}
		if err = bindPolicyResources(ctx, fixed.config, fixed.execution, fixed.execution.Workspace.Worktree, linkedArgs); err != nil {
			return err
		}
	}
	if receipt.Workspace != nil {
		applyWorkflowWorkspaceEvidence(run, fixed.execution, fixed.execution.Workspace)
	}
	status := string(receipt.Disposition)
	diagnostic := receipt.Diagnostic
	if callErr != nil {
		diagnostic = callErr.Error()
		if receipt.Disposition != executionapi.OperationRejected {
			status = "outcome_unknown"
		}
	} else if status == "accepted" && receipt.State.ResourceID == "" && receipt.State.WorktreeID == "" && receipt.Process == nil {
		status, diagnostic = "outcome_unknown", "resource lifecycle receipt did not identify its accepted resource"
	}
	if err := s.setOperationState(opID, status, receipt.RequestID, mustJSON(receipt), diagnostic); err != nil {
		return err
	}
	if status == "accepted" && (candidate.Action == executionapi.LifecycleStartTurn || candidate.Action == executionapi.LifecycleSubmitInput) {
		if receipt.State.TurnID != "" {
			fixed.execution.PendingTurnID = receipt.State.TurnID
		}
		fixed.execution.Status = "pending"
		fixed.attempt.Status, fixed.attempt.UpdatedAt = "dispatched", now()
		s.refreshRun(run)
	}
	var worker *Worker
	if observer != nil {
		if !receipt.State.InputAccepted || receipt.State.TurnID == "" || receipt.Process == nil {
			_ = observer.Close(context.Background())
			return errors.New("lifecycle observer lacks a confirmed original turn receipt")
		}
		worker = workerFromLifecycle(run.RunID, run.AttemptID, observer)
		if worker == nil || worker.proc == nil || worker.proc.Identity() != *receipt.Process {
			if worker != nil {
				_ = worker.detachLocalObservation(context.Background())
			}
			return ErrWorkerTurnStale
		}
		fixed.execution.PendingTurnID = receipt.State.TurnID
		fixed.execution.Status = "pending"
		fixed.attempt.Status, fixed.attempt.UpdatedAt = "dispatched", now()
		s.refreshRun(run)
	}
	if s.pendingOperations == nil {
		s.pendingOperations = make(map[string]string)
	}
	s.pendingOperations[opID] = run.RunID
	s.refreshRun(run)
	var attachment workerCommitAttachment
	if previousProcess == nil && fixed.execution.Process != nil && fixed.execution.SessionGeneration != "" {
		ref := lifecycleExecutionRef(run, fixed.execution, fixed.execution.SessionGeneration)
		attachment.Binding = &ref
	}
	if err := s.commitWorkerAttached(run, attachment, opID); err != nil {
		if worker != nil {
			_ = worker.detachLocalObservation(context.Background())
		}
		return fmt.Errorf("lifecycle result could not be committed; only reconcile the original operation: %w", err)
	}
	if worker != nil {
		s.registerWorker(run, worker)
	}
	if hadUnknownMutation && fixed.execution.Environment != nil {
		// The original identity can be reconciled while still preserving the
		// explicit host recheck which an uncertain preparation requires.
		return errors.Join(s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "resource_preparation_unverified", Confidence: "unverified", Source: "lifecycle.reconciliation", Detail: "Original operation was reconciled after losing its mutation receipt; confirm the retained environment before more input."}), errLifecycleNeedsReconciliation)
	}
	if callErr != nil || status == "outcome_unknown" || status == "rejected" {
		if candidate.Action == executionapi.LifecycleCleanup {
			return errors.New(diagnostic)
		}
		return errors.Join(s.waitWorkerFlow(run, diagnostic), errLifecycleNeedsReconciliation)
	}
	return nil
}

func (s *Service) prepareLifecycleResources(ctx context.Context, run *Run, lifecycle executionapi.LifecyclePort) error {
	for step := 0; step < 16; step++ {
		fixed, err := s.workerFlowContext(run)
		if err != nil {
			return err
		}
		snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
		if err != nil {
			return err
		}
		pending, hasPending, err := unresolvedLifecycleCandidate(snapshot)
		if err != nil {
			return err
		}
		if hasPending {
			if err := s.executeLifecycleStep(ctx, run, lifecycle, pending); err != nil {
				if errors.Is(err, errLifecycleNeedsReconciliation) {
					return nil
				}
				return err
			}
			continue
		}
		candidate, err := lifecycle.Next(ctx, snapshot.Clone())
		if err != nil {
			return err
		}
		if err := candidate.Validate(); err != nil {
			return err
		}
		if candidate.Done || candidate.Action == executionapi.LifecycleStartTurn {
			return nil
		}
		if candidate.Action != executionapi.LifecycleCreateResource && candidate.Action != executionapi.LifecyclePrepareSession {
			return errors.New("lifecycle setup proposed a step outside resource preparation")
		}
		if err := s.executeLifecycleStep(ctx, run, lifecycle, candidate); err != nil {
			if errors.Is(err, errLifecycleNeedsReconciliation) {
				return nil
			}
			return err
		}
	}
	return errors.New("lifecycle setup exceeded the bounded resource preparation steps")
}

func (s *Service) cleanupLifecycleResources(ctx context.Context, run *Run, lifecycle executionapi.LifecyclePort) error {
	for step := 0; step < 8; step++ {
		fixed, err := s.workerFlowContext(run)
		if err != nil {
			return err
		}
		snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
		if err != nil {
			return err
		}
		pending, hasPending, err := unresolvedLifecycleCandidate(snapshot)
		if err != nil {
			return err
		}
		if hasPending {
			if err := s.executeLifecycleStep(ctx, run, lifecycle, pending); err != nil {
				return err
			}
			continue
		}
		candidate, err := lifecycle.Next(ctx, snapshot.Clone())
		if err != nil {
			return err
		}
		if err := candidate.Validate(); err != nil {
			return err
		}
		if candidate.Done {
			return nil
		}
		if candidate.Action != executionapi.LifecycleCleanup {
			return errors.New("completed lifecycle plan proposed a non-cleanup operation")
		}
		if err := s.executeLifecycleStep(ctx, run, lifecycle, candidate); err != nil {
			return err
		}
	}
	return errors.New("lifecycle cleanup exceeded its bounded resource list")
}

func (s *Service) acknowledgeLifecycleDelivery(run *Run, deliveryID string) error {
	if run == nil || deliveryID == "" {
		return errors.New("lifecycle Delivery acknowledgement lacks its original identity")
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	key := "worker-delivery-ack:" + fixed.execution.ExecutionID + ":" + deliveryID
	prior := s.operationByKey(key)
	if fixed.execution.PendingDeliveryID != "" && fixed.execution.PendingDeliveryID != deliveryID || fixed.execution.PendingDeliveryID == "" && prior == nil {
		return ErrWorkerTurnStale
	}
	input := executionapi.LifecycleInput{Ack: &executionapi.AcknowledgeInput{DeliveryID: deliveryID}}
	var opID string
	if prior != nil && prior.Status != "pending" {
		opID = prior.OperationID
	} else {
		opID, err = s.startWorkerOperation(run, string(executionapi.LifecycleAcknowledge), key, input.Ack)
		if err != nil {
			return err
		}
	}
	operation := s.operationByKey(key)
	if operation == nil || operation.OperationID != opID {
		return errors.New("lifecycle Delivery acknowledgement lost its persisted Operation")
	}
	if err := validateSavedLifecycleOperation(operation, executionapi.LifecycleAcknowledge, input); err != nil {
		return err
	}
	if operation.Status == "confirmed" {
		fixed.execution.PendingDeliveryID = ""
		s.refreshRun(run)
		return s.commitWorker(run, opID)
	}
	if prior != nil && operation.Status == "rejected" {
		return errors.New("original lifecycle Delivery acknowledgement was rejected")
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	request := lifecycleRequest(snapshot, *operation, executionapi.LifecycleAcknowledge, input)
	if err := request.Validate(); err != nil {
		return err
	}
	var receipt executionapi.LifecycleReceipt
	if prior != nil && (operation.Status == "pending" || operation.Status == "in_flight" || operation.Status == "accepted" || operation.Status == "outcome_unknown") {
		receipt, _, err = lifecycle.Reconcile(context.Background(), request.Clone())
	} else {
		receipt, _, err = lifecycle.Execute(context.Background(), request.Clone())
	}
	if validationErr := receipt.ValidateFor(request); validationErr != nil {
		return validationErr
	}
	if prior == nil && (err != nil || receipt.Disposition == executionapi.OperationUnknown) {
		// An acknowledgement with a lost receipt is never repeated. Reconcile its
		// original operation immediately so a live observer cannot strand the
		// committed delivery behind its own pending-consumption gate.
		reconciled, _, reconcileErr := lifecycle.Reconcile(context.Background(), request.Clone())
		if validationErr := reconciled.ValidateFor(request); validationErr != nil {
			return validationErr
		}
		if reconcileErr == nil && reconciled.Disposition == executionapi.OperationConfirmed {
			receipt, err = reconciled, nil
		} else if reconcileErr != nil {
			err = errors.Join(err, reconcileErr)
		}
	}
	if mergeErr := mergeLifecycleExecutionReceipt(fixed.execution, receipt); mergeErr != nil {
		return mergeErr
	}
	status := string(receipt.Disposition)
	diagnostic := receipt.Diagnostic
	if err != nil {
		status, diagnostic = "outcome_unknown", err.Error()
	}
	if receipt.Disposition != executionapi.OperationConfirmed {
		if status != "outcome_unknown" {
			status = "outcome_unknown"
		}
		if diagnostic == "" {
			diagnostic = "original Delivery acknowledgement remains unconfirmed"
		}
	}
	if receipt.Disposition == executionapi.OperationConfirmed && err == nil {
		fixed.execution.PendingDeliveryID = ""
	}
	if err := s.setOperationState(opID, status, receipt.RequestID, mustJSON(receipt), diagnostic); err != nil {
		return err
	}
	s.refreshRun(run)
	if err := s.commitWorker(run, opID); err != nil {
		return err
	}
	if status != "confirmed" {
		return s.waitWorkerFlow(run, "Original Delivery acknowledgement needs reconciliation: "+diagnostic)
	}
	return nil
}

func sameLifecycleStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Service) launchAppServerLifecycle(ctx context.Context, runID string, lifecycle executionapi.LifecyclePort, request executionapi.LifecycleRequest, reconcile bool) {
	var receipt executionapi.LifecycleReceipt
	var observer executionapi.LifecycleObserver
	var executeErr error
	if reconcile {
		receipt, observer, executeErr = lifecycle.Reconcile(ctx, request.Clone())
	} else {
		receipt, observer, executeErr = lifecycle.Execute(ctx, request.Clone())
	}
	message := &appServerLaunchMessage{phase: "turn_result", request: request.Clone(), receipt: receipt.Clone(), observer: observer, err: executeErr}
	if executionapi.LifecycleAction(request.Operation.Action) == executionapi.LifecyclePrepareSession {
		message.phase = "session_prepared"
		ack := make(chan appServerLaunchAck, 1)
		message.ack = ack
		if !s.sendAppServerLaunch(runID, message, ctx) {
			if observer != nil {
				_ = observer.Close(context.Background())
			}
			return
		}
		var next appServerLaunchAck
		select {
		case next = <-ack:
		case <-ctx.Done():
			return
		}
		if !next.Proceed {
			return
		}
		request = next.Request.Clone()
		receipt, observer, executeErr = lifecycle.Execute(ctx, request)
		message = &appServerLaunchMessage{phase: "turn_result", request: request, receipt: receipt.Clone(), observer: observer, err: executeErr}
	}
	if !s.sendAppServerLaunch(runID, message, ctx) && observer != nil {
		_ = observer.Close(context.Background())
	}
}

func (s *Service) sendAppServerLaunch(runID string, message *appServerLaunchMessage, ctx context.Context) bool {
	select {
	case s.workerMessages <- workerMessage{runID: runID, launch: message}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *Service) applyAppServerSessionReceipt(run *Run, request executionapi.LifecycleRequest, receipt executionapi.LifecycleReceipt, executeErr error) (executionapi.LifecycleRequest, bool, error) {
	if err := receipt.ValidateFor(request); err != nil {
		return executionapi.LifecycleRequest{}, false, err
	}
	if err := s.validateLifecycleResultBinding(run, request); err != nil {
		return executionapi.LifecycleRequest{}, false, err
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return executionapi.LifecycleRequest{}, false, err
	}
	if request.Operation.Action != string(executionapi.LifecyclePrepareSession) || request.Snapshot.Session.ExecutionID != fixed.execution.ExecutionID ||
		request.Snapshot.Session.AttemptID != fixed.execution.CurrentAttemptID || request.Snapshot.Session.Generation != fixed.execution.SessionGeneration ||
		request.Snapshot.ResourceGeneration != fixed.execution.Generation || request.Snapshot.ConfigurationDigest != fixed.execution.ConfigurationDigest ||
		request.Operation.PayloadDigest == "" || fixed.execution.PendingCommandID != request.Input.Turn.CommandID {
		return executionapi.LifecycleRequest{}, false, ErrWorkerTurnStale
	}
	if err := validateSavedLifecycleOperation(snapshotOperation(&s.domain, request.Operation.ID), executionapi.LifecyclePrepareSession, request.Input); err != nil {
		return executionapi.LifecycleRequest{}, false, err
	}
	execution := fixed.execution
	if receipt.Handle != nil && (receipt.Process == nil || receipt.Handle.Identity() != *receipt.Process) {
		return executionapi.LifecycleRequest{}, false, ErrWorkerTurnStale
	}
	if err := mergeLifecycleExecutionReceipt(execution, receipt); err != nil {
		return executionapi.LifecycleRequest{}, false, err
	}
	if owned, ok := receipt.Handle.(executionapi.OwnedProcessHandle); ok && owned.OwnedProcess() {
		if s.ownedProcesses == nil {
			s.ownedProcesses = make(map[string]executionapi.StopHandle)
		}
		s.ownedProcesses[run.RunID] = owned
	}
	if receipt.EventPath != "" {
		fixed.attempt.EventPath = receipt.EventPath
		execution.LogPath = ptr(receipt.EventPath)
	}
	status := string(receipt.Disposition)
	proceed := executeErr == nil && receipt.Disposition == executionapi.OperationConfirmed && receipt.State.SessionID != "" && receipt.Process != nil && receipt.State.SessionGeneration != ""
	if executeErr != nil || !proceed {
		if status != string(executionapi.OperationRejected) {
			status = "outcome_unknown"
		}
		execution.Status = "blocked"
		diagnostic := receipt.Diagnostic
		if executeErr != nil {
			diagnostic = executeErr.Error()
		}
		execution.Error = ptr("Original app-server session preparation is unverified: " + diagnostic)
	}
	if err := s.setOperationState(request.Operation.ID, status, receipt.State.SessionID, mustJSON(receipt), receipt.Diagnostic); err != nil {
		return executionapi.LifecycleRequest{}, false, err
	}
	if s.pendingOperations == nil {
		s.pendingOperations = make(map[string]string)
	}
	s.pendingOperations[request.Operation.ID] = run.RunID
	s.refreshRun(run)
	var attachment workerCommitAttachment
	if execution.Process != nil && receipt.State.SessionGeneration != "" {
		ref := lifecycleExecutionRef(run, execution, receipt.State.SessionGeneration)
		attachment.Binding = &ref
	}
	if err := s.commitWorkerAttached(run, attachment, request.Operation.ID); err != nil {
		return executionapi.LifecycleRequest{}, false, err
	}
	if !proceed {
		return executionapi.LifecycleRequest{}, false, nil
	}
	turn := *request.Input.Turn
	key := "worker-turn:" + execution.ExecutionID + ":" + turn.CommandID
	operationID, err := s.startWorkerOperation(run, "start_worker_turn", key, &turn)
	if err != nil {
		return executionapi.LifecycleRequest{}, false, err
	}
	operation := s.operationByKey(key)
	if operation == nil || operation.OperationID != operationID {
		return executionapi.LifecycleRequest{}, false, errors.New("app-server session receipt did not authorize the original turn operation")
	}
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	snapshot, err := s.lifecycleSnapshot(run, workflow, execution, attempt, &turn)
	if err != nil {
		return executionapi.LifecycleRequest{}, false, err
	}
	return lifecycleRequest(snapshot, *operation, executionapi.LifecycleStartTurn, executionapi.LifecycleInput{Turn: &turn}), true, nil
}

func lifecycleExecutionRef(run *Run, execution *ExecutionRecord, generation string) executionapi.ExecutionRef {
	ref := executionapi.ExecutionRef{
		Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendKind(execution.Backend),
		Protocol: executionapi.ExecutionProtocol(execution.Protocol), Target: execution.Target,
		Agent: executionapi.AgentSpec{Name: execution.Agent}, RunID: run.RunID, AttemptID: run.AttemptID,
		WorkspacePath: execution.Workspace.Worktree, WorkspaceBranch: execution.Workspace.Branch,
		BaseCommit: execution.Workspace.Base, Generation: generation,
	}
	if execution.Process != nil {
		ref.External = *execution.Process
	}
	return ref
}

func (s *Service) applyAppServerTurnReceipt(run *Run, request executionapi.LifecycleRequest, receipt executionapi.LifecycleReceipt, observer executionapi.LifecycleObserver, executeErr error) error {
	if err := receipt.ValidateFor(request); err != nil {
		if observer != nil {
			_ = observer.Close(context.Background())
		}
		return err
	}
	if err := s.validateLifecycleResultBinding(run, request); err != nil {
		if observer != nil {
			_ = observer.Close(context.Background())
		}
		return err
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		if observer != nil {
			_ = observer.Close(context.Background())
		}
		return err
	}
	if request.Operation.Action != string(executionapi.LifecycleStartTurn) || request.Snapshot.Session.ExecutionID != fixed.execution.ExecutionID ||
		request.Snapshot.Session.AttemptID != fixed.execution.CurrentAttemptID || request.Snapshot.ResourceGeneration != fixed.execution.Generation ||
		request.Snapshot.ConfigurationDigest != fixed.execution.ConfigurationDigest || request.Snapshot.Session.Generation != fixed.execution.SessionGeneration ||
		fixed.execution.PendingCommandID != request.Input.Turn.CommandID {
		if observer != nil {
			_ = observer.Close(context.Background())
		}
		return ErrWorkerTurnStale
	}
	if err := validateSavedLifecycleOperation(snapshotOperation(&s.domain, request.Operation.ID), executionapi.LifecycleStartTurn, request.Input); err != nil {
		if observer != nil {
			_ = observer.Close(context.Background())
		}
		return err
	}
	execution, attempt := fixed.execution, fixed.attempt
	previousProcess := execution.Process
	if err := mergeLifecycleExecutionReceipt(execution, receipt); err != nil {
		if observer != nil {
			_ = observer.Close(context.Background())
		}
		return err
	}
	if receipt.Process != nil && previousProcess != nil && !sameExecutionSessionProcess(*previousProcess, *receipt.Process) {
		if observer != nil {
			_ = observer.Close(context.Background())
		}
		return ErrWorkerTurnStale
	}
	if receipt.Process != nil {
		process := *receipt.Process
		execution.Process = &process
	}
	worker := workerFromLifecycle(run.RunID, run.AttemptID, observer)
	accepted := executeErr == nil && receipt.Disposition == executionapi.OperationAccepted && receipt.State.InputAccepted && receipt.State.TurnStarted && receipt.State.TurnID != "" && worker != nil
	if accepted {
		if receipt.Process == nil || worker.proc == nil || worker.proc.Identity() != *receipt.Process {
			_ = worker.detachLocalObservation(context.Background())
			return ErrWorkerTurnStale
		}
		execution.PendingTurnID = receipt.State.TurnID
		execution.Status = "pending"
		attempt.Status, attempt.UpdatedAt = "dispatched", now()
		if receipt.EventPath != "" {
			attempt.EventPath = receipt.EventPath
			execution.LogPath = ptr(receipt.EventPath)
		}
	} else {
		if worker != nil {
			_ = worker.detachLocalObservation(context.Background())
		}
		status := string(receipt.Disposition)
		if executeErr != nil || status == string(executionapi.OperationAccepted) {
			status = "outcome_unknown"
		}
		execution.Status = "blocked"
		diagnostic := receipt.Diagnostic
		if executeErr != nil {
			diagnostic = executeErr.Error()
		}
		execution.Error = ptr("Original app-server turn start is unverified: " + diagnostic)
		if err := s.setOperationState(request.Operation.ID, status, receipt.State.TurnID, mustJSON(receipt), diagnostic); err != nil {
			return err
		}
		s.refreshRun(run)
		return s.commitWorker(run, request.Operation.ID)
	}
	if err := s.setOperationState(request.Operation.ID, "accepted", receipt.State.TurnID, mustJSON(receipt), ""); err != nil {
		_ = worker.detachLocalObservation(context.Background())
		return err
	}
	s.refreshRun(run)
	var attachment workerCommitAttachment
	if previousProcess == nil && receipt.Process != nil && receipt.State.SessionGeneration != "" {
		ref := lifecycleExecutionRef(run, execution, receipt.State.SessionGeneration)
		attachment.Binding = &ref
	}
	if err := s.commitWorkerAttached(run, attachment, request.Operation.ID); err != nil {
		_ = worker.detachLocalObservation(context.Background())
		return fmt.Errorf("worker started but its exact process receipt could not be saved: %w", err)
	}
	s.registerWorker(run, worker)
	return nil
}

func (s *Service) deliverLifecycleEnvironmentFeedback(ctx context.Context, run *Run, answer DecisionAnswer,
	commandID string, request DecisionRequest, record *DecisionRecord) error {
	if run == nil || record == nil || commandID == "" {
		return errors.New("deferred worker feedback has no original command or decision")
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	input := executionapi.LifecycleInput{Feedback: &executionapi.FeedbackInput{DecisionID: record.DecisionID,
		Message: workerFeedbackText(answer, request), Answer: &executionapi.Answer{Answers: answer.Answers}}}
	key := "worker-environment-feedback:" + fixed.execution.ExecutionID + ":" + commandID
	prior := s.operationByKey(key)
	if prior != nil && prior.Status == "confirmed" {
		if record.Status == "reply_accepted" {
			return nil
		}
		record.Status, record.Version, record.UpdatedAt = "reply_accepted", record.Version+1, now()
		s.markWorkerDecisionWrite(record.DecisionID)
		return s.commitWorker(run)
	}
	if prior != nil && prior.Status == "rejected" {
		return s.waitWorkerFlow(run, "The original feedback operation was rejected; no duplicate input was sent")
	}
	var operationID string
	if prior != nil && prior.Status != "pending" {
		operationID = prior.OperationID
	} else {
		operationID, err = s.startWorkerOperation(run, string(executionapi.LifecycleDeliverFeedback), key, input.Feedback)
		if err != nil {
			return err
		}
	}
	operation := s.operationByKey(key)
	if operation == nil || operation.OperationID != operationID {
		return errors.New("deferred feedback lost its original lifecycle operation")
	}
	if err := validateSavedLifecycleOperation(operation, executionapi.LifecycleDeliverFeedback, input); err != nil {
		return err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	// Host feedback is a status message, never an answer to the restored worker question.
	snapshot.QuestionRoute = nil
	lifecycleRequest := lifecycleRequest(snapshot, *operation, executionapi.LifecycleDeliverFeedback, input)
	var receipt executionapi.LifecycleReceipt
	var callErr error
	if prior != nil && (operation.Status == "in_flight" || operation.Status == "accepted" || operation.Status == "outcome_unknown") {
		receipt, _, callErr = lifecycle.Reconcile(ctx, lifecycleRequest.Clone())
	} else {
		receipt, _, callErr = lifecycle.Execute(ctx, lifecycleRequest.Clone())
	}
	if err := receipt.ValidateFor(lifecycleRequest); err != nil {
		return err
	}
	status, diagnostic := string(receipt.Disposition), receipt.Diagnostic
	if callErr != nil {
		status, diagnostic = "outcome_unknown", callErr.Error()
	}
	if receipt.Disposition != executionapi.OperationConfirmed || !receipt.State.InputAccepted {
		if status != "outcome_unknown" {
			status = "outcome_unknown"
		}
		if diagnostic == "" {
			diagnostic = "deferred feedback is not confirmed"
		}
	} else if record.Status != "reply_accepted" {
		record.Status, record.Version, record.UpdatedAt = "reply_accepted", record.Version+1, now()
		s.markWorkerDecisionWrite(record.DecisionID)
	}
	if err := s.setOperationState(operationID, status, receipt.RequestID, mustJSON(receipt), diagnostic); err != nil {
		return err
	}
	if err := s.commitWorker(run, operationID); err != nil {
		return err
	}
	if status != "confirmed" {
		return s.waitWorkerFlow(run, "Deferred feedback needs original-operation reconciliation: "+diagnostic)
	}
	return nil
}

// Deferred host feedback retains the same question/turn and Operation. This
// path never sends input, even when the current business turn has changed.
func (s *Service) reconcileDeferredLifecycleFeedback(ctx context.Context, run *Run, operation *OperationRecord,
	input executionapi.LifecycleInput, route *executionapi.QuestionRoute, turnID string) (bool, error) {
	if err := validateSavedLifecycleOperation(operation, executionapi.LifecycleDeliverFeedback, input); err != nil {
		return false, err
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return false, err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return false, err
	}
	snapshot.QuestionRoute = route
	snapshot.Session.CurrentTurnID, snapshot.Session.PendingTurnID = turnID, ""
	snapshot.Session.CurrentCommandID, snapshot.Session.PendingCommandID = turnID, ""
	request := lifecycleRequest(snapshot, *operation, executionapi.LifecycleDeliverFeedback, input)
	if err := request.Validate(); err != nil {
		return false, err
	}
	receipt, observer, err := lifecycle.Reconcile(ctx, request.Clone())
	if observer != nil {
		_ = observer.Close(context.Background())
	}
	if validationErr := receipt.ValidateFor(request); validationErr != nil {
		return false, validationErr
	}
	if err != nil || receipt.Disposition != executionapi.OperationConfirmed || !receipt.State.InputAccepted {
		return false, err
	}
	if err := s.changeWorkerOperation(run, operation.OperationID, "confirmed", receipt.RequestID, mustJSON(receipt), ""); err != nil {
		return false, err
	}
	return true, nil
}
