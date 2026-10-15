package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	model "github.com/erranliu/SteerLine/internal/execution/model"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// SetRunnerDispatch changes only Task dispatch authority and its caller receipt.
// No worker operation or native execution record is created.
func (s *SQLiteStore) SetRunnerDispatch(ctx context.Context, in ports.RunnerDispatch) (model.ProjectTaskAuthority, error) {
	var result model.ProjectTaskAuthority
	if in.Project == "" || in.RequestID == "" || in.ExpectedAuthorityVersion == 0 || strings.TrimSpace(in.RequestedBy) == "" || in.Mode == nil && in.Paused == nil || in.Mode != nil && *in.Mode != "manual" && *in.Mode != "auto" {
		return result, errors.New("dispatch change requires fixed project/request/version and mode or pause intent")
	}
	body, _ := json.Marshal(in)
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		old, found, err := tx.LookupCommand(ctx, in.Project, in.RequestID)
		if err != nil {
			return err
		}
		if found {
			if old.Kind != "runner_dispatch" || old.TargetID != in.Project || old.RequestedBy != in.RequestedBy || old.ExpectedVersion != in.ExpectedAuthorityVersion || !bytes.Equal(old.Payload, body) {
				return nativeErr("idempotency_conflict", "dispatch request has different original input")
			}
			if old.Status != "accepted" || json.Unmarshal(old.Result, &result) != nil || result.ProjectIdentity != in.Project {
				return errors.New("dispatch receipt is invalid")
			}
			return errAtomicReplay
		}
		graph, err := tx.TaskGraphSnapshot(ctx, in.Project)
		if err != nil {
			return err
		}
		if graph.Authority.AuthorityVersion != in.ExpectedAuthorityVersion {
			return nativeErr("stale_version", "dispatch authority changed")
		}
		result = graph.Authority
		result.AuthorityVersion++
		result.UpdatedAt = now()
		if in.Mode != nil {
			result.DispatchMode = *in.Mode
		}
		if in.Paused != nil {
			result.DispatchPaused = *in.Paused
		}
		command := ports.CommandRecord{ProjectIdentity: in.Project, CommandID: in.RequestID, Kind: "runner_dispatch", TargetID: in.Project, ExpectedVersion: in.ExpectedAuthorityVersion, IdempotencyKey: in.RequestID, RequestedBy: in.RequestedBy, Payload: body, Status: "pending", CreatedAt: result.UpdatedAt}
		if _, created, err := tx.EnqueueCommand(command); err != nil {
			return err
		} else if !created {
			return nativeErr("idempotency_conflict", "dispatch request already exists")
		}
		if err := tx.UpdateProjectTaskAuthorityCAS(result, in.ExpectedAuthorityVersion); err != nil {
			return err
		}
		command.Status = "accepted"
		command.Result, _ = json.Marshal(result)
		command.UpdatedAt = result.UpdatedAt
		return tx.UpdateCommand(command)
	})
	if errors.Is(err, errAtomicReplay) {
		err = nil
	}
	return result, err
}
