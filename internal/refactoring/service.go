package refactoring

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/prompts"
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
	Project  string
	Store    Store
	Runner   Runner
	Prepare  func(context.Context, model.Start) (model.Frozen, error)
	Reserve  func(context.Context, string, func() error) error
	Admit    func(context.Context) error
	Verify   func(context.Context, model.Record, model.Result) (*runnercontract.PRFact, error)
	ReadCost func(context.Context, model.Record, model.Operation) (model.Cost, error)
	Queue    interface {
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
func (s *Service) Command(ctx context.Context, c model.Command, in *model.Start) (model.Record, error) {
	if !model.ValidID(c.ID) || !json.Valid(c.Raw) {
		return model.Record{}, errors.New("无效维护请求")
	}
	raw, result, e := s.Store.RefactoringCommand(ctx, s.Project, c.ID)
	if e == nil {
		if string(raw) != string(c.Raw) {
			return model.Record{}, errors.New("request_conflict: 原维护命令不同")
		}
		var r model.Record
		e = json.Unmarshal(result, &r)
		r.Project = s.Project
		return r, e
	}
	if !errors.Is(e, model.ErrNotFound) {
		return model.Record{}, e
	}
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
		r := model.Record{ID: c.ID, Project: s.Project, Input: *in, Frozen: f, Phase: "select", CreatedAt: now, ActiveSince: &now, Operations: []model.Operation{}, Answers: []model.Answer{}, Reports: []model.Report{}}
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
		if r.Phase == "waiting_input" {
			r.Phase = "finish"
			r.Question = nil
			now := s.now()
			r.ActiveSince = &now
		}
		if !r.Active() {
			r.StopRequested = false
			now := s.now()
			r.ActiveSince = &now
			r.Phase = "finish"
		}
		r.FinishRequested = true
		r.FinishedAt = nil
	case "refactoring_answer":
		if r.Question == nil || r.Question.ID != c.QuestionID || strings.TrimSpace(c.Answer) == "" || len(c.Answer) > 32<<10 {
			return r, errors.New("原问题已变化或答复无效")
		}
		r.Answers = append(r.Answers, model.Answer{QuestionID: c.QuestionID, Text: c.Answer})
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
		if _, e := s.Verify(ctx, r, model.Result{PR: r.PR.URL, Head: r.PR.HeadRevision}); e != nil {
			return r, e
		}
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
	for _, r := range rows {
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
		return nil
	}
	return nil
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
	if r.Phase == "verifying" {
		return s.settleCandidate(ctx, r.ID)
	}
	if r.Phase == "waiting_input" || r.Phase == "blocked" || r.Phase == "delivered" || r.Phase == "stopped" {
		return nil
	}
	if r.Phase == "select" && (r.FinishRequested || r.BudgetExhausted(s.now())) {
		_, e := s.update(ctx, r.ID, func(r *model.Record) error { r.FinishRequested = true; r.Phase = "finish"; return nil })
		return e
	}
	if s.Admit != nil {
		if e := s.Admit(ctx); e != nil {
			return e
		}
	}
	_, e := s.update(ctx, r.ID, func(r *model.Record) error {
		if r.StopRequested {
			return nil
		}
		op, e := s.prepareOperation(*r)
		if e != nil {
			r.Phase = "blocked"
			r.Diagnostic = e.Error()
			r.Pause(s.now())
			return nil
		}
		r.Operations = append(r.Operations, op)
		r.PendingPrompt = ""
		return nil
	})
	return e
}

type facts struct {
	CostComplete     bool                         `json:"cost_complete"`
	CostIssues       []string                     `json:"cost_issues"`
	Diagnostic       string                       `json:"diagnostic"`
	Publication      map[string]string            `json:"publication"`
	ActionID         string                       `json:"action_id"`
	Mode             string                       `json:"mode"`
	Direction        string                       `json:"direction"`
	Strategy         string                       `json:"strategy"`
	Requirements     string                       `json:"requirements"`
	Round            int                          `json:"round"`
	Budget           model.Budget                 `json:"budget"`
	Finish           bool                         `json:"finish"`
	Repository       runnercontract.Repository    `json:"repository"`
	ManagedWorkspace string                       `json:"managed_workspace,omitempty"`
	ManagedBranch    string                       `json:"managed_branch,omitempty"`
	WorkspaceGit     *runnercontract.WorkspaceGit `json:"workspace_git,omitempty"`
	PR               *runnercontract.PRFact       `json:"pr,omitempty"`
	Answers          []model.Answer               `json:"answers"`
	Feedback         []map[string]any             `json:"feedback"`
	ReportCandidate  *model.Result                `json:"report_candidate,omitempty"`
	Amount           *string                      `json:"amount,omitempty"`
	AsOf             time.Time                    `json:"as_of"`
}

func (s *Service) facts(r model.Record) facts {
	v, complete := r.CostTotal()
	f := facts{ActionID: r.ID, Mode: r.Phase, Direction: r.Input.Direction, Strategy: r.Input.Strategy, Requirements: r.Input.Requirements, Round: r.Round, Budget: r.Input.Budget, Finish: r.FinishRequested, Repository: r.Frozen.Repository, PR: r.PR, Answers: r.Answers, ReportCandidate: r.Candidate, Amount: v, AsOf: s.now(), Feedback: []map[string]any{}}
	f.Repository.LocalPath = ""
	f.ManagedWorkspace = r.Workspace
	f.Publication = r.Frozen.Publication
	f.Diagnostic = r.Diagnostic
	f.CostComplete = complete
	f.CostIssues = []string{}
	for _, o := range r.Operations {
		if o.Settled() {
			if o.Run != nil && o.Run.Workflow.ID == "execute" && o.Run.RunID == r.LatestOwner {
				f.ManagedBranch = o.Run.Execution.Branch
				f.WorkspaceGit = o.WorkspaceGit
			}
			if !o.Cost.Complete && o.Cost.Diagnostic != "" {
				f.CostIssues = append(f.CostIssues, o.RunID+": "+o.Cost.Diagnostic)
			}
			item := map[string]any{"purpose": o.Purpose, "round": o.Round, "state": o.State, "text": o.Feedback, "result": o.Result, "execution": o.Run, "cost": o.Cost}
			if o.Purpose == "publish" || o.Purpose == "conflict" {
				item["report_reference_cost"] = o.ReportCost
				item["report_as_of"] = o.ReportAsOf
			}
			f.Feedback = append(f.Feedback, item)
		}
	}
	return f
}
func (s *Service) prepareOperation(r model.Record) (model.Operation, error) {
	o := model.Operation{ID: model.ID(), Purpose: r.Phase, Round: r.Round, State: "pending", CreatedAt: s.now()}
	o.Closing = r.FinishRequested || r.Phase == "finish" || r.WriteKind == "conflict"
	correlation := map[string]string{"steerline_project": r.Project, "steerline_refactoring": r.ID, "refactoring_operation": o.ID}
	var input any
	if r.Phase == "execute" {
		o.Prompt = r.PendingPrompt
		o.Purpose = r.WriteKind
		body, e := prompts.MaintenanceExecute()
		if e != nil {
			return o, e
		}
		contextFacts := s.facts(r)
		if o.Purpose == "publish" || o.Purpose == "conflict" {
			if e := useFinishReportReference(r, &contextFacts); e != nil {
				return o, e
			}
		}
		o.ReportCost = contextFacts.Amount
		o.ReportAsOf = contextFacts.AsOf
		// The steering prompt is the handoff. Old raw Run observations remain in
		// the read-only decision context, not in every writable seed.
		contextFacts.Feedback = nil
		seed := struct {
			ActionID    string `json:"action_id"`
			OperationID string `json:"operation_id"`
			Instruction string `json:"instruction"`
			Rules       string `json:"rules"`
			Context     facts  `json:"context"`
			Completed   bool   `json:"completed"`
		}{r.ID, o.ID, r.PendingPrompt, r.Frozen.Rules, contextFacts, false}
		data, e := json.Marshal(seed)
		if e != nil {
			return o, e
		}
		if len(data) > runnercontract.MaxProgressBytes {
			return o, errors.New("维护操作完整输入超过Runner初始化文件限额")
		}
		input = runnercontract.RunInput{Title: "重构与维护 · " + o.Purpose, NamePrefix: "maintenance", Correlation: correlation, Repository: r.Frozen.Repository, Documents: r.Frozen.Documents, Execution: r.Frozen.Write, Workflow: runnercontract.Workflow{ID: "execute", Version: 1, PromptOverrides: map[string]string{"execute": body}}, ReuseWorktreeFromRunID: r.LatestOwner, ResultFile: &runnercontract.ProgressFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString(data)}}
	} else {
		body, e := decision.Instruction()
		if e != nil {
			return o, e
		}
		data, e := json.Marshal(s.facts(r))
		if e != nil {
			return o, e
		}
		if len(data) > 1<<20 {
			return o, errors.New("完整维护上下文超过限额")
		}
		o.Prompt = body
		rules, contextBody := r.Frozen.Rules, string(data)
		directory := r.Workspace
		if directory == "" {
			if r.LatestOwner != "" {
				return o, errors.New("原执行工作区尚未核实，保留原记录")
			}
			directory = r.Frozen.Repository.LocalPath
		}
		generate := runnercontract.GenerateInput{Instruction: body, Materials: []runnercontract.Document{{Kind: "file", Path: "maintenance/rules.md", Content: &rules}, {Kind: "file", Path: "maintenance/context.json", Content: &contextBody}}, OutputSchema: decision.Schema()}
		if r.Workspace == "" {
			generate.Repository = &runnercontract.GenerateRepository{LocalPath: r.Frozen.Repository.LocalPath, Commit: r.Frozen.Repository.BaseRevision}
		} else {
			generate.Directory = &runnercontract.DirectoryAccess{Path: directory, Access: "read_only"}
		}
		input = runnercontract.GenerateRunInput{Title: "重构与维护 · " + o.Purpose, Correlation: correlation, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Execution: r.Frozen.Read, Generate: generate}
	}
	raw, e := json.Marshal(input)
	o.Request = runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: model.ID(), Input: raw}
	return o, e
}

func useFinishReportReference(r model.Record, destination *facts) error {
	if len(r.Operations) == 0 {
		return errors.New("缺少原收尾决策费用快照")
	}
	previous := r.Operations[len(r.Operations)-1]
	var original runnercontract.GenerateRunInput
	if previous.Purpose != "finish" || previous.State != "complete" || json.Unmarshal(previous.Request.Input, &original) != nil || original.Workflow.ID != "generate" {
		return errors.New("缺少原收尾决策费用快照")
	}
	materials := original.Generate.Materials
	if len(materials) != 2 || materials[1].Kind != "file" || materials[1].Path != "maintenance/context.json" || materials[1].Content == nil {
		return errors.New("原收尾决策费用快照缺少冻结材料")
	}
	var snapshot struct {
		ActionID     string    `json:"action_id"`
		Mode         string    `json:"mode"`
		Amount       *string   `json:"amount"`
		AsOf         time.Time `json:"as_of"`
		CostComplete bool      `json:"cost_complete"`
		CostIssues   []string  `json:"cost_issues"`
	}
	if json.Unmarshal([]byte(*materials[1].Content), &snapshot) != nil || snapshot.ActionID != r.ID || snapshot.Mode != "finish" || snapshot.AsOf.IsZero() {
		return errors.New("原收尾决策费用快照无效")
	}
	destination.Amount, destination.AsOf = snapshot.Amount, snapshot.AsOf
	destination.CostComplete, destination.CostIssues = snapshot.CostComplete, snapshot.CostIssues
	return nil
}

func (s *Service) submit(ctx context.Context, r model.Record, o model.Operation) error {
	s.dispatch.Lock()
	defer s.dispatch.Unlock()
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
		last.RunID = run.RunID
		last.InputDigest = run.InputDigest
		last.Run = run
		r.Diagnostic = ""
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
	decisionSucceeded := run.Status == "completed" && run.Final != nil && run.Final.Verified && run.Final.NativeOutcome == "succeeded" &&
		(run.ContentResult == nil || run.ContentResult.Status == "succeeded")
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
		} else if in.Generate.Directory != nil && !runnerbinding.SamePath(run.Execution.Workspace, in.Generate.Directory.Path) {
			return errors.New("只读目录来源不匹配")
		}
		if run.ContentResult != nil {
			if err := runnerclient.VerifyFileMaterials(in.Generate.Materials, run.ContentResult.FileMaterials); err != nil {
				return err
			}
			if in.Generate.Repository != nil {
				if e := runnerclient.VerifyRepositoryFiles(run.ContentResult.RepositorySource, *in.Generate.Repository, nil); e != nil {
					return e
				}
			}
			if !reflect.DeepEqual(run.ContentResult.Source, *run.Final) {
				return errors.New("只读结果与原生Final来源不匹配")
			}
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
			if json.Unmarshal(data, &value) == nil && value.ActionID == r.ID && value.OperationID == o.ID && (value.Outcome == "done" || value.Outcome == "needs_input" || value.Outcome == "blocked") {
				result = &value
			} else {
				feedback += "\n结果文件缺少有效的本次操作结果"
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
		if r.StopRequested {
			return nil
		}
		if binding.WorkflowID == "execute" {
			r.Phase = "audit"
			if (o.Purpose == "publish" || o.Purpose == "conflict") && run.Final != nil && run.Final.Verified && run.Final.NativeOutcome == "succeeded" {
				r.Candidate = result
			}
			return nil
		}
		if !decisionSucceeded {
			detail := run.Diagnostic
			if detail == "" && run.ContentResult != nil {
				detail = run.ContentResult.Diagnostic
			}
			if detail == "" {
				detail = run.Status
			}
			if retryableDecisionOutput(run.ContentResult) {
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
		var raw []byte
		if run.ContentResult != nil && len(run.ContentResult.JSON) > 0 {
			raw = run.ContentResult.JSON
		} else {
			raw = []byte(feedback)
		}
		d, de := decision.Accept(raw, o.Purpose)
		if de != nil {
			r.Phase = "blocked"
			r.Diagnostic = de.Error()
			r.Pause(now)
			return nil
		}
		return s.applyDecision(r, o.Purpose, d)
	})
	if e != nil {
		return e
	}
	if e = s.verifyPublished(ctx, r.ID); e != nil {
		return e
	}
	return s.settleCandidate(ctx, r.ID)
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

func (s *Service) verifyPublished(ctx context.Context, id string) error {
	r, e := s.Store.Refactoring(ctx, s.Project, id)
	if e != nil || r.Candidate == nil || r.Candidate.PR == "" || r.StopRequested {
		return e
	}
	if s.Verify == nil {
		return errors.New("缺少只读PR核验")
	}
	pr, e := s.Verify(ctx, r, *r.Candidate)
	if e != nil {
		if pr != nil {
			_, saveErr := s.update(ctx, id, func(r *model.Record) error { r.PR = pr; r.Diagnostic = e.Error(); return nil })
			if saveErr != nil {
				return saveErr
			}
		}
		return e
	}
	_, e = s.update(ctx, id, func(r *model.Record) error { r.PR = pr; return nil })
	return e
}

func (s *Service) applyDecision(r *model.Record, mode string, d decision.Decision) error {
	r.Summary = d.Summary
	_, complete := r.CostTotal()
	if mode == "select" && (r.FinishRequested || r.BudgetExhausted(s.now()) || r.Input.Budget.Dollars != "" && !complete) {
		r.FinishRequested = true
		r.Phase = "finish"
		return nil
	}
	switch d.Kind {
	case "ask":
		r.Question = &model.Question{ID: model.ID(), Text: d.Question, Mode: mode}
		r.Phase = "waiting_input"
		r.Pause(s.now())
	case "blocked":
		r.Phase = "blocked"
		r.Diagnostic = d.Summary
		r.Pause(s.now())
	case "finish":
		r.FinishRequested = true
		r.Phase = "finish"
	case "accept":
		if r.Candidate != nil {
			r.Phase = "verifying"
		} else if r.FinishRequested || r.BudgetExhausted(s.now()) {
			r.FinishRequested = true
			r.Phase = "finish"
		} else {
			r.Phase = "select"
		}
	case "execute":
		if mode == "select" {
			_, complete := r.CostTotal()
			if r.FinishRequested || r.BudgetExhausted(s.now()) || r.Input.Budget.Dollars != "" && !complete {
				r.FinishRequested = true
				r.Phase = "finish"
				return nil
			}
			r.Round++
			r.WriteKind = "implement"
		} else if mode == "finish" {
			if r.WriteKind != "conflict" {
				r.WriteKind = "publish"
			}
		} else {
			r.WriteKind = "repair"
			r.Candidate = nil
		}
		r.PendingPrompt = d.Prompt
		r.Phase = "execute"
	}
	return nil
}
func (s *Service) settleCandidate(ctx context.Context, id string) error {
	r, e := s.Store.Refactoring(ctx, s.Project, id)
	if e != nil || r.Phase != "verifying" || r.Candidate == nil {
		return e
	}
	value := *r.Candidate
	diagnostic := ""
	if r.PR != nil && value.PR != r.PR.URL {
		diagnostic = "已有PR的行动必须继续交付原PR，不能以本轮无改动替代"
	} else if value.Report == "" || value.Outcome != "done" {
		diagnostic = "收尾报告缺失或验证尚未完成"
	} else if !model.ValidRevision(value.Head) || !model.ValidRevision(value.Base) {
		diagnostic = "报告head/base必须是对应完整提交SHA"
	} else if value.PR == "" && !value.NoChanges {
		diagnostic = "存在改动但缺少PR来源"
	}
	if diagnostic != "" {
		_, e = s.update(ctx, id, func(r *model.Record) error {
			r.Phase = "blocked"
			r.Diagnostic = diagnostic
			r.Pause(s.now())
			return nil
		})
		return e
	}
	var pr *runnercontract.PRFact
	if value.PR != "" {
		if s.Verify == nil {
			return errors.New("缺少只读PR核验")
		}
		pr, e = s.Verify(ctx, r, value)
		if e != nil {
			return e
		}
	} else if !value.NoChanges {
		return errors.New("存在改动但缺少PR来源")
	}
	_, e = s.update(ctx, id, func(r *model.Record) error {
		if r.StopRequested {
			return nil
		}
		var amount *string
		var asOf time.Time
		for i := len(r.Operations) - 1; i >= 0; i-- {
			if r.Operations[i].Purpose == "publish" || r.Operations[i].Purpose == "conflict" {
				amount = r.Operations[i].ReportCost
				asOf = r.Operations[i].ReportAsOf
				break
			}
		}
		r.PR = pr
		r.Reports = append(r.Reports, model.Report{Body: value.Report, Head: value.Head, Base: value.Base, PR: value.PR, CreatedAt: s.now(), AsOf: asOf, Cost: amount})
		r.Candidate = nil
		r.Phase = "delivered"
		now := s.now()
		r.FinishedAt = &now
		r.Pause(now)
		return nil
	})
	return e
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
		if !r.Active() {
			continue
		}
		_, e = s.update(ctx, r.ID, func(r *model.Record) error {
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
			active = active || r.Active()
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
