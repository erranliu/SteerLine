package roadmap

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type Store interface {
	Roadmap(context.Context, string, string) (model.Record, error)
	Roadmaps(context.Context, string) ([]model.Record, error)
	SaveRoadmap(context.Context, model.Record, uint64) error
	RoadmapRequest(context.Context, string, string) (model.Record, error)
	RoadmapMapping(context.Context, string, string) (map[string]string, error)
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
	Source              func(context.Context, string) (model.Source, error)
	Graph               func(context.Context) ([]model.Existing, error)
	Prepare             func(context.Context) (runnercontract.ExecutionInput, *runnerclient.Catalog, error)
	PrepareDocuments    func(context.Context, model.Record) (model.DocumentContext, error)
	ReadCommitted       func(context.Context, string, string) ([]byte, error)
	Admit               func(context.Context) error
	mu                  sync.Mutex
	next                int
}
type ParseInput struct {
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
	Digest    string `json:"source_digest"`
}

func Token() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func (s *Service) save(ctx context.Context, r model.Record) (model.Record, error) {
	if r.GenerationID != "" {
		return s.saveGeneration(ctx, r)
	}
	old := r.Revision
	r.Revision++
	e := s.Store.SaveRoadmap(ctx, r, old)
	if e == nil {
		s.track(r)
	}
	return r, e
}
func (s *Service) track(r model.Record) {
	if s.Queue != nil {
		for _, v := range r.Executions() {
			s.Queue.UpdateContent(s.Project, queueID(v), v.CreatedAt, v.Submission != "pending", v.Settled())
		}
	}
}
func (s *Service) Parse(ctx context.Context, in ParseInput) (model.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !model.IDPattern.MatchString(in.RequestID) || !model.SourcePath(in.Path) || len(in.Digest) != 64 {
		return model.Record{}, errors.New("解析需要原请求号及 Markdown 路径和摘要")
	}
	if old, e := s.Store.Roadmap(ctx, s.Project, in.RequestID); e == nil {
		if old.Source.Path != in.Path || old.Source.Digest != in.Digest {
			return old, errors.New("request_conflict: original parse input differs")
		}
		return old, nil
	} else if !errors.Is(e, model.ErrNotFound) {
		return model.Record{}, e
	}
	if s.Admit != nil {
		if e := s.Admit(ctx); e != nil {
			return model.Record{}, e
		}
	}
	source, e := s.Source(ctx, in.Path)
	if e != nil {
		return model.Record{}, e
	}
	if source.Digest != in.Digest {
		return model.Record{}, errors.New("Markdown 已变化，请刷新后再解析")
	}
	previous, e := s.Store.Roadmaps(ctx, s.Project)
	if e != nil {
		return model.Record{}, e
	}
	for _, old := range previous {
		s.track(old)
		if old.Source.ID == source.ID && !old.ObservationComplete() {
			return old, errors.New("原解析仍在运行或资源未核实，请查询/停止原请求")
		}
	}
	graph, e := s.Graph(ctx)
	if e != nil {
		return model.Record{}, e
	}
	body, e := json.Marshal(graph)
	if e != nil {
		return model.Record{}, e
	}
	mapping, e := s.Store.RoadmapMapping(ctx, s.Project, source.ID)
	if e != nil {
		return model.Record{}, e
	}
	mappingBody, _ := json.Marshal(mapping)
	if len(body)+len(source.Body)+len(mappingBody) > model.MaxBytes {
		return model.Record{}, errors.New("Markdown 和已有任务材料超过生成上限，不能截断解析")
	}
	execution, catalog, e := s.Prepare(ctx)
	if e != nil {
		return model.Record{}, e
	}
	if reason := catalog.FileMaterialGenerationUnavailable(); reason != "" {
		return model.Record{}, errors.New(reason)
	}
	inputs := []runnercontract.Document{fileMaterial(SourceInputPath, []byte(source.Body)), fileMaterial(GraphPath, body), fileMaterial(MappingPath, mappingBody)}
	rules, e := prompts.RoadmapParseRules()
	if e != nil {
		return model.Record{}, e
	}
	file, e := s.fixedFile(ctx, source.Head, ParseRulesPath, rules, true)
	if e != nil {
		return model.Record{}, e
	}
	materials := []runnercontract.Document{file}
	planning, err := s.planningMaterials(ctx, source.Head)
	if err != nil {
		return model.Record{}, err
	}
	materials = append(materials, planning...)
	instruction, e := prompts.RoadmapParse(source.Path, source.Head)
	if e != nil {
		return model.Record{}, e
	}
	paths := []string{}
	for _, material := range materials {
		paths = append(paths, material.Path)
	}
	input := runnercontract.GenerateRunInput{Title: "Roadmap 任务及依赖解析: " + source.Path, Correlation: map[string]string{"steerline_project": s.Project, "steerline_roadmap_parse": in.RequestID}, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Execution: execution, Generate: runnercontract.GenerateInput{
		Instruction: instruction, Repository: &runnercontract.GenerateRepository{LocalPath: s.Repository, Commit: source.Head, Paths: paths}, Materials: inputs, OutputSchema: ParseSchema(),
	}}
	if e = catalog.ValidateRepositoryGenerate(input); e != nil {
		return model.Record{}, e
	}
	payload, e := json.Marshal(input)
	if e != nil {
		return model.Record{}, e
	}
	r := model.Record{ID: in.RequestID, RequestID: in.RequestID, Project: s.Project, Revision: 1, Source: source, Existing: graph, GraphDigest: model.GraphDigest(graph), CreatedAt: time.Now().UTC(), State: "queued", Runner: s.Build, Submission: "pending", CodeCommit: source.Head, Materials: materials, PlanningMaterials: materials, Documents: []model.Documents{}, Mapping: mapping, Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: Token(), Input: payload}}
	if e = s.Store.SaveRoadmap(ctx, r, 0); e != nil {
		return r, e
	}
	s.track(r)
	return s.dispatch(ctx, r)
}
func (s *Service) dispatch(ctx context.Context, r model.Record) (model.Record, error) {
	if r.Submission != "pending" || r.Cancelled {
		return r, nil
	}
	if !runnerbinding.SamePath(r.Runner.StateDir, s.Build.StateDir) {
		return r, errors.New("original Runner namespace differs")
	}
	if s.Admit != nil {
		if e := s.Admit(ctx); e != nil {
			return r, e
		}
	}
	if s.Queue != nil && !s.Queue.AcquireContent(s.Project, queueID(r)) {
		return r, nil
	}
	r.Submission = "started"
	r.State = "unknown"
	var e error
	r, e = s.save(ctx, r)
	if e != nil {
		return r, e
	}
	if r.Cancelled {
		return r, nil
	}
	receipt, callErr := s.Runner.Submit(ctx, r.Request)
	if callErr == nil {
		r.RunID = receipt.RunID
		r.State = "accepted"
	} else {
		r.Diagnostic = callErr.Error()
		var failure *runnerclient.Failure
		if errors.As(callErr, &failure) && !failure.Unknown {
			r.State = "rejected"
		}
	}
	r, e = s.save(context.WithoutCancel(ctx), r)
	if e != nil {
		return r, e
	}
	return r, callErr
}
func (s *Service) ObserveNext(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, e := s.Store.Roadmaps(ctx, s.Project)
	if e != nil {
		return e
	}
	pending := []model.Record{}
	for _, r := range rows {
		s.track(r)
		for _, v := range r.Executions() {
			if !v.ObservationComplete() {
				pending = append(pending, v)
			}
		}
	}
	if len(pending) == 0 {
		return nil
	}
	at := s.next % len(pending)
	s.next = (at + 1) % len(pending)
	r := pending[at]
	if r.Submission == "pending" {
		_, e = s.dispatch(ctx, r)
	} else {
		_, e = s.observe(ctx, r)
	}
	return e
}
func (s *Service) Observe(ctx context.Context, id string) (model.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.Store.Roadmap(ctx, s.Project, id)
	if e != nil {
		return r, e
	}
	if r.ObservationComplete() {
		return r, nil
	}
	if r.Purpose == "documents" {
		return s.driveDocuments(ctx, r)
	}
	if r.Submission == "pending" {
		return s.dispatch(ctx, r)
	}
	return s.observe(ctx, r)
}
func (s *Service) observe(ctx context.Context, r model.Record) (out model.Record, err error) {
	observed := false
	before := r
	defer func() { s.recordObservation(ctx, before, out, observed, err) }()
	if !runnerbinding.SamePath(r.Runner.StateDir, s.Build.StateDir) {
		return r, errors.New("original Runner namespace differs")
	}
	binding := contentBinding(r)
	binding.Project = s.Project
	selector, id := binding.Selector()
	observation, e := s.Runner.Inspect(ctx, selector, id)
	if e != nil {
		return r, e
	}
	if observation.Run == nil {
		return r, nil
	}
	run := observation.Run
	if err := runnerbinding.ValidateContent(binding, observation); err != nil {
		return r, err
	}
	observed = true
	r.RunID, r.InputDigest, r.Projection = run.RunID, run.InputDigest, run
	r.Diagnostic = run.Diagnostic
	if r.Cancelled {
		r.State = "cancelled"
		if runnerbinding.NeedsContentStop(r.StopRequest, r.StopState, run.StopStatus) {
			r.StopRequest = runnerbinding.ContentStop(Token(), run.Revision)
			r.StopState = "unknown"
			r, e = s.save(ctx, r)
			if e != nil {
				return r, e
			}
			response, callErr := s.Runner.Call(ctx, "stop", []string{"--run-id", r.RunID}, r.StopRequest)
			var receipt runnerclient.Receipt
			if callErr == nil && (json.Unmarshal(response.Data, &receipt) != nil || receipt.RunID != r.RunID || receipt.CommandID == "" || receipt.ReceiptStatus != "accepted") {
				callErr = errors.New("原解析停止回执未核实")
			}
			if callErr == nil {
				r.StopState = "accepted"
			} else {
				r.Diagnostic = callErr.Error()
				var failure *runnerclient.Failure
				if errors.As(callErr, &failure) && !failure.Unknown && failure.Code == "stale_revision" {
					r.StopState = "rejected"
				}
			}
		}
	} else if len(r.Result) == 0 {
		if result := run.ContentResult; result != nil {
			if result.Status != "succeeded" || !result.Source.Verified || result.Source.NativeOutcome != "succeeded" || result.Source.CommandID == "" || result.Source.TurnID == "" {
				r.State = "failed"
				r.Diagnostic = s.contentFailure(ctx, run)
			} else {
				if run.Status != "completed" || run.StopStatus != "stopped" {
					return s.save(ctx, r)
				}
				var frozen runnercontract.GenerateRunInput
				if err := json.Unmarshal(r.Request.Input, &frozen); err != nil {
					return r, err
				}
				if err := runnerclient.VerifyFileMaterials(frozen.Generate.Materials, run.Materials); err != nil {
					r.State, r.Diagnostic = "invalid", err.Error()
					return s.save(ctx, r)
				}
				if err := runnerclient.VerifyFileMaterials(frozen.Generate.Materials, result.FileMaterials); err != nil || model.ValueDigest(run.Materials) != model.ValueDigest(result.FileMaterials) {
					r.State, r.Diagnostic = "invalid", "Run 与结果的固定文件材料不一致"
					return s.save(ctx, r)
				}
				if r.Purpose == "documents" {
					if result.RepositorySource == nil || run.RepositorySource == nil ||
						!sameRepositoryPath(result.RepositorySource.LocalPath, s.Repository) ||
						model.ValueDigest(result.RepositorySource) != model.ValueDigest(run.RepositorySource) {
						r.State = "invalid"
						r.Diagnostic = "文档 Run 与结果的固定仓库来源不一致"
						return s.save(ctx, r)
					}
					r.Result = append(json.RawMessage(nil), result.JSON...)
					if err := s.verifyInvestigation(ctx, r, r.Result, result.RepositorySource); err != nil {
						r.State, r.Diagnostic = "invalid", err.Error()
						return s.save(ctx, r)
					}
					applied, err := model.DecodeDocuments(r, r.Result, result.RepositorySource)
					if err == nil && applied.State == "documents-review" {
						var catalogueErr error
						catalogue, catalogueErr := TestCatalogue(applied)
						if catalogueErr != nil {
							err = catalogueErr
						} else {
							err = ValidateTestIDs(applied, catalogue)
						}
					}
					if err != nil {
						r.State = "invalid"
						r.Diagnostic = err.Error()
					} else {
						r = applied
					}
					return s.save(ctx, r)
				}
				if frozen.Generate.Repository != nil {
					expected := map[string]string{}
					for _, material := range r.Materials {
						if material.Content != nil {
							expected[material.Path] = model.Digest([]byte(*material.Content))
						}
					}
					if model.ValueDigest(result.RepositorySource) != model.ValueDigest(run.RepositorySource) {
						r.State = "invalid"
						r.Diagnostic = "原解析 Run 与结果的固定仓库来源不一致"
						return s.save(ctx, r)
					}
					if err := runnerclient.VerifyRepositoryFiles(result.RepositorySource, *frozen.Generate.Repository, expected); err != nil {
						r.State = "invalid"
						r.Diagnostic = err.Error()
						return s.save(ctx, r)
					}
				}
				r.Result = append(json.RawMessage(nil), result.JSON...)
				a, err := model.DecodeAnalysis(r.Result, r.Source, r.Existing)
				if err == nil {
					err = model.ValidateSourceMapping(a, r.Mapping)
				}
				if err != nil {
					r.State = "invalid"
					r.Diagnostic = err.Error()
				} else {
					r.Analysis = &a
					if err := validateProviders(r); err != nil {
						r.State = "invalid"
						r.Diagnostic = err.Error()
						return s.save(ctx, r)
					}
					r.State = "review"
					if len(a.Questions) > 0 {
						r.State = "blocked"
					}
				}
			}
		} else if run.Status == "failed" || run.Status == "stopped" || run.Status == "completed" {
			r.State = "failed"
			r.Diagnostic = s.contentFailure(ctx, run)
		}
	}
	return s.save(ctx, r)
}
func (s *Service) Confirm(ctx context.Context, id, request string, expected uint64) (model.Record, error) {
	if !model.IDPattern.MatchString(request) {
		return model.Record{}, errors.New("invalid request_id")
	}
	r, e := s.Store.Roadmap(ctx, s.Project, id)
	if e != nil {
		return r, e
	}
	if r.ConfirmationRequest == request {
		return r, nil
	}
	if r.Revision != expected {
		return r, errors.New("stale_revision: 请刷新解析结果")
	}
	source, e := s.Source(ctx, r.Source.Path)
	if e != nil {
		return r, e
	}
	graph, e := s.Graph(ctx)
	if e != nil {
		return r, e
	}
	if e = s.verifyPlanning(ctx, r, source.Head); e != nil {
		return r, e
	}
	r, e = model.Confirm(r, source, graph, request)
	if e != nil {
		return r, e
	}
	return s.save(ctx, r)
}
func (s *Service) Stop(ctx context.Context, id, request string) (model.Record, error) {
	if !model.IDPattern.MatchString(request) {
		return model.Record{}, errors.New("invalid request_id")
	}
	r, e := s.Store.Roadmap(ctx, s.Project, id)
	if e != nil {
		return r, e
	}
	if r.Cancelled {
		if r.StopID != request {
			return r, errors.New("已存在原停止请求")
		}
		return r, nil
	}
	if r.ObservationComplete() {
		return r, errors.New("原内容流程已结束，无需再停止")
	}
	r.Cancelled = true
	r.StopID = request
	r.State = "cancelled"
	return s.save(ctx, r)
}
func (s *Service) Close(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rows, e := s.Store.Roadmaps(ctx, s.Project)
	if e != nil {
		return e
	}
	var errs []error
	for _, r := range rows {
		if r.ObservationComplete() {
			continue
		}
		if !r.Cancelled {
			r, e = s.Stop(ctx, r.ID, Token())
			if e != nil {
				errs = append(errs, e)
				continue
			}
		}
		for !r.ObservationComplete() {
			r, e = s.Observe(ctx, r.ID)
			if e != nil || r.ObservationComplete() {
				break
			}
			select {
			case <-ctx.Done():
				e = ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
			if e != nil {
				break
			}
		}
		errs = append(errs, e)
	}
	return errors.Join(errs...)
}

func ParseSchema() json.RawMessage {
	dependency := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"local", "formal"}, "properties": map[string]any{"local": map[string]any{"type": "array", "uniqueItems": true, "items": map[string]string{"type": "string"}}, "formal": map[string]any{"type": "array", "uniqueItems": true, "items": map[string]string{"type": "string", "pattern": "^(0|[1-9][0-9]*)$"}}}}
	properties := map[string]any{}
	required := []string{"local_key", "key_origin", "title", "goal", "scope", "constraints", "acceptance", "evidence", "original_dependencies", "proposed_dependencies", "reason", "existing_task_id", "provides_tests"}
	for _, key := range required {
		properties[key] = map[string]string{"type": "string"}
	}
	// The output shape is structural only; original evidence stays in the
	// pinned file and DecodeAnalysis verifies literal source lines afterwards.
	properties["evidence"] = map[string]any{"type": "string", "minLength": 1}
	properties["key_origin"] = map[string]any{"type": "string", "enum": []string{"markdown", "suggested"}}
	properties["provides_tests"] = map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string", "minLength": 1}}
	properties["original_dependencies"] = dependency
	properties["proposed_dependencies"] = dependency
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"tasks", "questions", "overlap", "coordination"}, "properties": map[string]any{"tasks": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}}, "questions": map[string]any{"type": "array", "items": map[string]string{"type": "string"}}, "overlap": map[string]string{"type": "string"}, "coordination": map[string]any{"type": "string", "minLength": 1}}}
	raw, e := json.Marshal(schema)
	if e != nil {
		panic(fmt.Sprint(e))
	}
	return raw
}

const SourceInputPath = "roadmap-inputs/source.md"
const GraphPath = "roadmap-inputs/existing-task-graph.json"
const MappingPath = "roadmap-inputs/existing-roadmap-source-map.json"
const PlanningPath = "roadmap-inputs/confirmed-planning.json"
const ParseRulesPath = "SteerlineFlow/Roadmap/parse-rules.md"
const DocumentRulesPath = "SteerlineFlow/Roadmap/document-rules.md"

func fileMaterial(path string, body []byte) runnercontract.Document {
	text := string(body)
	return runnercontract.Document{Kind: "file", Path: path, Content: &text}
}

// fixedFile reads only the caller's already versioned Git material; it never
// exports a dynamic record, invents a model path, or changes the repository.
func (s *Service) fixedFile(ctx context.Context, commit, path string, expected []byte, rules bool) (runnercontract.Document, error) {
	if s.ReadCommitted == nil || commit == "" {
		return runnercontract.Document{}, errors.New("材料准备不足：缺少正式固定 Git 文件读取能力")
	}
	actual, err := s.ReadCommitted(ctx, commit, path)
	if err != nil {
		return runnercontract.Document{}, fmt.Errorf("材料准备不足：%s 不可读：%w", path, err)
	}
	matches := bytes.Equal(actual, expected)
	if rules {
		matches = prompts.SameRules(actual, expected)
	} else if model.SourcePath(path) {
		matches = model.SameMarkdown(actual, expected)
	}
	if !matches {
		return runnercontract.Document{}, fmt.Errorf("材料准备不足：%s 与原始固定来源不同", path)
	}
	text := string(actual)
	return runnercontract.Document{Kind: "inline", Path: path, Content: &text}, nil
}

func contentBinding(r model.Record) runnerbinding.Content {
	revision := uint64(0)
	if r.Projection != nil {
		revision = r.Projection.Revision
	}
	return runnerbinding.Content{StateDir: r.Runner.StateDir, Request: r.Request, RunID: r.RunID, InputDigest: r.InputDigest, Revision: revision, Project: r.Project, ObjectKey: "steerline_roadmap_parse", ObjectID: r.ID, WorkflowID: "generate", WorkflowVersion: 1, RequireDigest: true}
}
