package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (p *runtimeProjectPort) SubmitContinuationSuggestion(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	out := maintenanceResponse(in)
	if in.Action != "main_agent_cancel" && in.Action != "main_agent_regenerate" {
		return out, errors.New("unsupported advisory action")
	}
	if err := in.Validate(httpcontract.ProjectID(p.config.Identity()), p.instance); err != nil {
		return out, err
	}
	if p.config.runtimeContinuationSuggestion == nil || p.actor == nil || !p.Server.Identity(ctx).Ready {
		return out, projectStoppingError()
	}
	var payload struct{}
	if err := decodeStrictJSON(in.Payload, &payload); err != nil {
		return out, err
	}
	old, found, err := p.store.LookupCommand(ctx, p.config.Identity(), in.RequestID)
	if err != nil {
		return out, err
	}
	raw, _ := json.Marshal(in)
	if found {
		if old.Kind != "main_agent_command" || string(old.Payload) != string(raw) {
			return out, errors.New("request_conflict: advisory command differs")
		}
		r, e := p.ContinuationSuggestionReceipt(ctx, in.RequestID)
		if r != nil {
			return *r, e
		}
		return out, e
	}
	c := ports.CommandRecord{ProjectIdentity: p.config.Identity(), CommandID: in.RequestID, IdempotencyKey: in.RequestID, Kind: "main_agent_command", TargetID: in.TargetID, ExpectedVersion: in.ExpectedVersion, RequestedBy: "local-user", Payload: raw, Status: "pending", CreatedAt: time.Now().UTC()}
	c, created, err := p.store.EnqueueCommand(ctx, c)
	if err != nil {
		return out, err
	}
	if !created {
		return out, errors.New("original advisory command is pending")
	}
	advisory := p.config.runtimeContinuationSuggestion
	rejectedCode := "advisory_cancel_rejected"
	if in.Action == "main_agent_regenerate" {
		rejectedCode = "advisory_regenerate_rejected"
		err = advisory.regenerate(ctx, in.TargetID, in.ExpectedVersion, in.RequestID)
	} else {
		_, err = advisory.service.Cancel(ctx, in.TargetID, in.ExpectedVersion)
	}
	out.Status = "applied"
	if err != nil {
		out.Status = "rejected"
		out.Code = rejectedCode
		out.Diagnostic = err.Error()
	}
	c.Status = out.Status
	c.Result, _ = json.Marshal(out)
	if e := p.store.UpdateCommand(context.WithoutCancel(ctx), c); e != nil {
		return out, e
	}
	return out, nil
}
func (p *runtimeProjectPort) ContinuationSuggestionReceipt(ctx context.Context, id string) (*httpcontract.CommandResponse, error) {
	old, found, err := p.store.LookupCommand(ctx, p.config.Identity(), id)
	if err != nil || !found || old.Kind != "main_agent_command" {
		return nil, err
	}
	var in httpcontract.CommandEnvelope
	if err = json.Unmarshal(old.Payload, &in); err != nil {
		return nil, err
	}
	out := maintenanceResponse(in)
	if len(old.Result) > 0 {
		if err = json.Unmarshal(old.Result, &out); err != nil {
			return nil, err
		}
	} else {
		if in.Action == "main_agent_regenerate" && p.config.runtimeContinuationSuggestion != nil {
			applied, err := p.config.runtimeContinuationSuggestion.service.RegenerationApplied(ctx, in.TargetID, id)
			if err != nil {
				return nil, err
			}
			if applied {
				out.Status = "applied"
				return &out, nil
			}
		}
		out.Status = "outcome_unknown"
		out.Code = "outcome_unknown"
		out.Diagnostic = "原建议操作结果待核实，不自动重发"
	}
	return &out, nil
}

func (p *continuationSuggestionPort) regenerate(ctx context.Context, id string, version uint64, commandID string) error {
	old, err := p.service.Get(ctx, id)
	if err != nil {
		return err
	}
	candidate, err := p.store.Candidate(ctx, p.config.Identity(), old.Source.CandidateID)
	if err != nil {
		return err
	}
	source, ok := materialAdviceSource(candidate)
	if !ok || source.Occurrence != old.Source.Occurrence {
		return errors.New("原问题已不再等待回答")
	}
	return p.service.Regenerate(ctx, id, version, source, commandID)
}
