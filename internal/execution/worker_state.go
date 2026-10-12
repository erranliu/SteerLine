package execution

import (
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
)

func workerFlowState(workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord, sessionOverride ...string) executionmodel.State {
	sessionID, _ := executionSessionFacts(execution)
	if len(sessionOverride) > 0 && sessionOverride[0] != "" {
		sessionID = sessionOverride[0]
	}
	return executionmodel.State{
		WorkflowRunID: workflow.WorkflowRunID, TaskID: workflow.TaskID, WorkflowVersion: workflow.WorkflowVersion,
		Status: workflow.Status, PRURL: workflow.PRURL, PRState: workflow.PRState,
		WorkflowUpdatedAt: workflow.UpdatedAt, ExecutionID: execution.ExecutionID,
		ExecutionWorkflowRunID: execution.WorkflowRunID, SessionID: sessionID,
		PendingTurnID: execution.PendingTurnID, CurrentTurnID: execution.CurrentTurnID,
		LastStoppedTurnID: execution.LastStoppedTurnID, ExecutionStatus: execution.Status,
		TurnNumber: execution.TurnNumber, ExecutionUpdatedAt: execution.UpdatedAt,
	}
}

func applyWorkerFlowState(workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord, state executionmodel.State) {
	workflow.Status, workflow.PRURL, workflow.PRState, workflow.UpdatedAt =
		state.Status, state.PRURL, state.PRState, state.WorkflowUpdatedAt
	execution.SessionID = state.SessionID
	execution.PendingTurnID, execution.CurrentTurnID = state.PendingTurnID, state.CurrentTurnID
	execution.LastStoppedTurnID, execution.Status, execution.TurnNumber, execution.UpdatedAt =
		state.LastStoppedTurnID, state.ExecutionStatus, state.TurnNumber, state.ExecutionUpdatedAt
}

func workerFlowBinding(workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord) error {
	if workflow == nil || execution == nil {
		return errors.New("worker flow has no fixed workflow and execution binding")
	}
	return executionmodel.ValidateBinding(workerFlowState(workflow, execution))
}

// BeginWorkerTurn records a platform-proven turn start for the original session.
func BeginWorkerTurn(workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord, sessionID, turnID string) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	next, err := executionmodel.BeginTurn(workerFlowState(workflow, execution, sessionID), sessionID, turnID, time.Now().UTC())
	if err == nil {
		applyWorkerFlowState(workflow, execution, next)
	}
	return err
}

func NoteWorkerQuestion(workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord, turnID string) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	next, err := executionmodel.NoteQuestion(workerFlowState(workflow, execution), turnID, time.Now().UTC())
	if err == nil {
		applyWorkerFlowState(workflow, execution, next)
	}
	return err
}

func ContinueWorkerQuestion(workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord, turnID string) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	next, err := executionmodel.ContinueQuestion(workerFlowState(workflow, execution), turnID, time.Now().UTC())
	if err == nil {
		applyWorkerFlowState(workflow, execution, next)
	}
	return err
}

// BeginWorkerFeedbackDelivery moves a saved answer for an open PR into the
// feedback phase before the original worker session receives another turn.
func BeginWorkerFeedbackDelivery(workflow *executionmodel.WorkflowRunRecord, execution executionmodel.ExecutionRecord, decision executionmodel.DecisionRecord, at time.Time) error {
	if err := workerFlowBinding(workflow, &execution); err != nil {
		return err
	}
	if workflow.Status != "waiting_external" || workflow.SelectedResult != execution.ExecutionID ||
		workflow.ProjectIdentity != execution.ProjectIdentity || workflow.ProjectIdentity != decision.ProjectIdentity ||
		execution.DecisionID == "" || execution.DecisionID != decision.DecisionID ||
		decision.Scope != "execution" || decision.WorkflowRunID != workflow.WorkflowRunID ||
		decision.ExecutionID != execution.ExecutionID || execution.CurrentStep != "worker" || decision.AttemptID == "" ||
		decision.Status != "answer_saved" || decision.Version == 0 || at.IsZero() {
		return errors.New("saved worker feedback no longer belongs to the fixed PR execution")
	}
	workflow.Status, workflow.UpdatedAt = "waiting_input", at
	return nil
}

// StopWorkerTurn accepts only the current turn's terminal observation. The
// caller passes whether a Decision for this turn is still pending; this fact is
// read from the existing Decision authority, not copied into Execution.
func StopWorkerTurn(workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord, turnID string, pendingQuestion bool) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	next, err := executionmodel.StopTurn(workerFlowState(workflow, execution), turnID, pendingQuestion, time.Now().UTC())
	if err == nil {
		applyWorkerFlowState(workflow, execution, next)
	}
	return err
}

func BindWorkerPR(workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord, info executionmodel.PRInfo, candidateURL, expectedRepository string, pendingQuestion bool) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	if err := publication.ValidateWorkerPRIdentity(info, candidateURL, expectedRepository, workflow.Branch, execution.Workspace.Branch); err != nil {
		return err
	}
	next, err := executionmodel.BindPR(workerFlowState(workflow, execution), info.URL, info.State, pendingQuestion, time.Now().UTC())
	if err == nil {
		applyWorkerFlowState(workflow, execution, next)
	}
	return err
}

// Every platform reaches the same business transition after its own adapter
// has verified the original turn's terminal evidence.
func (s *Service) stopVerifiedWorkerTurn(run *reconstructiontypes.RunView, workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord, turnID, outcome string, question bool) error {
	if err := StopWorkerTurn(workflow, execution, turnID, question); err != nil {
		return err
	}
	execution.TurnEndedAt = ptr(now())
	execution.LastTurnOutcome = outcome
	if execution.RecoveryRequired {
		available := execution.RecoverySessionAvailable != nil && *execution.RecoverySessionAvailable
		s.setWorkerRecoveryFacts(run, executionmodel.RecoveryFacts{Completed: true}, available)
	}
	s.refreshRun(run)
	return nil
}

type stoppedWorkerResolution struct {
	TurnID       string
	LastMessage  string
	Issue        string
	Question     bool
	WaitReason   string
	RecordPRStop bool
}

func (s *Service) resolveStoppedWorkerTurn(run *reconstructiontypes.RunView, workflow *executionmodel.WorkflowRunRecord, execution *executionmodel.ExecutionRecord, resolution stoppedWorkerResolution) error {
	if workflow.Status == "waiting_external" {
		if err := s.offerWorkerPRFeedback(run, workflow.PRURL); err != nil {
			return err
		}
		if resolution.Issue != "" {
			execution.Error = ptr(clip(resolution.Issue, 2000))
			s.refreshRun(run)
		}
		if resolution.RecordPRStop {
			s.note(run, "steer", "worker", "stopped", "worker turn ended with a verified PR", workflow.PRURL, map[string]any{"turn_id": resolution.TurnID})
		}
		return s.commitWorker(run)
	}
	context := resolution.WaitReason
	if context == "" {
		context = strings.TrimSpace(resolution.LastMessage)
		if context == "" {
			context = "The worker stopped without a final reply."
		}
		if resolution.Issue != "" {
			context += "\n\nStop reason: " + resolution.Issue
		}
	}
	if resolution.Question {
		decision, err := WriteDecisionRequest(run, executionmodel.DecisionRequest{Question: clip(strings.TrimSpace(resolution.LastMessage), 1800), Context: clip(context, 7000), Options: []executionbackend.DecisionOption{{ID: "continue", Label: "Continue current work"}}, AllowFreeText: true})
		if err == nil {
			s.setRunDecision(run, decision)
		}
	}
	return s.waitWorkerFlow(run, context)
}
