package task

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

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (c Commands) ValidateTaskBatch(ctx context.Context, data []byte) (taskmodel.TaskBatchValidation, error) {
	batch, _, err := taskmodel.DecodeBatch(data)
	if err != nil {
		return taskmodel.TaskBatchValidation{}, err
	}
	if c.Store == nil || c.Input.Project == "" || batch.ProjectID != c.Input.Project {
		return taskmodel.TaskBatchValidation{}, taskError("authority_mismatch", "batch project_id, command target and configured project must match")
	}
	snapshot, err := c.readGraph(ctx)
	if err != nil {
		return taskmodel.TaskBatchValidation{}, err
	}
	result := taskmodel.TaskBatchValidation{ProjectID: c.Input.Project, TaskCount: len(batch.Tasks), DependencyCount: len(batch.Dependencies), AuthorityVersion: snapshot.Authority.AuthorityVersion, GraphRevision: snapshot.Authority.GraphRevision, DatabaseRevision: snapshot.DatabaseRevision, DatabaseID: snapshot.DatabaseID, SampledAt: snapshot.SampledAt, Issues: []taskmodel.TaskBatchIssue{}}
	issues, cycle := c.validateTaskBatchAgainstGraph(ctx, batch, snapshot, true)
	result.Issues, result.Cycle = issues, cycle
	result.Valid = len(issues) == 0
	return result, nil
}

func (c Commands) validateTaskBatchAgainstGraph(ctx context.Context, batch taskmodel.TaskBatchInput, graph taskmodel.Snapshot, verifyFiles bool) ([]taskmodel.TaskBatchIssue, []string) {
	var verify func(taskmodel.TaskContract) error = taskmodel.ValidateContract
	if verifyFiles {
		service := Service{Input: c.Input, Documents: c.Documents}
		verify = func(contract taskmodel.TaskContract) error { return service.VerifyContractFiles(ctx, contract) }
	}
	return taskmodel.ValidateBatchAgainstGraph(batch, graph, c.Input.ExecutableTypes, verify)
}

type taskBatchReplay struct{ Receipt taskmodel.TaskBatchReceipt }

func (e taskBatchReplay) Error() string { return "task batch command already applied" }

// CreateTaskBatch verifies the fixed Git references before entering SQLite,
// then persists the batch, tasks, contracts, edges, event and command receipt
// in one center transaction.
func (c Commands) CreateTaskBatch(ctx context.Context, data []byte, requestID string, expectedAuthorityVersion uint64, requestedBy string) (taskmodel.TaskBatchReceipt, error) {
	if err := c.validateInput(); err != nil {
		return taskmodel.TaskBatchReceipt{}, err
	}
	batch, canonicalPayload, err := taskmodel.DecodeBatch(data)
	if err != nil {
		return taskmodel.TaskBatchReceipt{}, err
	}
	store := c.Store
	if store == nil || c.Input.Project == "" || batch.ProjectID != c.Input.Project {
		return taskmodel.TaskBatchReceipt{}, taskError("authority_mismatch", "batch project_id, command target and configured project must match")
	}
	if !controlIDPattern.MatchString(requestID) || expectedAuthorityVersion == 0 {
		return taskmodel.TaskBatchReceipt{}, taskError("invalid_batch", "create requires a valid request_id and expected authority version")
	}
	if strings.TrimSpace(requestedBy) == "" {
		requestedBy = c.Input.DefaultRequestedBy
	}
	if existing, found, e := store.LookupCommand(ctx, c.Input.Project, requestID); e != nil {
		return taskmodel.TaskBatchReceipt{}, e
	} else if found {
		return replayTaskBatch(existing, canonicalPayload, batch.ProjectID, requestedBy)
	}
	validation, err := c.ValidateTaskBatch(ctx, data)
	if err != nil {
		return taskmodel.TaskBatchReceipt{}, err
	}
	if !validation.Valid {
		return taskmodel.TaskBatchReceipt{}, taskmodel.BatchValidationError(validation.Issues, validation.Cycle)
	}
	if validation.AuthorityVersion != expectedAuthorityVersion {
		return taskmodel.TaskBatchReceipt{}, taskError("stale_version", fmt.Sprintf("expected authority version %d, sampled %d", expectedAuthorityVersion, validation.AuthorityVersion))
	}
	payloadDigest := digestCanonical(canonicalPayload)
	var receipt taskmodel.TaskBatchReceipt
	err = c.Store.TransactTaskBatch(ctx, func(tx BatchTx) error {
		if old, found, e := tx.LookupCommand(ctx, c.Input.Project, requestID); e != nil {
			return e
		} else if found {
			r, e := replayTaskBatch(old, canonicalPayload, batch.ProjectID, requestedBy)
			if e != nil {
				return e
			}
			receipt = r
			return taskBatchReplay{Receipt: r}
		}
		graph, e := tx.TaskSnapshot(ctx, c.Input.Project)
		if e != nil {
			return e
		}
		if graph.Authority.AuthorityVersion != expectedAuthorityVersion {
			return taskError("stale_version", fmt.Sprintf("expected authority version %d, found %d", expectedAuthorityVersion, graph.Authority.AuthorityVersion))
		}
		issues, cycle := c.validateTaskBatchAgainstGraph(ctx, batch, graph, false)
		if len(issues) > 0 {
			return taskmodel.BatchValidationError(issues, cycle)
		}
		command := Command{ProjectIdentity: c.Input.Project, CommandID: requestID, Kind: "create_task_batch", TargetID: c.Input.Project, ExpectedVersion: expectedAuthorityVersion, IdempotencyKey: requestID, RequestedBy: requestedBy, Payload: append([]byte(nil), canonicalPayload...), Status: "in_flight", CreatedAt: time.Now().UTC()}
		createdCommand, created, e := tx.EnqueueCommand(command)
		if e != nil {
			return taskError("idempotency_conflict", e.Error())
		}
		if !created {
			return taskError("idempotency_conflict", "request_id already belongs to another command")
		}
		command = createdCommand
		batchID := newID()
		createdAt := time.Now().UTC()
		plan := taskmodel.PlanBatch(batch, batchID, requestID, payloadDigest, createdAt)
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
			if e = tx.PutTaskContract(c.Input.Project, id, input.Contract); e != nil {
				return e
			}
			body, _ := json.Marshal(map[string]any{"batch_id": batchID, "local_key": input.LocalKey, "task_id": id, "contract_digest": input.Contract.Digest, "payload_digest": payloadDigest})
			if _, e = tx.AppendEvent(Event{ProjectIdentity: c.Input.Project, TaskID: id, ExternalSource: "native-task-command", ExternalID: requestID + ":" + input.LocalKey, Type: "task.created", ReceivedAt: createdAt, Payload: body}); e != nil {
				return e
			}
		}
		for _, edge := range plan.Edges {
			if e = tx.PutTaskDependency(edge); e != nil {
				if strings.Contains(strings.ToLower(e.Error()), "unique") {
					return taskError("duplicate_dependency", "dependency edge is already present")
				}
				return e
			}
		}
		graphRevision := graph.Authority.GraphRevision + 1
		body, _ := json.Marshal(map[string]any{"batch_id": batchID, "task_ids": mapping, "graph_revision": graphRevision, "payload_digest": payloadDigest})
		if _, e = tx.AppendEvent(Event{ProjectIdentity: c.Input.Project, ExternalSource: "native-task-command", ExternalID: requestID, Type: "task.batch.created", ReceivedAt: createdAt, Payload: body}); e != nil {
			return e
		}
		receipt = taskmodel.TaskBatchReceipt{RequestID: requestID, BatchID: batchID, TaskIDs: mapping, GraphRevision: graphRevision, PayloadDigest: payloadDigest, Status: "accepted", DispatchNote: "Task creation does not start work. If dispatch is already auto and unpaused, normal polling may consider these tasks."}
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
		return taskmodel.TaskBatchReceipt{}, err
	}
	return receipt, nil
}

func replayTaskBatch(old Command, payload []byte, project, requestedBy string) (taskmodel.TaskBatchReceipt, error) {
	if old.Kind != "create_task_batch" || old.TargetID != project || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, payload) {
		return taskmodel.TaskBatchReceipt{}, taskError("idempotency_conflict", "request_id was already used with different command content")
	}
	if old.Status != "accepted" || len(old.Result) == 0 {
		return taskmodel.TaskBatchReceipt{}, taskError("idempotency_conflict", "request_id has no committed task batch receipt")
	}
	var receipt taskmodel.TaskBatchReceipt
	if err := json.Unmarshal(old.Result, &receipt); err != nil {
		return receipt, taskError("unsupported_schema", "saved task batch receipt is unreadable")
	}
	if receipt.RequestID != old.CommandID || receipt.Status != "accepted" {
		return taskmodel.TaskBatchReceipt{}, taskError("unsupported_schema", "saved task batch receipt identity is inconsistent")
	}
	return receipt, nil
}

func digestCanonical(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
