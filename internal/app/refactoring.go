package app

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/billing"
	"github.com/erranliu/SteerLine/internal/billingconfig"
	"github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/product/query"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/refactoring"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/testmanagement/model"
)

type refactoringPort struct {
	config    Config
	store     *sqlite.SQLiteStore
	service   *refactoring.Service
	materials committedMaterialReader
}

func newRefactoringPort(c Config, store *sqlite.SQLiteStore, cli runnerclient.Client, admit func(context.Context) error, materials committedMaterialReader, reserver readyExecutionReserver) *refactoringPort {
	p := &refactoringPort{config: c, store: store, materials: materials}
	p.service = &refactoring.Service{Project: c.Identity(), Store: store, Runner: cli, Admit: admit, Prepare: p.prepare, Verify: p.verify, ReviewDiff: p.reviewDiff, ReadCost: p.cost, Queue: c.runtimeGenerations, Reserve: reserver.reserveReadyExecution, InvestigationPath: investigationPath, Investigate: p.investigation, ReleaseInvestigation: releaseInvestigation}
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
	entryBody, e := p.materials.readCommittedMaterial(ctx, observation.Commit, "SteerlineFlow/environment-project.json")
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
		if _, e = p.materials.readCommittedMaterial(ctx, observation.Commit, path); e != nil {
			return out, e
		}
	}
	for phase, destination := range map[string]*string{"first": &out.FirstRules, "followup": &out.FollowRules, "report": &out.ReportRules} {
		rules, err := prompts.MaintenanceRules(phase)
		if err != nil {
			return out, err
		}
		*destination = string(rules)
	}
	strategy, e := prompts.MaintenanceStrategy(in.Strategy)
	if e != nil {
		return out, e
	}
	out.Strategy = string(strategy)
	cli, e := c.runnerClient(ctx)
	if e != nil {
		return out, e
	}
	out.Direction = direction
	out.Repository = runnercontract.Repository{URL: remoteURL, LocalPath: c.Repository, Remote: c.Remote, BaseBranch: c.Branch, BaseRevision: observation.Commit}
	out.Documents = runnercontract.Documents{Spec: runnercontract.Document{Path: requirementsPath}, Plan: runnercontract.Document{Path: entry.TestingCatalogue}}
	out.Write, out.Read, out.Runner = *write.Execution, read, cli.BuildPin()
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

func investigationPath(id string) (string, error) {
	if !ref.ValidID(id) {
		return "", errors.New("首轮调查目录身份无效")
	}
	return filepath.Join(HomeDir(), "maintenance-investigations", id), nil
}

func releaseInvestigation(id string) error {
	dir, err := investigationPath(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (p *refactoringPort) investigation(ctx context.Context, r ref.Record, id string) error {
	dir, err := investigationPath(id)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return err
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		return err
	}
	source := filepath.ToSlash(r.Frozen.Repository.LocalPath)
	if !strings.HasPrefix(source, "/") {
		source = "/" + source
	}
	local := (&url.URL{Scheme: "file", Path: source}).String()
	commit := r.Frozen.Repository.BaseRevision
	git := commandRunner{Dir: dir, ProcessHost: p.config.ProcessHost, Timeout: positiveDuration(p.config.CommandTimeout, 30*time.Second)}
	for _, args := range [][]string{
		{"init", "--quiet", "--template="},
		{"fetch", "--quiet", "--no-tags", "--depth=1", local, commit},
		{"-c", "core.hooksPath=" + filepath.Join(dir, ".git", "disabled-hooks"), "checkout", "--quiet", "--detach", commit},
	} {
		if _, err = git.Output(ctx, backend.Command{Executable: "git"}, args...); err != nil {
			return errors.Join(err, releaseInvestigation(id))
		}
	}
	return nil
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
func (p *refactoringPort) verify(ctx context.Context, r ref.Record, result ref.Result) (*runnercontract.PRFact, string, error) {
	c := p.config
	client := publication.QueryClient{Command: commandRunner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second), ProcessHost: c.ProcessHost}, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}
	info, e := client.Query(ctx, c.PR, result.PR)
	if e != nil {
		return nil, "", e
	}
	branch := ""
	for _, o := range r.Operations {
		if o.Run != nil && o.Run.Execution.Branch != "" {
			branch = o.Run.Execution.Branch
			break
		}
	}
	if c.PR.Repository != r.Frozen.Publication["repository"] || c.PR.BaseURL != r.Frozen.Publication["base_url"] || info.BaseBranch != r.Frozen.Repository.BaseBranch || info.Repository != c.PR.Repository || info.HeadRepository != c.PR.Repository || branch == "" || info.HeadBranch != branch || r.PR != nil && info.URL != r.PR.URL {
		return nil, "", errors.New("PR目标、原分支或报告提交不匹配")
	}
	if info.BaseRevision == "" && r.Frozen.Repository.URL != "" && c.ProcessHost != nil {
		refName := "refs/heads/" + info.BaseBranch
		remote, remoteErr := c.repositoryGit(ctx, "remote", "get-url", r.Frozen.Repository.Remote)
		if remoteErr != nil || strings.TrimSpace(string(remote)) != r.Frozen.Repository.URL {
			return nil, "", errors.New("PR 目标分支与冻结远端不匹配")
		}
		out, gitErr := c.repositoryGit(ctx, "ls-remote", "--exit-code", "--", r.Frozen.Repository.Remote, refName)
		fields := strings.Fields(string(out))
		if gitErr == nil && len(fields) == 2 && fields[1] == refName {
			info.BaseRevision = fields[0]
		}
	}
	if !ref.ValidRevision(info.HeadRevision) || !ref.ValidRevision(info.BaseRevision) {
		return nil, "", errors.New("原PR平台未返回可信的head/base提交")
	}
	fact := &runnercontract.PRFact{URL: info.URL, Provider: c.PR.Kind, Repository: info.Repository, HeadRepository: info.HeadRepository, Number: func() int { n, _ := c.PR.PRNumber(info.URL); return n }(), BaseBranch: info.BaseBranch, BaseRevision: info.BaseRevision, HeadBranch: info.HeadBranch, HeadRevision: info.HeadRevision, Draft: info.Draft, State: info.State, Merged: info.Merged, CheckedAt: time.Now().UTC()}
	return fact, info.Checks, nil
}

func (p *refactoringPort) reviewDiff(ctx context.Context, r ref.Record, pr runnercontract.PRFact) (string, error) {
	c := p.config
	remote, err := c.repositoryGit(ctx, "remote", "get-url", r.Frozen.Repository.Remote)
	if err != nil || strings.TrimSpace(string(remote)) != r.Frozen.Repository.URL {
		return "", errors.New("PR 评审仓库与冻结来源不一致")
	}
	// Fetch only pinned objects. No ref, index or execution worktree is used as evidence.
	if _, err := c.repositoryGit(ctx, "fetch", "--no-tags", "--no-write-fetch-head", "--no-recurse-submodules", "--no-auto-maintenance", "--refmap=", "--", r.Frozen.Repository.Remote, pr.BaseRevision, pr.HeadRevision); err != nil {
		return "", err
	}
	diff, err := c.repositoryGit(ctx, "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--find-renames", "--ignore-submodules=none", pr.BaseRevision+"..."+pr.HeadRevision, "--")
	if len(diff) > 512<<10 || !utf8.Valid(diff) {
		return "", errors.New("PR 差异超出评审材料限额或编码不可读")
	}
	return string(diff), err
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
	if len(r.Reports) > 0 && r.PR != nil {
		report := r.Reports[len(r.Reports)-1]
		v["report_stale"] = report.Head != r.PR.HeadRevision || report.Base != r.PR.BaseRevision || r.PR.Merged || r.PR.State != "open"
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
