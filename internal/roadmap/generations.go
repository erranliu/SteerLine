package roadmap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func queueID(r model.Record) string {
	if r.GenerationID != "" {
		return "roadmap:" + r.GenerationID
	}
	return "roadmap:" + r.ID
}

// SaveExecution is used by startup reconciliation before the observer runs.
func (s *Service) SaveExecution(ctx context.Context, r model.Record) (model.Record, error) {
	return s.save(ctx, r)
}

// Only this merge writes a single execution back into its owning record.
// Reading the owner again preserves concurrent stop and review edits.
func (s *Service) saveGeneration(ctx context.Context, view model.Record) (model.Record, error) {
	r, err := s.Store.Roadmap(ctx, s.Project, view.ID)
	if err != nil {
		return view, err
	}
	at := -1
	for i, g := range r.Generations {
		if g.ID == view.GenerationID {
			at = i
			break
		}
	}
	if at < 0 {
		return view, errors.New("原生成执行不存在")
	}
	g := r.Generations[at]
	// A stop saved after queue admission still fences an unsent execution.
	// Do not turn it into an unknown original request that was never sent.
	if r.Cancelled && g.Submission == "pending" && view.Submission != "pending" {
		s.track(r)
		return r.Execution(g), nil
	}
	if g.RunID != "" && view.RunID != g.RunID || g.InputDigest != "" && view.InputDigest != g.InputDigest {
		return view, errors.New("原生成执行绑定变化")
	}
	if g.Projection != nil && view.Projection != nil && view.Projection.Revision < g.Projection.Revision {
		return view, errors.New("stale_revision: 原执行已更新")
	}
	g.State, g.Diagnostic, g.Submission = view.State, view.Diagnostic, view.Submission
	g.RunID, g.InputDigest, g.Projection = view.RunID, view.InputDigest, view.Projection
	g.Result, g.StopRequest, g.StopState = view.Result, view.StopRequest, view.StopState
	g.Questions, g.Investigation = view.Questions, view.Investigation
	if len(view.Documents) == 1 {
		d := view.Documents[0]
		g.Document = &d
	}
	r.Generations = append([]model.Generation(nil), r.Generations...)
	r.Generations[at] = g
	if !r.DocumentsVerified {
		s.aggregate(&r)
	}
	r, err = s.save(ctx, r)
	if err != nil {
		return view, err
	}
	return r.Execution(g), nil
}

func (s *Service) aggregate(r *model.Record) {
	if r.Cancelled {
		r.State = "cancelled"
		return
	}
	current := r.CurrentGenerations()
	docs := []model.Documents{}
	refs := []model.Investigation{}
	questions := []string{}
	failed := []string{}
	failureState := "failed"
	unknown := false
	complete := len(current) == len(model.PlannedDocuments(*r))
	for _, g := range current {
		v := r.Execution(g)
		if g.State == "unknown" {
			unknown = true
		}
		if !v.ObservationComplete() {
			complete = false
		}
		if len(g.Questions) > 0 {
			questions = append(questions, g.Questions...)
		}
		if g.Document != nil && g.State == "documents-review" {
			docs = append(docs, *g.Document)
			refs = append(refs, g.Investigation...)
		} else if v.ObservationComplete() && len(g.Questions) == 0 {
			failed = append(failed, g.Key+": "+g.Diagnostic)
			if g.State == "invalid" {
				failureState = "invalid"
			}
		}
	}
	r.Questions = questions
	if !complete {
		r.State = "accepted"
		if unknown {
			r.State = "unknown"
		}
		r.Diagnostic = strings.Join(failed, "；")
		return
	}
	if len(questions) > 0 {
		r.State = "documents-blocked"
		return
	}
	if len(failed) > 0 {
		r.State = failureState
		r.Diagnostic = strings.Join(failed, "；")
		return
	}
	uniqueRefs := []model.Investigation{}
	seenRefs := map[string]bool{}
	for _, ref := range refs {
		key := model.ValueDigest(ref)
		if !seenRefs[key] {
			seenRefs[key] = true
			uniqueRefs = append(uniqueRefs, ref)
		}
	}
	r.Documents, r.Investigation, r.Design = docs, uniqueRefs, r.Analysis.Coordination
	r.DocumentsVerified = true
	catalogue, err := TestCatalogue(*r)
	if err == nil {
		err = ValidateTestIDs(*r, catalogue)
	}
	if err == nil {
		err = model.ValidateDocuments(*r)
	}
	if err != nil {
		r.DocumentsVerified = false
		r.State = "invalid"
		r.Diagnostic = err.Error()
		return
	}
	r.State, r.Diagnostic = "documents-review", ""
}

func (s *Service) driveDocuments(ctx context.Context, r model.Record) (model.Record, error) {
	for _, v := range r.Executions() {
		if v.ObservationComplete() {
			continue
		}
		var err error
		if v.Submission == "pending" {
			_, err = s.dispatch(ctx, v)
		} else {
			_, err = s.observe(ctx, v)
		}
		if err != nil {
			latest, readErr := s.Store.Roadmap(ctx, s.Project, r.ID)
			if readErr == nil {
				return latest, err
			}
			return r, err
		}
		r, err = s.Store.Roadmap(ctx, s.Project, r.ID)
		if err != nil {
			return r, err
		}
	}
	return r, nil
}

// RetryDocuments is an explicit new execution, never a replay of an unknown send.
func (s *Service) RetryDocuments(ctx context.Context, id, key, request string, expected uint64) (model.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.Store.Roadmap(ctx, s.Project, id)
	if err != nil {
		return r, err
	}
	for _, g := range r.Generations {
		if g.RetryRequest == request {
			if g.Key != key {
				return r, errors.New("request_conflict")
			}
			return r, nil
		}
	}
	if !model.IDPattern.MatchString(request) || r.Revision != expected || r.Purpose != "documents" || r.Cancelled || r.Publication != nil || r.DocumentsVerified || len(r.Questions) > 0 {
		return r, errors.New("当前文档批次不能重新生成")
	}
	rows, err := s.Store.Roadmaps(ctx, s.Project)
	if err != nil {
		return r, err
	}
	for _, other := range rows {
		if other.ID == id || other.Source.ID != r.Source.ID {
			continue
		}
		newer := other.CreatedAt.After(r.CreatedAt) || other.CreatedAt.Equal(r.CreatedAt) && other.ID > r.ID
		if !other.ObservationComplete() || other.Publication != nil && other.State != "admitted" || other.Purpose == "documents" && newer {
			return r, errors.New("同来源存在其它活动、发布或更新文档批次")
		}
	}
	var previous *model.Generation
	for _, g := range r.CurrentGenerations() {
		if g.Key == key {
			copy := g
			previous = &copy
		}
	}
	if previous == nil {
		return r, errors.New("未知任务代号")
	}
	v := r.Execution(*previous)
	if !v.ObservationComplete() || len(v.Questions) > 0 || v.State != "failed" && v.State != "invalid" && v.State != "rejected" {
		return r, errors.New("只能重新生成已结算的明确失败任务")
	}
	if s.Admit != nil {
		if err = s.Admit(ctx); err != nil {
			return r, err
		}
	}
	execution, catalog, err := s.Prepare(ctx)
	if err != nil {
		return r, err
	}
	var input runnercontract.GenerateRunInput
	if err = json.Unmarshal(previous.Request.Input, &input); err != nil {
		return r, err
	}
	input.Execution = execution
	if err = catalog.ValidateRepositoryGenerate(input); err != nil {
		return r, err
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return r, err
	}
	g := model.Generation{ID: Token(), Key: key, CreatedAt: time.Now().UTC(), RetryRequest: request, State: "queued", Submission: "pending",
		Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: Token(), Input: payload}}
	r.Generations = append(append([]model.Generation(nil), r.Generations...), g)
	r.State, r.Diagnostic = "queued", ""
	r, err = s.save(ctx, r)
	if err != nil {
		return r, err
	}
	return r, nil
}
