package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type taskDiscardIntent struct {
	TaskID            string   `json:"task_id"`
	ExpectedVersion   uint64   `json:"expected_version"`
	Reason            string   `json:"reason"`
	RequestedBy       string   `json:"requested_by"`
	PRURL             string   `json:"pr_url,omitempty"`
	PRURLs            []string `json:"pr_urls,omitempty"`
	CandidateBindings string   `json:"candidate_bindings"`
	UncertainPR       bool     `json:"uncertain_pr,omitempty"`
	CleanupPending    []string `json:"cleanup_pending,omitempty"`
}

func (c CommandService) DiscardNativeTask(ctx context.Context, config Config, taskID, requestID string, expectedVersion uint64, reason, requestedBy string) (taskmodel.TaskDiscardReceipt, error) {
	if err := validateWorkflowConfig(&config, nil); err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	if c.Store == nil || c.Project == "" || config.Identity() != c.Project {
		return taskmodel.TaskDiscardReceipt{}, nativeErr("authority_mismatch", "discard requires the configured native project")
	}
	if err := taskmodel.ValidateID(taskID); err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	if !controlIDPattern.MatchString(requestID) || expectedVersion == 0 || strings.TrimSpace(reason) == "" {
		return taskmodel.TaskDiscardReceipt{}, nativeErr("immutable_task", "discard requires request identity, expected version and reason")
	}
	if strings.TrimSpace(requestedBy) == "" {
		requestedBy = DefaultRequestedBy()
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > 4000 {
		return taskmodel.TaskDiscardReceipt{}, nativeErr("immutable_task", "discard reason exceeds 4000 bytes")
	}
	sqlite := c.Store
	if c.taskResults == nil {
		return taskmodel.TaskDiscardReceipt{}, nativeErr("authority_mismatch", "task intent port is required")
	}
	request, err := c.taskResults.Intent(taskmodel.DiscardIntent{TaskID: taskID, ExpectedVersion: expectedVersion, Reason: reason, RequestedBy: requestedBy})
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	requestJSON, _ := json.Marshal(request)
	canonicalRequest, err := taskmodel.CanonicalJSON(requestJSON)
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	if old, found, e := sqlite.LookupCommand(ctx, c.Project, requestID); e != nil {
		return taskmodel.TaskDiscardReceipt{}, e
	} else if found {
		if old.Kind != "discard_task" || old.TargetID != taskID || old.ExpectedVersion != expectedVersion || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, canonicalRequest) {
			return taskmodel.TaskDiscardReceipt{}, nativeErr("idempotency_conflict", "request_id was already used with different discard content")
		}
		if old.Status == "accepted" || old.Status == "rejected" || old.Status == "pending_verification" {
			if old.Status != "pending_verification" {
				var saved taskmodel.TaskDiscardReceipt
				if json.Unmarshal(old.Result, &saved) == nil {
					return saved, nil
				}
			}
		}
	}
	snapshot, err := sqlite.Query(ctx, c.Project)
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	if w := latestNativeWorkflow(snapshot.WorkflowRuns, taskID); w != nil {
		config, err = fixedWorkflowPublicationConfig(config, *w)
		if err != nil {
			return taskmodel.TaskDiscardReceipt{}, err
		}
	}
	authority, err := sqlite.NativeAuthority(ctx, c.Project)
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	if err = validateNativeConfiguredTarget(ctx, config, authority); err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	plan, task, err := taskDiscardPlanFromStore(ctx, sqlite, c.Project, taskID)
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	if task.Version != expectedVersion {
		return taskmodel.TaskDiscardReceipt{}, nativeErr("stale_version", fmt.Sprintf("expected task version %d, found %d", expectedVersion, task.Version))
	}
	intent := taskDiscardIntent{TaskID: taskID, ExpectedVersion: expectedVersion, Reason: reason, RequestedBy: requestedBy, PRURL: plan.PRURL, PRURLs: plan.PRURLs, CandidateBindings: plan.CandidateBindings, UncertainPR: plan.UncertainPR, CleanupPending: uniqueSorted(plan.CleanupPending)}
	intentBytes, _ := json.Marshal(intent)
	canonicalIntent, _ := taskmodel.CanonicalJSON(intentBytes)
	var staged executionmodel.OperationRecord
	err = sqlite.TransactDiscardIntent(ctx, func(tx persistence.DiscardIntentTx) error {
		if old, found, e := tx.LookupCommand(ctx, c.Project, requestID); e != nil {
			return e
		} else if found {
			if old.Kind != "discard_task" || old.TargetID != taskID || old.ExpectedVersion != expectedVersion || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, canonicalRequest) {
				return nativeErr("idempotency_conflict", "request_id was already used with different discard content")
			}
			return taskBatchReplay{Receipt: taskmodel.TaskBatchReceipt{RequestID: requestID, Status: "accepted"}}
		}
		current, e := tx.Task(ctx, c.Project, taskID)
		if e != nil {
			return nativeErr("task_not_found", "native task was not found")
		}
		status, version := current.Status, current.Version
		if version != expectedVersion {
			return nativeErr("stale_version", fmt.Sprintf("expected task version %d, found %d", expectedVersion, version))
		}
		deliveries, e := tx.DeliveryCount(ctx, c.Project, taskID)
		if e != nil {
			return e
		}
		if deliveries > 0 {
			return nativeErr("already_merged", "task already has a verified delivery")
		}
		if status != "open" && status != "in_progress" {
			return nativeErr("immutable_task", "only open or in_progress native tasks can be discarded")
		}
		command := storageports.CommandRecord{ProjectIdentity: c.Project, CommandID: requestID, Kind: "discard_task", TargetID: taskID, ExpectedVersion: expectedVersion, IdempotencyKey: requestID, RequestedBy: requestedBy, Payload: canonicalRequest, Status: "in_flight", CreatedAt: time.Now().UTC()}
		var created bool
		command, created, e = tx.EnqueueCommand(command)
		if e != nil {
			return nativeErr("idempotency_conflict", e.Error())
		}
		if !created {
			return nativeErr("idempotency_conflict", "request_id already belongs to another discard command")
		}
		staged = executionmodel.OperationRecord{ProjectIdentity: c.Project, OperationID: requestID, IdempotencyKey: requestID, Kind: "discard_task", Status: "pending", PayloadDigest: digestCanonical(canonicalRequest), Request: canonicalIntent, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if _, created, e = tx.PutOperation(staged); e != nil {
			return e
		} else if !created {
			return nativeErr("idempotency_conflict", "discard operation identity is already in use")
		}
		_, e = tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: c.Project, TaskID: taskID, ExternalSource: "native-task-command", ExternalID: requestID + ":intent", Type: "task.discard.intent", ReceivedAt: time.Now().UTC(), Payload: canonicalIntent})
		return e
	})
	if replay, ok := err.(taskBatchReplay); ok && replay.Receipt.Status == "accepted" {
		old, found, e := sqlite.LookupCommand(ctx, c.Project, requestID)
		if e != nil {
			return taskmodel.TaskDiscardReceipt{}, e
		}
		if !found {
			return taskmodel.TaskDiscardReceipt{}, nativeErr("unsupported_schema", "discard command checkpoint disappeared")
		}
		if old.Status == "accepted" || old.Status == "rejected" {
			var saved taskmodel.TaskDiscardReceipt
			if e = json.Unmarshal(old.Result, &saved); e != nil {
				return taskmodel.TaskDiscardReceipt{}, e
			}
			return saved, nil
		}
		staged, e = sqlite.Operation(ctx, c.Project, requestID)
		if e != nil {
			return taskmodel.TaskDiscardReceipt{}, e
		}
		if e = json.Unmarshal(staged.Request, &intent); e != nil {
			return taskmodel.TaskDiscardReceipt{}, nativeErr("unsupported_schema", "discard operation intent is unreadable")
		}
	} else if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	if staged.OperationID == "" {
		staged, err = sqlite.Operation(ctx, c.Project, requestID)
		if err != nil {
			return taskmodel.TaskDiscardReceipt{}, err
		}
		if err = json.Unmarshal(staged.Request, &intent); err != nil {
			return taskmodel.TaskDiscardReceipt{}, nativeErr("unsupported_schema", "discard operation intent is unreadable")
		}
	}
	if len(staged.Receipt) > 0 {
		var checkpoint taskDiscardIntent
		if err = json.Unmarshal(staged.Receipt, &checkpoint); err != nil {
			return taskmodel.TaskDiscardReceipt{}, nativeErr("unsupported_schema", "discard reconciliation checkpoint is unreadable")
		}
		intent = checkpoint
	}
	currentPlan, currentTask, err := taskDiscardPlanFromStore(ctx, sqlite, c.Project, taskID)
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	if currentTask.Version != expectedVersion {
		return taskmodel.TaskDiscardReceipt{}, nativeErr("stale_version", fmt.Sprintf("expected task version %d, found %d", expectedVersion, currentTask.Version))
	}
	if intent.PRURL != "" && currentPlan.PRURL != intent.PRURL {
		return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", "the fixed PR binding no longer matches the task's recorded publication")
	}
	currentCleanup := uniqueSorted(currentPlan.CleanupPending)
	if intent.PRURL != currentPlan.PRURL || !slices.Equal(intent.PRURLs, currentPlan.PRURLs) || intent.CandidateBindings != currentPlan.CandidateBindings || intent.UncertainPR != currentPlan.UncertainPR || !slices.Equal(intent.CleanupPending, currentCleanup) {
		intent.PRURL = currentPlan.PRURL
		intent.PRURLs, intent.CandidateBindings = currentPlan.PRURLs, currentPlan.CandidateBindings
		intent.UncertainPR = currentPlan.UncertainPR
		intent.CleanupPending = currentCleanup
		if err = updateDiscardIntentReceipt(ctx, sqlite, &staged, intent); err != nil {
			return taskmodel.TaskDiscardReceipt{}, err
		}
	}
	var prInfo *executionmodel.PRInfo
	if intent.UncertainPR {
		return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", "publication outcome is not conclusive")
	}
	if intent.PRURL != "" {
		target, targetErr := configuredRepositoryTarget(ctx, config)
		if targetErr != nil || config.PR.ValidateRemote(target) != nil {
			return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", "PR provider configuration does not match the fixed repository target")
		}
	}
	knownSnapshot, err := sqlite.Query(ctx, c.Project)
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	if executionmodel.CandidateBindings(knownSnapshot.WorkflowRuns, knownSnapshot.Executions, taskID) != intent.CandidateBindings {
		return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", "candidate bindings changed before full PR verification")
	}
	known, verifyErr := verifyKnownCandidatePRs(ctx, config, knownSnapshot, taskID, nil, "")
	if verifyErr != nil && !isNativeErrorCode(verifyErr, "already_merged") {
		return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", verifyErr.Error())
	}
	for _, pr := range known {
		if pr.URL == intent.PRURL {
			prInfo = &pr.Info
			break
		}
	}
	if isNativeErrorCode(verifyErr, "already_merged") {
		for _, pr := range known {
			if pr.Info.Merged || pr.Info.State == "merged" {
				prInfo = &pr.Info
				break
			}
		}
	}
	return c.commitDiscard(ctx, sqlite, requestID, staged, intent, prInfo, config.PR.Repository, known)
}

func updateDiscardIntentReceipt(ctx context.Context, s persistence.CommandStore, op *executionmodel.OperationRecord, intent taskDiscardIntent) error {
	data, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	next := *op
	next.Receipt = data
	next.Status = "pending"
	next.UpdatedAt = time.Now().UTC()
	if err = s.UpdateDiscardReceipt(ctx, next); err != nil {
		return err
	}
	*op = next
	return nil
}

func taskDiscardPlanFromStore(ctx context.Context, s persistence.CommandStore, project, taskID string) (executionmodel.DiscardFacts, taskmodel.TaskRecord, error) {
	snapshot, err := s.Query(ctx, project)
	if err != nil {
		return executionmodel.DiscardFacts{}, taskmodel.TaskRecord{}, err
	}
	return taskDiscardPlanFromSnapshot(snapshot, taskID)
}

func taskDiscardPlanFromSnapshot(snapshot DomainSnapshot, taskID string) (executionmodel.DiscardFacts, taskmodel.TaskRecord, error) {
	var task taskmodel.TaskRecord
	found := false
	for _, candidate := range snapshot.Tasks {
		if candidate.TaskID == taskID {
			task = candidate
			found = true
			break
		}
	}
	if !found {
		return executionmodel.DiscardFacts{}, task, nativeErr("task_not_found", "native task was not found")
	}
	return executionmodel.CandidateDiscardFacts(snapshot.WorkflowRuns, snapshot.Executions, taskID), task, nil
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

func persistDiscardPending(ctx context.Context, s persistence.CommandStore, project, requestID string, op executionmodel.OperationRecord, intent taskDiscardIntent, code, message string) (taskmodel.TaskDiscardReceipt, error) {
	receipt := taskmodel.TaskDiscardReceipt{RequestID: requestID, TaskID: intent.TaskID, Status: "pending_verification", Code: code, Message: message, CleanupPending: uniqueSorted(intent.CleanupPending)}
	checkpoint, err := json.Marshal(intent)
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	err = s.TransactDiscardPending(ctx, func(tx persistence.DiscardPendingTx) error {
		op.Status = "outcome_unknown"
		op.Receipt = checkpoint
		op.UnknownReason = message
		op.UpdatedAt = time.Now().UTC()
		if e := tx.UpdateOperation(op); e != nil {
			return e
		}
		raw, _ := json.Marshal(receipt)
		cmd := storageports.CommandRecord{ProjectIdentity: project, CommandID: requestID, Status: "pending_verification", Result: raw}
		if e := tx.UpdateCommand(cmd); e != nil {
			return e
		}
		body, _ := json.Marshal(receipt)
		_, e := tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: project, TaskID: intent.TaskID, ExternalSource: "native-task-command", ExternalID: requestID + ":verification-unknown", Type: "task.discard.verification_unknown", ReceivedAt: time.Now().UTC(), Payload: body})
		return e
	})
	return receipt, err
}

func (c CommandService) commitDiscard(ctx context.Context, s persistence.CommandStore, requestID string, op executionmodel.OperationRecord, intent taskDiscardIntent, info *executionmodel.PRInfo, expectedPRRepository string, known []executionmodel.VerifiedCandidatePR) (taskmodel.TaskDiscardReceipt, error) {
	graph, err := s.TaskGraphSnapshot(ctx, c.Project)
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	before, found := graph.ByID[intent.TaskID]
	if !found {
		return taskmodel.TaskDiscardReceipt{}, nativeErr("task_not_found", "discard task missing")
	}
	facts, _, err := taskDiscardPlanFromStore(ctx, s, c.Project, intent.TaskID)
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	if facts.CandidateBindings != intent.CandidateBindings || facts.PRURL != intent.PRURL || !slices.Equal(facts.PRURLs, intent.PRURLs) || facts.UncertainPR != intent.UncertainPR {
		return taskmodel.TaskDiscardReceipt{}, nativeErr("merge_state_unknown", "candidate bindings changed during discard verification")
	}
	taskIntent := taskmodel.DiscardIntent{TaskID: intent.TaskID, ExpectedVersion: intent.ExpectedVersion, Reason: intent.Reason, RequestedBy: intent.RequestedBy}
	cleanup := append([]string(nil), facts.CleanupPending...)
	for _, pr := range known {
		if pr.Info.State == "open" {
			cleanup = append(cleanup, "pull_request_close:"+pr.URL)
		}
	}
	after, err := c.taskResults.Discard(before, taskIntent, requestID, uniqueSorted(cleanup), time.Now().UTC())
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	return s.CommitTaskDiscard(ctx, storageports.DiscardCommit{Before: before, After: after, Intent: taskIntent, AuthorityVersion: graph.Authority.AuthorityVersion, GraphRevision: graph.Authority.GraphRevision, Facts: facts, Known: known, Operation: op})
}
