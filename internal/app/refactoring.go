package app

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/billing"
	"github.com/erranliu/SteerLine/internal/billingconfig"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/product/query"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/refactoring"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/steeragent/maintenance"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/testmanagement/model"
)

type refactoringPort struct {
	config  Config
	store   *sqlite.SQLiteStore
	service *refactoring.Service
}

func newRefactoringPort(c Config, store *sqlite.SQLiteStore, cli runnerclient.Client, admit func(context.Context) error) *refactoringPort {
	p := &refactoringPort{config: c, store: store}
	p.service = &refactoring.Service{Project: c.Identity(), Store: store, Runner: cli, Admit: admit, Prepare: p.prepare, Verify: p.verify, ReadCost: p.cost, Queue: c.runtimeGenerations}
	p.service.Reserve = func(ctx context.Context, commit string, save func() error) error {
		return c.runtimeMaintenance.reserveReadyExecution(ctx, commit, save)
	}
	return p
}
func (p *refactoringPort) prepare(ctx context.Context, in ref.Start) (ref.Frozen, error) {
	var out ref.Frozen
	direction, e := refactoring.ResolveDirection(HomeDir(), in.Direction, in.DirectionRevision)
	if e != nil {
		return out, e
	}
	c := p.config
	m := c.runtimeMaintenance
	if m == nil || c.runtimeNative == nil {
		return out, errors.New("项目或Runner尚未就绪")
	}
	observation := m.current()
	remoteHead, e := m.git(ctx, "--no-optional-locks", "ls-remote", "--exit-code", c.Remote, "refs/heads/"+c.Branch)
	if e != nil {
		return out, e
	}
	remoteFields := strings.Fields(string(remoteHead))
	if len(remoteFields) != 2 || remoteFields[0] != observation.Commit {
		return out, errors.New("远端HEAD已变化，请先完成项目维护检查与就绪")
	}
	ready, e := m.projectReadiness(ctx)
	if e != nil {
		return out, e
	}
	if e = requireProjectReadiness(ready); e != nil {
		return out, e
	}
	profile, e := c.RunnerExecution.Profile(in.Profile, "")
	if e != nil {
		return out, e
	}
	write, e := c.deriveProfile(ctx, profile, "execute", "full_access")
	if e != nil || write.Execution == nil {
		if e == nil {
			e = errors.New("写入执行配置缺失")
		}
		return out, e
	}
	read, _, e := c.prepareSteerAgentGeneration(ctx)
	if e != nil {
		return out, e
	}
	if e = c.runtimeNative.Catalog.ValidateMaintenance(*write.Execution, read); e != nil {
		return out, e
	}
	if c.environmentPolicy.RequireReadiness() {
		if c.runtimeEnvironment == nil {
			return out, errors.New("项目运行环境未就绪")
		}
		if _, _, e = c.runtimeEnvironment.admission(ctx, in.Profile, observation.Commit, true); e != nil {
			return out, e
		}
	}
	remote, e := m.git(ctx, "remote", "get-url", c.Remote)
	if e != nil {
		return out, e
	}
	remoteURL := strings.TrimSpace(string(remote))
	if e = c.PR.ValidateRemote(remoteURL); e != nil {
		return out, e
	}
	// These are real existing references, not generated maintenance Spec/Plan.
	var entry model.ProjectEntry
	entryBody, e := m.read(ctx, observation.Commit, "SteerlineFlow/environment-project.json")
	if e != nil {
		return out, e
	}
	entry, e = model.ParseProjectEntry(entryBody)
	if e != nil {
		return out, e
	}
	if len(entry.Requirements) == 0 || entry.TestingCatalogue == entry.Requirements[0] {
		return out, errors.New("项目缺少真实测试要求参考路径")
	}
	requirementsPath := entry.Requirements[0]
	for _, path := range []string{requirementsPath, entry.TestingCatalogue} {
		if _, e = m.read(ctx, observation.Commit, path); e != nil {
			return out, e
		}
	}
	rules, e := maintenance.Rules()
	if e != nil {
		return out, e
	}
	cli, e := c.runnerClient(ctx)
	if e != nil {
		return out, e
	}
	out = ref.Frozen{Direction: direction, Repository: runnercontract.Repository{URL: remoteURL, LocalPath: c.Repository, Remote: c.Remote, BaseBranch: c.Branch, BaseRevision: observation.Commit}, Documents: runnercontract.Documents{Spec: runnercontract.Document{Path: requirementsPath}, Plan: runnercontract.Document{Path: entry.TestingCatalogue}}, Write: *write.Execution, Read: read, Runner: cli.BuildPin(), Rules: string(rules)}
	out.Publication = map[string]string{"provider": c.PR.Kind, "base_url": c.PR.BaseURL, "repository": c.PR.Repository, "base_branch": c.Branch, "token_env": c.PR.TokenEnv}
	settings, priceErr := (billingconfig.Store{Home: HomeDir()}).Read()
	if priceErr == nil {
		policy, err := settings.ActivePolicy()
		if err == nil {
			out.Pricing = &policy
		}
	}
	if in.Budget.Dollars != "" {
		if out.Pricing == nil || !cli.RunUsageSupported || !cli.ConfiguredUsageSupported {
			return out, errors.New("美元预算需要明确模型、有效价表和Runner用量采集")
		}
		if out.Pricing.Currency != "USD" {
			return out, errors.New("美元预算需要USD价表")
		}
		for _, execution := range []runnercontract.ExecutionInput{out.Write, out.Read} {
			var selected struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal(execution.Settings, &selected); err != nil || selected.Model == "" {
				return out, errors.New("美元预算需要明确 SteerAgent 和 Runner 的模型")
			}
			one, zero := int64(1), int64(0)
			estimate, err := billing.Calculate(billing.Usage{RunID: "preflight", Scope: "main_session", Revision: 1, Status: "available", Finalized: true, CoverageComplete: true, PriceAt: time.Now().UTC(), Records: []billing.Record{{ID: "preflight", Model: selected.Model, Tokens: billing.Tokens{InputUncached: &one, CacheRead: &zero, CacheWrite: &zero, Output: &one}}}}, *out.Pricing)
			if err != nil || estimate.Total == nil {
				return out, errors.New("明确模型在当前价表中没有唯一完整价格")
			}
		}
	}
	return out, nil
}
func (p *refactoringPort) cost(ctx context.Context, r ref.Record, o ref.Operation) (ref.Cost, error) {
	out := ref.Cost{ObservedAt: time.Now().UTC(), Untracked: []string{"subagents", "tool_internal_models", "unobserved_provider_calls"}}
	cli, e := p.config.runnerClient(ctx)
	if e != nil {
		return out, e
	}
	u, e := cli.RunUsage(ctx, o.RunID)
	if e != nil {
		return out, e
	}
	events, e := cli.RunUsageEvents(ctx, u)
	if e != nil {
		return out, e
	}
	out.Snapshot = u.Revision
	if r.Frozen.Pricing == nil {
		return out, errors.New("行动未固定价表")
	}
	sum := new(big.Rat)
	out.Complete = true
	known := false
	for _, scope := range []string{"main_session", "runner_internal"} {
		usage := billing.Usage{RunID: o.RunID, Scope: scope, Revision: u.Revision, Status: u.Status, Finalized: u.Coverage.Finalized, PriceAt: r.CreatedAt, CoverageComplete: u.Coverage.ExpectedSources != nil && *u.Coverage.ExpectedSources == u.Coverage.VerifiedSources}
		if scope == "main_session" {
			usage.ZeroKnown = u.Tokens != nil && u.Tokens.Total != nil && *u.Tokens.Total == 0
		} else {
			if u.ModelCallCoverage == nil {
				out.Complete = false
				out.Diagnostic = "内部调用覆盖尚不可核实"
				continue
			}
			m := u.ModelCallCoverage
			usage.Status = "available"
			usage.Finalized = m.InternalPending == 0
			usage.CoverageComplete = m.InternalUncovered == 0 && m.InternalPending == 0
			usage.ZeroKnown = m.InternalRecorded == 0
			out.Untracked = m.Untracked
		}
		for _, event := range events {
			eventScope := event.Scope
			if eventScope == "" {
				eventScope = "main_session"
			}
			if eventScope == scope && event.Decision == "counted" {
				usage.Records = append(usage.Records, billing.Record{ID: event.ID, StepID: event.StepID, Model: event.ConfiguredModel, ReasoningEffort: event.ConfiguredReasoningEffort, Tokens: billing.Tokens(event.Tokens)})
			}
		}
		estimate, err := billing.Calculate(usage, *r.Frozen.Pricing)
		if err != nil {
			return out, err
		}
		if estimate.Covered != nil {
			n, _ := new(big.Rat).SetString(*estimate.Covered)
			sum.Add(sum, n)
			known = true
		}
		if estimate.Total == nil {
			out.Complete = false
			out.Diagnostic = strings.Join(estimate.Issues, "；")
		}
	}
	if known {
		amount := sum.FloatString(18)
		out.Amount = &amount
	}
	return out, nil
}
func (p *refactoringPort) verify(ctx context.Context, r ref.Record, result ref.Result) (*runnercontract.PRFact, error) {
	c := p.config
	client := publication.QueryClient{Command: commandRunner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second), ProcessHost: c.ProcessHost}, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}
	info, e := client.Query(ctx, c.PR, result.PR)
	if e != nil {
		return nil, e
	}
	branch := ""
	for _, o := range r.Operations {
		if o.Run != nil && o.Run.Execution.Branch != "" {
			branch = o.Run.Execution.Branch
			break
		}
	}
	if c.PR.Repository != r.Frozen.Publication["repository"] || c.PR.BaseURL != r.Frozen.Publication["base_url"] || info.BaseBranch != r.Frozen.Repository.BaseBranch || info.Repository != c.PR.Repository || info.HeadRepository != c.PR.Repository || info.HeadRevision != result.Head || branch == "" || info.HeadBranch != branch || r.PR != nil && info.URL != r.PR.URL {
		return nil, errors.New("PR目标、原分支或报告提交不匹配")
	}
	if info.Merged || !strings.EqualFold(info.State, "open") {
		return nil, errors.New("原PR已关闭或合并")
	}
	fact := &runnercontract.PRFact{URL: info.URL, Provider: c.PR.Kind, Repository: info.Repository, HeadRepository: info.HeadRepository, Number: func() int { n, _ := c.PR.PRNumber(info.URL); return n }(), BaseBranch: info.BaseBranch, HeadBranch: info.HeadBranch, HeadRevision: info.HeadRevision, State: info.State, Merged: info.Merged, CheckedAt: time.Now().UTC()}
	if result.Report != "" && info.Body != result.Report {
		return fact, errors.New("原PR正文与封存报告不一致，需更新原PR正文后重新审计")
	}
	return fact, nil
}

type refactoringReadiness struct {
	Branch      string       `json:"branch"`
	Project     pm.Readiness `json:"project"`
	Environment string       `json:"environment"`
	Reason      string       `json:"reason,omitempty"`
}

func supportsRefactoringProfile(profile RunnerProfile) bool {
	return profile.Execution.Adapter == "codex-app-server" && profile.Execution.Target.Kind == "local"
}

func (p *refactoringPort) readiness(ctx context.Context, id string) (refactoringReadiness, error) {
	c := p.config
	m := c.runtimeMaintenance
	if m == nil {
		return refactoringReadiness{}, errors.New("项目维护状态不可用")
	}
	project, err := m.projectReadiness(ctx)
	if err != nil {
		return refactoringReadiness{}, err
	}
	out := refactoringReadiness{Branch: c.Branch, Project: project, Environment: "unverified"}
	if id == "" {
		return out, nil
	}
	profile, err := c.RunnerExecution.Profile(id, "")
	if err != nil || !supportsRefactoringProfile(profile) {
		out.Environment, out.Reason = "not_ready", "所选执行配置不可用于本地 Codex 维护"
		return out, nil
	}
	if !c.environmentPolicy.RequireReadiness() {
		out.Environment = "bypassed"
		return out, nil
	}
	if c.runtimeEnvironment == nil || c.runtimeNative == nil || project.Commit == "" {
		out.Reason = "项目运行环境状态待核实"
		return out, nil
	}
	// Use the observed baseline, without starting a remote check or preparing an environment.
	source := func(ctx context.Context) (string, error) {
		commit, err := c.runtimeEnvironment.observedSource(ctx, false)
		if err == nil && commit != project.Commit {
			err = errors.New("项目基线已变化，请重新核实运行环境")
		}
		return commit, err
	}
	_, record, err := c.runtimeEnvironment.qualifyEnvironment(ctx, id, "", source)
	if err != nil {
		out.Environment, out.Reason = "not_ready", err.Error()
		if record.ID != "" && record.Status() != "ready" && record.DiagnosticText() != "" {
			out.Reason += " · " + record.DiagnosticText()
		}
		return out, nil
	}
	if _, err = source(ctx); err != nil {
		out.Reason = err.Error()
		return out, nil
	}
	out.Environment = "ready"
	return out, nil
}

func (p *refactoringPort) read(ctx context.Context, route string, values url.Values) (any, error) {
	if route == "/refactoring/readiness" {
		if len(values) == 0 || len(values) == 1 && len(values["profile"]) == 1 {
			return p.readiness(ctx, values.Get("profile"))
		}
		return nil, errors.New("维护就绪查询只接受一个执行配置")
	}
	if len(values) > 0 {
		return nil, errors.New("维护查询不接受额外参数")
	}
	if route == "/refactoring/directions" {
		return refactoring.ListDirections(HomeDir()), nil
	}
	if route == "/refactoring" {
		rows, e := p.store.Refactorings(ctx, p.config.Identity())
		if e != nil {
			return nil, e
		}
		views := []map[string]any{}
		for i, r := range rows {
			if i >= 50 {
				break
			}
			v := query.Refactoring(r, time.Now().UTC())
			delete(v, "operations")
			delete(v, "reports")
			delete(v, "direction_prompt")
			views = append(views, v)
		}
		profiles := []map[string]string{}
		for _, profile := range p.config.RunnerExecution.Profiles {
			if supportsRefactoringProfile(profile) {
				profiles = append(profiles, map[string]string{"id": profile.ID, "name": profile.Name})
			}
		}
		agent, err := executionconfig.SteerAgentSettings(p.config.RunnerExecution.SteerAgent)
		if err != nil {
			return nil, err
		}
		return map[string]any{"records": views, "profiles": profiles, "default_profile": p.config.RunnerExecution.DefaultProfile, "steer_agent": agent}, nil
	}
	id := strings.TrimPrefix(route, "/refactoring/")
	if !ref.ValidID(id) {
		return nil, errors.New("无效维护行动ID")
	}
	r, e := p.store.Refactoring(ctx, p.config.Identity(), id)
	if e != nil {
		return nil, e
	}
	v := query.Refactoring(r, time.Now().UTC())
	if len(r.Reports) > 0 && p.config.runtimeMaintenance != nil {
		base := p.config.runtimeMaintenance.current().Commit
		v["report_stale"] = base != "" && base != r.Reports[len(r.Reports)-1].Base
	}
	return v, nil
}
func (p *refactoringPort) run(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for ctx.Err() == nil {
		step, cancel := context.WithTimeout(ctx, 30*time.Second)
		_ = p.service.Tick(step)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
