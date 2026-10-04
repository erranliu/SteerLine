package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func recordFingerprint(value any) string { return operationDigest(mustJSON(value)) }

// Delivery freezes execution evidence; only subsequent diagnostic notes may change.
func deliveredExecutionFingerprint(v executionmodel.ExecutionRecord) string {
	v.ResourceStopOperationID, v.ResourceStoppedAt = "", nil
	v.Error, v.LastEvent, v.Summary = nil, nil, nil
	v.UpdatedAt = v.CreatedAt
	return recordFingerprint(v)
}

func controlFingerprint(value any) string {
	switch v := value.(type) {
	case executionmodel.WorkflowRunRecord:
		return recordFingerprint(struct {
			Phase, Selected, PRURL, PRState string
			Pending                         int
			Exit                            any
			History                         []string
		}{v.Phase, v.SelectedResult, v.PRURL, v.PRState, v.PendingDecisions, v.Abandonment, v.PRHistory})
	case executionmodel.ExecutionRecord:
		return recordFingerprint(struct {
			Status, Outcome, Decision, Step                                       string
			PendingCommand, CurrentCommand, PendingTurn, CurrentTurn, StoppedTurn string
			TurnNumber                                                            uint64
			Process                                                               *executionbackend.ProcessIdentity
			Target                                                                executionbackend.ExecutionTarget
			Intent                                                                *executionbackend.WorkspaceIntent
			Worktree, Branch, Base, Generation, ConfigurationDigest               string
			ImplementationData                                                    []executionbackend.ImplementationDataEnvelope
			PendingQuestions                                                      []executionmodel.PendingQuestion
			ManualStop                                                            *executionmodel.ManualStopConfirmation
			Attempt, Owner, Recovery                                              string
			RecoveryRequired                                                      bool
			RecoveryVersion                                                       uint64
			RecoveryMarkedAt                                                      *time.Time
		}{v.Status, v.LastTurnOutcome, v.DecisionID, v.CurrentStep,
			v.PendingCommandID, v.CurrentCommandID, v.PendingTurnID, v.CurrentTurnID, v.LastStoppedTurnID, v.TurnNumber,
			v.Process, v.Target, v.WorkspaceIntent, v.Workspace.Worktree, v.Workspace.Branch, v.Workspace.Base, v.Generation, v.ConfigurationDigest, v.ImplementationData, v.PendingQuestions, v.ManualStop,
			v.CurrentAttemptID, v.OwnerServiceInstanceID, v.RecoveryState, v.RecoveryRequired, v.RecoveryVersion, v.RecoveryMarkedAt})
	case executionmodel.DecisionRecord:
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
		case executionmodel.WorkflowRunRecord:
			previous = &executionmodel.WorkflowRunRecord{}
		case executionmodel.ExecutionRecord:
			previous = &executionmodel.ExecutionRecord{}
		case executionmodel.DecisionRecord:
			previous = &executionmodel.DecisionRecord{}
		}
		if err = json.Unmarshal(body, previous); err != nil {
			return err
		}
		switch p := previous.(type) {
		case *executionmodel.WorkflowRunRecord:
			previous = *p
		case *executionmodel.ExecutionRecord:
			previous = *p
		case *executionmodel.DecisionRecord:
			previous = *p
		}
		if controlFingerprint(previous) == controlFingerprint(value) {
			return nil
		}
	}
	_, err = t.tx.ExecContext(t.ctx, `UPDATE workflow_runs SET control_revision=control_revision+1 WHERE project_identity=? AND workflow_run_id=?`, project, workflow)
	return err
}
