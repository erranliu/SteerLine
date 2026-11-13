package refactoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	decision "github.com/erranliu/SteerLine/internal/steeragent/maintenance"
)

func (s *Service) block(r *model.Record, message string) {
	r.Phase = "blocked"
	r.Diagnostic = message
	r.Pause(s.now())
}

func closingPhase(r model.Record) string {
	if pendingReport(r) != nil {
		return "sync"
	}
	if r.Candidate != nil {
		return "verifying"
	}
	return "finish"
}

func uncertainPublication(r model.Record) bool {
	if r.Candidate != nil {
		return false
	}
	for i := len(r.Operations) - 1; i >= 0; i-- {
		o := r.Operations[i]
		switch o.Purpose {
		case "sync":
			return false
		case "publish", "conflict":
			if o.State == "complete" {
				if o.InitializationFailure != "" {
					continue
				}
				return !successfulExecution(o) || o.Result.Outcome != "needs_input" || o.Result.PR != ""
			}
		}
	}
	return false
}

func (s *Service) closeAction(r *model.Record) {
	r.FinishRequested = true
	r.Phase = closingPhase(*r)
	if r.LatestOwner == "" && r.Candidate == nil {
		base := r.Frozen.Repository.BaseRevision
		summary := r.Summary
		if summary == "" {
			summary = "行动在发起可写执行前结束"
		}
		r.Candidate = &model.Result{NoChanges: true, Outcome: "done", Head: base, Base: base, Summary: summary}
		r.Phase = "verifying"
	}
}

func (s *Service) applyDecision(r *model.Record, mode string, d decision.Decision) error {
	if mode == "report" {
		s.applyReport(r, d)
		return nil
	}
	r.Summary = d.Summary
	if mode == "first" || mode == "select" {
		_, costComplete := r.CostTotal()
		if r.FinishRequested || r.BudgetExhausted(s.now()) || r.Input.Budget.Dollars != "" && !costComplete {
			s.closeAction(r)
			return nil
		}
	}
	switch d.Kind {
	case "ask":
		var body model.QuestionBody
		decoder := json.NewDecoder(bytes.NewReader(d.Question))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || !validQuestion(&body) {
			s.block(r, "人工决定缺少有效问题、选项或推荐")
			return nil
		}
		last := &r.Operations[len(r.Operations)-1]
		r.Question = &model.Question{ID: model.ID(), Mode: mode, OperationID: last.ID, QuestionBody: body}
		last.Question = r.Question
		r.Phase = "waiting_input"
		r.Pause(s.now())
	case "blocked":
		s.block(r, d.Summary)
	case "finish":
		if mode == "version_audit" {
			r.Candidate = nil
			r.WriteKind = "conflict"
		}
		s.closeAction(r)
	case "accept":
		if mode == "version_audit" {
			if !versionEvidence(*r) {
				s.block(r, "新PR版本缺少对应证据，请补证后复核")
				return nil
			}
			r.EvidenceNeed = ""
			r.Phase = "verifying"
			return nil
		}
		for i := len(r.Operations) - 2; i >= 0; i-- {
			if o := r.Operations[i]; o.Content(*r).WorkflowID == "execute" {
				if !successfulExecution(o) || o.Result.Outcome != "done" {
					s.block(r, "原执行缺少可信成功证据，需要补证或修复")
					return nil
				}
				r.Operations[len(r.Operations)-1].AcceptedOperation = o.ID
				break
			}
		}
		if r.FinishRequested || r.BudgetExhausted(s.now()) {
			s.closeAction(r)
		} else {
			r.Phase = "select"
		}
	case "execute":
		switch mode {
		case "first", "select":
			r.Round++
			r.WriteKind = "implement"
		case "finish":
			if r.PR != nil || r.WriteKind == "conflict" {
				r.WriteKind = "conflict"
			} else {
				r.WriteKind = "publish"
			}
		case "evidence":
			r.WriteKind = "collect_evidence"
		case "version_audit":
			r.WriteKind = "version_check"
		case "audit":
			r.WriteKind = "repair"
		}
		r.PendingPrompt = d.Prompt
		r.Phase = "execute"
	}
	return nil
}

func successfulExecution(o model.Operation) bool {
	return o.Result != nil && o.Run != nil && o.Run.Status == "completed" && o.Run.Final != nil && o.Run.Final.Verified && o.Run.Final.NativeOutcome == "succeeded"
}

func versionEvidence(r model.Record) bool {
	if r.Candidate != nil && matchesVersion(r.PR, r.Candidate.PR, r.Candidate.Head, r.Candidate.Base) {
		return true
	}
	for i := len(r.Operations) - 1; i >= 0; i-- {
		o := r.Operations[i]
		if (o.Purpose == "version_check" || o.Purpose == "collect_evidence") &&
			successfulExecution(o) && o.Result.Outcome == "done" && r.PR != nil &&
			strings.TrimSpace(o.Result.Summary) != "" &&
			o.Result.Head == r.PR.HeadRevision && o.Result.Base == r.PR.BaseRevision {
			return true
		}
	}
	return false
}

func (s *Service) applyExecution(r *model.Record, o model.Operation) {
	trusted := successfulExecution(o)
	completed := trusted && o.Result.Outcome == "done" && strings.TrimSpace(o.Result.Summary) != ""
	if (o.Purpose == "publish" || o.Purpose == "conflict") && completed {
		r.Candidate = o.Result
	}
	if r.StopRequested {
		return
	}
	if o.InitializationFailure != "" {
		s.block(r, o.InitializationFailure)
		return
	}
	switch o.Purpose {
	case "publish", "conflict":
		if completed {
			r.Phase = "verifying"
		} else if trusted && o.Result.Outcome == "needs_input" && o.Result.PR == "" {
			r.Phase = "finish"
		} else {
			s.block(r, "原发布操作未取得可核验结果，请核实原操作及PR状态")
		}
	case "collect_evidence", "version_check":
		if completed {
			if r.PR != nil && (o.Result.Head != r.PR.HeadRevision || o.Result.Base != r.PR.BaseRevision) {
				s.block(r, "补证结果与当前核验版本不匹配")
				return
			}
			r.Phase = "verifying"
		} else if o.Result != nil && o.Result.Outcome == "needs_input" {
			r.Phase = "evidence"
			if o.Purpose == "version_check" {
				r.Phase = "version_audit"
			}
		} else {
			s.block(r, "未取得可信报告补证结果")
		}
	case "sync":
		r.Phase = "sync_verifying"
	default:
		r.Phase = "audit"
	}
}

func pendingReport(r model.Record) *model.Report {
	if r.PR == nil || len(r.Reports) == 0 {
		return nil
	}
	v := &r.Reports[len(r.Reports)-1]
	if v.SyncedAt == nil && matchesVersion(r.PR, v.PR, v.Head, v.Base) {
		return v
	}
	return nil
}

func matchesVersion(pr *runnercontract.PRFact, url, head, base string) bool {
	return pr != nil && pr.URL == url && pr.HeadRevision == head && pr.BaseRevision == base
}

func prVersion(pr *runnercontract.PRFact) string {
	if pr == nil {
		return ""
	}
	return pr.HeadRevision + ":" + pr.BaseRevision
}

func (s *Service) applyReport(r *model.Record, d decision.Decision) {
	if d.Kind == "needs_evidence" {
		need := strings.TrimSpace(d.Summary)
		if r.EvidenceNeed == need {
			s.block(r, "同一报告证据缺口仍未解决："+need)
			return
		}
		r.EvidenceNeed = need
		r.Phase = "evidence"
		return
	}
	if (r.PR == nil) != (strings.TrimSpace(d.Prompt) == "") || r.Candidate == nil {
		s.block(r, "维护报告缺少与本次交付匹配的同步指令或证据")
		return
	}
	o := r.Operations[len(r.Operations)-1]
	report := model.Report{Body: d.Report, SyncPrompt: d.Prompt, CreatedAt: s.now(), Cost: o.ReportCost, AsOf: o.ReportAsOf}
	if r.PR != nil {
		report.PR, report.Head, report.Base = r.PR.URL, r.PR.HeadRevision, r.PR.BaseRevision
	} else {
		report.Head, report.Base = r.Candidate.Head, r.Candidate.Base
	}
	r.Reports = append(r.Reports, report)
	if r.PR != nil {
		preview := *r
		preview.Phase, preview.WriteKind, preview.PendingPrompt = "execute", "sync", report.SyncPrompt
		if _, err := s.prepareOperation(preview, r.ID, ""); err != nil {
			r.Reports = r.Reports[:len(r.Reports)-1]
			s.block(r, "无法交付报告："+err.Error())
			return
		}
		r.Phase = "sync"
	} else {
		s.deliver(r)
	}
	r.EvidenceNeed = ""
}

func (s *Service) deliver(r *model.Record) {
	r.Phase = "delivered"
	r.Candidate = nil
	r.Diagnostic = ""
	now := s.now()
	r.FinishedAt = &now
	r.Pause(now)
}

func (s *Service) verifyPR(ctx context.Context, r model.Record, v model.Result) (*runnercontract.PRFact, string, error) {
	if s.Verify == nil {
		return nil, "", errors.New("缺少只读PR核验")
	}
	pr, body, err := s.Verify(ctx, r, v)
	if err != nil {
		return nil, "", err
	}
	if pr == nil || pr.URL != v.PR || !model.ValidRevision(pr.HeadRevision) || !model.ValidRevision(pr.BaseRevision) {
		return nil, "", errors.New("平台PR身份或head/base尚未核实")
	}
	return pr, body, nil
}

func (s *Service) verifyDelivery(ctx context.Context, r model.Record) error {
	if r.Phase == "sync_verifying" {
		return s.checkSync(ctx, r, true)
	}
	if r.Candidate == nil {
		_, err := s.update(ctx, r.ID, func(current *model.Record) error { s.block(current, "收尾缺少可核验结果"); return nil })
		return err
	}
	value := *r.Candidate
	if value.PR == "" {
		_, err := s.update(ctx, r.ID, func(current *model.Record) error {
			if current.Revision != r.Revision || current.StopRequested {
				return nil
			}
			if current.PR != nil || !value.NoChanges || value.Outcome != "done" || !model.ValidRevision(value.Head) || !model.ValidRevision(value.Base) {
				s.block(current, "收尾结果缺少有效PR或无改动证据")
			} else {
				current.Phase = "report"
			}
			return nil
		})
		return err
	}
	pr, _, err := s.verifyPR(ctx, r, value)
	if err != nil {
		return err
	}
	_, err = s.update(ctx, r.ID, func(current *model.Record) error {
		if current.Revision != r.Revision || current.StopRequested {
			return nil
		}
		if n := len(current.Operations); n > 0 {
			o := current.Operations[n-1]
			if o.Purpose == "version_audit" && o.DecisionKind == "accept" {
				current.PR = pr
				if o.PRVersion == prVersion(pr) {
					current.Phase = "report"
				} else if r.PR != nil && prVersion(r.PR) != prVersion(pr) {
					s.block(current, "复核期间PR版本再次变化，请核验后继续")
				} else {
					current.Phase = "version_audit"
				}
				return nil
			}
			if o.Result != nil && (o.Purpose == "collect_evidence" || o.Purpose == "version_check") {
				if !matchesVersion(pr, value.PR, o.Result.Head, o.Result.Base) {
					current.PR = pr
					if prVersion(r.PR) == prVersion(pr) {
						current.Phase = "version_audit"
					} else {
						s.block(current, "补证期间PR版本再次变化，请核验后继续")
					}
				} else {
					current.PR = pr
					current.Phase = "report"
					if o.Purpose == "version_check" {
						current.Phase = "version_audit"
					}
				}
				return nil
			}
		}
		current.PR = pr
		if matchesVersion(pr, value.PR, value.Head, value.Base) {
			current.Phase = "report"
		} else {
			current.Phase = "version_audit"
		}
		return nil
	})
	return err
}

func (s *Service) prepareSync(ctx context.Context, r model.Record) error {
	return s.checkSync(ctx, r, false)
}

func (s *Service) checkSync(ctx context.Context, r model.Record, after bool) error {
	report := pendingReport(r)
	if report == nil {
		_, err := s.update(ctx, r.ID, func(current *model.Record) error { s.block(current, "缺少待同步的报告版本"); return nil })
		return err
	}
	pr, body, err := s.verifyPR(ctx, r, model.Result{PR: report.PR, Head: report.Head, Base: report.Base})
	if err != nil {
		return err
	}
	_, err = s.update(ctx, r.ID, func(current *model.Record) error {
		if current.Revision != r.Revision || current.StopRequested {
			return nil
		}
		current.PR = pr
		if !matchesVersion(pr, report.PR, report.Head, report.Base) {
			current.Phase = "version_audit"
			previous := len(current.Reports) - 2
			if previous >= 0 && current.Reports[previous].SyncedAt == nil {
				s.block(current, "PR版本再次变化，请核验后继续")
			}
		} else if body == report.Body {
			now := s.now()
			current.Reports[len(current.Reports)-1].SyncedAt = &now
			s.deliver(current)
		} else if after {
			s.block(current, "原PR正文未同步，核实原操作后可继续同步")
		} else {
			current.PendingPrompt = report.SyncPrompt
			current.WriteKind = "sync"
			current.Phase = "execute"
		}
		return nil
	})
	return err
}

func questionText(s string) bool { return strings.TrimSpace(s) != "" }

func validQuestion(q *model.QuestionBody) bool {
	if q == nil || !questionText(q.Prompt) || len(q.Facts) == 0 || q.Attempts == nil ||
		len(q.Options) == 0 || !questionText(q.RecommendationReason) {
		return false
	}
	for _, values := range [][]string{q.Facts, q.Attempts} {
		for _, value := range values {
			if !questionText(value) {
				return false
			}
		}
	}
	ids := make(map[string]bool, len(q.Options))
	for _, option := range q.Options {
		if !questionText(option.ID) || ids[option.ID] || !questionText(option.Label) || !questionText(option.Impact) {
			return false
		}
		ids[option.ID] = true
	}
	return ids[q.RecommendedOptionID]
}
