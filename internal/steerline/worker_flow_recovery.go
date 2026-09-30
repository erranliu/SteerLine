package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

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
	if run.Protocol != string(ProtocolLocalCodexAppServer) || !execution.RecoveryRequired ||
		(execution.RecoveryState != "interrupted" && execution.RecoveryState != "completed") ||
		execution.Process == nil || inspectProcess(execution.Process) != "process absent" {
		return errors.New("the original worker process and its descendants are not confirmed stopped")
	}
	if mode == "continue" && !codexThreadID.MatchString(execution.SessionID) {
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
	var receipt appServerProcessReceipt
	raw, err := os.ReadFile(filepath.Join(appServerDirectory(fixed.config, run), "process.json"))
	if err != nil || json.Unmarshal(raw, &receipt) != nil || !receipt.Managed || receipt.Identity != *execution.Process {
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
	for _, op := range s.State.Operations {
		if op.Status != "outcome_unknown" && op.Status != "in_flight" {
			continue
		}
		if strings.Contains(op.IdempotencyKey, execution.ExecutionID) && !strings.HasPrefix(op.IdempotencyKey, "worker-turn:"+execution.ExecutionID+":") {
			return errors.New("an earlier external operation has an unknown outcome; reconcile it before continuing")
		}
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
	s.State.Attempts = append(s.State.Attempts, StepAttemptRecord{
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
	return s.save()
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
	for _, op := range s.State.Operations {
		if op.IdempotencyKey != "appserver-answer:"+execution.ExecutionID+":"+old.ID {
			continue
		}
		if op.Status == "confirmed" {
			s.setDecisionStatus(run, "reply_accepted")
			return s.save()
		}
		if op.Status == "in_flight" || op.Status == "accepted" || op.Status == "outcome_unknown" {
			execution.RecoveryState = "unverified"
			return s.waitWorkerFlow(run, "Original answer send has an unknown outcome; reconcile its native result before confirming again")
		}
	}
	if s.domainStore != nil && (old.Request.OrcaAsk != nil || run.Protocol == string(ProtocolOrcaTerminalV2)) {
		oldAnswer := DecisionAnswer{DecisionID: old.ID, RunID: run.RunID, OptionID: old.OptionID, Note: old.Note, Answers: old.Answers, SourceAttemptID: old.SourceAttemptID, ExpectedRecoveryVersion: old.RecoveryVersion}
		if command, err := s.domainStore.CommandByIdempotency(context.Background(), s.Config.identity, decisionAnswerKey(oldAnswer)); err == nil {
			for _, op := range s.State.Operations {
				if op.IdempotencyKey == "orca-native-reply:"+execution.ExecutionID+":"+command.CommandID ||
					op.IdempotencyKey == "orca-native-send:"+execution.ExecutionID+":"+command.CommandID {
					if op.Status == "confirmed" {
						s.setDecisionStatus(run, "reply_accepted")
						return s.save()
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
	if err := s.save(); err != nil {
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
	}
	return s.save()
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
	task := nativeTaskRecord(s.State.Tasks, run.DomainTaskID)
	if task == nil {
		return errors.New("fixed task is unavailable")
	}
	contract, err := nativeContractForTask(ctx, s.domainStore, fixed.config.identity, *task)
	if err != nil {
		return err
	}
	prompt, err := workerflow.Prompt(*task, contract, run.RunID, true)
	if err != nil {
		return err
	}
	prompt += "\n\nThe service reopened before the first worker turn. Inspect the current worktree before acting.\n\n" + workerFeedbackText(answer, run.Decision.Request)
	key := "worker-workspace:" + execution.ExecutionID
	var preparation *OperationRecord
	for i := range s.State.Operations {
		if s.State.Operations[i].IdempotencyKey == key {
			preparation = &s.State.Operations[i]
			break
		}
	}
	if preparation != nil && preparation.Status != "confirmed" {
		return s.waitWorkerFlow(run, "Original workspace preparation has an unknown outcome; reconcile it before continuing")
	}
	if execution.Workspace.Worktree == "" {
		if preparation == nil {
			opID, err := s.startOperation("prepare_workspace", key, struct {
				WorkflowRunID string          `json:"workflow_run_id"`
				ExecutionID   string          `json:"execution_id"`
				Workspace     WorkspaceIntent `json:"workspace"`
			}{run.RunID, execution.ExecutionID, *run.WorkspaceIntent})
			if err != nil {
				return err
			}
			if _, err := os.Lstat(run.WorkspaceIntent.Path); os.IsNotExist(err) {
				if err := prepareWorktreeOnly(ctx, fixed.config, run); err != nil {
					_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
					return s.waitWorkerFlow(run, "Original workspace preparation needs reconciliation: "+err.Error())
				}
			} else if err != nil {
				return err
			}
			workspace, err := s.inspectWorkflowWorkspace(ctx, run)
			if err != nil {
				_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
				return s.waitWorkerFlow(run, "Original workspace identity is unverified: "+err.Error())
			}
			applyWorkflowWorkspaceEvidence(run, execution, workspace)
			if err := s.changeOperation(opID, "confirmed", "", mustJSON(workspace), ""); err != nil {
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
	if err := s.save(); err != nil {
		return err
	}
	return s.startWorkerFlowTurn(ctx, run, prompt, commandID)
}
