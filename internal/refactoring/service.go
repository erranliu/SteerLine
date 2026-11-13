package refactoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	decision "github.com/erranliu/SteerLine/internal/steeragent/maintenance"
)

type Store interface {
	Refactoring(context.Context, string, string) (model.Record, error)
	Refactorings(context.Context, string) ([]model.Record, error)
	SaveRefactoring(context.Context, model.Record, uint64, *model.Command, ...env.Admission) error
	RefactoringCommand(context.Context, string, string) ([]byte, []byte, error)
}
type Runner interface {
	Submit(context.Context, runnercontract.Request) (runnerclient.Receipt, error)
	Inspect(context.Context, string, string) (runnerclient.Inspection, error)
	Call(context.Context, string, []string, *runnercontract.Request) (runnerclient.Response, error)
}
type Service struct {
	Project              string
	Store                Store
	Runner               Runner
	Prepare              func(context.Context, model.Start) (model.Frozen, error)
	Reserve              func(context.Context, string, func() error) error
	Admit                func(context.Context) error
	Qualify              func(context.Context, model.Record) (env.Admission, error)
	Authorize            func(context.Context, model.Record, func() error) error
	Verify               func(context.Context, model.Record, model.Result) (*runnercontract.PRFact, string, error)
	VerifyConflictBase   func(context.Context, model.Record, runnercontract.PRFact) error
	ReviewDiff           func(context.Context, model.Record, runnercontract.PRFact) (string, error)
	InvestigationPath    func(string) (string, error)
	Investigate          func(context.Context, model.Record, string) error
	ReleaseInvestigation func(string) error
	ReadCost             func(context.Context, model.Record, model.Operation) (model.Cost, error)
	Queue                interface {
		UpdateContent(string, string, time.Time, bool, bool)
		AcquireContent(string, string) bool
	}
	Now           func() time.Time
	mu            sync.Mutex
	advance       sync.Mutex
	reviewChecked map[string]time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Service) save(ctx context.Context, r *model.Record, cmd *model.Command, proof ...env.Admission) error {
	expected := r.Revision
	r.Revision++
	r.UpdatedAt = s.now()
	if !r.Unsettled() {
		if !r.Active() || r.Phase == "waiting_input" {
			r.Slot = false
			r.WaitReason = ""
			r.Pause(r.UpdatedAt)
		}
		if !r.Active() && r.FinishedAt == nil {
			r.FinishedAt = &r.UpdatedAt
		}
	}
	if err := s.Store.SaveRefactoring(ctx, *r, expected, cmd, proof...); err != nil {
		r.Revision = expected
		return err
	}
	if s.Queue != nil {
		for _, o := range r.Operations {
			if o.Content(*r).WorkflowID == "generate" {
				s.Queue.UpdateContent(s.Project, "refactoring:"+o.ID, o.CreatedAt, o.State == "submitted", o.Settled() || !r.Slot)
			}
		}
	}
	return nil
}
func (s *Service) update(ctx context.Context, id string, apply func(*model.Record) error) (model.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.Store.Refactoring(ctx, s.Project, id)
	if e != nil {
		return r, e
	}
	if e = apply(&r); e == nil {
		e = s.save(ctx, &r, nil)
	}
	return r, e
}
func (s *Service) replay(ctx context.Context, c model.Command) (model.Record, bool, error) {
	raw, result, e := s.Store.RefactoringCommand(ctx, s.Project, c.ID)
	if e == nil {
		if string(raw) != string(c.Raw) {
			return model.Record{}, true, errors.New("request_conflict: 原维护命令不同")
		}
		var r model.Record
		e = json.Unmarshal(result, &r)
		r.Project = s.Project
		return r, true, e
	}
	if !errors.Is(e, model.ErrNotFound) {
		return model.Record{}, true, e
	}
	return model.Record{}, false, nil
}
func (s *Service) Command(ctx context.Context, c model.Command, in *model.Start) (model.Record, error) {
	if !model.ValidID(c.ID) || !json.Valid(c.Raw) {
		return model.Record{}, errors.New("无效维护请求")
	}
	if r, found, err := s.replay(ctx, c); found {
		return r, err
	}
	var e error
	if c.Action == "refactoring_start" {
		if in == nil || c.Target != s.Project || c.Expected != 1 {
			return model.Record{}, errors.New("启动目标或版本无效")
		}
		if e = in.Validate(); e != nil {
			return model.Record{}, e
		}
		if s.Admit != nil {
			if e = s.Admit(ctx); e != nil {
				return model.Record{}, e
			}
		}
		f, e := s.Prepare(ctx, *in)
		if e != nil {
			return model.Record{}, e
		}
		now := s.now()
		r := model.Record{ID: c.ID, Project: s.Project, Input: *in, Frozen: f, Phase: "first", CreatedAt: now, EnqueuedAt: now, Operations: []model.Operation{}, Answers: []model.Answer{}, Reports: []model.Report{}}
		create := func() error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if replay, found, err := s.replay(ctx, c); found {
				r = replay
				return err
			}
			if f.Environment != nil {
				return s.save(ctx, &r, &c, *f.Environment)
			}
			return s.save(ctx, &r, &c)
		}
		if s.Reserve != nil {
			e = s.Reserve(ctx, f.Repository.BaseRevision, create)
		} else {
			e = create()
		}
		return r, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, found, err := s.replay(ctx, c); found {
		return r, err
	}
	r, e := s.Store.Refactoring(ctx, s.Project, c.Target)
	if e != nil {
		return r, e
	}
	if c.Expected == 0 || c.Expected > r.Revision || r.Revision != c.Expected && c.Action != "refactoring_stop" && c.Action != "refactoring_finish" {
		return r, model.ErrConflict
	}
	switch c.Action {
	case "refactoring_stop":
		if !r.Active() || r.StopRequested {
			return r, errors.New("当前行动不可停止")
		}
		r.StopRequested = true
		r.Phase = "stopping"
		r.Pause(s.now())
		r.Question = nil
		r.WaitReason = ""
		if n := len(r.Operations); n > 0 && r.Operations[n-1].State == "pending" {
			r.Operations[n-1].State = "cancelled"
		}
		if !r.Unsettled() {
			r.Phase = "stopped"
		}
	case "refactoring_finish":
		if !r.CanFinish() {
			return r, errors.New("当前行动不可收尾")
		}
		if !r.Active() && uncertainPublication(r) {
			return r, errors.New("原发布结果未核实，请先核对原请求和原PR")
		}
		if n := len(r.Operations); n > 0 {
			last := &r.Operations[n-1]
			if last.State == "pending" && (last.Purpose == "first" || last.Purpose == "select") {
				last.State = "cancelled"
			}
		}
		if r.Phase == "execute" && r.WriteKind == "implement" {
			if n := len(r.Operations); n > 0 && r.Operations[n-1].Purpose == "implement" && r.Operations[n-1].State == "pending" {
				r.Operations[n-1].State = "cancelled"
				r.PendingPrompt = ""
				r.Round--
				r.Phase = closingPhase(r)
			} else if r.PendingPrompt != "" {
				r.PendingPrompt = ""
				r.Round--
				r.Phase = closingPhase(r)
			}
		}
		if r.Phase == "waiting_input" {
			r.Phase = closingPhase(r)
			r.Question = nil
			r.EnqueuedAt = s.now()
		}
		if !r.Active() {
			r.StopRequested = false
			r.EnqueuedAt = s.now()
			r.Phase = closingPhase(r)
		}
		r.FinishRequested = true
		r.FinishedAt = nil
	case "refactoring_answer":
		if r.Phase != "waiting_input" || r.Question == nil || r.Question.ID != c.QuestionID || len(c.Note) > 32<<10 {
			return r, errors.New("原问题已变化或答复无效")
		}
		var chosen *model.QuestionOption
		for i := range r.Question.Options {
			if r.Question.Options[i].ID == c.OptionID {
				chosen = &r.Question.Options[i]
				break
			}
		}
		if chosen == nil {
			return r, errors.New("原问题中没有该选项")
		}
		r.Answers = append(r.Answers, model.Answer{QuestionID: c.QuestionID, Option: *chosen, Note: c.Note})
		r.Phase = r.Question.Mode
		r.Question = nil
		r.EnqueuedAt = s.now()
	case "refactoring_review":
		if !r.CanReview() {
			return r, errors.New("当前行动不可重新评审")
		}
		r.Phase = "report"
		r.StopRequested = false
		r.FinishedAt = nil
		r.EnqueuedAt = s.now()
	case "refactoring_conflict":
		if r.Phase != "delivered" || r.PR == nil || r.PR.Merged || r.PR.State == "closed" {
			return r, errors.New("原PR不允许冲突续作")
		}
		if s.Verify == nil {
			return r, errors.New("缺少原PR核验")
		}
		s.mu.Unlock()
		pr, _, err := s.verifyPR(ctx, r, model.Result{PR: r.PR.URL})
		s.mu.Lock()
		if err != nil {
			return r, err
		}
		if pr.Merged || !strings.EqualFold(pr.State, "open") {
			return r, errors.New("原 PR 已关闭或合并")
		}
		current, err := s.Store.Refactoring(ctx, s.Project, r.ID)
		if err != nil {
			return r, err
		}
		if current.Revision != r.Revision {
			return r, model.ErrConflict
		}
		r.PR = pr
		r.ConflictPending = true
		r.Phase = "finish"
		r.FinishRequested = true
		r.StopRequested = false
		r.Candidate = nil
		r.WriteKind = "conflict"
		r.FinishedAt = nil
		r.EnqueuedAt = s.now()
	default:
		return r, errors.New("不支持的维护操作")
	}
	r.Diagnostic = ""
	e = s.save(ctx, &r, &c)
	return r, e
}

func (s *Service) tick(ctx context.Context, r model.Record) error {
	if len(r.Operations) > 0 {
		o := r.Operations[len(r.Operations)-1]
		if !o.Settled() {
			if o.State == "pending" {
				return s.submit(ctx, r, o)
			}
			return s.observe(ctx, r, o)
		}
	}
	if r.StopRequested {
		_, e := s.update(ctx, r.ID, func(r *model.Record) error { r.Phase = "stopped"; r.Pause(s.now()); return nil })
		return e
	}
	switch r.Phase {
	case "verifying":
		return s.verifyDelivery(ctx, r)
	}
	if r.Phase == "waiting_input" || r.Phase == "blocked" || r.Phase == "delivered" || r.Phase == "stopped" {
		return nil
	}
	if (r.Phase == "first" || r.Phase == "select") && (r.FinishRequested || r.BudgetExhausted(s.now())) || r.Phase == "finish" && r.LatestOwner == "" {
		_, e := s.update(ctx, r.ID, func(r *model.Record) error { s.closeAction(r); return nil })
		return e
	}
	if s.Admit != nil {
		if e := s.Admit(ctx); e != nil {
			return e
		}
	}
	id := model.ID()
	var directory string
	var prepareErr error
	if r.Phase == "first" {
		if s.Investigate == nil || s.InvestigationPath == nil {
			prepareErr = errors.New("缺少固定首轮调查目录")
		} else {
			directory, prepareErr = s.InvestigationPath(id)
		}
	}
	var pr *runnercontract.PRFact
	var review model.ReviewEvidence
	if r.Phase == "report" {
		if r.PR == nil {
			return errors.New("缺少待评审 PR")
		}
		var err error
		pr, review, err = s.reviewEvidence(ctx, r)
		if err != nil {
			return err
		}
		if review.Limitation != "" {
			latest, checks, err := s.verifyPR(ctx, r, model.Result{PR: r.PR.URL})
			if err != nil {
				return err
			}
			if !matchesVersion(pr, latest.URL, latest.HeadRevision, latest.BaseRevision) || checks != review.Checks {
				return nil
			}
			pr = latest
		}
	}
	_, e := s.update(ctx, r.ID, func(current *model.Record) error {
		if current.StopRequested || current.Revision != r.Revision {
			return nil
		}
		if pr != nil {
			current.PR = pr
			if pr.Merged || !strings.EqualFold(pr.State, "open") {
				s.deliver(current)
				return nil
			}
			if review.Limitation != "" {
				s.saveReview(current, review.Checks, "insufficient_evidence", review.Limitation)
				return nil
			}
		}
		if prepareErr == nil {
			var op model.Operation
			op, prepareErr = s.prepareOperation(*current, id, directory, review)
			if prepareErr == nil {
				current.Operations = append(current.Operations, op)
				current.PendingPrompt = ""
				return nil
			}
		}
		s.block(current, prepareErr.Error())
		return nil
	})
	return e
}

func (s *Service) submit(ctx context.Context, r model.Record, o model.Operation) error {
	if s.Admit != nil {
		if err := s.Admit(ctx); err != nil {
			_, saveErr := s.update(ctx, r.ID, func(current *model.Record) error {
				last := &current.Operations[len(current.Operations)-1]
				if last.ID == o.ID && last.State == "pending" && !current.StopRequested {
					last.State = "cancelled"
					current.Phase = "blocked"
					current.Diagnostic = err.Error()
				}
				return nil
			})
			return saveErr
		}
	}
	if r.ConflictPending {
		fact, _, err := s.verifyPR(ctx, r, model.Result{PR: r.PR.URL})
		if err == nil && (fact.Merged || !strings.EqualFold(fact.State, "open")) {
			_, err = s.update(ctx, r.ID, func(current *model.Record) error {
				last := &current.Operations[len(current.Operations)-1]
				if last.ID == o.ID && last.State == "pending" && !current.StopRequested {
					last.State = "cancelled"
					current.Phase = "delivered"
					current.ConflictPending = false
					current.Diagnostic = model.ErrPRClosed.Error()
					if fact != nil {
						current.PR = fact
					}
				}
				return nil
			})
			return err
		}
		if err != nil {
			return err
		}
		if fact.HeadRevision != r.PR.HeadRevision {
			_, err = s.update(ctx, r.ID, func(current *model.Record) error {
				last := &current.Operations[len(current.Operations)-1]
				if last.ID == o.ID && last.State == "pending" && !current.StopRequested {
					last.State = "cancelled"
					s.block(current, "排队期间原 PR 的 head 已变化，请重新核对原交付")
				}
				return nil
			})
			return err
		}
		if fact.BaseRevision != r.PR.BaseRevision {
			_, err = s.update(ctx, r.ID, func(current *model.Record) error {
				last := &current.Operations[len(current.Operations)-1]
				if last.ID == o.ID && last.State == "pending" && !current.StopRequested {
					last.State = "cancelled"
					current.PR = fact
					current.Phase = "finish"
					current.PendingPrompt = ""
					current.Diagnostic = "目标基线已变化，重新准备原 PR 续作"
				}
				return nil
			})
			return err
		}
	}
	var proof []env.Admission
	if !r.Authorized && s.Qualify != nil {
		qualification, err := s.Qualify(ctx, r)
		if err != nil {
			return s.deferFirstDispatch(ctx, r.ID, o.ID, err)
		}
		proof = append(proof, qualification)
	}
	var send bool
	mark := func() error {
		var err error
		send, err = s.markSubmitted(ctx, r.ID, o, proof)
		return err
	}
	var err error
	if !r.Authorized && s.Authorize != nil {
		err = s.Authorize(ctx, r, mark)
	} else {
		err = mark()
	}
	if errors.Is(err, model.ErrEnvironmentChanged) || errors.Is(err, model.ErrQualificationUnverified) {
		return s.deferFirstDispatch(ctx, r.ID, o.ID, err)
	}
	if err != nil || !send {
		return err
	}
	if o.Investigation != "" {
		if err := s.Investigate(ctx, r, o.ID); err != nil {
			_, saveErr := s.update(context.WithoutCancel(ctx), r.ID, func(current *model.Record) error {
				last := &current.Operations[len(current.Operations)-1]
				if last.ID != o.ID || last.State != "submitted" {
					return nil
				}
				last.State = "rejected"
				last.Feedback = err.Error()
				if !current.StopRequested {
					s.block(current, err.Error())
				}
				return nil
			})
			return saveErr
		}
	}
	receipt, callErr := s.Runner.Submit(ctx, o.Request)
	_, e := s.update(context.WithoutCancel(ctx), r.ID, func(r *model.Record) error {
		last := &r.Operations[len(r.Operations)-1]
		if last.ID != o.ID {
			return errors.New("原维护输入被替换")
		}
		if callErr == nil && receipt.RunID != "" && receipt.ReceiptStatus == "accepted" {
			last.RunID = receipt.RunID
			if last.Content(*r).WorkflowID == "execute" {
				r.LatestOwner = receipt.RunID
			}
		} else {
			last.Feedback = "原提交结果待核实"
			if callErr != nil {
				last.Feedback = callErr.Error()
			}
			var failure *runnerclient.Failure
			if errors.As(callErr, &failure) && !failure.Unknown {
				last.State = "rejected"
				r.Phase = "blocked"
				r.Diagnostic = last.Feedback
				r.Pause(s.now())
			}
		}
		return nil
	})
	return e
}

func (s *Service) observe(ctx context.Context, r model.Record, o model.Operation) error {
	binding := o.Content(r)
	selector, id := binding.Selector()
	view, e := s.Runner.Inspect(ctx, selector, id)
	if e != nil {
		return e
	}
	if e = runnerbinding.ValidateContent(binding, view); e != nil {
		return e
	}
	run := view.Run
	if run.Correlation["refactoring_operation"] != o.ID {
		return errors.New("维护操作来源不匹配")
	}
	if binding.WorkflowID == "execute" {
		var in runnercontract.RunInput
		if json.Unmarshal(o.Request.Input, &in) != nil {
			return errors.New("执行原请求无效")
		}
		if run.ReuseWorktreeFromRunID != in.ReuseWorktreeFromRunID || r.Identity != "" && run.WorkspaceIdentity != r.Identity || r.Origin != "" && run.WorkspaceOriginRunID != r.Origin {
			return errors.New("维护工作区来源链不匹配")
		}
	}
	r, e = s.update(ctx, r.ID, func(r *model.Record) error {
		last := &r.Operations[len(r.Operations)-1]
		if last.ID != o.ID {
			return model.ErrConflict
		}
		if !r.StopRequested && (last.Run == nil || run.Revision > last.Run.Revision) {
			if run.ExecutionWait != nil && run.ExecutionWait.Stage == "queue" && run.StopStatus != "stopped" {
				r.WaitReason = model.WaitRunnerCapacity
				r.Pause(observationTime(run.ExecutionWait.StartedAt, s.now()))
			} else if r.WaitReason == model.WaitRunnerCapacity {
				r.WaitReason = ""
				r.Resume(observationTime(run.UpdatedAt, s.now()))
			}
		}
		last.ObserveRun(run)
		if binding.WorkflowID == "execute" {
			r.LatestOwner = run.RunID
			if run.WorkspaceIdentity != "" {
				r.Identity = run.WorkspaceIdentity
				r.Origin = run.WorkspaceOriginRunID
				r.Workspace = run.Execution.Workspace
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	o = r.Operations[len(r.Operations)-1]
	if r.StopRequested && run.StopStatus != "stopped" {
		return s.stopOperation(ctx, r, o)
	}
	if run.StopStatus != "stopped" {
		return nil
	}
	content := run.ContentResult
	decisionSucceeded := run.Status == "completed" && run.Final != nil && run.Final.Verified && run.Final.NativeOutcome == "succeeded" &&
		content != nil && content.Status == "succeeded" && len(content.JSON) > 0
	if binding.WorkflowID == "generate" && decisionSucceeded && !r.StopRequested {
		var in runnercontract.GenerateRunInput
		if json.Unmarshal(o.Request.Input, &in) != nil {
			return errors.New("原只读请求无效")
		}
		if err := runnerclient.VerifyFileMaterials(in.Generate.Materials, run.Materials); err != nil {
			return err
		}
		if in.Generate.Repository != nil {
			if e := runnerclient.VerifyRepositoryFiles(run.RepositorySource, *in.Generate.Repository, nil); e != nil {
				return e
			}
		} else if run.RepositorySource != nil ||
			in.Generate.Directory != nil && !runnerbinding.SamePath(run.Execution.Workspace, in.Generate.Directory.Path) ||
			in.Generate.Directory == nil && !runnerbinding.SamePath(run.Execution.Workspace, filepath.Join(r.Frozen.Runner.StateDir, "runs", run.RunID, "content")) {
			return errors.New("只读目录来源不匹配")
		}
		if err := runnerclient.VerifyFileMaterials(in.Generate.Materials, content.FileMaterials); err != nil {
			return err
		}
		if in.Generate.Repository != nil {
			if e := runnerclient.VerifyRepositoryFiles(content.RepositorySource, *in.Generate.Repository, nil); e != nil {
				return e
			}
		} else if content.RepositorySource != nil {
			return errors.New("只读结果意外关联了仓库快照")
		}
		if !reflect.DeepEqual(content.Source, *run.Final) {
			return errors.New("只读结果与原生Final来源不匹配")
		}
	}
	feedback := ""
	if run.Final != nil {
		feedback = run.Final.Text
	}
	if run.ContentResult != nil && feedback == "" {
		feedback = run.ContentResult.Text
		if feedback == "" && len(run.ContentResult.JSON) > 0 {
			feedback = string(run.ContentResult.JSON)
		}
	}
	var result *model.Result
	if binding.WorkflowID == "execute" {
		if nativeFinalBound(run) {
			var value model.Result
			var fieldType *json.UnmarshalTypeError
			decoder := json.NewDecoder(bytes.NewReader([]byte(run.Final.Text)))
			decoder.DisallowUnknownFields()
			switch err := decoder.Decode(&value); {
			case !json.Valid([]byte(run.Final.Text)):
				feedback += "\n最终回复不是单个有效的维护 JSON 对象"
			case errors.As(err, &fieldType) && fieldType.Field != "":
				feedback += "\n最终回复字段类型无效：" + fieldType.Field
			case err != nil:
				feedback += "\n最终回复不是有效的维护回执"
			case value.ActionID != r.ID || value.OperationID != o.ID:
				feedback += "\n最终回复与原维护操作不匹配"
			case value.Outcome != "done" && value.Outcome != "needs_input" && value.Outcome != "blocked":
				feedback += "\n最终回复 outcome 无效"
			default:
				result = &value
			}
		}
		if result != nil && result.Outcome == "done" && !nativeExecutionSucceeded(run) {
			feedback += "\n原生执行未成功，不能采用 done 回执"
			result = nil
		}
		if !r.StopRequested {
			o.WorkspaceGit = s.observeWorkspaceGit(ctx, run.RunID)
		}
	}
	if s.ReadCost != nil && r.Frozen.Pricing != nil {
		cost, err := s.ReadCost(ctx, r, o)
		if err != nil {
			cost = model.Cost{Diagnostic: err.Error(), ObservedAt: s.now()}
		}
		if !cost.Complete && r.Input.Budget.Dollars != "" && !r.StopRequested && s.now().Sub(run.UpdatedAt) < 35*time.Second {
			return nil
		}
		o.Cost = cost
	}
	var latest *runnercontract.PRFact
	var latestChecks string
	var conflictBaseErr error
	if o.Purpose == "report" && decisionSucceeded && !r.StopRequested {
		latest, latestChecks, e = s.verifyPR(ctx, r, model.Result{PR: r.PR.URL})
		if e != nil {
			return e
		}
		conflictBaseErr = s.verifyConflictReviewBase(ctx, r, *latest)
	}
	_, e = s.update(ctx, r.ID, func(r *model.Record) error {
		last := &r.Operations[len(r.Operations)-1]
		if last.ID != o.ID {
			return model.ErrConflict
		}
		last.Feedback = feedback
		last.Result = result
		last.Cost = o.Cost
		last.WorkspaceGit = o.WorkspaceGit
		last.State = "complete"
		now := s.now()
		last.CompletedAt = &now
		if s.Queue != nil && binding.WorkflowID == "generate" {
			s.Queue.UpdateContent(s.Project, "refactoring:"+o.ID, o.CreatedAt, true, true)
		}
		if binding.WorkflowID == "execute" {
			s.applyExecution(r, *last)
			return nil
		}
		if r.StopRequested {
			return nil
		}
		if conflictBaseErr != nil {
			s.block(r, conflictBaseErr.Error())
			return nil
		}
		if !decisionSucceeded {
			detail := run.Diagnostic
			if detail == "" && content != nil {
				detail = content.Diagnostic
			}
			if detail == "" && run.Status == "completed" && (content == nil || content.Status == "succeeded" && len(content.JSON) == 0) {
				detail = "缺少结构化决策结果 ContentResult.JSON"
			}
			if detail == "" {
				detail = run.Status
			}
			if retryableDecisionOutput(content) {
				alreadyRetried := false
				if len(r.Operations) > 1 {
					previous := r.Operations[len(r.Operations)-2]
					alreadyRetried = previous.Purpose == o.Purpose && previous.Round == o.Round && previous.Run != nil && retryableDecisionOutput(previous.Run.ContentResult)
				}
				if !alreadyRetried {
					r.Diagnostic = "只读决策输出无效，重新生成一次：" + detail
					return nil
				}
			}
			r.Phase = "blocked"
			r.Diagnostic = "只读决策未取得可信成功结果：" + detail
			r.Pause(now)
			return nil
		}
		if o.Purpose == "report" {
			review, err := decision.AcceptReview(content.JSON)
			if err != nil {
				s.block(r, err.Error())
			} else {
				s.applyReview(r, *last, latest, latestChecks, review)
			}
			return nil
		}
		d, de := decision.Accept(content.JSON, o.Purpose)
		if de != nil {
			r.Phase = "blocked"
			r.Diagnostic = de.Error()
			r.Pause(now)
			return nil
		}
		last.DecisionSummary, last.DecisionKind = d.Summary, d.Kind
		return s.applyDecision(r, o.Purpose, d)
	})
	return e
}

func retryableDecisionOutput(result *runnercontract.ContentResult) bool {
	return result != nil && result.Status == "failed" &&
		(result.Diagnostic == "human_input_required" || invalidDecisionJSON(result))
}

func invalidDecisionJSON(result *runnercontract.ContentResult) bool {
	return result != nil && result.Status == "failed" &&
		(result.Diagnostic == "invalid_json" || strings.HasPrefix(result.Diagnostic, "invalid_json:"))
}

func (s *Service) observeWorkspaceGit(ctx context.Context, runID string) *runnercontract.WorkspaceGit {
	response, err := s.Runner.Call(ctx, "workspace-git", []string{"--run-id", runID}, nil)
	var fact runnercontract.WorkspaceGit
	if err != nil || json.Unmarshal(response.Data, &fact) != nil || fact.RunID != runID || !fact.Verified || fact.Commit == nil || !model.ValidRevision(*fact.Commit) || fact.Clean == nil || fact.ObservedAt.IsZero() {
		return nil
	}
	return &fact
}

func (s *Service) stopOperation(ctx context.Context, r model.Record, o model.Operation) error {
	if o.Stop == nil || o.StopState == "rejected" {
		request := runnerbinding.ContentStop(model.ID(), o.Run.Revision)
		_, e := s.update(ctx, r.ID, func(r *model.Record) error {
			last := &r.Operations[len(r.Operations)-1]
			if last.Stop == nil || last.StopState == "rejected" {
				last.Stop = request
				last.StopState = "pending"
			}
			return nil
		})
		return e
	}
	if o.StopState == "unknown" {
		v, e := s.Runner.Inspect(ctx, "request-id", o.Stop.RequestID)
		if e != nil {
			return e
		}
		if v.Command == nil || v.Command.RequestID != o.Stop.RequestID || v.Command.Command != "stop" || v.Command.RunID == nil || *v.Command.RunID != o.RunID {
			return errors.New("原停止绑定不匹配")
		}
		if v.Command.ActionStatus == "rejected" || v.Command.ActionStatus == "failed" {
			_, e = s.update(ctx, r.ID, func(r *model.Record) error {
				last := &r.Operations[len(r.Operations)-1]
				last.StopState = "rejected"
				last.PriorStops = append(last.PriorStops, model.StopAttempt{Request: *last.Stop, State: "rejected"})
				r.Diagnostic = "原停止明确失败，按原Run最新版本重新停止"
				return nil
			})
			return e
		}
		return nil
	}
	if o.StopState != "pending" {
		return nil
	}
	_, e := s.update(ctx, r.ID, func(r *model.Record) error {
		last := &r.Operations[len(r.Operations)-1]
		last.StopState = "unknown"
		return nil
	})
	if e != nil {
		return e
	}
	_, e = s.Runner.Call(ctx, "stop", []string{"--run-id", o.RunID}, o.Stop)
	var failure *runnerclient.Failure
	if errors.As(e, &failure) && !failure.Unknown {
		_, saveErr := s.update(context.WithoutCancel(ctx), r.ID, func(r *model.Record) error {
			last := &r.Operations[len(r.Operations)-1]
			last.StopState = "rejected"
			last.PriorStops = append(last.PriorStops, model.StopAttempt{Request: *last.Stop, State: "rejected"})
			r.Diagnostic = e.Error()
			return nil
		})
		return saveErr
	}
	return e
}

// Recover revokes old pending intentions and observes/stops original executions.
func (s *Service) Recover(ctx context.Context) error { return s.revoke(ctx, true) }
func (s *Service) revoke(ctx context.Context, offline bool) error {
	rows, e := s.Store.Refactorings(ctx, s.Project)
	if e != nil {
		return e
	}
	for _, r := range rows {
		if !r.Active() || r.Phase == "waiting_input" {
			continue
		}
		_, e = s.update(ctx, r.ID, func(r *model.Record) error {
			if r.Phase == "waiting_input" {
				return nil
			}
			r.StopRequested = true
			r.Phase = "stopping"
			until := s.now()
			if offline {
				until = r.UpdatedAt
			}
			r.Pause(until)
			for i := range r.Operations {
				if r.Operations[i].State == "pending" {
					r.Operations[i].State = "cancelled"
				}
			}
			if !r.Unsettled() {
				r.Phase = "stopped"
			}
			return nil
		})
		if e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) Close(ctx context.Context) error {
	if e := s.revoke(ctx, false); e != nil {
		return e
	}
	return s.run(ctx, true)
}
