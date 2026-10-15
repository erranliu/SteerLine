package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// CommitRunnerDiscard consumes the complete caller candidate set and fresh
// unmerged PR facts. Task state, receipt and stop obligations commit together.
func (s *SQLiteStore) CommitRunnerDiscard(ctx context.Context, in ports.RunnerDiscard) (taskmodel.TaskDiscardReceipt, error) {
	var receipt taskmodel.TaskDiscardReceipt
	if err := taskmodel.ValidateDiscardIntent(in.Intent); err != nil {
		return receipt, err
	}
	if in.Project == "" || in.RequestID == "" {
		return receipt, errors.New("discard requires project and request identity")
	}
	body, _ := json.Marshal(in.Intent)
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		if old, found, err := tx.LookupCommand(ctx, in.Project, in.RequestID); err != nil {
			return err
		} else if found {
			if old.Kind != "runner_discard" || !bytes.Equal(old.Payload, body) || json.Unmarshal(old.Result, &receipt) != nil {
				return nativeErr("idempotency_conflict", "discard request has different original input")
			}
			return errAtomicReplay
		}
		graph, err := tx.TaskGraphSnapshot(ctx, in.Project)
		if err != nil {
			return err
		}
		current, ok := graph.ByID[in.Intent.TaskID]
		if !ok {
			return nativeErr("task_not_found", "task is missing")
		}
		if current.Version != in.Intent.ExpectedVersion || graph.Authority.AuthorityVersion != in.AuthorityVersion || graph.Authority.GraphRevision != in.GraphRevision {
			return nativeErr("stale_version", "task or graph changed during discard verification")
		}
		if _, found := graph.Deliveries[current.TaskID]; found {
			return nativeErr("already_merged", "task already has a verified delivery")
		}
		candidates, err := readRunnerCandidates(ctx, tx.tx, in.Project, current.TaskID)
		if err != nil {
			return err
		}
		if len(candidates) != len(in.CandidateRevisions) {
			return nativeErr("stale_version", "candidate set changed during discard verification")
		}
		stops := []string{}
		for _, c := range candidates {
			if in.CandidateRevisions[c.ID] != c.Revision {
				return nativeErr("stale_version", "candidate changed during discard verification")
			}
			if c.RunID == "" {
				if c.SubmitState != "pending" && c.SubmitState != "cancelled" && c.SubmitState != "rejected" {
					return nativeErr("merge_state_unknown", "original submission may have been accepted")
				}
				continue
			}
			_, pr, err := runnerPRBinding(c, graph.Authority.Branch)
			if err != nil {
				return err
			}
			if in.VerificationStarted.IsZero() || pr.CheckedAt.Before(in.VerificationStarted) || pr.QueryError != "" {
				return nativeErr("merge_state_unknown", "discard requires a fresh query of every known PR")
			}
			if pr.Merged || pr.State == "merged" {
				return nativeErr("already_merged", "candidate PR already merged")
			}
			if pr.State != "open" && pr.State != "closed_unmerged" {
				return nativeErr("merge_state_unknown", "candidate PR merge state is unknown")
			}
			if c.Projection.StopStatus != "stopped" {
				stops = append(stops, "runner_stop:"+c.RunID)
			}
		}
		at := now()
		after, err := taskmodel.Discard(current, in.Intent.Reason, in.Intent.RequestedBy, in.RequestID, stops, at)
		if err != nil {
			return err
		}
		command := ports.CommandRecord{ProjectIdentity: in.Project, CommandID: in.RequestID, Kind: "runner_discard", TargetID: current.TaskID, ExpectedVersion: in.Intent.ExpectedVersion, IdempotencyKey: in.RequestID, RequestedBy: in.Intent.RequestedBy, Payload: body, Status: "pending", CreatedAt: at}
		if _, created, err := tx.EnqueueCommand(command); err != nil {
			return err
		} else if !created {
			return nativeErr("idempotency_conflict", "discard command already exists")
		}
		if err := tx.UpdateNativeTaskCAS(after, current.Version); err != nil {
			return err
		}
		receipt = taskmodel.TaskDiscardReceipt{RequestID: in.RequestID, TaskID: current.TaskID, Status: "discarded", GraphRevision: in.GraphRevision + 1, StopPending: stops}
		command.Status = "accepted"
		command.Result, _ = json.Marshal(receipt)
		command.UpdatedAt = at
		if err := tx.UpdateCommand(command); err != nil {
			return err
		}
		_, err = tx.AppendEvent(ports.DomainEvent{ProjectIdentity: in.Project, TaskID: current.TaskID, ExternalSource: "native-task-command", ExternalID: in.RequestID + ":discarded", Type: "task.discarded", ReceivedAt: at, Payload: command.Result})
		return err
	})
	if errors.Is(err, errAtomicReplay) {
		err = nil
	}
	return receipt, err
}
