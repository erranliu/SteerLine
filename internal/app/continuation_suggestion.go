package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/steeragent/suggestion/contract"
	suggestionbridge "github.com/erranliu/SteerLine/internal/steeragentbridge/suggestion"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

type continuationSuggestionPort struct {
	config  Config
	store   *sqlite.SQLiteStore
	service *suggestionbridge.Service
}

func newContinuationSuggestionPort(c Config, store *sqlite.SQLiteStore, cli runnerclient.Client, admit func(context.Context) error) *continuationSuggestionPort {
	c = bindNativeConfiguration(c, cli)
	p := &continuationSuggestionPort{config: c, store: store}
	p.service = &suggestionbridge.Service{Project: c.Identity(), Store: store, Build: p.context, Runner: cli, RunnerBuild: cli.BuildPin(), Queue: c.runtimeGenerations, Admit: admit}
	p.service.Prepare = func(ctx context.Context) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		settings, err := executionconfig.ContinuationSuggestionSettings(c.RunnerExecution.ContinuationSuggestion)
		if err != nil {
			return runnercontract.ExecutionInput{}, nil, err
		}
		resolved, err := c.runtimeNative.Resolve(ctx, runnercontract.ExecutionConfigurationRequest{Operation: "compose", BaseExecution: executionconfig.ContinuationSuggestionBase(c.RunnerExecution.Profiles), Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Selection: &runnercontract.ExecutionSelection{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Agent: "codex", Model: &settings.Model, ReasoningEffort: &settings.ReasoningEffort, AccessMode: "read_only"}})
		if err != nil {
			return runnercontract.ExecutionInput{}, nil, err
		}
		if resolved.Execution == nil {
			return runnercontract.ExecutionInput{}, nil, errors.New("Runner 未返回对话续作 · 提示词建议 执行配置")
		}
		return *resolved.Execution, c.runtimeNative.Catalog, nil
	}
	return p
}

// The source is the persisted public question and reply control, never prose classification.
func materialAdviceSource(c runnerstate.Candidate) (suggestionbridge.Source, bool) {
	r := c.Projection
	if c.Participation != "active" || c.StopRequested || r == nil || r.Question == nil || r.Question.ID == "" || r.Execution.AttemptID == "" || r.Closed || !r.Controls.CanReply {
		return suggestionbridge.Source{}, false
	}
	source := suggestionbridge.Source{CandidateID: c.ID, TaskID: c.TaskID, Occurrence: r.Execution.AttemptID + ":" + r.Question.ID, Revision: r.Revision, Reason: "pending_question", Goal: r.Question.Text}
	source.BaseRevision, source.ContractDigest = c.BaseRevision, c.ContractDigest
	q, _ := json.Marshal(r.Question)
	source.Facts = []contract.Material{{ID: "question", Label: "原问题", Body: string(q), Source: c.ID, Revision: source.Occurrence}}
	if r.Final != nil && r.Final.Verified {
		source.Facts = append(source.Facts, contract.Material{ID: "final", Label: "已核实的原结果", Body: r.Final.Text, Source: c.ID, Revision: source.Occurrence})
	}
	if len(c.TestPlan) > 0 {
		source.Facts = append(source.Facts, contract.Material{ID: "test-plan", Label: "原测试安排", Body: string(c.TestPlan), Source: c.ID})
	}
	return source, source.Valid()
}
func (p *continuationSuggestionPort) tick(ctx context.Context) error {
	snapshot, err := p.store.ReadRunnerSnapshot(ctx, p.config.Identity())
	if err != nil {
		return err
	}
	sources := []suggestionbridge.Source{}
	for _, c := range snapshot.Candidates {
		if source, ok := materialAdviceSource(c); ok {
			sources = append(sources, source)
		}
	}
	if err = p.service.Sync(ctx, sources); err != nil {
		return err
	}
	return p.service.ProcessNext(ctx)
}
func (p *continuationSuggestionPort) run(ctx context.Context) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		step, cancel := context.WithTimeout(ctx, 10*time.Second)
		_ = p.tick(step)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}
func (p *continuationSuggestionPort) context(ctx context.Context, source suggestionbridge.Source, id string) (contract.Request, error) {
	out := contract.Request{Protocol: contract.Protocol, RequestID: id, Capability: contract.Capability, Context: contract.Context{Goal: source.Goal, Materials: []contract.Material{}}}
	c, err := p.store.Candidate(ctx, p.config.Identity(), source.CandidateID)
	if err != nil {
		return out, err
	}
	current, ok := materialAdviceSource(c)
	if !ok || current.Occurrence != source.Occurrence || c.BaseRevision != source.BaseRevision || c.ContractDigest != source.ContractDigest {
		return out, errors.New("原待答问题已变化")
	}
	// Use the original occurrence/goal. Later observations cannot replace the frozen request.
	out.Context.Materials = append(out.Context.Materials, source.Facts...)
	refs := append(append(append(c.Contract.CommonRefs[:0:0], c.Contract.CommonRefs...), c.Contract.AcceptanceRefs...), c.Contract.SpecRef, c.Contract.PlanRef)
	seen := map[string]bool{}
	for _, ref := range refs {
		if ref.Path == "" || seen[ref.Path] {
			continue
		}
		seen[ref.Path] = true
		body, e := p.config.runtimeMaintenance.read(ctx, ref.Commit, ref.Path)
		if e != nil {
			return out, e
		}
		if ref.Commit == "" || contract.Digest(body) != ref.SHA256 {
			return out, errors.New("原绑定材料正文或版本不匹配：" + ref.Path)
		}
		out.Context.Materials = append(out.Context.Materials, contract.Material{ID: ref.Path, Label: ref.Path, Body: string(body), Source: ref.Path, Revision: ref.Commit})
	}
	return out, contract.Validate(out)
}
func (p *continuationSuggestionPort) read(ctx context.Context, route string, values url.Values) (any, error) {
	if route == "/main-agent" {
		if len(values) == 2 && len(values["candidate_id"]) == 1 && len(values["occurrence"]) == 1 {
			source := suggestionbridge.Source{CandidateID: values.Get("candidate_id"), Occurrence: values.Get("occurrence"), Revision: 1, Reason: "pending_question"}
			if !source.Valid() {
				return nil, errors.New("invalid advisory source")
			}
			record, err := p.service.Get(ctx, suggestionbridge.SourceID(p.config.Identity(), source))
			if errors.Is(err, contract.ErrNotFound) {
				return struct {
					Record *suggestionbridge.Detail `json:"record"`
				}{}, nil
			}
			if err != nil {
				return nil, err
			}
			detail := suggestionbridge.View(record)
			return struct {
				Record *suggestionbridge.Detail `json:"record"`
			}{&detail}, nil
		}
		if len(values) != 1 || len(values["candidate_id"]) != 1 || !contract.ID.MatchString(values.Get("candidate_id")) {
			return nil, errors.New("exact candidate_id required")
		}
		rows, more, err := p.service.List(ctx, values.Get("candidate_id"))
		if err != nil {
			return nil, err
		}
		return struct {
			Records []json.RawMessage `json:"records"`
			HasMore bool              `json:"has_more"`
		}{rows, more}, nil
	}
	id := strings.TrimPrefix(route, "/main-agent/")
	if len(values) > 0 || !contract.ID.MatchString(id) {
		return nil, errors.New("invalid advisory request")
	}
	r, err := p.service.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return suggestionbridge.View(r), nil
}
