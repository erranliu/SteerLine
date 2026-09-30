package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func environmentTurn(execution *ExecutionRecord) string {
	if turn := nativeOrcaCurrentTurn(execution); turn != nil {
		if p := execution.Environment.Preparation; !p.Verified && turn.Settled && turn.TurnID != p.TurnID {
			return p.TurnID
		}
		return turn.TurnID
	}
	return execution.Environment.Preparation.TurnID
}

func (s *Service) blockWorkerEnvironment(run *Run, fact orcaworker.HostFact) error {
	wf, execution := s.workflowRecords(run)
	if execution == nil || execution.Environment == nil || wf.Phase == "completed" || wf.Phase == "abandoned" || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nil
	}
	env := execution.Environment
	turnID := environmentTurn(execution)
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
		fact, observeErr := orcaworker.ObserveHost(queryCtx, orcaWorkerMetaCaller(config), workerOrcaTerminalIdentity(*identity))
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
		return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "preparation_unverified", Confidence: "unverified", Source: "preparation.deadline", Turn: env.Preparation.TurnID})
	}
	if turn := nativeOrcaCurrentTurn(execution); turn != nil && turn.Purpose == "business" && !turn.Settled && turn.DoneMessageID == "" && execution.CurrentTurnID != turn.TurnID && !now().Before(turn.IntentAt.Add(positiveDuration(config.Orca.WaitTimeout, 2*time.Minute))) {
		return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "business_start_unverified", Confidence: "unverified", Source: "dispatch.intent.deadline", Turn: turn.TurnID})
	}
	return nil
}

func (s *Service) setOrcaQuestion(run *Run, decision *DecisionState) {
	execution := s.executionRecord(run)
	if execution.Environment != nil && execution.Environment.Blocker == nil && decision.Request.OrcaAsk != nil {
		if turn := nativeOrcaCurrentTurn(execution); turn != nil && turn.Purpose == "readiness" && run.Decision != nil && run.Decision.Request.OrcaAsk == nil {
			execution.Environment.DeferredDecisionID = run.Decision.ID
		}
	}
	if execution.Environment == nil || execution.Environment.Blocker == nil {
		s.setRunDecision(run, decision)
		return
	}
	hostDecision := execution.DecisionID
	s.setRunDecision(run, decision)
	execution.Environment.DeferredDecisionID = decision.ID
	execution.DecisionID = hostDecision
	s.refreshRun(run)
}

// Preparation owns one persisted budget spanning resource creation and native
// settlement. Each actor pass performs bounded work; restarts reuse its intent.
func (s *Service) advanceOrcaPreparation(ctx context.Context, run *Run) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	execution, env := fixed.execution, fixed.execution.Environment
	if env == nil {
		return errors.New("managed Orca execution has no persisted readiness intent")
	}
	if err := s.observeWorkerEnvironment(ctx, run); err != nil {
		return err
	}
	if env.Blocker != nil {
		return nil
	}
	if env.Preparation.Verified && !env.Preparation.Invalidated {
		env.Ready = true
		if execution.PendingTurnID != "" {
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
	if err = s.prepareNativeOrcaWorker(prepareCtx, run); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "resource_preparation_unverified", Confidence: "unverified", Source: "orca.resource.operation", Detail: clip(err.Error(), 1000)})
	}
	if err := s.observeWorkerEnvironment(ctx, run); err != nil {
		return err
	}
	if env.Blocker != nil {
		return nil
	}
	p := env.Preparation
	if turn := nativeOrcaCurrentTurn(execution); turn == nil || turn.DispatchID == "" {
		fact, ready, err := orcaworker.InspectAgentInput(prepareCtx, orcaWorkerMetaCaller(fixed.config), workerOrcaTerminalIdentity(*execution.Process))
		if err != nil {
			return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "agent_input_unverified", Confidence: "unverified", Source: "orca.terminal.input", Detail: clip(err.Error(), 500)})
		}
		if !ready {
			return s.blockWorkerEnvironment(run, fact)
		}
	}
	identity := orcaworker.ProbeIdentity{Execution: execution.ExecutionID, Turn: p.TurnID, Nonce: p.Nonce, ProfileDigest: env.ProfileDigest, Runtime: execution.Process.RuntimeID, Terminal: execution.Process.Terminal, Incarnation: execution.Process.Incarnation, Worktree: execution.Workspace.Worktree}
	if p.Command == "" {
		if err = orcaworker.EnsureReadinessIgnored(fixed.config.identity); err != nil {
			return err
		}
		p.Probe = identity
		p.Command, p.InputDigest, p.ScriptDigest, err = orcaworker.PrepareProbe(identity)
		if err != nil {
			return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "probe_preparation_failed", Confidence: "confirmed", Source: "readiness.files", Detail: err.Error()})
		}
		if err = s.commitWorker(run); err != nil {
			return err
		}
	} else if p.Probe != identity {
		return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "identity_mismatch", Confidence: "confirmed", Source: "readiness.probe.identity"})
	}
	prompt := orcaworker.ProbePrompt(p.Command, p.Nonce, p.ScriptDigest) + nativeOrcaRelayInstruction(run.RunID, p.TurnID)
	if err = s.startNativeOrcaTurn(prepareCtx, run, execution, p.TurnID, prompt); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		if env.Blocker != nil {
			return nil
		}
		return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "preparation_dispatch_unverified", Confidence: "unverified", Source: "orca.dispatch.operation", Detail: clip(err.Error(), 1000)})
	}
	return s.commitWorker(run)
}

func (s *Service) reconcileEnvironmentBusinessIntent(ctx context.Context, run *Run) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	e := fixed.execution
	op := s.operationByKey("worker-turn:" + e.ExecutionID + ":" + e.PendingTurnID)
	if op == nil {
		return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "business_dispatch_unverified", Confidence: "unverified", Source: "business.operation"})
	}
	var input struct {
		Prompt string `json:"prompt"`
	}
	if json.Unmarshal(op.Request, &input) != nil || input.Prompt == "" {
		return errors.New("saved business input is invalid")
	}
	if err := s.startNativeOrcaTurn(ctx, run, e, e.PendingTurnID, input.Prompt); err != nil {
		return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "business_dispatch_unverified", Confidence: "unverified", Source: "business.operation", Detail: clip(err.Error(), 1000)})
	}
	turn := nativeOrcaCurrentTurn(e)
	if op.Status != "confirmed" && op.Status != "accepted" {
		return s.changeWorkerOperation(run, op.OperationID, "accepted", turn.DispatchRequestID, mustJSON(*turn), "")
	}
	return nil
}

func (s *Service) finishOrcaPreparation(ctx context.Context, run *Run, execution *ExecutionRecord, turn *OrcaNativeTurn, outcome string) error {
	p := execution.Environment.Preparation
	if turn.TurnID != p.TurnID {
		return nil
	}
	if outcome != "succeeded" {
		return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "preparation_failed", Confidence: "confirmed", Source: "orca.dispatch.settlement", Turn: turn.TurnID})
	}
	if p.Receipt == nil {
		receipt, err := orcaworker.ReadProbe(p.Probe, p.InputDigest)
		if err != nil {
			return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "probe_receipt_invalid", Confidence: "confirmed", Source: "readiness.receipt", Detail: err.Error(), Turn: turn.TurnID})
		}
		p.Receipt = receipt
	}
	if p.Receipt == nil || !p.Evidence.Verified || p.Invalidated {
		return s.commitWorker(run)
	}
	p.Verified = true
	if run.Decision != nil && run.Decision.Request.OrcaAsk != nil && run.Decision.Request.OrcaAsk.DispatchID == turn.DispatchID {
		if run.Decision.Status == "answer_saved" {
			return s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "answer_delivery_unverified", Confidence: "unverified", Source: "orca.question.reply", Turn: turn.TurnID})
		}
		if run.Decision.Status == "pending" {
			s.setDecisionStatus(run, "superseded")
		}
		s.restoreEnvironmentDecision(run)
	}
	if !now().Before(p.Deadline) && !execution.Environment.Ready && execution.Environment.Blocker == nil {
		if err := s.blockWorkerEnvironment(run, orcaworker.HostFact{Reason: "preparation_unverified", Confidence: "unverified", Source: "preparation.deadline", Turn: turn.TurnID}); err != nil {
			return err
		}
	}
	if err := s.commitWorker(run); err != nil {
		return err
	}
	if err := orcaworker.CleanupProbe(p.Probe); err != nil {
		execution.LastEvent = ptr("准备证据已保存；文件清理待核实：" + err.Error())
	}
	removeNativeOrcaRelayRequest(execution.Workspace.Worktree, run.RunID, turn.TurnID)
	if execution.Environment.Blocker != nil {
		return nil
	}
	return s.advanceOrcaPreparation(ctx, run)
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
		fact, err := orcaworker.ObserveHost(ctx, orcaWorkerMetaCaller(fixed.config), workerOrcaTerminalIdentity(*identity))
		if err != nil {
			return err
		}
		if fact.Reason == "identity_mismatch" || fact.Reason == "terminal_exited" {
			return errors.New("original terminal cannot be located")
		}
		// Switching is a locating UI action; it grants no permissions.
		_, err = nativeOrcaClient(fixed.config).MetaCall(ctx, "terminal", "switch", "--terminal", identity.Terminal, "--json")
		if err != nil {
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
		fact, err := orcaworker.ObserveHost(ctx, orcaWorkerMetaCaller(fixed.config), workerOrcaTerminalIdentity(*execution.Process))
		if err != nil || fact.Reason != "" {
			return s.renewEnvironmentDecision(run)
		}
		if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
			return s.renewEnvironmentDecision(run)
		}
	}
	turn := nativeOrcaCurrentTurn(execution)
	if turn != nil && turn.DispatchID == "" && turn.TaskID != "" && execution.Process != nil {
		key := "orca-native-dispatch:" + execution.ExecutionID + ":" + turn.TurnID
		if turn.InjectionAttempts > 0 {
			key += fmt.Sprintf(":retry:%d", turn.InjectionAttempts)
		}
		if original := s.operationByKey(key); original != nil && original.Status != "rejected" {
			id, err := orcaworker.RecoverDispatch(ctx, orcaWorkerMetaCaller(fixed.config), turn.TaskID, execution.Target.OrchestrationRun, *execution.Process)
			if err != nil {
				return s.renewEnvironmentDecision(run)
			}
			turn.DispatchID = id
			s.bindNativeOrcaDispatch(run, execution, turn)
			if original.Status != "accepted" && original.Status != "confirmed" {
				if err := s.changeWorkerOperation(run, original.OperationID, "accepted", original.PlatformRequestID, mustJSON(*turn), ""); err != nil {
					return err
				}
			}
		}
	}
	if turn != nil && turn.DispatchID != "" {
		if err := orcaworker.VerifyRecoveredDispatch(ctx, orcaWorkerMetaCaller(fixed.config), orcaworker.DispatchIdentity{DispatchID: turn.DispatchID, TaskID: turn.TaskID, RunID: execution.Target.OrchestrationRun, RuntimeID: execution.Process.RuntimeID, Terminal: execution.Process.Terminal, Incarnation: execution.Process.Incarnation, WorktreeID: execution.Process.WorktreeID}); err != nil {
			return s.renewEnvironmentDecision(run)
		}
	}
	if turn != nil && turn.Purpose == "readiness" && !env.Preparation.Invalidated {
		// An explicit recheck may reread a settled probe's historical tool records
		// when earlier bounded/unsupported pages provided no tool proof.
		if turn.Settled && !env.Preparation.Evidence.Verified && env.Preparation.Evidence.CallID == "" {
			turn.OutputCursor = ""
		}
		if err := s.pollNativeOrcaWorker(ctx, run); err != nil {
			return err
		}
		if !env.Preparation.Verified && !now().Before(env.Preparation.Deadline) {
			return s.renewEnvironmentDecision(run)
		}
	}
	if blocker.Fact.Reason == "business_start_unverified" && turn != nil && execution.CurrentTurnID != turn.TurnID {
		if err := s.pollNativeOrcaWorker(ctx, run); err != nil {
			return err
		}
		if execution.CurrentTurnID != turn.TurnID && !turn.Settled {
			return s.renewEnvironmentDecision(run)
		}
	}
	// Polling can install a newer blocker; an old action cannot clear it.
	if env.Blocker == nil || env.Blocker.ID != blocker.ID || run.Decision == nil || run.Decision.ID != actionDecisionID || run.Decision.Version != actionDecisionVersion || environmentTurn(execution) != target.TurnID {
		return s.changeWorkerOperation(run, op.OperationID, "confirmed", commandID, mustJSON(map[string]string{"result": "stale_target", "blocker_id": blocker.ID}), "")
	}
	// Reconcile original resource/Dispatch operations through their exact saved
	// keys. No retry creates another terminal or Task when a result is unknown.
	if env.Preparation.Invalidated {
		if turn == nil || !turn.Settled {
			return s.renewEnvironmentDecision(run)
		}
		env.History = append(env.History, *env.Preparation)
		started := now()
		env.Preparation = &WorkerPreparation{TurnID: newID(), Nonce: newID(), StartedAt: started, Deadline: started.Add(positiveDuration(fixed.config.Orca.WaitTimeout, 2*time.Minute))}
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
		return s.advanceOrcaPreparation(ctx, run)
	}
	if turn != nil && turn.DispatchID != "" && !turn.Settled {
		return s.pollNativeOrcaWorker(ctx, run)
	}
	return s.advanceOrcaPreparation(ctx, run)
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
	if request.OrcaAsk == nil {
		return true, nil
	}
	settled := false
	for _, turn := range e.OrcaNative.Turns {
		if turn.DispatchID == request.OrcaAsk.DispatchID {
			settled = turn.Settled
		}
	}
	if !settled {
		return true, nil
	}
	status := "superseded"
	if record.Status == "answer_saved" {
		var answer DecisionAnswer
		if json.Unmarshal(record.Answer, &answer) != nil {
			return false, errors.New("original saved answer is invalid")
		}
		command, err := s.domainStore.CommandByIdempotency(ctx, s.Config.identity, decisionAnswerKey(answer))
		if err != nil {
			return false, err
		}
		op := s.operationByKey("orca-native-reply:" + e.ExecutionID + ":" + command.CommandID)
		if op != nil {
			if op.Status != "confirmed" && op.Status != "accepted" {
				config, err := s.executionConfigForRun(run)
				if err != nil {
					return false, err
				}
				ask := request.OrcaAsk
				id, err := findNativeOrcaReply(ctx, config, ask.RunID, ask.DispatchID, ask.MessageID, nativeOrcaAnswerText(answer, request))
				if err != nil || id == "" {
					return false, nil
				}
				if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"reply_message_id": id}), ""); err != nil {
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
	execution.DecisionID = env.DeferredDecisionID
	env.DeferredDecisionID = ""
	s.refreshRun(run)
	if run.Decision != nil && run.Decision.Request.OrcaAsk != nil {
		ask := run.Decision.Request.OrcaAsk
		active := false
		for _, turn := range execution.OrcaNative.Turns {
			if turn.DispatchID == ask.DispatchID && !turn.Settled {
				active = true
			}
		}
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
	var turn *OrcaNativeTurn
	for i := range e.OrcaNative.Turns {
		if e.OrcaNative.Turns[i].TurnID == feedback.TurnID {
			turn = &e.OrcaNative.Turns[i]
			break
		}
	}
	config, err := s.executionConfigForRun(run)
	if err != nil {
		return err
	}
	op := s.operationByKey("orca-native-send:" + e.ExecutionID + ":" + feedback.CommandID)
	if turn == nil || turn.Settled || turn.Purpose != "business" || nativeOrcaCurrentTurn(e).TurnID != turn.TurnID {
		// Unknown delivery stays bound to its old Dispatch and only reads history.
		if op != nil && op.Status != "confirmed" && op.Status != "accepted" && op.Status != "rejected" && turn != nil {
			id, err := findNativeOrcaSend(ctx, config, e.Target.OrchestrationRun, turn.DispatchID, "SteerLine feedback "+feedback.CommandID, workerFeedbackText(answer, request))
			if err != nil || id == "" {
				return nil
			}
			if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"message_id": id}), ""); err != nil {
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
		fact, err := orcaworker.ObserveHost(ctx, orcaWorkerMetaCaller(config), workerOrcaTerminalIdentity(*e.Process))
		if err != nil {
			e.LastEvent = ptr("待投递文字的原宿主观察未核实：" + clip(err.Error(), 500))
			return s.commitWorker(run)
		}
		if fact.Reason != "" {
			return s.blockWorkerEnvironment(run, fact)
		}
		if err := s.verifyNativeOrcaWorker(ctx, run, e); err != nil {
			e.LastEvent = ptr("待投递文字的原会话未核实：" + clip(err.Error(), 500))
			return s.commitWorker(run)
		}
		if err := s.sendNativeOrcaFeedbackToDecision(ctx, run, wf, e, turn, answer, feedback.CommandID, request, record.DecisionID, false); err != nil {
			return err
		}
		if record.Status != "reply_accepted" {
			return nil
		}
	}
	env.DeferredFeedback = env.DeferredFeedback[1:]
	return s.commitWorker(run)
}
