package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// Frozen Runner inputs and execution settings stay in storage, not browser DTOs.
func publicEnvironment(r env.Record) any {
	var generated map[string]any
	if r.Generation != nil {
		manifest, _ := env.ParseManifest([]byte(r.Generation.Document))
		generated = map[string]any{"document": r.Generation.Document, "script": r.Generation.Script, "gaps": r.Generation.Gaps, "script_path": manifest.Script}
	}
	rounds := []any{}
	for _, a := range r.Rounds {
		var run any
		if a.Run != nil {
			reply := ""
			verified := false
			if a.Run.Final != nil {
				reply = a.Run.Final.Text
				verified = a.Run.Final.Verified
			}
			run = map[string]any{"status": a.Run.Status, "stop_status": a.Run.StopStatus, "phase": a.Run.Phase, "step": a.Run.Step, "summary": environmentExcerpt(a.Run.Summary), "diagnostic": environmentExcerpt(a.Run.Diagnostic), "agent_reply": environmentExcerpt(reply), "agent_reply_verified": verified}
		}
		rounds = append(rounds, map[string]any{"number": a.Number, "deadline": a.Deadline, "run_id": a.RunID, "submission": a.Submission, "settled": a.Settled, "timed_out": a.TimedOut, "diagnostic": a.Diagnostic, "progress": a.Progress, "result": a.Result, "report": a.Report, "run": run})
	}
	return map[string]any{"id": r.ID, "revision": r.Revision, "kind": r.Kind, "profile_id": r.Profile, "state": r.State, "status": r.Status(), "settled": r.Settled(), "created_at": r.CreatedAt, "diagnostic": r.Diagnostic, "rounds": rounds, "generated": generated, "source_commit": r.Prepared.Commit, "script_digest": r.Prepared.ScriptDigest, "inputs": r.Prepared.Inputs, "round_timeout_seconds": r.TimeoutSeconds}
}
func environmentExcerpt(s string) string {
	r := []rune(s)
	if len(r) > 4096 {
		return string(r[:4096]) + "…（截断，完整内容保留于原Run）"
	}
	return s
}
func (p *runtimeProjectPort) environmentCommand(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	out := maintenanceResponse(in)
	if e := in.Validate(httpcontract.ProjectID(p.config.Identity()), p.instance); e != nil {
		return out, e
	}
	if p.actor == nil || p.config.runtimeEnvironment == nil || p.stopping.Load() {
		return out, projectStoppingError()
	}
	e := p.actor.Call(ctx, func(ctx context.Context) error {
		raw, _ := json.Marshal(in)
		old, found, e := p.store.LookupCommand(ctx, p.config.Identity(), in.RequestID)
		if e != nil {
			return e
		}
		if found {
			if old.Kind != "project_environment" || !bytes.Equal(old.Payload, raw) {
				return errors.New("request_conflict: original environment envelope differs")
			}
			receipt, e := p.environmentReceipt(ctx, in.RequestID)
			if receipt != nil {
				out = *receipt
			}
			return e
		}
		old = storageports.CommandRecord{ProjectIdentity: p.config.Identity(), CommandID: in.RequestID, Kind: "project_environment", TargetID: in.TargetID, ExpectedVersion: in.ExpectedVersion, IdempotencyKey: in.RequestID, RequestedBy: "local-user", Payload: raw, Status: "pending", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if old, _, e = p.store.EnqueueCommand(ctx, old); e != nil {
			return e
		}
		s := p.config.runtimeEnvironment.service
		var r env.Record
		switch in.Action {
		case "environment_start", "environment_generate":
			if in.TargetID != in.ProjectID || in.ExpectedVersion != 1 {
				e = errors.New("environment target/version differs")
				break
			}
			var shape map[string]json.RawMessage
			if in.Action == "environment_start" && (json.Unmarshal(in.Payload, &shape) != nil || shape["expected_current_revision"] == nil || string(shape["expected_current_revision"]) == "null") {
				e = errors.New("expected_current_revision is required")
				break
			}
			var start env.Start
			if e = decodeStrictJSON(in.Payload, &start); e == nil {
				r, e = s.Start(ctx, in.RequestID, raw, start, in.Action == "environment_generate")
			}
		case "environment_invalidate":
			if in.TargetID != in.ProjectID || in.ExpectedVersion != 1 {
				e = errors.New("environment target/version differs")
				break
			}
			var invalidate struct {
				Key      string  `json:"environment_key"`
				Revision *uint64 `json:"expected_current_revision"`
				Reason   string  `json:"reason"`
			}
			if e = decodeStrictJSON(in.Payload, &invalidate); e != nil {
				break
			}
			if invalidate.Revision == nil || !p.config.runtimeEnvironment.supportsEnvironment(invalidate.Key) {
				e = errors.New("invalid current environment target/version")
				break
			}
			out.Status = "applied"
			old.Status = "applied"
			old.UpdatedAt = time.Now().UTC()
			old.Result, _ = json.Marshal(out)
			e = p.config.runtimeEnvironment.store.InvalidateEnvironment(ctx, p.config.Identity(), invalidate.Key, invalidate.Reason, *invalidate.Revision, old)
			if e == nil {
				return nil
			}
			if p.reconcileInvalidationFailure(ctx, old, &out, e) {
				return nil
			}
		case "environment_stop":
			var empty struct{}
			if e = decodeStrictJSON(in.Payload, &empty); e == nil {
				r, e = s.Stop(ctx, in.TargetID, in.RequestID, in.ExpectedVersion, raw)
			}
		default:
			e = errors.New("unknown environment command")
		}
		out.Status = "applied"
		if e != nil {
			out.Status = "rejected"
			out.Code = "environment_rejected"
			out.Diagnostic = e.Error()
		} else {
			out.Environment, _ = json.Marshal(publicEnvironment(r))
		}
		if e != nil {
			rows, readErr := s.Store.Environments(context.WithoutCancel(ctx), s.Project)
			if readErr != nil {
				out.Status = "outcome_unknown"
			} else {
				for _, row := range rows {
					for _, request := range row.Requests {
						if request.ID == in.RequestID && bytes.Equal(request.Original, raw) {
							out.Status = "applied"
							out.Code = ""
							out.Environment, _ = json.Marshal(publicEnvironment(row))
						}
					}
				}
			}
		}
		old.Result, _ = json.Marshal(out)
		old.Status = out.Status
		old.UpdatedAt = time.Now().UTC()
		return p.store.UpdateCommand(ctx, old)
	})
	return out, e
}
func (p *runtimeProjectPort) environmentReceipt(ctx context.Context, id string) (*httpcontract.CommandResponse, error) {
	old, found, e := p.store.LookupCommand(ctx, p.config.Identity(), id)
	if e != nil || !found || old.Kind != "project_environment" {
		return nil, e
	}
	var in httpcontract.CommandEnvelope
	if e = json.Unmarshal(old.Payload, &in); e != nil {
		return nil, e
	}
	out := maintenanceResponse(in)
	if len(old.Result) > 0 {
		if e = json.Unmarshal(old.Result, &out); e != nil {
			return nil, e
		}
	}
	if out.Status == "outcome_unknown" {
		rows, e := p.config.runtimeEnvironment.store.Environments(ctx, p.config.Identity())
		if e != nil {
			return nil, e
		}
		for _, r := range rows {
			for _, q := range r.Requests {
				if q.ID == id {
					if !bytes.Equal(q.Original, old.Payload) {
						return nil, errors.New("original environment receipt input differs")
					}
					out.Status = "applied"
					out.Code = ""
					out.Environment, _ = json.Marshal(publicEnvironment(r))
					return &out, nil
				}
			}
		}
		out.Diagnostic = "原环境操作结果待核实；查询不会重新提交"
	}
	return &out, nil
}
func (p *runtimeProjectPort) environmentRead(ctx context.Context, route string) (any, error) {
	port := p.config.runtimeEnvironment
	if port == nil {
		return nil, errors.New("project environment unavailable")
	}
	if route == "/environment" {
		rows, e := port.store.Environments(ctx, p.config.Identity())
		if e != nil {
			return nil, e
		}
		out := []any{}
		for _, r := range rows {
			if len(out) == 20 {
				break
			}
			out = append(out, map[string]any{"id": r.ID, "revision": r.Revision, "kind": r.Kind, "profile_id": r.Profile, "state": r.State, "status": r.Status(), "settled": r.Settled(), "diagnostic": r.Diagnostic, "rounds": []any{}})
		}
		return map[string]any{"records": out}, nil
	}
	if route == "/environment/options" {
		return port.readinessOptions(ctx)
	}

	id := strings.TrimPrefix(route, "/environment/")
	r, e := port.store.Environment(ctx, p.config.Identity(), id)
	return publicEnvironment(r), e
}

// Reconcile only the original invalidation receipt after an uncertain storage
// outcome. Never overwrite a possibly committed applied receipt with rejected.
func (p *runtimeProjectPort) reconcileInvalidationFailure(ctx context.Context, original storageports.CommandRecord, out *httpcontract.CommandResponse, cause error) bool {
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	saved, found, readErr := p.store.LookupCommand(readCtx, original.ProjectIdentity, original.IdempotencyKey)
	if readErr == nil && found && saved.Status == "applied" && bytes.Equal(saved.Payload, original.Payload) && len(saved.Result) > 0 {
		if json.Unmarshal(saved.Result, out) == nil {
			return true
		}
		readErr = errors.New("original invalidation receipt is malformed")
	}
	var storageErr *storageports.StorageError
	if readErr != nil || errors.As(cause, &storageErr) || errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		out.Status = "outcome_unknown"
		out.Code = "environment_outcome_unknown"
		out.Diagnostic = "原环境失效操作结果待核实；只查询原请求，不重新提交"
		return true
	}
	return false
}
