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
	if r.Candidate != nil {
		return "verifying"
	}
	if r.PR != nil && r.WriteKind != "conflict" {
		return "report"
	}
	return "finish"
}

func uncertainPublication(r model.Record) bool {
	if r.Candidate != nil || r.PR != nil {
		return false
	}
	for i := len(r.Operations) - 1; i >= 0; i-- {
		o := r.Operations[i]
		if o.Purpose == "publish" {
			return o.State == "complete"
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
		s.closeAction(r)
	case "accept":
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
		case "audit":
			r.WriteKind = "repair"
		}
		r.PendingPrompt = d.Prompt
		r.Phase = "execute"
	}
	return nil
}

func successfulExecution(o model.Operation) bool {
	return o.Result != nil && nativeExecutionSucceeded(o.Run)
}

func nativeExecutionSucceeded(run *runnercontract.Run) bool {
	return nativeFinalBound(run) && run.Status == "completed" && run.Final.NativeOutcome == "succeeded"
}

func nativeFinalBound(run *runnercontract.Run) bool {
	if run == nil || run.StopStatus != "stopped" || run.Final == nil {
		return false
	}
	f := run.Final
	return f.Verified && f.CommandID != "" &&
		f.AttemptID != "" && f.AttemptID == run.Execution.AttemptID &&
		f.SessionID != "" && f.SessionID == run.Execution.SessionID &&
		f.TurnID != "" && f.TurnID == run.Execution.TurnID
}

func (s *Service) applyExecution(r *model.Record, o model.Operation) {
	trusted := successfulExecution(o)
	completed := trusted && o.Result.Outcome == "done" && strings.TrimSpace(o.Result.Summary) != ""
	switch o.Purpose {
	case "conflict":
		if r.PR != nil {
			r.Candidate = &model.Result{PR: r.PR.URL}
			if completed && o.Result.PRURL() == r.PR.URL {
				r.Candidate = o.Result
			}
		}
	case "publish":
		if completed && (o.Result.PR != "" || o.Result.NoChanges && model.ValidRevision(o.Result.Head) && model.ValidRevision(o.Result.Base)) {
			r.Candidate = o.Result
		} else if trusted && o.Result.PR != "" {
			r.Candidate = &model.Result{PR: o.Result.PR}
		}
	}
	if r.StopRequested {
		return
	}
	switch o.Purpose {
	case "publish", "conflict":
		if r.Candidate != nil {
			r.Phase = "verifying"
		} else {
			s.block(r, "原发布操作未取得可核验结果，请核实原操作及PR状态")
		}
	default:
		r.Phase = "audit"
	}
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

func (s *Service) deliver(r *model.Record) {
	r.Phase = "delivered"
	r.Candidate = nil
	r.ConflictReview = false
	r.Diagnostic = ""
	now := s.now()
	r.FinishedAt = &now
	r.Pause(now)
}

func (s *Service) verifyPR(ctx context.Context, r model.Record, v model.Result) (*runnercontract.PRFact, string, error) {
	if s.Verify == nil {
		return nil, "", errors.New("缺少只读PR核验")
	}
	v.PR = v.PRURL()
	pr, checks, err := s.Verify(ctx, r, v)
	if err != nil {
		return nil, "", err
	}
	if pr == nil || pr.URL != v.PR || !model.ValidRevision(pr.HeadRevision) || !model.ValidRevision(pr.BaseRevision) {
		return nil, "", errors.New("平台PR身份或head/base尚未核实")
	}
	return pr, checks, nil
}

func (s *Service) verifyDelivery(ctx context.Context, r model.Record) error {
	if r.Candidate == nil {
		_, err := s.update(ctx, r.ID, func(current *model.Record) error {
			if current.Revision == r.Revision {
				s.block(current, "收尾缺少可核验结果")
			}
			return nil
		})
		return err
	}
	value := *r.Candidate
	var pr *runnercontract.PRFact
	if value.PR != "" {
		var err error
		pr, _, err = s.verifyPR(ctx, r, value)
		if err != nil {
			return err
		}
		if r.WriteKind == "conflict" && strings.EqualFold(pr.State, "open") && !pr.Merged &&
			r.PR != nil && r.PR.HeadRevision != pr.HeadRevision && s.VerifyConflictBase != nil {
			if err := s.VerifyConflictBase(ctx, r, *pr); err != nil {
				_, saveErr := s.update(ctx, r.ID, func(current *model.Record) error {
					if current.Revision == r.Revision && !current.StopRequested {
						s.block(current, err.Error())
					}
					return nil
				})
				return saveErr
			}
		}
	}
	_, err := s.update(ctx, r.ID, func(current *model.Record) error {
		if current.Revision != r.Revision || current.StopRequested {
			return nil
		}
		switch {
		case pr != nil && r.WriteKind == "conflict" && value.Outcome == "done" &&
			(value.Head != pr.HeadRevision || value.Base != pr.BaseRevision):
			s.block(current, "续作回执的 head/base 与原 PR 当前版本不一致")
		case pr != nil && current.WriteKind == "conflict" && current.PR != nil && current.PR.HeadRevision == pr.HeadRevision:
			s.block(current, "原 PR 尚无可核实的新提交，请核对原发布操作")
		case pr != nil && strings.EqualFold(pr.State, "open") && !pr.Merged:
			current.PR, current.Candidate = pr, nil
			current.ConflictReview = current.WriteKind == "conflict"
			current.WriteKind = ""
			current.Phase = "report"
			current.Diagnostic = ""
		case pr == nil && current.PR == nil && value.NoChanges && value.Outcome == "done" &&
			model.ValidRevision(value.Head) && model.ValidRevision(value.Base):
			current.Summary = value.Summary
			s.deliver(current)
		default:
			s.block(current, "收尾结果缺少有效的待审 PR 或无改动证据")
		}
		return nil
	})
	return err
}

func (s *Service) reviewEvidence(ctx context.Context, r model.Record) (*runnercontract.PRFact, model.ReviewEvidence, error) {
	pr, checks, err := s.verifyPR(ctx, r, model.Result{PR: r.PR.URL})
	if err != nil {
		return nil, model.ReviewEvidence{}, err
	}
	evidence := model.ReviewEvidence{Checks: checks}
	if pr.Merged || !strings.EqualFold(pr.State, "open") {
		return pr, evidence, nil
	}
	if err := s.verifyConflictReviewBase(ctx, r, *pr); err != nil {
		return nil, model.ReviewEvidence{}, err
	}
	if s.ReviewDiff == nil {
		evidence.Limitation = "未配置读取 PR 差异的能力"
	} else {
		evidence.Diff, err = s.ReviewDiff(ctx, r, *pr)
		if err != nil || strings.TrimSpace(evidence.Diff) == "" {
			evidence.Diff = ""
			evidence.Limitation = "无法取得该 PR 版本的完整差异"
		}
	}
	return pr, evidence, nil
}

func (s *Service) verifyConflictReviewBase(ctx context.Context, r model.Record, pr runnercontract.PRFact) error {
	if r.ConflictReview && s.VerifyConflictBase != nil && pr.State == "open" && !pr.Merged &&
		!matchesVersion(r.PR, pr.URL, pr.HeadRevision, pr.BaseRevision) {
		return s.VerifyConflictBase(ctx, r, pr)
	}
	return nil
}

func (s *Service) applyReview(r *model.Record, op model.Operation, latest *runnercontract.PRFact, checks string, review decision.Review) {
	r.PR = latest
	if latest.Merged || !strings.EqualFold(latest.State, "open") {
		s.deliver(r)
		return
	}
	if op.PRVersion != prVersion(latest) || op.ReviewChecks != checks {
		r.Phase = "report"
		r.Diagnostic = "PR 版本或检查状态已变化，正在重新评审"
		return
	}
	s.saveReview(r, checks, review.Recommendation, review.Report)
}

func (s *Service) saveReview(r *model.Record, checks, recommendation, body string) {
	r.Reports = append(r.Reports, model.Report{Body: body, Recommendation: recommendation,
		PR: r.PR.URL, Head: r.PR.HeadRevision, Base: r.PR.BaseRevision, Checks: checks, CreatedAt: s.now()})
	s.deliver(r)
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
