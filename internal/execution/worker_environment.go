package execution

import (
	"context"
	"encoding/json"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/execution/reliable"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

func (s *Service) environmentTurn(run *reconstructiontypes.RunView) string {
	execution := s.executionRecord(run)
	if turn, err := s.currentHostTurn(run); err == nil && turn != nil {
		if p := execution.Environment.Preparation; !p.Verified && turn.Settled && turn.TurnID != p.TurnID {
			return p.TurnID
		}
		return turn.TurnID
	}
	return execution.Environment.Preparation.TurnID
}

func (s *Service) blockWorkerEnvironment(run *reconstructiontypes.RunView, fact executionbackend.HostFact) error {
	if s.candidateBusinessClosed(run) {
		return nil
	}
	workflow, execution := s.workflowRecords(run)
	if execution == nil || execution.Environment == nil || workflow.Status == "completed" || workflow.Status == "abandoned" || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nil
	}
	env := execution.Environment
	turnID := s.environmentTurn(run)
	if !reliable.AcceptHostFact(fact, turnID, execution.Process) {
		return nil
	}
	if reliable.InvalidatesPreparation(fact) {
		env.Ready = false
		env.Preparation.Invalidated = true
	}
	var prior *reliable.BlockerRevision
	if env.Blocker != nil {
		prior = &reliable.BlockerRevision{TurnID: env.Blocker.TurnID, Fact: env.Blocker.Fact, Version: env.Blocker.Version}
	}
	change, fresh := reliable.BlockerChange(prior, turnID, fact)
	if fresh {
		env.Blocker = &executionmodel.WorkerHostBlocker{ID: newID(), TurnID: change.TurnID, Fact: change.Fact, Version: change.Version, ObservedAt: now()}
	} else if env.Blocker.Version != change.Version {
		env.Blocker.Fact, env.Blocker.Version, env.Blocker.ObservedAt = change.Fact, change.Version, now()
	}
	if workflow.Status != "waiting_external" {
		workflow.Status, workflow.UpdatedAt = "waiting_input", now()
	}
	execution.Error = ptr("Worker environment blocked: " + fact.Reason + ": " + fact.Detail)
	s.refreshRun(run)
	s.note(run, "steer", "environment", fact.Confidence, "worker environment blocked", fact.Reason, map[string]any{"source": fact.Source, "turn_id": turnID})
	if run.Decision != nil && run.Decision.Status == "answer_saved" {
		return s.rejectUnsentBlockedAnswer(run)
	}
	return s.commitWorker(run)
}

// A saved answer without an input intent is not a delivery queue. Preserve its
// audit record and original question, but require a new answer after recheck.
// Once an input could have been sent, only its original receipt may settle it.
func (s *Service) rejectUnsentBlockedAnswer(run *reconstructiontypes.RunView) error {
	answer, err := ReadDecisionAnswer(run)
	if err != nil {
		return err
	}
	ctx := context.Background()
	command, err := s.commandStore.CommandByIdempotency(ctx, s.Config.identity, DecisionAnswerKey(answer))
	if err != nil {
		return err
	}
	e := s.executionRecord(run)
	for _, key := range []string{"worker-turn:" + e.ExecutionID + ":" + command.CommandID, "worker-live-feedback:" + e.ExecutionID + ":" + command.CommandID, "worker-question-answer:" + e.ExecutionID + ":" + answer.DecisionID} {
		if s.operationByKey(key) != nil {
			return s.commitWorker(run)
		}
	}
	for _, operation := range s.domain.Operations {
		if operation.Kind != string(executionbackend.LifecycleCreateResource) || operation.LifecycleIdentity == nil || operation.LifecycleIdentity.ExecutionID != e.ExecutionID || operation.LifecycleIdentity.Ref.AttemptID != run.AttemptID {
			continue
		}
		var resource executionbackend.ResourceIntent
		if json.Unmarshal(operation.Request, &resource) == nil && resource.TurnID == command.CommandID {
			return s.commitWorker(run)
		}
	}
	if s.lifecycleEffectPending(run.RunID) || s.hasUnknownWorkerOperation(run) {
		return s.commitWorker(run)
	}
	old := *run.Decision
	s.setDecisionStatus(run, "superseded")
	s.setRunDecision(run, nil)
	question, err := WriteDecisionRequest(run, old.Request)
	if err != nil {
		return err
	}
	s.setRunDecision(run, question)
	s.decisionRecord(run).RecoveryOfDecision = old.ID
	if e.PendingCommandID == command.CommandID {
		e.PendingCommandID, e.PendingTurnID = "", ""
	}
	if err := s.commitWorker(run); err != nil {
		return err
	}
	return (s.commandService(s.Config.identity)).SetStatus(ctx, command, "rejected", mustJSON(map[string]string{"code": "environment_blocked", "error": "unsent answer was cancelled by the environment blocker; review and submit a new answer after recheck"}))
}

func (s *Service) observeWorkerEnvironment(ctx context.Context, run *reconstructiontypes.RunView) error {
	wf, execution := s.workflowRecords(run)
	if execution == nil || execution.Environment == nil || ctx.Err() != nil || wf.Status == "completed" || wf.Status == "abandoned" {
		return nil
	}
	config, err := s.executionConfigForRun(run)
	if err != nil {
		return err
	}
	identity := execution.Process
	if identity == nil {
		identity = execution.Environment.OriginalTerminal
	}
	if identity != nil {
		queryCtx, cancel := context.WithTimeout(ctx, positiveDuration(config.CommandTimeout, 10*time.Second))
		host, _, _, _, hostErr := s.hostEnvironmentForRun(run)
		if hostErr != nil {
			cancel()
			return hostErr
		}
		fact, observeErr := host.ObserveHost(queryCtx, *identity)
		cancel()
		if observeErr == nil && fact.Reason != "" {
			return s.blockWorkerEnvironment(run, fact)
		}
		// A disconnected channel cannot erase a blocker or manufacture one.
		if observeErr != nil {
			execution.LastEvent = ptr("宿主观察缺口：" + clip(observeErr.Error(), 500))
		}
	}
	env := execution.Environment
	if env.Blocker != nil && wf.Status != "waiting_external" && wf.Status != "waiting_input" {
		wf.Status = "waiting_input"
		if err := s.commitWorker(run); err != nil {
			return err
		}
	}
	if fact := reliable.EnvironmentDeadline(env.Preparation.Verified, env.Preparation.Deadline, env.Preparation.TurnID, nil, execution.CurrentTurnID, now()); fact != nil {
		return s.blockWorkerEnvironment(run, *fact)
	}
	turn, turnErr := s.currentHostTurn(run)
	if turnErr != nil {
		return turnErr
	}
	if fact := reliable.EnvironmentDeadline(env.Preparation.Verified, env.Preparation.Deadline, env.Preparation.TurnID, turn, execution.CurrentTurnID, now()); fact != nil {
		return s.blockWorkerEnvironment(run, *fact)
	}
	return nil
}

// Preparation owns one persisted budget spanning resource creation and host
// settlement. Each actor pass performs bounded work; restarts reuse its intent.
func (s *Service) advanceWorkerPreparation(ctx context.Context, run *reconstructiontypes.RunView) error {
	if s.State.Service.Status == "stopping" {
		return nil
	}
	if s.candidateBusinessClosed(run) {
		return nil
	}
	if s.lifecycleEffectPending(run.RunID) {
		return nil
	}
	fixed, err := s.workerRecords(run)
	if err != nil {
		return err
	}
	execution, env := fixed.execution, fixed.execution.Environment
	if env == nil {
		return errors.New("managed execution has no persisted preparation intent")
	}
	if err := s.observeWorkerEnvironment(ctx, run); err != nil {
		return err
	}
	progress := reliable.PreparationState(reliable.PreparationControl{Blocked: env.Blocker != nil, RecoveryRequired: execution.RecoveryRequired, Verified: env.Preparation.Verified, Invalidated: env.Preparation.Invalidated})
	if progress == reliable.PreparationWait {
		return nil
	}
	if progress == reliable.PreparationRecover {
		return s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "service_recovery_required", Confidence: "confirmed", Source: "service.recovery", Detail: "原准备状态已保留；重新检查后才允许继续派发。"})
	}
	if progress == reliable.PreparationReady {
		env.Ready = true
		if execution.TurnNumber > 0 && execution.CurrentTurnID != "" && execution.CurrentTurnID == execution.LastStoppedTurnID && execution.PendingCommandID == "" && execution.PendingTurnID == "" && fixed.workflow.Status != "waiting_external" &&
			(run.Decision == nil || run.Decision.Status == "reply_accepted" || run.Decision.Status == "applied") {
			// A turn may complete while a host gate hides its consumed answer.
			// Successful manual recheck restores a fresh choice, never new input.
			return s.waitWorkerFlow(run, "The original turn completed while the environment was being checked.")
		}
		if execution.PendingCommandID != "" || execution.PendingTurnID != "" {
			return s.reconcileEnvironmentBusinessIntent(ctx, run)
		}
		if execution.TurnNumber == 0 && execution.PendingTurnID == "" {
			task := nativeTaskRecord(s.domain.Tasks, run.TaskID)
			if task == nil {
				return errors.New("readiness execution lost its task")
			}
			return s.startInitialWorkerFlowTurn(ctx, *task, run)
		}
		return nil
	}
	remaining := reliable.PreparationWindow(env.Preparation.Deadline, now(), duration(fixed.config.CommandTimeout))
	if remaining <= 0 {
		return s.observeWorkerEnvironment(ctx, run)
	}
	prepareCtx, cancel := context.WithTimeout(ctx, remaining)
	defer cancel()
	if execution.Workspace.Worktree == "" || execution.Process == nil {
		task := nativeTaskRecord(s.domain.Tasks, run.TaskID)
		if task == nil {
			return errors.New("readiness execution lost its fixed task")
		}
		if err = s.prepareWorkerWorkspace(prepareCtx, *task, run); err != nil {
			if errors.Is(err, errLifecycleResourcePending) {
				return nil
			}
			if ctx.Err() != nil {
				return nil
			}
			return s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "resource_preparation_unverified", Confidence: "unverified", Source: "host.resource-operation", Detail: clip(err.Error(), 1000)})
		}
		if execution.Workspace.Worktree == "" || execution.Process == nil || run.Decision != nil {
			return nil
		}
	}
	if err := s.observeWorkerEnvironment(ctx, run); err != nil {
		return err
	}
	if env.Blocker != nil {
		return nil
	}
	p := env.Preparation
	turn, turnErr := s.currentHostTurn(run)
	if turnErr != nil {
		return turnErr
	}
	if turn == nil || !turn.HasDispatch {
		host, _, _, _, hostErr := s.hostEnvironmentForRun(run)
		if hostErr != nil {
			return hostErr
		}
		observation, err := host.InspectAgentInput(prepareCtx, *execution.Process)
		if err != nil {
			return s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "agent_input_unverified", Confidence: "unverified", Source: "host.input-qualification", Detail: clip(err.Error(), 500)})
		}
		if !observation.Ready {
			return s.blockWorkerEnvironment(run, observation.Fact)
		}
	}
	identity := executionbackend.ProbeIdentity{Execution: execution.ExecutionID, Turn: p.TurnID, Nonce: p.Nonce, ProfileDigest: env.ProfileDigest, Runtime: execution.Process.RuntimeID, Terminal: execution.Process.Terminal, Incarnation: execution.Process.Incarnation, Worktree: execution.Workspace.Worktree}
	host, lifecycle, _, _, err := s.hostEnvironmentForRun(run)
	if err != nil {
		return err
	}
	if p.Command == "" {
		plan, planErr := host.PrepareProbe(identity)
		if planErr != nil {
			return s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "probe_preparation_failed", Confidence: "confirmed", Source: "readiness.files", Detail: planErr.Error()})
		}
		p.Probe, p.Command, p.InputDigest, p.ScriptDigest = identity, plan.Command, plan.InputDigest, plan.ScriptDigest
		if err = s.commitWorker(run); err != nil {
			return err
		}
	}
	if p.Probe != identity {
		return s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "identity_mismatch", Confidence: "confirmed", Source: "readiness.probe.identity"})
	}
	if err = s.executeHostAction(prepareCtx, run, lifecycle, executionbackend.HostActionInput{Kind: executionbackend.HostPrepareProbeFiles, Probe: &identity}); err != nil {
		if errors.Is(err, errLifecycleEffectPending) {
			return nil
		}
		if ctx.Err() != nil {
			return nil
		}
		return s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "probe_preparation_failed", Confidence: "confirmed", Source: "readiness.files", Detail: err.Error()})
	}
	plan := executionbackend.ProbePlan{Command: p.Command, InputDigest: p.InputDigest, ScriptDigest: p.ScriptDigest}
	prompt := host.ProbePrompt(plan, p.Nonce, p.ScriptDigest)
	if err = s.startWorkerFlowTurn(prepareCtx, run, prompt, p.TurnID); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		if env.Blocker != nil {
			return nil
		}
		return s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "preparation_dispatch_unverified", Confidence: "unverified", Source: "host.operation-receipt", Detail: clip(err.Error(), 1000)})
	}
	return s.commitWorker(run)
}

func (s *Service) reconcileEnvironmentBusinessIntent(ctx context.Context, run *reconstructiontypes.RunView) error {
	fixed, err := s.workerRecords(run)
	if err != nil {
		return err
	}
	commandID := fixed.execution.PendingCommandID
	if commandID == "" {
		return errors.New("saved business input lost its original Command")
	}
	operation := s.operationByKey("worker-input-value:" + fixed.execution.ExecutionID + ":" + commandID)
	var input executionbackend.TurnInput
	if operation == nil || operation.Kind != "worker_input_value" || (operation.Status != "confirmed" && operation.Status != "in_flight") || operation.LifecycleIdentity == nil || operation.LifecycleIdentity.ExecutionID != run.ExecutionID || operation.LifecycleIdentity.Ref.AttemptID != run.AttemptID || operation.LifecycleIdentity.ConfigurationDigest != fixed.execution.ConfigurationDigest || operation.LifecycleIdentity.ResourceGeneration != fixed.execution.Generation || operation.PayloadDigest != operationDigest(operation.Request) || json.Unmarshal(operation.Request, &input) != nil || input.CommandID != commandID || input.Prompt == "" {
		return errors.New("saved business input is invalid or belongs to another original execution")
	}
	// A value journal alone does not authorize replay of an old saved answer.
	// Resume only an already-journaled external step, through its original intent.
	externalIntent := false
	for _, candidate := range s.domain.Operations {
		if candidate.LifecycleIdentity == nil || candidate.LifecycleIdentity.ExecutionID != run.ExecutionID || candidate.LifecycleIdentity.Ref.AttemptID != run.AttemptID {
			continue
		}
		if candidate.IdempotencyKey == "worker-turn:"+run.ExecutionID+":"+commandID {
			externalIntent = true
			break
		}
		if candidate.Kind == string(executionbackend.LifecycleCreateResource) {
			var resource executionbackend.ResourceIntent
			if json.Unmarshal(candidate.Request, &resource) == nil && resource.TurnID == commandID {
				externalIntent = true
				break
			}
		}
	}
	if !externalIntent {
		return errors.New("original input has no external intent; a saved answer needs fresh confirmation")
	}
	lifecycle, _, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	return s.startWorkerLifecycle(ctx, run, lifecycle, fixed, input.CommandID, input.Prompt)
}

func (s *Service) finishWorkerPreparation(ctx context.Context, run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord, turn *executionbackend.HostTurnFacts, outcome string) error {
	p := execution.Environment.Preparation
	if turn.TurnID != p.TurnID {
		return nil
	}
	if execution.PendingTurnID == turn.TurnID {
		execution.PendingTurnID = ""
	}
	if execution.PendingCommandID == turn.TurnID {
		execution.PendingCommandID = ""
	}
	if outcome != "succeeded" {
		return s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "preparation_failed", Confidence: "confirmed", Source: "host.operation-settlement", Turn: turn.TurnID})
	}
	if p.Receipt == nil {
		host, _, _, _, hostErr := s.hostEnvironmentForRun(run)
		if hostErr != nil {
			return hostErr
		}
		receipt, err := host.ReadProbe(p.Probe, p.InputDigest)
		if err != nil {
			return s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "probe_receipt_invalid", Confidence: "confirmed", Source: "readiness.receipt", Detail: err.Error(), Turn: turn.TurnID})
		}
		p.Receipt = receipt
	}
	if !reliable.ProbeSettled(p.Receipt, p.Evidence, p.Invalidated) {
		return s.commitWorker(run)
	}
	p.Verified = true
	if run.Decision != nil && run.Decision.Request.QuestionRoute != nil && run.Decision.Request.QuestionRoute.TurnID == turn.TurnID {
		if run.Decision.Status == "answer_saved" {
			return s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "answer_delivery_unverified", Confidence: "unverified", Source: "host.question-reply", Turn: turn.TurnID})
		}
		if run.Decision.Status == "pending" {
			s.setDecisionStatus(run, "superseded")
		}
		s.setRunDecision(run, nil)
	}
	if !now().Before(p.Deadline) && !execution.Environment.Ready && execution.Environment.Blocker == nil {
		if err := s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "preparation_unverified", Confidence: "unverified", Source: "preparation.deadline", Turn: turn.TurnID}); err != nil {
			return err
		}
	}
	if err := s.commitWorker(run); err != nil {
		return err
	}
	hostInput := executionbackend.HostActionInput{Kind: executionbackend.HostCleanupProbeFiles, Probe: &p.Probe}
	if err := func() error {
		_, lifecycle, _, _, hostErr := s.hostEnvironmentForRun(run)
		if hostErr != nil {
			return hostErr
		}
		return s.executeHostAction(ctx, run, lifecycle, hostInput)
	}(); err != nil {
		if errors.Is(err, errLifecycleEffectPending) {
			return nil
		}
		execution.LastEvent = ptr("准备证据已保存；文件清理待核实：" + err.Error())
	}
	if _, lifecycle, _, _, hostErr := s.hostEnvironmentForRun(run); hostErr == nil {
		cleanup := executionbackend.HostActionInput{Kind: executionbackend.HostRemoveFeedback, Worktree: execution.Workspace.Worktree, WorkflowRun: run.RunID, TurnID: turn.TurnID}
		_ = s.executeHostAction(ctx, run, lifecycle, cleanup)
	}
	if execution.Environment.Blocker != nil {
		return nil
	}
	return s.advanceWorkerPreparation(ctx, run)
}

func (s *Service) applyEnvironmentAction(ctx context.Context, run *reconstructiontypes.RunView, action string, input productapi.HostActionTarget, commandID string) error {
	fixed, err := s.workerRecords(run)
	if err != nil {
		return err
	}
	execution, env := fixed.execution, fixed.execution.Environment
	target := input
	if env == nil || env.Blocker == nil || target.ExecutionID != execution.ExecutionID || target.RunID != run.RunID || target.AttemptID != run.AttemptID || target.TurnID != env.Blocker.TurnID || target.BlockerID != env.Blocker.ID || target.Version != env.Blocker.Version {
		return nativeErr("stale_version", "environment action target is no longer current")
	}
	if action == "open_terminal" {
		identity := execution.Process
		if identity == nil {
			identity = env.OriginalTerminal
		}
		if identity == nil {
			return errors.New("original terminal is not bound")
		}
		host, lifecycle, _, _, err := s.hostEnvironmentForRun(run)
		if err != nil {
			return err
		}
		fact, err := host.ObserveHost(ctx, *identity)
		if err != nil {
			return err
		}
		if fact.Reason == "identity_mismatch" || fact.Reason == "terminal_exited" {
			return errors.New("original terminal cannot be located")
		}
		original := *identity
		authorization := &executionbackend.HostActionAuthorization{ExecutionID: target.ExecutionID, AttemptID: run.AttemptID, TurnID: target.TurnID,
			CommandID: commandID, BlockerID: target.BlockerID, BlockerVersion: target.Version}
		if err = s.executeHostAction(ctx, run, lifecycle, executionbackend.HostActionInput{Kind: executionbackend.HostOpenOriginal, Process: &original, Authorization: authorization}); err != nil {
			return err
		}
		return nil
	}
	blocker := env.Blocker
	key := "environment-recheck:" + execution.ExecutionID + ":" + blocker.ID
	op := s.operationByKey(key)
	if op == nil {
		id, err := s.startWorkerOperation(run, "environment_recheck", key, target)
		if err != nil {
			return err
		}
		blocker.RecheckOperationID = id
		op = s.operationByKey(key)
	}
	if op.Status == "confirmed" {
		return nil
	}
	if execution.Process != nil {
		host, _, _, _, hostErr := s.hostEnvironmentForRun(run)
		if hostErr != nil {
			return hostErr
		}
		fact, err := host.ObserveHost(ctx, *execution.Process)
		if err != nil || fact.Reason != "" {
			return nativeErr("environment_unverified", "original environment is not ready; no input was sent")
		}
		if err := s.verifyHostExecutionEvidence(ctx, run, execution); err != nil {
			return nativeErr("environment_unverified", "original environment is not ready; no input was sent")
		}
	}
	turn, turnErr := s.currentHostTurn(run)
	if turnErr != nil {
		return nativeErr("environment_unverified", "original environment is not ready; no input was sent")
	}
	if turn != nil && !turn.HasDispatch && turn.TaskID != "" && execution.Process != nil {
		lifecycle, _, err := s.lifecycleForRun(run)
		if err != nil {
			return nativeErr("environment_unverified", "original environment is not ready; no input was sent")
		}
		if _, err := s.reconcilePendingLifecycleSteps(ctx, run, lifecycle); err != nil {
			return nativeErr("environment_unverified", "original environment is not ready; no input was sent")
		}
		turn, _ = s.currentHostTurn(run)
	}
	if turn != nil && turn.Purpose == "readiness" && !env.Preparation.Invalidated {
		// An explicit recheck may reread a settled probe's historical tool records
		// when earlier bounded/unsupported pages provided no tool proof.
		if turn.Settled && !env.Preparation.Evidence.Verified && env.Preparation.Evidence.CallID == "" {
			if err := s.resetHostTurnOutput(run, turn.TurnID); err != nil {
				return err
			}
		}
		if lifecycle, _, err := s.lifecycleForRun(run); err != nil {
			return err
		} else if err := s.resumeLifecycleObservation(ctx, run, lifecycle); err != nil {
			return err
		}
		turn, _ = s.currentHostTurn(run)
		if !env.Preparation.Verified && !now().Before(env.Preparation.Deadline) {
			return nativeErr("environment_unverified", "original environment is not ready; no input was sent")
		}
	}
	if blocker.Fact.Reason == "business_start_unverified" && turn != nil && execution.CurrentTurnID != turn.TurnID {
		if lifecycle, _, err := s.lifecycleForRun(run); err != nil {
			return err
		} else if err := s.resumeLifecycleObservation(ctx, run, lifecycle); err != nil {
			return err
		}
		turn, _ = s.currentHostTurn(run)
		if turn == nil || execution.CurrentTurnID != turn.TurnID && !turn.Settled {
			return nativeErr("environment_unverified", "original environment is not ready; no input was sent")
		}
	}
	// Polling can install a newer blocker; an old action cannot clear it.
	if env.Blocker == nil || env.Blocker.ID != blocker.ID || env.Blocker.Version != target.Version || s.environmentTurn(run) != target.TurnID {
		return s.changeWorkerOperation(run, op.OperationID, "confirmed", commandID, mustJSON(map[string]string{"result": "stale_target", "blocker_id": blocker.ID}), "")
	}
	// Reconcile original resource/Dispatch operations through their exact saved
	// keys. No retry creates another terminal or Task when a result is unknown.
	if env.Preparation.Invalidated {
		if turn == nil || !turn.Settled {
			return nativeErr("environment_unverified", "original environment is not ready; no input was sent")
		}
		host, _, _, _, hostErr := s.hostEnvironmentForRun(run)
		if hostErr != nil {
			return hostErr
		}
		budget := host.PreparationBudget()
		if budget <= 0 {
			return errors.New("host preparation budget must be positive")
		}
		env.History = append(env.History, *env.Preparation)
		started := now()
		env.Preparation = &executionmodel.WorkerPreparation{TurnID: newID(), Nonce: newID(), StartedAt: started, Deadline: started.Add(budget)}
	}
	env.Blocker = nil
	execution.RecoveryRequired = false
	if fixed.workflow.Status == "waiting_input" && execution.DecisionID == "" {
		fixed.workflow.Status = "queued"
		if execution.CurrentTurnID != "" && execution.CurrentTurnID != execution.LastStoppedTurnID {
			fixed.workflow.Status = "running"
		}
	}
	if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", commandID, mustJSON(map[string]string{"blocker_id": blocker.ID}), ""); err != nil {
		return err
	}
	if env.Preparation.Verified && !env.Preparation.Invalidated {
		return s.advanceWorkerPreparation(ctx, run)
	}
	if turn != nil && turn.HasDispatch && !turn.Settled {
		if lifecycle, _, err := s.lifecycleForRun(run); err != nil {
			return err
		} else {
			return s.resumeLifecycleObservation(ctx, run, lifecycle)
		}
	}
	return s.advanceWorkerPreparation(ctx, run)
}
