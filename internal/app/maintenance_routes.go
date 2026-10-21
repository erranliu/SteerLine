package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/testmanagement"
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
	e := p.actor.Call(ctx, func() error {
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
			}
		} else {
			old = storageports.CommandRecord{ProjectIdentity: p.config.Identity(), CommandID: in.RequestID, Kind: "project_maintenance", TargetID: in.TargetID, ExpectedVersion: in.ExpectedVersion, IdempotencyKey: in.RequestID, RequestedBy: "local-user", Payload: raw, Status: "pending", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			old, _, e = p.store.EnqueueCommand(ctx, old)
			if e != nil {
				return e
			}
		}
		result, e := p.maintenanceAction(ctx, in)
		out.Status = "applied"
		if e != nil {
			out.Status = "rejected"
			out.Code = "maintenance_rejected"
			out.Diagnostic = e.Error()
			var external *runnerclient.Failure
			durableUnknown := false
			if in.Action == "maintenance_generate" {
				g, readErr := p.config.runtimeMaintenance.store.Generation(context.WithoutCancel(ctx), p.config.Identity(), in.RequestID)
				durableUnknown = readErr == nil && g.State != "rejected"
			}
			if in.Action == "maintenance_stop_generation" {
				var target api.TestStopGenerationRequest
				if json.Unmarshal(in.Payload, &target) == nil {
					g, readErr := p.config.runtimeMaintenance.store.Generation(context.WithoutCancel(ctx), p.config.Identity(), target.GenerationID)
					durableUnknown = readErr == nil && g.Cancelled && g.StopID == in.RequestID
				}
			}
			if durableUnknown || errors.Is(e, testmanagement.ErrApplicationUnknown) || errors.As(e, &external) && external.Unknown {
				out.Status = "outcome_unknown"
				out.Code = "outcome_unknown"
			}
		} else {
			out.Maintenance, _ = json.Marshal(result)
			if g, ok := result.(api.TestGeneration); ok && in.Action == "maintenance_generate" && g.RunID == "" && g.State == "unknown" {
				out.Status = "outcome_unknown"
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
		return m.check(ctx)
	case "maintenance_review":
		var v materialReview
		if e := decodeStrictJSON(in.Payload, &v); e != nil {
			return nil, e
		}
		if in.TargetID != v.Plan {
			return nil, errors.New("review target mismatch")
		}
		v.RequestID = in.RequestID
		return m.approve(ctx, v, in.ExpectedVersion)
	case "maintenance_confirm_task":
		var v taskReview
		if e := decodeStrictJSON(in.Payload, &v); e != nil {
			return nil, e
		}
		if in.TargetID != v.TaskID || in.ExpectedVersion != v.Version {
			return nil, errors.New("task target mismatch")
		}
		v.RequestID = in.RequestID
		return m.confirm(ctx, v)
	case "maintenance_generate":
		var v api.TestGenerateRequest
		if e := decodeStrictJSON(in.Payload, &v); e != nil {
			return nil, e
		}
		if in.TargetID != v.Plan {
			return nil, errors.New("generation target mismatch")
		}
		v.RequestID = in.RequestID
		return p.config.runtimeTests.GenerateTests(ctx, v)
	case "maintenance_apply":
		var v api.TestApplyRequest
		if e := decodeStrictJSON(in.Payload, &v); e != nil {
			return nil, e
		}
		if in.TargetID != v.GenerationID || in.ExpectedVersion != v.ExpectedRevision {
			return nil, errors.New("application target/version mismatch")
		}
		v.RequestID = in.RequestID
		return p.config.runtimeTests.ApplyTests(ctx, v)
	case "maintenance_stop_generation":
		var v api.TestStopGenerationRequest
		if e := decodeStrictJSON(in.Payload, &v); e != nil {
			return nil, e
		}
		if in.TargetID != v.GenerationID {
			return nil, errors.New("stop target mismatch")
		}
		v.RequestID = in.RequestID
		return p.config.runtimeTests.StopTestGeneration(ctx, v)
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
		case "maintenance_generate":
			if g, err := p.config.runtimeMaintenance.store.Generation(ctx, p.config.Identity(), in.RequestID); err == nil {
				value = publicGeneration(g)
				if g.RunID != "" {
					out.Status = "applied"
				} else if g.State == "rejected" {
					out.Status = "rejected"
				}
			}
		case "maintenance_apply", "maintenance_stop_generation":
			var target struct {
				GenerationID string `json:"generation_id"`
			}
			if json.Unmarshal(in.Payload, &target) == nil {
				if g, err := p.config.runtimeMaintenance.store.Generation(ctx, p.config.Identity(), target.GenerationID); err == nil {
					value = publicGeneration(g)
					if in.Action == "maintenance_stop_generation" && g.Cancelled && g.StopID == in.RequestID {
						out.Status = "applied"
					}
					for _, a := range g.Applications {
						if in.Action == "maintenance_apply" && a.Command.RequestID == in.RequestID && a.State == "applied" {
							out.Status = "applied"
						}
					}
				}
			}
		case "maintenance_review", "maintenance_confirm_task":
			if raw, err := p.config.runtimeMaintenance.store.MaintenanceReceipt(ctx, p.config.Identity(), in.RequestID); err == nil {
				value = raw
				out.Status = "applied"
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
	if strings.HasPrefix(route, "/tests/generations/") {
		return p.config.runtimeTests.TestGeneration(ctx, strings.TrimPrefix(route, "/tests/generations/"))
	}
	m := p.config.runtimeMaintenance
	if m == nil {
		return nil, errors.New("maintenance unavailable")
	}
	if strings.HasPrefix(route, "/maintenance/tasks/") {
		return m.taskPreview(ctx, strings.TrimPrefix(route, "/maintenance/tasks/"))
	}
	return m.view(ctx, mode, plan)
}
