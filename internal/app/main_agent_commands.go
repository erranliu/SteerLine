package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/mainagent"
	ma "github.com/erranliu/SteerLine/internal/mainagent/model"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (p *runtimeProjectPort) SubmitMainAgent(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	out := maintenanceResponse(in)
	if err := in.Validate(httpcontract.ProjectID(p.config.Identity()), p.instance); err != nil {
		return out, err
	}
	port := p.config.runtimeMainAgent
	if port == nil || p.actor == nil || !p.Server.Identity(ctx).Ready {
		return out, projectStoppingError()
	}
	raw, _ := json.Marshal(in)
	old, found, err := p.store.LookupCommand(ctx, p.config.Identity(), in.RequestID)
	if err != nil {
		return out, err
	}
	if found {
		if old.Kind != "main_agent_command" || !bytes.Equal(old.Payload, raw) {
			return out, errors.New("request_conflict: original main Agent command differs")
		}
		receipt, e := p.MainAgentReceipt(ctx, in.RequestID)
		if e != nil {
			return out, e
		}
		if receipt != nil {
			return *receipt, nil
		}
		return out, nil
	}
	command := ports.CommandRecord{ProjectIdentity: p.config.Identity(), CommandID: in.RequestID, IdempotencyKey: in.RequestID, Kind: "main_agent_command", TargetID: in.TargetID, ExpectedVersion: in.ExpectedVersion, RequestedBy: "local-user", Payload: raw, Status: "pending", CreatedAt: time.Now().UTC()}
	command, created, err := p.store.EnqueueCommand(ctx, command)
	if err != nil || !created {
		return out, err
	}
	var result any
	var actionErr error
	if in.Action == "main_agent_answer" {
		result, actionErr = p.applyMainAgentAnswer(ctx, in)
	} else {
		result, actionErr = port.action(ctx, in)
	}
	out.Status = "applied"
	if actionErr != nil {
		out.Status = "rejected"
		out.Code = "main_agent_rejected"
		out.Diagnostic = actionErr.Error()
		if _, e := port.store.Analysis(context.WithoutCancel(ctx), p.config.Identity(), in.RequestID); e == nil {
			out.Status = "outcome_unknown"
			out.Code = "outcome_unknown"
		}
	}
	if actionErr != nil && in.Action == "main_agent_publish" {
		if r, e := port.store.Analysis(context.WithoutCancel(ctx), p.config.Identity(), in.TargetID); e == nil && r.Publication != nil && r.Publication.RequestID == in.RequestID {
			out.Status = "outcome_unknown"
			out.Code = "outcome_unknown"
		}
	}
	if actionErr != nil && in.Action == "main_agent_answer" {
		if r, e := port.store.Analysis(context.WithoutCancel(ctx), p.config.Identity(), in.TargetID); e == nil && r.AnswerAction != nil && r.AnswerAction.CallerID == in.RequestID {
			out.Status = "outcome_unknown"
			out.Code = "outcome_unknown"
		}
	}
	if result != nil {
		out.MainAgent, _ = json.Marshal(result)
	}
	command.Status = out.Status
	command.Result, _ = json.Marshal(out)
	if err = p.store.UpdateCommand(context.WithoutCancel(ctx), command); err != nil {
		return out, err
	}
	return out, nil
}
func (p *mainAgentPort) action(ctx context.Context, in httpcontract.CommandEnvelope) (any, error) {
	switch in.Action {
	case "main_agent_prepare":
		var input struct {
			Commit string `json:"commit"`
		}
		if err := decodeStrictJSON(in.Payload, &input); err != nil {
			return nil, err
		}
		if in.TargetID != httpcontract.ProjectID(p.config.Identity()) || in.ExpectedVersion != 1 || input.Commit == "" {
			return nil, errors.New("exact project/commit required")
		}
		head, err := p.config.runtimeMaintenance.git(ctx, "rev-parse", "HEAD^{commit}")
		if err != nil || strings.TrimSpace(string(head)) != input.Commit {
			return nil, errors.New("stale_revision: project HEAD changed")
		}
		if err = mainagent.PrepareRules(p.config.Repository, false); err != nil {
			return nil, err
		}
		return p.options(ctx), nil
	case "main_agent_analyze":
		var input struct {
			Kind     string `json:"kind"`
			Note     string `json:"note"`
			ParentID string `json:"parent_id,omitempty"`
		}
		if err := decodeStrictJSON(in.Payload, &input); err != nil {
			return nil, err
		}
		return p.service.Start(ctx, in.RequestID, ma.Start{CandidateID: in.TargetID, CandidateRevision: in.ExpectedVersion, Kind: input.Kind, Note: input.Note, ParentID: input.ParentID})
	case "main_agent_stop":
		if err := decodeStrictJSON(in.Payload, &struct{}{}); err != nil {
			return nil, err
		}
		r, err := p.store.Analysis(ctx, p.config.Identity(), in.TargetID)
		if err != nil {
			return nil, err
		}
		if r.Revision != in.ExpectedVersion {
			return nil, errors.New("stale_revision: analysis changed")
		}
		return p.service.Stop(ctx, r.ID)
	case "main_agent_publish":
		var input struct {
			Review string `json:"review"`
		}
		if err := decodeStrictJSON(in.Payload, &input); err != nil {
			return nil, err
		}
		return p.publish(ctx, in.TargetID, in.RequestID, in.ExpectedVersion, input.Review)
	default:
		return nil, errors.New("unsupported main Agent action")
	}
}
func (p *runtimeProjectPort) MainAgentReceipt(ctx context.Context, id string) (*httpcontract.CommandResponse, error) {
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
	}
	if old.Status == "pending" || out.Status == "outcome_unknown" {
		out.Status = "outcome_unknown"
		out.Code = "outcome_unknown"
		out.Diagnostic = "原操作结果待核实，不自动重发"
		if in.Action == "main_agent_analyze" {
			r, e := p.config.runtimeMainAgent.store.Analysis(ctx, p.config.Identity(), id)
			if e == nil {
				out.MainAgent, _ = json.Marshal(r)
				out.Status = "applied"
				out.Code = ""
				out.Diagnostic = ""
			} else if !errors.Is(e, ma.ErrNotFound) {
				return nil, e
			}
		}
		if in.Action == "main_agent_answer" {
			r, e := p.config.runtimeMainAgent.store.Analysis(ctx, p.config.Identity(), in.TargetID)
			if e != nil {
				return nil, e
			}
			if r.AnswerAction != nil && r.AnswerAction.CallerID == id {
				value, e := p.Server.Read(ctx, "/commands/"+r.AnswerAction.RequestID, nil)
				if e == nil {
					raw, _ := json.Marshal(value)
					var child httpcontract.CommandResponse
					if json.Unmarshal(raw, &child) == nil && (child.Status == "applied" || child.Status == "accepted" || child.Status == "rejected") {
						out.Status = child.Status
						if child.Status == "accepted" {
							out.Status = "applied"
						}
						out.Code = child.Code
						out.Diagnostic = child.Diagnostic
					}
				}
				out.MainAgent, _ = json.Marshal(r)
			}
		}
		if in.Action == "main_agent_publish" {
			r, e := p.config.runtimeMainAgent.store.Analysis(ctx, p.config.Identity(), in.TargetID)
			if e != nil {
				return nil, e
			}
			if r.Publication != nil && r.Publication.RequestID == id {
				r, e = p.config.runtimeMainAgent.observePublication(ctx, r)
				if e == nil && r.Publication.URL != "" {
					out.Status = "applied"
					out.Code = ""
					out.Diagnostic = ""
				}
				out.MainAgent, _ = json.Marshal(r)
			}
		}
		if in.Action == "main_agent_prepare" {
			if e := mainagent.PrepareRules(p.config.Repository, true); e == nil {
				out.Status = "applied"
				out.Code = ""
				out.Diagnostic = "规则文件已核实，请自行审阅提交并合入"
				out.MainAgent, _ = json.Marshal(p.config.runtimeMainAgent.options(ctx))
			}
		}
	}
	return &out, nil
}

func (p *runtimeProjectPort) applyMainAgentAnswer(ctx context.Context, in httpcontract.CommandEnvelope) (any, error) {
	port := p.config.runtimeMainAgent
	port.publicationMu.Lock()
	defer port.publicationMu.Unlock()
	var input struct {
		Review string `json:"review"`
	}
	if err := decodeStrictJSON(in.Payload, &input); err != nil {
		return nil, err
	}
	r, err := port.store.Analysis(ctx, p.config.Identity(), in.TargetID)
	if err != nil {
		return nil, err
	}
	if r.AnswerAction != nil {
		return r, errors.New("原建议已有回答请求，请核实原投递，不重复发送")
	}
	if r.Revision != in.ExpectedVersion || input.Review != ma.Review(r) || r.State != "review" || r.Result == nil || r.Result.Answer == nil || r.Context.Question == nil {
		return r, errors.New("stale_revision: 原建议或问题已变化")
	}
	baseline, err := port.config.runtimeMaintenance.check(ctx)
	if err != nil {
		return r, err
	}
	if baseline.Commit != r.Context.RepositoryCommit {
		return r, errors.New("stale_revision: 当前材料基线已变化，请重新分析")
	}
	// The original input owner rechecks participation, question set, task status,
	// revisions, pending/unknown input and final UTF-8 answer size below.
	childID := ma.Hash("main-agent-answer:" + in.RequestID)[:32]
	r.AnswerAction = &ma.AnswerAction{CallerID: in.RequestID, RequestID: childID, Review: input.Review}
	r, err = port.service.Save(ctx, r)
	if err != nil {
		return r, err
	}
	answer := r.Result.Answer
	payload := map[string]any{"run_id": r.Context.CandidateID, "decision_id": r.Context.Question.ID, "expected_run_revision": r.Context.CandidateRevision, "source_attempt_id": r.Context.SourceAttemptID, "expected_recovery_version": r.Context.RunnerRevision}
	if len(answer.Answers) > 0 {
		payload["answers"] = answer.Answers
	} else {
		payload["note"], payload["option_id"] = answer.Note, answer.OptionID
	}
	body, _ := json.Marshal(payload)
	child := httpcontract.CommandEnvelope{RequestID: childID, ProjectID: in.ProjectID, InstanceID: in.InstanceID, Action: "answer_decision", TargetID: r.Context.Question.ID, ExpectedVersion: r.Context.RunnerRevision, Payload: body}
	receipt, err := p.Server.Command(ctx, child)
	if err != nil {
		return r, err
	}
	if receipt.Status == "rejected" || receipt.Status == "outcome_unknown" {
		return r, errors.New("原回答请求待核实或被拒绝：" + receipt.Diagnostic)
	}
	return r, nil
}
