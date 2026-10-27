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

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	roadmapservice "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func (p *runtimeProjectPort) SubmitRoadmap(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	return p.roadmapCommand(ctx, in)
}
func (p *runtimeProjectPort) RoadmapReceipt(ctx context.Context, id string) (*httpcontract.CommandResponse, error) {
	return p.roadmapReceipt(ctx, id)
}

type roadmapPort struct {
	config        Config
	store         *sqlite.SQLiteStore
	service       *roadmapservice.Service
	publicationMu sync.Mutex
	mutateTask    func(context.Context, func(context.Context) error) error
}

func newRoadmapPort(c Config, store *sqlite.SQLiteStore, cli runnerclient.Client, admit func(context.Context) error) *roadmapPort {
	p := &roadmapPort{config: c, store: store}
	p.service = &roadmapservice.Service{Project: c.Identity(), Repository: c.Repository, Store: store, Runner: cli, Build: cli.BuildPin(), Queue: c.runtimeGenerations, Admit: admit}
	p.service.Source = p.source
	p.service.Graph = p.graph
	p.service.Prepare = func(ctx context.Context, id string) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		profile, e := c.RunnerExecution.Profile(id, "")
		if e != nil {
			return runnercontract.ExecutionInput{}, nil, e
		}
		execution, e := runnerclient.GenerationExecution(profile.Execution)
		if e != nil {
			return execution, nil, e
		}
		if c.runtimeCatalog != nil {
			return execution, c.runtimeCatalog, nil
		}
		version, e := cli.Version(ctx)
		if e != nil {
			return execution, nil, e
		}
		catalog, e := cli.Catalog(ctx, version)
		return execution, catalog, e
	}
	p.service.PrepareDocuments = p.documentContext
	p.service.VerifyBinding = func(r roadmap.Record) error {
		if cli.Takeover == nil || !runnerbinding.Compatible(*cli.Takeover, cli.BuildPin()) {
			return errors.New("原解析 Runner 接管证明不可用")
		}
		return verifyRoadmapTakeover(r, *cli.Takeover)
	}
	return p
}
func (p *roadmapPort) source(ctx context.Context, relative string) (roadmap.Source, error) {
	m := p.config.runtimeMaintenance
	if m == nil {
		return roadmap.Source{}, errors.New("project Git unavailable")
	}
	head, e := m.git(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if e != nil {
		return roadmap.Source{}, e
	}
	source, e := roadmapservice.ReadSource(p.config.Repository, p.config.Identity(), relative, strings.TrimSpace(string(head)))
	if e != nil {
		return source, e
	}
	committed, readErr := m.read(ctx, source.Head, relative)
	source.Modified = readErr != nil || roadmap.Digest(committed) != source.Digest
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
		spec, e := p.config.runtimeMaintenance.read(ctx, observation.Commit, contract.SpecRef.Path)
		if e != nil {
			return nil, fmt.Errorf("已有任务 #%s 的 Spec 无法读取: %w", t.TaskID, e)
		}
		n := roadmap.Existing{ID: t.TaskID, Title: t.Title, Spec: contract.SpecRef.Path, SpecText: string(spec), MaterialDigest: roadmap.Digest(spec), Predecessors: []string{}}
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
				roadmap.Record
				Preview any `json:"publication_preview"`
			}{r, preview}, nil
		}
		return r, e
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
	}
	profiles := []map[string]string{}
	for _, profile := range p.config.RunnerExecution.Profiles {
		if _, e := generationProfile(profile); e == nil {
			profiles = append(profiles, map[string]string{"id": profile.ID, "name": profile.Name})
		}
	}
	return struct {
		Sources            []roadmap.Source    `json:"sources"`
		Records            []roadmap.Record    `json:"records"`
		Profiles           []map[string]string `json:"profiles"`
		DocumentsAvailable bool                `json:"documents_available"`
		DocumentsReason    string              `json:"documents_reason"`
	}{sources, records, profiles, p.config.runtimeCatalog.RepositoryGenerationUnavailable() == "", p.config.runtimeCatalog.RepositoryGenerationUnavailable()}, nil
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
	allowed := map[string]bool{"roadmap_parse": true, "roadmap_confirm": true, "roadmap_observe": true, "roadmap_stop": true, "roadmap_generate_documents": true, "roadmap_publish": true, "roadmap_edit_documents": true, "roadmap_admit": true, "roadmap_resume": true}
	if !allowed[in.Action] {
		return out, errors.New("unknown roadmap action")
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
		if receipt, e := p.roadmapReceipt(ctx, in.RequestID); e != nil {
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
		if record, e := port.store.RoadmapRequest(context.WithoutCancel(ctx), port.config.Identity(), in.RequestID); e == nil && roadmapEffectPending(in.Action, record) {
			out.Status = "outcome_unknown"
			out.Code = "outcome_unknown"
		}
	} else {
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
	if in.Action == "roadmap_parse" {
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
		if e := decodeStrictJSON(in.Payload, &struct{}{}); e != nil {
			return nil, e
		}
		return p.service.GenerateDocuments(ctx, r.ID, in.RequestID, in.ExpectedVersion)
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
	if out.Status == "outcome_unknown" || old.Status == "pending" {
		r, e := p.config.runtimeRoadmaps.store.RoadmapRequest(ctx, p.config.Identity(), id)
		if e == nil {
			out.Roadmap, _ = json.Marshal(r)
			if !roadmapEffectPending(envelope.Action, r) {
				out.Status = "applied"
				out.Code = ""
				out.Diagnostic = ""
			}
		} else if !errors.Is(e, roadmap.ErrNotFound) {
			return nil, e
		}
	}
	return &out, nil
}

func roadmapEffectPending(action string, r roadmap.Record) bool {
	switch action {
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
