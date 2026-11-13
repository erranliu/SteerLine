package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
)

func (p *runtimeProjectPort) refactoringCommand(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	out := maintenanceResponse(in)
	if e := in.Validate(httpcontract.ProjectID(p.config.Identity()), p.instance); e != nil {
		return out, e
	}
	port := p.config.runtimeRefactoring
	if port == nil || p.stopping.Load() {
		return out, projectStoppingError()
	}
	command := ref.Command{ID: in.RequestID, Action: in.Action, Target: in.TargetID, Expected: in.ExpectedVersion}
	command.Raw, _ = json.Marshal(in)
	var start *ref.Start
	switch in.Action {
	case "refactoring_start":
		start = &ref.Start{}
		if e := decodeStrictJSON(in.Payload, start); e != nil {
			return out, e
		}
	case "refactoring_answer":
		var payload struct {
			QuestionID string `json:"question_id"`
			OptionID   string `json:"option_id"`
			Note       string `json:"note,omitempty"`
		}
		if e := decodeStrictJSON(in.Payload, &payload); e != nil {
			return out, e
		}
		command.QuestionID = payload.QuestionID
		command.OptionID = payload.OptionID
		command.Note = payload.Note
	case "refactoring_finish", "refactoring_stop", "refactoring_conflict":
		if e := decodeStrictJSON(in.Payload, &struct{}{}); e != nil {
			return out, e
		}
	default:
		return out, errors.New("不支持的维护操作")
	}
	r, e := port.service.Command(ctx, command, start)
	out.Status = "applied"
	if e != nil {
		out.Status = "rejected"
		out.Code = "refactoring_rejected"
		out.Diagnostic = e.Error()
		return out, nil
	}
	out.Refactoring, _ = json.Marshal(map[string]any{"id": r.ID, "revision": r.Revision})
	return out, nil
}
func (p *runtimeProjectPort) refactoringReceipt(ctx context.Context, id string) (*httpcontract.CommandResponse, error) {
	raw, result, e := p.config.runtimeRefactoring.store.RefactoringCommand(ctx, p.config.Identity(), id)
	if errors.Is(e, ref.ErrNotFound) || errors.Is(e, ref.ErrConflict) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var in httpcontract.CommandEnvelope
	if e = json.Unmarshal(raw, &in); e != nil {
		return nil, e
	}
	out := maintenanceResponse(in)
	out.Status = "applied"
	out.Refactoring = result
	return &out, nil
}
