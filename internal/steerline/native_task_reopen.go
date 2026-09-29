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

// TaskReopenInput abandons one stopped or conclusively settled undelivered Run.
// The old workflow and its evidence remain readable under their original IDs.
type TaskReopenInput struct {
	TaskID                  string
	WorkflowRunID           string
	RequestID               string
	ExpectedVersion         uint64
	ExpectedGraphRevision   *uint64
	PredecessorIDs          []string
	ConfirmedProcessStopped bool
	ConfirmUnfinishedReset  bool
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
	if !stableExecutionID(input.WorkflowRunID) || !controlIDPattern.MatchString(input.RequestID) || input.ExpectedVersion == 0 || strings.TrimSpace(input.RequestedBy) == "" || input.ConfirmedProcessStopped == input.ConfirmUnfinishedReset {
		return empty, nativeErr("contract_mismatch", "reopen requires the exact run, request, task version, operator and one stopped-process or unfinished-reset confirmation")
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
		Reset          bool     `json:"confirm_unfinished_reset,omitempty"`
		Reason         string   `json:"reason"`
		RequestedBy    string   `json:"requested_by"`
	}{input.TaskID, input.WorkflowRunID, input.ExpectedVersion, input.ExpectedGraphRevision, input.PredecessorIDs, input.ConfirmedProcessStopped, input.ConfirmUnfinishedReset, reason, input.RequestedBy}))
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
		} else if candidate != nil && candidate.TaskID == input.TaskID && (candidate.ExecutionStatus == "unknown" || candidate.ExecutionStatus == "pending" || candidate.ExecutionStatus == "running" || candidate.FinishTaskStatus == "waiting_decision") {
			return empty, nativeErr("slot_busy", "another run for this task still holds an execution reservation")
		}
	}
	if run == nil || runtimeState.CurrentTask != nil && *runtimeState.CurrentTask != input.TaskID || run.TaskID != input.TaskID || run.DomainTaskID != input.TaskID || run.WorkflowVersion != MinimalWorkflowVersion {
		return empty, nativeErr("immutable_task", "reopen requires the exact undelivered Orca run")
	}
	settled := input.ConfirmUnfinishedReset
	unstarted := settled && run.Target.Dispatch == ""
	if settled && (run.Protocol != string(ProtocolOrcaSupervised) || run.FinishTaskStatus == "completed" || unstarted && (run.Process != nil || run.Target.WorktreeID != "" || deref(run.PR.URL) != "") || !unstarted && run.Target.WorktreeID == "") || !settled && (!runtimeState.SlotOccupied || run.ExecutionStatus != "unknown" || run.FinishTaskStatus != "blocked" || run.WorktreePath == nil || deref(run.PR.URL) != "" || run.Process == nil || run.Process.Kind != "orca" && run.Process.Kind != "orca_supervised" || run.Process.Terminal == "" || run.Process.RuntimeID == "" || run.Process.Incarnation == "") {
		return empty, nativeErr("immutable_task", "reopen lacks its fixed stopped process or settled Dispatch identity")
	}
	if unstarted {
		for _, attempt := range runtimeState.Attempts {
			if attempt.ExecutionID == run.ExecutionID && (attempt.ExternalBinding.Dispatch != "" || attempt.ExternalBinding.WorktreeID != "" || attempt.Status != "planned" && attempt.Status != "failed" && attempt.Status != "rejected") {
				return empty, nativeErr("cleanup_pending", "old Run has an unverified worker Attempt")
			}
		}
		for _, operation := range runtimeState.Operations {
			if operation.Kind != "dispatch_step" && operation.Kind != "prepare_workspace" && operation.Kind != "create_orca_run" {
				continue
			}
			var request struct {
				WorkflowRunID string `json:"workflow_run_id"`
			}
			if json.Unmarshal(operation.Request, &request) != nil {
				return empty, nativeErr("cleanup_pending", "external start operation has an unreadable identity")
			}
			if request.WorkflowRunID == run.RunID && (operation.Kind != "create_orca_run" || operation.Status != "confirmed" || run.Target.OrchestrationRun == "") {
				return empty, nativeErr("cleanup_pending", "old Run has an unverified external start operation")
			}
		}
	}
	if !settled && run.Process.Kind == "orca" {
		if _, statErr := os.Lstat(*run.WorktreePath); !os.IsNotExist(statErr) {
			return empty, nativeErr("cleanup_pending", "original worktree must be removed before reopening the task")
		}
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
	var settledRef ExecutionRef
	if settled {
		if executionConfig.hostName() != "orca_supervised" || executionConfig.Orca.Supervised == nil || !unstarted && executionConfig.Orca.Supervised.EnvironmentID != run.Target.ExecutionHost {
			return empty, nativeErr("contract_mismatch", "old workflow has no matching pinned supervised Orca profile")
		}
		if !unstarted {
			settledRef, err = settledReopenBinding(config, &runtimeState, run)
			if err != nil {
				return empty, err
			}
			if run.Process != nil && *run.Process != settledRef.External {
				return empty, nativeErr("authority_mismatch", "active Run process differs from its final Dispatch binding")
			}
			if _, err = (orcaSupervisedBackend{config: executionConfig}).settledWorker(ctx, settledRef, ""); err != nil {
				return empty, nativeErr("cleanup_pending", "original supervised Dispatch is not conclusively settled: "+err.Error())
			}
		}
	} else if run.Process.Kind == "orca_supervised" {
		if executionConfig.hostName() != "orca_supervised" || executionConfig.Orca.Supervised == nil || executionConfig.Orca.Supervised.EnvironmentID != run.Process.ExecutionHost {
			return empty, nativeErr("contract_mismatch", "old workflow has no matching pinned supervised Orca profile")
		}
		if err := verifyStoppedSupervisedReopen(ctx, executionConfig, *run.Process); err != nil {
			return empty, err
		}
	} else {
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
	}
	if settled {
		if deref(run.PR.URL) == "" {
			for _, attempt := range runtimeState.Attempts {
				if attempt.ExecutionID == run.ExecutionID && attempt.StepID == "publish_pr" && attempt.Status != "planned" {
					return empty, nativeErr("pending_verification", "publish_pr may have created a PR without a fixed URL")
				}
			}
		} else {
			pr := &PRClient{Config: config.PR, Runner: Runner{Dir: config.Repository, Timeout: 30 * time.Second}}
			info, queryErr := pr.Query(ctx, *run.PR.URL)
			if queryErr != nil {
				return empty, nativeErr("pending_verification", "old PR merge state cannot be verified: "+queryErr.Error())
			}
			if info.Merged || info.State == "merged" || info.MergedAt != nil && !info.MergedAt.IsZero() || info.MergeCommit != nil && *info.MergeCommit != "" {
				return empty, nativeErr("already_merged", "old PR was merged; reconcile delivery before resetting")
			}
			var candidate string
			for _, execution := range runtimeState.Executions {
				if execution.ExecutionID == run.ExecutionID {
					candidate = execution.CandidateRevision
				}
			}
			if info.URL != *run.PR.URL || info.Repository != config.PR.Repository || info.BaseBranch != config.Branch || candidate == "" || info.HeadRevision != candidate {
				return empty, nativeErr("contract_mismatch", "old PR differs from the fixed repository, base, or candidate head")
			}
		}
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
		if workflow == nil || settled && (workflow.Phase == "completed" || workflow.Phase == "abandoned" || deref(run.PR.URL) == "" && workflow.SelectedResult != "") || !settled && (workflow.Phase != "blocked" || workflow.SelectedResult != "" || workflow.PendingDecisions != 0) {
			return nativeErr("immutable_task", "old workflow is not eligible for an undelivered reset")
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
		if execution == nil || settled && (unstarted && (execution.Process != nil || execution.Target.Dispatch != "" || execution.Target.WorktreeID != "") || !unstarted && (execution.Process != nil && *execution.Process != settledRef.External || execution.Target.OrchestrationRun != settledRef.External.OrchestrationRun || execution.Target.WorktreeID != settledRef.External.WorktreeID)) || !settled && (execution.Status != "blocked" || execution.CurrentStep != "implement" || execution.Process == nil || *execution.Process != *run.Process) {
			return nativeErr("immutable_task", "old implement execution or process binding changed")
		}
		var attempt *StepAttemptRecord
		var settledFinal *StepAttemptRecord
		for i := range snapshot.Attempts {
			candidate := &snapshot.Attempts[i]
			if candidate.ExecutionID != execution.ExecutionID {
				continue
			}
			if !settled && candidate.StepID != "prepare" && candidate.StepID != "implement" {
				return nativeErr("immutable_task", "later step was dispatched; PR publication cannot be ruled out")
			}
			if unstarted && (candidate.ExternalBinding.Dispatch != "" || candidate.ExternalBinding.WorktreeID != "" || candidate.Status != "planned" && candidate.Status != "failed" && candidate.Status != "rejected") {
				return nativeErr("cleanup_pending", "old Run gained an unverified worker Attempt")
			}
			if settled && candidate.AttemptID == settledRef.AttemptID {
				if settledFinal != nil || candidate.ExternalBinding.Dispatch != settledRef.External.Dispatch {
					return nativeErr("immutable_task", "final settled Attempt changed before reset")
				}
				settledFinal = candidate
			}
			if candidate.AttemptID == run.AttemptID {
				attempt = candidate
			}
		}
		if attempt == nil || settled && (!unstarted && settledFinal == nil || unstarted && settledFinal != nil || attempt.StepID != execution.CurrentStep) || !settled && (attempt.StepID != "implement" || attempt.Status != "outcome_unknown" || len(attempt.Result) != 0) {
			return nativeErr("immutable_task", "original unknown implement attempt changed")
		}
		for _, decision := range snapshot.Decisions {
			if decision.WorkflowRunID != workflow.WorkflowRunID {
				continue
			}
			if settled {
				continue
			}
			if decision.Status != "failed" && decision.Status != "superseded" {
				return nativeErr("immutable_task", "old workflow has an unresolved or applied decision")
			}
		}
		stamp := time.Now().UTC()
		if settled {
			for _, decision := range snapshot.Decisions {
				if decision.WorkflowRunID == workflow.WorkflowRunID && decision.Status != "failed" && decision.Status != "superseded" && decision.Status != "applied" {
					decision.Status, decision.Version, decision.UpdatedAt = "superseded", decision.Version+1, stamp
					if e = tx.PutDecision(decision); e != nil {
						return e
					}
				}
			}
			workflow.PendingDecisions = 0
			for i := range snapshot.Attempts {
				old := &snapshot.Attempts[i]
				if old.ExecutionID != execution.ExecutionID {
					continue
				}
				if e = abandonSettledAttempt(old); e != nil {
					return e
				}
				if e = tx.PutStepAttempt(*old); e != nil {
					return e
				}
			}
		} else if e = TransitionStepAttempt(attempt, "stopped"); e != nil {
			return e
		}
		if !settled {
			e = tx.PutStepAttempt(*attempt)
		}
		if e != nil {
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
		oldTerminal := run.Process
		if settled {
			oldTerminal = &settledRef.External
		}
		body, _ := json.Marshal(map[string]any{"receipt": receipt, "old_execution_id": execution.ExecutionID, "old_attempt_id": attempt.AttemptID, "old_terminal": oldTerminal, "old_worktree_id": run.Target.WorktreeID, "old_pr_url": deref(run.PR.URL), "confirmed_process_stopped": input.ConfirmedProcessStopped, "confirm_unfinished_reset": settled, "reason": reason, "requested_by": input.RequestedBy, "added_predecessors": input.PredecessorIDs})
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
		reservation := &Service{State: runtimeState}
		reservation.refreshSlotOccupancy()
		runtimeState = reservation.State
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

// settledReopenBinding identifies the final Dispatch without borrowing a
// newly planned step's empty process binding. The old worktree remains history.
func settledReopenBinding(config Config, state *Runtime, run *Run) (ExecutionRef, error) {
	var latest *StepAttemptRecord
	for i := range state.Attempts {
		attempt := &state.Attempts[i]
		if attempt.ExecutionID != run.ExecutionID {
			continue
		}
		if attempt.ExternalBinding.Dispatch == run.Target.Dispatch && run.Target.Dispatch != "" {
			if latest != nil || attempt.Status == "planned" {
				return ExecutionRef{}, nativeErr("immutable_task", "the final Dispatch is not unique or was never started")
			}
			latest = attempt
		} else if attempt.ExternalBinding.Dispatch != "" && attempt.Status != "succeeded" && attempt.Status != "failed" && attempt.Status != "stopped" {
			return ExecutionRef{}, nativeErr("immutable_task", "an earlier Dispatch is not settled")
		} else if attempt.ExternalBinding.Dispatch == "" && attempt.Status != "planned" && attempt.Status != "succeeded" && attempt.Status != "failed" && attempt.Status != "stopped" {
			return ExecutionRef{}, nativeErr("immutable_task", "an Attempt without a Dispatch has an uncertain side effect")
		}
	}
	if latest == nil {
		return ExecutionRef{}, nativeErr("immutable_task", "the final supervised Dispatch is missing")
	}
	ref, err := (executionRecords{root: config.RunDir}).loadBinding(run.RunID, latest.AttemptID)
	if err != nil {
		return ExecutionRef{}, nativeErr("cleanup_pending", "final Dispatch binding cannot be verified: "+err.Error())
	}
	if ref.Protocol != ProtocolOrcaSupervised || ref.RunID != run.RunID || ref.AttemptID != latest.AttemptID || ref.Target.OrchestrationRun != run.Target.OrchestrationRun || ref.Target.Dispatch != run.Target.Dispatch || ref.Target.WorktreeID != run.Target.WorktreeID || ref.Target.CoordinatorTerminal != run.Target.CoordinatorTerminal || ref.External.Kind != "orca_supervised" || ref.External.OrchestrationRun != run.Target.OrchestrationRun || ref.External.Task != run.Target.ExternalTask || ref.External.Dispatch != run.Target.Dispatch || ref.External.WorktreeID != run.Target.WorktreeID || ref.External.ExecutionHost != run.Target.ExecutionHost || ref.External.Terminal == "" || ref.External.Incarnation == "" {
		return ExecutionRef{}, nativeErr("authority_mismatch", "final Dispatch binding differs from the fixed Run and worktree")
	}
	return ref, nil
}

func abandonSettledAttempt(attempt *StepAttemptRecord) error {
	switch attempt.Status {
	case "planned":
		return TransitionStepAttempt(attempt, "failed")
	case "dispatching", "dispatched":
		if err := TransitionStepAttempt(attempt, "outcome_unknown"); err != nil {
			return err
		}
		return TransitionStepAttempt(attempt, "stopped")
	case "running", "waiting_decision":
		if err := TransitionStepAttempt(attempt, "stopping"); err != nil {
			return err
		}
		return TransitionStepAttempt(attempt, "stopped")
	case "stopping", "outcome_unknown":
		return TransitionStepAttempt(attempt, "stopped")
	case "succeeded", "failed", "rejected", "stopped":
		return nil
	default:
		return nativeErr("unsupported_schema", "old Attempt has an unsupported status")
	}
}

// verifyStoppedSupervisedReopen does not infer safety from a missing local
// Windows path. The execution host must confirm the original Dispatch,
// terminal incarnation and worktree have all stopped or been removed.
func verifyStoppedSupervisedReopen(ctx context.Context, config Config, p ProcessIdentity) error {
	if p.Kind != "orca_supervised" || p.CoordinatorRuntimeID == "" || p.OrchestrationRun == "" || p.Task == "" || p.Dispatch == "" || p.ExecutionHost == "" || p.WorktreeID == "" {
		return nativeErr("contract_mismatch", "supervised reopen requires the complete original external binding")
	}
	client := orcaClient{cfg: config}
	call := func(args ...string) (orcaEnvelope, error) {
		out, callErr := client.call(ctx, positiveDuration(config.CommandTimeout, 30*time.Second), args...)
		env, parseErr := parseEnvelope(out)
		if callErr != nil || parseErr != nil {
			return env, errors.Join(callErr, parseErr)
		}
		return env, nil
	}
	env, err := call("orchestration", "worker-show", "--dispatch", p.Dispatch, "--json")
	if err != nil {
		return nativeErr("cleanup_pending", "original supervised Dispatch cannot be inspected")
	}
	var worker struct {
		Dispatch struct {
			ID     string `json:"id"`
			RunID  string `json:"runId"`
			TaskID string `json:"taskId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID          string            `json:"dispatchId"`
			State               string            `json:"state"`
			WorktreeID          string            `json:"worktreeId"`
			AgentTerminalHandle string            `json:"agentTerminalHandle"`
			ResidualResources   []json.RawMessage `json:"residualResources"`
		} `json:"worker"`
		Server struct {
			EnvironmentID string `json:"environmentId"`
		} `json:"server"`
		RemoteRuntimeEpoch string `json:"remoteRuntimeEpoch"`
		Observation        struct {
			Status      string `json:"status"`
			ExactWorker bool   `json:"exactWorker"`
		} `json:"observation"`
	}
	if json.Unmarshal(env.Result, &worker) != nil || env.Meta.RuntimeID != p.CoordinatorRuntimeID || worker.Dispatch.ID != p.Dispatch || worker.Dispatch.RunID != p.OrchestrationRun || worker.Dispatch.TaskID != p.Task || worker.Dispatch.Status != "completed" || worker.Worker.DispatchID != p.Dispatch || worker.Worker.State != "succeeded" || worker.Worker.WorktreeID != p.WorktreeID || worker.Worker.AgentTerminalHandle != p.Terminal || worker.Worker.ResidualResources == nil || len(worker.Worker.ResidualResources) != 0 || worker.Server.EnvironmentID != p.ExecutionHost || worker.RemoteRuntimeEpoch != p.RuntimeID || !worker.Observation.ExactWorker || worker.Observation.Status != "exited" {
		return nativeErr("cleanup_pending", "original supervised worker is not the exact settled and exited Dispatch")
	}
	env, err = call("terminal", "show", "--environment", p.ExecutionHost, "--terminal", p.Terminal, "--json")
	if err != nil {
		return nativeErr("cleanup_pending", "original remote terminal cannot be inspected")
	}
	var remote struct {
		Terminal struct {
			Handle        string                 `json:"handle"`
			IncarnationID string                 `json:"incarnationId"`
			WorktreeID    string                 `json:"worktreeId"`
			Connected     *bool                  `json:"connected"`
			ExitCause     *orcaTerminalExitCause `json:"exitCause"`
		} `json:"terminal"`
	}
	if json.Unmarshal(env.Result, &remote) != nil || env.Meta.RuntimeID != p.RuntimeID || remote.Terminal.Handle != p.Terminal || remote.Terminal.IncarnationID != p.Incarnation || remote.Terminal.WorktreeID != p.WorktreeID || remote.Terminal.Connected == nil || *remote.Terminal.Connected || remote.Terminal.ExitCause == nil || remote.Terminal.ExitCause.Kind != "operator_close" {
		return nativeErr("cleanup_pending", "original remote terminal has not been exactly operator-closed")
	}
	out, _ := client.call(ctx, positiveDuration(config.CommandTimeout, 30*time.Second), "worktree", "show", "--environment", p.ExecutionHost, "--worktree", "id:"+p.WorktreeID, "--json")
	var absent orcaEnvelope
	if json.Unmarshal(out, &absent) != nil || absent.OK || absent.Error == nil || absent.Error.Code != "selector_not_found" || absent.Meta.RuntimeID != p.RuntimeID {
		return nativeErr("cleanup_pending", "original remote worktree is not verifiably removed")
	}
	return nil
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
