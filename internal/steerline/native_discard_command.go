package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	"sort"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/task"
)

type TaskDiscardReceipt = task.TaskDiscardReceipt

type taskDiscardIntent struct {
	TaskID          string   `json:"task_id"`
	ExpectedVersion uint64   `json:"expected_version"`
	Reason          string   `json:"reason"`
	RequestedBy     string   `json:"requested_by"`
	PRURL           string   `json:"pr_url,omitempty"`
	UncertainPR     bool     `json:"uncertain_pr,omitempty"`
	CleanupPending  []string `json:"cleanup_pending,omitempty"`
}

func discardTaskRecord(record TaskRecord, reason, requestedBy, requestID string, cleanup []string, at time.Time) (TaskRecord, error) {
	return task.Discard(record, reason, requestedBy, requestID, cleanup, false, at)
}

type taskDiscardPlan struct {
	PRURL          string
	UncertainPR    bool
	CleanupPending []string
}

func (c CommandService) DiscardNativeTask(ctx context.Context, config Config, taskID, requestID string, expectedVersion uint64, reason, requestedBy string) (TaskDiscardReceipt, error) {
	if c.Store == nil || c.Project == "" || config.Identity() != c.Project {
		return TaskDiscardReceipt{}, nativeErr("authority_mismatch", "discard requires the configured native project")
	}
	if err := ValidateNativeTaskID(taskID); err != nil {
		return TaskDiscardReceipt{}, err
	}
	if !controlIDPattern.MatchString(requestID) || expectedVersion == 0 || strings.TrimSpace(reason) == "" {
		return TaskDiscardReceipt{}, nativeErr("immutable_task", "discard requires request identity, expected version and reason")
	}
	if strings.TrimSpace(requestedBy) == "" {
		requestedBy = DefaultRequestedBy()
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > 4000 {
		return TaskDiscardReceipt{}, nativeErr("immutable_task", "discard reason exceeds 4000 bytes")
	}
	sqlite := c.Store
	request := struct {
		TaskID          string `json:"task_id"`
		ExpectedVersion uint64 `json:"expected_version"`
		Reason          string `json:"reason"`
		RequestedBy     string `json:"requested_by"`
	}{taskID, expectedVersion, reason, requestedBy}
	requestJSON, _ := json.Marshal(request)
	canonicalRequest, err := CanonicalJSON(requestJSON)
	if err != nil {
		return TaskDiscardReceipt{}, err
	}
	if old, found, e := sqlite.lookupNativeCommand(ctx, c.Project, requestID); e != nil {
		return TaskDiscardReceipt{}, e
	} else if found {
		if old.Kind != "discard_task" || old.TargetID != taskID || old.ExpectedVersion != expectedVersion || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, canonicalRequest) {
			return TaskDiscardReceipt{}, nativeErr("idempotency_conflict", "request_id was already used with different discard content")
		}
		if old.Status == "accepted" || old.Status == "rejected" || old.Status == "pending_verification" {
			if old.Status != "pending_verification" {
				var saved TaskDiscardReceipt
				if json.Unmarshal(old.Result, &saved) == nil {
					return saved, nil
				}
			}
		}
	}
	snapshot, err := sqlite.Query(ctx, c.Project)
	if err != nil {
		return TaskDiscardReceipt{}, err
	}
	if w := latestNativeWorkflow(snapshot.WorkflowRuns, taskID); w != nil {
		config, err = fixedWorkflowPublicationConfig(config, *w)
		if err != nil {
			return TaskDiscardReceipt{}, err
		}
	}
	authority, err := sqlite.nativeAuthority(ctx, c.Project)
	if err != nil {
		return TaskDiscardReceipt{}, err
	}
	if err = validateNativeConfiguredTarget(ctx, config, authority); err != nil {
		return TaskDiscardReceipt{}, err
	}
	plan, task, err := taskDiscardPlanFromStore(ctx, sqlite, c.Project, taskID)
	if err != nil {
		return TaskDiscardReceipt{}, err
	}
	if task.Version != expectedVersion {
		return TaskDiscardReceipt{}, nativeErr("stale_version", fmt.Sprintf("expected task version %d, found %d", expectedVersion, task.Version))
	}
	intent := taskDiscardIntent{TaskID: taskID, ExpectedVersion: expectedVersion, Reason: reason, RequestedBy: requestedBy, PRURL: plan.PRURL, UncertainPR: plan.UncertainPR, CleanupPending: uniqueSorted(plan.CleanupPending)}
	intentBytes, _ := json.Marshal(intent)
	canonicalIntent, _ := CanonicalJSON(intentBytes)
	var staged OperationRecord
	err = sqlite.transactSQLite(ctx, func(tx *sqliteTx) error {
		if old, found, e := tx.lookupNativeCommand(ctx, c.Project, requestID); e != nil {
			return e
		} else if found {
			if old.Kind != "discard_task" || old.TargetID != taskID || old.ExpectedVersion != expectedVersion || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, canonicalRequest) {
				return nativeErr("idempotency_conflict", "request_id was already used with different discard content")
			}
			return taskBatchReplay{Receipt: TaskBatchReceipt{RequestID: requestID, Status: "accepted"}}
		}
		var oldPayload []byte
		var status string
		var version uint64
		if e := tx.tx.QueryRowContext(ctx, `SELECT payload,status,version FROM tasks WHERE project_identity=? AND task_id=?`, c.Project, taskID).Scan(&oldPayload, &status, &version); e != nil {
			return nativeErr("task_not_found", "native task was not found")
		}
		if version != expectedVersion {
			return nativeErr("stale_version", fmt.Sprintf("expected task version %d, found %d", expectedVersion, version))
		}
		var deliveries int
		if e := tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, c.Project, taskID).Scan(&deliveries); e != nil {
			return e
		}
		if deliveries > 0 {
			return nativeErr("already_merged", "task already has a verified delivery")
		}
		if status != "open" && status != "in_progress" {
			return nativeErr("immutable_task", "only open or in_progress native tasks can be discarded")
		}
		command := CommandRecord{ProjectIdentity: c.Project, CommandID: requestID, Kind: "discard_task", TargetID: taskID, ExpectedVersion: expectedVersion, IdempotencyKey: requestID, RequestedBy: requestedBy, Payload: canonicalRequest, Status: "in_flight", CreatedAt: time.Now().UTC()}
		var created bool
		command, created, e := tx.EnqueueCommand(command)
		if e != nil {
			return nativeErr("idempotency_conflict", e.Error())
		}
		if !created {
			return nativeErr("idempotency_conflict", "request_id already belongs to another discard command")
		}
		staged = OperationRecord{ProjectIdentity: c.Project, OperationID: requestID, IdempotencyKey: requestID, Kind: "discard_task", Status: "pending", PayloadDigest: digestCanonical(canonicalRequest), Request: canonicalIntent, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if _, created, e = tx.PutOperation(staged); e != nil {
			return e
		} else if !created {
			return nativeErr("idempotency_conflict", "discard operation identity is already in use")
		}
		_, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: taskID, ExternalSource: "native-task-command", ExternalID: requestID + ":intent", Type: "task.discard.intent", ReceivedAt: time.Now().UTC(), Payload: canonicalIntent})
		return e
	})
	if replay, ok := err.(taskBatchReplay); ok && replay.Receipt.Status == "accepted" {
		old, found, e := sqlite.lookupNativeCommand(ctx, c.Project, requestID)
		if e != nil {
			return TaskDiscardReceipt{}, e
		}
		if !found {
			return TaskDiscardReceipt{}, nativeErr("unsupported_schema", "discard command checkpoint disappeared")
		}
		if old.Status == "accepted" || old.Status == "rejected" {
			var saved TaskDiscardReceipt
			if e = json.Unmarshal(old.Result, &saved); e != nil {
				return TaskDiscardReceipt{}, e
			}
			return saved, nil
		}
		staged, e = lookupOperation(ctx, sqlite, c.Project, requestID)
		if e != nil {
			return TaskDiscardReceipt{}, e
		}
		if e = json.Unmarshal(staged.Request, &intent); e != nil {
			return TaskDiscardReceipt{}, nativeErr("unsupported_schema", "discard operation intent is unreadable")
		}
	} else if err != nil {
		return TaskDiscardReceipt{}, err
	}
	if staged.OperationID == "" {
		staged, err = lookupOperation(ctx, sqlite, c.Project, requestID)
		if err != nil {
			return TaskDiscardReceipt{}, err
		}
		if err = json.Unmarshal(staged.Request, &intent); err != nil {
			return TaskDiscardReceipt{}, nativeErr("unsupported_schema", "discard operation intent is unreadable")
		}
	}
	if len(staged.Receipt) > 0 {
		var checkpoint taskDiscardIntent
		if err = json.Unmarshal(staged.Receipt, &checkpoint); err != nil {
			return TaskDiscardReceipt{}, nativeErr("unsupported_schema", "discard reconciliation checkpoint is unreadable")
		}
		intent = checkpoint
	}
	currentPlan, currentTask, err := taskDiscardPlanFromStore(ctx, sqlite, c.Project, taskID)
	if err != nil {
		return TaskDiscardReceipt{}, err
	}
	if currentTask.Version != expectedVersion {
		return TaskDiscardReceipt{}, nativeErr("stale_version", fmt.Sprintf("expected task version %d, found %d", expectedVersion, currentTask.Version))
	}
	if intent.PRURL != "" && currentPlan.PRURL != intent.PRURL {
		return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", "the fixed PR binding no longer matches the task's recorded publication")
	}
	currentCleanup := uniqueSorted(currentPlan.CleanupPending)
	if intent.PRURL != currentPlan.PRURL || intent.UncertainPR != currentPlan.UncertainPR || !sameStrings(intent.CleanupPending, currentCleanup) {
		intent.PRURL = currentPlan.PRURL
		intent.UncertainPR = currentPlan.UncertainPR
		intent.CleanupPending = currentCleanup
		if err = updateDiscardIntentReceipt(ctx, sqlite, &staged, intent); err != nil {
			return TaskDiscardReceipt{}, err
		}
	}
	var prInfo *PRInfo
	var queryErr error
	if intent.UncertainPR {
		return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", "publication outcome is not conclusive")
	}
	if intent.PRURL != "" {
		target, targetErr := configuredRepositoryTarget(ctx, config)
		if targetErr != nil || config.PR.ValidateRemote(target) != nil {
			return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", "PR provider configuration does not match the fixed repository target")
		}
		client := publication.NewClient(config.PR, Runner{Dir: config.Repository, Timeout: duration(config.CommandTimeout)}, duration(config.CommandTimeout))
		info, e := client.Query(ctx, intent.PRURL)
		if e != nil {
			queryErr = e
		} else {
			prInfo = &info
		}
		if queryErr != nil {
			return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", "PR merge state could not be verified: "+clip(queryErr.Error(), 1000))
		}
	}
	return c.commitDiscard(ctx, sqlite, requestID, staged, intent, prInfo, config.PR.Repository)
}

func updateDiscardIntentReceipt(ctx context.Context, s *SQLiteStore, op *OperationRecord, intent taskDiscardIntent) error {
	data, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	next := *op
	next.Receipt = data
	next.Status = "pending"
	next.UpdatedAt = time.Now().UTC()
	if err = s.Transact(ctx, func(domainTx DomainTx) error { return domainTx.UpdateOperation(next) }); err != nil {
		return err
	}
	*op = next
	return nil
}

func taskDiscardPlanFromStore(ctx context.Context, s *SQLiteStore, project, taskID string) (taskDiscardPlan, TaskRecord, error) {
	snapshot, err := s.Query(ctx, project)
	if err != nil {
		return taskDiscardPlan{}, TaskRecord{}, err
	}
	return taskDiscardPlanFromSnapshot(snapshot, taskID)
}

func taskDiscardPlanFromSnapshot(snapshot DomainSnapshot, taskID string) (taskDiscardPlan, TaskRecord, error) {
	var task TaskRecord
	found := false
	for _, candidate := range snapshot.Tasks {
		if candidate.TaskID == taskID {
			task = candidate
			found = true
			break
		}
	}
	if !found {
		return taskDiscardPlan{}, task, nativeErr("task_not_found", "native task was not found")
	}
	runs := map[string]bool{}
	for _, run := range snapshot.WorkflowRuns {
		if run.TaskID == taskID {
			runs[run.WorkflowRunID] = true
		}
	}
	executions := map[string]ExecutionRecord{}
	for _, execution := range snapshot.Executions {
		if runs[execution.WorkflowRunID] {
			executions[execution.ExecutionID] = execution
		}
	}
	plan := taskDiscardPlan{CleanupPending: []string{}}
	prs := map[string]bool{}
	for _, run := range snapshot.WorkflowRuns {
		if run.TaskID != taskID || run.PRURL == "" || run.PRState == "closed_unmerged" {
			continue
		}
		prs[run.PRURL] = true
		if run.PRState == "" || run.PRState == "unknown" {
			plan.UncertainPR = true
		}
	}
	if active := latestNativeWorkflow(snapshot.WorkflowRuns, taskID); active != nil {
		if active.PRURL == "" && active.Phase != "queued" {
			// A started worker may have published a PR before the center saw a
			// link. Discard cannot assume that no external PR exists.
			plan.UncertainPR = true
		}
	}
	for _, execution := range executions {
		if execution.Workspace.Worktree != "" || execution.Target.WorktreeID != "" {
			id := execution.Target.WorktreeID
			if id == "" {
				id = execution.Workspace.Worktree
			}
			plan.CleanupPending = append(plan.CleanupPending, "workspace_cleanup:"+id)
		}
		if execution.Status == "active" || execution.Status == "waiting_decision" || execution.Status == "blocked" || execution.Status == "pending" {
			plan.CleanupPending = append(plan.CleanupPending, "execution_stop_manual:"+execution.ExecutionID)
		}
	}
	if len(prs) > 1 {
		return plan, task, nativeErr("immutable_task", "multiple PR identities are bound to this task; manual reconciliation is required")
	}
	for url := range prs {
		plan.PRURL = url
	}
	plan.CleanupPending = uniqueSorted(plan.CleanupPending)
	return plan, task, nil
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func lookupOperation(ctx context.Context, s *SQLiteStore, project, id string) (OperationRecord, error) {
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM operations WHERE project_identity=? AND operation_id=?`, project, id).Scan(&body)
	if err != nil {
		return OperationRecord{}, err
	}
	var op OperationRecord
	if err = json.Unmarshal(body, &op); err != nil {
		return op, err
	}
	return op, nil
}

func persistDiscardPending(ctx context.Context, s *SQLiteStore, project, requestID string, op OperationRecord, intent taskDiscardIntent, code, message string) (TaskDiscardReceipt, error) {
	receipt := TaskDiscardReceipt{RequestID: requestID, TaskID: intent.TaskID, Status: "pending_verification", Code: code, Message: message, CleanupPending: uniqueSorted(intent.CleanupPending)}
	checkpoint, err := json.Marshal(intent)
	if err != nil {
		return TaskDiscardReceipt{}, err
	}
	err = s.transactSQLite(ctx, func(tx *sqliteTx) error {
		op.Status = "outcome_unknown"
		op.Receipt = checkpoint
		op.UnknownReason = message
		op.UpdatedAt = time.Now().UTC()
		if e := tx.UpdateOperation(op); e != nil {
			return e
		}
		raw, _ := json.Marshal(receipt)
		cmd := CommandRecord{ProjectIdentity: project, CommandID: requestID, Status: "pending_verification", Result: raw}
		if e := tx.UpdateCommand(cmd); e != nil {
			return e
		}
		body, _ := json.Marshal(receipt)
		_, e := tx.AppendEvent(DomainEvent{ProjectIdentity: project, TaskID: intent.TaskID, ExternalSource: "native-task-command", ExternalID: requestID + ":verification-unknown", Type: "task.discard.verification_unknown", ReceivedAt: time.Now().UTC(), Payload: body})
		return e
	})
	return receipt, err
}

func (c CommandService) commitDiscard(ctx context.Context, s *SQLiteStore, requestID string, op OperationRecord, intent taskDiscardIntent, info *PRInfo, expectedPRRepository string) (TaskDiscardReceipt, error) {
	var receipt TaskDiscardReceipt
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		graph, e := taskGraphSnapshotTx(ctx, tx, c.Project)
		if e != nil {
			return e
		}
		task, ok := graph.ByID[intent.TaskID]
		if !ok {
			return nativeErr("task_not_found", "native task was not found")
		}
		if task.Version != intent.ExpectedVersion {
			return nativeErr("stale_version", fmt.Sprintf("expected task version %d, found %d", intent.ExpectedVersion, task.Version))
		}
		var deliveries int
		if e = tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, c.Project, intent.TaskID).Scan(&deliveries); e != nil {
			return e
		}
		if deliveries > 0 {
			return nativeErr("already_merged", "task already has a verified delivery")
		}
		if task.Status != "open" && task.Status != "in_progress" {
			return nativeErr("immutable_task", "only open or in_progress native tasks can be discarded")
		}
		if intent.PRURL != "" && (info == nil || info.URL != intent.PRURL || expectedPRRepository != "" && info.Repository != expectedPRRepository || info.BaseBranch != graph.Authority.Branch) {
			return nativeErr("merge_state_unknown", "PR binding or target branch changed during discard verification")
		}
		if info != nil && info.Merged || info != nil && info.State == "merged" {
			receipt = TaskDiscardReceipt{RequestID: requestID, TaskID: intent.TaskID, Status: "rejected", Code: "already_merged", Message: "task PR is already merged", GraphRevision: graph.Authority.GraphRevision}
			return finishDiscardCommand(tx, op, requestID, receipt, "rejected", false)
		}
		if intent.UncertainPR {
			return nativeErr("merge_state_unknown", "PR publication state remains unknown")
		}
		currentPlan, currentTask, e := taskDiscardPlanTx(ctx, tx, c.Project, intent.TaskID)
		if e != nil {
			return e
		}
		if currentTask.Version != intent.ExpectedVersion {
			return nativeErr("stale_version", "task version changed during PR verification")
		}
		if currentPlan.UncertainPR || currentPlan.PRURL != intent.PRURL {
			return nativeErr("merge_state_unknown", "published PR binding changed or remains uncertain")
		}
		if info != nil && info.State != "open" && info.State != "closed_unmerged" {
			return nativeErr("merge_state_unknown", "PR provider did not confirm an unmerged PR state")
		}
		cleanup := append([]string(nil), currentPlan.CleanupPending...)
		if info != nil && info.State == "open" {
			cleanup = append(cleanup, "pull_request_close:"+intent.PRURL)
		}
		cleanup = uniqueSorted(cleanup)
		discardedAt := time.Now().UTC()
		task, e = discardTaskRecord(task, intent.Reason, intent.RequestedBy, requestID, cleanup, discardedAt)
		if e != nil {
			return e
		}
		if e = tx.UpdateNativeTaskCAS(task, intent.ExpectedVersion); e != nil {
			return e
		}
		receipt = TaskDiscardReceipt{RequestID: requestID, TaskID: intent.TaskID, Status: "discarded", GraphRevision: graph.Authority.GraphRevision + 1, CleanupPending: cleanup}
		body, _ := json.Marshal(map[string]any{"task_id": intent.TaskID, "request_id": requestID, "reason": intent.Reason, "discarded_by": intent.RequestedBy, "discarded_at": discardedAt, "cleanup_pending": cleanup})
		if _, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: intent.TaskID, ExternalSource: "native-task-command", ExternalID: requestID + ":discarded", Type: "task.discarded", ReceivedAt: discardedAt, Payload: body}); e != nil {
			return e
		}
		return finishDiscardCommand(tx, op, requestID, receipt, "accepted", true)
	})
	return receipt, err
}

func finishDiscardCommand(tx *sqliteTx, op OperationRecord, requestID string, receipt TaskDiscardReceipt, status string, confirmed bool) error {
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	command := CommandRecord{ProjectIdentity: op.ProjectIdentity, CommandID: requestID, Status: status, Result: receiptBytes}
	if err = tx.UpdateCommand(command); err != nil {
		return err
	}
	if confirmed {
		op.Status = "confirmed"
	} else {
		op.Status = "rejected"
	}
	op.UpdatedAt = time.Now().UTC()
	if err = tx.UpdateOperation(op); err != nil {
		return err
	}
	return nil
}

func taskDiscardPlanTx(ctx context.Context, tx *sqliteTx, project, taskID string) (taskDiscardPlan, TaskRecord, error) {
	snapshot, err := querySnapshot(ctx, tx.tx, project)
	if err != nil {
		return taskDiscardPlan{}, TaskRecord{}, err
	}
	return taskDiscardPlanFromSnapshot(snapshot, taskID)
}
