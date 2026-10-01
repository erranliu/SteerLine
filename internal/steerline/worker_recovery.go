package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

// Operations are bound by their saved request and exact identity components,
// including host publication which is keyed by WorkflowRun rather than Execution.
func (s *Service) hasUnknownWorkerOperation(run *Run) bool {
	for _, op := range s.domain.Operations {
		if op.Status != "outcome_unknown" && op.Status != "in_flight" {
			continue
		}
		var target struct {
			ExecutionID   string `json:"execution_id"`
			WorkflowRunID string `json:"workflow_run_id"`
			RunID         string `json:"run_id"`
		}
		if json.Unmarshal(op.Request, &target) == nil && (target.ExecutionID == run.ExecutionID || target.WorkflowRunID == run.RunID || target.RunID == run.RunID) {
			return true
		}
		for _, identity := range strings.Split(op.IdempotencyKey, ":") {
			if identity == run.ExecutionID || identity == run.RunID {
				return true
			}
		}
	}
	return false
}

// beginLocalRecoveryAttempt advances the worker generation in one service
// write. The prior process and session stay on the retired Attempt; its
// process receipt and execution binding stay at their original paths.
func (s *Service) beginLocalRecoveryAttempt(ctx context.Context, run *Run, mode string) error {
	if mode == "" {
		mode = "continue" // Free-text feedback keeps the verified original thread.
	}
	if mode != "continue" && mode != "new_session" {
		return errors.New("choose the original Codex session or explicitly choose a new session")
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	execution, old := fixed.execution, fixed.attempt
	if run.Protocol != string(executionapi.ProtocolLocalCodexAppServer) || !execution.RecoveryRequired ||
		(execution.RecoveryState != "interrupted" && execution.RecoveryState != "completed") ||
		execution.Process == nil || inspectProcess(execution.Process) != "process absent" {
		return errors.New("the original worker process and its descendants are not confirmed stopped")
	}
	if mode == "continue" && !codexworker.ValidNativeID(execution.SessionID) {
		return errors.New("the original Codex thread identity is unavailable; choose a new session")
	}
	if execution.Workspace.Worktree == "" || run.WorktreePath == nil || canonical(*run.WorktreePath) != canonical(execution.Workspace.Worktree) {
		return errors.New("the original worktree identity is not verified")
	}
	workspace, err := s.inspectWorkflowWorkspace(ctx, run)
	if err != nil || canonical(workspace.Worktree) != canonical(execution.Workspace.Worktree) || workspace.Branch != execution.Workspace.Branch {
		return errors.New("the original worktree and branch could not be reverified")
	}
	applyWorkflowWorkspaceEvidence(run, execution, workspace)
	if !codexBackend(fixed.config, run).ManagedProcessAbsent(codexBinding(execution)) {
		return errors.New("the original process tree is not proven to be service managed")
	}
	if owned := s.ownedLocalProcesses[run.RunID]; owned != nil {
		if owned.Identity() != *execution.Process {
			return errors.New("owned process identity changed before recovery")
		}
		owned.Close()
		delete(s.ownedLocalProcesses, run.RunID)
	}
	if !workerTreeAbsent(*execution.Process) {
		return errors.New("original worker descendants may still write to the worktree")
	}
	if s.hasUnknownWorkerOperation(run) {
		return errors.New("an earlier external operation has an unknown outcome; reconcile it before continuing")
	}
	old.WorkerProcess = copyPtr(execution.Process)
	old.WorkerSessionID = execution.SessionID
	old.Status, old.UpdatedAt = "stopped", now()
	if run.Decision != nil {
		s.setDecisionStatus(run, "applied")
		s.setRunDecision(run, nil)
	}
	created := now()
	newAttemptID := newID()
	s.domain.Attempts = append(s.domain.Attempts, StepAttemptRecord{
		ProjectIdentity: old.ProjectIdentity, AttemptID: newAttemptID, ExecutionID: old.ExecutionID,
		StepID: "worker", AttemptNumber: old.AttemptNumber + 1, Status: "planned",
		InputRevision: old.InputRevision, ContractDigest: old.ContractDigest,
		ExternalBinding: old.ExternalBinding, CreatedAt: created, UpdatedAt: created,
	})
	execution.Process = nil
	execution.CurrentAttemptID = newAttemptID
	execution.AppServerEndpoint, execution.AppServerGeneration = "", ""
	execution.PendingCommandID, execution.CurrentCommandID = "", ""
	execution.PendingTurnID, execution.CurrentTurnID, execution.LastStoppedTurnID = "", "", ""
	execution.PendingAsks = nil
	execution.TurnStartedAt, execution.TurnEndedAt = nil, nil
	execution.LogPath = nil
	execution.Status, execution.LastTurnOutcome, execution.RecoveryState = "pending", "", "resuming"
	execution.RecoveryRequired = false
	if mode == "new_session" {
		execution.SessionID = ""
	}
	execution.UpdatedAt = created
	s.refreshRun(run)
	return s.commitWorker(run)
}

// A saved answer from a previous service instance is historical input. A new
// Decision gives the user a distinct, first-wins authorization for this run.
func (s *Service) refreshSavedRecoveryDecision(run *Run, preserveNative bool, reason string) error {
	if run == nil || run.Decision == nil || run.Decision.Status != "answer_saved" {
		return nil
	}
	old := *run.Decision
	execution := s.executionRecord(run)
	if execution == nil {
		return errors.New("saved recovery answer lost its execution")
	}
	if old.SourceAttemptID == run.AttemptID && old.RecoveryVersion == execution.RecoveryVersion &&
		execution.RecoveryMarkedAt != nil && old.AnsweredAt != nil && old.AnsweredAt.After(*execution.RecoveryMarkedAt) {
		return nil // This answer already authorizes the current recovery revision.
	}
	for _, op := range s.domain.Operations {
		if op.IdempotencyKey != "appserver-answer:"+execution.ExecutionID+":"+old.ID {
			continue
		}
		if op.Status == "confirmed" {
			s.setDecisionStatus(run, "reply_accepted")
			return s.commitWorker(run)
		}
		if op.Status == "in_flight" || op.Status == "accepted" || op.Status == "outcome_unknown" {
			execution.RecoveryState = "unverified"
			return s.waitWorkerFlow(run, "Original answer send has an unknown outcome; reconcile its native result before confirming again")
		}
	}
	if s.domainStore != nil && (old.Request.OrcaAsk != nil || run.Protocol == string(executionapi.ProtocolOrcaTerminalV2)) {
		oldAnswer := DecisionAnswer{DecisionID: old.ID, RunID: run.RunID, OptionID: old.OptionID, Note: old.Note, Answers: old.Answers, SourceAttemptID: old.SourceAttemptID, ExpectedRecoveryVersion: old.RecoveryVersion}
		if command, err := s.domainStore.CommandByIdempotency(context.Background(), s.Config.identity, decisionAnswerKey(oldAnswer)); err == nil {
			for _, op := range s.domain.Operations {
				if op.IdempotencyKey == "orca-native-reply:"+execution.ExecutionID+":"+command.CommandID ||
					op.IdempotencyKey == "orca-native-send:"+execution.ExecutionID+":"+command.CommandID {
					if op.Status == "confirmed" {
						s.setDecisionStatus(run, "reply_accepted")
						return s.commitWorker(run)
					}
					if op.Status != "in_flight" && op.Status != "accepted" && op.Status != "outcome_unknown" {
						continue
					}
					execution.RecoveryState = "unverified"
					return s.nativeOrcaUncertain(run, "Original Orca reply/send has an unknown outcome; reconcile its receipt before confirming again")
				}
			}
		}
	}
	oldInput := "Prior saved answer: " + workerFeedbackText(DecisionAnswer{OptionID: old.OptionID, Note: old.Note}, old.Request)
	if len(old.Answers) != 0 {
		oldInput += "\nNative answers: " + string(mustJSON(old.Answers))
	}
	s.setDecisionStatus(run, "superseded")
	if err := s.commitWorker(run); err != nil {
		return err
	}
	s.setRunDecision(run, nil)
	if preserveNative {
		request := old.Request
		request.Context = clip(reason+"\n\n"+oldInput+"\n\nPlease confirm or revise this answer for the live original request.", 7000)
		decision, err := WriteDecisionRequest(run, request)
		if err != nil {
			return err
		}
		s.setRunDecision(run, decision)
	} else {
		if err := s.waitWorkerFlow(run, reason+"\n\n"+oldInput); err != nil {
			return err
		}
	}
	if record := s.decisionRecord(run); record != nil {
		record.RecoveryOfDecision = old.ID
		s.markWorkerDecisionWrite(record.DecisionID)
		record.RecoveryVersion = execution.RecoveryVersion
		record.RecoveryState = execution.RecoveryState
	}
	return s.commitWorker(run)
}

func (s *Service) startManuallyRecoveredInitialCodex(ctx context.Context, run *Run, answer DecisionAnswer, commandID string) error {
	if answer.OptionID != "continue" || run.Decision == nil || run.Decision.Request.NativeAsk != nil {
		return errors.New("initial local execution requires an explicit continue decision")
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	execution := fixed.execution
	if !execution.RecoveryRequired || execution.Process != nil || execution.PendingCommandID != "" || execution.CurrentTurnID != "" ||
		execution.TurnNumber != 0 && run.DomainAttemptNumber <= 1 || run.WorkspaceIntent == nil {
		return errors.New("initial worker preparation changed during recovery")
	}
	if s.hasUnknownWorkerOperation(run) {
		return s.waitWorkerFlow(run, "An earlier external operation has an unknown outcome; reconcile it before continuing")
	}
	task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
	if task == nil {
		return errors.New("fixed task is unavailable")
	}
	contract, err := nativeContractForTask(ctx, s.domainStore, fixed.config.identity, *task)
	if err != nil {
		return err
	}
	prompt, err := workerflow.Prompt(*task, contract, run.RunID, fixed.config.PublicationPolicy)
	if err != nil {
		return err
	}
	prompt += "\n\nThe service reopened before the first worker turn. Inspect the current worktree before acting.\n\n" + workerFeedbackText(answer, run.Decision.Request)
	key := "worker-workspace:" + execution.ExecutionID
	var preparation *OperationRecord
	for i := range s.domain.Operations {
		if s.domain.Operations[i].IdempotencyKey == key {
			preparation = &s.domain.Operations[i]
			break
		}
	}
	if preparation != nil && preparation.Status != "confirmed" {
		return s.waitWorkerFlow(run, "Original workspace preparation has an unknown outcome; reconcile it before continuing")
	}
	if execution.Workspace.Worktree == "" {
		if preparation == nil {
			opID, err := s.startWorkerOperation(run, "prepare_workspace", key, struct {
				WorkflowRunID string                       `json:"workflow_run_id"`
				ExecutionID   string                       `json:"execution_id"`
				Workspace     executionapi.WorkspaceIntent `json:"workspace"`
			}{run.RunID, execution.ExecutionID, *run.WorkspaceIntent})
			if err != nil {
				return err
			}
			if _, err := os.Lstat(run.WorkspaceIntent.Path); os.IsNotExist(err) {
				preparation, prepareErr := prepareWorktree(ctx, fixed.config, run)
				if prepareErr != nil {
					status := string(preparation.Status)
					if status != string(executionapi.OperationRejected) {
						status = "outcome_unknown"
					}
					_ = s.changeWorkerOperation(run, opID, status, "", mustJSON(preparation), prepareErr.Error())
					return s.waitWorkerFlow(run, "Original workspace preparation needs reconciliation: "+prepareErr.Error())
				}
			} else if err != nil {
				return err
			}
			workspace, err := s.inspectWorkflowWorkspace(ctx, run)
			if err != nil {
				_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
				return s.waitWorkerFlow(run, "Original workspace identity is unverified: "+err.Error())
			}
			applyWorkflowWorkspaceEvidence(run, execution, workspace)
			if err := s.changeWorkerOperation(run, opID, "confirmed", "", mustJSON(workspace), ""); err != nil {
				return err
			}
		} else {
			workspace, err := s.inspectWorkflowWorkspace(ctx, run)
			if err != nil {
				return s.waitWorkerFlow(run, "Prepared workspace identity is unverified: "+err.Error())
			}
			applyWorkflowWorkspaceEvidence(run, execution, workspace)
		}
	}
	workspace, err := s.inspectWorkflowWorkspace(ctx, run)
	if err != nil {
		return s.waitWorkerFlow(run, "Current worktree identity is unverified: "+err.Error())
	}
	applyWorkflowWorkspaceEvidence(run, execution, workspace)
	s.setDecisionStatus(run, "applied")
	s.setRunDecision(run, nil)
	execution.RecoveryRequired = false
	execution.RecoveryState = "resuming"
	if err := s.commitWorker(run); err != nil {
		return err
	}
	return s.startWorkerFlowTurn(ctx, run, prompt, commandID)
}

// Every service instance must revalidate unfinished executions before a saved
// answer or dispatch intent may write to an external worker. The gate is part
// of the Execution row, so an abrupt process exit has the same startup rule.
func (s *Service) adoptOrGateServiceExecutions() error {
	return s.markServiceRecoveryGateForPriorOwner()
}

func (s *Service) markServiceRecoveryGateForPriorOwner() error {
	for _, run := range s.runs {
		if run == nil || run.WorkflowVersion != WorkerFlowVersion || run.TaskStatus != "in_progress" {
			continue
		}
		workflow, execution := s.workflowRecords(run)
		if workflow == nil || execution == nil || workflow.Phase == "completed" || workflow.Phase == "abandoned" {
			continue
		}
		if execution.OwnerServiceInstanceID != s.State.Service.InstanceID {
			s.markOneServiceRecoveryGate(run)
			execution.OwnerServiceInstanceID = s.State.Service.InstanceID
			s.refreshRun(run)
			if err := s.commitWorker(run); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) markServiceRecoveryGate() error {
	for _, run := range s.runs {
		s.markOneServiceRecoveryGate(run)
		if run != nil && run.TaskStatus == "in_progress" {
			if err := s.commitWorker(run); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) markOneServiceRecoveryGate(run *Run) {
	if run == nil || run.WorkflowVersion != WorkerFlowVersion {
		return
	}
	task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
	workflow, execution := s.workflowRecords(run)
	if task == nil || workflow == nil || execution == nil || task.Status != "in_progress" || workflow.Phase == "completed" || workflow.Phase == "abandoned" {
		return
	}
	execution.RecoveryRequired = true
	execution.RecoveryVersion++
	execution.RecoveryState = "unverified"
	marked := now()
	execution.RecoveryMarkedAt = &marked
	execution.UpdatedAt = marked
	s.refreshRun(run)
}

func (s *Service) resumeConfiguredWork(ctx context.Context) error {
	if err := s.resumeWorkerFlow(ctx); err != nil {
		return err
	}
	for _, run := range s.runs {
		if run == nil || run.WorkflowVersion != WorkerFlowVersion {
			continue
		}
		workflow, execution := s.workflowRecords(run)
		if workflow == nil || execution == nil || workflow.Phase != "completed" {
			continue
		}
		entry, err := s.workerFlowRegistration(run)
		if err != nil {
			return err
		}
		if entry.Cleanup == nil {
			continue
		}
		cleanupErr := entry.Cleanup(s, ctx, run)
		if err := cleanupErr; err != nil {
			execution.LastEvent = ptr("Orca delivery completed; owned terminal cleanup needs reconciliation: " + err.Error())
			s.refreshRun(run)
			if saveErr := s.commitWorker(run); saveErr != nil {
				return saveErr
			}
		}
	}
	return nil
}

func (s *Service) resumeWorkerFlow(ctx context.Context) error {
	if err := s.resumeAppServerWorkers(ctx); err != nil {
		return err
	}
	for _, run := range s.runs {
		if run == nil || run.WorkflowVersion != WorkerFlowVersion || s.workers[run.RunID] != nil {
			continue
		}
		task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
		workflow, execution := s.workflowRecords(run)
		if task == nil || workflow == nil || execution == nil || task.Status != "in_progress" || workflow.Phase == "completed" {
			continue
		}
		adapter, err := s.workerFlowRegistration(run)
		if err != nil {
			return err
		}
		_, err = s.resumeRegisteredWorkerFlow(ctx, task, run, adapter)
		if err != nil {
			return err
		}
	}
	return nil
}
