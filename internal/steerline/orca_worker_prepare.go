package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func nativeOrcaFixedAgentTitle(executionID string) string {
	return orcaworker.FixedAgentTitle(executionID)
}

func nativeOrcaFixedAgentCommand(c Config) (string, error) {
	return nativeOrcaCommand(c, "", nil)
}

func nativeOrcaCommand(c Config, path string, roots []string) (string, error) {
	if path != "" {
		if err := codexworker.ValidateManagedArgs(c.Codex.Args); err != nil {
			return "", err
		}
		native, err := codexworker.NativeExecutable(c.Codex.Executable)
		if err != nil {
			return "", err
		}
		c.Codex.Executable = native
	}
	command, err := codexworker.ManagedCommand(codexworker.ManagedCommandOptions{
		Policy: c.ExecutionPolicy, WorkingDirectory: path, WriteRoots: roots, Host: c.hostName(), Agent: c.agentName(), Model: c.Model, ReasoningEffort: c.ReasoningEffort,
		Executable: c.Codex.Executable, Args: c.Codex.Args, Sandbox: c.Sandbox, WindowsSandbox: c.WindowsSandbox,
	})
	if err != nil {
		return "", err
	}
	return orcaworker.WrapAgentCommand(command, c.Orca.Shell)
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
			return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "fixed Orca worktree creation has no unique resource"}
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
			return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "fixed Orca worktree creation needs reconciliation", Cause: callErr}
		}
		parsedWorktree, err := orcaworker.ParseFixedWorktreeReceipt(platform, name, repoID, base)
		if err != nil {
			_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", platform.Payload, "fixed worktree receipt is incomplete")
			return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "fixed Orca worktree receipt is incomplete", Cause: err}
		}
		worktree = parsedWorktree
		if err := s.changeWorkerOperation(run, opID, "confirmed", "", platform.Payload, ""); err != nil {
			return err
		}
	}
	if err := orcaworker.VerifyWorktreeOwner(worktree, execution.ExecutionID, fixedConfig.agentName(), true); err != nil {
		return err
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
	var savedReceipt []byte
	if op != nil {
		savedReceipt = op.Receipt
	}
	terminal, err := orcaworker.FixedAgentTerminal(terminals, savedReceipt, title, worktree.ID)
	if err != nil {
		return err
	}
	if op != nil {
		if terminal.Handle == "" {
			return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch has no identifiable terminal"}
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
		return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "fixed Orca agent terminal exists without its launch intent"}
	}
	writeArgs, err := codexLinkedWorktreeWriteArgs(ctx, fixedConfig, worktree.Path)
	if err != nil {
		return err
	}
	roots := []string{}
	for i := 1; i < len(writeArgs); i += 2 {
		roots = append(roots, writeArgs[i])
	}
	execution.Workspace = WorkspaceEvidence{Repository: fixedConfig.identity, Worktree: worktree.Path, Branch: strings.TrimPrefix(worktree.Branch, "refs/heads/"), Base: base, Head: base, VerifiedAt: now()}
	if err := bindPolicyResources(ctx, fixedConfig, execution, worktree.Path, writeArgs); err != nil {
		return err
	}
	execution.PolicyResources.ProfileDigest = fixed.workflow.ExecutionProfile.Digest
	execution.PolicyResources.Digest = policyResourceDigest(*execution.PolicyResources)
	command, err := nativeOrcaCommand(fixedConfig, worktree.Path, roots)
	if err != nil {
		return err
	}
	if env := execution.Environment; env != nil {
		if env.LaunchCommand != "" && env.LaunchCommand != command {
			return errors.New("managed Orca launch evidence differs from original command")
		}
		env.LaunchCommand = command
		env.WriteRoots = []string{worktree.Path}
		for i := 1; i < len(writeArgs); i += 2 {
			env.WriteRoots = append(env.WriteRoots, writeArgs[i])
		}
	}
	opID, err := s.startWorkerOperation(run, "orca_native_agent_terminal", key, map[string]string{"worktree": worktree.ID, "title": title, "model": fixedConfig.Model, "effort": fixedConfig.ReasoningEffort, "command": command})
	if err != nil {
		return err
	}
	platform, callErr := orcaworker.CreateFixedAgentTerminal(ctx, orcaWorkerMetaCaller(fixedConfig), worktree.ID, title, fixedConfig.Orca.Shell, command)
	if callErr != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, callErr.Error())
		return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch needs reconciliation", Cause: callErr}
	}
	parsedTerminal, err := orcaworker.ParseFixedAgentTerminalReceipt(platform, worktree.ID, title)
	if err != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", platform.Payload, "fixed agent terminal creation receipt is incomplete")
		return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch receipt is incomplete", Cause: err}
	}
	if env := execution.Environment; env != nil {
		env.Runtime = platform.RuntimeID
		env.OriginalTerminal = &executionapi.ProcessIdentity{Kind: "orca-native", Terminal: parsedTerminal.Handle, RuntimeID: platform.RuntimeID, Incarnation: parsedTerminal.IncarnationID, WorktreeID: worktree.ID}
	}
	if err := s.changeWorkerOperation(run, opID, "accepted", "", platform.Payload, ""); err != nil {
		return err
	}
	if err := s.waitAndBindNativeOrcaFixedAgent(ctx, run, execution, worktree, parsedTerminal.Handle, repoID, base); err != nil {
		if errors.Is(err, orcaworker.ErrAgentStarting) {
			return err // Keep the accepted creation receipt for the next actor pass.
		}
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", platform.Payload, err.Error())
		return err
	}
	if err := s.changeWorkerOperation(run, opID, "confirmed", "", platform.Payload, ""); err != nil {
		return err
	}
	return s.prepareNativeOrcaCoordinator(ctx, run, execution)
}

func (s *Service) waitAndBindNativeOrcaFixedAgent(ctx context.Context, run *Run, execution *ExecutionRecord, worktree workerOrcaWorktree, handle, repoID, base string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	if execution.Environment == nil {
		if err := orcaworker.AwaitAgentTerminal(ctx, orcaWorkerMetaCaller(fixedConfig), handle, worktree.Path, strings.TrimPrefix(worktree.Branch, "refs/heads/"), worktree.ID, fixedConfig.agentName(), min(positiveDuration(fixedConfig.Orca.WaitTimeout, 30*time.Second), 30*time.Second), true); err != nil {
			return err
		}
	}
	return s.bindNativeOrcaAgent(ctx, run, execution, worktree, handle, repoID, base)
}

func nativeOrcaRepo(ctx context.Context, c Config) (string, error) {
	repoID, err := orcaworker.FindRepo(ctx, orcaWorkerCaller(c), c.Repository)
	if errors.Is(err, orcaworker.ErrRepoUnavailable) {
		return "", &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: err.Error()}
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

func findNativeOrcaRun(ctx context.Context, c Config, objective, coordinator string) (string, error) {
	return orcaworker.FindRun(ctx, orcaWorkerCaller(c), objective, coordinator)
}

func (s *Service) bindNativeOrcaAgent(ctx context.Context, run *Run, execution *ExecutionRecord, worktree workerOrcaWorktree, handle, repoID, base string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	if err := orcaworker.VerifyWorktreeOwner(worktree, execution.ExecutionID, fixedConfig.agentName(), fixedConfig.managedWindowsOrca() || fixedConfig.Model != ""); err != nil {
		return err
	}
	if err := applyWorkerOrcaWorkspace(fixedConfig, run, execution, worktree, base); err != nil {
		return err
	}
	identity, err := orcaworker.BindAgent(ctx, orcaWorkerMetaCaller(fixedConfig), worktree, handle, fixedConfig.agentName(), fixedConfig.Orca.Executable)
	if err != nil {
		return workerOrcaBackendError(err)
	}
	execution.Process = &identity
	if execution.Environment != nil {
		execution.Environment.Runtime = identity.RuntimeID
	}
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
		if fixedConfig.managedWindowsOrca() || fixedConfig.Model != "" {
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
	if fixedConfig.managedWindowsOrca() || fixedConfig.Model != "" {
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
			return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "original Orca worktree creation has no unique resource; do not create another agent"}
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
		return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "Orca worktree/agent creation has no confirmed receipt; do not create another agent", Cause: callErr}
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
				var err error
				handle, err = orcaworker.FindOriginalCoordinator(ctx, orcaWorkerMetaCaller(fixedConfig), fixedConfig.Repository, execution.Process.RuntimeID, marker)
				if err != nil {
					return err
				}
			}
			if handle == "" {
				return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "original Orca coordinator creation is unverified; do not create another"}
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
				return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "original Orca Run creation has no matching unique Run"}
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
	identity, err := orcaworker.BindCoordinator(term, fixedConfig.Repository, execution.Process.RuntimeID, fixedConfig.Orca.Executable)
	if err != nil {
		return err
	}
	execution.OrcaNative.Coordinator = identity
	execution.Target.CoordinatorTerminal = term.Handle
	s.refreshRun(run)
	return s.commitWorker(run)
}

func (s *Service) verifyNativeOrcaWorker(ctx context.Context, run *Run, execution *ExecutionRecord) error {
	entry, err := s.workerFlowRegistration(run)
	if err != nil {
		return err
	}
	if entry.VerifyWorker == nil {
		return unsupported("worker identity verification")
	}
	return entry.VerifyWorker(s, ctx, run, execution)
}
func (s *Service) verifyLocalNativeOrcaWorker(ctx context.Context, run *Run, execution *ExecutionRecord) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	if execution.OrcaNative == nil || execution.Process == nil || execution.Process.Kind != "orca-native" {
		return errors.New("native Orca worker binding is incomplete")
	}
	if err := orcaworker.VerifyAgentTerminal(ctx, orcaWorkerMetaCaller(fixedConfig), *execution.Process, execution.Workspace.Worktree, execution.Workspace.Branch, fixedConfig.agentName()); err != nil {
		return workerOrcaBackendError(err)
	}
	if env := execution.Environment; env != nil {
		if env.Runtime != execution.Process.RuntimeID || len(env.WriteRoots) == 0 || canonical(env.WriteRoots[0]) != canonical(execution.Workspace.Worktree) {
			return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "managed Orca resource permission identity changed"}
		}
		args, err := codexLinkedWorktreeWriteArgs(ctx, fixedConfig, env.WriteRoots[0])
		if err != nil {
			return err
		}
		if execution.PolicyResources == nil {
			return errors.New("original permission resource snapshot is missing")
		}
		if err := bindPolicyResources(ctx, fixedConfig, execution, env.WriteRoots[0], args); err != nil {
			return err
		}
		roots := []string{env.WriteRoots[0]}
		for i := 1; i < len(args); i += 2 {
			roots = append(roots, args[i])
		}
		command, err := nativeOrcaCommand(fixedConfig, env.WriteRoots[0], roots[1:])
		if err != nil {
			return err
		}
		if digestValue(roots) != digestValue(env.WriteRoots) || command != env.LaunchCommand {
			return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "managed Orca launch command or write roots changed"}
		}
		op := s.operationByKey("orca-native-agent-terminal:" + execution.ExecutionID)
		var request map[string]string
		if op == nil || json.Unmarshal(op.Request, &request) != nil || request["command"] != command || request["worktree"] != execution.Process.WorktreeID {
			return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "managed Orca launch differs from its original operation"}
		}
	}
	coordinator := execution.OrcaNative.Coordinator
	if coordinator.Terminal != "" {
		if err := orcaworker.VerifyCoordinatorTerminal(ctx, orcaWorkerMetaCaller(fixedConfig), coordinator, fixedConfig.Repository); err != nil {
			return workerOrcaBackendError(err)
		}
	}
	return nil
}
