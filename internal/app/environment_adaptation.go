package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation"
	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

type environmentPort struct {
	service *env.Service
	config  Config
	store   *sqlite.SQLiteStore
}

func newEnvironmentPort(c Config, store *sqlite.SQLiteStore, cli runnerclient.Client, admit func(context.Context) error) *environmentPort {
	p := &environmentPort{config: c, store: store}
	s := &env.Service{Project: c.Identity(), Store: store, Runner: cli, Build: cli.BuildPin(), Admit: admit}
	p.service = s
	s.Prepare = p.prepare
	s.PrepareGeneration = p.prepareGeneration
	if c.runtimeEnvironmentAdmission != nil {
		s.WithTarget = c.runtimeEnvironmentAdmission.adapt
	}
	if q := c.runtimeGenerations; q != nil {
		s.AcquireGeneration = func(r envmodel.Record) bool { return q.TryAcquire(r.Project, "environment:"+r.ID, r.CreatedAt) }
		s.ReleaseGeneration = func(r envmodel.Record) { q.Release(r.Project, "environment:"+r.ID) }
	}
	return p
}
func (p *environmentPort) git() commandRunner {
	return commandRunner{Dir: p.config.Repository, Timeout: positiveDuration(p.config.CommandTimeout, 30*time.Second), ProcessHost: p.config.ProcessHost}
}
func (p *environmentPort) source(ctx context.Context) (string, error) {
	c := p.config
	g := p.git()
	if _, e := g.Output(ctx, c.Git, "fetch", "--no-tags", "--no-write-fetch-head", "--", c.Remote, "+refs/heads/"+c.Branch+":refs/remotes/"+c.Remote+"/"+c.Branch); e != nil {
		return "", e
	}
	b, e := g.Output(ctx, c.Git, "rev-parse", "--verify", "refs/remotes/"+c.Remote+"/"+c.Branch+"^{commit}")
	return strings.TrimSpace(string(b)), e
}
func (p *environmentPort) read(ctx context.Context, commit, path string) ([]byte, error) {
	reader := taskDocuments{repository: p.config.Repository, git: p.config.Git, timeout: positiveDuration(p.config.CommandTimeout, 30*time.Second), processHost: p.config.ProcessHost}
	return reader.ReadCommittedRegularFile(ctx, taskmodel.FileReference{Path: path, Commit: commit, SHA256: strings.Repeat("0", 64)})
}
func (p *environmentPort) prepare(ctx context.Context, id string) (envmodel.Prepared, error) {
	return p.material(ctx, id, true)
}
func (p *environmentPort) material(ctx context.Context, id string, checkBase bool) (envmodel.Prepared, error) {
	return p.materialAt(ctx, id, "", checkBase)
}
func (p *environmentPort) materialAt(ctx context.Context, id, commit string, checkBase bool) (envmodel.Prepared, error) {
	var out envmodel.Prepared
	c := p.config
	profile, e := c.RunnerExecution.Profile(id, "")
	if e != nil {
		return out, e
	}
	key, e := envmodel.Key(profile.Execution)
	if e != nil {
		return out, e
	}
	if c.runtimeCatalog == nil {
		return out, errors.New("Runner能力目录不可用")
	}
	if e = c.runtimeCatalog.ValidateProgress(); e != nil {
		return out, e
	}
	if e = c.runtimeCatalog.ValidateResult(); e != nil {
		return out, e
	}
	if commit == "" {
		commit, e = p.source(ctx)
		if e != nil {
			return out, e
		}
	}
	document, e := p.read(ctx, commit, envmodel.DocumentPath(key))
	if e != nil {
		return out, fmt.Errorf("环境准备文档缺失：%w", e)
	}
	manifest, e := envmodel.ParseManifest(document)
	if e == nil && manifest.EnvironmentKey != key {
		e = errors.New("环境材料目标与所选环境不符")
	}
	if e != nil {
		return out, e
	}
	if e := manifest.Ready(); e != nil {
		return out, e
	}
	if strings.EqualFold(manifest.ValidationBase, c.Branch) {
		return out, errors.New("验证base不能是业务主分支")
	}
	g := p.git()
	if _, e = g.Output(ctx, c.Git, "check-ref-format", "refs/heads/"+manifest.ValidationBase); e != nil {
		return out, errors.New("验证base格式无效")
	}
	if checkBase {
		if _, e = g.Output(ctx, c.Git, "fetch", "--no-tags", "--no-write-fetch-head", "--", c.Remote, "+refs/heads/"+manifest.ValidationBase+":refs/remotes/"+c.Remote+"/"+manifest.ValidationBase); e != nil {
			return out, errors.New("项目须预先准备固定验证base；适配不会自动创建")
		}
		if _, e = g.Output(ctx, c.Git, "merge-base", "--is-ancestor", commit, "refs/remotes/"+c.Remote+"/"+manifest.ValidationBase); e != nil {
			return out, errors.New("固定验证base尚未包含本次项目源码，请按正常Git流程更新")
		}
	}
	script, e := p.read(ctx, commit, manifest.Script)
	if e != nil {
		return out, e
	}
	entry, e := p.projectEntryAt(ctx, commit)
	if e != nil {
		return out, e
	}
	catalogueRaw, e := p.read(ctx, commit, entry.TestingCatalogue)
	if e != nil {
		return out, e
	}
	catalogue, e := testmanagement.ParseCatalogue(catalogueRaw)
	if e != nil {
		return out, e
	}
	definitions := []testmanagement.Definition{}
	ids := []string{}
	for _, d := range catalogue.Tests {
		if d.Level == "L2" {
			definitions = append(definitions, d)
			ids = append(ids, d.ID)
		}
	}
	if len(ids) == 0 {
		return out, errors.New("项目未登记可执行L2，不能判定环境通过")
	}
	sort.Strings(ids)
	executionRules, e := p.read(ctx, commit, envmodel.ExecutionRulesPath)
	if e != nil {
		return out, fmt.Errorf("材料准备不足：环境执行规则不可读：%w", e)
	}
	expectedRules, e := prompts.EnvironmentExecutionRules()
	if e != nil {
		return out, e
	}
	if !prompts.SameRules(executionRules, expectedRules) {
		return out, errors.New("材料准备不足：环境执行规则不同源")
	}
	files := map[string]string{envmodel.ExecutionRulesPath: envmodel.Digest(executionRules), envmodel.DocumentPath(key): envmodel.Digest(document), manifest.Script: envmodel.Digest(script)}
	deps := append(append([]string{}, entry.Requirements...), manifest.Dependencies...)
	for _, path := range deps {
		b, e := p.read(ctx, commit, path)
		if e != nil {
			return out, fmt.Errorf("环境依据文件%s不可读：%w", path, e)
		}
		files[path] = envmodel.Digest(b)
	}
	publication := c.runnerPublication()
	publication.BaseBranch = manifest.ValidationBase
	input := runnercontract.RunInput{Execution: profile.Execution, Workflow: runnercontract.Workflow{ID: "development", Version: 2}, Repository: runnercontract.Repository{URL: "", LocalPath: c.Repository, Remote: c.Remote, BaseBranch: manifest.ValidationBase, BaseRevision: commit}, Publication: publication, Documents: runnercontract.Documents{Spec: runnercontract.Document{Path: envmodel.DocumentPath(key)}, Plan: runnercontract.Document{Path: manifest.Script}, References: []runnercontract.Document{{Path: envmodel.ExecutionRulesPath}, {Path: pm.ProjectEntryPath}, {Path: entry.TestingCatalogue}}}}
	referenceSet := map[string]bool{envmodel.DocumentPath(key): true, manifest.Script: true, envmodel.ExecutionRulesPath: true, pm.ProjectEntryPath: true, entry.TestingCatalogue: true}
	// Runtime dependencies remain validated/hashed above and are available in
	// the pinned worktree through the manifest. Agent reading references carry
	// project policy, not a second listing of every runtime helper.
	for _, path := range entry.Requirements {
		if !referenceSet[path] {
			input.Documents.References = append(input.Documents.References, runnercontract.Document{Path: path})
			referenceSet[path] = true
		}
	}
	input.Repository.URL, e = c.TaskRepositoryTarget(ctx)
	if e != nil {
		return out, e
	}
	if e = c.runtimeCatalog.ValidateConfiguration(input.Execution, input.Workflow); e != nil {
		return out, e
	}
	if e = c.runtimeCatalog.ValidatePublication(publication); e != nil {
		return out, e
	}
	preparationContext, e := envmodel.Context(profile.Execution)
	if e != nil {
		return out, e
	}
	basis, _ := json.Marshal(struct {
		Entry       pm.ProjectEntry
		Files       map[string]string
		Tests       []testmanagement.Definition
		Publication runnercontract.Publication
		Execution   runnercontract.ExecutionInput
		Repository  []string
	}{entry, files, definitions, publication, preparationContext, []string{input.Repository.URL, c.Remote, c.Branch}})
	out = envmodel.Prepared{Commit: commit, Inputs: envmodel.Digest(basis), ScriptDigest: envmodel.Digest(script), Manifest: manifest, L2: ids, Input: input}
	return out, nil
}
func (p *environmentPort) prepareGeneration(ctx context.Context, in envmodel.Generate) (envmodel.GenerationPrepared, error) {
	var prepared envmodel.GenerationPrepared
	out, err := p.generationInput(ctx, in)
	if err != nil {
		return prepared, err
	}
	target, err := p.targetProfile(in.TargetKey)
	if err != nil {
		return prepared, err
	}
	body, err := p.generationContext(ctx, in.TargetKey)
	if err != nil {
		return prepared, err
	}
	return envmodel.GenerationPrepared{Input: out, Target: target.Execution, Context: body}, nil
}

func (p *environmentPort) generationInput(ctx context.Context, in envmodel.Generate) (runnercontract.GenerateRunInput, error) {
	var out runnercontract.GenerateRunInput
	key := in.TargetKey
	if _, err := p.targetProfile(key); err != nil {
		return out, err
	}
	id := in.Profile
	profile, e := p.config.RunnerExecution.Profile(id, "")
	if e != nil {
		return out, e
	}
	execution, e := runnerclient.GenerationExecution(profile.Execution)
	if e != nil {
		return out, e
	}
	commit, e := p.source(ctx)
	if e != nil {
		return out, e
	}
	selected, survey, e := p.generationFiles(ctx, commit, key)
	if e != nil {
		return out, e
	}
	facts, e := p.generationContext(ctx, key)
	if e != nil {
		return out, e
	}
	instruction, e := prompts.EnvironmentGenerate(commit, string(facts), envmodel.DocumentPath(key), envmodel.MaterialDirectory(key)+"/verify-environment", survey)
	if e != nil {
		return out, e
	}
	materials := []runnercontract.Document{}
	out = runnercontract.GenerateRunInput{Title: "项目环境准备文档与验收脚本", Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Execution: execution, Generate: runnercontract.GenerateInput{Repository: &runnercontract.GenerateRepository{LocalPath: p.config.Repository, Commit: commit, Paths: selected}, Instruction: instruction, Materials: materials, OutputSchema: json.RawMessage(env.GenerationSchema)}}
	if p.config.runtimeCatalog == nil {
		return out, errors.New("Runner能力目录不可用")
	}
	return out, p.config.runtimeCatalog.ValidateRepositoryGenerate(out)
}

// observedSource is query-only: fetch belongs to explicit prepare/start or the
// existing project maintenance observation, never the environment page poll.
func (p *environmentPort) observedSource(ctx context.Context) (string, error) {
	if m := p.config.runtimeMaintenance; m != nil {
		observation := m.current()
		if observation.Diagnostic != "" {
			return "", errors.New(observation.Diagnostic)
		}
		if observation.Commit != "" {
			return observation.Commit, nil
		}
	}
	raw, err := p.git().Output(ctx, p.config.Git, "rev-parse", "--verify", "refs/remotes/"+p.config.Remote+"/"+p.config.Branch+"^{commit}")
	return strings.TrimSpace(string(raw)), err
}
func (p *environmentPort) current(ctx context.Context, id string) (envmodel.Current, envmodel.Record, error) {
	profile, err := p.config.RunnerExecution.Profile(id, "")
	if err != nil {
		return envmodel.Current{}, envmodel.Record{}, err
	}
	key, err := envmodel.Key(profile.Execution)
	if err != nil {
		return envmodel.Current{}, envmodel.Record{}, err
	}
	current, err := p.store.CurrentEnvironment(ctx, p.config.Identity(), key)
	if errors.Is(err, envmodel.ErrNotFound) {
		return envmodel.Current{Project: p.config.Identity(), Key: key}, envmodel.Record{}, nil
	}
	if err != nil {
		return current, envmodel.Record{}, err
	}
	if current.ActivityID == "" {
		return current, envmodel.Record{}, nil
	}
	record, err := p.store.Environment(ctx, p.config.Identity(), current.ActivityID)
	return current, record, err
}
func (p *environmentPort) admission(ctx context.Context, id, candidateCommit string, refresh bool) (envmodel.Admission, envmodel.Record, error) {
	current, record, err := p.current(ctx, id)
	if err != nil {
		return envmodel.Admission{}, record, err
	}
	if _, err := envmodel.Qualify(current, record, record.Prepared.Inputs); err != nil {
		return envmodel.Admission{}, record, err
	}
	var commit string
	if refresh {
		if maintenance := p.config.runtimeMaintenance; maintenance != nil {
			observation, readErr := maintenance.check(ctx)
			commit, err = observation.Commit, readErr
		} else {
			commit, err = p.source(ctx)
		}
	} else {
		commit, err = p.observedSource(ctx)
	}
	if err != nil {
		return envmodel.Admission{}, record, err
	}
	prepared, err := p.materialAt(ctx, id, commit, false)
	if err != nil {
		return envmodel.Admission{}, record, err
	}
	if candidateCommit != "" && candidateCommit != commit {
		candidate, err := p.materialAt(ctx, id, candidateCommit, false)
		if err != nil {
			return envmodel.Admission{}, record, err
		}
		if candidate.Inputs != prepared.Inputs {
			return envmodel.Admission{}, record, errors.New("环境未就绪：候选固定提交的环境材料与当前依据不同")
		}
	}
	proof, err := envmodel.Qualify(current, record, prepared.Inputs)
	return proof, record, err
}
