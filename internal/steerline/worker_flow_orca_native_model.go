package steerline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
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
	repoID, err := nativeOrcaRepo(ctx, s.Config)
	if err != nil {
		return err
	}
	name, err := workerOrcaWorktreeName(run.RunID)
	if err != nil {
		return err
	}
	base := run.WorkspaceIntent.BaseCommit
	worktree, exists, err := findWorkerOrcaWorktree(ctx, s.Config, repoID, name, base)
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
		env, callErr := workerOrcaCall(ctx, s.Config, "worktree", "create", "--repo", "id:"+repoID, "--name", name, "--base-branch", base, "--no-parent", "--setup", "skip", "--comment", nativeOrcaWorktreeComment(execution.ExecutionID), "--json")
		if callErr != nil {
			_ = s.changeOperation(opID, "outcome_unknown", "", nil, callErr.Error())
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca worktree creation needs reconciliation", Cause: callErr}
		}
		var result struct {
			Worktree workerOrcaWorktree `json:"worktree"`
		}
		result, err = nativeOrcaResult[struct {
			Worktree workerOrcaWorktree `json:"worktree"`
		}](env)
		if err != nil || validateWorkerOrcaWorktree(result.Worktree, name, repoID, base) != nil || result.Worktree.CreatedWithAgent != "" {
			_ = s.changeOperation(opID, "outcome_unknown", "", env.Result, "fixed worktree receipt is incomplete")
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca worktree receipt is incomplete", Cause: err}
		}
		worktree = result.Worktree
		if err := s.changeOperation(opID, "confirmed", "", env.Result, ""); err != nil {
			return err
		}
	}
	if worktree.Comment != nativeOrcaWorktreeComment(execution.ExecutionID) || worktree.CreatedWithAgent != "" {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "fixed Orca worktree belongs to another execution or agent"}
	}
	return s.prepareNativeOrcaFixedAgentTerminal(ctx, run, execution, worktree, repoID, base)
}

func (s *Service) prepareNativeOrcaFixedAgentTerminal(ctx context.Context, run *Run, execution *ExecutionRecord, worktree workerOrcaWorktree, repoID, base string) error {
	title := nativeOrcaFixedAgentTitle(execution.ExecutionID)
	terminals, _, err := nativeOrcaTerminals(ctx, s.Config, "id:"+worktree.ID)
	if err != nil {
		return err
	}
	handle := ""
	for _, terminal := range terminals {
		if terminal.Title != title {
			continue
		}
		if handle != "" || terminal.WorktreeID != worktree.ID {
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent terminal is not unique"}
		}
		handle = terminal.Handle
	}
	key := "orca-native-agent-terminal:" + execution.ExecutionID
	if op := s.nativeOrcaOperation(key); op != nil {
		if handle == "" {
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch has no identifiable terminal"}
		}
		if err := s.waitAndBindNativeOrcaFixedAgent(ctx, run, execution, worktree, handle, repoID, base); err != nil {
			return err
		}
		if op.Status != "confirmed" {
			if err := s.changeOperation(op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"terminal": handle}), ""); err != nil {
				return err
			}
		}
		return s.prepareNativeOrcaCoordinator(ctx, run, execution)
	}
	if handle != "" {
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent terminal exists without its launch intent"}
	}
	command, err := nativeOrcaFixedAgentCommand(s.Config)
	if err != nil {
		return err
	}
	opID, err := s.startOperation("orca_native_agent_terminal", key, map[string]string{"worktree": worktree.ID, "title": title, "model": s.Config.Model, "effort": s.Config.ReasoningEffort, "command": command})
	if err != nil {
		return err
	}
	env, callErr := workerOrcaCall(ctx, s.Config, "terminal", "create", "--worktree", "id:"+worktree.ID, "--title", title, "--shell", s.Config.Orca.Shell, "--command", command, "--json")
	if callErr != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, callErr.Error())
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch needs reconciliation", Cause: callErr}
	}
	var result struct {
		Terminal orcaTerminal `json:"terminal"`
	}
	result, err = nativeOrcaResult[struct {
		Terminal orcaTerminal `json:"terminal"`
	}](env)
	if err != nil || result.Terminal.Handle == "" {
		_ = s.changeOperation(opID, "outcome_unknown", "", env.Result, "agent terminal handle omitted")
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch omitted its terminal", Cause: err}
	}
	if err := s.changeOperation(opID, "accepted", "", env.Result, ""); err != nil {
		return err
	}
	if err := s.waitAndBindNativeOrcaFixedAgent(ctx, run, execution, worktree, result.Terminal.Handle, repoID, base); err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", env.Result, err.Error())
		return err
	}
	if err := s.changeOperation(opID, "confirmed", "", env.Result, ""); err != nil {
		return err
	}
	return s.prepareNativeOrcaCoordinator(ctx, run, execution)
}

func (s *Service) waitAndBindNativeOrcaFixedAgent(ctx context.Context, run *Run, execution *ExecutionRecord, worktree workerOrcaWorktree, handle, repoID, base string) error {
	deadline := time.Now().Add(min(positiveDuration(s.Config.Orca.WaitTimeout, 30*time.Second), 30*time.Second))
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
