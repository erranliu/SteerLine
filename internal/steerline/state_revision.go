package steerline

import (
	"database/sql"
	"encoding/json"
	"errors"
)

func recordFingerprint(value any) string { return operationDigest(mustJSON(value)) }

// Delivery freezes execution evidence; only subsequent diagnostic notes may change.
func deliveredExecutionFingerprint(v ExecutionRecord) string {
	v.Error, v.LastEvent, v.Summary = nil, nil, nil
	v.UpdatedAt = v.CreatedAt
	return recordFingerprint(v)
}

func controlFingerprint(value any) string {
	switch v := value.(type) {
	case WorkflowRunRecord:
		return recordFingerprint(struct {
			Phase, Selected, PRURL, PRState string
			Pending                         int
		}{v.Phase, v.SelectedResult, v.PRURL, v.PRState, v.PendingDecisions})
	case ExecutionRecord:
		return recordFingerprint(struct {
			Status, Outcome, Session, Decision, Step                              string
			PendingCommand, CurrentCommand, PendingTurn, CurrentTurn, StoppedTurn string
			TurnNumber                                                            uint64
			Process                                                               *ProcessIdentity
			Target                                                                ExecutionTarget
			Intent                                                                *WorkspaceIntent
			Worktree, Branch, Base, Endpoint, Generation                          string
			Asks                                                                  []codexAskCall
			ManualStop                                                            *ManualStopConfirmation
		}{v.Status, v.LastTurnOutcome, v.SessionID, v.DecisionID, v.CurrentStep,
			v.PendingCommandID, v.CurrentCommandID, v.PendingTurnID, v.CurrentTurnID, v.LastStoppedTurnID, v.TurnNumber,
			v.Process, v.Target, v.WorkspaceIntent, v.Workspace.Worktree, v.Workspace.Branch, v.Workspace.Base, v.AppServerEndpoint, v.AppServerGeneration, v.PendingAsks, v.ManualStop})
	case DecisionRecord:
		return recordFingerprint(struct {
			ID, Workflow, Execution, Attempt, Question, Status string
			Version                                            uint64
			Request                                            json.RawMessage
		}{v.DecisionID, v.WorkflowRunID, v.ExecutionID, v.AttemptID, v.QuestionID, v.Status, v.Version, v.Request})
	}
	return ""
}

func (t *sqliteTx) advanceControlRevision(table, key, project, id, workflow string, value any) error {
	var body []byte
	err := t.tx.QueryRowContext(t.ctx, "SELECT payload FROM "+table+" WHERE project_identity=? AND "+key+"=?", project, id).Scan(&body)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		var previous any
		switch value.(type) {
		case WorkflowRunRecord:
			previous = &WorkflowRunRecord{}
		case ExecutionRecord:
			previous = &ExecutionRecord{}
		case DecisionRecord:
			previous = &DecisionRecord{}
		}
		if err = json.Unmarshal(body, previous); err != nil {
			return err
		}
		switch p := previous.(type) {
		case *WorkflowRunRecord:
			previous = *p
		case *ExecutionRecord:
			previous = *p
		case *DecisionRecord:
			previous = *p
		}
		if controlFingerprint(previous) == controlFingerprint(value) {
			return nil
		}
	}
	_, err = t.tx.ExecContext(t.ctx, `UPDATE workflow_runs SET control_revision=control_revision+1 WHERE project_identity=? AND workflow_run_id=?`, project, workflow)
	return err
}
