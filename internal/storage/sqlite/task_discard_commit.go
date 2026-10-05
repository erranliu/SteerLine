package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (s *SQLiteStore) CommitTaskDiscard(ctx context.Context, commit storageports.DiscardCommit) (taskmodel.TaskDiscardReceipt, error) {
	var receipt taskmodel.TaskDiscardReceipt
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		project, id := commit.Before.ProjectIdentity, commit.Before.TaskID
		command, found, err := tx.LookupCommand(ctx, project, commit.Operation.OperationID)
		if err != nil {
			return err
		}
		canonicalIntent, err := taskmodel.CanonicalJSON(mustResultJSON(commit.Intent))
		if err != nil {
			return err
		}
		if !found || command.Kind != "discard_task" || command.TargetID != id || command.ExpectedVersion != commit.Intent.ExpectedVersion || command.RequestedBy != commit.Intent.RequestedBy || !bytes.Equal(command.Payload, canonicalIntent) {
			return nativeErr("idempotency_conflict", "discard does not match original task intent")
		}
		if command.Status == "accepted" || command.Status == "rejected" {
			if err := json.Unmarshal(command.Result, &receipt); err != nil {
				return err
			}
			return errAtomicReplay
		}
		var operationBytes []byte
		if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM operations WHERE project_identity=? AND operation_id=?`, project, commit.Operation.OperationID).Scan(&operationBytes); err != nil {
			return err
		}
		var currentOperation executionmodel.OperationRecord
		if err := json.Unmarshal(operationBytes, &currentOperation); err != nil {
			return err
		}
		if digestValue(currentOperation) != digestValue(commit.Operation) || currentOperation.Kind != "discard_task" {
			return nativeErr("stale_version", "discard original operation changed")
		}
		snapshot, err := tx.ReadTaskCandidateSnapshot(ctx, project, id)
		if err != nil {
			return err
		}
		graph := snapshot.Graph
		current, found := graph.ByID[id]
		if !found {
			return nativeErr("task_not_found", "discard task missing")
		}
		if graph.Authority.AuthorityVersion != commit.AuthorityVersion || graph.Authority.GraphRevision != commit.GraphRevision || !sameTaskRecord(current, commit.Before) || current.Version != commit.Intent.ExpectedVersion {
			return nativeErr("stale_version", "discard task or graph changed during verification")
		}
		if _, found := graph.Deliveries[id]; found {
			return nativeErr("already_merged", "task has a verified delivery")
		}
		if current.Status != "open" && current.Status != "in_progress" {
			return nativeErr("immutable_task", "task cannot be discarded")
		}
		facts := executionmodel.CandidateDiscardFacts(snapshot.WorkflowRuns, snapshot.Executions, id)
		if facts.UncertainPR || digestValue(facts) != digestValue(commit.Facts) {
			return nativeErr("merge_state_unknown", "candidate publication or resource facts changed")
		}
		verified := map[string]bool{}
		cleanup := append([]string(nil), facts.CleanupPending...)
		for _, pr := range commit.Known {
			if verified[pr.URL] || !slices.Contains(facts.PRURLs, pr.URL) || pr.Info.URL != pr.URL {
				return nativeErr("merge_state_unknown", "discard proof does not cover original PR set")
			}
			verified[pr.URL] = true
			if pr.Info.Merged || pr.Info.State == "merged" {
				receipt = taskmodel.TaskDiscardReceipt{RequestID: command.CommandID, TaskID: id, Status: "rejected", Code: "already_merged", Message: "task PR is already merged", GraphRevision: graph.Authority.GraphRevision}
				return tx.finishDiscardCommit(command, currentOperation, receipt, false)
			}
			if pr.Info.State != "open" && pr.Info.State != "closed_unmerged" || pr.Info.BaseBranch != graph.Authority.Branch {
				return nativeErr("merge_state_unknown", "provider did not confirm the original unmerged PR")
			}
			bound := false
			for _, w := range snapshot.WorkflowRuns {
				if w.TaskID != id || w.WorkflowRunID != pr.WorkflowID {
					continue
				}
				urls := append(append(append([]string(nil), w.PRHistory...), w.PRURL), w.PRCandidateURL)
				if !slices.Contains(urls, pr.URL) {
					continue
				}
				for _, e := range snapshot.Executions {
					if e.ExecutionID == w.SelectedResult && e.WorkflowRunID == w.WorkflowRunID && e.Workspace.Branch != "" && pr.Info.HeadBranch == e.Workspace.Branch && pr.Info.HeadRepository == pr.Info.Repository {
						bound = true
					}
				}
			}
			if !bound {
				return nativeErr("merge_state_unknown", "discard proof lost its fixed candidate identity")
			}
			if pr.Info.State == "open" {
				cleanup = append(cleanup, "pull_request_close:"+pr.URL)
			}
		}
		if len(verified) != len(facts.PRURLs) {
			return nativeErr("merge_state_unknown", "discard requires every known PR proof")
		}
		cleanup = sortedCleanup(cleanup)
		if err := taskmodel.ValidateDiscardIntent(commit.Intent); err != nil {
			return err
		}
		expected, err := taskmodel.Discard(current, commit.Intent.Reason, commit.Intent.RequestedBy, command.CommandID, cleanup, commit.After.UpdatedAt)
		if err != nil {
			return err
		}
		if !sameTaskRecord(expected, commit.After) {
			return nativeErr("contract_mismatch", "discard declaration differs from task intent")
		}
		if err := tx.UpdateNativeTaskCAS(commit.After, current.Version); err != nil {
			return err
		}
		receipt = taskmodel.TaskDiscardReceipt{RequestID: command.CommandID, TaskID: id, Status: "discarded", GraphRevision: graph.Authority.GraphRevision + 1, CleanupPending: cleanup}
		body, _ := json.Marshal(map[string]any{"task_id": id, "request_id": command.CommandID, "reason": commit.Intent.Reason, "discarded_by": commit.Intent.RequestedBy, "discarded_at": commit.After.DiscardedAt, "cleanup_pending": cleanup})
		if _, err := tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: project, TaskID: id, ExternalSource: "native-task-command", ExternalID: command.CommandID + ":discarded", Type: "task.discarded", ReceivedAt: commit.After.UpdatedAt, Payload: body}); err != nil {
			return err
		}
		return tx.finishDiscardCommit(command, currentOperation, receipt, true)
	})
	if err != nil && !errors.Is(err, errAtomicReplay) {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	return receipt, nil
}

func (tx *sqliteTx) finishDiscardCommit(command storageports.CommandRecord, operation executionmodel.OperationRecord, receipt taskmodel.TaskDiscardReceipt, confirmed bool) error {
	command.Status, operation.Status = "rejected", "rejected"
	if confirmed {
		command.Status, operation.Status = "accepted", "confirmed"
	}
	command.Result, _ = json.Marshal(receipt)
	operation.UpdatedAt = now()
	if err := tx.UpdateCommand(command); err != nil {
		return err
	}
	return tx.UpdateOperation(operation)
}

func sortedCleanup(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
