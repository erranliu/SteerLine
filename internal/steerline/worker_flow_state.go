package steerline

import (
	"errors"
	"fmt"
	"time"
)

var ErrWorkerTurnStale = errors.New("worker turn is no longer current")

func workerFlowBinding(workflow *WorkflowRunRecord, execution *ExecutionRecord) error {
	if workflow == nil || execution == nil || workflow.WorkflowVersion != WorkerFlowVersion ||
		workflow.WorkflowRunID == "" || execution.ExecutionID == "" || execution.WorkflowRunID != workflow.WorkflowRunID {
		return errors.New("worker flow has no fixed workflow and execution binding")
	}
	if workflow.Phase == "completed" || workflow.Phase == "abandoned" {
		return errors.New("terminal worker flow cannot resume")
	}
	return nil
}

// BeginWorkerTurn records a platform-proven turn start. SessionID identifies
// the logical conversation and must never be changed by a process restart.
func BeginWorkerTurn(workflow *WorkflowRunRecord, execution *ExecutionRecord, sessionID, turnID string) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	if sessionID == "" || turnID == "" {
		return errors.New("worker turn start lacks native session or turn identity")
	}
	if execution.PendingTurnID != "" && execution.PendingTurnID != turnID {
		return errors.New("worker turn start differs from the persisted send intent")
	}
	if execution.SessionID != "" && execution.SessionID != sessionID {
		return errors.New("worker session replacement is not an ordinary continuation")
	}
	if execution.CurrentTurnID == turnID {
		if execution.LastStoppedTurnID == turnID {
			return ErrWorkerTurnStale
		}
		return nil
	}
	if execution.CurrentTurnID != "" && execution.LastStoppedTurnID != execution.CurrentTurnID {
		return errors.New("previous worker turn has not stopped")
	}
	switch workflow.Phase {
	case "queued", "waiting_decision", "waiting_pr_merge":
	default:
		return fmt.Errorf("worker turn cannot start from %s", workflow.Phase)
	}
	execution.SessionID = sessionID
	execution.PendingTurnID = ""
	execution.CurrentTurnID = turnID
	execution.TurnNumber++
	execution.Status = "active"
	execution.UpdatedAt = time.Now().UTC()
	workflow.Phase = "executing"
	workflow.UpdatedAt = execution.UpdatedAt
	return nil
}

// NoteWorkerQuestion gives an unanswered question precedence over an open PR.
// A question does not assert that the platform turn has stopped.
func NoteWorkerQuestion(workflow *WorkflowRunRecord, execution *ExecutionRecord, turnID string) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	if turnID == "" || execution.CurrentTurnID != turnID || execution.LastStoppedTurnID == turnID {
		return ErrWorkerTurnStale
	}
	if workflow.Phase != "executing" && workflow.Phase != "waiting_decision" {
		return fmt.Errorf("worker question cannot arrive in %s", workflow.Phase)
	}
	workflow.Phase = "waiting_decision"
	workflow.UpdatedAt = time.Now().UTC()
	return nil
}

// ContinueWorkerQuestion records acceptance of feedback by a still-running
// platform turn. It does not invent a second turn or replace the session.
func ContinueWorkerQuestion(workflow *WorkflowRunRecord, execution *ExecutionRecord, turnID string) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	if turnID == "" || execution.CurrentTurnID != turnID || execution.LastStoppedTurnID == turnID {
		return ErrWorkerTurnStale
	}
	if workflow.Phase != "waiting_decision" {
		return fmt.Errorf("worker question is not waiting in %s", workflow.Phase)
	}
	workflow.Phase = "executing"
	workflow.UpdatedAt = time.Now().UTC()
	return nil
}

// BeginWorkerFeedbackDelivery moves a saved answer for an open PR into the
// feedback phase before the original worker session receives another turn.
func BeginWorkerFeedbackDelivery(workflow *WorkflowRunRecord, execution ExecutionRecord, decision DecisionRecord, at time.Time) error {
	if err := workerFlowBinding(workflow, &execution); err != nil {
		return err
	}
	if workflow.Phase != "waiting_pr_merge" || workflow.SelectedResult != execution.ExecutionID ||
		workflow.ProjectIdentity != execution.ProjectIdentity || workflow.ProjectIdentity != decision.ProjectIdentity ||
		execution.DecisionID == "" || execution.DecisionID != decision.DecisionID ||
		decision.Scope != "execution" || decision.WorkflowRunID != workflow.WorkflowRunID ||
		decision.ExecutionID != execution.ExecutionID || execution.CurrentStep != "worker" || decision.AttemptID == "" ||
		decision.Status != "answer_saved" || decision.Version == 0 || at.IsZero() {
		return errors.New("saved worker feedback no longer belongs to the fixed PR execution")
	}
	workflow.Phase, workflow.UpdatedAt = "waiting_decision", at
	return nil
}

// StopWorkerTurn accepts only the current turn's terminal observation. The
// caller passes whether a Decision for this turn is still pending; this fact is
// read from the existing Decision authority, not copied into Execution.
func StopWorkerTurn(workflow *WorkflowRunRecord, execution *ExecutionRecord, turnID string, pendingQuestion bool) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	if turnID == "" || execution.CurrentTurnID != turnID {
		return ErrWorkerTurnStale
	}
	if execution.LastStoppedTurnID == turnID {
		return nil
	}
	if workflow.Phase != "executing" && workflow.Phase != "waiting_decision" {
		return fmt.Errorf("worker turn cannot stop from %s", workflow.Phase)
	}
	execution.LastStoppedTurnID = turnID
	execution.Status = "idle"
	execution.UpdatedAt = time.Now().UTC()
	if pendingQuestion || workflow.PRURL == "" || workflow.PRState != "open" && workflow.PRState != "merged" {
		workflow.Phase = "waiting_decision"
	} else {
		workflow.Phase = "waiting_pr_merge"
	}
	workflow.UpdatedAt = execution.UpdatedAt
	return nil
}

// BindWorkerPR receives a PR only after the provider and execution branch have
// passed identity validation. A closed unmerged PR may be superseded; its old
// URL remains in the domain event history.
func BindWorkerPR(workflow *WorkflowRunRecord, execution *ExecutionRecord, info PRInfo, candidateURL, expectedRepository string, pendingQuestion bool) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	if err := validateWorkerPRIdentity(info, candidateURL, expectedRepository, workflow.Branch, execution.Workspace.Branch); err != nil {
		return err
	}
	if workflow.PRURL != "" && workflow.PRURL != info.URL && workflow.PRState != "closed_unmerged" {
		return errors.New("another PR is already bound to this execution")
	}
	workflow.PRURL, workflow.PRState = info.URL, info.State
	if execution.CurrentTurnID != "" && execution.LastStoppedTurnID != execution.CurrentTurnID {
		// A PR can be published while the worker is still working.
		if workflow.Phase != "waiting_decision" || !pendingQuestion {
			workflow.Phase = "executing"
		}
	} else if pendingQuestion || info.State == "closed_unmerged" {
		workflow.Phase = "waiting_decision"
	} else {
		workflow.Phase = "waiting_pr_merge"
	}
	workflow.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkWorkerDeliveryCommitted runs only after the atomic TaskDelivery/Task
// transaction has succeeded. A late worker event cannot reopen the workflow.
func MarkWorkerDeliveryCommitted(task TaskRecord, workflow *WorkflowRunRecord, delivery TaskDeliveryRecord) error {
	if workflow == nil || workflow.WorkflowVersion != WorkerFlowVersion || task.Status != "closed" ||
		task.TaskID != workflow.TaskID || delivery.TaskID != task.TaskID ||
		delivery.SelectedWorkflowRunID != workflow.WorkflowRunID || delivery.PRURL == "" ||
		workflow.PRURL != delivery.PRURL || delivery.MergeCommit == "" || delivery.MergedAt.IsZero() {
		return errors.New("completed worker flow requires committed matching delivery")
	}
	if workflow.Phase == "completed" {
		return nil
	}
	if workflow.Phase == "abandoned" {
		return errors.New("discarded worker flow cannot become completed")
	}
	workflow.Phase = "completed"
	workflow.PRState = "merged"
	workflow.UpdatedAt = time.Now().UTC()
	return nil
}
