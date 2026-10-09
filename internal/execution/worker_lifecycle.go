package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

var errLifecycleNeedsReconciliation = errors.New("lifecycle operation is waiting for read-only reconciliation")

var errLifecycleResourcePending = errors.New("original lifecycle resource is still initializing")

// acceptStartedWorkerTurn commits a generic, already-accepted observer handle.
// It remains the shared persistence boundary used by receipt commit-failure tests.
func (s *Service) acceptStartedWorkerTurn(run *reconstructiontypes.RunView, worker *Worker, events, platformRequestID, operationID, commandID string, local bool) (retErr error) {
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

func lifecycleSetupForRun(run *reconstructiontypes.RunView, fixed workerFlowContext, processHost executionbackend.ManagedProcessHost, snapshot executionbackend.LifecycleSnapshot) executionbackend.LifecycleSetup {
	blocked := append([]string(nil), fixed.config.publicationSettings().AuthenticationRefs...)
	blocked = append(blocked, "BEADS_*", "BD_*", "GH_TOKEN", "GITHUB_TOKEN", "GITEA_TOKEN", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_*", "GIT_CONFIG_VALUE_*")
	return executionbackend.LifecycleSetup{
		Snapshot: snapshot.Clone(), ProfileData: CloneImplementationData(fixed.config.ImplementationData),
		ProfileID: fixed.config.selectedProfileID, AcceptedConfigVersion: fixed.config.acceptedConfigVersion,
		ProcessHost: processHost, RepositoryPath: fixed.config.Repository,
		RunDirectory: fixed.config.RunDir, CommandTimeout: duration(fixed.config.CommandTimeout),
		AttemptNumber: run.DomainAttemptNumber, BlockedEnvironment: blocked,
		GitExecutable: fixed.config.Git.Executable, GitArgs: append([]string(nil), fixed.config.Git.Args...),
		RemoteName: fixed.config.Remote, TargetBranch: fixed.config.Branch,
	}
}

func lifecycleSetupForConfig(c Config, entry ExecutionRegistration, processHost executionbackend.ManagedProcessHost) (executionbackend.LifecycleSetup, error) {
	if c.identity == "" {
		return executionbackend.LifecycleSetup{}, errors.New("app-resolved repository identity is required")
	}
	ref := executionbackend.ExecutionRef{
		Version: executionbackend.ExecutionRecordVersion, Backend: entry.Key.Backend, Protocol: entry.Key.Protocol,
		Target: entry.Target(c), Agent: AgentFor(c), WorkspacePath: c.Repository,
	}
	session := executionbackend.SessionContext{
		ProjectID: c.identity, TaskID: "capability-probe", WorkflowRunID: "00000000000000000000000000000001",
		ExecutionID: "00000000000000000000000000000002", AttemptID: "00000000000000000000000000000003",
		Agent: AgentFor(c), Target: ref.Target,
	}
	snapshot := executionbackend.LifecycleSnapshot{
		Ref: ref, Session: session, ConfigurationDigest: c.fixedProfileDigest,
		ResourceGeneration: "capability-probe",
	}
	blocked := append([]string(nil), c.publicationSettings().AuthenticationRefs...)
	blocked = append(blocked, "BEADS_*", "BD_*", "GH_TOKEN", "GITHUB_TOKEN", "GITEA_TOKEN", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_*", "GIT_CONFIG_VALUE_*")
	return executionbackend.LifecycleSetup{
		Snapshot: snapshot, ProfileData: CloneImplementationData(c.ImplementationData),
		ProfileID: c.selectedProfileID, AcceptedConfigVersion: c.acceptedConfigVersion,
		ProcessHost:    processHost,
		RepositoryPath: c.Repository, RunDirectory: c.RunDir, CommandTimeout: duration(c.CommandTimeout),
		AttemptNumber: 1, BlockedEnvironment: blocked, GitExecutable: c.Git.Executable,
		GitArgs: append([]string(nil), c.Git.Args...), RemoteName: c.Remote, TargetBranch: c.Branch,
	}, nil
}

func (s *Service) lifecycleForRun(run *reconstructiontypes.RunView) (executionbackend.LifecyclePort, workerFlowContext, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return nil, workerFlowContext{}, err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return nil, workerFlowContext{}, err
	}
	setup := lifecycleSetupForRun(run, fixed, s.runtime.ProcessHost, snapshot)
	setup.OwnedProcess = s.ownedProcesses[run.RunID]
	setup.WorkerTreeAbsent = s.resources.WorkerTreeAbsent
	port, err := s.lifecycleResolver.ResolveLifecycle(setup.Clone())
	if err != nil {
		return nil, workerFlowContext{}, err
	}
	if port.PolicyCapabilities().ServiceStopBehavior != fixed.workflow.ExecutionProfile.ServiceStopBehavior {
		return nil, workerFlowContext{}, errors.New("worker service-stop behavior differs from the frozen execution")
	}
	if s.workerServiceOwners == nil {
		s.workerServiceOwners = make(map[string]workerServiceOwner)
	}
	s.workerServiceOwners[run.RunID] = workerServiceOwner{port: port, snapshot: snapshot.Clone()}
	return port, fixed, nil
}

func (s *Service) resumeLifecycleWorkerFlow(ctx context.Context, task *taskmodel.TaskRecord, run *reconstructiontypes.RunView) (bool, error) {
	if s.lifecycleEffectPending(run.RunID) {
		return true, nil
	}
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
			operation.Kind != string(executionbackend.LifecycleAcknowledge) || operation.Status != "in_flight" && operation.Status != "outcome_unknown" && operation.Status != "accepted" {
			continue
		}
		var input executionbackend.AcknowledgeInput
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
		if handled, err := s.reconcileWorkerStops(ctx, run, lifecycle); handled || err != nil {
			return true, err
		}
		handled, reconcileErr := s.reconcilePendingLifecycleSteps(ctx, run, lifecycle)
		if reconcileErr != nil {
			return true, reconcileErr
		}
		if handled || s.workers[run.RunID] != nil {
			return true, nil
		}
	}
	if fixed.workflow.Status == "blocked" || fixed.workflow.Status == "reconciling" {
		if execution.PendingCommandID != "" && s.operationByKey("worker-turn:"+execution.ExecutionID+":"+execution.PendingCommandID) == nil && s.operationByKey("worker-session:"+execution.ExecutionID+":"+execution.PendingCommandID) == nil {
			return true, nil
		}
	}
	if fixed.workflow.Status == "reconciling" && execution.PendingCommandID == "" && execution.CurrentTurnID == execution.LastStoppedTurnID {
		return true, s.reconcileStageFacts(ctx, run)
	}
	if handled, err := s.dispatchQueuedStage(ctx, run); handled || err != nil {
		return true, err
	}
	if !execution.RecoveryRequired && execution.PendingCommandID != "" && (execution.Environment == nil || execution.Environment.Blocker == nil) && s.workers[run.RunID] == nil {
		saved := s.operationByKey("worker-input-value:" + execution.ExecutionID + ":" + execution.PendingCommandID)
		if saved != nil {
			var input executionbackend.TurnInput
			if json.Unmarshal(saved.Request, &input) != nil {
				return true, errors.New("saved input is invalid")
			}
			lifecycle, fixed, err := s.lifecycleForRun(run)
			if err != nil {
				return true, err
			}
			return true, s.startWorkerLifecycle(ctx, run, lifecycle, fixed, input.CommandID, input.Prompt)
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
		if !execution.Environment.Ready || fixed.workflow.StepID == executionmodel.StepImplement && len(fixed.workflow.StageInputs) == 0 && execution.CurrentTurnID != "" && execution.CurrentTurnID == execution.LastStoppedTurnID && execution.PendingCommandID == "" && execution.PendingTurnID == "" &&
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
	if execution.RecoveryRequired {
		lifecycle, fixed, lifecycleErr := s.lifecycleForRun(run)
		if lifecycleErr != nil {
			return true, lifecycleErr
		}
		// A continuing worker can have completed while this service was offline.
		// Read its original result/observation before applying manual-restart gates.
		if lifecycle.PolicyCapabilities().ResumeActiveObservation && (execution.PendingCommandID != "" || execution.CurrentTurnID != "" && execution.CurrentTurnID != execution.LastStoppedTurnID) {
			return true, s.resumeLifecycleObservation(ctx, run, lifecycle)
		}
		snapshot, snapshotErr := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
		if snapshotErr != nil {
			return true, snapshotErr
		}
		evidence, verifyErr := lifecycle.VerifyManualRecovery(ctx, snapshot)
		if verifyErr != nil {
			if ctx.Err() != nil {
				return true, nil
			}
			execution.RecoveryState = "unverified"
			return true, s.waitWorkerFlow(run, "Original worker identity cannot be verified: "+verifyErr.Error())
		}
		beforeRecovery := recordFingerprint(*execution)
		completed := settledOriginalWorkerTurn(execution)
		s.setWorkerRecoveryFacts(run, executionmodel.RecoveryFacts{WorkerLive: evidence.ManagedProcess && !evidence.ProcessAbsent, WorkerAbsent: evidence.ProcessAbsent, WritersAbsent: evidence.WritersAbsent, Completed: completed}, evidence.SessionAvailable)
		if !completed && (!evidence.SessionAvailable || execution.SessionID == "") {
			execution.RecoveryState = "unverified"
			return true, s.waitWorkerFlow(run, "The original session is unavailable. Handle the original execution manually; recovery cannot create a replacement session.")
		}
		s.refreshRun(run)
		if beforeRecovery != recordFingerprint(*execution) {
			if err := s.commitWorker(run); err != nil {
				return true, err
			}
		}
		if !completed && evidence.ManagedProcess && !evidence.ProcessAbsent && lifecycle.PolicyCapabilities().ResumeActiveObservation {
			return true, s.resumeLifecycleObservation(ctx, run, lifecycle)
		}
		if run.Decision != nil && run.Decision.Status == "answer_saved" {
			return true, s.refreshSavedRecoveryDecision(run, false, "服务重开后请确认旧答案；原轮次不会自动继续。")
		}
		if completed && fixed.workflow.PRURL != "" && !s.hasUnknownWorkerOperation(run) {
			return true, s.pollWorkerFlowPR(ctx, run, task, fixed.workflow)
		}
		// An optional feedback decision cannot replace a verified PR's merge gate
		// merely because the completed worker process disappeared during restart.
		if execution.RecoveryState == "completed" && fixed.workflow.Status != "waiting_external" && run.Decision != nil && run.Decision.Status == "pending" {
			if record := s.decisionRecord(run); record != nil && (record.RecoveryVersion != execution.RecoveryVersion || record.RecoveryState != execution.RecoveryState) {
				oldID := record.DecisionID
				s.setDecisionStatus(run, "superseded")
				if err := s.commitWorker(run); err != nil {
					return true, err
				}
				s.setRunDecision(run, nil)
				if err := s.waitWorkerFlow(run, "原 worker 已退出，原轮次已结束。核对当前成果后只可继续原 thread；原会话不可恢复时需人工处理。"); err != nil {
					return true, err
				}
				if next := s.decisionRecord(run); next != nil {
					next.RecoveryOfDecision = oldID
					s.markWorkerDecisionWrite(next.DecisionID)
					return true, s.commitWorker(run)
				}
			}
		}
		return true, nil
	}
	if fixed.workflow.Status != "queued" {
		return true, nil
	}
	return false, s.prepareWorkerFlow(ctx, *task, run)
}

func (s *Service) resumeLifecycleObservation(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort) error {
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
		if ctx.Err() != nil {
			return nil
		}
		return s.waitWorkerFlow(run, "Original worker observation needs reconciliation: "+observeErr.Error())
	}
	if observer != nil {
		if err := mergeLifecycleExecutionReceipt(fixed.execution, receipt); err != nil {
			_ = observer.Close(context.Background())
			return err
		}
		if fixed.execution.RecoveryRequired {
			facts := executionmodel.RecoveryFacts{WorkerLive: receipt.State.WorkerLive, QuestionPending: fixed.execution.DecisionID != "" && run.Decision != nil && run.Decision.Request.QuestionRoute != nil, OutcomeUncertain: s.hasUnknownWorkerOperation(run) || !receipt.State.WorkerLive}
			s.setWorkerRecoveryFacts(run, facts, receipt.State.SessionAvailable)
			if run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute != nil {
				record := s.decisionRecord(run)
				if record != nil && (record.RecoveryVersion != fixed.execution.RecoveryVersion || record.RecoveryState != fixed.execution.RecoveryState) {
					record.RecoveryVersion, record.RecoveryState = fixed.execution.RecoveryVersion, fixed.execution.RecoveryState
					record.Version++
					s.markWorkerDecisionWrite(record.DecisionID)
					s.refreshRun(run)
				}
			}
			if run.Decision != nil && run.Decision.Status == "answer_saved" {
				if err := s.refreshSavedRecoveryDecision(run, run.Decision.Request.QuestionRoute != nil, "服务重开后请确认保留的旧答案，再向原 worker 投递。"); err != nil {
					_ = observer.Close(context.Background())
					return err
				}
			}
		}
		if err := s.commitWorker(run); err != nil {
			_ = observer.Close(context.Background())
			return err
		}
		s.registerWorker(run, workerFromLifecycle(run.RunID, run.AttemptID, observer))
		return nil
	}
	if receipt.TurnEvidence != nil && receipt.TurnEvidence.Completed {
		if err := mergeLifecycleExecutionReceipt(fixed.execution, receipt); err != nil {
			return err
		}
		return s.finishWorkerFlowFor(run, nil)
	}
	reason := receipt.Diagnostic
	if reason == "" {
		reason = "Original worker observation did not confirm an active or completed turn"
	}
	return s.waitWorkerFlow(run, reason)
}

func (s *Service) reconcilePendingLifecycleSteps(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort) (bool, error) {
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
		case executionbackend.LifecyclePrepareWorkspace, executionbackend.LifecycleCreateResource,
			executionbackend.LifecycleStartTurn, executionbackend.LifecycleSubmitInput, executionbackend.LifecycleReportCompletion, executionbackend.LifecycleHostAction:
			if err := s.executeLifecycleStep(ctx, run, lifecycle, candidate); err != nil {
				if errors.Is(err, errLifecycleNeedsReconciliation) || errors.Is(err, errLifecycleEffectPending) {
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

func lifecycleSnapshot(run *reconstructiontypes.RunView, workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord, attempt *executionmodel.StepAttemptRecord, pending *executionbackend.TurnInput) (executionbackend.LifecycleSnapshot, error) {
	if run == nil || workflow == nil || execution == nil || attempt == nil || run.RunID != workflow.WorkflowRunID ||
		run.ExecutionID != execution.ExecutionID || execution.WorkflowRunID != workflow.WorkflowRunID || attempt.ExecutionID != execution.ExecutionID {
		return executionbackend.LifecycleSnapshot{}, errors.New("lifecycle snapshot has no fixed execution records")
	}
	ref := executionbackend.ExecutionRef{
		Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendKind(execution.Backend),
		Protocol: executionbackend.ExecutionProtocol(execution.Protocol), Target: execution.Target,
		Agent: executionbackend.AgentSpec{Name: execution.Agent}, RunID: run.RunID, AttemptID: run.AttemptID,
		WorkspacePath: execution.Workspace.Worktree, WorkspaceBranch: execution.Workspace.Branch,
		BaseCommit: execution.Workspace.Base, Generation: execution.Generation,
	}
	if execution.Process != nil {
		ref.External = *execution.Process
	}
	session := executionbackend.SessionContext{
		ProjectID: execution.ProjectIdentity, TaskID: run.TaskID, WorkflowRunID: run.RunID,
		ExecutionID: execution.ExecutionID, AttemptID: run.AttemptID,
		Agent: executionbackend.AgentSpec{Name: execution.Agent}, Target: execution.Target,
		SessionID: execution.SessionID, Generation: execution.SessionGeneration, CurrentTurnID: execution.CurrentTurnID,
		PendingTurnID: execution.PendingTurnID, LastStoppedTurnID: execution.LastStoppedTurnID,
		CurrentCommandID: execution.CurrentCommandID,
		PendingCommandID: execution.PendingCommandID,
	}
	snapshot := executionbackend.LifecycleSnapshot{
		Ref: ref, Session: session, TaskTitle: run.Title, WorkflowPhase: workflow.Status, TaskDelivered: run.TaskStatus == "closed",
		ConfigurationDigest: execution.ConfigurationDigest,
		ResourceGeneration:  execution.Generation,
		Implementation:      CloneImplementationData(execution.ImplementationData),
		WorkspaceDirty:      execution.Workspace.Dirty,
		Workspace:           executionbackend.WorkspaceIntent{Path: execution.Workspace.Worktree, Branch: execution.Workspace.Branch, BaseCommit: execution.Workspace.Base},
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

func (s *Service) lifecycleSnapshot(run *reconstructiontypes.RunView, workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord, attempt *executionmodel.StepAttemptRecord, pending *executionbackend.TurnInput) (executionbackend.LifecycleSnapshot, error) {
	snapshot, err := lifecycleSnapshot(run, workflow, execution, attempt, pending)
	if err != nil {
		return executionbackend.LifecycleSnapshot{}, err
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
			!strings.HasPrefix(record.IdempotencyKey, "worker-delivery-ack:") &&
			!strings.HasPrefix(record.IdempotencyKey, "worker-stop-observation:") {
			continue
		}
		action := executionbackend.LifecycleAction(record.Kind)
		switch record.Kind {
		case "prepare_worker_session":
			action = executionbackend.LifecyclePrepareSession
		case "start_worker_turn":
			action = executionbackend.LifecycleStartTurn
		case "submit_worker_input":
			action = executionbackend.LifecycleSubmitInput
		case string(executionbackend.LifecycleReportCompletion):
			action = executionbackend.LifecycleReportCompletion
		case "reply_execution_question":
			action = executionbackend.LifecycleDeliverFeedback
		case "stop_worker_observation":
			action = executionbackend.LifecycleStopObservation
		case "worker_delivery_ack":
			action = executionbackend.LifecycleAcknowledge
		}
		if !knownLifecycleAction(action) {
			continue
		}
		if identity := record.LifecycleIdentity; identity == nil || identity.ExecutionID != execution.ExecutionID || identity.Ref.AttemptID != run.AttemptID {
			continue
		}
		attemptID := run.AttemptID
		operation := executionbackend.Operation{
			Version: executionbackend.ExecutionRecordVersion, ID: record.OperationID, RunID: run.RunID, AttemptID: attemptID,
			Backend: executionbackend.BackendKind(execution.Backend), Protocol: executionbackend.ExecutionProtocol(execution.Protocol), Target: execution.Target,
			Action: string(action), IdempotencyKey: record.IdempotencyKey, Request: append([]byte(nil), record.Request...), Receipt: append([]byte(nil), record.Receipt...), PayloadDigest: record.PayloadDigest,
			Status: executionbackend.OperationStatus(record.Status), StartedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
			Ref: &snapshot.Ref,
		}
		bindOriginalLifecycleIdentity(&operation, record)
		snapshot.Operations = append(snapshot.Operations, operation)
	}
	return snapshot.Clone(), nil
}

func knownLifecycleAction(action executionbackend.LifecycleAction) bool {
	switch action {
	case executionbackend.LifecyclePrepareWorkspace, executionbackend.LifecycleCreateResource, executionbackend.LifecyclePrepareSession,
		executionbackend.LifecycleStartTurn, executionbackend.LifecycleSubmitInput, executionbackend.LifecycleReportCompletion, executionbackend.LifecycleDeliverFeedback,
		executionbackend.LifecycleAcknowledge, executionbackend.LifecycleStopObservation, executionbackend.LifecycleStopExecution, executionbackend.LifecycleCleanup, executionbackend.LifecycleHostAction:
		return true
	default:
		return false
	}
}

func unresolvedLifecycleCandidate(snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleCandidate, bool, error) {
	for _, operation := range snapshot.Operations {
		if operation.Status != executionbackend.OperationInFlight && operation.Status != executionbackend.OperationUnknown {
			continue
		}
		candidate, err := executionbackend.CandidateForOperation(operation)
		return candidate, err == nil, err
	}
	return executionbackend.LifecycleCandidate{}, false, nil
}

func lifecycleRequest(snapshot executionbackend.LifecycleSnapshot, operation executionmodel.OperationRecord, action executionbackend.LifecycleAction, input executionbackend.LifecycleInput) executionbackend.LifecycleRequest {
	request := executionbackend.LifecycleRequest{
		Snapshot: snapshot.Clone(),
		Operation: executionbackend.Operation{
			Version: executionbackend.ExecutionRecordVersion, ID: operation.OperationID,
			RunID: snapshot.Session.WorkflowRunID, AttemptID: snapshot.Session.AttemptID,
			Backend: snapshot.Ref.Backend, Protocol: snapshot.Ref.Protocol, Target: snapshot.Ref.Target,
			Action: string(action), PayloadDigest: operation.PayloadDigest,
			IdempotencyKey: operation.IdempotencyKey, Request: append([]byte(nil), operation.Request...), Receipt: append([]byte(nil), operation.Receipt...),
			Status: executionbackend.OperationStatus(operation.Status), StartedAt: operation.CreatedAt,
			UpdatedAt: operation.UpdatedAt, Ref: &snapshot.Ref,
		},
		Input: input.Clone(),
	}
	bindOriginalLifecycleIdentity(&request.Operation, operation)
	return request
}

func bindOriginalLifecycleIdentity(operation *executionbackend.Operation, record executionmodel.OperationRecord) {
	if record.LifecycleIdentity == nil {
		return
	}
	identity := *record.LifecycleIdentity
	operation.LifecycleIdentity = &identity
	ref := identity.Ref
	operation.Ref, operation.RunID, operation.AttemptID = &ref, ref.RunID, ref.AttemptID
	operation.Backend, operation.Protocol, operation.Target = ref.Backend, ref.Protocol, ref.Target
}

func (s *Service) validateLifecycleResultBinding(run *reconstructiontypes.RunView, request executionbackend.LifecycleRequest) error {
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
		executionbackend.LifecycleBindingDigest(current) != executionbackend.LifecycleBindingDigest(request.Snapshot) {
		return executionmodel.ErrTurnStale
	}
	return nil
}

func lifecycleTurnInput(commandID, prompt string, linkedArgs []string) executionbackend.TurnInput {
	return executionbackend.TurnInput{CommandID: commandID, Prompt: prompt, Purpose: "business", LinkedWorktreeArgs: append([]string(nil), linkedArgs...)}
}

func (s *Service) originalLifecycleRequest(run *reconstructiontypes.RunView, fixed workerFlowContext) (executionbackend.LifecycleRequest, error) {
	commandID := fixed.execution.CurrentCommandID
	if fixed.execution.PendingCommandID != "" {
		commandID = fixed.execution.PendingCommandID
	}
	if commandID == "" {
		return executionbackend.LifecycleRequest{}, errors.New("fixed execution has no original command ID")
	}
	turnKey := "worker-turn:" + fixed.execution.ExecutionID + ":" + commandID
	sessionKey := "worker-session:" + fixed.execution.ExecutionID + ":" + commandID
	operation := s.operationByKey(turnKey)
	action := executionbackend.LifecycleStartTurn
	if operation == nil {
		operation = s.operationByKey(sessionKey)
		action = executionbackend.LifecyclePrepareSession
	}
	if operation == nil {
		return executionbackend.LifecycleRequest{}, errors.New("fixed worker input has no durable lifecycle operation")
	}
	var input executionbackend.TurnInput
	if err := json.Unmarshal(operation.Request, &input); err != nil {
		return executionbackend.LifecycleRequest{}, err
	}
	if input.CommandID != commandID || input.Prompt == "" {
		return executionbackend.LifecycleRequest{}, errors.New("fixed lifecycle operation has a damaged or changed input")
	}
	if err := validateSavedLifecycleOperation(operation, action, executionbackend.LifecycleInput{Turn: &input}); err != nil {
		return executionbackend.LifecycleRequest{}, err
	}
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	snapshot, err := s.lifecycleSnapshot(run, workflow, execution, attempt, &input)
	if err != nil {
		return executionbackend.LifecycleRequest{}, err
	}
	return lifecycleRequest(snapshot, *operation, action, executionbackend.LifecycleInput{Turn: &input}), nil
}

func mergeLifecycleExecutionReceipt(execution *executionmodel.ExecutionRecord, receipt executionbackend.LifecycleReceipt) error {
	if execution == nil {
		return errors.New("lifecycle receipt has no fixed execution record")
	}
	// A rejected receipt must not leave even a partial private candidate which
	// a later diagnostic commit could accidentally publish.
	var candidate executionmodel.ExecutionRecord
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

func mergeLifecycleExecutionCandidate(execution *executionmodel.ExecutionRecord, receipt executionbackend.LifecycleReceipt) error {
	if receipt.Target != nil {
		current, next := execution.Target, *receipt.Target
		if next.ID != current.ID || next.ConnectionID != current.ConnectionID || next.ExternalRuntime != current.ExternalRuntime {
			return executionmodel.ErrTurnStale
		}
		for _, field := range [][2]string{{current.WorktreeID, next.WorktreeID},
			{current.CoordinatorTerminal, next.CoordinatorTerminal}, {current.OrchestrationRun, next.OrchestrationRun}, {current.ExecutionHost, next.ExecutionHost}} {
			if field[0] != "" && field[0] != field[1] {
				return executionmodel.ErrTurnStale
			}
		}
		if current.WorktreeID != "" && (current.Branch != next.Branch || current.BaseRevision != next.BaseRevision) {
			return executionmodel.ErrTurnStale
		}
		execution.Target = next
	}
	if receipt.Workspace != nil {
		if execution.Workspace.Base != "" && execution.Workspace.Base != receipt.Workspace.Base {
			return executionmodel.ErrTurnStale
		}
		workspace := receipt.Workspace
		if !execution.Workspace.VerifiedAt.IsZero() && execution.Workspace.Worktree != "" && (canonical(execution.Workspace.Worktree) != canonical(workspace.Worktree) || execution.Workspace.Branch != workspace.Branch) {
			return executionmodel.ErrTurnStale
		}
		if workspace.Worktree == "" || workspace.Branch == "" || workspace.Base == "" || workspace.Head == "" {
			return errors.New("lifecycle workspace receipt is incomplete")
		}
		execution.Workspace = executionmodel.WorkspaceEvidence{Repository: workspace.Repository, Worktree: workspace.Worktree, Branch: workspace.Branch,
			Base: workspace.Base, Head: workspace.Head, Dirty: workspace.Dirty, VerifiedAt: workspace.CheckedAt}
	}
	if receipt.State.WorktreeID != "" {
		if execution.Target.WorktreeID != "" && execution.Target.WorktreeID != receipt.State.WorktreeID {
			return executionmodel.ErrTurnStale
		}
		execution.Target.WorktreeID = receipt.State.WorktreeID
	}
	if receipt.OriginalHost != nil && execution.Environment != nil {
		original := *receipt.OriginalHost
		if execution.Environment.OriginalTerminal != nil && !sameExecutionSessionProcess(*execution.Environment.OriginalTerminal, original) {
			return executionmodel.ErrTurnStale
		}
		execution.Environment.OriginalTerminal = &original
		execution.Environment.Runtime = original.RuntimeID
	}
	if receipt.Process != nil {
		if execution.Process != nil && *execution.Process != *receipt.Process && !sameExecutionSessionProcess(*execution.Process, *receipt.Process) {
			return executionmodel.ErrTurnStale
		}
		process := *receipt.Process
		execution.Process = &process
		if execution.Environment != nil {
			if execution.Environment.OriginalTerminal != nil && !sameExecutionSessionProcess(*execution.Environment.OriginalTerminal, process) {
				return executionmodel.ErrTurnStale
			}
			execution.Environment.OriginalTerminal = &process
			execution.Environment.Runtime = process.RuntimeID
		}
	}
	if receipt.State.SessionID != "" {
		if execution.SessionID != "" && execution.SessionID != receipt.State.SessionID {
			return executionmodel.ErrTurnStale
		}
		execution.SessionID = receipt.State.SessionID
	}
	if receipt.State.SessionGeneration != "" {
		if execution.SessionGeneration != "" && execution.SessionGeneration != receipt.State.SessionGeneration {
			return executionmodel.ErrTurnStale
		}
		execution.SessionGeneration = receipt.State.SessionGeneration
	}
	if receipt.State.TurnID != "" {
		if execution.CurrentTurnID == "" && execution.PendingTurnID != "" && execution.PendingTurnID != receipt.State.TurnID {
			return executionmodel.ErrTurnStale
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

func sameExecutionSessionProcess(a, b executionbackend.ProcessIdentity) bool {
	return a.Kind == b.Kind && a.PID == b.PID && a.Created == b.Created && a.Executable == b.Executable &&
		a.Terminal == b.Terminal && a.RuntimeID == b.RuntimeID && a.Incarnation == b.Incarnation &&
		a.CoordinatorRuntimeID == b.CoordinatorRuntimeID && a.ExecutionHost == b.ExecutionHost && a.WorktreeID == b.WorktreeID
}

func (s *Service) startWorkerLifecycle(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort, fixed workerFlowContext, commandID, prompt string) error {
	if fixed.workflow.ExecutionProfile == nil {
		return errors.New("worker lifecycle has no fixed accepted execution profile")
	}
	if fixed.execution.ConfigurationDigest != "" && fixed.execution.ConfigurationDigest != fixed.workflow.ExecutionProfile.Digest {
		return errors.New("worker lifecycle configuration identity differs from the accepted profile")
	}
	input := lifecycleTurnInput(commandID, prompt, nil)
	if binding := stageInput(fixed.workflow, commandID); binding != nil {
		input.StepID, input.InputVersion = binding.StepID, binding.Version
		input.ReportToolPolicy = binding.ReportToolPolicy
	}
	if fixed.execution.Environment != nil && fixed.execution.Environment.Preparation != nil && fixed.execution.Environment.Preparation.TurnID == commandID {
		input.Purpose = "readiness"
	}
	policyCapabilities := lifecycle.PolicyCapabilities()
	if input.StepID == executionmodel.StepReport && policyCapabilities.ReportToolPolicy != "" && input.ReportToolPolicy != policyCapabilities.ReportToolPolicy {
		return errors.New("report native policy changed after its input was frozen")
	}
	if policyCapabilities.LinkedWorkspaceWriteRoots {
		args, err := linkedWorktreeWriteArgs(ctx, fixed.config, s.runtime.Directory, fixed.execution.Workspace.Worktree)
		if err != nil {
			return err
		}
		input.LinkedWorktreeArgs = args
		if err := bindPolicyResources(ctx, fixed.config, s.runtime.Directory, s.runtime.ProcessHost, fixed.execution, fixed.execution.Workspace.Worktree, input.LinkedWorktreeArgs); err != nil {
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

	var candidate executionbackend.LifecycleCandidate
	var snapshot executionbackend.LifecycleSnapshot
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
		resourceStep := candidate.Action == executionbackend.LifecycleCreateResource
		turnStep := policyCapabilities.SeparateTurnStart && (candidate.Action == executionbackend.LifecycleStartTurn || candidate.Action == executionbackend.LifecycleSubmitInput)
		if !resourceStep && !turnStep {
			break
		}
		if candidate.Input.Turn != nil && (candidate.Input.Turn.CommandID != input.CommandID || candidate.Input.Turn.Prompt != input.Prompt || candidate.Input.Turn.Purpose != input.Purpose || candidate.Input.Turn.StepID != input.StepID || candidate.Input.Turn.InputVersion != input.InputVersion || candidate.Input.Turn.ReportToolPolicy != input.ReportToolPolicy || !slices.Equal(candidate.Input.Turn.LinkedWorktreeArgs, input.LinkedWorktreeArgs)) {
			return errors.New("lifecycle planner changed the saved business input or resource grant")
		}
		if err := s.executeLifecycleStep(ctx, run, lifecycle, candidate); err != nil {
			if errors.Is(err, errLifecycleNeedsReconciliation) || errors.Is(err, errLifecycleEffectPending) {
				return nil
			}
			return err
		}
		if s.workers[run.RunID] != nil {
			return nil
		}
	}
	if candidate.Input.Turn == nil || candidate.Input.Turn.CommandID != input.CommandID || candidate.Input.Turn.Prompt != input.Prompt || candidate.Input.Turn.Purpose != input.Purpose || candidate.Input.Turn.StepID != input.StepID || candidate.Input.Turn.InputVersion != input.InputVersion || candidate.Input.Turn.ReportToolPolicy != input.ReportToolPolicy ||
		!slices.Equal(candidate.Input.Turn.LinkedWorktreeArgs, input.LinkedWorktreeArgs) {
		return errors.New("lifecycle planner changed the saved business input or resource grant")
	}
	if candidate.Action != executionbackend.LifecyclePrepareSession && candidate.Action != executionbackend.LifecycleStartTurn {
		return errors.New("lifecycle planner proposed an action outside the authorized worker turn")
	}
	key, kind := "worker-turn:"+fixed.execution.ExecutionID+":"+commandID, "start_worker_turn"
	if candidate.Action == executionbackend.LifecyclePrepareSession {
		key, kind = "worker-session:"+fixed.execution.ExecutionID+":"+commandID, "prepare_worker_session"
	}
	prior := s.operationByKey(key)
	if s.candidateBusinessClosed(run) && !retiredResourceAction(candidate.Action, candidate.Input) && (prior == nil || prior.Status == "pending") {
		return nativeErr("candidate_inactive", "retired candidate cannot authorize another lifecycle action")
	}
	operation, err := s.newLifecycleOperation(run, kind, key, candidate.Input.Turn)
	if err != nil {
		return err
	}
	request, err := checkedLifecycleRequest(snapshot, operation, candidate.Action, candidate.Input)
	if err != nil {
		return err
	}
	err = s.dispatchWorkerTurnEffect(ctx, run, lifecycle, request, prior != nil)
	if errors.Is(err, errLifecycleEffectPending) {
		return nil
	}
	return err
}

func validateSavedLifecycleOperation(operation *executionmodel.OperationRecord, action executionbackend.LifecycleAction, input executionbackend.LifecycleInput) error {
	if operation == nil {
		return errors.New("lifecycle operation is missing")
	}
	value, err := lifecycleInputValue(action, input)
	if err != nil {
		return err
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

func lifecycleInputValue(action executionbackend.LifecycleAction, input executionbackend.LifecycleInput) (any, error) {
	if err := input.Validate(action); err != nil {
		return nil, err
	}
	switch action {
	case executionbackend.LifecyclePrepareWorkspace:
		return input.Workspace, nil
	case executionbackend.LifecycleCreateResource:
		return input.Resource, nil
	case executionbackend.LifecyclePrepareSession, executionbackend.LifecycleStartTurn, executionbackend.LifecycleSubmitInput:
		return input.Turn, nil
	case executionbackend.LifecycleReportCompletion:
		return input.Completion, nil
	case executionbackend.LifecycleDeliverFeedback:
		return input.Feedback, nil
	case executionbackend.LifecycleAcknowledge:
		return input.Ack, nil
	case executionbackend.LifecycleStopObservation, executionbackend.LifecycleStopExecution:
		return input.Stop, nil
	case executionbackend.LifecycleCleanup:
		return input.Cleanup, nil
	case executionbackend.LifecycleHostAction:
		return input.Host, nil
	default:
		return nil, errors.New("unsupported lifecycle operation action")
	}
}

func lifecycleOperationKey(executionID string, action executionbackend.LifecycleAction, input executionbackend.LifecycleInput) (string, any, error) {
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

func (s *Service) executeLifecycleStep(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort, candidate executionbackend.LifecycleCandidate) error {
	if run != nil && s.lifecycleEffectPending(run.RunID) {
		return errLifecycleEffectPending
	}

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
	if candidate.Action == executionbackend.LifecycleStartTurn {
		key, kind, value = "worker-turn:"+fixed.execution.ExecutionID+":"+candidate.Input.Turn.CommandID, "start_worker_turn", candidate.Input.Turn
	} else if candidate.Action == executionbackend.LifecycleSubmitInput {
		key, kind, value = "worker-input:"+fixed.execution.ExecutionID+":"+candidate.Input.Turn.CommandID, "submit_worker_input", candidate.Input.Turn
	}
	prior := s.operationByKey(key)
	if s.candidateBusinessClosed(run) && !retiredResourceAction(candidate.Action, candidate.Input) && (prior == nil || prior.Status == "pending") {
		return nativeErr("candidate_inactive", "retired candidate cannot authorize another lifecycle action")
	}
	operation, reconcile, err := s.originalLifecycleOperation(run, kind, key, value)
	if err != nil {
		return err
	}
	if operation.Status == "confirmed" {
		return nil
	}
	if reconcile && operation.Status == "rejected" {
		return errors.New("original lifecycle operation was rejected; a new user-authorized intent is required")
	}
	input := candidate.Input.Clone()
	if err := validateSavedLifecycleOperation(operation, candidate.Action, input); err != nil {
		return err
	}
	if candidate.Action == executionbackend.LifecycleCreateResource &&
		lifecycle.PolicyCapabilities().LinkedWorkspaceWriteRoots && fixed.execution.PolicyResources == nil {
		var linkedArgs []string
		linkedArgs, err = linkedWorktreeWriteArgs(ctx, fixed.config, s.runtime.Directory, fixed.execution.Workspace.Worktree)
		if err != nil {
			return err
		}
		if err = bindPolicyResources(ctx, fixed.config, s.runtime.Directory, s.runtime.ProcessHost, fixed.execution, fixed.execution.Workspace.Worktree, linkedArgs); err != nil {
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
	var pending *executionbackend.TurnInput
	if candidate.Input.Turn != nil && (candidate.Action == executionbackend.LifecycleStartTurn || candidate.Action == executionbackend.LifecycleSubmitInput) {
		pending = candidate.Input.Clone().Turn
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, pending)
	if err != nil {
		return err
	}
	if input.Host != nil && input.Host.Authorization != nil {
		authorization := input.Host.Authorization
		env := fixed.execution.Environment
		if env == nil || env.Blocker == nil || authorization.ExecutionID != run.ExecutionID || authorization.AttemptID != run.AttemptID || authorization.BlockerID != env.Blocker.ID || authorization.TurnID != env.Blocker.TurnID || authorization.BlockerVersion != env.Blocker.Version {
			return executionmodel.ErrTurnStale
		}
		snapshot.HostAuthorization = authorization
	}
	request := lifecycleRequest(snapshot, *operation, candidate.Action, input)
	if err := request.Validate(); err != nil {
		return err
	}
	return s.dispatchLifecycleEffect(ctx, run, lifecycle, request, reconcile, func(run *reconstructiontypes.RunView, result lifecycleEffectResult) error {
		return s.applyLifecycleStepReceipt(run, request, result)
	})
}

func (s *Service) applyLifecycleStepReceipt(run *reconstructiontypes.RunView, request executionbackend.LifecycleRequest, result lifecycleEffectResult) error {
	ctx := s.runContext
	if ctx == nil {
		ctx = context.Background()
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	lifecycle, _, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	candidate := executionbackend.LifecycleCandidate{Action: executionbackend.LifecycleAction(request.Operation.Action), Input: request.Input.Clone()}
	if err := s.validateLifecycleResultBinding(run, request); err != nil {
		if result.observer != nil {
			_ = result.observer.Close(context.Background())
		}
		return err
	}
	receipt, observer, callErr, hadUnknownMutation := result.receipt, result.observer, result.err, result.uncertain
	opID := request.Operation.ID
	if observer != nil && candidate.Action != executionbackend.LifecycleStartTurn && candidate.Action != executionbackend.LifecycleSubmitInput {
		_ = observer.Close(context.Background())
		return errors.New("resource lifecycle step unexpectedly returned an observer")
	}
	if err := receipt.ValidateFor(request); err != nil {
		return err
	}
	previousProcess := fixed.execution.Process
	executionForReceipt := fixed.execution
	if candidate.Action == executionbackend.LifecycleCleanup && (run.TaskStatus == "closed" || run.TaskStatus == "discarded") {
		var frozen executionmodel.ExecutionRecord
		if err := json.Unmarshal(mustJSON(fixed.execution), &frozen); err != nil {
			return err
		}
		executionForReceipt = &frozen // Validate resource facts, retain delivered evidence.
	}
	if err := mergeLifecycleExecutionReceipt(executionForReceipt, receipt); err != nil {
		return errors.Join(s.waitWorkerFlow(run, "lifecycle receipt changed the fixed resource identity: "+err.Error()), err)
	}
	if receipt.Process != nil && fixed.execution.PolicyResources == nil && lifecycle.PolicyCapabilities().LinkedWorkspaceWriteRoots {
		var linkedArgs []string
		linkedArgs, err = linkedWorktreeWriteArgs(ctx, fixed.config, s.runtime.Directory, fixed.execution.Workspace.Worktree)
		if err != nil {
			return err
		}
		if err = bindPolicyResources(ctx, fixed.config, s.runtime.Directory, s.runtime.ProcessHost, fixed.execution, fixed.execution.Workspace.Worktree, linkedArgs); err != nil {
			return err
		}
	}
	if receipt.Workspace != nil {
		applyWorkflowWorkspaceEvidence(run, fixed.execution, fixed.execution.Workspace)
	}
	status, diagnostic := lifecycleOutcome(receipt, callErr)
	failedStart := candidate.Action == executionbackend.LifecycleStartTurn && status == "rejected" &&
		receipt.State.Stopped && receipt.State.WorkerAbsent && receipt.State.WritersAbsent
	if failedStart {
		// The adapter proved the original start's resources were stopped and
		// released. Retain the intent/effects, but stop retrying its pending input.
		fixed.execution.PendingCommandID, fixed.execution.PendingTurnID = "", ""
		fixed.execution.Status = "blocked"
		fixed.execution.RecoveryRequired, fixed.execution.RecoveryState = true, "unverified"
		fixed.execution.RecoveryVersion++
		fixed.execution.RecoveryMarkedAt = ptr(now())
		fixed.attempt.Status, fixed.attempt.UpdatedAt = "failed", now()
	}
	if callErr == nil && status == "accepted" && receipt.State.ResourceID == "" && receipt.State.WorktreeID == "" && receipt.Process == nil {
		status, diagnostic = "outcome_unknown", "resource lifecycle receipt did not identify its accepted resource"
	}
	if err := s.setOperationState(opID, status, receipt.RequestID, mustJSON(receipt), diagnostic); err != nil {
		return err
	}
	if status == "accepted" && (candidate.Action == executionbackend.LifecycleStartTurn || candidate.Action == executionbackend.LifecycleSubmitInput) {
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
			return executionmodel.ErrTurnStale
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
	if status == "accepted" && receipt.State.ResourcePending && candidate.Action == executionbackend.LifecycleCreateResource {
		return errLifecycleResourcePending
	}
	if hadUnknownMutation && fixed.execution.Environment != nil && !failedStart {
		// The original identity can be reconciled while still preserving the
		// explicit host recheck which an uncertain preparation requires.
		if err := s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "resource_preparation_unverified", Confidence: "unverified", Source: "lifecycle.reconciliation", Detail: "Original operation was reconciled after losing its mutation receipt; confirm the retained environment before more input."}); err != nil {
			return err
		}
		return errLifecycleNeedsReconciliation
	}
	if callErr != nil || status == "outcome_unknown" || status == "rejected" {
		if candidate.Action == executionbackend.LifecycleCleanup {
			return errors.New(diagnostic)
		}
		if err := s.waitWorkerFlow(run, diagnostic); err != nil {
			return err
		}
		return errLifecycleNeedsReconciliation
	}
	return nil
}

func (s *Service) prepareLifecycleResources(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort) error {
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
				if errors.Is(err, errLifecycleNeedsReconciliation) || errors.Is(err, errLifecycleEffectPending) {
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
		if candidate.Done || candidate.Action == executionbackend.LifecycleStartTurn {
			return nil
		}
		if candidate.Action != executionbackend.LifecycleCreateResource && candidate.Action != executionbackend.LifecyclePrepareSession {
			return errors.New("lifecycle setup proposed a step outside resource preparation")
		}
		if err := s.executeLifecycleStep(ctx, run, lifecycle, candidate); err != nil {
			if errors.Is(err, errLifecycleNeedsReconciliation) || errors.Is(err, errLifecycleEffectPending) {
				return nil
			}
			return err
		}
	}
	return errors.New("lifecycle setup exceeded the bounded resource preparation steps")
}

func (s *Service) cleanupLifecycleResources(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort) error {
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
		if candidate.Action != executionbackend.LifecycleCleanup {
			return errors.New("completed lifecycle plan proposed a non-cleanup operation")
		}
		if err := s.executeLifecycleStep(ctx, run, lifecycle, candidate); err != nil {
			return err
		}
	}
	return errors.New("lifecycle cleanup exceeded its bounded resource list")
}

func (s *Service) acknowledgeLifecycleDelivery(run *reconstructiontypes.RunView, deliveryID string) error {
	if run != nil && s.lifecycleEffectPending(run.RunID) {
		return nil
	}

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
		return executionmodel.ErrTurnStale
	}
	input := executionbackend.LifecycleInput{Ack: &executionbackend.AcknowledgeInput{DeliveryID: deliveryID}}
	operation, reconcile, err := s.originalLifecycleOperation(run, string(executionbackend.LifecycleAcknowledge), key, input.Ack)
	if err != nil {
		return err
	}
	opID := operation.OperationID
	if err := validateSavedLifecycleOperation(operation, executionbackend.LifecycleAcknowledge, input); err != nil {
		return err
	}
	if operation.Status == "confirmed" {
		fixed.execution.PendingDeliveryID = ""
		s.refreshRun(run)
		return s.commitWorker(run, opID)
	}
	if reconcile && operation.Status == "rejected" {
		return errors.New("original lifecycle Delivery acknowledgement was rejected")
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	request, err := checkedLifecycleRequest(snapshot, operation, executionbackend.LifecycleAcknowledge, input)
	if err != nil {
		return err
	}
	err = s.dispatchLifecycleEffect(context.Background(), run, lifecycle, request, reconcile, func(run *reconstructiontypes.RunView, result lifecycleEffectResult) error {
		if result.observer != nil {
			_ = result.observer.Close(context.Background())
		}
		if validationErr := result.receipt.ValidateFor(request); validationErr != nil {
			return validationErr
		}
		fixed, currentErr := s.workerFlowContext(run)
		if currentErr != nil {
			return currentErr
		}
		if fixed.execution.PendingDeliveryID != "" && fixed.execution.PendingDeliveryID != deliveryID {
			return executionmodel.ErrTurnStale
		}
		receipt, callErr := result.receipt, result.err
		if mergeErr := mergeLifecycleExecutionReceipt(fixed.execution, receipt); mergeErr != nil {
			return mergeErr
		}
		status, diagnostic := lifecycleOutcome(receipt, callErr)
		if callErr != nil {
			status = "outcome_unknown"
		}
		if receipt.Disposition != executionbackend.OperationConfirmed {
			if status != "outcome_unknown" {
				status = "outcome_unknown"
			}
			if diagnostic == "" {
				diagnostic = "original Delivery acknowledgement remains unconfirmed"
			}
		}
		if receipt.Disposition == executionbackend.OperationConfirmed && callErr == nil {
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
	})
	if errors.Is(err, errLifecycleEffectPending) {
		return nil
	}
	return err
}

func (s *Service) applyAppServerSessionReceipt(run *reconstructiontypes.RunView, request executionbackend.LifecycleRequest, receipt executionbackend.LifecycleReceipt, executeErr error) (executionbackend.LifecycleRequest, bool, error) {
	fixed, err := s.checkedWorkerTurnReceipt(run, request, receipt, executionbackend.LifecyclePrepareSession)
	if err != nil {
		return executionbackend.LifecycleRequest{}, false, err
	}
	execution := fixed.execution
	if receipt.Handle != nil && (receipt.Process == nil || receipt.Handle.Identity() != *receipt.Process) {
		return executionbackend.LifecycleRequest{}, false, executionmodel.ErrTurnStale
	}
	if err := mergeLifecycleExecutionReceipt(execution, receipt); err != nil {
		return executionbackend.LifecycleRequest{}, false, err
	}
	if owned, ok := receipt.Handle.(executionbackend.OwnedProcessHandle); ok && owned.OwnedProcess() {
		if s.ownedProcesses == nil {
			s.ownedProcesses = make(map[string]executionbackend.StopHandle)
		}
		s.ownedProcesses[run.RunID] = owned
	}
	if receipt.EventPath != "" {
		fixed.attempt.EventPath = receipt.EventPath
		execution.LogPath = ptr(receipt.EventPath)
	}
	status, diagnostic := lifecycleOutcome(receipt, executeErr)
	proceed := executeErr == nil && receipt.Disposition == executionbackend.OperationConfirmed && receipt.State.SessionID != "" && receipt.Process != nil && receipt.State.SessionGeneration != ""
	if executeErr != nil || !proceed {
		if status != string(executionbackend.OperationRejected) {
			status = "outcome_unknown"
		}
		execution.Status = "blocked"
		execution.Error = ptr("Original app-server session preparation is unverified: " + diagnostic)
	}
	if err := s.setOperationState(request.Operation.ID, status, receipt.State.SessionID, mustJSON(receipt), receipt.Diagnostic); err != nil {
		return executionbackend.LifecycleRequest{}, false, err
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
		return executionbackend.LifecycleRequest{}, false, err
	}
	if !proceed || s.State.Service.Status == "stopping" {
		return executionbackend.LifecycleRequest{}, false, nil
	}
	turn := *request.Input.Turn
	key := "worker-turn:" + execution.ExecutionID + ":" + turn.CommandID
	operation, err := s.newLifecycleOperation(run, "start_worker_turn", key, &turn)
	if err != nil {
		return executionbackend.LifecycleRequest{}, false, err
	}
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	snapshot, err := s.lifecycleSnapshot(run, workflow, execution, attempt, &turn)
	if err != nil {
		return executionbackend.LifecycleRequest{}, false, err
	}
	request, err = checkedLifecycleRequest(snapshot, operation, executionbackend.LifecycleStartTurn, executionbackend.LifecycleInput{Turn: &turn})
	return request, err == nil, err
}

func lifecycleExecutionRef(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord, generation string) executionbackend.ExecutionRef {
	ref := executionbackend.ExecutionRef{
		Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendKind(execution.Backend),
		Protocol: executionbackend.ExecutionProtocol(execution.Protocol), Target: execution.Target,
		Agent: executionbackend.AgentSpec{Name: execution.Agent}, RunID: run.RunID, AttemptID: run.AttemptID,
		WorkspacePath: execution.Workspace.Worktree, WorkspaceBranch: execution.Workspace.Branch,
		BaseCommit: execution.Workspace.Base, Generation: generation,
	}
	if execution.Process != nil {
		ref.External = *execution.Process
	}
	return ref
}

func (s *Service) applyAppServerTurnReceipt(run *reconstructiontypes.RunView, request executionbackend.LifecycleRequest, receipt executionbackend.LifecycleReceipt, observer executionbackend.LifecycleObserver, executeErr error) error {
	fixed, err := s.checkedWorkerTurnReceipt(run, request, receipt, executionbackend.LifecycleStartTurn)
	if err != nil {
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
		return executionmodel.ErrTurnStale
	}
	if receipt.Process != nil {
		process := *receipt.Process
		execution.Process = &process
	}
	worker := workerFromLifecycle(run.RunID, run.AttemptID, observer)
	accepted := executeErr == nil && receipt.Disposition == executionbackend.OperationAccepted && receipt.State.InputAccepted && receipt.State.TurnStarted && receipt.State.TurnID != "" && worker != nil
	if accepted {
		if receipt.Process == nil || worker.proc == nil || worker.proc.Identity() != *receipt.Process {
			_ = worker.detachLocalObservation(context.Background())
			return executionmodel.ErrTurnStale
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
		status, diagnostic := lifecycleOutcome(receipt, executeErr)
		if executeErr != nil || status == string(executionbackend.OperationAccepted) {
			status = "outcome_unknown"
		}
		execution.Status = "blocked"
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

// Active feedback keeps its original command, separate from native question replies.
func (s *Service) deliverLifecycleActiveFeedback(ctx context.Context, run *reconstructiontypes.RunView, answer executionmodel.DecisionAnswer,
	commandID string, request executionmodel.DecisionRequest, record *executionmodel.DecisionRecord) error {
	if run != nil && s.lifecycleEffectPending(run.RunID) {
		return nil
	}

	if run == nil || record == nil || commandID == "" {
		return errors.New("active worker feedback has no original command or decision")
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	input := executionbackend.LifecycleInput{Feedback: &executionbackend.FeedbackInput{DecisionID: record.DecisionID,
		Message: workerFeedbackText(answer, request), Answer: &executionbackend.Answer{Answers: answer.Answers}}}
	key := "worker-live-feedback:" + fixed.execution.ExecutionID + ":" + commandID
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
	operation, reconcile, err := s.originalLifecycleOperation(run, string(executionbackend.LifecycleDeliverFeedback), key, input.Feedback)
	if err != nil {
		return err
	}
	operationID := operation.OperationID
	if err := validateSavedLifecycleOperation(operation, executionbackend.LifecycleDeliverFeedback, input); err != nil {
		return err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	// Host feedback is a status message, never an answer to the restored worker question.
	snapshot.QuestionRoute = nil
	lifecycleRequest := lifecycleRequest(snapshot, *operation, executionbackend.LifecycleDeliverFeedback, input)
	requestDecisionID := record.DecisionID
	err = s.dispatchLifecycleEffect(ctx, run, lifecycle, lifecycleRequest, reconcile, func(run *reconstructiontypes.RunView, result lifecycleEffectResult) error {
		if result.observer != nil {
			_ = result.observer.Close(context.Background())
		}
		receipt, callErr := result.receipt, result.err
		record := s.decisionRecord(run)
		if record == nil || record.DecisionID != requestDecisionID || record.Status != "answer_saved" {
			return executionmodel.ErrTurnStale
		}
		if err := receipt.ValidateFor(lifecycleRequest); err != nil {
			return err
		}
		status, diagnostic := lifecycleOutcome(receipt, callErr)
		if callErr != nil {
			status = "outcome_unknown"
		}
		if receipt.Disposition != executionbackend.OperationConfirmed || !receipt.State.InputAccepted {
			if status != "outcome_unknown" {
				status = "outcome_unknown"
			}
			if diagnostic == "" {
				diagnostic = "active feedback is not confirmed"
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
			return s.waitWorkerFlow(run, "Active feedback needs original-operation reconciliation: "+diagnostic)
		}
		return nil
	})
	if errors.Is(err, errLifecycleEffectPending) {
		return nil
	}
	return err
}

func (s *Service) dispatchWorkerTurnEffect(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort, request executionbackend.LifecycleRequest, reconcile bool) error {
	return s.dispatchLifecycleEffect(ctx, run, lifecycle, request, reconcile, func(run *reconstructiontypes.RunView, result lifecycleEffectResult) error {
		if request.Operation.Action == string(executionbackend.LifecyclePrepareSession) {
			next, proceed, err := s.applyAppServerSessionReceipt(run, request, result.receipt, result.err)
			if result.observer != nil {
				_ = result.observer.Close(context.Background())
			}
			if err != nil || !proceed {
				if owner, ok := lifecycle.(executionbackend.LifecyclePreparationOwner); ok {
					owner.ReleasePreparation()
				}
				return err
			}
			if s.State.Service.Status == "stopping" {
				if owner, ok := lifecycle.(executionbackend.LifecyclePreparationOwner); ok {
					owner.ReleasePreparation()
				}
				return nil
			}
			return s.dispatchWorkerTurnEffect(ctx, run, lifecycle, next, false)
		}
		defer func() {
			if owner, ok := lifecycle.(executionbackend.LifecyclePreparationOwner); ok {
				owner.ReleasePreparation()
			}
		}()
		return s.applyAppServerTurnReceipt(run, request, result.receipt, result.observer, result.err)
	})
}
