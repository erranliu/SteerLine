package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	tasklegacy "github.com/erranliu/SteerLine/internal/steerline/task"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type NativeDispatchUpdate struct {
	RequestID                string
	ExpectedAuthorityVersion uint64
	Mode                     *string
	Paused                   *bool
	RequestedBy              string
}

func (c CommandService) SetNativeDispatchState(ctx context.Context, update NativeDispatchUpdate) (tasklegacy.ProjectTaskAuthority, error) {
	sqlite := c.Store
	if sqlite == nil || c.Project == "" {
		return tasklegacy.ProjectTaskAuthority{}, nativeErr("authority_mismatch", "dispatch control requires the center SQLite project")
	}
	if !controlIDPattern.MatchString(update.RequestID) || update.ExpectedAuthorityVersion == 0 {
		return tasklegacy.ProjectTaskAuthority{}, nativeErr("invalid_batch", "dispatch control requires request ID and sampled authority version")
	}
	if update.Mode == nil && update.Paused == nil {
		return tasklegacy.ProjectTaskAuthority{}, nativeErr("invalid_batch", "dispatch control must change mode or pause state")
	}
	if update.Mode != nil && *update.Mode != "manual" && *update.Mode != "auto" {
		return tasklegacy.ProjectTaskAuthority{}, nativeErr("invalid_batch", "dispatch mode must be auto or manual")
	}
	if strings.TrimSpace(update.RequestedBy) == "" {
		update.RequestedBy = DefaultRequestedBy()
	}
	requestBytes, err := json.Marshal(struct {
		Mode        *string `json:"mode,omitempty"`
		Paused      *bool   `json:"paused,omitempty"`
		RequestedBy string  `json:"requested_by"`
	}{update.Mode, update.Paused, update.RequestedBy})
	if err != nil {
		return tasklegacy.ProjectTaskAuthority{}, err
	}
	requestPayload, err := taskmodel.CanonicalJSON(requestBytes)
	if err != nil {
		return tasklegacy.ProjectTaskAuthority{}, err
	}
	if old, found, lookupErr := sqlite.LookupCommand(ctx, c.Project, update.RequestID); lookupErr != nil {
		return tasklegacy.ProjectTaskAuthority{}, lookupErr
	} else if found {
		return replayNativeDispatchState(old, requestPayload, c.Project, update.RequestedBy)
	}
	graph, err := sqlite.TaskGraphSnapshot(ctx, c.Project)
	if err != nil {
		return tasklegacy.ProjectTaskAuthority{}, err
	}
	if graph.Authority.AuthorityVersion != update.ExpectedAuthorityVersion {
		return tasklegacy.ProjectTaskAuthority{}, nativeErr("stale_version", fmt.Sprintf("expected authority version %d, found %d", update.ExpectedAuthorityVersion, graph.Authority.AuthorityVersion))
	}
	next := graph.Authority
	next.AuthorityVersion++
	if update.Mode != nil {
		next.DispatchMode = *update.Mode
	}
	if update.Paused != nil {
		next.DispatchPaused = *update.Paused
	}
	next.UpdatedAt = time.Now().UTC()
	var result tasklegacy.ProjectTaskAuthority
	err = sqlite.TransactDispatch(ctx, func(tx persistence.DispatchTx) error {
		if old, found, lookupErr := tx.LookupCommand(ctx, c.Project, update.RequestID); lookupErr != nil {
			return lookupErr
		} else if found {
			value, replayErr := replayNativeDispatchState(old, requestPayload, c.Project, update.RequestedBy)
			if replayErr != nil {
				return replayErr
			}
			result = value
			return taskBatchReplay{Receipt: taskmodel.TaskBatchReceipt{RequestID: update.RequestID, Status: "accepted"}}
		}
		current, readErr := tx.TaskGraphSnapshot(ctx, c.Project)
		if readErr != nil {
			return readErr
		}
		if current.Authority.AuthorityVersion != update.ExpectedAuthorityVersion {
			return nativeErr("stale_version", "native dispatch authority changed during control preflight")
		}
		command := storageports.CommandRecord{ProjectIdentity: c.Project, CommandID: update.RequestID, Kind: "set_dispatch_state", TargetID: c.Project, ExpectedVersion: update.ExpectedAuthorityVersion, IdempotencyKey: update.RequestID, RequestedBy: update.RequestedBy, Payload: requestPayload, Status: "in_flight", CreatedAt: next.UpdatedAt}
		if _, made, enqueueErr := tx.EnqueueCommand(command); enqueueErr != nil {
			return nativeErr("idempotency_conflict", enqueueErr.Error())
		} else if !made {
			return nativeErr("idempotency_conflict", "dispatch request ID is already in use")
		}
		operation := executionmodel.OperationRecord{ProjectIdentity: c.Project, OperationID: update.RequestID, IdempotencyKey: update.RequestID, Kind: "set_dispatch_state", Status: "confirmed", PayloadDigest: digestCanonical(requestPayload), Request: requestPayload, CreatedAt: next.UpdatedAt, UpdatedAt: next.UpdatedAt}
		if _, made, putErr := tx.PutOperation(operation); putErr != nil {
			return putErr
		} else if !made {
			return nativeErr("idempotency_conflict", "dispatch operation request ID is already in use")
		}
		if writeErr := tx.UpdateProjectTaskAuthorityCAS(next, update.ExpectedAuthorityVersion); writeErr != nil {
			return writeErr
		}
		command.Status = "accepted"
		command.Result, _ = json.Marshal(next)
		command.UpdatedAt = next.UpdatedAt
		if writeErr := tx.UpdateCommand(command); writeErr != nil {
			return writeErr
		}
		result = next
		body, _ := json.Marshal(next)
		_, writeErr := tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: c.Project, ExternalSource: "native-task-command", ExternalID: update.RequestID, Type: "project.dispatch.updated", ReceivedAt: next.UpdatedAt, Payload: body})
		return writeErr
	})
	var replay taskBatchReplay
	if errors.As(err, &replay) {
		return result, nil
	}
	return result, err
}

func replayNativeDispatchState(old storageports.CommandRecord, requestPayload []byte, project, requestedBy string) (tasklegacy.ProjectTaskAuthority, error) {
	if old.ProjectIdentity != project || old.Kind != "set_dispatch_state" || old.IdempotencyKey == "" || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, requestPayload) || old.Status != "accepted" || len(old.Result) == 0 {
		return tasklegacy.ProjectTaskAuthority{}, nativeErr("idempotency_conflict", "dispatch request ID was used for a different command")
	}
	var result tasklegacy.ProjectTaskAuthority
	if err := decode(old.Result, &result); err != nil || result.ProjectIdentity != project {
		return tasklegacy.ProjectTaskAuthority{}, nativeErr("unsupported_schema", "saved dispatch command receipt is invalid")
	}
	return result, nil
}
