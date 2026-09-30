package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func nativeOrcaFixedAgentTitle(executionID string) string {
	return "SteerLine agent " + executionID
}

func nativeOrcaFixedAgentCommand(c Config) (string, error) {
	if c.hostName() != "orca" || c.agentName() != "codex" || c.Model == "" || c.ReasoningEffort == "" || c.Codex.Executable == "" || c.Sandbox != "workspace-write" {
		return "", errors.New("fixed Orca agent launch requires Codex, model, effort, executable and workspace-write")
	}
	if c.Orca.Shell != "powershell.exe" && c.Orca.Shell != "pwsh.exe" {
		return "", errors.New("fixed Orca agent launch requires a PowerShell terminal")
	}
	args := append([]string(nil), c.Codex.Args...)
	args = append(args, "--model", c.Model, "--config", "model_reasoning_effort="+c.ReasoningEffort, "--sandbox", c.Sandbox)
	if runtime.GOOS == "windows" {
		// The worker must reach its Git/PR remote while the Codex workspace
		// sandbox still limits filesystem writes.
		args = append(args, "--config", "sandbox_workspace_write.network_access=true")
	}
	parts := []string{"&", powershellLiteral(c.Codex.Executable)}
	for _, arg := range args {
		parts = append(parts, powershellLiteral(arg))
	}
	return strings.Join(parts, " "), nil
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
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca worktree creation has no unique resource"}
		}
		if op.Status != "confirmed" {
			if err := s.changeOperation(op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(worktree), ""); err != nil {
				return err
			}
		}
	} else {
		if exists {
			return errors.New("fixed Orca worktree exists without its creation intent")
		}
		opID, err := s.startOperation("orca_native_worktree", key, map[string]string{"repo_id": repoID, "name": name, "base": base, "comment": nativeOrcaWorktreeComment(execution.ExecutionID)})
		if err != nil {
			return err
		}
		platform, callErr := orcaworker.CreateFixedWorktree(ctx, orcaWorkerMetaCaller(fixedConfig), repoID, name, base, nativeOrcaWorktreeComment(execution.ExecutionID))
		if callErr != nil {
			_ = s.changeOperation(opID, "outcome_unknown", "", nil, callErr.Error())
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca worktree creation needs reconciliation", Cause: callErr}
		}
		parsedWorktree, err := orcaworker.ParseFixedWorktreeReceipt(platform, name, repoID, base)
		if err != nil {
			_ = s.changeOperation(opID, "outcome_unknown", "", platform.Payload, "fixed worktree receipt is incomplete")
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca worktree receipt is incomplete", Cause: err}
		}
		worktree = parsedWorktree
		if err := s.changeOperation(opID, "confirmed", "", platform.Payload, ""); err != nil {
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
	op := s.nativeOrcaOperation(key)
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
			if err := s.changeOperation(op.OperationID, "confirmed", op.PlatformRequestID, receipt, ""); err != nil {
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
	for _, arg := range writeArgs {
		command += " " + powershellLiteral(arg)
	}
	opID, err := s.startOperation("orca_native_agent_terminal", key, map[string]string{"worktree": worktree.ID, "title": title, "model": fixedConfig.Model, "effort": fixedConfig.ReasoningEffort, "command": command})
	if err != nil {
		return err
	}
	platform, callErr := orcaworker.CreateFixedAgentTerminal(ctx, orcaWorkerMetaCaller(fixedConfig), worktree.ID, title, fixedConfig.Orca.Shell, command)
	if callErr != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, callErr.Error())
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch needs reconciliation", Cause: callErr}
	}
	parsedTerminal, err := orcaworker.ParseFixedAgentTerminalReceipt(platform, worktree.ID, title)
	if err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", platform.Payload, "fixed agent terminal creation receipt is incomplete")
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch receipt is incomplete", Cause: err}
	}
	if err := s.changeOperation(opID, "accepted", "", platform.Payload, ""); err != nil {
		return err
	}
	if err := s.waitAndBindNativeOrcaFixedAgent(ctx, run, execution, worktree, parsedTerminal.Handle, repoID, base); err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", platform.Payload, err.Error())
		return err
	}
	if err := s.changeOperation(opID, "confirmed", "", platform.Payload, ""); err != nil {
		return err
	}
	return s.prepareNativeOrcaCoordinator(ctx, run, execution)
}

// Orca may rename an agent terminal after it starts. The creation receipt is
// the immutable link between the launch intent and the live terminal.
func fixedOrcaAgentTerminal(terminals []orcaTerminal, op *OperationRecord, title, worktreeID string) (orcaTerminal, error) {
	var expected orcaTerminal
	if op != nil && len(op.Receipt) != 0 {
		var receipt struct {
			Terminal orcaTerminal `json:"terminal"`
		}
		if err := json.Unmarshal(op.Receipt, &receipt); err != nil {
			return orcaTerminal{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch receipt is invalid", Cause: err}
		}
		expected = receipt.Terminal
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
