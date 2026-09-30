package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

// Native Orca Dispatches are unsupervised. The center owns the exact agent
// terminal created with the worktree and never uses Codex's exec protocol.
type orcaAskCall struct {
	RunID      string              `json:"run_id"`
	TaskID     string              `json:"task_id"`
	DispatchID string              `json:"dispatch_id"`
	MessageID  string              `json:"message_id"`
	Questions  []nativeAskQuestion `json:"questions"`
}

func orcaQuestionID(decision *DecisionState) string {
	if decision == nil || decision.Request.OrcaAsk == nil {
		return ""
	}
	return decision.Request.OrcaAsk.MessageID
}

func nativeOrcaAnswerText(answer DecisionAnswer, request DecisionRequest) string {
	if request.OrcaAsk == nil {
		return workerFeedbackText(answer, request)
	}
	return strings.Join(answer.Answers[request.OrcaAsk.MessageID], "\n\n")
}

func nativeOrcaSession(p ProcessIdentity) string {
	return "orca-terminal:" + p.RuntimeID + ":" + p.Terminal + ":" + p.Incarnation
}

func nativeOrcaRepo(ctx context.Context, c Config) (string, error) {
	repoID, err := orcaworker.FindRepo(ctx, orcaWorkerCaller(c), c.Repository)
	if errors.Is(err, orcaworker.ErrRepoUnavailable) {
		return "", &BackendError{Code: BackendUnsupported, Diagnostic: err.Error()}
	}
	return repoID, err
}

func (s *Service) nativeOrcaOperation(key string) *OperationRecord {
	for i := range s.State.Operations {
		if s.State.Operations[i].IdempotencyKey == key {
			return &s.State.Operations[i]
		}
	}
	return nil
}

func nativeOrcaTerminals(ctx context.Context, c Config, worktree string) ([]orcaTerminal, string, error) {
	return orcaworker.ListTerminals(ctx, orcaWorkerMetaCaller(c), worktree)
}

func nativeOrcaWorktreeComment(executionID string) string {
	return "SteerLine execution " + executionID
}

func nativeOrcaCoordinatorMarker(executionID string) string {
	return "SLCOORD:" + executionID
}

func nativeOrcaTerminalHasMarker(ctx context.Context, c Config, handle, marker string) (bool, error) {
	return orcaworker.TerminalHasMarker(ctx, orcaWorkerCaller(c), handle, marker)
}
func findNativeOrcaRun(ctx context.Context, c Config, objective, coordinator string) (string, error) {
	return orcaworker.FindRun(ctx, orcaWorkerCaller(c), objective, coordinator)
}
func findNativeOrcaTask(ctx context.Context, c Config, runID, coordinator, spec string) (string, error) {
	return orcaworker.FindTask(ctx, orcaWorkerCaller(c), runID, coordinator, spec)
}
func findNativeOrcaDispatch(ctx context.Context, c Config, taskID string) (string, string, string, string, error) {
	return orcaworker.FindDispatch(ctx, orcaWorkerCaller(c), taskID)
}
func findNativeOrcaReply(ctx context.Context, c Config, runID, dispatchID, questionID, answer string) (string, error) {
	return orcaworker.FindReply(ctx, orcaWorkerCaller(c), runID, dispatchID, questionID, answer)
}
func findNativeOrcaSend(ctx context.Context, c Config, runID, dispatchID, subject, body string) (string, error) {
	return orcaworker.FindSend(ctx, orcaWorkerCaller(c), runID, dispatchID, subject, body)
}
func verifyRecoveredNativeOrcaDispatch(ctx context.Context, c Config, dispatchID, taskID, runID string, identity ProcessIdentity) error {
	err := orcaworker.VerifyRecoveredDispatch(ctx, orcaWorkerMetaCaller(c), orcaworker.DispatchIdentity{
		DispatchID: dispatchID, TaskID: taskID, RunID: runID,
		RuntimeID: identity.RuntimeID, Terminal: identity.Terminal,
		Incarnation: identity.Incarnation, WorktreeID: identity.WorktreeID,
	})
	if errors.Is(err, orcaworker.ErrDispatchIdentity) {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: err.Error()}
	}
	return err
}

func (s *Service) bindNativeOrcaAgent(ctx context.Context, run *Run, execution *ExecutionRecord, worktree workerOrcaWorktree, handle, repoID, base string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	createdWithExpectedAgent := worktree.CreatedWithAgent == fixedConfig.agentName()
	if fixedConfig.Model != "" {
		createdWithExpectedAgent = worktree.CreatedWithAgent == ""
	}
	if worktree.Comment != nativeOrcaWorktreeComment(execution.ExecutionID) || !createdWithExpectedAgent {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca worktree has no matching execution/agent ownership marker"}
	}
	if err := applyWorkerOrcaWorkspace(fixedConfig, run, execution, worktree, base); err != nil {
		return err
	}
	if handle == "" {
		terms, runtimeID, err := nativeOrcaTerminals(ctx, fixedConfig, "id:"+worktree.ID)
		if err != nil {
			return err
		}
		if len(terms) != 1 || terms[0].AgentIdentity != fixedConfig.agentName() || runtimeID == "" {
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca worktree has no unique original agent terminal"}
		}
		handle = terms[0].Handle
	}
	term, err := showWorkerOrcaTerminal(ctx, fixedConfig, handle)
	if err != nil {
		return err
	}
	if err := verifyWorkerOrcaTerminal(term, run, nil); err != nil {
		return err
	}
	if term.WorktreeID != worktree.ID || term.AgentIdentity != fixedConfig.agentName() {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca started a different worktree or agent"}
	}
	identity := workerOrcaIdentity(term, fixedConfig)
	identity.Kind, identity.WorktreeID = "orca-native", worktree.ID
	execution.Process = &identity
	s.refreshRun(run)
	execution.OrcaNative = &OrcaNativeExecution{RepositoryID: repoID}
	return s.save()
}

func (s *Service) prepareNativeOrcaWorker(ctx context.Context, run *Run) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || run.WorkspaceIntent == nil || fixedConfig.hostName() != "orca" {
		return errors.New("native Orca preparation lacks a fixed worker claim")
	}
	if execution.Process != nil {
		if execution.Process.Kind != "orca-native" || execution.OrcaNative == nil {
			return errors.New("old Orca worker state cannot be continued by the native protocol")
		}
		if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
			return err
		}
		if fixedConfig.Model != "" {
			if op := s.nativeOrcaOperation("orca-native-agent-terminal:" + execution.ExecutionID); op != nil && op.Status != "confirmed" {
				receipt := op.Receipt
				if len(receipt) == 0 {
					receipt = mustJSON(map[string]string{"terminal": execution.Process.Terminal})
				}
				if err := s.changeOperation(op.OperationID, "confirmed", op.PlatformRequestID, receipt, ""); err != nil {
					return err
				}
			}
		}
		return s.prepareNativeOrcaCoordinator(ctx, run, execution)
	}
	if fixedConfig.Model != "" {
		return s.prepareNativeOrcaFixedModelWorker(ctx, run, execution)
	}
	repoID, err := nativeOrcaRepo(ctx, fixedConfig)
	if err != nil {
		return err
	}
	name, err := workerOrcaWorktreeName(run.RunID)
	if err != nil {
		return err
	}
	base := run.WorkspaceIntent.BaseCommit
	worktree, exists, err := findWorkerOrcaWorktree(ctx, fixedConfig, repoID, name, base)
	if err != nil {
		return err
	}
	key := "orca-native-worktree:" + execution.ExecutionID
	if op := s.nativeOrcaOperation(key); op != nil {
		if !exists {
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original Orca worktree creation has no unique resource; do not create another agent"}
		}
		var receipt struct {
			AgentTerminalHandle string `json:"agentTerminalHandle"`
			StartupTerminal     struct {
				Handle string `json:"handle"`
			} `json:"startupTerminal"`
		}
		if len(op.Receipt) > 0 && json.Unmarshal(op.Receipt, &receipt) != nil {
			return errors.New("saved Orca agent receipt cannot be decoded")
		}
		handle := receipt.AgentTerminalHandle
		if handle == "" {
			handle = receipt.StartupTerminal.Handle
		}
		if err := s.bindNativeOrcaAgent(ctx, run, execution, worktree, handle, repoID, base); err != nil {
			return err
		}
		if op.Status != "confirmed" {
			if err := s.changeOperation(op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(worktree), ""); err != nil {
				return err
			}
		}
		return s.prepareNativeOrcaCoordinator(ctx, run, execution)
	}
	if exists {
		return errors.New("fixed Orca worktree already exists without the original creation intent; preserve it for manual reconciliation")
	}
	opID, err := s.startOperation("orca_native_worktree", key, map[string]string{"repo_id": repoID, "name": name, "base": base, "agent": fixedConfig.agentName(), "comment": nativeOrcaWorktreeComment(execution.ExecutionID)})
	if err != nil {
		return err
	}
	platform, callErr := orcaworker.CreateNativeWorktree(ctx, orcaWorkerMetaCaller(fixedConfig), repoID, name, base, fixedConfig.agentName(), nativeOrcaWorktreeComment(execution.ExecutionID))
	if callErr != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, callErr.Error())
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca worktree/agent creation has no confirmed receipt; do not create another agent", Cause: callErr}
	}
	receipt, err := orcaworker.ParseNativeWorktreeReceipt(platform, name, repoID, base)
	if err != nil {
		if errors.Is(err, orcaworker.ErrAgentHandleOmitted) {
			_ = s.changeOperation(opID, "outcome_unknown", "", platform.Payload, "agent handle omitted")
			return err
		}
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		return workerOrcaWorkspaceError(err)
	}
	handle := receipt.Handle
	if err := s.changeOperation(opID, "accepted", "", platform.Payload, ""); err != nil {
		return err
	}
	if err := s.bindNativeOrcaAgent(ctx, run, execution, receipt.Worktree, handle, repoID, base); err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", platform.Payload, err.Error())
		return err
	}
	if err := s.changeOperation(opID, "confirmed", "", platform.Payload, ""); err != nil {
		return err
	}
	return s.prepareNativeOrcaCoordinator(ctx, run, execution)
}

func (s *Service) prepareNativeOrcaCoordinator(ctx context.Context, run *Run, execution *ExecutionRecord) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	state := execution.OrcaNative
	if state == nil || execution.Process == nil {
		return errors.New("Orca agent binding is missing")
	}
	if state.Coordinator.Terminal == "" {
		key := "orca-native-coordinator:" + execution.ExecutionID
		title := "SteerLine coordinator " + run.RunID
		marker := nativeOrcaCoordinatorMarker(execution.ExecutionID)
		if op := s.nativeOrcaOperation(key); op != nil {
			handle := ""
			if len(op.Receipt) > 0 {
				var receipt struct {
					Terminal orcaTerminal `json:"terminal"`
				}
				if err := json.Unmarshal(op.Receipt, &receipt); err != nil {
					return err
				}
				handle = receipt.Terminal.Handle
			}
			if handle == "" {
				terms, runtimeID, err := nativeOrcaTerminals(ctx, fixedConfig, "path:"+filepath.ToSlash(fixedConfig.Repository))
				if err != nil {
					return err
				}
				if runtimeID != execution.Process.RuntimeID {
					return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca coordinator runtime changed"}
				}
				for _, candidate := range terms {
					if candidate.AgentIdentity != "" {
						continue
					}
					found, err := nativeOrcaTerminalHasMarker(ctx, fixedConfig, candidate.Handle, marker)
					if err != nil {
						return err
					}
					if found {
						if handle != "" {
							return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original Orca coordinator is not uniquely identifiable"}
						}
						handle = candidate.Handle
					}
				}
			}
			if handle == "" {
				return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original Orca coordinator creation is unverified; do not create another"}
			}
			if err := s.bindNativeOrcaCoordinator(ctx, run, execution, handle); err != nil {
				return err
			}
			if op.Status != "confirmed" {
				if err := s.changeOperation(op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(state.Coordinator), ""); err != nil {
					return err
				}
			}
		} else {
			opID, err := s.startOperation("orca_native_coordinator", key, map[string]string{"execution_id": execution.ExecutionID, "repository": fixedConfig.Repository, "title": title})
			if err != nil {
				return err
			}
			platform, err := orcaworker.CreateCoordinatorTerminal(ctx, orcaWorkerMetaCaller(fixedConfig), fixedConfig.Repository, title, marker)
			if err != nil {
				_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
				return err
			}
			handle, err := orcaworker.ParseCoordinatorTerminalReceipt(platform)
			if err != nil {
				_ = s.changeOperation(opID, "outcome_unknown", "", platform.Payload, "coordinator handle omitted")
				return err
			}
			if err := s.changeOperation(opID, "accepted", "", platform.Payload, ""); err != nil {
				return err
			}
			if err := s.bindNativeOrcaCoordinator(ctx, run, execution, handle); err != nil {
				return err
			}
			if err := s.changeOperation(opID, "confirmed", "", platform.Payload, ""); err != nil {
				return err
			}
		}
	}
	if execution.Target.OrchestrationRun == "" {
		objective := "SteerLine worker " + execution.ExecutionID
		key := "orca-native-run:" + execution.ExecutionID
		if op := s.nativeOrcaOperation(key); op != nil {
			id, err := findNativeOrcaRun(ctx, fixedConfig, objective, state.Coordinator.Terminal)
			if err != nil {
				return err
			}
			if id == "" {
				return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original Orca Run creation has no matching unique Run"}
			}
			execution.Target.OrchestrationRun = id
			s.refreshRun(run)
			if op.Status != "confirmed" {
				if err := s.changeOperation(op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"run_id": id}), ""); err != nil {
					return err
				}
			} else if err := s.save(); err != nil {
				return err
			}
			return nil
		}
		opID, err := s.startOperation("orca_native_run", key, map[string]string{"objective": objective, "coordinator": state.Coordinator.Terminal})
		if err != nil {
			return err
		}
		receipt, err := orcaworker.CreateRun(ctx, orcaWorkerMetaCaller(fixedConfig), objective, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
		if err != nil {
			var callErr *orcaworker.CallError
			if errors.As(err, &callErr) {
				_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
			}
			return err
		}
		execution.Target.OrchestrationRun = receipt.ID
		execution.Target.OrchestrationRunRequestID = receipt.RequestID
		execution.Target.OrchestrationRun = receipt.ID
		s.refreshRun(run)
		if err := s.changeOperation(opID, "confirmed", receipt.RequestID, receipt.Payload, ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) bindNativeOrcaCoordinator(ctx context.Context, run *Run, execution *ExecutionRecord, handle string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	term, err := showWorkerOrcaTerminal(ctx, fixedConfig, handle)
	if err != nil {
		return err
	}
	if canonical(filepath.FromSlash(term.WorktreePath)) != canonical(fixedConfig.Repository) || term.RuntimeID != execution.Process.RuntimeID || term.AgentIdentity != "" {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca coordinator is not the fixed shell on the local runtime/repository"}
	}
	execution.OrcaNative.Coordinator = ProcessIdentity{Kind: "orca-native-coordinator", Terminal: term.Handle, RuntimeID: term.RuntimeID, Incarnation: term.IncarnationID, Executable: fixedConfig.Orca.Executable}
	execution.Target.CoordinatorTerminal = term.Handle
	execution.Target.CoordinatorTerminal = term.Handle
	s.refreshRun(run)
	return s.save()
}

func (s *Service) verifyNativeOrcaWorker(ctx context.Context, run *Run, execution *ExecutionRecord) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	if execution.OrcaNative == nil || execution.Process == nil || execution.Process.Kind != "orca-native" {
		return errors.New("native Orca worker binding is incomplete")
	}
	term, err := showWorkerOrcaTerminal(ctx, fixedConfig, execution.Process.Terminal)
	if err != nil {
		return err
	}
	if err := verifyWorkerOrcaTerminal(term, run, nil); err != nil {
		return err
	}
	if term.RuntimeID != execution.Process.RuntimeID || term.IncarnationID != execution.Process.Incarnation || term.WorktreeID != execution.Process.WorktreeID || term.AgentIdentity != fixedConfig.agentName() {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "original Orca agent terminal or agent has changed"}
	}
	coordinator := execution.OrcaNative.Coordinator
	if coordinator.Terminal != "" {
		shown, err := showWorkerOrcaTerminal(ctx, fixedConfig, coordinator.Terminal)
		if err != nil {
			return err
		}
		if shown.RuntimeID != coordinator.RuntimeID || shown.IncarnationID != coordinator.Incarnation {
			return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "original Orca coordinator changed"}
		}
	}
	return nil
}

func (s *Service) startNativeOrcaTurn(ctx context.Context, run *Run, execution *ExecutionRecord, turnID, prompt string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
		return err
	}
	if err := ensureNativeOrcaRelayIgnored(ctx, fixedConfig, execution.Workspace.Worktree); err != nil {
		return err
	}
	if len([]byte(prompt)) > 16<<10 || len([]rune(prompt)) > 12000 {
		return &BackendError{Code: BackendRejected, Diagnostic: "Orca native task spec exceeds the conservative Windows command-line bound"}
	}
	if execution.OrcaNative.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		return errors.New("Orca coordinator and Run are not prepared")
	}
	state := execution.OrcaNative
	var current *OrcaNativeTurn
	for i := range state.Turns {
		if state.Turns[i].TurnID == turnID {
			current = &state.Turns[i]
			break
		}
	}
	if current != nil && current.DispatchID != "" {
		return nil
	}
	taskKey := "orca-native-task:" + execution.ExecutionID + ":" + turnID
	if current == nil {
		if op := s.nativeOrcaOperation(taskKey); op != nil {
			id, err := findNativeOrcaTask(ctx, fixedConfig, execution.Target.OrchestrationRun, state.Coordinator.Terminal, prompt)
			if err != nil {
				return err
			}
			if id == "" {
				return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original Orca Task creation has no unique matching Task"}
			}
			state.Turns = append(state.Turns, OrcaNativeTurn{TurnID: turnID, TaskID: id, TaskRequestID: op.PlatformRequestID})
			execution.Target.ExternalTask = id
			s.refreshRun(run)
			if op.Status != "confirmed" {
				if err := s.changeOperation(op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"task_id": id}), ""); err != nil {
					return err
				}
			} else if err := s.save(); err != nil {
				return err
			}
		} else {
			taskOp, err := s.startOperation("orca_native_task", taskKey, map[string]string{"turn_id": turnID, "spec": prompt, "run": execution.Target.OrchestrationRun})
			if err != nil {
				return err
			}
			receipt, err := orcaworker.CreateTask(ctx, orcaWorkerMetaCaller(fixedConfig), prompt, run.Title, execution.Target.OrchestrationRun, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
			if err != nil {
				var callErr *orcaworker.CallError
				if errors.As(err, &callErr) {
					_ = s.changeOperation(taskOp, "outcome_unknown", "", nil, err.Error())
				}
				return err
			}
			state.Turns = append(state.Turns, OrcaNativeTurn{TurnID: turnID, TaskID: receipt.ID, TaskRequestID: receipt.RequestID})
			execution.Target.ExternalTask = receipt.ID
			s.refreshRun(run)
			if err := s.changeOperation(taskOp, "confirmed", receipt.RequestID, receipt.Payload, ""); err != nil {
				return err
			}
		}
		current = &state.Turns[len(state.Turns)-1]
	}
	taskID := current.TaskID
	dispatchKey := "orca-native-dispatch:" + execution.ExecutionID + ":" + turnID
	if op := s.nativeOrcaOperation(dispatchKey); op != nil {
		id, runID, handle, incarnation, err := findNativeOrcaDispatch(ctx, fixedConfig, taskID)
		if err != nil {
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original Orca Dispatch creation is unverified", Cause: err}
		}
		if runID != execution.Target.OrchestrationRun || handle != execution.Process.Terminal || !strings.HasSuffix(incarnation, ":"+execution.Process.Incarnation) {
			return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "recovered Orca Dispatch targets a different worker or Run"}
		}
		if err := verifyRecoveredNativeOrcaDispatch(ctx, fixedConfig, id, taskID, runID, *execution.Process); err != nil {
			return err
		}
		current.DispatchID, current.DispatchRequestID = id, op.PlatformRequestID
		s.bindNativeOrcaDispatch(run, execution, current)
		if op.Status != "accepted" && op.Status != "confirmed" {
			return s.changeOperation(op.OperationID, "accepted", op.PlatformRequestID, mustJSON(map[string]string{"dispatch_id": id}), "")
		}
		return s.save()
	}
	dispatchOp, err := s.startOperation("orca_native_dispatch", dispatchKey, map[string]string{"task": taskID, "terminal": execution.Process.Terminal, "run": execution.Target.OrchestrationRun})
	if err != nil {
		return err
	}
	deadline := time.Now().Add(positiveDuration(fixedConfig.Orca.WaitTimeout, 2*time.Minute))
	var env orcaworker.Result
	for {
		env, err = orcaworker.Dispatch(ctx, orcaWorkerMetaCaller(fixedConfig), taskID, execution.Process.Terminal, execution.Target.OrchestrationRun, state.Coordinator.Terminal)
		if err == nil {
			break
		}
		if !orcaworker.NoAgentDetected(env) {
			_ = s.changeOperation(dispatchOp, "outcome_unknown", "", nil, err.Error())
			return err
		}
		if time.Now().After(deadline) {
			_ = s.changeOperation(dispatchOp, "rejected", "", nil, err.Error())
			return err
		}
		if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	receipt, err := orcaworker.DispatchReceipt(env, taskID, execution.Target.OrchestrationRun, state.Coordinator.RuntimeID)
	if err != nil {
		return err
	}
	current.DispatchID, current.DispatchRequestID = receipt.ID, receipt.RequestID
	s.bindNativeOrcaDispatch(run, execution, current)
	if err := s.changeOperation(dispatchOp, "accepted", receipt.RequestID, receipt.Payload, ""); err != nil {
		return err
	}
	return nil
}

func (s *Service) bindNativeOrcaDispatch(run *Run, execution *ExecutionRecord, turn *OrcaNativeTurn) {
	execution.Target.ExternalTask = turn.TaskID
	s.refreshRun(run)
	execution.Target.Dispatch = turn.DispatchID
	s.refreshRun(run)
	execution.Process.Task, execution.Process.Dispatch, execution.Process.OrchestrationRun = turn.TaskID, turn.DispatchID, execution.Target.OrchestrationRun
	s.refreshRun(run)
	if attempt := s.workflowAttempt(run, run.AttemptID); attempt != nil {
		attempt.ExternalBinding = execution.Target
		attempt.OutputSource, attempt.OutputCursor = "", ""
	}
}

func nativeOrcaCurrentTurn(execution *ExecutionRecord) *OrcaNativeTurn {
	if execution == nil || execution.OrcaNative == nil || len(execution.OrcaNative.Turns) == 0 {
		return nil
	}
	return &execution.OrcaNative.Turns[len(execution.OrcaNative.Turns)-1]
}

func nativeOrcaQuestion(message OrcaInboxMessage, runID string, turn OrcaNativeTurn) DecisionRequest {
	question := strings.TrimSpace(message.Body)
	if question == "" {
		question = strings.TrimSpace(message.Subject)
	}
	request := DecisionRequest{Question: clip(question, 1800), Context: clip(message.Body, 7000), AllowFreeText: true}
	if payload, err := supervisedMessagePayload(message); err == nil {
		parsed := orcaworker.ParseQuestion(payload)
		if strings.TrimSpace(parsed.Text) != "" {
			request.Question = clip(parsed.Text, 1800)
		}
		for _, option := range parsed.Options {
			request.Options = append(request.Options, DecisionOption{ID: option.ID, Label: option.Label, Detail: option.Detail})
		}
	}
	request.OrcaAsk = &orcaAskCall{RunID: runID, TaskID: turn.TaskID, DispatchID: turn.DispatchID, MessageID: message.ID, Questions: []nativeAskQuestion{{ID: message.ID, Header: "Orca", Question: request.Question, Options: request.Options, AllowFreeText: true}}}
	return request
}

func nativeOrcaMessageMatches(message OrcaInboxMessage, turn OrcaNativeTurn, agentTerminal string) bool {
	payload, err := supervisedMessagePayload(message)
	return err == nil && orcaworker.MessageMatches(message.FromHandle, message.Type, payload, turn.TaskID, turn.DispatchID, agentTerminal)
}

func nativeOrcaDoneOutcome(message OrcaInboxMessage, turn OrcaNativeTurn) (string, error) {
	payload, err := supervisedMessagePayload(message)
	if err != nil {
		return "", err
	}
	return orcaworker.DoneOutcome(payload, turn.TaskID, turn.DispatchID)
}

func nativeOrcaSessionIdentity(execution *ExecutionRecord) (string, error) {
	if execution == nil || execution.Process == nil || execution.Process.Kind != "orca-native" || execution.Process.RuntimeID == "" || execution.Process.Terminal == "" || execution.Process.Incarnation == "" {
		return "", fmt.Errorf("native Orca session origin is incomplete")
	}
	return nativeOrcaSession(*execution.Process), nil
}

func (s *Service) beginNativeOrcaTurn(run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	if execution.CurrentTurnID == turn.TurnID {
		return nil
	}
	session, err := nativeOrcaSessionIdentity(execution)
	if err != nil {
		return err
	}
	if err := BeginWorkerTurn(workflow, execution, session, turn.TurnID); err != nil {
		return err
	}
	if attempt := s.workflowAttempt(run, run.AttemptID); attempt != nil {
		attempt.Status, attempt.UpdatedAt = "running", now()
	}
	execution.Status, execution.Error = "active", nil
	s.refreshRun(run)
	if run.Decision != nil && run.Decision.Status == "answer_saved" && orcaQuestionID(run.Decision) == "" {
		s.setDecisionStatus(run, "reply_accepted")
	}
	execution.TurnStartedAt = ptr(now())
	s.refreshRun(run)
	for _, op := range s.State.Operations {
		if op.IdempotencyKey == "worker-turn:"+execution.ExecutionID+":"+turn.TurnID && op.Status != "confirmed" {
			if err := s.setOperationState(op.OperationID, "confirmed", turn.DispatchRequestID, mustJSON(*turn), ""); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func (s *Service) applyNativeOrcaDecision(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, answer DecisionAnswer, commandID string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	turn := nativeOrcaCurrentTurn(execution)
	if execution.Process == nil || execution.Process.Kind != "orca-native" || execution.OrcaNative == nil || turn == nil || turn.DispatchID == "" {
		return s.waitWorkerFlow(run, "Original Orca Dispatch identity is unavailable; feedback was retained")
	}
	if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
		execution.Error = ptr("Feedback retained: original Orca session is unavailable: " + err.Error())
		s.refreshRun(run)
		return s.save()
	}
	if orcaQuestionID(run.Decision) != "" {
		if turn.DoneMessageID != "" || execution.LastStoppedTurnID == turn.TurnID {
			execution.Error = ptr("The original Orca question expired with its Dispatch; feedback was retained")
			s.refreshRun(run)
			return s.save()
		}
		key := "orca-native-reply:" + execution.ExecutionID + ":" + commandID
		for _, op := range s.State.Operations {
			if op.IdempotencyKey == key {
				if op.Status != "confirmed" && op.Status != "accepted" {
					messageID, err := findNativeOrcaReply(ctx, fixedConfig, execution.Target.OrchestrationRun, turn.DispatchID, orcaQuestionID(run.Decision), nativeOrcaAnswerText(answer, run.Decision.Request))
					if err != nil || messageID == "" {
						return s.nativeOrcaUncertain(run, "Orca reply outcome is unverified; the original question was not answered twice")
					}
					if err := s.changeOperation(op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"reply_message_id": messageID}), ""); err != nil {
						return err
					}
				}
				s.setDecisionStatus(run, "reply_accepted")
				if err := ContinueWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
					return err
				}
				s.refreshRun(run)
				return s.save()
			}
		}
		body := nativeOrcaAnswerText(answer, run.Decision.Request)
		opID, err := s.startOperation("orca_native_reply", key, map[string]string{"question": orcaQuestionID(run.Decision), "answer": body, "dispatch": turn.DispatchID})
		if err != nil {
			return err
		}
		receipt, err := orcaworker.ReplyQuestion(ctx, orcaWorkerMetaCaller(fixedConfig), orcaQuestionID(run.Decision), body, execution.Target.OrchestrationRun, execution.OrcaNative.Coordinator.Terminal, turn.DispatchID, execution.OrcaNative.Coordinator.RuntimeID)
		if err != nil {
			var callErr *orcaworker.CallError
			if errors.As(err, &callErr) {
				_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
				return s.nativeOrcaUncertain(run, "Orca reply outcome needs reconciliation: "+err.Error())
			}
			return s.nativeOrcaUncertain(run, "Orca reply receipt lacks its original request identity")
		}
		if err := s.changeOperation(opID, "confirmed", receipt.RequestID, receipt.Payload, ""); err != nil {
			return err
		}
		s.setDecisionStatus(run, "reply_accepted")
		if err := ContinueWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
			return err
		}
		s.refreshRun(run)
		return s.save()
	}
	if turn.DoneMessageID == "" && execution.CurrentTurnID == turn.TurnID && execution.LastStoppedTurnID != turn.TurnID {
		return s.sendNativeOrcaFeedback(ctx, run, workflow, execution, turn, answer, commandID)
	}
	if turn.DoneMessageID == "" || execution.LastStoppedTurnID != turn.TurnID {
		execution.Error = ptr("Feedback retained until the original Orca Dispatch settles")
		s.refreshRun(run)
		return s.save()
	}
	if op := s.nativeOrcaOperation("orca-native-send:" + execution.ExecutionID + ":" + commandID); op != nil && op.Status != "rejected" {
		execution.Error = ptr("Feedback may have been accepted by the settled Orca Dispatch; it was not copied into a new Task")
		s.refreshRun(run)
		return s.save()
	}
	for _, op := range s.State.Operations {
		if op.IdempotencyKey == "worker-turn:"+execution.ExecutionID+":"+commandID {
			if op.Status == "accepted" || op.Status == "confirmed" {
				return nil
			}
			execution.Error = ptr("Feedback delivery remains unverified; no duplicate Orca Task was created")
			s.refreshRun(run)
			return s.save()
		}
	}
	return s.startWorkerFlowTurn(ctx, run, workerFeedbackText(answer, run.Decision.Request), commandID)
}

func (s *Service) sendNativeOrcaFeedback(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turn *OrcaNativeTurn, answer DecisionAnswer, commandID string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	key := "orca-native-send:" + execution.ExecutionID + ":" + commandID
	name := "SteerLine feedback " + commandID
	body := workerFeedbackText(answer, run.Decision.Request)
	for _, op := range s.State.Operations {
		if op.IdempotencyKey != key {
			continue
		}
		if op.Status == "confirmed" || op.Status == "accepted" {
			s.setDecisionStatus(run, "reply_accepted")
			_ = ContinueWorkerQuestion(workflow, execution, turn.TurnID)
			s.refreshRun(run)
			return s.save()
		}
		messageID, err := findNativeOrcaSend(ctx, fixedConfig, execution.Target.OrchestrationRun, turn.DispatchID, name, body)
		if err != nil || messageID == "" {
			execution.Error = ptr("Orca feedback send remains unverified; it was not copied to another Dispatch")
			s.refreshRun(run)
			return s.save()
		}
		if err := s.changeOperation(op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"message_id": messageID}), ""); err != nil {
			return err
		}
		s.setDecisionStatus(run, "reply_accepted")
		if err := ContinueWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
			return err
		}
		s.refreshRun(run)
		return s.save()
	}
	opID, err := s.startOperation("orca_native_send", key, map[string]string{"dispatch": turn.DispatchID, "subject": name, "body": body})
	if err != nil {
		return err
	}
	receipt, err := orcaworker.SendFeedback(ctx, orcaWorkerMetaCaller(fixedConfig), name, body, turn.DispatchID, execution.Target.OrchestrationRun, execution.OrcaNative.Coordinator.Terminal, execution.OrcaNative.Coordinator.RuntimeID)
	if err != nil {
		var callErr *orcaworker.CallError
		if errors.As(err, &callErr) {
			if callErr.Result.ErrorCode == "dispatch_inactive" {
				_ = s.changeOperation(opID, "rejected", "", nil, err.Error())
				execution.Error = ptr("Original Orca Dispatch is inactive; feedback remains saved until settlement is confirmed")
				s.refreshRun(run)
				return s.save()
			}
			_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
			return s.nativeOrcaUncertain(run, "Orca send receipt is unknown: "+err.Error())
		}
		return s.nativeOrcaUncertain(run, "Orca accepted feedback without an exact Dispatch receipt")
	}
	if err := s.changeOperation(opID, "confirmed", receipt.RequestID, receipt.Payload, ""); err != nil {
		return err
	}
	s.setDecisionStatus(run, "reply_accepted")
	if err := ContinueWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
		return err
	}
	execution.LastEvent = ptr("Orca accepted feedback for the current Dispatch; agent consumption is unverified")
	s.refreshRun(run)
	return s.save()
}

func nativeOrcaSeen(state *OrcaNativeExecution, id string) bool {
	for _, seen := range state.SeenMessages {
		if seen == id {
			return true
		}
	}
	return false
}

func (s *Service) nativeOrcaUncertain(run *Run, reason string) error {
	if run == nil {
		return errors.New(reason)
	}
	execution := s.executionRecord(run)
	diagnostic := clip(reason, 2000)
	if execution.Status == "blocked" && execution.Error != nil && *execution.Error == diagnostic {
		return nil
	}
	execution.Error = ptr(diagnostic)
	s.refreshRun(run)
	execution.Status = "blocked"
	s.refreshRun(run)
	s.note(run, "steer", "worker", "unverified", "Orca observation needs reconciliation", reason, nil)
	return s.save()
}

func (s *Service) pollNativeOrcaWorker(ctx context.Context, run *Run) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || execution.OrcaNative == nil {
		return errors.New("native Orca poll has no fixed execution")
	}
	turn := nativeOrcaCurrentTurn(execution)
	if turn == nil || turn.DispatchID == "" {
		return s.nativeOrcaUncertain(run, "Orca Task has no confirmed Dispatch; original request must be reconciled")
	}
	if execution.LastStoppedTurnID == turn.TurnID && execution.OrcaNative.DeliveryID == "" {
		return nil
	}
	state := execution.OrcaNative
	if state.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		return errors.New("native Orca inbox has no bound coordinator/Run")
	}
	if turn.DoneMessageID == "" {
		delivery, err := readNativeOrcaDelivery(ctx, fixedConfig, execution)
		if err != nil {
			if ctx.Err() != nil {
				return nil // Service shutdown is not a worker failure.
			}
			return s.nativeOrcaUncertain(run, "Orca inbox is temporarily unavailable: "+err.Error())
		}
		if len(delivery.Messages) > 0 {
			for _, message := range delivery.Messages {
				if message.ID == "" || nativeOrcaSeen(state, message.ID) {
					continue
				}
				fact, matched, err := decodeNativeOrcaMessage(message, execution.Target.OrchestrationRun, *turn, execution.Process.Terminal)
				if err != nil {
					return err
				}
				if !matched {
					state.SeenMessages = append(state.SeenMessages, message.ID)
					continue
				}
				switch fact.Kind {
				case "question":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					if run.Decision == nil || orcaQuestionID(run.Decision) != fact.MessageID {
						decision, err := WriteDecisionRequest(run, *fact.Question)
						if err != nil {
							return err
						}
						s.setRunDecision(run, decision)
					}
					if err := NoteWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
						return err
					}
					s.refreshRun(run)
				case "worker_done":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					turn.DoneMessageID, turn.Outcome = fact.MessageID, fact.Outcome
					if err := s.bindNativeOrcaPRCandidates(ctx, run, workflow, execution, fact.PRCandidates); err != nil {
						execution.Error = ptr(clip(err.Error(), 2000))
						s.refreshRun(run)
					}
				case "status":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					if err := s.bindNativeOrcaPRCandidates(ctx, run, workflow, execution, fact.PRCandidates); err != nil {
						execution.Error = ptr(clip(err.Error(), 2000))
						s.refreshRun(run)
					}
				case "escalation":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					if run.Decision == nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved" {
						decision, err := WriteDecisionRequest(run, *fact.Question)
						if err != nil {
							return err
						}
						s.setRunDecision(run, decision)
					}
					if err := NoteWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
						return err
					}
					s.refreshRun(run)
				}
				state.SeenMessages = append(state.SeenMessages, message.ID)
			}
			state.DeliveryID = delivery.DeliveryID
			if err := s.save(); err != nil {
				return err
			} // Entire Delivery precedes acknowledgement.
		}
	}
	if state.DeliveryID != "" {
		if err := s.ackNativeOrcaDelivery(ctx, run, execution, state.DeliveryID); err != nil {
			return s.nativeOrcaUncertain(run, "Orca Delivery acknowledgement needs reconciliation: "+err.Error())
		}
		state.DeliveryID = ""
		if err := s.save(); err != nil {
			return err
		}
	}
	if turn.DoneMessageID == "" {
		if err := s.relayNativeOrcaDone(ctx, run, execution, turn); err != nil {
			if ctx.Err() != nil {
				return nil // Service shutdown is not a worker result failure.
			}
			return s.nativeOrcaUncertain(run, "Orca worker result relay needs reconciliation: "+err.Error())
		}
	}
	if err := s.readNativeOrcaOutput(ctx, run, execution, turn); err != nil {
		execution.Error = ptr(clip("Orca output is bounded or unavailable: "+err.Error(), 2000))
		s.refreshRun(run)
		_ = s.save()
	}
	if turn.DoneMessageID != "" {
		return s.finishNativeOrcaTurn(ctx, run, workflow, execution, turn)
	}
	if execution.Status == "blocked" && execution.Error != nil && strings.Contains(*execution.Error, "context canceled") {
		if err := verifyNativeOrcaActiveDispatch(ctx, fixedConfig, execution, turn); err != nil {
			return err
		}
		execution.Status, execution.Error = "pending", nil
		s.refreshRun(run)
		return s.save()
	}
	if workflow.Phase == "executing" && run.ExecutionStatus == "unknown" {
		execution.Status = "active"
		execution.Error = nil
		s.refreshRun(run)
		return s.save()
	}
	return nil
}

func (s *Service) bindNativeOrcaPR(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, body string) error {
	return s.bindNativeOrcaPRCandidates(ctx, run, workflow, execution, workerPRClues(body))
}

func (s *Service) bindNativeOrcaPRCandidates(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, urls []string) error {
	if len(urls) == 0 {
		return nil
	}
	if len(urls) != 1 {
		return errors.New("worker reported multiple PR links")
	}
	workflow.PRCandidateURL = urls[0]
	info, err := s.prs.Query(ctx, urls[0])
	if err != nil {
		return err
	}
	pendingQuestion := run.Decision != nil && run.Decision.Status == "pending" && orcaQuestionID(run.Decision) != ""
	if err := BindWorkerPR(workflow, execution, info, urls[0], s.projectExecutionConfig().PR.Repository, pendingQuestion); err != nil {
		return err
	}
	workflow.PRURL, workflow.PRState = info.URL, info.State
	s.refreshRun(run)
	return nil
}

func (s *Service) ackNativeOrcaDelivery(ctx context.Context, run *Run, execution *ExecutionRecord, deliveryID string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	state := execution.OrcaNative
	key := "orca-native-ack:" + execution.ExecutionID + ":" + deliveryID
	op := s.nativeOrcaOperation(key)
	if op != nil && op.Status == "confirmed" {
		return nil
	}
	if op != nil {
		// The ack response may have been lost after Orca advanced its FIFO.
		// Read the original Run before deciding whether another ack is needed.
		current, err := orcaworker.CheckInbox(ctx, orcaWorkerMetaCaller(fixedConfig), state.Coordinator.Terminal, execution.Target.OrchestrationRun)
		if err != nil {
			return err
		}
		if current.RuntimeID != state.Coordinator.RuntimeID || current.RunID != execution.Target.OrchestrationRun {
			return errors.New("Orca Delivery recovery changed coordinator or Run")
		}
		if current.DeliveryID != deliveryID {
			return s.changeOperation(op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"delivery_id": deliveryID, "proof": "absent_from_original_run_fifo"}), "")
		}
	}
	opID := ""
	if op == nil {
		var err error
		opID, err = s.startOperation("orca_native_ack", key, map[string]string{"delivery": deliveryID, "run": execution.Target.OrchestrationRun, "coordinator": state.Coordinator.Terminal})
		if err != nil {
			return err
		}
	} else {
		opID = op.OperationID
	}
	receipt, err := orcaworker.AckInbox(ctx, orcaWorkerMetaCaller(fixedConfig), state.Coordinator.Terminal, execution.Target.OrchestrationRun, deliveryID, state.Coordinator.RuntimeID)
	if err != nil {
		var callErr *orcaworker.CallError
		if errors.As(err, &callErr) {
			_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		}
		return err
	}
	return s.changeOperation(opID, "confirmed", receipt.RequestID, receipt.Payload, "")
}

func (s *Service) readNativeOrcaOutput(ctx context.Context, run *Run, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	attempt := s.workflowAttempt(run, run.AttemptID)
	if attempt == nil {
		return errors.New("worker output has no attempt")
	}
	result, resetCursor, err := orcaworker.ReadWorkerOutput(ctx, orcaWorkerMetaCaller(fixedConfig), turn.DispatchID, attempt.OutputCursor, execution.OrcaNative.Coordinator.RuntimeID)
	if resetCursor {
		attempt.OutputCursor = ""
	}
	if err != nil {
		return err
	}
	attempt.OutputSource = result.Source
	attempt.OutputCursor = result.NextCursor
	if attempt.OutputCursor == "" {
		attempt.OutputCursor = result.Cursor
	}
	turn.OutputSource, turn.OutputCursor, turn.OutputComplete = result.Source, attempt.OutputCursor, result.ContentComplete
	turn.OutputClipping, turn.OutputFallbackReason = append([]string(nil), result.Clipping...), result.FallbackReason
	content := strings.Join(result.Terminal.Tail, "\n")
	if result.Source == "transcript" && len(result.Transcript) > 0 {
		content = string(result.Transcript)
	}
	if content != "" {
		digest := digestValue(struct{ Source, Content string }{result.Source, content})
		if digest != turn.OutputDigest {
			turn.OutputDigest = digest
			status := "bounded"
			if result.ContentComplete && len(result.Clipping) == 0 {
				status = "complete"
			}
			s.note(run, "worker", "output", status, "Orca worker output", clip(content, 7000), map[string]any{"source": result.Source, "fallback_reason": result.FallbackReason, "clipping": result.Clipping, "dispatch_id": turn.DispatchID})
		}
	}
	if result.SourceChanged || len(result.Clipping) > 0 || !result.ContentComplete {
		execution.LastEvent = ptr("Orca output is bounded (" + result.Source + "; " + result.FallbackReason + ")")
		s.refreshRun(run)
	}
	return s.save()
}

func (s *Service) finishNativeOrcaTurn(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	settlement, err := readNativeOrcaSettlement(ctx, fixed.config, execution, turn)
	if err != nil {
		return s.nativeOrcaUncertain(run, "Orca worker_done arrived but Dispatch settlement is unverified: "+err.Error())
	}
	if run.Decision != nil && orcaQuestionID(run.Decision) != "" && run.Decision.Status == "answer_saved" {
		return s.nativeOrcaUncertain(run, "Orca Dispatch settled while the original question reply remains unverified")
	}
	pendingQuestion := run.Decision != nil && run.Decision.Status == "pending" && orcaQuestionID(run.Decision) != ""
	if err := s.stopVerifiedWorkerTurn(run, workflow, execution, turn.TurnID, settlement.Outcome, pendingQuestion); err != nil {
		return err
	}
	if pendingQuestion {
		s.setDecisionStatus(run, "superseded")
		if err := s.save(); err != nil {
			return err
		}
		s.setRunDecision(run, nil)
	}
	if err := s.resolveStoppedWorkerTurn(run, workflow, execution, stoppedWorkerResolution{TurnID: turn.TurnID, WaitReason: "The original Orca Dispatch settled (" + settlement.Outcome + "). Send feedback to continue the same agent session."}); err != nil {
		return err
	}
	removeNativeOrcaRelayRequest(execution.Workspace.Worktree, run.RunID, turn.TurnID)
	return nil
}

func (s *Service) closeNativeOrcaTerminal(ctx context.Context, execution *ExecutionRecord, identity ProcessIdentity, role string) error {
	if execution == nil {
		return errors.New("Orca terminal has no fixed execution")
	}
	config, configErr := s.executionConfigForWorkflowID(execution.WorkflowRunID)
	if configErr != nil {
		return configErr
	}
	fixedConfig := copyWorkerFlowConfig(config)
	if identity.Terminal == "" || identity.Incarnation == "" || identity.RuntimeID == "" || identity.RuntimeID != execution.Process.RuntimeID {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca terminal cleanup lacks its original identity"}
	}
	key := "orca-native-close:" + execution.ExecutionID + ":" + role
	if op := s.nativeOrcaOperation(key); op != nil && op.Status == "confirmed" {
		return nil
	}
	term, err := showWorkerOrcaTerminal(ctx, fixedConfig, identity.Terminal)
	if err != nil {
		if op := s.nativeOrcaOperation(key); op != nil {
			selector := "path:" + filepath.ToSlash(fixedConfig.Repository)
			if role == "agent" {
				selector = "id:" + identity.WorktreeID
			}
			terms, runtimeID, listErr := nativeOrcaTerminals(ctx, fixedConfig, selector)
			if listErr == nil && runtimeID == identity.RuntimeID {
				for _, candidate := range terms {
					if candidate.Handle == identity.Terminal {
						return err
					}
				}
				return s.changeOperation(op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"closed_handle": identity.Terminal, "proof": "absent_on_original_runtime"}), "")
			}
		}
		return err
	}
	if term.RuntimeID != identity.RuntimeID || term.IncarnationID != identity.Incarnation || role == "agent" && term.WorktreeID != identity.WorktreeID || role == "coordinator" && canonical(filepath.FromSlash(term.WorktreePath)) != canonical(fixedConfig.Repository) {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca cleanup terminal was replaced or moved"}
	}
	op := s.nativeOrcaOperation(key)
	opID := ""
	if op == nil {
		opID, err = s.startOperation("orca_native_terminal_close", key, map[string]string{"role": role, "handle": identity.Terminal, "incarnation": identity.Incarnation, "runtime": identity.RuntimeID})
		if err != nil {
			return err
		}
	} else {
		opID = op.OperationID
	}
	receipt, err := orcaworker.CloseTerminal(ctx, orcaWorkerMetaCaller(fixedConfig), identity.Terminal, identity.RuntimeID)
	if err != nil {
		var callErr *orcaworker.CallError
		if errors.As(err, &callErr) {
			_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		}
		return workerOrcaBackendError(err)
	}
	return s.changeOperation(opID, "confirmed", "", receipt, "")
}

func (s *Service) finishNativeOrcaCleanup(ctx context.Context, run *Run) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || execution.OrcaNative == nil || execution.Process == nil || execution.Process.Kind != "orca-native" || workflow.Phase != "completed" {
		return nil
	}
	state := execution.OrcaNative
	if execution.PendingTurnID != "" || execution.CurrentTurnID == "" || execution.CurrentTurnID != execution.LastStoppedTurnID || state.DeliveryID != "" {
		return nil // Delivery succeeded, but the external worker or inbox is not yet safe to close.
	}
	if err := s.closeNativeOrcaTerminal(ctx, execution, *execution.Process, "agent"); err != nil {
		return err
	}
	if state.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		return nil
	}
	delivery, err := orcaworker.CheckInbox(ctx, orcaWorkerMetaCaller(fixedConfig), state.Coordinator.Terminal, execution.Target.OrchestrationRun)
	if err != nil {
		return err
	}
	if delivery.RuntimeID != state.Coordinator.RuntimeID || delivery.RunID != execution.Target.OrchestrationRun || delivery.DeliveryID != "" || len(delivery.Messages) != 0 {
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca coordinator inbox is not drained after delivery"}
	}
	return s.closeNativeOrcaTerminal(ctx, execution, state.Coordinator, "coordinator")
}
