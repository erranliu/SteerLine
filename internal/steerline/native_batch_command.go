package steerline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/task"
)

const taskBatchSchemaVersion = task.BatchSchemaVersion

type TaskBatchInput = task.TaskBatchInput
type TaskBatchTask = task.TaskBatchTask
type TaskBatchReference = task.TaskBatchReference
type TaskBatchEdge = task.TaskBatchEdge
type TaskBatchIssue = task.TaskBatchIssue
type TaskBatchValidation = task.TaskBatchValidation
type TaskBatchReceipt = task.TaskBatchReceipt

func decodeTaskBatch(data []byte) (TaskBatchInput, []byte, error) {
	return task.DecodeBatch(data)
}

func (c CommandService) ValidateTaskBatch(ctx context.Context, config Config, data []byte) (TaskBatchValidation, error) {
	batch, _, err := decodeTaskBatch(data)
	if err != nil {
		return TaskBatchValidation{}, err
	}
	if c.Store == nil || c.Project == "" || config.Identity() != c.Project || batch.ProjectID != c.Project {
		return TaskBatchValidation{}, nativeErr("authority_mismatch", "batch project_id, command target and configured project must match")
	}
	snapshot, err := taskGraphFromStore(ctx, c.Store, c.Project)
	if err != nil {
		return TaskBatchValidation{}, err
	}
	if err = validateNativeConfiguredTarget(ctx, config, snapshot.Authority); err != nil {
		return TaskBatchValidation{}, err
	}
	result := TaskBatchValidation{ProjectID: c.Project, TaskCount: len(batch.Tasks), DependencyCount: len(batch.Dependencies), AuthorityVersion: snapshot.Authority.AuthorityVersion, GraphRevision: snapshot.Authority.GraphRevision, DatabaseRevision: snapshot.DatabaseRevision, DatabaseID: snapshot.DatabaseID, SampledAt: snapshot.SampledAt, Issues: []TaskBatchIssue{}}
	issues, cycle := validateTaskBatchAgainstGraph(ctx, config, batch, snapshot, true)
	result.Issues, result.Cycle = issues, cycle
	result.Valid = len(issues) == 0
	return result, nil
}

func validateTaskBatchAgainstGraph(ctx context.Context, config Config, batch TaskBatchInput, graph taskGraphSnapshot, verifyFiles bool) ([]TaskBatchIssue, []string) {
	var verify func(TaskContract) error = ValidateTaskContract
	if verifyFiles {
		verify = func(contract TaskContract) error { return VerifyTaskContractFiles(ctx, config, contract) }
	}
	return task.ValidateBatchAgainstGraph(batch, graph, config.ExecutableTypes, verify)
}

type taskBatchReplay struct{ Receipt TaskBatchReceipt }

func (e taskBatchReplay) Error() string { return "task batch command already applied" }

// CreateTaskBatch verifies the fixed Git references before entering SQLite,
// then persists the batch, tasks, contracts, edges, event and command receipt
// in one center transaction.
func (c CommandService) CreateTaskBatch(ctx context.Context, config Config, data []byte, requestID string, expectedAuthorityVersion uint64, requestedBy string) (TaskBatchReceipt, error) {
	if err := validateWorkflowConfig(&config, nil); err != nil {
		return TaskBatchReceipt{}, err
	}
	batch, canonicalPayload, err := decodeTaskBatch(data)
	if err != nil {
		return TaskBatchReceipt{}, err
	}
	sqlite := c.Store
	if sqlite == nil || c.Project == "" || config.Identity() != c.Project || batch.ProjectID != c.Project {
		return TaskBatchReceipt{}, nativeErr("authority_mismatch", "batch project_id, command target and configured project must match")
	}
	if !controlIDPattern.MatchString(requestID) || expectedAuthorityVersion == 0 {
		return TaskBatchReceipt{}, nativeErr("invalid_batch", "create requires a valid request_id and expected authority version")
	}
	if strings.TrimSpace(requestedBy) == "" {
		requestedBy = DefaultRequestedBy()
	}
	if existing, found, e := sqlite.lookupNativeCommand(ctx, c.Project, requestID); e != nil {
		return TaskBatchReceipt{}, e
	} else if found {
		return replayTaskBatch(existing, canonicalPayload, batch.ProjectID, requestedBy)
	}
	validation, err := c.ValidateTaskBatch(ctx, config, data)
	if err != nil {
		return TaskBatchReceipt{}, err
	}
	if !validation.Valid {
		return TaskBatchReceipt{}, taskBatchValidationError(validation.Issues, validation.Cycle)
	}
	if validation.AuthorityVersion != expectedAuthorityVersion {
		return TaskBatchReceipt{}, nativeErr("stale_version", fmt.Sprintf("expected authority version %d, sampled %d", expectedAuthorityVersion, validation.AuthorityVersion))
	}
	payloadDigest := digestCanonical(canonicalPayload)
	var receipt TaskBatchReceipt
	err = c.Store.transactSQLite(ctx, func(tx *sqliteTx) error {
		if old, found, e := tx.lookupNativeCommand(ctx, c.Project, requestID); e != nil {
			return e
		} else if found {
			r, e := replayTaskBatch(old, canonicalPayload, batch.ProjectID, requestedBy)
			if e != nil {
				return e
			}
			receipt = r
			return taskBatchReplay{Receipt: r}
		}
		graph, e := taskGraphSnapshotTx(ctx, tx, c.Project)
		if e != nil {
			return e
		}
		if graph.Authority.AuthorityVersion != expectedAuthorityVersion {
			return nativeErr("stale_version", fmt.Sprintf("expected authority version %d, found %d", expectedAuthorityVersion, graph.Authority.AuthorityVersion))
		}
		issues, cycle := validateTaskBatchAgainstGraph(ctx, config, batch, graph, false)
		if len(issues) > 0 {
			return taskBatchValidationError(issues, cycle)
		}
		command := CommandRecord{ProjectIdentity: c.Project, CommandID: requestID, Kind: "create_task_batch", TargetID: c.Project, ExpectedVersion: expectedAuthorityVersion, IdempotencyKey: requestID, RequestedBy: requestedBy, Payload: append([]byte(nil), canonicalPayload...), Status: "in_flight", CreatedAt: time.Now().UTC()}
		createdCommand, created, e := tx.EnqueueCommand(command)
		if e != nil {
			return nativeErr("idempotency_conflict", e.Error())
		}
		if !created {
			return nativeErr("idempotency_conflict", "request_id already belongs to another command")
		}
		command = createdCommand
		batchID := newID()
		createdAt := time.Now().UTC()
		plan := task.PlanBatch(batch, batchID, requestID, payloadDigest, createdAt)
		mapping := plan.Batch.TaskIDs
		batchRecord := plan.Batch
		if e = tx.PutTaskBatch(batchRecord, expectedAuthorityVersion, graph.Authority.GraphRevision); e != nil {
			return e
		}
		for i, input := range batch.Tasks {
			task := plan.Tasks[i]
			id := task.TaskID
			if e = tx.PutTask(task); e != nil {
				return e
			}
			if e = tx.PutTaskContract(c.Project, id, input.Contract); e != nil {
				return e
			}
			body, _ := json.Marshal(map[string]any{"batch_id": batchID, "local_key": input.LocalKey, "task_id": id, "contract_digest": input.Contract.Digest, "payload_digest": payloadDigest})
			if _, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: id, ExternalSource: "native-task-command", ExternalID: requestID + ":" + input.LocalKey, Type: "task.created", ReceivedAt: createdAt, Payload: body}); e != nil {
				return e
			}
		}
		for _, edge := range plan.Edges {
			if e = tx.PutTaskDependency(edge); e != nil {
				if strings.Contains(strings.ToLower(e.Error()), "unique") {
					return nativeErr("duplicate_dependency", "dependency edge is already present")
				}
				return e
			}
		}
		graphRevision := graph.Authority.GraphRevision + 1
		body, _ := json.Marshal(map[string]any{"batch_id": batchID, "task_ids": mapping, "graph_revision": graphRevision, "payload_digest": payloadDigest})
		if _, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, ExternalSource: "native-task-command", ExternalID: requestID, Type: "task.batch.created", ReceivedAt: createdAt, Payload: body}); e != nil {
			return e
		}
		receipt = TaskBatchReceipt{RequestID: requestID, BatchID: batchID, TaskIDs: mapping, GraphRevision: graphRevision, PayloadDigest: payloadDigest, Status: "accepted", DispatchNote: "Task creation does not start work. If dispatch is already auto and unpaused, normal polling may consider these tasks."}
		command.Result, _ = json.Marshal(receipt)
		command.Status = "accepted"
		if e = tx.UpdateCommand(command); e != nil {
			return e
		}
		return nil
	})
	var replay taskBatchReplay
	if errors.As(err, &replay) {
		return replay.Receipt, nil
	}
	if err != nil {
		return TaskBatchReceipt{}, err
	}
	return receipt, nil
}

func resolveTaskBatchEdges(batch TaskBatchInput) []TaskDependencyRecord {
	return task.ResolveBatchEdges(batch)
}

func taskBatchValidationError(issues []TaskBatchIssue, cycle []string) error {
	return task.BatchValidationError(issues, cycle)
}

func replayTaskBatch(old CommandRecord, payload []byte, project, requestedBy string) (TaskBatchReceipt, error) {
	if old.Kind != "create_task_batch" || old.TargetID != project || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, payload) {
		return TaskBatchReceipt{}, nativeErr("idempotency_conflict", "request_id was already used with different command content")
	}
	if old.Status != "accepted" || len(old.Result) == 0 {
		return TaskBatchReceipt{}, nativeErr("idempotency_conflict", "request_id has no committed task batch receipt")
	}
	var receipt TaskBatchReceipt
	if err := json.Unmarshal(old.Result, &receipt); err != nil {
		return receipt, nativeErr("unsupported_schema", "saved task batch receipt is unreadable")
	}
	if receipt.RequestID != old.CommandID || receipt.Status != "accepted" {
		return TaskBatchReceipt{}, nativeErr("unsupported_schema", "saved task batch receipt identity is inconsistent")
	}
	return receipt, nil
}

func digestCanonical(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
