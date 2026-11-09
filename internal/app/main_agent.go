package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/mainagent"
	ma "github.com/erranliu/SteerLine/internal/mainagent/model"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

type mainAgentPort struct {
	config        Config
	store         *sqlite.SQLiteStore
	service       *mainagent.Service
	publicationMu sync.Mutex
}

func newMainAgentPort(c Config, store *sqlite.SQLiteStore, cli runnerclient.Client, admit func(context.Context) error) *mainAgentPort {
	c = bindNativeConfiguration(c, cli)
	p := &mainAgentPort{config: c, store: store}
	p.service = &mainagent.Service{Project: c.Identity(), Repository: c.Repository, Store: store, Runner: cli, Build: cli.BuildPin(), Queue: c.runtimeGenerations, Admit: admit, Context: p.context, Prepare: p.prepare, Read: c.runtimeMaintenance.read}
	return p
}
func (p *mainAgentPort) prepare(ctx context.Context) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
	settings, err := executionconfig.MainAgentSettings(p.config.RunnerExecution.MainAgent)
	if err != nil {
		return runnercontract.ExecutionInput{}, nil, err
	}
	selection := runnercontract.ExecutionSelection{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Agent: "codex", Model: &settings.Model, ReasoningEffort: &settings.ReasoningEffort, AccessMode: "read_only"}
	result, err := p.config.runtimeNative.Resolve(ctx, runnercontract.ExecutionConfigurationRequest{Operation: "compose", Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Selection: &selection, BaseExecution: executionconfig.MainAgentBase(p.config.RunnerExecution.Profiles)})
	if err != nil {
		return runnercontract.ExecutionInput{}, nil, err
	}
	if result.Execution == nil {
		return runnercontract.ExecutionInput{}, nil, errors.New("Runner did not return a local Codex configuration")
	}
	if result.Execution.Adapter != "codex-app-server" || result.Execution.Target.Kind != "local" || result.View.Model == nil || *result.View.Model != settings.Model || result.View.ReasoningEffort == nil || *result.View.ReasoningEffort != settings.ReasoningEffort || result.View.AccessMode != "read_only" {
		return runnercontract.ExecutionInput{}, nil, errors.New("主 Agent 本地 Codex/模型/强度/只读配置未核实")
	}
	return *result.Execution, p.config.runtimeCatalog, nil
}
func (p *mainAgentPort) context(ctx context.Context, in ma.Start) (ma.Context, error) {
	var out ma.Context
	c, err := p.store.Candidate(ctx, p.config.Identity(), in.CandidateID)
	if err != nil {
		return out, err
	}
	if c.Revision != in.CandidateRevision || c.Participation != "active" || c.StopRequested {
		return out, errors.New("stale_revision: 原候选已变化或停止参与")
	}
	graph, err := p.store.TaskGraphSnapshot(ctx, p.config.Identity())
	if err != nil {
		return out, err
	}
	task, ok := graph.ByID[c.TaskID]
	if !ok || task.Status != "in_progress" {
		return out, errors.New("原任务已结束，不接受新材料分析")
	}
	observation, err := p.config.runtimeMaintenance.check(ctx)
	if err != nil {
		return out, err
	}
	out = ma.Context{CandidateID: c.ID, CandidateRevision: c.Revision, TaskID: c.TaskID, TaskVersion: task.Version, Title: task.Title, ContractDigest: c.ContractDigest, OriginalBase: c.BaseRevision, RepositoryCommit: observation.Commit, SpecPath: c.Contract.SpecRef.Path, PlanPath: c.Contract.PlanRef.Path, RunID: c.RunID, Original: []ma.Document{}, Current: []ma.Document{}, CurrentMissing: map[string]string{}, TestPlan: c.TestPlan}
	if c.Projection != nil {
		out.RunnerRevision = c.Projection.Revision
		out.SourceAttemptID = c.Projection.Execution.AttemptID
		out.Question = c.Projection.Question
		if c.Projection.Final != nil && c.Projection.Final.Verified {
			out.Final = c.Projection.Final
		}
	}
	out.Inputs, _ = json.Marshal(c.Actions)
	paths := []string{out.SpecPath, out.PlanPath}
	for _, ref := range append(c.Contract.CommonRefs, c.Contract.AcceptanceRefs...) {
		paths = append(paths, ref.Path)
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		body, e := p.config.runtimeMaintenance.read(ctx, c.BaseRevision, path)
		if e != nil {
			return out, fmt.Errorf("原候选材料 %s: %w", path, e)
		}
		if len(body) > ma.MaxBytes || !utf8.Valid(body) {
			return out, errors.New("原材料超过分析容量")
		}
		out.Original = append(out.Original, ma.Document{Path: path, Commit: c.BaseRevision, SHA256: ma.Digest(body), Body: string(body)})
		current, e := p.config.runtimeMaintenance.read(ctx, observation.Commit, path)
		if e != nil {
			var missing *missingCommittedFileError
			if errors.As(e, &missing) {
				if out.CurrentMissing == nil {
					out.CurrentMissing = map[string]string{}
				}
				out.CurrentMissing[path] = e.Error()
				continue
			}
			return out, fmt.Errorf("当前材料 %s: %w", path, e)
		}
		if len(current) > ma.MaxBytes || !utf8.Valid(current) {
			return out, errors.New("当前材料不是完整有效的 UTF-8 正文")
		}
		out.Current = append(out.Current, ma.Document{Path: path, Commit: observation.Commit, SHA256: ma.Digest(current), Body: string(current)})
	}
	return out, nil
}

type mainAgentOptions struct {
	Commit     string `json:"commit"`
	RulesReady bool   `json:"rules_ready"`
	Diagnostic string `json:"diagnostic"`
	Model      string `json:"model"`
	Effort     string `json:"reasoning_effort"`
}

func (p *mainAgentPort) options(ctx context.Context) mainAgentOptions {
	settings, _ := executionconfig.MainAgentSettings(p.config.RunnerExecution.MainAgent)
	out := mainAgentOptions{Model: settings.Model, Effort: settings.ReasoningEffort}
	head, err := p.config.runtimeMaintenance.git(ctx, "rev-parse", "HEAD^{commit}")
	if err != nil {
		out.Diagnostic = err.Error()
		return out
	}
	out.Commit = strings.TrimSpace(string(head))
	body, err := p.config.runtimeMaintenance.read(ctx, out.Commit, ma.RulesPath)
	expected, e := prompts.MainAgentRules()
	if err != nil || e != nil || !prompts.SameRules(body, expected) {
		out.Diagnostic = "请准备并审阅主 Agent 规则，再提交并合入项目目标分支"
		return out
	}
	out.RulesReady = true
	return out
}
func (p *mainAgentPort) read(ctx context.Context, route string, values url.Values) (any, error) {
	if route == "/main-agent/options" {
		if len(values) > 0 {
			return nil, errors.New("invalid main Agent options query")
		}
		return p.options(ctx), nil
	}
	if route == "/main-agent" {
		if len(values) != 1 || len(values["candidate_id"]) != 1 || !ma.ID.MatchString(values.Get("candidate_id")) {
			return nil, errors.New("exact candidate_id required")
		}
		rows, more, err := p.store.AnalysisSummaries(ctx, p.config.Identity(), values.Get("candidate_id"))
		if err != nil {
			return nil, err
		}
		return struct {
			Records []ma.Summary `json:"records"`
			HasMore bool         `json:"has_more"`
		}{rows, more}, nil
	}
	if len(values) > 0 {
		return nil, errors.New("analysis query does not accept parameters")
	}
	id := strings.TrimPrefix(route, "/main-agent/")
	if !ma.ID.MatchString(id) {
		return nil, errors.New("invalid analysis id")
	}
	r, err := p.store.Analysis(ctx, p.config.Identity(), id)
	if err != nil {
		return nil, err
	}
	return struct {
		ma.Record
		Review string `json:"review"`
	}{r, ma.Review(r)}, nil
}
