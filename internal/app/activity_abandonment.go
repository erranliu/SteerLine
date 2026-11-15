package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

const activityAbandon = "activity_abandon"

func (p *runtimeProjectPort) abandonActivity(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	out := maintenanceResponse(in)
	project := p.config.Identity()
	if err := in.Validate(httpcontract.ProjectID(project), p.instance); err != nil {
		return out, err
	}
	var payload *struct{}
	if err := decodeStrictJSON(in.Payload, &payload); err != nil || payload == nil {
		return out, &httpclient.ClientError{Status: 400, Code: "invalid_payload", Reason: "放弃活动的载荷必须是空对象"}
	}
	source, id, err := query.DecodeActivityID(project, in.TargetID)
	if err != nil {
		return out, &httpclient.ClientError{Status: 400, Code: "invalid_activity", Reason: err.Error()}
	}
	store, ok := p.store.(ports.ActivityAbandonmentStore)
	reader, readable := p.store.(ports.AsyncActivityReader)
	if !ok || !readable || p.actor == nil || p.Server == nil || !p.Server.Identity(ctx).Ready {
		return out, projectStoppingError()
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	response := make(chan httpcontract.CommandResponse, 1)
	err = p.actor.Call(ctx, func(ctx context.Context) (retErr error) {
		out := maintenanceResponse(in)
		defer func() {
			if retErr == nil {
				response <- out
			}
		}()
		old, found, err := p.store.LookupCommand(ctx, project, in.RequestID)
		if err != nil {
			return err
		}
		if found {
			if old.Kind != activityAbandon || !bytes.Equal(old.Payload, raw) {
				return errors.New("request_conflict: activity abandonment differs")
			}
			return json.Unmarshal(old.Result, &out)
		}
		activity, err := query.ReadOneActivity(ctx, reader, project, p.instance, in.TargetID, in.ProjectID)
		if errors.Is(err, ports.ErrNotFound) {
			out.Status, out.Code, out.Diagnostic = "rejected", "activity_not_found", "找不到这条正式活动"
		} else if err != nil {
			return err
		}
		var stop func(context.Context) error
		if out.Status != "rejected" {
			out.Status = "applied"
			out.AbandonedAt = activity.AbandonedAt
			if activity.AbandonedAt == nil {
				out.AbandonedAt = &out.SampledAt
				stop, err = p.activityStop(source, id, in.RequestID, raw)
				if err != nil {
					return err
				}
			}
		}
		result, err := json.Marshal(out)
		if err != nil {
			return err
		}
		receipt := ports.CommandRecord{ProjectIdentity: project, CommandID: in.RequestID, Kind: activityAbandon, TargetID: in.TargetID, ExpectedVersion: in.ExpectedVersion, IdempotencyKey: in.RequestID, RequestedBy: "local-user", Payload: raw, Status: out.Status, Result: result, CreatedAt: out.SampledAt}
		if out.Status == "rejected" {
			_, _, err = p.store.EnqueueCommand(ctx, receipt)
			return err
		}
		return store.CommitActivityAbandonment(ctx, source, id, receipt, stop)
	})
	if err != nil {
		return out, err
	}
	return <-response, nil
}

func (p *runtimeProjectPort) activityStop(source, id, request string, raw json.RawMessage) (func(context.Context) error, error) {
	switch source {
	case "environment_detection":
		if port := p.config.runtimeEnvironment; port != nil {
			return func(ctx context.Context) error { return port.service.StopForAbandonment(ctx, id, request, raw) }, nil
		}
	case "roadmap":
		if port := p.config.runtimeRoadmaps; port != nil {
			return func(ctx context.Context) error {
				return port.service.StopForAbandonment(ctx, id, request)
			}, nil
		}
	case "main_agent_advisory":
		if port := p.config.runtimeContinuationSuggestion; port != nil {
			return func(ctx context.Context) error { return port.service.StopForAbandonment(ctx, id) }, nil
		}
	case "refactoring":
		if port := p.config.runtimeRefactoring; port != nil {
			return func(ctx context.Context) error { return port.service.StopForAbandonment(ctx, id) }, nil
		}
	case "baseline_check", "task_candidate":
		return nil, nil
	}
	return nil, errors.New("活动业务服务不可用")
}

func (p *runtimeProjectPort) activityAbandonmentReceipt(ctx context.Context, request string) (*httpcontract.CommandResponse, error) {
	old, found, err := p.store.LookupCommand(ctx, p.config.Identity(), request)
	if err != nil || !found || old.Kind != activityAbandon {
		return nil, err
	}
	var out httpcontract.CommandResponse
	err = json.Unmarshal(old.Result, &out)
	return &out, err
}
