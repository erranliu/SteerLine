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

func (s *Service) prepareNativeOrcaWorker(ctx context.Context, run *Run) error {
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || run.WorkspaceIntent == nil || s.Config.hostName() != "orca" {
		return errors.New("native Orca preparation lacks a fixed worker claim")
	}
	if execution.Process != nil {
		if execution.Process.Kind != "orca-native" || execution.OrcaNative == nil {
			return errors.New("old Orca worker state cannot be continued by the native protocol")
		}
		if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
			return err
		}
		return s.prepareNativeOrcaCoordinator(ctx, run, execution)
	}
	repoID, err := nativeOrcaRepo(ctx, s.Config)
	if err != nil {
		return err
	}
	name, err := workerOrcaWorktreeName(run.RunID)
	if err != nil {
		return err
	}
	base := run.WorkspaceIntent.BaseCommit
	if _, exists, err := findWorkerOrcaWorktree(ctx, s.Config, repoID, name, base); err != nil {
		return err
	} else if exists {
		return errors.New("fixed Orca worktree already exists without a saved agent terminal receipt; preserve it for manual reconciliation")
	}
	opID, err := s.startOperation("orca_native_worktree", "orca-native-worktree:"+execution.ExecutionID, map[string]string{"repo_id": repoID, "name": name, "base": base, "agent": s.Config.agentName()})
	if err != nil {
		return err
	}
	env, callErr := workerOrcaCall(ctx, s.Config, "worktree", "create", "--repo", "id:"+repoID, "--name", name, "--base-branch", base, "--no-parent", "--setup", "skip", "--agent", s.Config.agentName(), "--json")
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
	if err := applyWorkerOrcaWorkspace(s.Config, run, execution, result.Worktree, base); err != nil {
		return err
	}
	term, err := showWorkerOrcaTerminal(ctx, s.Config, handle)
	if err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", env.Result, err.Error())
		return err
	}
	if err := verifyWorkerOrcaTerminal(term, run, nil); err != nil {
		return err
	}
	if term.WorktreeID != result.Worktree.ID || term.AgentIdentity != s.Config.agentName() {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca started a different worktree or agent"}
	}
	identity := workerOrcaIdentity(term, s.Config)
	identity.Kind = "orca-native"
	identity.WorktreeID = result.Worktree.ID
	run.Process, execution.Process = &identity, &identity
	execution.OrcaNative = &OrcaNativeExecution{RepositoryID: repoID}
	if err := s.changeOperation(opID, "confirmed", "", env.Result, ""); err != nil {
		return err
	}
	return s.prepareNativeOrcaCoordinator(ctx, run, execution)
}

func (s *Service) prepareNativeOrcaCoordinator(ctx context.Context, run *Run, execution *ExecutionRecord) error {
	state := execution.OrcaNative
	if state == nil || execution.Process == nil {
		return errors.New("Orca agent binding is missing")
	}
	if state.Coordinator.Terminal == "" {
		opID, err := s.startOperation("orca_native_coordinator", "orca-native-coordinator:"+execution.ExecutionID, map[string]string{"execution_id": execution.ExecutionID, "repository": s.Config.Repository})
		if err != nil {
			return err
		}
		env, err := workerOrcaCall(ctx, s.Config, "terminal", "create", "--worktree", "path:"+filepath.ToSlash(s.Config.Repository), "--title", "SteerLine coordinator "+run.RunID, "--shell", "cmd.exe", "--json")
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
		term, err := showWorkerOrcaTerminal(ctx, s.Config, result.Terminal.Handle)
		if err != nil {
			return err
		}
		if canonical(filepath.FromSlash(term.WorktreePath)) != canonical(s.Config.Repository) || term.RuntimeID != execution.Process.RuntimeID {
			return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca coordinator is not on the fixed local runtime/repository"}
		}
		state.Coordinator = ProcessIdentity{Kind: "orca-native-coordinator", Terminal: term.Handle, RuntimeID: term.RuntimeID, Incarnation: term.IncarnationID, Executable: s.Config.Orca.Executable}
		execution.Target.CoordinatorTerminal = term.Handle
		run.Target.CoordinatorTerminal = term.Handle
		if err := s.changeOperation(opID, "confirmed", "", env.Result, ""); err != nil {
			return err
		}
	}
	if execution.Target.OrchestrationRun == "" {
		objective := "SteerLine worker " + execution.ExecutionID
		opID, err := s.startOperation("orca_native_run", "orca-native-run:"+execution.ExecutionID, map[string]string{"objective": objective, "coordinator": state.Coordinator.Terminal})
		if err != nil {
			return err
		}
		env, err := workerOrcaCall(ctx, s.Config, "orchestration", "run-create", "--objective", objective, "--from", state.Coordinator.Terminal, "--json")
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
		run.Target.OrchestrationRun = result.Run.ID
		if err := s.changeOperation(opID, "confirmed", result.Mutation.RequestID, env.Result, ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) verifyNativeOrcaWorker(ctx context.Context, run *Run, execution *ExecutionRecord) error {
	if execution.OrcaNative == nil || execution.Process == nil || execution.Process.Kind != "orca-native" {
		return errors.New("native Orca worker binding is incomplete")
	}
	term, err := showWorkerOrcaTerminal(ctx, s.Config, execution.Process.Terminal)
	if err != nil {
		return err
	}
	if err := verifyWorkerOrcaTerminal(term, run, nil); err != nil {
		return err
	}
	if term.RuntimeID != execution.Process.RuntimeID || term.IncarnationID != execution.Process.Incarnation || term.WorktreeID != execution.Process.WorktreeID || term.AgentIdentity != s.Config.agentName() {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "original Orca agent terminal or agent has changed"}
	}
	coordinator := execution.OrcaNative.Coordinator
	if coordinator.Terminal != "" {
		shown, err := showWorkerOrcaTerminal(ctx, s.Config, coordinator.Terminal)
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
	if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
		return err
	}
	if len([]byte(prompt)) > 64<<10 {
		return &BackendError{Code: BackendRejected, Diagnostic: "Orca native task spec exceeds the 64 KiB request bound"}
	}
	if execution.OrcaNative.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		return errors.New("Orca coordinator and Run are not prepared")
	}
	state := execution.OrcaNative
	for _, previous := range state.Turns {
		if previous.TurnID == turnID {
			return errors.New("Orca turn already has an original Task/Dispatch binding")
		}
	}
	taskOp, err := s.startOperation("orca_native_task", "orca-native-task:"+execution.ExecutionID+":"+turnID, map[string]string{"turn_id": turnID, "spec": prompt, "run": execution.Target.OrchestrationRun})
	if err != nil {
		return err
	}
	env, err := workerOrcaCall(ctx, s.Config, "orchestration", "task-create", "--spec", prompt, "--task-title", run.Title, "--run", execution.Target.OrchestrationRun, "--from", state.Coordinator.Terminal, "--json")
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
	task, err = nativeOrcaResult[struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}](env)
	if err != nil || task.Task.ID == "" || task.Mutation.RequestID == "" || env.Meta.RuntimeID != state.Coordinator.RuntimeID {
		return errors.New("Orca Task receipt lacks exact Task/request/runtime identity")
	}
	state.Turns = append(state.Turns, OrcaNativeTurn{TurnID: turnID, TaskID: task.Task.ID, TaskRequestID: task.Mutation.RequestID})
	execution.Target.ExternalTask, run.Target.ExternalTask = task.Task.ID, task.Task.ID
	if err := s.changeOperation(taskOp, "confirmed", task.Mutation.RequestID, env.Result, ""); err != nil {
		return err
	}
	dispatchOp, err := s.startOperation("orca_native_dispatch", "orca-native-dispatch:"+execution.ExecutionID+":"+turnID, map[string]string{"task": task.Task.ID, "terminal": execution.Process.Terminal, "run": execution.Target.OrchestrationRun})
	if err != nil {
		return err
	}
	deadline := time.Now().Add(positiveDuration(s.Config.Orca.WaitTimeout, 2*time.Minute))
	for {
		env, err = workerOrcaCall(ctx, s.Config, "orchestration", "dispatch", "--task", task.Task.ID, "--to", execution.Process.Terminal, "--run", execution.Target.OrchestrationRun, "--from", state.Coordinator.Terminal, "--inject", "--json")
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
	if err != nil || dispatched.Dispatch.ID == "" || dispatched.Mutation.RequestID == "" || env.Meta.RuntimeID != state.Coordinator.RuntimeID || dispatched.Dispatch.TaskID != "" && dispatched.Dispatch.TaskID != task.Task.ID || dispatched.Dispatch.RunID != "" && dispatched.Dispatch.RunID != execution.Target.OrchestrationRun {
		return errors.New("Orca Dispatch receipt lacks exact identity")
	}
	current := &state.Turns[len(state.Turns)-1]
	current.DispatchID, current.DispatchRequestID = dispatched.Dispatch.ID, dispatched.Mutation.RequestID
	execution.Target.Dispatch, run.Target.Dispatch = current.DispatchID, current.DispatchID
	execution.Process.Task, execution.Process.Dispatch, execution.Process.OrchestrationRun = task.Task.ID, current.DispatchID, execution.Target.OrchestrationRun
	run.Process = execution.Process
	if attempt := s.workflowAttempt(run, run.AttemptID); attempt != nil {
		attempt.ExternalBinding = execution.Target
		attempt.OutputSource, attempt.OutputCursor = "", ""
	}
	if err := s.changeOperation(dispatchOp, "accepted", dispatched.Mutation.RequestID, env.Result, ""); err != nil {
		return err
	}
	return nil
}

func nativeOrcaCurrentTurn(execution *ExecutionRecord) *OrcaNativeTurn {
	if execution == nil || execution.OrcaNative == nil || len(execution.OrcaNative.Turns) == 0 {
		return nil
	}
	return &execution.OrcaNative.Turns[len(execution.OrcaNative.Turns)-1]
}

func nativeOrcaQuestion(message OrcaInboxMessage) DecisionRequest {
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
	run.ExecutionStatus, run.FinishTaskStatus, run.Error = "running", "pending", nil
	if run.Decision != nil && run.Decision.Status == "answer_saved" && run.Decision.ExternalQuestionID == "" {
		run.Decision.Status = "reply_accepted"
		run.Decision.Version++
	}
	run.StartedAt = ptr(now())
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
	turn := nativeOrcaCurrentTurn(execution)
	if execution.Process == nil || execution.Process.Kind != "orca-native" || execution.OrcaNative == nil || turn == nil || turn.DispatchID == "" {
		return s.waitWorkerFlow(run, "Original Orca Dispatch identity is unavailable; feedback was retained")
	}
	if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
		run.Error = ptr("Feedback retained: original Orca session is unavailable: " + err.Error())
		return s.save()
	}
	if run.Decision.ExternalQuestionID != "" {
		if turn.DoneMessageID != "" || execution.LastStoppedTurnID == turn.TurnID {
			run.Error = ptr("The original Orca question expired with its Dispatch; feedback was retained")
			return s.save()
		}
		key := "orca-native-reply:" + execution.ExecutionID + ":" + commandID
		for _, op := range s.State.Operations {
			if op.IdempotencyKey == key {
				if op.Status == "confirmed" || op.Status == "accepted" {
					run.Decision.Status = "reply_accepted"
					run.Decision.Version++
					_ = ContinueWorkerQuestion(workflow, execution, turn.TurnID)
					return s.save()
				}
				run.Error = ptr("Orca reply outcome is unverified; the original question was not answered twice")
				return s.save()
			}
		}
		body := workerFeedbackText(answer, run.Decision.Request)
		opID, err := s.startOperation("orca_native_reply", key, map[string]string{"question": run.Decision.ExternalQuestionID, "answer": body, "dispatch": turn.DispatchID})
		if err != nil {
			return err
		}
		env, err := workerOrcaCall(ctx, s.Config, "orchestration", "reply", "--id", run.Decision.ExternalQuestionID, "--body", body, "--run", execution.Target.OrchestrationRun, "--from", execution.OrcaNative.Coordinator.Terminal, "--json")
		if err != nil {
			_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
			return s.waitWorkerFlow(run, "Orca reply outcome needs reconciliation: "+err.Error())
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
		if err != nil || result.Mutation.RequestID == "" || env.Meta.RuntimeID != execution.OrcaNative.Coordinator.RuntimeID || result.Question.MessageID != run.Decision.ExternalQuestionID || result.Question.DispatchID != turn.DispatchID || result.Question.AnswerBody != body {
			return s.waitWorkerFlow(run, "Orca reply receipt lacks its original request identity")
		}
		if err := s.changeOperation(opID, "confirmed", result.Mutation.RequestID, env.Result, ""); err != nil {
			return err
		}
		run.Decision.Status = "reply_accepted"
		run.Decision.Version++
		if err := ContinueWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
			return err
		}
		run.FinishTaskStatus = "pending"
		return s.save()
	}
	if turn.DoneMessageID == "" || execution.LastStoppedTurnID != turn.TurnID {
		run.Error = ptr("Feedback retained until the original Orca Dispatch settles")
		return s.save()
	}
	for _, op := range s.State.Operations {
		if op.IdempotencyKey == "worker-turn:"+execution.ExecutionID+":"+commandID {
			if op.Status == "accepted" || op.Status == "confirmed" {
				return nil
			}
			run.Error = ptr("Feedback delivery remains unverified; no duplicate Orca Task was created")
			return s.save()
		}
	}
	return s.startWorkerFlowTurn(ctx, run, workerFeedbackText(answer, run.Decision.Request), commandID)
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
	run.Error = ptr(clip(reason, 2000))
	run.ExecutionStatus = "unknown"
	s.note(run, "steer", "worker", "unverified", "Orca observation needs reconciliation", reason, nil)
	return s.save()
}

func (s *Service) pollNativeOrcaWorker(ctx context.Context, run *Run) error {
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
		env, err := workerOrcaCall(ctx, s.Config, "orchestration", "check", "--terminal", state.Coordinator.Terminal, "--run", execution.Target.OrchestrationRun, "--json")
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
					if run.Decision == nil || run.Decision.ExternalQuestionID != message.ID {
						decision, err := WriteDecisionRequest(run, nativeOrcaQuestion(message))
						if err != nil {
							return err
						}
						decision.ExternalQuestionID = message.ID
						run.Decision = decision
					}
					if err := NoteWorkerQuestion(workflow, execution, turn.TurnID); err != nil {
						return err
					}
					run.FinishTaskStatus = "waiting_decision"
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
						run.Error = ptr(clip(err.Error(), 2000))
					}
				case "status":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					if err := s.bindNativeOrcaPR(ctx, run, workflow, execution, message.Body); err != nil {
						run.Error = ptr(clip(err.Error(), 2000))
					}
				case "escalation":
					if err := s.beginNativeOrcaTurn(run, workflow, execution, turn); err != nil {
						return err
					}
					return s.waitWorkerFlow(run, "Orca worker escalation: "+message.Body)
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
		run.Error = ptr(clip("Orca output is bounded or unavailable: "+err.Error(), 2000))
		_ = s.save()
	}
	if turn.DoneMessageID != "" {
		return s.finishNativeOrcaTurn(ctx, run, workflow, execution, turn)
	}
	if workflow.Phase == "executing" && run.ExecutionStatus == "unknown" {
		run.ExecutionStatus = "running"
		run.Error = nil
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
	if err := BindWorkerPR(workflow, execution, info, urls[0], s.Config.PR.Repository, false); err != nil {
		return err
	}
	run.PR.URL, run.PR.State = ptr(info.URL), info.State
	return nil
}

func (s *Service) ackNativeOrcaDelivery(ctx context.Context, run *Run, execution *ExecutionRecord, deliveryID string) error {
	state := execution.OrcaNative
	env, err := workerOrcaCall(ctx, s.Config, "orchestration", "check", "--terminal", state.Coordinator.Terminal, "--run", execution.Target.OrchestrationRun, "--ack", deliveryID, "--json")
	if err != nil {
		return err
	}
	var result struct {
		Acknowledged string `json:"acknowledged"`
	}
	result, err = nativeOrcaResult[struct {
		Acknowledged string `json:"acknowledged"`
	}](env)
	if err != nil || env.Meta.RuntimeID != state.Coordinator.RuntimeID || result.Acknowledged != deliveryID {
		return errors.New("Orca did not acknowledge the saved Delivery")
	}
	return nil
}

func (s *Service) readNativeOrcaOutput(ctx context.Context, run *Run, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	attempt := s.workflowAttempt(run, run.AttemptID)
	if attempt == nil {
		return errors.New("worker output has no attempt")
	}
	args := []string{"orchestration", "worker-read", "--dispatch", turn.DispatchID, "--source", "auto", "--limit", "100", "--json"}
	if attempt.OutputCursor != "" {
		args = append(args, "--cursor", attempt.OutputCursor)
	}
	env, err := workerOrcaCall(ctx, s.Config, args...)
	if err != nil && strings.Contains(err.Error(), "source_changed") {
		attempt.OutputCursor = ""
		env, err = workerOrcaCall(ctx, s.Config, "orchestration", "worker-read", "--dispatch", turn.DispatchID, "--source", "auto", "--limit", "100", "--json")
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
	if result.SourceChanged || len(result.Clipping) > 0 || !result.ContentComplete {
		run.LastEvent = ptr("Orca output is bounded (" + result.Source + "; " + result.FallbackReason + ")")
	}
	return s.save()
}

func (s *Service) finishNativeOrcaTurn(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	env, err := workerOrcaCall(ctx, s.Config, "orchestration", "worker-show", "--dispatch", turn.DispatchID, "--json")
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
	pendingQuestion := run.Decision != nil && run.Decision.Status == "pending" && run.Decision.ExternalQuestionID != ""
	if err := StopWorkerTurn(workflow, execution, turn.TurnID, pendingQuestion); err != nil {
		return err
	}
	run.EndedAt = ptr(now())
	run.ExecutionStatus = turn.Outcome
	if workflow.Phase == "waiting_pr_merge" {
		run.FinishTaskStatus = "waiting_pr_merge"
		if err := offerWorkerPRFeedback(run, workflow.PRURL); err != nil {
			return err
		}
		return s.save()
	}
	return s.waitWorkerFlow(run, "The original Orca Dispatch settled ("+turn.Outcome+"). Send feedback to continue the same agent session.")
}
