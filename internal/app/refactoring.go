package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/billing"
	"github.com/erranliu/SteerLine/internal/billingconfig"
	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
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
	p.service = &refactoring.Service{Project: c.Identity(), Store: store, Runner: cli, Admit: admit, Prepare: p.prepare, Qualify: p.qualifyFirst, Verify: p.verify, VerifyConflictBase: p.verifyConflictBase, ReviewDiff: p.reviewDiff, ReadCost: p.cost, Queue: c.runtimeGenerations, InvestigationPath: investigationPath, Investigate: p.investigation, ReleaseInvestigation: releaseInvestigation}
	p.service.Reserve = func(ctx context.Context, commit string, save func() error) error {
		return reserver.reserveReadyExecution(ctx, commit, func() error {
			return c.reserveCurrentProfiles(ctx, save)
		})
	}
	return p
}

func (p *refactoringPort) remoteHead(ctx context.Context) (string, error) {
	return p.remoteBranchHead(ctx, p.config.Remote, p.config.Branch)
}

func (p *refactoringPort) remoteBranchHead(ctx context.Context, remote, branch string) (string, error) {
	refName := "refs/heads/" + branch
	raw, err := p.config.runtimeMaintenance.git(ctx, "--no-optional-locks", "ls-remote", "--exit-code", "--", remote, refName)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 2 || !ref.ValidRevision(fields[0]) || fields[1] != refName {
		return "", errors.New("远端目标分支提交无法核实")
	}
	return fields[0], nil
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
	remoteHead, e := p.remoteHead(ctx)
	if e != nil {
		return out, e
	}
	if remoteHead != observation.Commit || in.BaseCommit != observation.Commit {
		return out, errors.New("远端HEAD已变化，请先完成项目维护检查与就绪")
	}
	ready, e := m.projectReadiness(ctx)
	if e != nil {
		return out, e
	}
	if e = requireProjectReadiness(ready); e != nil {
		return out, e
	}
	profile, e := c.executionConfig(ctx).Profile(in.Profile, "")
	if e != nil {
		return out, e
	}
	if !supportsRefactoringProfile(profile) || in.ProfileVersion != refactoringProfileVersion(profile) {
		return out, ref.ErrEnvironmentChanged
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
	if !c.environmentPolicy.RequireReadiness() || c.runtimeEnvironment == nil {
		return out, errors.New("开始维护需要已就绪的项目运行环境")
	}
	proof, _, e := c.runtimeEnvironment.qualifyEnvironment(ctx, in.Profile, observation.Commit, func(ctx context.Context) (string, error) {
		commit, err := c.runtimeEnvironment.observedSource(ctx, false)
		if err == nil && commit != observation.Commit {
			err = ref.ErrEnvironmentChanged
		}
		return commit, err
	})
	if e != nil {
		return out, e
	}
	if proof.Key != in.EnvironmentKey || proof.Revision != in.EnvironmentRevision {
		return out, ref.ErrEnvironmentChanged
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
	out.Environment, out.EnvironmentName, out.ProfileName = &proof, "本机 · Codex 直连", profile.Name
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
	info, e := client.Query(ctx, c.PR, result.PRURL())
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
	if err := p.fetchPRVersion(ctx, r, pr); err != nil {
		return "", err
	}
	diff, err := p.config.repositoryGit(ctx, "--no-replace-objects", "diff", "--patch", "--no-ext-diff", "--no-textconv", "--no-color", "--find-renames", "--ignore-submodules=none", pr.BaseRevision+"..."+pr.HeadRevision, "--")
	if len(diff) > 512<<10 || !utf8.Valid(diff) {
		return "", errors.New("PR 差异超出评审材料限额或编码不可读")
	}
	return string(diff), err
}

func (p *refactoringPort) verifyConflictBase(ctx context.Context, r ref.Record, pr runnercontract.PRFact) error {
	if err := p.fetchPRVersion(ctx, r, pr); err != nil {
		return err
	}
	_, err := p.config.repositoryGit(ctx, "--no-replace-objects", "merge-base", "--is-ancestor", pr.BaseRevision, pr.HeadRevision)
	if err != nil {
		return fmt.Errorf("原 PR 的交付提交未核实包含当前目标基线 %s: %w", pr.BaseRevision, err)
	}
	return nil
}

func (p *refactoringPort) fetchPRVersion(ctx context.Context, r ref.Record, pr runnercontract.PRFact) error {
	c := p.config
	remote, err := c.repositoryGit(ctx, "remote", "get-url", r.Frozen.Repository.Remote)
	if err != nil || strings.TrimSpace(string(remote)) != r.Frozen.Repository.URL {
		return errors.New("PR 核验仓库与冻结来源不一致")
	}
	// Fetch only pinned objects. No ref, index or execution worktree is used as evidence.
	_, err = c.repositoryGit(ctx, "fetch", "--no-tags", "--no-write-fetch-head", "--no-recurse-submodules", "--no-auto-maintenance", "--refmap=", "--", r.Frozen.Repository.Remote, pr.BaseRevision, pr.HeadRevision)
	return err
}

func supportsRefactoringProfile(profile RunnerProfile) bool {
	return profile.Execution.Adapter == "codex-app-server" && profile.Execution.Target.Kind == "local" &&
		(profile.Workflow.ID == "development" || profile.Workflow.ID == "development-audited")
}

func refactoringProfileVersion(profile RunnerProfile) string {
	profile.Name = ""
	return runnercontract.ConfigurationValueDigest(profile)
}

func (p *refactoringPort) qualifyFirst(ctx context.Context, r ref.Record) (env.Admission, error) {
	c := p.config
	ctx = c.withProfiles(ctx)
	profile, err := c.executionConfig(ctx).Profile(r.Input.Profile, "")
	if err != nil || !supportsRefactoringProfile(profile) || refactoringProfileVersion(profile) != r.Input.ProfileVersion {
		return env.Admission{}, fmt.Errorf("%w：原执行方案已变化", ref.ErrEnvironmentChanged)
	}
	if c.runtimeEnvironment == nil || !c.environmentPolicy.RequireReadiness() {
		return env.Admission{}, ref.ErrEnvironmentChanged
	}
	proof, _, err := c.runtimeEnvironment.qualifyEnvironment(ctx, r.Input.Profile, r.Frozen.Repository.BaseRevision, func(ctx context.Context) (string, error) {
		remote, err := p.remoteHead(ctx)
		if err != nil {
			return "", err
		}
		observed, err := c.runtimeEnvironment.observedSource(ctx, false)
		if err != nil {
			return "", err
		}
		if remote != observed {
			current, err := c.runtimeMaintenance.check(ctx)
			if err != nil {
				return "", err
			}
			observed = current.Commit
		}
		if observed != r.Frozen.Repository.BaseRevision && c.runtimeMaintenance.current().Rewritten {
			return "", fmt.Errorf("%w：目标分支历史已改写", ref.ErrEnvironmentChanged)
		}
		return observed, nil
	})
	if err != nil {
		var invalid *env.QualificationError
		if errors.As(err, &invalid) || confirmedEnvironmentBasisError(err) {
			return proof, fmt.Errorf("%w：%v", ref.ErrEnvironmentChanged, err)
		}
		return proof, err
	}
	if proof.Key != r.Input.EnvironmentKey {
		return proof, ref.ErrEnvironmentChanged
	}
	return proof, nil
}

type refactoringProfileView struct {
	environmentProfileView
	Version string `json:"version"`
}

type refactoringEnvironmentView struct {
	Key      string                         `json:"environment_key"`
	Name     string                         `json:"name"`
	Target   runnercontract.ExecutionTarget `json:"target"`
	Agent    string                         `json:"agent"`
	Status   string                         `json:"status"`
	Reason   string                         `json:"reason,omitempty"`
	Revision uint64                         `json:"current_revision"`
	Profiles []refactoringProfileView       `json:"profiles"`
}

func (p *refactoringPort) readiness(ctx context.Context) (any, error) {
	c := p.config
	ctx = c.withProfiles(ctx)
	current := c.executionConfig(ctx)
	if c.runtimeMaintenance == nil {
		return nil, errors.New("项目维护状态不可用")
	}
	project, err := c.runtimeMaintenance.projectReadiness(ctx)
	if err != nil {
		return nil, err
	}
	items := []refactoringEnvironmentView{}
	out := struct {
		Branch         string                       `json:"branch"`
		Project        pm.Readiness                 `json:"project"`
		Environments   []refactoringEnvironmentView `json:"environments"`
		DefaultProfile string                       `json:"default_profile"`
		SteerAgent     executionconfig.SteerAgent   `json:"steer_agent"`
		Reason         string                       `json:"reason,omitempty"`
	}{Branch: c.Branch, Project: project, Environments: items, DefaultProfile: current.DefaultProfile}
	if !c.environmentPolicy.RequireReadiness() {
		out.Reason = "开始维护需要已就绪的运行环境，请开启项目环境就绪检查"
		return out, nil
	}
	if c.runtimeEnvironment == nil || c.runtimeNative == nil {
		out.Reason = "项目运行环境状态待核实"
		return out, nil
	}
	agent, err := executionconfig.SteerAgentSettings(c.RunnerExecution.SteerAgent)
	if err != nil {
		out.Reason = err.Error()
		return out, nil
	}
	out.SteerAgent = agent
	read, readErr := c.runtimeNative.Lookup(runnercontract.ExecutionConfigurationRequest{
		Operation: "compose", BaseExecution: executionconfig.SteerAgentBase(c.RunnerExecution.Profiles),
		Workflow: runnercontract.Workflow{ID: "generate", Version: 1},
		Selection: &runnercontract.ExecutionSelection{
			Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Agent: "codex",
			Model: &agent.Model, ReasoningEffort: &agent.ReasoningEffort, AccessMode: "read_only",
		},
	})
	ctx = materialReads(ctx)
	for _, item := range c.runtimeEnvironment.configurationOptions(ctx).Environments {
		v := refactoringEnvironmentView{Key: item.Key, Name: item.Name, Target: item.Target, Agent: item.Agent, Status: "unverified", Profiles: []refactoringProfileView{}}
		proof, _, qualifyErr := c.runtimeEnvironment.qualifyEnvironment(ctx, item.Profiles[0].ID, "", func(context.Context) (string, error) {
			if project.Commit == "" {
				return "", errors.New("项目基线待核实")
			}
			return project.Commit, nil
		})
		if qualifyErr == nil {
			v.Status, v.Revision = "ready", proof.Revision
		} else {
			v.Reason = qualifyErr.Error()
			var invalid *env.QualificationError
			if errors.As(qualifyErr, &invalid) || confirmedEnvironmentBasisError(qualifyErr) {
				v.Status = "not_ready"
			}
		}
		for _, candidate := range item.Profiles {
			profile, err := current.Profile(candidate.ID, "")
			if err != nil || !supportsRefactoringProfile(profile) {
				continue
			}
			write, err := c.runtimeNative.Lookup(runnercontract.ExecutionConfigurationRequest{Operation: "derive", Execution: &profile.Execution, Workflow: runnercontract.Workflow{ID: "execute", Version: 1}, AccessMode: "full_access"})
			if err != nil || write.Execution == nil {
				continue
			}
			if readErr != nil || read.Execution == nil || c.runtimeNative.Catalog.ValidateMaintenance(*write.Execution, *read.Execution) != nil {
				continue
			}
			v.Name = "本机 · Codex 直连"
			v.Profiles = append(v.Profiles, refactoringProfileView{environmentProfileView: candidate, Version: refactoringProfileVersion(profile)})
		}
		if v.Status == "ready" && len(v.Profiles) == 0 {
			v.Status, v.Reason = "unsupported", "运行环境已就绪，当前执行组合不支持维护"
		}
		out.Environments = append(out.Environments, v)
	}
	if c.runtimeMaintenance.current().Commit != project.Commit {
		return nil, errors.New("项目基线观察已变化，请重新核实")
	}
	return out, nil
}

func (p *refactoringPort) read(ctx context.Context, route string, values url.Values) (any, error) {
	if route == "/refactoring/readiness" && len(values) == 0 {
		return p.readiness(ctx)
	}
	if route == "/refactoring" && (len(values) == 0 || len(values) == 1 && len(values["history_before"]) == 1) {
		return p.list(ctx, values.Get("history_before"))
	}
	if len(values) > 0 {
		return nil, errors.New("维护查询不接受额外参数")
	}
	if route == "/refactoring/directions" {
		return refactoring.ListDirections(HomeDir()), nil
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
		v["report_stale"] = report.Head != r.PR.HeadRevision || report.Base != r.PR.BaseRevision || r.PR.Merged || r.PR.State != "open" || r.Phase != "delivered"
	}
	return v, nil
}
func (p *refactoringPort) list(ctx context.Context, before string) (any, error) {
	rows, err := p.store.Refactorings(ctx, p.config.Identity())
	if err != nil {
		return nil, err
	}
	type result struct {
		Ongoing     []map[string]any `json:"ongoing"`
		History     []map[string]any `json:"history"`
		HistoryNext string           `json:"history_next,omitempty"`
		Used        int              `json:"slots_used"`
		Limit       int              `json:"slot_limit"`
	}
	out := result{Ongoing: []map[string]any{}, History: []map[string]any{}, Limit: ref.Concurrency}
	var cursorTime time.Time
	cursorTimeText, cursorID, _ := strings.Cut(before, "/")
	if before != "" {
		cursorTime, err = time.Parse(time.RFC3339Nano, cursorTimeText)
		if err != nil || !ref.ValidID(cursorID) {
			return nil, errors.New("无效维护历史游标")
		}
	}
	ended := func(r ref.Record) time.Time {
		if r.FinishedAt != nil {
			return *r.FinishedAt
		}
		return r.UpdatedAt
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := ended(rows[i]), ended(rows[j])
		if a.Equal(b) {
			return rows[i].ID > rows[j].ID
		}
		return a.After(b)
	})
	const historySize = 20
	now := time.Now().UTC()
	lastCursor := ""
	for _, r := range rows {
		if r.Slot {
			out.Used++
		}
		if r.Active() {
			if before == "" {
				out.Ongoing = append(out.Ongoing, query.RefactoringSummary(r, now))
			}
			continue
		}
		end := ended(r)
		if before != "" && (end.After(cursorTime) || end.Equal(cursorTime) && r.ID >= cursorID) {
			continue
		}
		if len(out.History) >= historySize {
			out.HistoryNext = lastCursor
			continue
		}
		out.History = append(out.History, query.RefactoringSummary(r, now))
		lastCursor = end.Format(time.RFC3339Nano) + "/" + r.ID
	}
	return out, nil
}

func (p *refactoringPort) run(ctx context.Context) { p.service.Run(ctx) }
