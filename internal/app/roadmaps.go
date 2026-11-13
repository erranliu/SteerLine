package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	roadmapservice "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

func (p *runtimeProjectPort) SubmitRoadmap(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	if strings.HasPrefix(in.Action, planningAction) {
		port := p.config.runtimeRoadmaps
		if port == nil || port.planning == nil || !p.Server.Identity(ctx).Ready {
			return maintenanceResponse(in), projectStoppingError()
		}
		return port.planning.command(ctx, in)
	}
	return p.roadmapCommand(ctx, in)
}
func (p *runtimeProjectPort) RoadmapReceipt(ctx context.Context, id string) (*httpcontract.CommandResponse, error) {
	if port := p.config.runtimeRoadmaps; port != nil && port.planning != nil {
		if receipt, err := port.planning.receipt(ctx, id); receipt != nil || err != nil {
			return receipt, err
		}
	}
	return p.roadmapReceipt(ctx, id)
}

type roadmapPort struct {
	planning         *planningPort
	config           Config
	store            *sqlite.SQLiteStore
	materials        committedMaterialReader
	service          *roadmapservice.Service
	publicationMu    sync.Mutex
	generationMu     sync.Mutex
	initializationMu sync.Mutex
	preparationMu    sync.Mutex
	preparationCache *roadmapPreparation
	taskAdmission    roadmapTaskAdmission
	mergeHistory     roadmapMergeHistory
}

func newRoadmapPort(c Config, store *sqlite.SQLiteStore, cli runnerclient.Client, admit func(context.Context) error, materials committedMaterialReader, taskAdmission roadmapTaskAdmission, mergeHistory roadmapMergeHistory, refreshBaselineForPush func(context.Context) (commit string, rewritten bool, err error)) *roadmapPort {
	c = bindNativeConfiguration(c, cli)
	p := &roadmapPort{config: c, store: store, materials: materials, taskAdmission: taskAdmission, mergeHistory: mergeHistory}
	p.planning = &planningPort{roadmapPort: p, runner: cli, admit: admit, refreshBaselineForPush: refreshBaselineForPush}
	p.service = &roadmapservice.Service{Project: c.Identity(), Repository: c.Repository, Store: store, Runner: cli, Build: cli.BuildPin(), Queue: c.runtimeGenerations, Admit: admit}
	p.service.Source = p.source
	p.service.Graph = p.graph
	p.service.Prepare = c.prepareSteerAgentGeneration
	p.service.PrepareDocuments = p.documentContext
	p.service.ReadCommitted = materials.readCommittedMaterial
	if c.runtimeMaintenance != nil {
		p.service.ReadCommittedFiles = c.runtimeMaintenance.readCommittedMaterials
	}
	return p
}
func (p *roadmapPort) source(ctx context.Context, relative string) (roadmap.Source, error) {
	if p.materials == nil {
		return roadmap.Source{}, errors.New("project Git unavailable")
	}
	head, e := p.config.repositoryGit(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if e != nil {
		return roadmap.Source{}, e
	}
	source, e := roadmapservice.ReadSource(p.config.Repository, p.config.Identity(), relative, strings.TrimSpace(string(head)))
	if e != nil {
		return source, e
	}
	committed, readErr := p.materials.readCommittedMaterial(ctx, source.Head, relative)
	source.Modified = readErr != nil || !roadmap.SameMarkdown(committed, []byte(source.Body))
	return source, nil
}
func (p *roadmapPort) graph(ctx context.Context) ([]roadmap.Existing, error) {
	graph, e := p.store.TaskGraphSnapshot(ctx, p.config.Identity())
	if e != nil {
		return nil, e
	}
	result := []roadmap.Existing{}
	if len(graph.Tasks) == 0 {
		return result, nil
	}
	observation, e := p.config.runtimeMaintenance.check(ctx)
	if e != nil {
		return nil, e
	}
	for _, t := range graph.Tasks {
		contract, e := p.store.TaskContract(ctx, p.config.Identity(), t)
		if e != nil {
			return nil, e
		}
		spec, e := p.materials.readCommittedMaterial(ctx, observation.Commit, contract.SpecRef.Path)
		if e != nil {
			return nil, fmt.Errorf("已有任务 #%s 的 Spec 无法读取: %w", t.TaskID, e)
		}
		n := roadmap.Existing{ID: t.TaskID, Title: t.Title, Status: t.Status, Spec: contract.SpecRef.Path, SpecText: string(spec), MaterialDigest: roadmap.Digest(spec), Predecessors: []string{}}
		for _, edge := range graph.Edges {
			if edge.SuccessorID == t.TaskID {
				n.Predecessors = append(n.Predecessors, edge.PredecessorID)
			}
		}
		sort.Strings(n.Predecessors)
		result = append(result, n)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

type publicRoadmapGeneration struct {
	roadmap.Generation
	Settled        bool   `json:"settled"`
	RunnerStatus   string `json:"runner_status,omitempty"`
	ResumeRequired bool   `json:"resume_required"`
	AttemptID      string `json:"attempt_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
}

type publicRoadmapRecord struct {
	Generations []publicRoadmapGeneration `json:"generations,omitempty"`
	roadmap.Record
	Execution api.ExecutionReference `json:"execution_ref"`
}

func publicRoadmap(r roadmap.Record) publicRoadmapRecord {
	ref := api.ExecutionReference{RequestID: r.RequestID, RunID: r.RunID}
	if r.Projection != nil {
		ref.AttemptID = r.Projection.Execution.AttemptID
	}
	out := publicRoadmapRecord{Record: r, Execution: ref}
	for _, g := range r.Generations {
		v := publicRoadmapGeneration{Generation: g, Settled: r.Execution(g).Settled()}
		if g.Projection != nil {
			v.RunnerStatus = g.Projection.Status
			v.ResumeRequired = g.Projection.ResumeRequired
			if v.Diagnostic == "" {
				v.Diagnostic = g.Projection.Diagnostic
			}
			v.AttemptID, v.SessionID = g.Projection.Execution.AttemptID, g.Projection.Execution.SessionID
		}
		out.Generations = append(out.Generations, v)
	}
	return out
}

func (p *roadmapPort) read(ctx context.Context, route string, values url.Values) (any, error) {
	if len(values) > 0 {
		return nil, errors.New("roadmap query does not accept parameters")
	}
	if strings.HasPrefix(route, "/roadmaps/sources/") {
		id := strings.TrimPrefix(route, "/roadmaps/sources/")
		sources, e := roadmapservice.ListSources(p.config.Repository, p.config.Identity())
		if e != nil {
			return nil, e
		}
		for _, source := range sources {
			if source.ID == id {
				return p.source(ctx, source.Path)
			}
		}
		return nil, roadmap.ErrNotFound
	}
	if route != "/roadmaps" {
		r, e := p.store.Roadmap(ctx, p.config.Identity(), strings.TrimPrefix(route, "/roadmaps/"))
		if e != nil {
			return nil, e
		}
		r.Review = roadmap.ReviewDigest(r)
		r.Mapping, e = p.store.RoadmapMapping(ctx, p.config.Identity(), r.Source.ID)
		if e == nil && r.DocumentsVerified && r.Publication == nil {
			preview, err := p.publicationPreview(ctx, r)
			if err != nil {
				return nil, err
			}
			return struct {
				publicRoadmapRecord
				Preview any `json:"publication_preview"`
			}{publicRoadmap(r), preview}, nil
		}
		return publicRoadmap(r), e
	}
	sources, e := roadmapservice.ListSources(p.config.Repository, p.config.Identity())
	if e != nil {
		return nil, e
	}
	records, e := p.store.Roadmaps(ctx, p.config.Identity())
	if e != nil {
		return nil, e
	}
	// Listing is read-only and bounded to summaries, never model calls or Git fetch.
	for i := range sources {
		sources[i].Body = ""
	}
	for i := range records {
		records[i].Mapping, e = p.store.RoadmapMapping(ctx, p.config.Identity(), records[i].Source.ID)
		if e != nil {
			return nil, e
		}
		records[i].Source.Body = ""
		records[i].Existing = nil
		records[i].Result = nil
		records[i].Documents = nil
		records[i].Materials = nil
		records[i].PlanningMaterials = nil
		for j := range records[i].Generations {
			records[i].Generations[j].Result = nil
			records[i].Generations[j].Document = nil
			records[i].Generations[j].Investigation = nil
		}
	}
	publicRecords := []publicRoadmapRecord{}
	for _, r := range records {
		publicRecords = append(publicRecords, publicRoadmap(r))
	}
	agent, err := executionconfig.SteerAgentSettings(p.config.RunnerExecution.SteerAgent)
	if err != nil {
		return nil, err
	}
	reason := p.config.runtimeCatalog.FileMaterialGenerationUnavailable()
	if reason == "" {
		if _, _, err = p.config.prepareSteerAgentGeneration(ctx); err != nil {
			reason = err.Error()
		}
	}
	return struct {
		Sources            []roadmap.Source           `json:"sources"`
		Records            []publicRoadmapRecord      `json:"records"`
		SteerAgent         executionconfig.SteerAgent `json:"steer_agent"`
		DocumentsAvailable bool                       `json:"documents_available"`
		DocumentsReason    string                     `json:"documents_reason"`
		Preparation        roadmapPreparation         `json:"preparation"`
	}{sources, publicRecords, agent, reason == "", reason, p.preparation(ctx)}, nil
}
func (p *runtimeProjectPort) roadmapCommand(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	out := maintenanceResponse(in)
	if e := in.Validate(httpcontract.ProjectID(p.config.Identity()), p.instance); e != nil {
		return out, e
	}
	port := p.config.runtimeRoadmaps
	if port == nil || p.actor == nil || !p.Server.Identity(ctx).Ready {
		return out, projectStoppingError()
	}
	allowed := map[string]bool{"roadmap_prepare_static": true, "roadmap_parse": true, "roadmap_confirm": true, "roadmap_observe": true, "roadmap_stop": true, "roadmap_generate_documents": true, "roadmap_publish": true, "roadmap_edit_documents": true, "roadmap_admit": true, "roadmap_resume": true, "roadmap_retry_documents": true, taskPlanningGenerate: true}
	if !allowed[in.Action] {
		return out, errors.New("unknown roadmap action")
	}
	if roadmapGenerationCommand(in.Action) {
		port.generationMu.Lock()
		defer port.generationMu.Unlock()
	}
	if in.Action == "roadmap_prepare_static" {
		port.initializationMu.Lock()
		defer port.initializationMu.Unlock()
	}
	raw, e := json.Marshal(in)
	if e != nil {
		return out, e
	}
	old, found, e := p.store.LookupCommand(ctx, p.config.Identity(), in.RequestID)
	if e != nil {
		return out, e
	}
	if found {
		if old.Kind != "roadmap_command" || !bytes.Equal(raw, old.Payload) {
			return out, errors.New("request_conflict: original roadmap envelope differs")
		}
		if receipt, e := p.roadmapReceiptLocked(ctx, in.RequestID); e != nil {
			return out, e
		} else if receipt != nil {
			return *receipt, nil
		}
		return out, nil
	}
	command := storageports.CommandRecord{ProjectIdentity: p.config.Identity(), CommandID: in.RequestID, Kind: "roadmap_command", TargetID: in.TargetID, ExpectedVersion: in.ExpectedVersion, IdempotencyKey: in.RequestID, RequestedBy: "local-user", Payload: raw, Status: "pending", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	command, created, e := p.store.EnqueueCommand(ctx, command)
	if e != nil {
		return out, e
	}
	if !created {
		return out, nil
	}
	result, actionErr := port.action(ctx, in)
	out.Status = "applied"
	if actionErr != nil {
		out.Status = "rejected"
		out.Code = "roadmap_rejected"
		out.Diagnostic = actionErr.Error()
		if record, e := port.store.RoadmapRequest(context.WithoutCancel(ctx), port.config.Identity(), in.RequestID); e == nil {
			if roadmapGenerationCommand(in.Action) {
				// A read-back binding proves the atomic work intent was saved,
				// even if its write receipt failed. Generation finishes separately.
				out.Status, out.Code, out.Diagnostic = "applied", "", ""
				out.Roadmap, _ = json.Marshal(publicRoadmap(record))
			} else if roadmapEffectPending(in.Action, record) {
				out.Status = "outcome_unknown"
				out.Code = "outcome_unknown"
			}
		}
	} else {
		if record, ok := result.(roadmap.Record); ok {
			result = publicRoadmap(record)
		}
		out.Roadmap, _ = json.Marshal(result)
	}
	command.Result, _ = json.Marshal(out)
	command.Status = out.Status
	command.UpdatedAt = time.Now().UTC()
	if e = p.store.UpdateCommand(context.WithoutCancel(ctx), command); e != nil {
		return out, e
	}
	return out, nil
}
func (p *roadmapPort) action(ctx context.Context, in httpcontract.CommandEnvelope) (any, error) {
	if in.Action == taskPlanningGenerate {
		return p.generateTaskDocuments(ctx, in)
	}
	if in.Action == "roadmap_prepare_static" {
		return p.prepareStatic(ctx, in)
	}
	if in.Action == "roadmap_parse" {
		p.config.runtimeNative.RefreshFailures()
		if !p.preparation(ctx).Ready {
			return nil, errors.New("Roadmap 静态材料未就绪：请先初始化材料，再提交并发布到项目目标分支后重试")
		}
		var input roadmapservice.ParseInput
		if e := decodeStrictJSON(in.Payload, &input); e != nil {
			return nil, e
		}
		if in.TargetID != roadmap.SourceID(p.config.Identity(), input.Path) || in.ExpectedVersion != 1 || input.RequestID != in.RequestID {
			return nil, errors.New("parse target/request differs")
		}
		return p.service.Parse(ctx, input)
	}
	if !roadmap.IDPattern.MatchString(in.TargetID) {
		return nil, errors.New("invalid roadmap record target")
	}
	if in.Action == "roadmap_confirm" {
		if e := decodeStrictJSON(in.Payload, &struct{}{}); e != nil {
			return nil, e
		}
		return p.service.Confirm(ctx, in.TargetID, in.RequestID, in.ExpectedVersion)
	}
	if in.Action == "roadmap_observe" {
		if e := decodeStrictJSON(in.Payload, &struct{}{}); e != nil {
			return nil, e
		}
		r, e := p.store.Roadmap(ctx, p.config.Identity(), in.TargetID)
		if e != nil {
			return nil, e
		}
		if r.State == "admission-unknown" {
			return p.reconcileAdmission(ctx, r)
		}
		if r.Publication != nil {
			return p.observePublication(ctx, r)
		}
		return p.service.Observe(ctx, in.TargetID)
	}
	if in.Action == "roadmap_stop" {
		if e := decodeStrictJSON(in.Payload, &struct{}{}); e != nil {
			return nil, e
		}
		return p.service.Stop(ctx, in.TargetID, in.RequestID)
	}
	r, e := p.store.Roadmap(ctx, p.config.Identity(), in.TargetID)
	if e != nil {
		return nil, e
	}
	if r.Revision != in.ExpectedVersion {
		return nil, errors.New("stale_revision: 请刷新 Roadmap")
	}
	switch in.Action {
	case "roadmap_resume":
		if e := decodeStrictJSON(in.Payload, &struct{}{}); e != nil {
			return nil, e
		}
		if r.State == "admission-unknown" {
			return p.resumeAdmission(ctx, r)
		}
		return p.resumePublication(ctx, r)
	case "roadmap_generate_documents":
		p.config.runtimeNative.RefreshFailures()
		if e := decodeStrictJSON(in.Payload, &struct{}{}); e != nil {
			return nil, e
		}
		if e := p.verifyCurrentTaskCard(ctx, r); e != nil {
			return nil, e
		}
		return p.service.GenerateDocuments(ctx, r.ID, in.RequestID, in.ExpectedVersion)
	case "roadmap_retry_documents":
		var input struct {
			Key string `json:"local_key"`
		}
		if e := decodeStrictJSON(in.Payload, &input); e != nil {
			return nil, e
		}
		if e := p.verifyCurrentTaskCard(ctx, r); e != nil {
			return nil, e
		}
		return p.service.RetryDocuments(ctx, r.ID, input.Key, in.RequestID, in.ExpectedVersion)
	case "roadmap_edit_documents":
		if r.State != "documents-review" || r.Publication != nil {
			return nil, errors.New("只有未发布的文档生成结果可以修改")
		}
		var input struct {
			Key  string `json:"local_key"`
			Kind string `json:"kind"`
			Body string `json:"body"`
		}
		if e := decodeStrictJSON(in.Payload, &input); e != nil {
			return nil, e
		}
		found := false
		for i := range r.Documents {
			if r.Documents[i].Key == input.Key {
				switch input.Kind {
				case "spec":
					r.Documents[i].Spec.Body = input.Body
				case "plan":
					r.Documents[i].Plan.Body = input.Body
				case "acceptance":
					r.Documents[i].Acceptance[0].Body = input.Body
				case "manual_checks":
					r.Documents[i].ManualChecks = nil
					if strings.TrimSpace(input.Body) != "" {
						r.Documents[i].ManualChecks = json.RawMessage(input.Body)
					}
				case "tests":
					var selection tm.Arrangement
					if err := tm.Decode([]byte(input.Body), &selection); err != nil {
						return nil, err
					}
					r.Documents[i].Tests = selection.Tests
				default:
					return nil, errors.New("文档类型无效")
				}
				found = true
			}
		}
		if !found {
			return nil, errors.New("内部任务代号不存在")
		}
		if e := roadmap.ValidateDocuments(r); e != nil {
			return nil, e
		}
		catalogue, e := roadmapservice.TestCatalogue(r)
		if e != nil {
			return nil, e
		}
		if e = roadmapservice.ValidateTestIDs(r, catalogue); e != nil {
			return nil, e
		}
		r.Review = ""
		return p.serviceSave(ctx, r)
	case "roadmap_publish":
		var input struct {
			ReviewDigest string `json:"review_digest"`
			Merge        bool   `json:"merge"`
		}
		if e := decodeStrictJSON(in.Payload, &input); e != nil {
			return nil, e
		}
		return p.publish(ctx, r, in.RequestID, input.ReviewDigest, input.Merge)
	case "roadmap_admit":
		if e := decodeStrictJSON(in.Payload, &struct{}{}); e != nil {
			return nil, e
		}
		return p.admit(ctx, r, in.RequestID)
	}
	return nil, errors.New("unsupported roadmap action")
}
func (p *runtimeProjectPort) roadmapReceipt(ctx context.Context, id string) (*httpcontract.CommandResponse, error) {
	old, found, err := p.store.LookupCommand(ctx, p.config.Identity(), id)
	if err != nil || !found || old.Kind != "roadmap_command" {
		return nil, err
	}
	var envelope httpcontract.CommandEnvelope
	if err = json.Unmarshal(old.Payload, &envelope); err != nil {
		return nil, err
	}
	if roadmapGenerationCommand(envelope.Action) {
		p.config.runtimeRoadmaps.generationMu.Lock()
		defer p.config.runtimeRoadmaps.generationMu.Unlock()
	}
	if envelope.Action == "roadmap_prepare_static" {
		p.config.runtimeRoadmaps.initializationMu.Lock()
		defer p.config.runtimeRoadmaps.initializationMu.Unlock()
	}
	return p.roadmapReceiptLocked(ctx, id)
}

func (p *runtimeProjectPort) roadmapReceiptLocked(ctx context.Context, id string) (*httpcontract.CommandResponse, error) {
	old, found, e := p.store.LookupCommand(ctx, p.config.Identity(), id)
	if e != nil || !found || old.Kind != "roadmap_command" {
		return nil, e
	}
	var envelope httpcontract.CommandEnvelope
	if e = json.Unmarshal(old.Payload, &envelope); e != nil {
		return nil, e
	}
	out := maintenanceResponse(envelope)
	if len(old.Result) > 0 {
		if e = json.Unmarshal(old.Result, &out); e != nil {
			return nil, e
		}
	}
	if envelope.Action == "roadmap_prepare_static" && (old.Status == "pending" || out.Status == "outcome_unknown") {
		return p.reconcilePreparation(ctx, old, envelope)
	}
	if out.Status == "outcome_unknown" || old.Status == "pending" {
		r, e := p.config.runtimeRoadmaps.store.RoadmapRequest(ctx, p.config.Identity(), id)
		if e == nil {
			out.Roadmap, _ = json.Marshal(publicRoadmap(r))
			if !roadmapEffectPending(envelope.Action, r) {
				out.Status = "applied"
				out.Code = ""
				out.Diagnostic = ""
			}
		} else if !errors.Is(e, roadmap.ErrNotFound) {
			return nil, e
		} else if envelope.Action == "roadmap_retry_documents" {
			// generationMu excludes an append still being committed. No request
			// binding proves this retry created no executable work.
			out.Status, out.Code, out.Diagnostic = "rejected", "roadmap_retry_not_submitted", "单任务重生成尚未提交，请刷新后重试"
			old.Result, _ = json.Marshal(out)
			old.Status, old.UpdatedAt = out.Status, time.Now().UTC()
			if err := p.store.UpdateCommand(ctx, old); err != nil {
				return nil, err
			}
		} else if envelope.Action == "roadmap_generate_documents" || envelope.Action == taskPlanningGenerate {
			// The generation lock excludes a command still preparing its child.
			// A saved confirmation without a child proves generation was not submitted.
			confirmation := roadmapservice.DocumentConfirmationRequest(id)
			parent, err := p.config.runtimeRoadmaps.store.RoadmapRequest(ctx, p.config.Identity(), confirmation)
			if err != nil && !errors.Is(err, roadmap.ErrNotFound) {
				return nil, err
			}
			confirmed := err == nil && parent.Purpose != "documents" && parent.State == "confirmed" && parent.ConfirmationRequest == confirmation && parent.ConfirmedDigest != ""
			bound := confirmed && (parent.ID == envelope.TargetID || envelope.Action == taskPlanningGenerate && parent.Source.PlanningID == envelope.TargetID)
			if bound || envelope.Action == taskPlanningGenerate {
				out.Status, out.Code = "rejected", "roadmap_generation_not_submitted"
				out.Diagnostic = "文档生成尚未提交，请刷新后重试"
				if bound {
					out.Diagnostic = "任务与依赖已确认，文档生成尚未提交，请重试生成 Spec / Plan"
					out.Roadmap, _ = json.Marshal(publicRoadmap(parent))
				}
				old.Result, _ = json.Marshal(out)
				old.Status, old.UpdatedAt = out.Status, time.Now().UTC()
				if err := p.store.UpdateCommand(ctx, old); err != nil {
					return nil, err
				}
			}
		}
	}
	return &out, nil
}

func roadmapGenerationCommand(action string) bool {
	return action == "roadmap_generate_documents" || action == "roadmap_retry_documents" || action == taskPlanningGenerate
}

func roadmapEffectPending(action string, r roadmap.Record) bool {
	switch action {
	case "roadmap_generate_documents", "roadmap_retry_documents", taskPlanningGenerate:
		// The original command creates durable work; model completion is observed separately.
		return false
	case "roadmap_publish":
		return r.Publication != nil && r.State != "awaiting-merge" && r.State != "awaiting-admission" && r.State != "admitted"
	case "roadmap_admit":
		return r.AdmissionRequest != "" && r.State != "admitted"
	case "roadmap_stop":
		return r.Cancelled && !r.Settled()
	default:
		return r.Submission != "pending" && !r.Settled()
	}
}
