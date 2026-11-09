package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/mainagent/contract"
	"github.com/erranliu/SteerLine/internal/mainagentbridge"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

type mainAgentPort struct {
	config  Config
	store   *sqlite.SQLiteStore
	service *mainagentbridge.Service
}

func newMainAgentPort(c Config, store *sqlite.SQLiteStore) *mainAgentPort {
	p := &mainAgentPort{config: c, store: store}
	p.service = &mainagentbridge.Service{Project: c.Identity(), Store: store, Build: p.context}
	return p
}

// The first source adapter accepts only explicit owner reason codes. Neither
// arbitrary question prose nor generic attention/blocked states classify a problem.
func materialAdviceSource(c runnerstate.Candidate) (mainagentbridge.Source, bool) {
	r := c.Projection
	if c.Participation != "active" || c.StopRequested || r == nil || r.Question == nil || r.Question.ID == "" || r.Execution.AttemptID == "" {
		return mainagentbridge.Source{}, false
	}
	source := mainagentbridge.Source{CandidateID: c.ID, TaskID: c.TaskID, Occurrence: r.Execution.AttemptID + ":" + r.Question.ID, Revision: r.Revision, Reason: r.Controls.Reason, Goal: r.Question.Text}
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
func (p *mainAgentPort) tick(ctx context.Context) error {
	snapshot, err := p.store.ReadRunnerSnapshot(ctx, p.config.Identity())
	if err != nil {
		return err
	}
	sources := []mainagentbridge.Source{}
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
func (p *mainAgentPort) run(ctx context.Context) {
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
func (p *mainAgentPort) context(ctx context.Context, source mainagentbridge.Source, id string) (contract.Request, error) {
	out := contract.Request{Protocol: contract.Protocol, RequestID: id, Capability: contract.Capability, Context: contract.Context{Goal: source.Goal, Materials: []contract.Material{}}}
	c, err := p.store.Candidate(ctx, p.config.Identity(), source.CandidateID)
	if err != nil {
		return out, err
	}
	current, ok := materialAdviceSource(c)
	if !ok || current.Occurrence != source.Occurrence || c.BaseRevision != source.BaseRevision || c.ContractDigest != source.ContractDigest {
		return out, errors.New("原材料问题已变化")
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
		body, e := p.config.runtimeMaintenance.read(ctx, c.BaseRevision, ref.Path)
		if e != nil {
			return out, e
		}
		out.Context.Materials = append(out.Context.Materials, contract.Material{ID: ref.Path, Label: ref.Path, Body: string(body), Source: ref.Path, Revision: c.BaseRevision})
	}
	return out, contract.Validate(out)
}
func (p *mainAgentPort) read(ctx context.Context, route string, values url.Values) (any, error) {
	if route == "/main-agent" {
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
	return mainagentbridge.View(r), nil
}
