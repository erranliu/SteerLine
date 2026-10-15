package taskrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	model "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type ControlStore interface {
	Store
	LookupCommand(context.Context, string, string) (ports.CommandRecord, bool, error)
	EnqueueCommand(context.Context, ports.CommandRecord) (ports.CommandRecord, bool, error)
	UpdateCommand(context.Context, ports.CommandRecord) error
	SetRunnerDispatch(context.Context, ports.RunnerDispatch) (model.ProjectTaskAuthority, error)
}
type Controls struct {
	Tasks *Service
	Store ControlStore
	// ResolveProfile validates the caller's current profile ID and digest. No
	// native profile, policy or lifecycle port is reachable through this callback.
	ResolveProfile func(string, string) (string, error)
}

func (c Controls) Handle(ctx context.Context, req api.ControlRequest, instance string, revision uint64) (api.ControlReceipt, error) {
	now := time.Now().UTC()
	receipt := api.ControlReceipt{Version: 1, RequestID: req.RequestID, RequestDigest: RequestDigest(req), InstanceID: req.InstanceID, Action: req.Action, Status: "rejected", RequestedBy: req.RequestedBy, SubmittedAt: req.CreatedAt, CompletedAt: &now, ControlRevision: revision}
	if c.Tasks == nil || c.Store == nil || c.ResolveProfile == nil {
		return receipt, errors.New("task controls are not composed")
	}
	body, _ := json.Marshal(req)
	old, found, err := c.Store.LookupCommand(ctx, c.Tasks.Project, req.RequestID)
	if err != nil {
		return receipt, err
	}
	if found {
		if old.Kind != "runner_control" || !bytes.Equal(old.Payload, body) {
			receipt.Code = "idempotency_conflict"
			receipt.Message = "control ID belongs to different original input"
			return receipt, nil
		}
		if len(old.Result) > 0 {
			var saved api.ControlReceipt
			if json.Unmarshal(old.Result, &saved) != nil || saved.RequestDigest != receipt.RequestDigest {
				return receipt, errors.New("original control receipt is invalid")
			}
			return saved, nil
		}
	}
	payload, decodeErr := DecodeControlPayload(req)
	code, validationErr := ValidateControlRequest(req, instance, c.Tasks.Project, revision, now)
	if decodeErr != nil {
		code = "invalid_request"
		validationErr = decodeErr
	}
	// An already reserved start is an accepted effect, even if the process
	// stopped before writing its outer receipt. Recover that fact by original ID.
	if found && (req.Action == "start_task" || req.Action == "start_candidate") {
		if candidate, err := c.Store.Candidate(ctx, c.Tasks.Project, req.RequestID); err == nil && candidate.TaskID == payload.TaskID {
			receipt.Status = "applied"
			receipt.RunID = candidate.ID
			receipt.RunRevision = candidate.Revision
			return c.finish(ctx, old, receipt)
		}
	}
	if found && req.Action == "stop_candidate" {
		if candidate, err := c.Store.Candidate(ctx, c.Tasks.Project, payload.RunID); err == nil && candidate.TaskID == payload.TaskID && candidate.StopRequested {
			receipt.Status = "applied"
			receipt.RunID = candidate.ID
			receipt.RunRevision = candidate.Revision
			return c.finish(ctx, old, receipt)
		}
	}
	if found && (req.Action == "runner_reply" || req.Action == "runner_resume") {
		if candidate, err := c.Store.Candidate(ctx, c.Tasks.Project, payload.RunID); err == nil {
			for _, action := range candidate.Actions {
				if action.Request.RequestID == "sl-input-"+req.RequestID {
					receipt.Status = "applied"
					receipt.RunID = candidate.ID
					receipt.RunRevision = candidate.Revision
					return c.finish(ctx, old, receipt)
				}
			}
		}
	}
	if found && (req.Action == "set_dispatch_mode" || req.Action == "pause_dispatch" || req.Action == "resume_dispatch") {
		if saved, present, err := c.Store.LookupCommand(ctx, c.Tasks.Project, "runner-dispatch-"+req.RequestID); err != nil {
			return receipt, err
		} else if present && saved.Status == "accepted" {
			if err := c.apply(ctx, req, payload, &receipt); err != nil {
				return receipt, err
			}
			receipt.Status = "applied"
			return c.finish(ctx, old, receipt)
		}
	}
	if validationErr != nil {
		receipt.Code = code
		receipt.Message = validationErr.Error()
		if found {
			return c.finish(ctx, old, receipt)
		}
		return receipt, nil
	}
	if !found {
		old = ports.CommandRecord{ProjectIdentity: c.Tasks.Project, CommandID: req.RequestID, Kind: "runner_control", TargetID: c.Tasks.Project, ExpectedVersion: req.ExpectedControlRevision, IdempotencyKey: req.RequestID, RequestedBy: req.RequestedBy, Payload: body, Status: "pending", CreatedAt: now}
		old, _, err = c.Store.EnqueueCommand(ctx, old)
		if err != nil {
			return receipt, err
		}
	}
	receipt.AcceptedAt = &now
	err = c.apply(ctx, req, payload, &receipt)
	if err == nil {
		receipt.Status = "applied"
	} else {
		var domain *taskmodel.Error
		if !errors.As(err, &domain) {
			return receipt, err
		}
		receipt.Status = "rejected"
		receipt.Code = domain.Code
		receipt.Message = domain.Message
	}
	return c.finish(ctx, old, receipt)
}
func (c Controls) finish(ctx context.Context, command ports.CommandRecord, receipt api.ControlReceipt) (api.ControlReceipt, error) {
	command.Status = receipt.Status
	command.Result, _ = json.Marshal(receipt)
	command.UpdatedAt = time.Now().UTC()
	return receipt, c.Store.UpdateCommand(ctx, command)
}
func (c Controls) apply(ctx context.Context, req api.ControlRequest, payload api.ControlPayload, receipt *api.ControlReceipt) error {
	switch req.Action {
	case "runner_reply", "runner_resume":
		candidate, err := c.Store.Candidate(ctx, c.Tasks.Project, payload.RunID)
		if err != nil {
			return err
		}
		if candidate.Revision != payload.ExpectedRunRevision {
			return failure("stale_version", "candidate changed before explicit input")
		}
		command := "reply"
		if req.Action == "runner_resume" {
			command = "resume"
		}
		body := json.RawMessage(*payload.RunnerInput)
		if payload.CallerAnswer != nil {
			if payload.CallerAnswer.ServiceInstanceID != req.InstanceID || payload.CallerAnswer.AnsweredBy != req.RequestedBy {
				return failure("invalid_request", "answer belongs to another service or requester")
			}
			want, fixed, err := InputForAnswer(candidate, *payload.CallerAnswer)
			if err != nil {
				return err
			}
			var actualValue, fixedValue any
			if json.Unmarshal(body, &actualValue) != nil || json.Unmarshal(fixed, &fixedValue) != nil || want != command || digest(actualValue) != digest(fixedValue) {
				return failure("invalid_request", "public Runner input differs from the original answer")
			}
		}
		err = c.Tasks.Bridge.QueueControl(ctx, c.Tasks.Project, candidate.ID, command, runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "sl-input-" + req.RequestID, Input: body})
		var validation *runnerbridge.InputError
		if errors.As(err, &validation) {
			return failure(validation.Code, validation.Message)
		}
		if err == nil {
			receipt.RunID = candidate.ID
			receipt.Message = "explicit input saved; native acceptance is observed separately"
		}
		return err
	case "start_task", "start_candidate":
		if payload.ExpectedTaskVersion == 0 {
			return failure("invalid_request", "start requires the displayed task version")
		}
		profileID, err := c.ResolveProfile(payload.ExecutionProfileID, payload.ExecutionProfileDigest)
		if err != nil {
			return err
		}

		candidate, err := c.Tasks.Start(ctx, Start{RequestID: req.RequestID, TaskID: payload.TaskID, TaskVersion: payload.ExpectedTaskVersion, GraphRevision: payload.ExpectedGraphRevision, ProfileID: profileID, Candidate: req.Action == "start_candidate", Explicit: true})
		if err == nil {
			receipt.RunID = candidate.ID
			receipt.RunRevision = candidate.Revision
		}
		return err
	case "stop_candidate":
		candidate, err := c.Store.Candidate(ctx, c.Tasks.Project, payload.RunID)
		if err != nil {
			return err
		}
		if candidate.TaskID != payload.TaskID {
			return failure("invalid_request", "candidate belongs to another task")
		}
		if candidate.Revision != payload.ExpectedRunRevision {
			return failure("stale_version", "candidate changed before withdrawal")
		}
		if !candidate.StopRequested {
			candidate, err = c.Tasks.Stop(ctx, payload.RunID, payload.ExpectedRunRevision)
		}
		if err == nil {
			receipt.RunID = candidate.ID
			receipt.RunRevision = candidate.Revision
		}
		return err
	case "set_dispatch_mode", "pause_dispatch", "resume_dispatch":
		update := ports.RunnerDispatch{Project: c.Tasks.Project, RequestID: "runner-dispatch-" + req.RequestID, ExpectedAuthorityVersion: req.ExpectedControlRevision, RequestedBy: req.RequestedBy}
		if req.Action == "set_dispatch_mode" {
			update.Mode = &payload.Mode
		} else {
			paused := req.Action == "pause_dispatch"
			update.Paused = &paused
		}
		authority, err := c.Store.SetRunnerDispatch(ctx, update)
		if err == nil {
			receipt.ControlRevision = authority.AuthorityVersion
		}
		return err
	default:
		return failure("unsupported_control", "this action is not provided by the public Runner CLI")
	}
}
