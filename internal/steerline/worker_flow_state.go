package steerline

import (
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

var ErrWorkerTurnStale = workerflow.ErrTurnStale

func workerFlowState(workflow *WorkflowRunRecord, execution *ExecutionRecord) workerflow.State {
	return workerflow.State{
		WorkflowRunID: workflow.WorkflowRunID, TaskID: workflow.TaskID, WorkflowVersion: workflow.WorkflowVersion,
		Phase: workflow.Phase, Branch: workflow.Branch, PRURL: workflow.PRURL, PRState: workflow.PRState,
		WorkflowUpdatedAt: workflow.UpdatedAt, ExecutionID: execution.ExecutionID,
		ExecutionWorkflowRunID: execution.WorkflowRunID, SessionID: execution.SessionID,
		PendingTurnID: execution.PendingTurnID, CurrentTurnID: execution.CurrentTurnID,
		LastStoppedTurnID: execution.LastStoppedTurnID, ExecutionStatus: execution.Status,
		TurnNumber: execution.TurnNumber, ExecutionUpdatedAt: execution.UpdatedAt,
	}
}

func applyWorkerFlowState(workflow *WorkflowRunRecord, execution *ExecutionRecord, state workerflow.State) {
	workflow.Phase, workflow.PRURL, workflow.PRState, workflow.UpdatedAt =
		state.Phase, state.PRURL, state.PRState, state.WorkflowUpdatedAt
	execution.SessionID, execution.PendingTurnID, execution.CurrentTurnID =
		state.SessionID, state.PendingTurnID, state.CurrentTurnID
	execution.LastStoppedTurnID, execution.Status, execution.TurnNumber, execution.UpdatedAt =
		state.LastStoppedTurnID, state.ExecutionStatus, state.TurnNumber, state.ExecutionUpdatedAt
}

func workerFlowBinding(workflow *WorkflowRunRecord, execution *ExecutionRecord) error {
	if workflow == nil || execution == nil {
		return errors.New("worker flow has no fixed workflow and execution binding")
	}
	return workerflow.ValidateBinding(workerFlowState(workflow, execution))
}

// BeginWorkerTurn records a platform-proven turn start for the original session.
func BeginWorkerTurn(workflow *WorkflowRunRecord, execution *ExecutionRecord, sessionID, turnID string) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	next, err := workerflow.BeginTurn(workerFlowState(workflow, execution), sessionID, turnID, time.Now().UTC())
	if err == nil {
		applyWorkerFlowState(workflow, execution, next)
	}
	return err
}

func NoteWorkerQuestion(workflow *WorkflowRunRecord, execution *ExecutionRecord, turnID string) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	next, err := workerflow.NoteQuestion(workerFlowState(workflow, execution), turnID, time.Now().UTC())
	if err == nil {
		applyWorkerFlowState(workflow, execution, next)
	}
	return err
}

func ContinueWorkerQuestion(workflow *WorkflowRunRecord, execution *ExecutionRecord, turnID string) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	next, err := workerflow.ContinueQuestion(workerFlowState(workflow, execution), turnID, time.Now().UTC())
	if err == nil {
		applyWorkerFlowState(workflow, execution, next)
	}
	return err
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
	next, err := workerflow.StopTurn(workerFlowState(workflow, execution), turnID, pendingQuestion, time.Now().UTC())
	if err == nil {
		applyWorkerFlowState(workflow, execution, next)
	}
	return err
}

func BindWorkerPR(workflow *WorkflowRunRecord, execution *ExecutionRecord, info PRInfo, candidateURL, expectedRepository string, pendingQuestion bool) error {
	if err := workerFlowBinding(workflow, execution); err != nil {
		return err
	}
	if err := validateWorkerPRIdentity(info, candidateURL, expectedRepository, workflow.Branch, execution.Workspace.Branch); err != nil {
		return err
	}
	next, err := workerflow.BindPR(workerFlowState(workflow, execution), info.URL, info.State, pendingQuestion, time.Now().UTC())
	if err == nil {
		applyWorkerFlowState(workflow, execution, next)
	}
	return err
}

func MarkWorkerDeliveryCommitted(task TaskRecord, workflow *WorkflowRunRecord, delivery TaskDeliveryRecord) error {
	if workflow == nil {
		return errors.New("completed worker flow requires committed matching delivery")
	}
	state := workerflow.State{
		WorkflowRunID: workflow.WorkflowRunID, TaskID: workflow.TaskID, WorkflowVersion: workflow.WorkflowVersion,
		Phase: workflow.Phase, PRURL: workflow.PRURL, PRState: workflow.PRState, WorkflowUpdatedAt: workflow.UpdatedAt,
	}
	next, err := workerflow.Complete(state, task.TaskID, task.Status, workerflow.Delivery{
		TaskID: delivery.TaskID, WorkflowRunID: delivery.SelectedWorkflowRunID, PRURL: delivery.PRURL,
		MergeCommit: delivery.MergeCommit, MergedAt: delivery.MergedAt,
	}, time.Now().UTC())
	if err == nil {
		workflow.Phase, workflow.PRState, workflow.UpdatedAt = next.Phase, next.PRState, next.WorkflowUpdatedAt
	}
	return err
}
