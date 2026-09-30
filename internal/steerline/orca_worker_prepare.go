package steerline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func nativeOrcaFixedAgentTitle(executionID string) string {
	return orcaworker.FixedAgentTitle(executionID)
}

func nativeOrcaFixedAgentCommand(c Config) (string, error) {
	return orcaworker.FixedAgentCommand(orcaworker.FixedAgentOptions{
		Host: c.hostName(), Agent: c.agentName(), Model: c.Model, ReasoningEffort: c.ReasoningEffort,
		Executable: c.Codex.Executable, Args: c.Codex.Args, Sandbox: c.Sandbox, Shell: c.Orca.Shell,
	})
}

// A fixed Orca model needs a launch command: worktree create --agent only
// accepts an agent ID and cannot pass model or effort. Keep worktree and agent
// terminal as separate, journaled operations for restart reconciliation.
func (s *Service) prepareNativeOrcaFixedModelWorker(ctx context.Context, run *Run, execution *ExecutionRecord) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	repoID, err := nativeOrcaRepo(ctx, fixedConfig)
	if err != nil {
		return err
	}
	name, err := workerOrcaWorktreeName(run.TaskID, run.RunID)
	if err != nil {
		return err
	}
	base := run.WorkspaceIntent.BaseCommit
	worktree, exists, err := findWorkerOrcaWorktree(ctx, fixedConfig, repoID, name, base)
	if err != nil {
		return err
	}
	key := "orca-native-worktree:" + execution.ExecutionID
	if op := s.operationByKey(key); op != nil {
		if !exists {
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca worktree creation has no unique resource"}
		}
		if op.Status != "confirmed" {
			if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(worktree), ""); err != nil {
				return err
			}
		}
	} else {
		if exists {
			return errors.New("fixed Orca worktree exists without its creation intent")
		}
		opID, err := s.startWorkerOperation(run, "orca_native_worktree", key, map[string]string{"repo_id": repoID, "name": name, "base": base, "comment": nativeOrcaWorktreeComment(execution.ExecutionID)})
		if err != nil {
			return err
		}
		platform, callErr := orcaworker.CreateFixedWorktree(ctx, orcaWorkerMetaCaller(fixedConfig), repoID, name, base, nativeOrcaWorktreeComment(execution.ExecutionID))
		if callErr != nil {
			_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, callErr.Error())
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca worktree creation needs reconciliation", Cause: callErr}
		}
		parsedWorktree, err := orcaworker.ParseFixedWorktreeReceipt(platform, name, repoID, base)
		if err != nil {
			_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", platform.Payload, "fixed worktree receipt is incomplete")
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca worktree receipt is incomplete", Cause: err}
		}
		worktree = parsedWorktree
		if err := s.changeWorkerOperation(run, opID, "confirmed", "", platform.Payload, ""); err != nil {
			return err
		}
	}
	if worktree.Comment != nativeOrcaWorktreeComment(execution.ExecutionID) || worktree.CreatedWithAgent != "" {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "fixed Orca worktree belongs to another execution or agent"}
	}
	return s.prepareNativeOrcaFixedAgentTerminal(ctx, run, execution, worktree, repoID, base)
}

func (s *Service) prepareNativeOrcaFixedAgentTerminal(ctx context.Context, run *Run, execution *ExecutionRecord, worktree workerOrcaWorktree, repoID, base string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	title := nativeOrcaFixedAgentTitle(execution.ExecutionID)
	terminals, _, err := nativeOrcaTerminals(ctx, fixedConfig, "id:"+worktree.ID)
	if err != nil {
		return err
	}
	key := "orca-native-agent-terminal:" + execution.ExecutionID
	op := s.operationByKey(key)
	terminal, err := fixedOrcaAgentTerminal(terminals, op, title, worktree.ID)
	if err != nil {
		return err
	}
	if op != nil {
		if terminal.Handle == "" {
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch has no identifiable terminal"}
		}
		if err := s.waitAndBindNativeOrcaFixedAgent(ctx, run, execution, worktree, terminal.Handle, repoID, base); err != nil {
			return err
		}
		if op.Status != "confirmed" {
			receipt := op.Receipt
			if len(receipt) == 0 {
				receipt = mustJSON(struct {
					Terminal orcaTerminal `json:"terminal"`
				}{Terminal: terminal})
			}
			if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, receipt, ""); err != nil {
				return err
			}
		}
		return s.prepareNativeOrcaCoordinator(ctx, run, execution)
	}
	if terminal.Handle != "" {
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent terminal exists without its launch intent"}
	}
	command, err := nativeOrcaFixedAgentCommand(fixedConfig)
	if err != nil {
		return err
	}
	writeArgs, err := codexLinkedWorktreeWriteArgs(ctx, fixedConfig, worktree.Path)
	if err != nil {
		return err
	}
	command = orcaworker.AppendPowerShellArgs(command, writeArgs)
	opID, err := s.startWorkerOperation(run, "orca_native_agent_terminal", key, map[string]string{"worktree": worktree.ID, "title": title, "model": fixedConfig.Model, "effort": fixedConfig.ReasoningEffort, "command": command})
	if err != nil {
		return err
	}
	platform, callErr := orcaworker.CreateFixedAgentTerminal(ctx, orcaWorkerMetaCaller(fixedConfig), worktree.ID, title, fixedConfig.Orca.Shell, command)
	if callErr != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, callErr.Error())
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch needs reconciliation", Cause: callErr}
	}
	parsedTerminal, err := orcaworker.ParseFixedAgentTerminalReceipt(platform, worktree.ID, title)
	if err != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", platform.Payload, "fixed agent terminal creation receipt is incomplete")
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch receipt is incomplete", Cause: err}
	}
	if err := s.changeWorkerOperation(run, opID, "accepted", "", platform.Payload, ""); err != nil {
		return err
	}
	if err := s.waitAndBindNativeOrcaFixedAgent(ctx, run, execution, worktree, parsedTerminal.Handle, repoID, base); err != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", platform.Payload, err.Error())
		return err
	}
	if err := s.changeWorkerOperation(run, opID, "confirmed", "", platform.Payload, ""); err != nil {
		return err
	}
	return s.prepareNativeOrcaCoordinator(ctx, run, execution)
}

// Orca may rename an agent terminal after it starts. The creation receipt is
// the immutable link between the launch intent and the live terminal.
func fixedOrcaAgentTerminal(terminals []orcaTerminal, op *OperationRecord, title, worktreeID string) (orcaTerminal, error) {
	var expected orcaTerminal
	if op != nil && len(op.Receipt) != 0 {
		terminal, err := orcaworker.SavedTerminal(op.Receipt)
		if err != nil {
			return orcaTerminal{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch receipt is invalid", Cause: err}
		}
		expected = terminal
		if expected.Handle == "" || expected.IncarnationID == "" || expected.WorktreeID != worktreeID || expected.Title != title {
			return orcaTerminal{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch receipt does not match the original intent"}
		}
	}
	var matched orcaTerminal
	for _, terminal := range terminals {
		if expected.Handle != "" && terminal.Handle != expected.Handle {
			continue
		}
		if expected.Handle == "" && terminal.Title != title {
			continue
		}
		if terminal.Handle == "" || matched.Handle != "" || terminal.WorktreeID != worktreeID {
			return orcaTerminal{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent terminal is not unique"}
		}
		if expected.Handle != "" && terminal.IncarnationID != expected.IncarnationID {
			return orcaTerminal{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "fixed Orca agent terminal incarnation changed"}
		}
		matched = terminal
	}
	return matched, nil
}

func (s *Service) waitAndBindNativeOrcaFixedAgent(ctx context.Context, run *Run, execution *ExecutionRecord, worktree workerOrcaWorktree, handle, repoID, base string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	deadline := time.Now().Add(min(positiveDuration(fixedConfig.Orca.WaitTimeout, 30*time.Second), 30*time.Second))
	for {
		err := s.bindNativeOrcaAgent(ctx, run, execution, worktree, handle, repoID, base)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("fixed Orca agent did not become identifiable: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
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
	if err := verifyWorkerOrcaTerminal(term, run); err != nil {
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
	return s.commitWorker(run)
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
			if op := s.operationByKey("orca-native-agent-terminal:" + execution.ExecutionID); op != nil && op.Status != "confirmed" {
				receipt := op.Receipt
				if len(receipt) == 0 {
					receipt = mustJSON(map[string]string{"terminal": execution.Process.Terminal})
				}
				if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, receipt, ""); err != nil {
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
	name, err := workerOrcaWorktreeName(run.TaskID, run.RunID)
	if err != nil {
		return err
	}
	base := run.WorkspaceIntent.BaseCommit
	worktree, exists, err := findWorkerOrcaWorktree(ctx, fixedConfig, repoID, name, base)
	if err != nil {
		return err
	}
	key := "orca-native-worktree:" + execution.ExecutionID
	if op := s.operationByKey(key); op != nil {
		if !exists {
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original Orca worktree creation has no unique resource; do not create another agent"}
		}
		handle, receiptErr := orcaworker.SavedAgentHandle(op.Receipt)
		if receiptErr != nil {
			return errors.New("saved Orca agent receipt cannot be decoded")
		}
		if err := s.bindNativeOrcaAgent(ctx, run, execution, worktree, handle, repoID, base); err != nil {
			return err
		}
		if op.Status != "confirmed" {
			if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(worktree), ""); err != nil {
				return err
			}
		}
		return s.prepareNativeOrcaCoordinator(ctx, run, execution)
	}
	if exists {
		return errors.New("fixed Orca worktree already exists without the original creation intent; preserve it for manual reconciliation")
	}
	opID, err := s.startWorkerOperation(run, "orca_native_worktree", key, map[string]string{"repo_id": repoID, "name": name, "base": base, "agent": fixedConfig.agentName(), "comment": nativeOrcaWorktreeComment(execution.ExecutionID)})
	if err != nil {
		return err
	}
	platform, callErr := orcaworker.CreateNativeWorktree(ctx, orcaWorkerMetaCaller(fixedConfig), repoID, name, base, fixedConfig.agentName(), nativeOrcaWorktreeComment(execution.ExecutionID))
	if callErr != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, callErr.Error())
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca worktree/agent creation has no confirmed receipt; do not create another agent", Cause: callErr}
	}
	receipt, err := orcaworker.ParseNativeWorktreeReceipt(platform, name, repoID, base)
	if err != nil {
		if errors.Is(err, orcaworker.ErrAgentHandleOmitted) {
			_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", platform.Payload, "agent handle omitted")
			return err
		}
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
		return workerOrcaWorkspaceError(err)
	}
	handle := receipt.Handle
	if err := s.changeWorkerOperation(run, opID, "accepted", "", platform.Payload, ""); err != nil {
		return err
	}
	if err := s.bindNativeOrcaAgent(ctx, run, execution, receipt.Worktree, handle, repoID, base); err != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", platform.Payload, err.Error())
		return err
	}
	if err := s.changeWorkerOperation(run, opID, "confirmed", "", platform.Payload, ""); err != nil {
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
		if op := s.operationByKey(key); op != nil {
			handle := ""
			if len(op.Receipt) > 0 {
				terminal, receiptErr := orcaworker.SavedTerminal(op.Receipt)
				if receiptErr != nil {
					return receiptErr
				}
				handle = terminal.Handle
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
				if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(state.Coordinator), ""); err != nil {
					return err
				}
			}
		} else {
			opID, err := s.startWorkerOperation(run, "orca_native_coordinator", key, map[string]string{"execution_id": execution.ExecutionID, "repository": fixedConfig.Repository, "title": title})
			if err != nil {
				return err
			}
			platform, err := orcaworker.CreateCoordinatorTerminal(ctx, orcaWorkerMetaCaller(fixedConfig), fixedConfig.Repository, title, marker)
			if err != nil {
				_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
				return err
			}
			handle, err := orcaworker.ParseCoordinatorTerminalReceipt(platform)
			if err != nil {
				_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", platform.Payload, "coordinator handle omitted")
				return err
			}
			if err := s.changeWorkerOperation(run, opID, "accepted", "", platform.Payload, ""); err != nil {
				return err
			}
			if err := s.bindNativeOrcaCoordinator(ctx, run, execution, handle); err != nil {
				return err
			}
			if err := s.changeWorkerOperation(run, opID, "confirmed", "", platform.Payload, ""); err != nil {
				return err
			}
		}
	}
	if execution.Target.OrchestrationRun == "" {
		objective := "SteerLine worker " + execution.ExecutionID
		key := "orca-native-run:" + execution.ExecutionID
		if op := s.operationByKey(key); op != nil {
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
				if err := s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"run_id": id}), ""); err != nil {
					return err
				}
			} else if err := s.commitWorker(run); err != nil {
				return err
			}
			return nil
		}
		opID, err := s.startWorkerOperation(run, "orca_native_run", key, map[string]string{"objective": objective, "coordinator": state.Coordinator.Terminal})
		if err != nil {
			return err
		}
		receipt, err := orcaworker.CreateRun(ctx, orcaWorkerMetaCaller(fixedConfig), objective, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
		if err != nil {
			var callErr *orcaworker.CallError
			if errors.As(err, &callErr) {
				_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
			}
			return err
		}
		execution.Target.OrchestrationRun = receipt.ID
		execution.Target.OrchestrationRunRequestID = receipt.RequestID
		execution.Target.OrchestrationRun = receipt.ID
		s.refreshRun(run)
		if err := s.changeWorkerOperation(run, opID, "confirmed", receipt.RequestID, receipt.Payload, ""); err != nil {
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
	return s.commitWorker(run)
}

func (s *Service) verifyNativeOrcaWorker(ctx context.Context, run *Run, execution *ExecutionRecord) error {
	if run.Protocol == string(ProtocolOrcaSupervised) {
		return s.verifyRemoteOrcaWorker(ctx, run, execution)
	}
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
	if err := verifyWorkerOrcaTerminal(term, run); err != nil {
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
