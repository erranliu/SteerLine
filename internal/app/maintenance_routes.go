package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func maintenanceResponse(in httpcontract.CommandEnvelope) httpcontract.CommandResponse {
	return httpcontract.CommandResponse{RequestID: in.RequestID, ProjectID: in.ProjectID, InstanceID: in.InstanceID, Action: in.Action, TargetID: in.TargetID, ExpectedVersion: in.ExpectedVersion, Original: &in, Status: "outcome_unknown", SampledAt: time.Now().UTC()}
}
func (p *runtimeProjectPort) maintenanceCommand(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	out := maintenanceResponse(in)
	if e := in.Validate(httpcontract.ProjectID(p.config.Identity()), p.instance); e != nil {
		return out, e
	}
	if p.actor == nil || p.config.runtimeMaintenance == nil || p.stopping.Load() {
		return out, projectStoppingError()
	}
	e := p.actor.Call(ctx, func(ctx context.Context) error {
		raw, _ := json.Marshal(in)
		old, found, e := p.store.LookupCommand(ctx, p.config.Identity(), in.RequestID)
		if e != nil {
			return e
		}
		if found {
			if old.Kind != "project_maintenance" || !bytes.Equal(old.Payload, raw) {
				return errors.New("request_conflict: original maintenance envelope differs")
			}
			if len(old.Result) > 0 {
				if e = json.Unmarshal(old.Result, &out); e != nil {
					return e
				}
				if out.Status != "outcome_unknown" {
					return nil
				}
				result, e := p.maintenanceReceipt(ctx, in.RequestID)
				if e != nil {
					return e
				}
				if result != nil {
					out = *result
				}
				return nil
			}
		} else {
			old = storageports.CommandRecord{ProjectIdentity: p.config.Identity(), CommandID: in.RequestID, Kind: "project_maintenance", TargetID: in.TargetID, ExpectedVersion: in.ExpectedVersion, IdempotencyKey: in.RequestID, RequestedBy: "local-user", Payload: raw, Status: "pending", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			old, _, e = p.store.EnqueueCommand(ctx, old)
			if e != nil {
				return e
			}
		}
		switch in.Action {
		case "maintenance_check":
			p.config.runtimeNative.RefreshFailures()
		}
		result, e := p.maintenanceAction(ctx, in)
		out.Status = "applied"
		if e != nil {
			out.Status = "rejected"
			out.Code = "maintenance_rejected"
			out.Diagnostic = e.Error()
			var external *runnerclient.Failure
			if errors.As(e, &external) && external.Unknown {
				out.Status = "outcome_unknown"
				out.Code = "outcome_unknown"
			}

		} else {
			out.Maintenance, _ = json.Marshal(result)
			if update, ok := result.(updateFact); ok && update.Status != "applied" {
				out.Status = update.Status
				out.Code = update.Status
				out.Diagnostic = update.Diagnostic
			}
		}
		// An accepted external operation keeps its own original durable ID even if this write fails.
		old.Result, _ = json.Marshal(out)
		old.Status = out.Status
		old.UpdatedAt = time.Now().UTC()
		return p.store.UpdateCommand(ctx, old)
	})
	return out, e
}
func (p *runtimeProjectPort) maintenanceAction(ctx context.Context, in httpcontract.CommandEnvelope) (any, error) {
	m := p.config.runtimeMaintenance
	switch in.Action {
	case "maintenance_check":
		if in.TargetID != in.ProjectID || in.ExpectedVersion != 1 {
			return nil, errors.New("observation target/version mismatch")
		}
		var empty struct{}
		if e := decodeStrictJSON(in.Payload, &empty); e != nil {
			return nil, e
		}
		if _, e := m.check(ctx); e != nil {
			return nil, e
		}
		return nil, nil
	case "maintenance_update":
		if e := decodeStrictJSON(in.Payload, &struct{}{}); e != nil {
			return nil, e
		}
		if in.TargetID != in.ProjectID || in.ExpectedVersion != 1 {
			return nil, errors.New("update target mismatch")
		}
		return m.updateLocal(ctx, in.RequestID)

	}
	return nil, errors.New("unsupported maintenance action")
}
func (p *runtimeProjectPort) maintenanceReceipt(ctx context.Context, id string) (*httpcontract.CommandResponse, error) {
	old, found, e := p.store.LookupCommand(ctx, p.config.Identity(), id)
	if e != nil || !found || old.Kind != "project_maintenance" {
		return nil, e
	}
	var in httpcontract.CommandEnvelope
	if e = json.Unmarshal(old.Payload, &in); e != nil {
		return nil, e
	}
	out := maintenanceResponse(in)
	if len(old.Result) > 0 {
		e = json.Unmarshal(old.Result, &out)
	} else {
		out.Diagnostic = "原维护操作结果待核实；查询不会重新生成或应用"
	}
	if e == nil && out.Status == "outcome_unknown" {
		var value any
		switch in.Action {
		case "maintenance_update":
			fact, e := p.config.runtimeMaintenance.observeUpdate(ctx, in.RequestID)
			if e == nil {
				value = fact
				out.Status = fact.Status
				out.Diagnostic = fact.Diagnostic
			}

		}
		if value != nil {
			out.Maintenance, _ = json.Marshal(value)
		}
		if out.Status == "applied" {
			out.Code = ""
			out.Diagnostic = ""
		}
	}
	return &out, e
}
func (p *runtimeProjectPort) maintenanceRead(ctx context.Context, route, mode, plan string) (any, error) {
	if route == "/maintenance/readiness" {
		if p.readiness == nil {
			return nil, errors.New("maintenance unavailable")
		}
		return p.readiness.projectReadiness(ctx)
	}
	m := p.config.runtimeMaintenance
	if m == nil {
		return nil, errors.New("maintenance unavailable")
	}
	if strings.HasPrefix(route, "/maintenance/tasks/") {
		return m.taskDocuments(ctx, strings.TrimPrefix(route, "/maintenance/tasks/"))
	}
	if route == "/maintenance/log" {
		if mode == "" {
			mode = "0"
		}
		return m.commitLog(ctx, mode, plan)
	}
	if strings.HasPrefix(route, "/maintenance/commits/") {
		return m.commitInfo(ctx, strings.TrimPrefix(route, "/maintenance/commits/"), true), nil
	}
	return m.view(ctx, mode, plan)
}
