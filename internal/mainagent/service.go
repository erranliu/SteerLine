package mainagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/mainagent/model"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type Store interface {
	Analysis(context.Context, string, string) (model.Record, error)
	Analyses(context.Context, string) ([]model.Record, error)
	SaveAnalysis(context.Context, model.Record, uint64) error
}
type Runner interface {
	Submit(context.Context, runnercontract.Request) (runnerclient.Receipt, error)
	Inspect(context.Context, string, string) (runnerclient.Inspection, error)
	Call(context.Context, string, []string, *runnercontract.Request) (runnerclient.Response, error)
}
type Queue interface {
	UpdateContent(string, string, time.Time, bool, bool)
	AcquireContent(string, string) bool
}
type Service struct {
	Project, Repository string
	Store               Store
	Runner              Runner
	Build               runnercontract.BuildPin
	Queue               Queue
	Context             func(context.Context, model.Start) (model.Context, error)
	Prepare             func(context.Context) (runnercontract.ExecutionInput, *runnerclient.Catalog, error)
	Read                func(context.Context, string, string) ([]byte, error)
	Admit               func(context.Context) error
	mu                  sync.Mutex
	next                int
}

func Binding(r model.Record) runnerbinding.Content {
	return runnerbinding.Content{StateDir: r.Runner.StateDir, Request: r.Request, RunID: r.RunID, InputDigest: r.InputDigest, Project: r.Project, ObjectKey: "steerline_material_analysis", ObjectID: r.ID, WorkflowID: "generate", WorkflowVersion: 1, RequireDigest: true, Unsubmitted: r.Submission == "pending" || r.Submission == "cancelled", Rejected: r.Submission == "rejected"}
}
func (s *Service) track(r model.Record) {
	if s.Queue != nil {
		s.Queue.UpdateContent(s.Project, "main-agent:"+r.ID, r.CreatedAt, r.Submission != "pending", r.Settled())
	}
}
func (s *Service) Save(ctx context.Context, r model.Record) (model.Record, error) {
	expected := r.Revision
	r.Revision++
	err := s.Store.SaveAnalysis(ctx, r, expected)
	if err == nil {
		s.track(r)
	}
	return r, err
}
func (s *Service) Start(ctx context.Context, id string, in model.Start) (model.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !model.ID.MatchString(id) || !model.ID.MatchString(in.CandidateID) || in.CandidateRevision == 0 || in.Kind != "clarification" && in.Kind != "conflict" || strings.TrimSpace(in.Note) == "" || len(in.Note) > 16000 || !utf8.ValidString(in.Note) {
		return model.Record{}, errors.New("analysis requires candidate/version, material problem kind and nonempty report")
	}
	if old, err := s.Store.Analysis(ctx, s.Project, id); err == nil {
		if model.Hash(old.Input) != model.Hash(in) {
			return old, errors.New("request_conflict: analysis input differs")
		}
		return old, nil
	} else if !errors.Is(err, model.ErrNotFound) {
		return old, err
	}
	if s.Admit != nil {
		if err := s.Admit(ctx); err != nil {
			return model.Record{}, err
		}
	}
	c, err := s.Context(ctx, in)
	if err != nil {
		return model.Record{}, err
	}
	if in.ParentID != "" {
		parent, e := s.Store.Analysis(ctx, s.Project, in.ParentID)
		if e != nil {
			return model.Record{}, e
		}
		if parent.Context.CandidateID != in.CandidateID || parent.State != "review" {
			return model.Record{}, errors.New("follow-up requires reviewed analysis of original candidate")
		}
		c.Prior, _ = json.Marshal(struct {
			Input   model.Start   `json:"input"`
			Context model.Context `json:"context"`
			Result  *model.Result `json:"result"`
		}{parent.Input, parent.Context, parent.Result})
	}
	execution, cat, err := s.Prepare(ctx)
	if err != nil {
		return model.Record{}, err
	}
	rules, err := s.Read(ctx, c.RepositoryCommit, model.RulesPath)
	if err != nil {
		return model.Record{}, errors.New("主 Agent 规则未提交到当前基线，请先显式准备规则并合入")
	}
	expected, err := prompts.MainAgentRules()
	if err != nil || !prompts.SameRules(rules, expected) {
		return model.Record{}, errors.New("主 Agent 固定规则版本不符，请审阅更新并合入")
	}
	body, err := json.Marshal(struct {
		Context  model.Context `json:"context"`
		Kind     string        `json:"kind"`
		FollowUp string        `json:"follow-up"`
	}{c, in.Kind, in.Note})
	if err != nil || len(body) > model.MaxBytes {
		return model.Record{}, errors.New("analysis context exceeds file capacity")
	}
	text := string(body)
	instruction, err := prompts.MainAgent()
	if err != nil {
		return model.Record{}, err
	}
	paths := []string{model.RulesPath}
	for _, doc := range c.Current {
		paths = append(paths, doc.Path)
	}
	input := runnercontract.GenerateRunInput{Title: "材料问题: " + c.Title, Correlation: map[string]string{"steerline_project": s.Project, "steerline_material_analysis": id}, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Execution: execution, Generate: runnercontract.GenerateInput{Instruction: instruction, Repository: &runnercontract.GenerateRepository{LocalPath: s.Repository, Commit: c.RepositoryCommit, Paths: paths}, Materials: []runnercontract.Document{{Kind: "file", Path: model.ContextPath, Content: &text}}, OutputSchema: ResultSchema()}}
	if err = cat.ValidateRepositoryGenerate(input); err != nil {
		return model.Record{}, err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return model.Record{}, err
	}
	r := model.Record{Project: s.Project, ID: id, CreatedAt: time.Now().UTC(), Input: in, Context: c, Runner: s.Build, Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: model.Token(), Input: raw}, State: "queued", Submission: "pending"}
	r, err = s.Save(ctx, r)
	if err != nil {
		return r, err
	}
	return s.dispatch(ctx, r)
}
func (s *Service) dispatch(ctx context.Context, r model.Record) (model.Record, error) {
	if r.Submission != "pending" || r.Cancelled {
		return r, nil
	}
	if s.Admit != nil {
		if err := s.Admit(ctx); err != nil {
			return r, err
		}
	}
	if s.Queue != nil && !s.Queue.AcquireContent(s.Project, "main-agent:"+r.ID) {
		return r, nil
	}
	r.Submission, r.State = "started", "unknown"
	r, err := s.Save(ctx, r)
	if err != nil {
		return r, err
	}
	receipt, callErr := s.Runner.Submit(ctx, r.Request)
	if callErr == nil && receipt.ReceiptStatus == "accepted" && receipt.RunID != "" && receipt.CommandID != "" {
		r.RunID, r.State = receipt.RunID, "accepted"
	} else {
		if callErr == nil {
			callErr = errors.New("analysis submit receipt unverified")
		}
		r.Diagnostic = callErr.Error()
		var f *runnerclient.Failure
		if errors.As(callErr, &f) && !f.Unknown {
			r.State, r.Submission = "rejected", "rejected"
		}
	}
	return s.Save(context.WithoutCancel(ctx), r)
}
func (s *Service) ObserveNext(ctx context.Context) error {
	// Auxiliary observation runs on the project actor. A slow user request's
	// material fetch or native configuration must not consume its stop/delivery budget.
	if !s.mu.TryLock() {
		return nil
	}
	defer s.mu.Unlock()
	rows, err := s.Store.Analyses(ctx, s.Project)
	if err != nil {
		return err
	}
	pending := []model.Record{}
	for _, r := range rows {
		s.track(r)
		if !r.Complete() {
			pending = append(pending, r)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	i := s.next % len(pending)
	s.next = (i + 1) % len(pending)
	if pending[i].Submission == "pending" {
		_, err = s.dispatch(ctx, pending[i])
	} else {
		_, err = s.observe(ctx, pending[i])
	}
	return err
}
func (s *Service) Observe(ctx context.Context, id string) (model.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.Store.Analysis(ctx, s.Project, id)
	if err != nil || r.Complete() {
		return r, err
	}
	if r.Submission == "pending" {
		return s.dispatch(ctx, r)
	}
	return s.observe(ctx, r)
}
func (s *Service) observe(ctx context.Context, r model.Record) (model.Record, error) {
	if !runnerbinding.SamePath(r.Runner.StateDir, s.Build.StateDir) {
		return r, errors.New("original analysis Runner namespace differs")
	}
	binding := Binding(r)
	selector, id := binding.Selector()
	view, err := s.Runner.Inspect(ctx, selector, id)
	if err != nil {
		return r, err
	}
	if err = runnerbinding.ValidateContent(binding, view); err != nil {
		return r, err
	}
	run := view.Run
	r.RunID, r.InputDigest, r.Projection = run.RunID, run.InputDigest, run
	r.Diagnostic = run.Diagnostic
	if r.Cancelled {
		r.State = "cancelled"
		if runnerbinding.NeedsContentStop(r.StopRequest, r.StopState, run.StopStatus) {
			r.StopRequest = runnerbinding.ContentStop(model.Token(), run.Revision)
			r.StopState = "unknown"
			r, err = s.Save(ctx, r)
			if err != nil {
				return r, err
			}
			out, e := s.Runner.Call(ctx, "stop", []string{"--run-id", run.RunID}, r.StopRequest)
			var receipt runnerclient.Receipt
			if e == nil && json.Unmarshal(out.Data, &receipt) == nil && receipt.ReceiptStatus == "accepted" && receipt.RunID == run.RunID {
				r.StopState = "accepted"
			} else {
				var f *runnerclient.Failure
				if errors.As(e, &f) && !f.Unknown && f.Code == "stale_revision" {
					r.StopState = "rejected"
				}
				r.Diagnostic = "原分析停止结果待核实"
			}
		}
		return s.Save(ctx, r)
	}
	if r.Result == nil && run.ContentResult != nil && run.StopStatus == "stopped" {
		result := run.ContentResult
		if result.Source.AttemptID == "" || result.Source.SessionID == "" || result.Source.AttemptID != run.Execution.AttemptID || result.Source.SessionID != run.Execution.SessionID || result.Source.TurnID != run.Execution.TurnID || run.Final == nil || !run.Final.Verified || result.Source.CommandID != run.Final.CommandID || result.Source.AttemptID != run.Final.AttemptID || result.Source.SessionID != run.Final.SessionID || result.Source.TurnID != run.Final.TurnID {
			r.State, r.Diagnostic = "failed", "analysis final source differs from original execution"
			return s.Save(ctx, r)
		}
		if result.Status != "succeeded" || run.Status != "completed" || !result.Source.Verified || result.Source.NativeOutcome != "succeeded" || result.Source.CommandID == "" || result.Source.TurnID == "" {
			r.State, r.Diagnostic = "failed", "analysis has no verified successful result"
			return s.Save(ctx, r)
		}
		var frozen runnercontract.GenerateRunInput
		if err = json.Unmarshal(r.Request.Input, &frozen); err != nil {
			return r, err
		}
		if err = runnerclient.VerifyFileMaterials(frozen.Generate.Materials, run.Materials); err == nil {
			err = runnerclient.VerifyFileMaterials(frozen.Generate.Materials, result.FileMaterials)
		}
		if err == nil && model.Hash(run.Materials) != model.Hash(result.FileMaterials) {
			err = errors.New("analysis Run/result file manifests differ")
		}
		rules, e := s.Read(ctx, r.Context.RepositoryCommit, model.RulesPath)
		if err == nil {
			err = e
		}
		if err == nil {
			err = runnerclient.VerifyRepositoryFiles(result.RepositorySource, *frozen.Generate.Repository, expectedFiles(r, rules))
		}
		if err == nil && model.Hash(run.RepositorySource) != model.Hash(result.RepositorySource) {
			err = errors.New("analysis repository sources differ")
		}
		var decoded model.Result
		if err == nil {
			if len(result.JSON) > model.MaxBytes || !utf8.Valid(result.JSON) {
				err = errors.New("analysis output exceeds capacity")
			} else {
				canonical, e := runnercontract.CanonicalJSON(result.JSON)
				err = e
				if err == nil {
					d := json.NewDecoder(bytes.NewReader(canonical))
					d.DisallowUnknownFields()
					err = d.Decode(&decoded)
				}
			}
		}
		if err == nil {
			err = model.Validate(r, decoded)
		}
		if err != nil {
			r.State, r.Diagnostic = "failed", err.Error()
		} else {
			r.Result, r.State = &decoded, "review"
		}
	} else if r.Result == nil && (run.Status == "failed" || run.Status == "stopped" || run.Status == "completed" && run.StopStatus == "stopped" && run.ContentResult == nil) {
		r.State = "failed"
		r.Diagnostic = "analysis ended without a verified result"
	}
	return s.Save(ctx, r)
}
func (s *Service) Stop(ctx context.Context, id string) (model.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.Store.Analysis(ctx, s.Project, id)
	if err != nil || r.Complete() {
		return r, err
	}
	r.Cancelled, r.State = true, "cancelled"
	if r.Submission == "pending" {
		r.Submission = "cancelled"
	}
	return s.Save(ctx, r)
}
func (s *Service) Close(ctx context.Context) error {
	rows, err := s.Store.Analyses(ctx, s.Project)
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range rows {
		if r.Complete() {
			continue
		}
		r, e := s.Stop(ctx, r.ID)
		for e == nil && !r.Settled() && ctx.Err() == nil {
			r, e = s.Observe(ctx, r.ID)
			if e == nil && !r.Settled() {
				select {
				case <-ctx.Done():
				case <-time.After(50 * time.Millisecond):
				}
			}
		}
		if e != nil {
			errs = append(errs, e)
		} else if !r.Settled() {
			errs = append(errs, errors.New("原分析资源停止尚未核实"))
		}
	}
	return errors.Join(errs...)
}

func expectedFiles(r model.Record, rules []byte) map[string]string {
	out := map[string]string{model.RulesPath: model.Digest(rules)}
	for _, doc := range r.Context.Current {
		out[doc.Path] = doc.SHA256
	}
	return out
}
