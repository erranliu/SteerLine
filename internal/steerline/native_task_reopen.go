package steerline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

// TaskReopenInput abandons one stopped, unpublished native implement attempt.
// The old workflow and its evidence remain readable under their original IDs.
type TaskReopenInput struct {
	TaskID                  string
	WorkflowRunID           string
	RequestID               string
	ExpectedVersion         uint64
	ExpectedGraphRevision   *uint64
	PredecessorIDs          []string
	ConfirmedProcessStopped bool
	Reason                  string
	RequestedBy             string
}

type TaskReopenReceipt struct {
	RequestID      string `json:"request_id"`
	TaskID         string `json:"task_id"`
	TaskVersion    uint64 `json:"task_version"`
	GraphRevision  uint64 `json:"graph_revision"`
	AbandonedRunID string `json:"abandoned_run_id"`
	Status         string `json:"status"`
}

func (c CommandService) ReopenNativeTask(ctx context.Context, config Config, input TaskReopenInput) (TaskReopenReceipt, error) {
	var empty TaskReopenReceipt
	store, ok := c.Store.(*SQLiteStore)
	if !ok || c.Project == "" || c.Project != config.identity || config.TaskAuthority != "native" {
		return empty, nativeErr("authority_mismatch", "native task reopen requires the fixed center SQLite authority")
	}
	if err := ValidateNativeTaskID(input.TaskID); err != nil {
		return empty, err
	}
	if !stableExecutionID(input.WorkflowRunID) || !controlIDPattern.MatchString(input.RequestID) || input.ExpectedVersion == 0 || strings.TrimSpace(input.RequestedBy) == "" || !input.ConfirmedProcessStopped {
		return empty, nativeErr("contract_mismatch", "reopen requires the exact run, request, task version, operator and stopped-process confirmation")
	}
	reason, err := validateConfirmationBasis(input.Reason)
	if err != nil {
		return empty, err
	}
	request, _ := CanonicalJSON(mustJSON(struct {
		TaskID         string   `json:"task_id"`
		RunID          string   `json:"workflow_run_id"`
		TaskVersion    uint64   `json:"expected_version"`
		GraphRevision  *uint64  `json:"expected_graph_revision,omitempty"`
		PredecessorIDs []string `json:"predecessor_ids,omitempty"`
		Stopped        bool     `json:"confirmed_process_stopped"`
		Reason         string   `json:"reason"`
		RequestedBy    string   `json:"requested_by"`
	}{input.TaskID, input.WorkflowRunID, input.ExpectedVersion, input.ExpectedGraphRevision, input.PredecessorIDs, true, reason, input.RequestedBy}))
	if previous, lookupErr := store.CommandByIdempotency(ctx, c.Project, input.RequestID); lookupErr == nil {
		if previous.Kind != "reopen_task" || previous.TargetID != input.TaskID || previous.ExpectedVersion != input.ExpectedVersion || previous.RequestedBy != input.RequestedBy || !bytes.Equal(previous.Payload, request) || previous.Status != "accepted" {
			return empty, nativeErr("idempotency_conflict", "reopen request ID belongs to a different or incomplete command")
		}
		var receipt TaskReopenReceipt
		if err := json.Unmarshal(previous.Result, &receipt); err != nil {
			return empty, err
		}
		return receipt, nil
	} else if !errors.Is(lookupErr, sql.ErrNoRows) {
		return empty, lookupErr
	}
	if len(input.PredecessorIDs) != 0 {
		authority, authorityErr := store.nativeAuthority(ctx, c.Project)
		if authorityErr != nil {
			return empty, authorityErr
		}
		if err := validateNativeConfiguredTarget(ctx, config, authority); err != nil {
			return empty, err
		}
	}

	projection, revision, err := store.LoadProjection(ctx, c.Project)
	if err != nil || len(projection) == 0 {
		return empty, errors.Join(err, nativeErr("inconsistent_delivery", "native runtime projection is missing"))
	}
	var runtimeState Runtime
	if err = json.Unmarshal(projection, &runtimeState); err != nil || runtimeState.SchemaVersion != 5 || runtimeState.Repository.Identity != c.Project || runtimeState.Control.Operation != nil {
		return empty, nativeErr("unsupported_schema", "reopen requires a settled current native runtime projection")
	}
	var run *Run
	for _, candidate := range runtimeState.Runs {
		if candidate != nil && candidate.RunID == input.WorkflowRunID {
			run = candidate
		} else if candidate != nil && (candidate.ExecutionStatus == "unknown" || candidate.ExecutionStatus == "pending" || candidate.ExecutionStatus == "running" || candidate.FinishTaskStatus == "waiting_decision") {
			return empty, nativeErr("slot_busy", "another run still holds an execution reservation")
		}
	}
	if run == nil || runtimeState.CurrentTask == nil || *runtimeState.CurrentTask != input.TaskID || !runtimeState.SlotOccupied || run.TaskID != input.TaskID || run.DomainTaskID != input.TaskID || run.WorkflowVersion != MinimalWorkflowVersion || run.ExecutionStatus != "unknown" || run.FinishTaskStatus != "blocked" || run.Process == nil || run.Process.Kind != "orca" || run.Process.Terminal == "" || run.Process.RuntimeID == "" || run.Process.Incarnation == "" || run.WorktreePath == nil || deref(run.PR.URL) != "" {
		return empty, nativeErr("immutable_task", "reopen requires the exact blocked, unpublished local Orca run")
	}
	if _, statErr := os.Lstat(*run.WorktreePath); !os.IsNotExist(statErr) {
		return empty, nativeErr("cleanup_pending", "original worktree must be removed before reopening the task")
	}
	var executionConfig Config
	for _, workflow := range runtimeState.WorkflowRuns {
		if workflow.WorkflowRunID == run.RunID && workflow.ExecutionProfile != nil {
			if !workflow.ExecutionProfile.Verify(workflow.ProjectSteps) {
				return empty, nativeErr("contract_mismatch", "old workflow execution profile does not match its pinned steps")
			}
			executionConfig = config.ApplyFixedExecutionProfile(*workflow.ExecutionProfile, workflow.ProjectSteps)
			break
		}
	}
	if executionConfig.hostName() != "orca" {
		return empty, nativeErr("contract_mismatch", "old workflow has no pinned local Orca execution profile")
	}
	term, showErr := (orcaClient{cfg: executionConfig}).showTerminal(ctx, run.Process.Terminal)
	if showErr != nil {
		diagnostic := strings.ToLower(showErr.Error())
		if !strings.Contains(diagnostic, "terminal_handle_stale") && !strings.Contains(diagnostic, "terminal handle stale") {
			return empty, nativeErr("cleanup_pending", "original Orca terminal is not verifiably absent: "+showErr.Error())
		}
	} else if term.Handle != run.Process.Terminal || term.RuntimeID != run.Process.RuntimeID || term.IncarnationID != run.Process.Incarnation || !term.exited() {
		return empty, nativeErr("cleanup_pending", "original Orca terminal still exists or changed identity, including a detached or orphaned tab")
	}

	var receipt TaskReopenReceipt
	err = store.Transact(ctx, func(domainTx DomainTx) error {
		tx := domainTx.(*sqliteTx)
		var currentRevision uint64
		if e := tx.tx.QueryRowContext(ctx, `SELECT revision FROM projection WHERE project_identity=?`, c.Project).Scan(&currentRevision); e != nil || currentRevision != revision {
			return nativeErr("stale_version", "native runtime projection changed before reopen")
		}
		graph, e := taskGraphSnapshotTx(ctx, tx, c.Project)
		if e != nil {
			return e
		}
		if input.ExpectedGraphRevision != nil && graph.Authority.GraphRevision != *input.ExpectedGraphRevision {
			return nativeErr("stale_version", "native graph changed before reopen")
		}
		if len(input.PredecessorIDs) != 0 {
			if e = validateNativePredecessorAddition(graph, input.TaskID, input.PredecessorIDs); e != nil {
				return e
			}
		}
		task, found := graph.ByID[input.TaskID]
		if !found || task.Status != "in_progress" || task.Version != input.ExpectedVersion || task.Claimant == "" || nativeDeliveryInconsistent(task, graph) {
			return nativeErr("stale_version", "native task claim or delivery changed before reopen")
		}
		if _, delivered := graph.Deliveries[input.TaskID]; delivered {
			return nativeErr("already_merged", "delivered task cannot be reopened")
		}
		snapshot, e := querySnapshot(ctx, tx.tx, c.Project)
		if e != nil {
			return e
		}
		var workflow *WorkflowRunRecord
		for i := range snapshot.WorkflowRuns {
			candidate := &snapshot.WorkflowRuns[i]
			if candidate.TaskID == input.TaskID && candidate.Phase != "completed" && candidate.Phase != "abandoned" {
				if workflow != nil || candidate.WorkflowRunID != input.WorkflowRunID {
					return nativeErr("immutable_task", "task has another active workflow")
				}
				workflow = candidate
			}
		}
		if workflow == nil || workflow.Phase != "blocked" || workflow.SelectedResult != "" || workflow.PendingDecisions != 0 {
			return nativeErr("immutable_task", "old workflow is not an unpublished blocked execution")
		}
		var execution *ExecutionRecord
		for i := range snapshot.Executions {
			candidate := &snapshot.Executions[i]
			if candidate.WorkflowRunID == workflow.WorkflowRunID {
				if execution != nil || candidate.ExecutionID != run.ExecutionID {
					return nativeErr("immutable_task", "old workflow has another execution")
				}
				execution = candidate
			}
		}
		if execution == nil || execution.Status != "blocked" || execution.CurrentStep != "implement" || execution.Process == nil || *execution.Process != *run.Process {
			return nativeErr("immutable_task", "old implement execution or process binding changed")
		}
		var attempt *StepAttemptRecord
		for i := range snapshot.Attempts {
			candidate := &snapshot.Attempts[i]
			if candidate.ExecutionID != execution.ExecutionID {
				continue
			}
			if candidate.StepID != "prepare" && candidate.StepID != "implement" {
				return nativeErr("immutable_task", "later step was dispatched; PR publication cannot be ruled out")
			}
			if candidate.AttemptID == run.AttemptID {
				attempt = candidate
			}
		}
		if attempt == nil || attempt.StepID != "implement" || attempt.Status != "outcome_unknown" || len(attempt.Result) != 0 {
			return nativeErr("immutable_task", "original unknown implement attempt changed")
		}
		for _, decision := range snapshot.Decisions {
			if decision.WorkflowRunID == workflow.WorkflowRunID && decision.Status != "failed" && decision.Status != "superseded" {
				return nativeErr("immutable_task", "old workflow has an unresolved decision")
			}
		}
		stamp := time.Now().UTC()
		if e = TransitionStepAttempt(attempt, "stopped"); e != nil {
			return e
		}
		if e = tx.PutStepAttempt(*attempt); e != nil {
			return e
		}
		execution.Status, execution.UpdatedAt = "stopped", stamp
		if e = tx.PutExecution(*execution); e != nil {
			return e
		}
		workflow.Phase, workflow.UpdatedAt = "abandoned", stamp
		if e = tx.PutWorkflowRun(*workflow); e != nil {
			return e
		}
		task.Status, task.Claimant, task.Version, task.UpdatedAt = "open", "", task.Version+1, stamp
		if e = tx.reopenNativeTaskCAS(task, input.ExpectedVersion); e != nil {
			return e
		}
		for _, predecessorID := range input.PredecessorIDs {
			if e = tx.PutTaskDependency(TaskDependencyRecord{ProjectIdentity: c.Project, PredecessorID: predecessorID, SuccessorID: input.TaskID}); e != nil {
				return e
			}
		}
		graphRevision, e := tx.AdvanceTaskGraphRevision(c.Project, graph.Authority.AuthorityVersion, graph.Authority.GraphRevision)
		if e != nil {
			return e
		}
		receipt = TaskReopenReceipt{RequestID: input.RequestID, TaskID: input.TaskID, TaskVersion: task.Version, GraphRevision: graphRevision, AbandonedRunID: run.RunID, Status: "open"}
		body, _ := json.Marshal(map[string]any{"receipt": receipt, "old_execution_id": execution.ExecutionID, "old_attempt_id": attempt.AttemptID, "old_terminal": run.Process, "confirmed_process_stopped": true, "reason": reason, "requested_by": input.RequestedBy, "added_predecessors": input.PredecessorIDs})
		if _, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: input.TaskID, WorkflowRunID: run.RunID, ExecutionID: execution.ExecutionID, AttemptID: attempt.AttemptID, ExternalSource: "native-task-command", ExternalID: input.RequestID, Type: "task.reopened", ReceivedAt: stamp, Payload: body}); e != nil {
			return e
		}
		result, _ := json.Marshal(receipt)
		command := CommandRecord{ProjectIdentity: c.Project, CommandID: input.RequestID, Kind: "reopen_task", TargetID: input.TaskID, ExpectedVersion: input.ExpectedVersion, IdempotencyKey: input.RequestID, RequestedBy: input.RequestedBy, Payload: request, Status: "accepted", Result: result, CreatedAt: stamp, UpdatedAt: stamp}
		if _, made, commandErr := tx.EnqueueCommand(command); commandErr != nil || !made {
			return errors.Join(commandErr, nativeErr("idempotency_conflict", "reopen command already exists"))
		}
		operation := OperationRecord{ProjectIdentity: c.Project, OperationID: input.RequestID, IdempotencyKey: input.RequestID, Kind: "reopen_task", Status: "confirmed", PayloadDigest: digestCanonical(request), Request: request, Receipt: result, CreatedAt: stamp, UpdatedAt: stamp}
		if _, made, opErr := tx.PutOperation(operation); opErr != nil || !made {
			return errors.Join(opErr, nativeErr("idempotency_conflict", "reopen operation already exists"))
		}
		updatedSnapshot, e := querySnapshot(ctx, tx.tx, c.Project)
		if e != nil {
			return e
		}
		remaining := make([]*Run, 0, len(runtimeState.Runs)-1)
		for _, candidate := range runtimeState.Runs {
			if candidate == nil || candidate.RunID != run.RunID {
				remaining = append(remaining, candidate)
			}
		}
		runtimeState.Runs = remaining
		runtimeState.CurrentTask = nil
		runtimeState.SlotOccupied = false
		runtimeState.Tasks = updatedSnapshot.Tasks
		runtimeState.WorkflowRuns = updatedSnapshot.WorkflowRuns
		runtimeState.Executions = updatedSnapshot.Executions
		runtimeState.Attempts = updatedSnapshot.Attempts
		runtimeState.Decisions = updatedSnapshot.Decisions
		runtimeState.Operations = append(runtimeState.Operations, operation)
		runtimeState.Revision = revision + 1
		runtimeState.UpdatedAt = stamp
		projectionBody, e := json.Marshal(runtimeState)
		if e != nil {
			return e
		}
		return tx.PutProjection(c.Project, runtimeState.Revision, projectionBody)
	})
	return receipt, err
}

func (t *sqliteTx) reopenNativeTaskCAS(next TaskRecord, expectedVersion uint64) error {
	if next.Status != "open" || next.Claimant != "" || next.Version != expectedVersion+1 {
		return nativeErr("contract_mismatch", "reopened native task must be open and unclaimed at the next version")
	}
	var oldPayload []byte
	if err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM tasks WHERE project_identity=? AND task_id=? AND status='in_progress' AND version=?`, next.ProjectIdentity, next.TaskID, expectedVersion).Scan(&oldPayload); err != nil {
		return nativeErr("stale_version", "task claim changed before reopen")
	}
	var old TaskRecord
	if err := json.Unmarshal(oldPayload, &old); err != nil {
		return err
	}
	old.Status, old.Claimant, old.Version, old.UpdatedAt = next.Status, next.Claimant, next.Version, next.UpdatedAt
	if !sameTaskRecord(old, next) {
		return nativeErr("contract_mismatch", "reopen cannot change the task identity, contract or audit fields")
	}
	var active, delivered int
	if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM workflow_runs WHERE project_identity=? AND task_id=? AND phase NOT IN ('completed','abandoned')`, next.ProjectIdentity, next.TaskID).Scan(&active); err != nil {
		return err
	}
	if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, next.ProjectIdentity, next.TaskID).Scan(&delivered); err != nil {
		return err
	}
	if active != 0 || delivered != 0 {
		return nativeErr("immutable_task", "task still has an active workflow or delivery")
	}
	body, err := json.Marshal(next)
	if err != nil {
		return err
	}
	result, err := t.tx.ExecContext(t.ctx, `UPDATE tasks SET status='open',version=?,payload=? WHERE project_identity=? AND task_id=? AND status='in_progress' AND version=?`, next.Version, body, next.ProjectIdentity, next.TaskID, expectedVersion)
	return requireIdentityWrite(result, err, "stale_version: task changed before reopen")
}

func sameTaskRecord(a, b TaskRecord) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
