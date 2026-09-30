package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
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

func nativeOrcaResult[T any](env orcaEnvelope) (T, error) {
	var value T
	if err := json.Unmarshal(env.Result, &value); err != nil {
		return value, err
	}
	return value, nil
}

func nativeOrcaRepo(ctx context.Context, c Config) (string, error) {
	env, err := workerOrcaCall(ctx, c, "repo", "list", "--json")
	if err != nil {
		return "", err
	}
	var listing struct {
		Repos []struct{ ID, Path, Kind string } `json:"repos"`
	}
	listing, err = nativeOrcaResult[struct {
		Repos []struct{ ID, Path, Kind string } `json:"repos"`
	}](env)
	if err != nil {
		return "", err
	}
	for _, repo := range listing.Repos {
		if repo.Kind == "git" && repo.ID != "" && canonical(filepath.FromSlash(repo.Path)) == canonical(c.Repository) {
			return repo.ID, nil
		}
	}
	return "", &BackendError{Code: BackendUnsupported, Diagnostic: "fixed repository is not registered with local Orca"}
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
	env, err := workerOrcaCall(ctx, c, "terminal", "list", "--worktree", worktree, "--limit", "1000", "--json")
	if err != nil {
		return nil, "", err
	}
	var result struct {
		Terminals []orcaTerminal `json:"terminals"`
		Truncated bool           `json:"truncated"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Truncated || env.Meta.RuntimeID == "" {
		return nil, "", errors.Join(err, errors.New("Orca terminal inventory is incomplete"))
	}
	return result.Terminals, env.Meta.RuntimeID, nil
}

func nativeOrcaWorktreeComment(executionID string) string {
	return "SteerLine execution " + executionID
}

func nativeOrcaCoordinatorMarker(executionID string) string {
	return "SLCOORD:" + executionID
}

func nativeOrcaTerminalHasMarker(ctx context.Context, c Config, handle, marker string) (bool, error) {
	env, err := workerOrcaCall(ctx, c, "terminal", "read", "--terminal", handle, "--limit", "1000", "--json")
	if err != nil {
		return false, err
	}
	var result struct {
		Terminal struct {
			Handle string   `json:"handle"`
			Tail   []string `json:"tail"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Terminal.Handle != handle {
		return false, errors.Join(err, errors.New("Orca terminal marker read changed handle"))
	}
	for _, line := range result.Terminal.Tail {
		if strings.Contains(line, marker) {
			return true, nil
		}
	}
	return false, nil
}

func findNativeOrcaRun(ctx context.Context, c Config, objective, coordinator string) (string, error) {
	cursor := ""
	found := ""
	for page := 0; page < 100; page++ {
		args := []string{"orchestration", "run-list", "--limit", "1000", "--json"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		env, err := workerOrcaCall(ctx, c, args...)
		if err != nil {
			return "", err
		}
		var listing struct {
			Runs []struct {
				ID          string `json:"id"`
				Objective   string `json:"objective"`
				Coordinator string `json:"coordinator_handle"`
			} `json:"runs"`
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(env.Result, &listing); err != nil {
			return "", err
		}
		for _, candidate := range listing.Runs {
			if candidate.Objective == objective && candidate.Coordinator == coordinator {
				if found != "" {
					return "", errors.New("more than one Orca Run has the fixed execution identity")
				}
				found = candidate.ID
			}
		}
		if listing.NextCursor == "" {
			return found, nil
		}
		if listing.NextCursor == cursor {
			return "", errors.New("Orca Run pagination did not advance")
		}
		cursor = listing.NextCursor
	}
	return "", errors.New("Orca Run inventory exceeded its recovery bound")
}

func findNativeOrcaTask(ctx context.Context, c Config, runID, coordinator, spec string) (string, error) {
	env, err := workerOrcaCall(ctx, c, "orchestration", "task-list", "--run", runID, "--json")
	if err != nil {
		return "", err
	}
	var listing struct {
		RunID string `json:"runId"`
		Tasks []struct {
			ID      string `json:"id"`
			RunID   string `json:"run_id"`
			Spec    string `json:"spec"`
			Creator string `json:"created_by_terminal_handle"`
		} `json:"tasks"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(env.Result, &listing); err != nil || listing.RunID != runID || listing.Count != len(listing.Tasks) {
		return "", errors.Join(err, errors.New("Orca Task inventory is incomplete or changed Run"))
	}
	found := ""
	for _, candidate := range listing.Tasks {
		if candidate.RunID == runID && candidate.Spec == spec && candidate.Creator == coordinator {
			if found != "" {
				return "", errors.New("more than one Orca Task matches the original turn spec")
			}
			found = candidate.ID
		}
	}
	return found, nil
}

func findNativeOrcaDispatch(ctx context.Context, c Config, taskID string) (string, string, string, string, error) {
	env, err := workerOrcaCall(ctx, c, "orchestration", "dispatch-show", "--task", taskID, "--json")
	if err != nil {
		return "", "", "", "", err
	}
	var result struct {
		Dispatch struct {
			ID          string `json:"id"`
			TaskID      string `json:"task_id"`
			RunID       string `json:"run_id"`
			Assignee    string `json:"assignee_handle"`
			Incarnation string `json:"process_incarnation"`
		} `json:"dispatch"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return "", "", "", "", err
	}
	if result.Dispatch.ID == "" || result.Dispatch.TaskID != taskID {
		return "", "", "", "", errors.New("Orca Dispatch is missing or changed Task")
	}
	return result.Dispatch.ID, result.Dispatch.RunID, result.Dispatch.Assignee, result.Dispatch.Incarnation, nil
}

func findNativeOrcaReply(ctx context.Context, c Config, runID, dispatchID, questionID, answer string) (string, error) {
	env, err := workerOrcaCall(ctx, c, "orchestration", "inbox", "--limit", "1000", "--full", "--json")
	if err != nil {
		return "", err
	}
	var result struct {
		Messages []struct {
			ID     string `json:"id"`
			RunID  string `json:"run_id"`
			From   string `json:"from_handle"`
			To     string `json:"to_handle"`
			Thread string `json:"thread_id"`
			Body   string `json:"body"`
			Type   string `json:"type"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return "", err
	}
	found := ""
	for _, message := range result.Messages {
		if message.RunID != runID || message.Thread != questionID || message.From != "run:"+runID || message.To != "dispatch:"+dispatchID || message.Type != "status" {
			continue
		}
		if message.Body != answer || message.ID == "" || found != "" {
			return "", errors.New("original Orca question has conflicting or duplicate reply history")
		}
		found = message.ID
	}
	return found, nil
}

func findNativeOrcaSend(ctx context.Context, c Config, runID, dispatchID, subject, body string) (string, error) {
	env, err := workerOrcaCall(ctx, c, "orchestration", "inbox", "--limit", "1000", "--full", "--json")
	if err != nil {
		return "", err
	}
	var result struct {
		Messages []struct {
			ID      string `json:"id"`
			RunID   string `json:"run_id"`
			From    string `json:"from_handle"`
			To      string `json:"to_handle"`
			Subject string `json:"subject"`
			Body    string `json:"body"`
			Type    string `json:"type"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return "", err
	}
	found := ""
	for _, message := range result.Messages {
		if message.RunID != runID || message.From != "run:"+runID || message.To != "dispatch:"+dispatchID || message.Subject != subject || message.Type != "status" {
			continue
		}
		if message.Body != body || message.ID == "" || found != "" {
			return "", errors.New("Orca feedback has conflicting or duplicate send history")
		}
		found = message.ID
	}
	return found, nil
}

func verifyRecoveredNativeOrcaDispatch(ctx context.Context, c Config, dispatchID, taskID, runID string, identity ProcessIdentity) error {
	env, err := workerOrcaCall(ctx, c, "orchestration", "worker-show", "--dispatch", dispatchID, "--json")
	if err != nil {
		return err
	}
	var result struct {
		Dispatch struct {
			ID     string `json:"id"`
			TaskID string `json:"taskId"`
			RunID  string `json:"runId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID    string `json:"dispatchId"`
			Stage         string `json:"stage"`
			AgentTerminal string `json:"agentTerminalHandle"`
		} `json:"worker"`
		Terminal struct {
			Handle      string `json:"handle"`
			Incarnation string `json:"incarnationId"`
			WorktreeID  string `json:"worktreeId"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return err
	}
	if env.Meta.RuntimeID != identity.RuntimeID || result.Dispatch.ID != dispatchID || result.Dispatch.TaskID != taskID || result.Dispatch.RunID != runID || result.Worker.DispatchID != dispatchID || result.Worker.Stage != "injected" || result.Worker.AgentTerminal != identity.Terminal || result.Terminal.Handle != identity.Terminal || result.Terminal.Incarnation != identity.Incarnation || result.Terminal.WorktreeID != identity.WorktreeID {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "recovered Orca Dispatch has no exact injected worker identity"}
	}
	return nil
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
	env, callErr := workerOrcaCall(ctx, fixedConfig, "worktree", "create", "--repo", "id:"+repoID, "--name", name, "--base-branch", base, "--no-parent", "--setup", "skip", "--agent", fixedConfig.agentName(), "--comment", nativeOrcaWorktreeComment(execution.ExecutionID), "--json")
	if callErr != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, callErr.Error())
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca worktree/agent creation has no confirmed receipt; do not create another agent", Cause: callErr}
	}
	var result struct {
		Worktree            workerOrcaWorktree `json:"worktree"`
		AgentTerminalHandle string             `json:"agentTerminalHandle"`
		StartupTerminal     struct {
			Handle string `json:"handle"`
		} `json:"startupTerminal"`
	}
	result, err = nativeOrcaResult[struct {
		Worktree            workerOrcaWorktree `json:"worktree"`
		AgentTerminalHandle string             `json:"agentTerminalHandle"`
		StartupTerminal     struct {
			Handle string `json:"handle"`
		} `json:"startupTerminal"`
	}](env)
	if err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		return err
	}
	if err := validateWorkerOrcaWorktree(result.Worktree, name, repoID, base); err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		return err
	}
	handle := result.AgentTerminalHandle
	if handle == "" {
		handle = result.StartupTerminal.Handle
	}
	if handle == "" {
		_ = s.changeOperation(opID, "outcome_unknown", "", env.Result, "agent handle omitted")
		return errors.New("Orca did not return the created agent terminal handle")
	}
	if err := s.changeOperation(opID, "accepted", "", env.Result, ""); err != nil {
		return err
	}
	if err := s.bindNativeOrcaAgent(ctx, run, execution, result.Worktree, handle, repoID, base); err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", env.Result, err.Error())
		return err
	}
	if err := s.changeOperation(opID, "confirmed", "", env.Result, ""); err != nil {
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
			env, err := workerOrcaCall(ctx, fixedConfig, "terminal", "create", "--worktree", "path:"+filepath.ToSlash(fixedConfig.Repository), "--title", title, "--shell", "cmd.exe", "--command", "echo "+marker, "--json")
			if err != nil {
				_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
				return err
			}
			var result struct {
				Terminal orcaTerminal `json:"terminal"`
			}
			result, err = nativeOrcaResult[struct {
				Terminal orcaTerminal `json:"terminal"`
			}](env)
			if err != nil || result.Terminal.Handle == "" {
				_ = s.changeOperation(opID, "outcome_unknown", "", env.Result, "coordinator handle omitted")
				return errors.New("Orca coordinator creation has no exact handle")
			}
			if err := s.changeOperation(opID, "accepted", "", env.Result, ""); err != nil {
				return err
			}
			if err := s.bindNativeOrcaCoordinator(ctx, run, execution, result.Terminal.Handle); err != nil {
				return err
			}
			if err := s.changeOperation(opID, "confirmed", "", env.Result, ""); err != nil {
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
		env, err := workerOrcaCall(ctx, fixedConfig, "orchestration", "run-create", "--objective", objective, "--from", state.Coordinator.Terminal, "--json")
		if err != nil {
			_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
			return err
		}
		var result struct {
			Run struct {
				ID string `json:"id"`
			} `json:"run"`
			Mutation struct {
				RequestID string `json:"requestId"`
			} `json:"mutation"`
		}
		result, err = nativeOrcaResult[struct {
			Run struct {
				ID string `json:"id"`
			} `json:"run"`
			Mutation struct {
				RequestID string `json:"requestId"`
			} `json:"mutation"`
		}](env)
		if err != nil || result.Run.ID == "" || result.Mutation.RequestID == "" || env.Meta.RuntimeID != state.Coordinator.RuntimeID {
			return errors.New("Orca Run receipt lacks exact runtime, Run or request identity")
		}
		execution.Target.OrchestrationRun = result.Run.ID
		execution.Target.OrchestrationRunRequestID = result.Mutation.RequestID
		execution.Target.OrchestrationRun = result.Run.ID
		s.refreshRun(run)
		if err := s.changeOperation(opID, "confirmed", result.Mutation.RequestID, env.Result, ""); err != nil {
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
			env, err := workerOrcaCall(ctx, fixedConfig, "orchestration", "task-create", "--spec", prompt, "--task-title", run.Title, "--run", execution.Target.OrchestrationRun, "--from", state.Coordinator.Terminal, "--json")
			if err != nil {
				_ = s.changeOperation(taskOp, "outcome_unknown", "", nil, err.Error())
				return err
			}
			var task struct {
				Task struct {
					ID string `json:"id"`
				} `json:"task"`
				Mutation struct {
					RequestID string `json:"requestId"`
				} `json:"mutation"`
			}
			if err := json.Unmarshal(env.Result, &task); err != nil || task.Task.ID == "" || task.Mutation.RequestID == "" || env.Meta.RuntimeID != state.Coordinator.RuntimeID {
				return errors.New("Orca Task receipt lacks exact Task/request/runtime identity")
			}
			state.Turns = append(state.Turns, OrcaNativeTurn{TurnID: turnID, TaskID: task.Task.ID, TaskRequestID: task.Mutation.RequestID})
			execution.Target.ExternalTask = task.Task.ID
			s.refreshRun(run)
			if err := s.changeOperation(taskOp, "confirmed", task.Mutation.RequestID, env.Result, ""); err != nil {
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
	var env orcaEnvelope
	for {
		env, err = workerOrcaCall(ctx, fixedConfig, "orchestration", "dispatch", "--task", taskID, "--to", execution.Process.Terminal, "--run", execution.Target.OrchestrationRun, "--from", state.Coordinator.Terminal, "--inject", "--json")
		if err == nil {
			break
		}
		var rejected struct {
			Reason string `json:"reason"`
		}
		if env.Error != nil {
			_ = json.Unmarshal(env.Error.Data, &rejected)
		}
		if env.Error == nil || env.Error.Code != "inject_rejected" || rejected.Reason != "no_agent_detected" {
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
	var dispatched struct {
		Dispatch struct {
			ID     string `json:"id"`
			TaskID string `json:"task_id"`
			RunID  string `json:"run_id"`
		} `json:"dispatch"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	dispatched, err = nativeOrcaResult[struct {
		Dispatch struct {
			ID     string `json:"id"`
			TaskID string `json:"task_id"`
			RunID  string `json:"run_id"`
		} `json:"dispatch"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}](env)
	if err != nil || dispatched.Dispatch.ID == "" || dispatched.Mutation.RequestID == "" || env.Meta.RuntimeID != state.Coordinator.RuntimeID || dispatched.Dispatch.TaskID != "" && dispatched.Dispatch.TaskID != taskID || dispatched.Dispatch.RunID != "" && dispatched.Dispatch.RunID != execution.Target.OrchestrationRun {
		return errors.New("Orca Dispatch receipt lacks exact identity")
	}
	current.DispatchID, current.DispatchRequestID = dispatched.Dispatch.ID, dispatched.Mutation.RequestID
	s.bindNativeOrcaDispatch(run, execution, current)
	if err := s.changeOperation(dispatchOp, "accepted", dispatched.Mutation.RequestID, env.Result, ""); err != nil {
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
	payload, err := supervisedMessagePayload(message)
	if err == nil {
		var body struct {
			Question string            `json:"question"`
			Options  []json.RawMessage `json:"options"`
		}
		if json.Unmarshal(payload, &body) == nil {
			if strings.TrimSpace(body.Question) != "" {
				request.Question = clip(body.Question, 1800)
			}
			for i, raw := range body.Options {
				var label string
				if json.Unmarshal(raw, &label) != nil {
					var option struct {
						ID     string `json:"id"`
						Label  string `json:"label"`
						Detail string `json:"detail"`
					}
					if json.Unmarshal(raw, &option) != nil {
						continue
					}
					if option.ID == "" {
						option.ID = fmt.Sprintf("option-%d", i+1)
					}
					if option.Label != "" {
						request.Options = append(request.Options, DecisionOption{ID: option.ID, Label: option.Label, Detail: option.Detail})
					}
				} else if label != "" {
					request.Options = append(request.Options, DecisionOption{ID: fmt.Sprintf("option-%d", i+1), Label: label})
				}
			}
		}
	}
	request.OrcaAsk = &orcaAskCall{RunID: runID, TaskID: turn.TaskID, DispatchID: turn.DispatchID, MessageID: message.ID, Questions: []nativeAskQuestion{{ID: message.ID, Header: "Orca", Question: request.Question, Options: request.Options, AllowFreeText: true}}}
	return request
}

func nativeOrcaMessageMatches(message OrcaInboxMessage, turn OrcaNativeTurn, agentTerminal string) bool {
	if message.FromHandle != "dispatch:"+turn.DispatchID && message.FromHandle != agentTerminal {
		return false
	}
	payload, err := supervisedMessagePayload(message)
	if err != nil {
		return false
	}
	if len(payload) == 0 && (message.Type == "status" || message.Type == "escalation") && message.FromHandle == "dispatch:"+turn.DispatchID {
		return true
	}
	var identity struct {
		TaskID     string `json:"taskId"`
		DispatchID string `json:"dispatchId"`
	}
	return json.Unmarshal(payload, &identity) == nil && identity.TaskID == turn.TaskID && identity.DispatchID == turn.DispatchID
}

func nativeOrcaDoneOutcome(message OrcaInboxMessage, turn OrcaNativeTurn) (string, error) {
	payload, err := supervisedMessagePayload(message)
	if err != nil {
		return "", err
	}
	var result struct {
		TaskID     string `json:"taskId"`
		DispatchID string `json:"dispatchId"`
		Outcome    string `json:"outcome"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return "", err
	}
	if result.TaskID != turn.TaskID || result.DispatchID != turn.DispatchID || result.Outcome != "succeeded" && result.Outcome != "failed" {
		return "", errors.New("worker_done does not match the current Orca Task/Dispatch")
	}
	return result.Outcome, nil
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
		env, err := workerOrcaCall(ctx, fixedConfig, "orchestration", "reply", "--id", orcaQuestionID(run.Decision), "--body", body, "--run", execution.Target.OrchestrationRun, "--from", execution.OrcaNative.Coordinator.Terminal, "--json")
		if err != nil {
			_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
			return s.nativeOrcaUncertain(run, "Orca reply outcome needs reconciliation: "+err.Error())
		}
		var result struct {
			Question struct {
				MessageID  string `json:"message_id"`
				DispatchID string `json:"dispatch_id"`
				AnswerBody string `json:"answer_body"`
			} `json:"question"`
			Mutation struct {
				RequestID string `json:"requestId"`
			} `json:"mutation"`
		}
		result, err = nativeOrcaResult[struct {
			Question struct {
				MessageID  string `json:"message_id"`
				DispatchID string `json:"dispatch_id"`
				AnswerBody string `json:"answer_body"`
			} `json:"question"`
			Mutation struct {
				RequestID string `json:"requestId"`
			} `json:"mutation"`
		}](env)
		if err != nil || result.Mutation.RequestID == "" || env.Meta.RuntimeID != execution.OrcaNative.Coordinator.RuntimeID || result.Question.MessageID != orcaQuestionID(run.Decision) || result.Question.DispatchID != turn.DispatchID || result.Question.AnswerBody != body {
			return s.nativeOrcaUncertain(run, "Orca reply receipt lacks its original request identity")
		}
		if err := s.changeOperation(opID, "confirmed", result.Mutation.RequestID, env.Result, ""); err != nil {
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
	env, err := workerOrcaCall(ctx, fixedConfig, "orchestration", "send", "--subject", name, "--body", body, "--type", "status", "--to", "dispatch:"+turn.DispatchID, "--run", execution.Target.OrchestrationRun, "--from", execution.OrcaNative.Coordinator.Terminal, "--json")
	if err != nil {
		if env.Error != nil && env.Error.Code == "dispatch_inactive" {
			_ = s.changeOperation(opID, "rejected", "", nil, err.Error())
			execution.Error = ptr("Original Orca Dispatch is inactive; feedback remains saved until settlement is confirmed")
			s.refreshRun(run)
			return s.save()
		}
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		return s.nativeOrcaUncertain(run, "Orca send receipt is unknown: "+err.Error())
	}
	var result struct {
		Message struct {
			ID       string `json:"id"`
			ToHandle string `json:"to_handle"`
		} `json:"message"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	result, err = nativeOrcaResult[struct {
		Message struct {
			ID       string `json:"id"`
			ToHandle string `json:"to_handle"`
		} `json:"message"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}](env)
	if err != nil || result.Mutation.RequestID == "" || result.Message.ID == "" || result.Message.ToHandle != "dispatch:"+turn.DispatchID || env.Meta.RuntimeID != execution.OrcaNative.Coordinator.RuntimeID {
		return s.nativeOrcaUncertain(run, "Orca accepted feedback without an exact Dispatch receipt")
	}
	if err := s.changeOperation(opID, "confirmed", result.Mutation.RequestID, env.Result, ""); err != nil {
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
	s.executionRecord(run).Error = ptr(clip(reason, 2000))
	s.refreshRun(run)
	s.executionRecord(run).Status = "blocked"
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
		env, err := workerOrcaCall(ctx, fixedConfig, "orchestration", "check", "--terminal", state.Coordinator.Terminal, "--run", execution.Target.OrchestrationRun, "--json")
		if err != nil {
			return s.nativeOrcaUncertain(run, "Orca inbox is temporarily unavailable: "+err.Error())
		}
		if env.Meta.RuntimeID != state.Coordinator.RuntimeID {
			return s.nativeOrcaUncertain(run, "Orca coordinator runtime changed")
		}
		var delivery struct {
			RunID          string             `json:"runId"`
			DeliveryID     string             `json:"deliveryId"`
			Messages       []OrcaInboxMessage `json:"messages"`
			ConnectionLost bool               `json:"connectionLost"`
		}
		delivery, err = nativeOrcaResult[struct {
			RunID          string             `json:"runId"`
			DeliveryID     string             `json:"deliveryId"`
			Messages       []OrcaInboxMessage `json:"messages"`
			ConnectionLost bool               `json:"connectionLost"`
		}](env)
		if err != nil || delivery.RunID != execution.Target.OrchestrationRun || delivery.ConnectionLost || len(delivery.Messages) > 0 && delivery.DeliveryID == "" {
			return s.nativeOrcaUncertain(run, "Orca inbox delivery identity is unverified")
		}
		if len(delivery.Messages) > 0 {
			for _, message := range delivery.Messages {
				if message.ID == "" || nativeOrcaSeen(state, message.ID) {
					continue
				}
				if !nativeOrcaMessageMatches(message, *turn, execution.Process.Terminal) {
					state.SeenMessages = append(state.SeenMessages, message.ID)
					continue
				}
				switch message.Type {
				case "question":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					if run.Decision == nil || orcaQuestionID(run.Decision) != message.ID {
						decision, err := WriteDecisionRequest(run, nativeOrcaQuestion(message, execution.Target.OrchestrationRun, *turn))
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
					outcome, err := nativeOrcaDoneOutcome(message, *turn)
					if err != nil {
						return err
					}
					turn.DoneMessageID, turn.Outcome = message.ID, outcome
					if err := s.bindNativeOrcaPR(ctx, run, workflow, execution, message.Body); err != nil {
						execution.Error = ptr(clip(err.Error(), 2000))
						s.refreshRun(run)
					}
				case "status":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					if err := s.bindNativeOrcaPR(ctx, run, workflow, execution, message.Body); err != nil {
						execution.Error = ptr(clip(err.Error(), 2000))
						s.refreshRun(run)
					}
				case "escalation":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					if run.Decision == nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved" {
						decision, err := WriteDecisionRequest(run, DecisionRequest{Question: clip("Orca worker escalation: "+message.Subject, 1800), Context: clip(message.Body, 7000), Options: []DecisionOption{{ID: "continue", Label: "Continue current work"}}, AllowFreeText: true})
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
	if err := s.readNativeOrcaOutput(ctx, run, execution, turn); err != nil {
		execution.Error = ptr(clip("Orca output is bounded or unavailable: "+err.Error(), 2000))
		s.refreshRun(run)
		_ = s.save()
	}
	if turn.DoneMessageID != "" {
		return s.finishNativeOrcaTurn(ctx, run, workflow, execution, turn)
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
	urls := workerPRClues(body)
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
		observed, err := workerOrcaCall(ctx, fixedConfig, "orchestration", "check", "--terminal", state.Coordinator.Terminal, "--run", execution.Target.OrchestrationRun, "--json")
		if err != nil {
			return err
		}
		var current struct {
			RunID      string `json:"runId"`
			DeliveryID string `json:"deliveryId"`
		}
		if err := json.Unmarshal(observed.Result, &current); err != nil || observed.Meta.RuntimeID != state.Coordinator.RuntimeID || current.RunID != execution.Target.OrchestrationRun {
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
	env, err := workerOrcaCall(ctx, fixedConfig, "orchestration", "check", "--terminal", state.Coordinator.Terminal, "--run", execution.Target.OrchestrationRun, "--ack", deliveryID, "--json")
	if err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		return err
	}
	var result struct {
		Acknowledged string `json:"acknowledged"`
		Mutation     struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	result, err = nativeOrcaResult[struct {
		Acknowledged string `json:"acknowledged"`
		Mutation     struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}](env)
	if err != nil || env.Meta.RuntimeID != state.Coordinator.RuntimeID || result.Acknowledged != deliveryID || result.Mutation.RequestID == "" {
		return errors.New("Orca did not acknowledge the saved Delivery")
	}
	return s.changeOperation(opID, "confirmed", result.Mutation.RequestID, env.Result, "")
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
	args := []string{"orchestration", "worker-read", "--dispatch", turn.DispatchID, "--source", "auto", "--limit", "100", "--json"}
	if attempt.OutputCursor != "" {
		args = append(args, "--cursor", attempt.OutputCursor)
	}
	env, err := workerOrcaCall(ctx, fixedConfig, args...)
	if err != nil && strings.Contains(err.Error(), "source_changed") {
		attempt.OutputCursor = ""
		env, err = workerOrcaCall(ctx, fixedConfig, "orchestration", "worker-read", "--dispatch", turn.DispatchID, "--source", "auto", "--limit", "100", "--json")
	}
	if err != nil {
		return err
	}
	var result struct {
		DispatchID      string   `json:"dispatchId"`
		Source          string   `json:"source"`
		Cursor          string   `json:"cursor"`
		NextCursor      string   `json:"nextCursor"`
		SourceChanged   bool     `json:"sourceChanged"`
		ContentComplete bool     `json:"contentComplete"`
		Clipping        []string `json:"clipping"`
		FallbackReason  string   `json:"fallbackReason"`
		Terminal        struct {
			Tail []string `json:"tail"`
		} `json:"terminal"`
		Transcript json.RawMessage `json:"transcript"`
	}
	result, err = nativeOrcaResult[struct {
		DispatchID      string   `json:"dispatchId"`
		Source          string   `json:"source"`
		Cursor          string   `json:"cursor"`
		NextCursor      string   `json:"nextCursor"`
		SourceChanged   bool     `json:"sourceChanged"`
		ContentComplete bool     `json:"contentComplete"`
		Clipping        []string `json:"clipping"`
		FallbackReason  string   `json:"fallbackReason"`
		Terminal        struct {
			Tail []string `json:"tail"`
		} `json:"terminal"`
		Transcript json.RawMessage `json:"transcript"`
	}](env)
	if err != nil || result.DispatchID != turn.DispatchID || result.Source == "" || env.Meta.RuntimeID != execution.OrcaNative.Coordinator.RuntimeID {
		return errors.New("Orca worker-read changed source identity")
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
	fixedConfig := fixed.config
	env, err := workerOrcaCall(ctx, fixedConfig, "orchestration", "worker-show", "--dispatch", turn.DispatchID, "--json")
	if err != nil {
		return s.nativeOrcaUncertain(run, "Orca worker_done arrived but Dispatch settlement is unverified: "+err.Error())
	}
	var result struct {
		Dispatch struct {
			ID     string `json:"id"`
			RunID  string `json:"runId"`
			TaskID string `json:"taskId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID          string `json:"dispatchId"`
			State               string `json:"state"`
			AgentTerminalHandle string `json:"agentTerminalHandle"`
		} `json:"worker"`
		Terminal struct {
			Handle        string `json:"handle"`
			IncarnationID string `json:"incarnationId"`
			WorktreeID    string `json:"worktreeId"`
		} `json:"terminal"`
	}
	result, err = nativeOrcaResult[struct {
		Dispatch struct {
			ID     string `json:"id"`
			RunID  string `json:"runId"`
			TaskID string `json:"taskId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID          string `json:"dispatchId"`
			State               string `json:"state"`
			AgentTerminalHandle string `json:"agentTerminalHandle"`
		} `json:"worker"`
		Terminal struct {
			Handle        string `json:"handle"`
			IncarnationID string `json:"incarnationId"`
			WorktreeID    string `json:"worktreeId"`
		} `json:"terminal"`
	}](env)
	if err != nil || env.Meta.RuntimeID != execution.OrcaNative.Coordinator.RuntimeID || result.Dispatch.ID != turn.DispatchID || result.Dispatch.TaskID != turn.TaskID || result.Dispatch.RunID != execution.Target.OrchestrationRun || result.Worker.DispatchID != turn.DispatchID || result.Worker.AgentTerminalHandle != execution.Process.Terminal || result.Terminal.Handle != execution.Process.Terminal || result.Terminal.IncarnationID != execution.Process.Incarnation || result.Terminal.WorktreeID != execution.Process.WorktreeID {
		return s.nativeOrcaUncertain(run, "Orca settled Dispatch identity differs from the fixed worker")
	}
	if turn.Outcome == "succeeded" && result.Dispatch.Status != "completed" || turn.Outcome == "failed" && result.Dispatch.Status != "failed" {
		return s.nativeOrcaUncertain(run, "Orca worker_done and Dispatch settlement disagree")
	}
	if run.Decision != nil && orcaQuestionID(run.Decision) != "" && run.Decision.Status == "answer_saved" {
		return s.nativeOrcaUncertain(run, "Orca Dispatch settled while the original question reply remains unverified")
	}
	pendingQuestion := run.Decision != nil && run.Decision.Status == "pending" && orcaQuestionID(run.Decision) != ""
	if err := StopWorkerTurn(workflow, execution, turn.TurnID, pendingQuestion); err != nil {
		return err
	}
	execution.TurnEndedAt = ptr(now())
	execution.LastTurnOutcome = turn.Outcome
	s.refreshRun(run)
	if pendingQuestion {
		s.setDecisionStatus(run, "superseded")
		if err := s.save(); err != nil {
			return err
		}
		s.setRunDecision(run, nil)
	}
	if workflow.Phase == "waiting_pr_merge" {
		s.refreshRun(run)
		if err := s.offerWorkerPRFeedback(run, workflow.PRURL); err != nil {
			return err
		}
		return s.save()
	}
	return s.waitWorkerFlow(run, "The original Orca Dispatch settled ("+turn.Outcome+"). Send feedback to continue the same agent session.")
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
	env, err := workerOrcaCall(ctx, fixedConfig, "terminal", "close", "--terminal", identity.Terminal, "--json")
	if err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		return err
	}
	var result struct {
		Close struct {
			Handle    string `json:"handle"`
			PTYKilled bool   `json:"ptyKilled"`
		} `json:"close"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || env.Meta.RuntimeID != identity.RuntimeID || result.Close.Handle != identity.Terminal || !result.Close.PTYKilled {
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca terminal close lacks proof for the original process"}
	}
	return s.changeOperation(opID, "confirmed", "", env.Result, "")
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
	env, err := workerOrcaCall(ctx, fixedConfig, "orchestration", "check", "--terminal", state.Coordinator.Terminal, "--run", execution.Target.OrchestrationRun, "--json")
	if err != nil {
		return err
	}
	var delivery struct {
		RunID      string             `json:"runId"`
		DeliveryID string             `json:"deliveryId"`
		Messages   []OrcaInboxMessage `json:"messages"`
	}
	if err := json.Unmarshal(env.Result, &delivery); err != nil || env.Meta.RuntimeID != state.Coordinator.RuntimeID || delivery.RunID != execution.Target.OrchestrationRun || delivery.DeliveryID != "" || len(delivery.Messages) != 0 {
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca coordinator inbox is not drained after delivery"}
	}
	return s.closeNativeOrcaTerminal(ctx, execution, state.Coordinator, "coordinator")
}
