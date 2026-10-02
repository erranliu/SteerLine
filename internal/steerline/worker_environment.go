package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (s *Service) environmentTurn(run *Run) string {
	execution := s.executionRecord(run)
	if turn, err := s.currentHostTurn(run); err == nil && turn != nil {
		if p := execution.Environment.Preparation; !p.Verified && turn.Settled && turn.TurnID != p.TurnID {
			return p.TurnID
		}
		return turn.TurnID
	}
	return execution.Environment.Preparation.TurnID
}

func (s *Service) blockWorkerEnvironment(run *Run, fact contracts.HostFact) error {
	wf, execution := s.workflowRecords(run)
	if execution == nil || execution.Environment == nil || wf.Phase == "completed" || wf.Phase == "abandoned" || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nil
	}
	env := execution.Environment
	turnID := s.environmentTurn(run)
	if fact.Turn != "" && fact.Turn != turnID {
		return nil
	}
	if fact.Runtime != "" && execution.Process != nil && (fact.Runtime != execution.Process.RuntimeID || fact.Terminal != execution.Process.Terminal || fact.Incarnation != execution.Process.Incarnation) && fact.Reason != "identity_mismatch" {
		return nil
	}
	existing := env.Blocker
	sameBlocker := existing != nil && existing.TurnID == turnID && existing.Fact.Reason == fact.Reason && existing.Fact.Source == fact.Source
	if sameBlocker && (wf.Phase == "waiting_pr_merge" || run.Decision != nil && run.Decision.Request.HostTarget != nil && run.Decision.Request.HostTarget.BlockerID == existing.ID) {
		return nil
	}
	if fact.Reason == "tool_initialization_failed" || fact.Reason == "preparation_failed" {
		env.Ready = false
		env.Preparation.Invalidated = true
	}
	if env.Blocker != nil && run.Decision != nil && run.Decision.Request.HostTarget != nil && run.Decision.Status == "pending" {
		s.setDecisionStatus(run, "superseded")
	}
	env.Blocker = &WorkerHostBlocker{ID: newID(), Fact: fact, TurnID: turnID, ObservedAt: now()}
	if sameBlocker {
		env.Blocker.ID = existing.ID
		env.Blocker.ObservedAt = existing.ObservedAt
		env.Blocker.RecheckOperationID = existing.RecheckOperationID
	}
	// PR observation and settled delivery outrank late terminal diagnostics.
	if wf.Phase == "waiting_pr_merge" && (run.Decision == nil || run.Decision.Status != "answer_saved") {
		return s.commitWorker(run)
	}
	if run.Decision != nil && run.Decision.Request.HostTarget == nil && (run.Decision.Status == "pending" || run.Decision.Status == "answer_saved") {
		if env.DeferredDecisionID == "" {
			env.DeferredDecisionID = run.Decision.ID
		}
	}
	options := []executionapi.DecisionOption{{ID: "recheck_environment", Label: "重新检查环境", Receiver: "center", Action: "recheck_environment"}}
	if execution.Process != nil || env.OriginalTerminal != nil {
		options = append([]executionapi.DecisionOption{{ID: "open_terminal", Label: "打开原 terminal", Receiver: "center", Action: "open_terminal"}}, options...)
	}
	request := DecisionRequest{Question: "Worker 环境需要核实", Context: fmt.Sprintf("原因：%s\n可信程度：%s\n来源：%s\n%s\n请在原生界面处理必要授权，然后重新检查。", fact.Reason, fact.Confidence, fact.Source, fact.Detail), Options: options, AllowFreeText: true, HostTarget: &HostActionTarget{ExecutionID: execution.ExecutionID, TurnID: turnID, BlockerID: env.Blocker.ID}}
	decision, err := WriteDecisionRequest(run, request)
	if err != nil {
		return err
	}
	s.setRunDecision(run, decision)
	env.Blocker.DecisionID = decision.ID
	wf.Phase, wf.UpdatedAt = "waiting_decision", now()
	execution.Error = ptr(request.Context)
	s.refreshRun(run)
	s.note(run, "steer", "environment", fact.Confidence, "worker environment blocked", fact.Reason, map[string]any{"source": fact.Source, "turn_id": turnID})
	return s.commitWorker(run)
}

func (s *Service) observeWorkerEnvironment(ctx context.Context, run *Run) error {
	wf, execution := s.workflowRecords(run)
	if execution == nil || execution.Environment == nil || ctx.Err() != nil || wf.Phase == "completed" || wf.Phase == "abandoned" {
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
	if env.Blocker != nil && wf.Phase != "waiting_pr_merge" && wf.Phase != "waiting_decision" {
		wf.Phase = "waiting_decision"
		if err := s.commitWorker(run); err != nil {
			return err
		}
	}
	if env.Blocker != nil && wf.Phase != "waiting_pr_merge" && (run.Decision == nil || run.Decision.Request.HostTarget == nil) {
		return s.blockWorkerEnvironment(run, env.Blocker.Fact)
	}
	if !env.Preparation.Verified && !now().Before(env.Preparation.Deadline) {
		return s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "preparation_unverified", Confidence: "unverified", Source: "preparation.deadline", Turn: env.Preparation.TurnID})
	}
	if turn, turnErr := s.currentHostTurn(run); turnErr != nil {
		return turnErr
	} else if turn != nil && turn.Purpose == "business" && !turn.Settled && !turn.HasDoneMessage && execution.CurrentTurnID != turn.TurnID && !now().Before(turn.Deadline) {
		return s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "business_start_unverified", Confidence: "unverified", Source: "dispatch.intent.deadline", Turn: turn.TurnID})
	}
	return nil
}

// Preparation owns one persisted budget spanning resource creation and host
// settlement. Each actor pass performs bounded work; restarts reuse its intent.
func (s *Service) advanceWorkerPreparation(ctx context.Context, run *Run) error {
	fixed, err := s.workerFlowContext(run)
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
	if env.Blocker != nil {
		return nil
	}
	if execution.RecoveryRequired {
		return s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "service_recovery_required", Confidence: "confirmed", Source: "service.recovery", Detail: "原准备状态已保留；重新检查后才允许继续派发。"})
	}
	if env.Preparation.Verified && !env.Preparation.Invalidated {
		env.Ready = true
		if execution.TurnNumber > 0 && execution.CurrentTurnID != "" && execution.CurrentTurnID == execution.LastStoppedTurnID && execution.PendingCommandID == "" && execution.PendingTurnID == "" && fixed.workflow.Phase != "waiting_pr_merge" &&
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
	remaining := time.Until(env.Preparation.Deadline)
	if remaining <= 0 {
		return s.observeWorkerEnvironment(ctx, run)
	}
	prepareCtx, cancel := context.WithTimeout(ctx, min(remaining, positiveDuration(fixed.config.CommandTimeout, 10*time.Second)))
	defer cancel()
	if execution.Workspace.Worktree == "" || execution.Process == nil {
		task := nativeTaskRecord(s.domain.Tasks, run.TaskID)
		if task == nil {
			return errors.New("readiness execution lost its fixed task")
		}
		if err = s.prepareWorkerWorkspace(prepareCtx, *task, run); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "resource_preparation_unverified", Confidence: "unverified", Source: "host.resource-operation", Detail: clip(err.Error(), 1000)})
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
			return s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "agent_input_unverified", Confidence: "unverified", Source: "host.input-qualification", Detail: clip(err.Error(), 500)})
		}
		if !observation.Ready {
			return s.blockWorkerEnvironment(run, observation.Fact)
		}
	}
	identity := executionapi.ProbeIdentity{Execution: execution.ExecutionID, Turn: p.TurnID, Nonce: p.Nonce, ProfileDigest: env.ProfileDigest, Runtime: execution.Process.RuntimeID, Terminal: execution.Process.Terminal, Incarnation: execution.Process.Incarnation, Worktree: execution.Workspace.Worktree}
	host, lifecycle, _, _, err := s.hostEnvironmentForRun(run)
	if err != nil {
		return err
	}
	if p.Command == "" {
		plan, planErr := host.PrepareProbe(identity)
		if planErr != nil {
			return s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "probe_preparation_failed", Confidence: "confirmed", Source: "readiness.files", Detail: planErr.Error()})
		}
		p.Probe, p.Command, p.InputDigest, p.ScriptDigest = identity, plan.Command, plan.InputDigest, plan.ScriptDigest
		if err = s.commitWorker(run); err != nil {
			return err
		}
	}
	if p.Probe != identity {
		return s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "identity_mismatch", Confidence: "confirmed", Source: "readiness.probe.identity"})
	}
	if err = s.executeHostAction(prepareCtx, run, lifecycle, executionapi.HostActionInput{Kind: executionapi.HostPrepareProbeFiles, Probe: &identity}); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "probe_preparation_failed", Confidence: "confirmed", Source: "readiness.files", Detail: err.Error()})
	}
	plan := contracts.ProbePlan{Command: p.Command, InputDigest: p.InputDigest, ScriptDigest: p.ScriptDigest}
	prompt := host.ProbePrompt(plan, p.Nonce, p.ScriptDigest)
	if err = s.startWorkerFlowTurn(prepareCtx, run, prompt, p.TurnID); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		if env.Blocker != nil {
			return nil
		}
		return s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "preparation_dispatch_unverified", Confidence: "unverified", Source: "host.operation-receipt", Detail: clip(err.Error(), 1000)})
	}
	return s.commitWorker(run)
}

func (s *Service) reconcileEnvironmentBusinessIntent(ctx context.Context, run *Run) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	commandID := fixed.execution.PendingCommandID
	if commandID == "" {
		return errors.New("saved business input lost its original Command")
	}
	operation := s.operationByKey("worker-input-value:" + fixed.execution.ExecutionID + ":" + commandID)
	var input executionapi.TurnInput
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
		if candidate.Kind == string(executionapi.LifecycleCreateResource) {
			var resource executionapi.ResourceIntent
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

func (s *Service) finishWorkerPreparation(ctx context.Context, run *Run, execution *ExecutionRecord, turn *contracts.HostTurnFacts, outcome string) error {
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
		return s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "preparation_failed", Confidence: "confirmed", Source: "host.operation-settlement", Turn: turn.TurnID})
	}
	if p.Receipt == nil {
		host, _, _, _, hostErr := s.hostEnvironmentForRun(run)
		if hostErr != nil {
			return hostErr
		}
		receipt, err := host.ReadProbe(p.Probe, p.InputDigest)
		if err != nil {
			return s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "probe_receipt_invalid", Confidence: "confirmed", Source: "readiness.receipt", Detail: err.Error(), Turn: turn.TurnID})
		}
		p.Receipt = receipt
	}
	if p.Receipt == nil || !p.Evidence.Verified || p.Invalidated {
		return s.commitWorker(run)
	}
	p.Verified = true
	if run.Decision != nil && run.Decision.Request.QuestionRoute != nil && run.Decision.Request.QuestionRoute.TurnID == turn.TurnID {
		if run.Decision.Status == "answer_saved" {
			return s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "answer_delivery_unverified", Confidence: "unverified", Source: "host.question-reply", Turn: turn.TurnID})
		}
		if run.Decision.Status == "pending" {
			s.setDecisionStatus(run, "superseded")
		}
		s.restoreEnvironmentDecision(run)
	}
	if !now().Before(p.Deadline) && !execution.Environment.Ready && execution.Environment.Blocker == nil {
		if err := s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "preparation_unverified", Confidence: "unverified", Source: "preparation.deadline", Turn: turn.TurnID}); err != nil {
			return err
		}
	}
	if err := s.commitWorker(run); err != nil {
		return err
	}
	hostInput := executionapi.HostActionInput{Kind: executionapi.HostCleanupProbeFiles, Probe: &p.Probe}
	if err := func() error {
		_, lifecycle, _, _, hostErr := s.hostEnvironmentForRun(run)
		if hostErr != nil {
			return hostErr
		}
		return s.executeHostAction(ctx, run, lifecycle, hostInput)
	}(); err != nil {
		execution.LastEvent = ptr("准备证据已保存；文件清理待核实：" + err.Error())
	}
	if _, lifecycle, _, _, hostErr := s.hostEnvironmentForRun(run); hostErr == nil {
		cleanup := executionapi.HostActionInput{Kind: executionapi.HostRemoveFeedback, Worktree: execution.Workspace.Worktree, WorkflowRun: run.RunID, TurnID: turn.TurnID}
		_ = s.executeHostAction(ctx, run, lifecycle, cleanup)
	}
	if execution.Environment.Blocker != nil {
		return nil
	}
	return s.advanceWorkerPreparation(ctx, run)
}

func (s *Service) applyEnvironmentAction(ctx context.Context, run *Run, answer DecisionAnswer, commandID string) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	execution, env := fixed.execution, fixed.execution.Environment
	target := run.Decision.Request.HostTarget
	if env == nil || env.Blocker == nil || target == nil || target.ExecutionID != execution.ExecutionID || target.TurnID != env.Blocker.TurnID || target.BlockerID != env.Blocker.ID || run.Decision.ID != env.Blocker.DecisionID {
		return nativeErr("stale_version", "host action target is no longer current")
	}
	if answer.OptionID != "open_terminal" && answer.OptionID != "recheck_environment" {
		seen := false
		for _, feedback := range env.DeferredFeedback {
			if feedback.DecisionID == answer.DecisionID {
				seen = true
			}
		}
		if !seen {
			env.DeferredFeedback = append(env.DeferredFeedback, EnvironmentFeedback{DecisionID: answer.DecisionID, TurnID: target.TurnID, CommandID: commandID})
		}
		request := run.Decision.Request
		decision, err := WriteDecisionRequest(run, request)
		if err != nil {
			return err
		}
		s.setRunDecision(run, decision)
		env.Blocker.DecisionID = decision.ID
		return s.commitWorker(run)
	} // Saved text stays pending; never inject into a host menu.
	if answer.OptionID == "open_terminal" {
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
		authorization := &executionapi.HostActionAuthorization{ExecutionID: target.ExecutionID, AttemptID: run.AttemptID, TurnID: target.TurnID,
			DecisionID: answer.DecisionID, BlockerID: target.BlockerID, DecisionVersion: run.Decision.Version}
		if err = s.executeHostAction(ctx, run, lifecycle, executionapi.HostActionInput{Kind: executionapi.HostOpenOriginal, Process: &original, Authorization: authorization}); err != nil {
			return err
		}
		return s.renewEnvironmentDecision(run)
	}
	blocker := env.Blocker
	actionDecisionID := run.Decision.ID
	actionDecisionVersion := run.Decision.Version
	key := "environment-recheck:" + execution.ExecutionID + ":" + blocker.ID
	op := s.operationByKey(key)
	if op == nil {
		id, err := s.startWorkerOperation(run, "environment_recheck", key, *target)
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
			return s.renewEnvironmentDecision(run)
		}
		if err := s.verifyHostExecutionEvidence(ctx, run, execution); err != nil {
			return s.renewEnvironmentDecision(run)
		}
	}
	turn, turnErr := s.currentHostTurn(run)
	if turnErr != nil {
		return s.renewEnvironmentDecision(run)
	}
	if turn != nil && !turn.HasDispatch && turn.TaskID != "" && execution.Process != nil {
		lifecycle, _, err := s.lifecycleForRun(run)
		if err != nil {
			return s.renewEnvironmentDecision(run)
		}
		if _, err := s.reconcilePendingLifecycleSteps(ctx, run, lifecycle); err != nil {
			return s.renewEnvironmentDecision(run)
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
			return s.renewEnvironmentDecision(run)
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
			return s.renewEnvironmentDecision(run)
		}
	}
	// Polling can install a newer blocker; an old action cannot clear it.
	if env.Blocker == nil || env.Blocker.ID != blocker.ID || run.Decision == nil || run.Decision.ID != actionDecisionID || run.Decision.Version != actionDecisionVersion || s.environmentTurn(run) != target.TurnID {
		return s.changeWorkerOperation(run, op.OperationID, "confirmed", commandID, mustJSON(map[string]string{"result": "stale_target", "blocker_id": blocker.ID}), "")
	}
	// Reconcile original resource/Dispatch operations through their exact saved
	// keys. No retry creates another terminal or Task when a result is unknown.
	if env.Preparation.Invalidated {
		if turn == nil || !turn.Settled {
			return s.renewEnvironmentDecision(run)
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
		env.Preparation = &WorkerPreparation{TurnID: newID(), Nonce: newID(), StartedAt: started, Deadline: started.Add(budget)}
	}
	if ok, err := s.reconcileEnvironmentQuestion(ctx, run); err != nil {
		return err
	} else if !ok {
		return s.renewEnvironmentDecision(run)
	}
	s.setDecisionStatus(run, "applied")
	env.Blocker = nil
	s.restoreEnvironmentDecision(run)
	execution.RecoveryRequired = false
	if fixed.workflow.Phase == "waiting_decision" && execution.DecisionID == "" {
		fixed.workflow.Phase = "queued"
		if execution.CurrentTurnID != "" && execution.CurrentTurnID != execution.LastStoppedTurnID {
			fixed.workflow.Phase = "executing"
		}
	}
	if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", commandID, mustJSON(map[string]string{"blocker_id": blocker.ID}), ""); err != nil {
		return err
	}
	if err := s.flushEnvironmentFeedback(ctx, run); err != nil {
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

func (s *Service) reconcileEnvironmentQuestion(ctx context.Context, run *Run) (bool, error) {
	e := s.executionRecord(run)
	env := e.Environment
	if env.DeferredDecisionID == "" {
		return true, nil
	}
	var record *DecisionRecord
	for i := range s.domain.Decisions {
		if s.domain.Decisions[i].DecisionID == env.DeferredDecisionID {
			record = &s.domain.Decisions[i]
			break
		}
	}
	if record == nil {
		return false, errors.New("original host-blocked question disappeared")
	}
	var request DecisionRequest
	if json.Unmarshal(record.Request, &request) != nil {
		return false, errors.New("original question request is invalid")
	}
	if request.QuestionRoute == nil {
		return true, nil
	}
	settled := e.LastStoppedTurnID == request.QuestionRoute.TurnID ||
		env.Preparation != nil && env.Preparation.Verified && env.Preparation.TurnID == request.QuestionRoute.TurnID
	if !settled {
		return true, nil
	}
	status := "superseded"
	if record.Status == "answer_saved" {
		var answer DecisionAnswer
		if json.Unmarshal(record.Answer, &answer) != nil {
			return false, errors.New("original saved answer is invalid")
		}
		op := s.operationByKey("worker-question-answer:" + e.ExecutionID + ":" + record.DecisionID)
		if op != nil {
			if op.Status != "confirmed" && op.Status != "accepted" {
				input := executionapi.LifecycleInput{Feedback: &executionapi.FeedbackInput{DecisionID: record.DecisionID, QuestionID: request.QuestionRoute.QuestionID,
					Message: workerFeedbackText(answer, request), Answer: &executionapi.Answer{Answers: answer.Answers}}}
				confirmed, err := s.reconcileDeferredLifecycleFeedback(ctx, run, op, input, request.QuestionRoute, request.QuestionRoute.TurnID)
				if err != nil || !confirmed {
					return false, err
				}
			}
			status = "reply_accepted"
		}
	}
	if record.Status == "pending" || record.Status == "answer_saved" {
		record.Status = status
		record.Version++
		record.UpdatedAt = now()
		s.markWorkerDecisionWrite(record.DecisionID)
	}
	env.DeferredDecisionID = ""
	return true, nil
}

func (s *Service) restoreEnvironmentDecision(run *Run) {
	execution := s.executionRecord(run)
	env := execution.Environment
	turn, turnErr := s.currentHostTurn(run)
	if turnErr != nil || turn == nil {
		return
	}
	execution.DecisionID = env.DeferredDecisionID
	env.DeferredDecisionID = ""
	s.refreshRun(run)
	if run.Decision != nil && run.Decision.Request.QuestionRoute != nil {
		active := turn.TurnID == run.Decision.Request.QuestionRoute.TurnID && !turn.Settled
		if !active && run.Decision.Status == "pending" {
			s.setDecisionStatus(run, "superseded")
			execution.DecisionID = ""
			s.refreshRun(run)
		}
	}
}

func (s *Service) renewEnvironmentDecision(run *Run) error {
	// Preserve the accepted answer and remark in history; a new question allows
	// another explicit check without changing the blocker or operation identity.
	request := run.Decision.Request
	s.setDecisionStatus(run, "applied")
	decision, err := WriteDecisionRequest(run, request)
	if err != nil {
		return err
	}
	s.setRunDecision(run, decision)
	s.executionRecord(run).Environment.Blocker.DecisionID = decision.ID
	return s.commitWorker(run)
}

func (s *Service) flushEnvironmentFeedback(ctx context.Context, run *Run) error {
	wf, e := s.workflowRecords(run)
	if e == nil || e.Environment == nil || ctx.Err() != nil || wf.Phase == "completed" || wf.Phase == "abandoned" {
		return nil
	}
	env := e.Environment
	if env.Blocker != nil || !env.Ready || len(env.DeferredFeedback) == 0 {
		return nil
	}
	feedback := env.DeferredFeedback[0]
	var record *DecisionRecord
	for i := range s.domain.Decisions {
		if s.domain.Decisions[i].DecisionID == feedback.DecisionID {
			record = &s.domain.Decisions[i]
			break
		}
	}
	if record == nil {
		return errors.New("deferred feedback lost its original Decision")
	}
	var answer DecisionAnswer
	var request DecisionRequest
	if json.Unmarshal(record.Answer, &answer) != nil || json.Unmarshal(record.Request, &request) != nil || record.ExecutionID != e.ExecutionID {
		return errors.New("deferred feedback identity differs from the original execution")
	}
	op := s.operationByKey("worker-environment-feedback:" + e.ExecutionID + ":" + feedback.CommandID)
	active := feedback.TurnID != "" && feedback.TurnID == e.CurrentTurnID && feedback.TurnID != e.LastStoppedTurnID &&
		(env.Preparation == nil || feedback.TurnID != env.Preparation.TurnID)
	if !active {
		// An unknown old send is read-only and remains on its original turn.
		if op != nil && op.Status != "confirmed" && op.Status != "accepted" && op.Status != "rejected" {
			input := executionapi.LifecycleInput{Feedback: &executionapi.FeedbackInput{DecisionID: record.DecisionID,
				Message: workerFeedbackText(answer, request), Answer: &executionapi.Answer{Answers: answer.Answers}}}
			confirmed, err := s.reconcileDeferredLifecycleFeedback(ctx, run, op, input, nil, feedback.TurnID)
			if err != nil || !confirmed {
				return err
			}
		}
		status := "superseded"
		if op != nil && (op.Status == "confirmed" || op.Status == "accepted") {
			status = "reply_accepted"
		}
		if record.Status != status {
			record.Status = status
			record.Version++
			record.UpdatedAt = now()
			s.markWorkerDecisionWrite(record.DecisionID)
		}
		e.LastEvent = ptr("宿主等待期间的文字保留了原目标；过期反馈未改投新轮次。")
	} else {
		host, _, _, _, hostErr := s.hostEnvironmentForRun(run)
		if hostErr != nil {
			return hostErr
		}
		fact, err := host.ObserveHost(ctx, *e.Process)
		if err != nil {
			e.LastEvent = ptr("待投递文字的原宿主观察未核实：" + clip(err.Error(), 500))
			return s.commitWorker(run)
		}
		if fact.Reason != "" {
			return s.blockWorkerEnvironment(run, fact)
		}
		if err := s.verifyHostExecutionEvidence(ctx, run, e); err != nil {
			e.LastEvent = ptr("待投递文字的原会话未核实：" + clip(err.Error(), 500))
			return s.commitWorker(run)
		}
		if err := s.deliverLifecycleEnvironmentFeedback(ctx, run, answer, feedback.CommandID, request, record); err != nil {
			return err
		}
		if record.Status != "reply_accepted" {
			return nil
		}
	}
	env.DeferredFeedback = env.DeferredFeedback[1:]
	return s.commitWorker(run)
}
