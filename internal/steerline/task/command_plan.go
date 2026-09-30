package task

import (
	"strconv"
	"strings"
	"time"
)

// BatchPlan is the task record set committed by one center transaction.
type BatchPlan struct {
	Batch TaskBatchRecord
	Tasks []TaskRecord
	Edges []TaskDependencyRecord
}

// PlanBatch builds records from a batch already validated against one snapshot.
func PlanBatch(batch TaskBatchInput, batchID, commandID, payloadDigest string, at time.Time) BatchPlan {
	mapping := make(map[string]string, len(batch.Tasks))
	records := make([]TaskRecord, 0, len(batch.Tasks))
	for _, input := range batch.Tasks {
		id := strconv.FormatInt(input.TaskID, 10)
		mapping[input.LocalKey] = id
		priority := 2
		if input.Priority != nil {
			priority = *input.Priority
		}
		records = append(records, TaskRecord{
			ProjectIdentity: batch.ProjectID, TaskID: id, BatchID: batchID, Title: input.Title, Type: input.Type,
			Priority: priority, GoalIDs: append([]string(nil), input.GoalIDs...), ContractID: input.Contract.ContractID,
			Source: "native", Status: "open", Version: 1, CreatedAt: at, UpdatedAt: at,
		})
	}
	return BatchPlan{
		Batch: TaskBatchRecord{BatchID: batchID, ProjectIdentity: batch.ProjectID, SchemaVersion: BatchSchemaVersion, PayloadDigest: payloadDigest, CreatedAt: at, CommandID: commandID, TaskIDs: mapping},
		Tasks: records, Edges: ResolveBatchEdges(batch),
	}
}

func Claim(record TaskRecord, claimant string, at time.Time) (TaskRecord, error) {
	if strings.TrimSpace(claimant) == "" {
		return TaskRecord{}, newError("contract_mismatch", "task claim requires a claimant")
	}
	if record.Status != "open" {
		return TaskRecord{}, newError("stale_version", "only an open task can be claimed")
	}
	next := record
	if err := SetStatus(&next, "in_progress", false, at); err != nil {
		return TaskRecord{}, err
	}
	next.Claimant = claimant
	return next, nil
}

// CompleteDelivery closes a claimed task only with matching merge evidence.
func CompleteDelivery(record TaskRecord, delivery TaskDeliveryRecord, targetBranch string, at time.Time) (TaskRecord, error) {
	if record.Status != "in_progress" {
		return TaskRecord{}, newError("stale_version", "only an in-progress task can be delivered")
	}
	if delivery.ProjectIdentity != record.ProjectIdentity || delivery.TaskID != record.TaskID {
		return TaskRecord{}, newError("inconsistent_delivery", "delivery belongs to another task")
	}
	if err := ValidateDelivery(delivery, targetBranch); err != nil {
		return TaskRecord{}, err
	}
	next := record
	if err := SetStatus(&next, "closed", false, at); err != nil {
		return TaskRecord{}, err
	}
	return next, nil
}

func Discard(record TaskRecord, reason, requestedBy, requestID string, cleanup []string, mergedPR bool, at time.Time) (TaskRecord, error) {
	if record.Status != "open" && record.Status != "in_progress" {
		return TaskRecord{}, newError("immutable_task", "only an open or in-progress task can be discarded")
	}
	if strings.TrimSpace(reason) == "" || strings.TrimSpace(requestedBy) == "" || requestID == "" {
		return TaskRecord{}, newError("immutable_task", "discard requires reason, requester and request identity")
	}
	next := record
	if err := SetStatus(&next, "discarded", mergedPR, at); err != nil {
		return TaskRecord{}, err
	}
	next.DiscardedAt = &at
	next.DiscardReason = reason
	next.DiscardedBy = requestedBy
	next.DiscardRequestID = requestID
	next.CleanupPending = append([]string(nil), cleanup...)
	return next, nil
}
