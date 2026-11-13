package refactoring

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	decision "github.com/erranliu/SteerLine/internal/steeragent/maintenance"
)

type Store interface {
	Refactoring(context.Context, string, string) (model.Record, error)
	Refactorings(context.Context, string) ([]model.Record, error)
	SaveRefactoring(context.Context, model.Record, uint64, *model.Command) error
	RefactoringCommand(context.Context, string, string) ([]byte, []byte, error)
}
type Runner interface {
	Submit(context.Context, runnercontract.Request) (runnerclient.Receipt, error)
	Inspect(context.Context, string, string) (runnerclient.Inspection, error)
	Call(context.Context, string, []string, *runnercontract.Request) (runnerclient.Response, error)
	ResultFile(context.Context, string) (runnercontract.ResultFile, error)
}
type Service struct {
	Project              string
	Store                Store
	Runner               Runner
	Prepare              func(context.Context, model.Start) (model.Frozen, error)
	Reserve              func(context.Context, string, func() error) error
	Admit                func(context.Context) error
	Verify               func(context.Context, model.Record, model.Result) (*runnercontract.PRFact, string, error)
	InvestigationPath    func(string) (string, error)
	Investigate          func(context.Context, model.Record, string) error
	ReleaseInvestigation func(string) error
	ReadCost             func(context.Context, model.Record, model.Operation) (model.Cost, error)
	Queue                interface {
		UpdateContent(string, string, time.Time, bool, bool)
		AcquireContent(string, string) bool
	}
	Now      func() time.Time
	mu       sync.Mutex
	advance  sync.Mutex
	dispatch sync.Mutex
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Service) save(ctx context.Context, r *model.Record, cmd *model.Command) error {
	expected := r.Revision
	r.Revision++
	r.UpdatedAt = s.now()
	if err := s.Store.SaveRefactoring(ctx, *r, expected, cmd); err != nil {
		r.Revision = expected
		return err
	}
	if s.Queue != nil {
		for _, o := range r.Operations {
			if o.Content(*r).WorkflowID == "generate" {
				s.Queue.UpdateContent(s.Project, "refactoring:"+o.ID, o.CreatedAt, o.State == "submitted", o.Settled())
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
		f, e := s.Prepare(ctx, *in)
		if e != nil {
			return model.Record{}, e
		}
		now := s.now()
		r := model.Record{ID: c.ID, Project: s.Project, Input: *in, Frozen: f, Phase: "first", CreatedAt: now, ActiveSince: &now, Operations: []model.Operation{}, Answers: []model.Answer{}, Reports: []model.Report{}}
		create := func() error { s.mu.Lock(); defer s.mu.Unlock(); return s.save(ctx, &r, &c) }
		if s.Reserve != nil {
			e = s.Reserve(ctx, f.Repository.BaseRevision, create)
		} else {
			e = create()
		}
		return r, e
	}
	s.dispatch.Lock()
	defer s.dispatch.Unlock()
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
	case "refactoring_finish":
		if r.Phase == "delivered" || r.StopRequested && r.Phase != "stopped" {
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
			now := s.now()
			r.ActiveSince = &now
		}
		if !r.Active() {
			r.StopRequested = false
			now := s.now()
			r.ActiveSince = &now
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
		now := s.now()
		r.ActiveSince = &now
	case "refactoring_conflict":
		if r.Phase != "delivered" || r.PR == nil || r.PR.Merged || r.PR.State == "closed" {
			return r, errors.New("原PR不允许冲突续作")
		}
		if s.Verify == nil {
			return r, errors.New("缺少原PR核验")
		}
		pr, _, err := s.verifyPR(ctx, r, model.Result{PR: r.PR.URL})
		if err != nil {
			return r, err
		}
		r.PR = pr
		r.Phase = "finish"
		r.FinishRequested = true
		r.StopRequested = false
		r.Candidate = nil
		r.WriteKind = "conflict"
		r.FinishedAt = nil
		now := s.now()
		r.ActiveSince = &now
	default:
		return r, errors.New("不支持的维护操作")
	}
	r.Diagnostic = ""
	e = s.save(ctx, &r, &c)
	return r, e
}

// Tick does bounded bookkeeping; all external calls happen outside the state lock.
func (s *Service) Tick(ctx context.Context) error {
	s.advance.Lock()
	defer s.advance.Unlock()
	rows, e := s.Store.Refactorings(ctx, s.Project)
	if e != nil {
		return e
	}
	var cleanupErr error
	for _, r := range rows {
		if s.ReleaseInvestigation != nil {
			for i, o := range r.Operations {
				if o.Settled() && o.Investigation != "" {
					if e = s.ReleaseInvestigation(o.ID); e != nil {
						cleanupErr = errors.Join(cleanupErr, e)
						continue
					}
					_, e = s.update(ctx, r.ID, func(current *model.Record) error {
						current.Operations[i].Investigation = ""
						return nil
					})
					return e
				}
			}
		}
		if !r.Active() {
			if s.ReadCost != nil && r.Frozen.Pricing != nil {
				for _, o := range r.Operations {
					if o.State == "complete" && !o.Cost.Complete && s.now().Sub(o.Cost.ObservedAt) > 30*time.Second {
						cost, err := s.ReadCost(ctx, r, o)
						if err != nil {
							cost = o.Cost
							cost.Diagnostic = err.Error()
						}
						cost.ObservedAt = s.now()
						_, err = s.update(ctx, r.ID, func(current *model.Record) error {
							for i := range current.Operations {
								if current.Operations[i].ID == o.ID {
									current.Operations[i].Cost = cost
									break
								}
							}
							return nil
						})
						return err
					}
				}
			}
			continue
		}
		if e = s.tick(ctx, r); e != nil {
			_, _ = s.update(context.WithoutCancel(ctx), r.ID, func(current *model.Record) error { current.Diagnostic = e.Error(); return nil })
			return e
		}
		return cleanupErr
	}
	return cleanupErr
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
	case "verifying", "sync_verifying":
		return s.verifyDelivery(ctx, r)
	case "sync":
		return s.prepareSync(ctx, r)
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
	_, e := s.update(ctx, r.ID, func(current *model.Record) error {
		if current.StopRequested || current.Revision != r.Revision {
			return nil
		}
		if prepareErr == nil {
			var op model.Operation
			op, prepareErr = s.prepareOperation(*current, id, directory)
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
	s.dispatch.Lock()
	defer s.dispatch.Unlock()
	current, err := s.Store.Refactoring(ctx, r.Project, r.ID)
	if err != nil {
		return err
	}
	if n := len(current.Operations); n == 0 || current.Operations[n-1].ID != o.ID || current.Operations[n-1].State != "pending" {
		return nil
	}
	r = current
	if r.StopRequested {
		_, e := s.update(ctx, r.ID, func(r *model.Record) error {
			last := &r.Operations[len(r.Operations)-1]
			if last.ID == o.ID && last.State == "pending" {
				last.State = "cancelled"
			}
			return nil
		})
		return e
	}
	if o.Content(r).WorkflowID == "generate" && s.Queue != nil {
		key := "refactoring:" + o.ID
		s.Queue.UpdateContent(s.Project, key, o.CreatedAt, false, false)
		if !s.Queue.AcquireContent(s.Project, key) {
			return nil
		}
	}
	if o.Investigation != "" {
		if err := s.Investigate(ctx, r, o.ID); err != nil {
			_, saveErr := s.update(ctx, r.ID, func(current *model.Record) error {
				last := &current.Operations[len(current.Operations)-1]
				if last.ID != o.ID || last.State != "pending" || current.StopRequested {
					return nil
				}
				last.State = "rejected"
				last.Feedback = err.Error()
				s.block(current, err.Error())
				return nil
			})
			return saveErr
		}
	}
	var send bool
	_, e := s.update(ctx, r.ID, func(r *model.Record) error {
		last := &r.Operations[len(r.Operations)-1]
		if last.ID == o.ID && last.State == "pending" && !r.StopRequested {
			last.State = "submitted"
			send = true
		}
		return nil
	})
	if e != nil || !send {
		if s.Queue != nil && o.Content(r).WorkflowID == "generate" {
			s.Queue.UpdateContent(s.Project, "refactoring:"+o.ID, o.CreatedAt, false, true)
		}
		return e
	}
	receipt, callErr := s.Runner.Submit(ctx, o.Request)
	_, e = s.update(context.WithoutCancel(ctx), r.ID, func(r *model.Record) error {
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
		last.ObserveRun(run)
		r.Diagnostic = last.InitializationFailure
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
	if (r.StopRequested || o.InitializationFailure != "") && run.StopStatus != "stopped" {
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
		file, readErr := s.Runner.ResultFile(ctx, run.RunID)
		if readErr != nil {
			return readErr
		}
		if file.CollectionStatus != "closed" {
			return nil
		}
		if file.Version == 1 && file.ContentBase64 != nil {
			data, de := base64.StdEncoding.DecodeString(*file.ContentBase64)
			if de != nil || model.Digest(data) != file.SHA256 || len(data) != file.SizeBytes {
				return errors.New("原维护结果文件校验失败")
			}
			var value model.Result
			var fieldType *json.UnmarshalTypeError
			switch err := json.Unmarshal(data, &value); {
			case errors.As(err, &fieldType) && fieldType.Field != "":
				feedback += "\n结果文件字段类型无效：" + fieldType.Field
			case err != nil:
				feedback += "\n结果文件不是有效的维护 JSON 对象"
			case value.ActionID != r.ID:
				feedback += "\n结果文件 action_id 与原行动不匹配"
			case value.OperationID != o.ID:
				feedback += "\n结果文件 operation_id 与本次操作不匹配"
			case value.Outcome != "done" && value.Outcome != "needs_input" && value.Outcome != "blocked":
				feedback += "\n结果文件 outcome 无效"
			default:
				result = &value
			}
		} else {
			feedback += "\n结果文件未有效封存：" + file.Diagnostic
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
		(result.Diagnostic == "human_input_required" || strings.HasPrefix(result.Diagnostic, "invalid_json:"))
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
	for {
		rows, e := s.Store.Refactorings(ctx, s.Project)
		if e != nil {
			return e
		}
		active := false
		for _, r := range rows {
			active = active || r.Active() && r.Phase != "waiting_input"
		}
		if !active {
			return nil
		}
		if e = s.Tick(ctx); e != nil {
			return e
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
