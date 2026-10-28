package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/prompts"
	"sort"
	"strings"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation"
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
		s.AcquireGeneration = func(r env.Record) bool { return q.TryAcquire(r.Project, "environment:"+r.ID, r.CreatedAt) }
		s.ReleaseGeneration = func(r env.Record) { q.Release(r.Project, "environment:"+r.ID) }
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
func (p *environmentPort) prepare(ctx context.Context, id string) (env.Prepared, error) {
	return p.material(ctx, id, true)
}
func (p *environmentPort) material(ctx context.Context, id string, checkBase bool) (env.Prepared, error) {
	return p.materialAt(ctx, id, "", checkBase)
}
func (p *environmentPort) materialAt(ctx context.Context, id, commit string, checkBase bool) (env.Prepared, error) {
	var out env.Prepared
	c := p.config
	profile, e := c.RunnerExecution.Profile(id, "")
	if e != nil {
		return out, e
	}
	if env.Target(profile.Execution) == "" || profile.Execution.Adapter != "orca-terminal" {
		return out, errors.New("请选择远端Orca执行配置")
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
	document, e := p.read(ctx, commit, env.DocumentPath)
	if e != nil {
		return out, fmt.Errorf("环境准备文档缺失：%w", e)
	}
	manifest, e := env.ParseManifest(document)
	if e != nil {
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
	catalogueRaw, e := p.read(ctx, commit, "SteerlineFlow/testing/test-list.json")
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
	executionRules, e := p.read(ctx, commit, env.ExecutionRulesPath)
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
	files := map[string]string{env.ExecutionRulesPath: env.Digest(executionRules), env.DocumentPath: env.Digest(document), manifest.Script: env.Digest(script)}
	deps := append([]string{"SteerlineFlow/testing.md"}, manifest.Dependencies...)
	for _, path := range deps {
		b, e := p.read(ctx, commit, path)
		if e != nil {
			return out, fmt.Errorf("环境依据文件%s不可读：%w", path, e)
		}
		files[path] = env.Digest(b)
	}
	publication := c.runnerPublication()
	publication.BaseBranch = manifest.ValidationBase
	input := runnercontract.RunInput{Execution: profile.Execution, Workflow: runnercontract.Workflow{ID: "development", Version: 2}, Repository: runnercontract.Repository{URL: "", LocalPath: c.Repository, Remote: c.Remote, BaseBranch: manifest.ValidationBase, BaseRevision: commit}, Publication: publication, Documents: runnercontract.Documents{Spec: runnercontract.Document{Path: env.DocumentPath}, Plan: runnercontract.Document{Path: manifest.Script}, References: []runnercontract.Document{{Path: env.ExecutionRulesPath}, {Path: "SteerlineFlow/testing.md"}, {Path: "SteerlineFlow/testing/test-list.json"}}}}
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
	basis, _ := json.Marshal(struct {
		Files       map[string]string
		Tests       []testmanagement.Definition
		Publication runnercontract.Publication
		Execution   runnercontract.ExecutionInput
		Repository  []string
	}{files, definitions, publication, profile.Execution, []string{input.Repository.URL, c.Remote, c.Branch}})
	out = env.Prepared{Commit: commit, Inputs: env.Digest(basis), ScriptDigest: env.Digest(script), Manifest: manifest, L2: ids, Input: input}
	return out, nil
}
func (p *environmentPort) prepareGeneration(ctx context.Context, id string) (runnercontract.GenerateRunInput, error) {
	var out runnercontract.GenerateRunInput
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
	paths := []string{env.GenerationRulesPath, env.ExecutionRulesPath, env.GenerationContextPath, "README.md", "SteerlineFlow/testing.md", "SteerlineFlow/testing/test-list.json", env.DocumentPath, "go.mod", "package.json", "pyproject.toml", "requirements.txt", "scripts/verify-environment.ps1", "scripts/verify-environment.sh"}
	required := map[string]bool{env.GenerationRulesPath: true, env.ExecutionRulesPath: true, env.GenerationContextPath: true, "SteerlineFlow/testing.md": true, "SteerlineFlow/testing/test-list.json": true}
	if document, e := p.read(ctx, commit, env.DocumentPath); e == nil {
		if manifest, e := env.ParseManifest(document); e == nil {
			paths = append(paths, manifest.Dependencies...)
			for _, path := range manifest.Dependencies {
				required[path] = true
			}
		}
	}
	materials := []runnercontract.Document{}
	total := 0
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		b, e := p.read(ctx, commit, path)
		if e != nil {
			if required[path] {
				return out, e
			}
			continue
		}
		total += len(b)
		if total > 1<<20 {
			return out, errors.New("环境生成材料超限，请精简项目说明")
		}

	}
	public, _ := json.Marshal(p.config.runnerPublication())
	var previous json.RawMessage
	if p.store != nil {
		rows, e := p.store.Environments(ctx, p.config.Identity())
		if e != nil {
			return out, e
		}
		for _, r := range rows {
			if r.Kind == "adaptation" && len(r.Rounds) > 0 {
				last := r.Rounds[len(r.Rounds)-1]
				fact := env.PreviousFacts(r, last)
				previous = fact
				break
			}
		}
	}
	rules, err := p.read(ctx, commit, env.GenerationRulesPath)
	if err != nil {
		return out, err
	}
	expectedRules, err := prompts.EnvironmentGenerationRules()
	if err != nil {
		return out, err
	}
	if !prompts.SameRules(rules, expectedRules) {
		return out, errors.New("材料准备不足：环境生成规则不同源")
	}
	executionRules, err := p.read(ctx, commit, env.ExecutionRulesPath)
	if err != nil {
		return out, err
	}
	expectedExecutionRules, err := prompts.EnvironmentExecutionRules()
	if err != nil {
		return out, err
	}
	if !prompts.SameRules(executionRules, expectedExecutionRules) {
		return out, errors.New("材料准备不足：环境报告/执行规则不同源")
	}
	contextBody, err := json.Marshal(struct {
		Publication json.RawMessage `json:"publication"`
		Previous    json.RawMessage `json:"previous"`
	}{public, previous})
	if err != nil {
		return out, err
	}
	contextFile, err := p.read(ctx, commit, env.GenerationContextPath)
	if err != nil {
		return out, err
	}
	if !bytes.Equal(contextFile, contextBody) {
		return out, errors.New("材料准备不足：环境上下文文件与原公开配置/完整前轮事实不符")
	}
	instruction, err := prompts.EnvironmentGenerate(commit)
	if err != nil {
		return out, err
	}
	out = runnercontract.GenerateRunInput{Title: "项目环境准备文档与验收脚本", Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Execution: execution, Generate: runnercontract.GenerateInput{Repository: &runnercontract.GenerateRepository{LocalPath: p.config.Repository, Commit: commit}, Instruction: instruction, Materials: materials, OutputSchema: json.RawMessage(env.GenerationSchema)}}
	if p.config.runtimeCatalog == nil {
		return out, errors.New("Runner能力目录不可用")
	}
	return out, p.config.runtimeCatalog.ValidateRepositoryGenerate(out)
}

func (p *environmentPort) ready(ctx context.Context, id string) (env.Record, error) {
	return p.readyFor(ctx, id, "")
}
func (p *environmentPort) readyFor(ctx context.Context, id, candidateCommit string) (env.Record, error) {
	prepared, e := p.material(ctx, id, false)
	if e != nil {
		return env.Record{}, fmt.Errorf("环境未就绪：%w", e)
	}
	if candidateCommit != "" && candidateCommit != prepared.Commit {
		candidate, e := p.materialAt(ctx, id, candidateCommit, false)
		if e != nil {
			return env.Record{}, fmt.Errorf("环境未就绪：候选固定环境材料不可核实：%w", e)
		}
		if candidate.Inputs != prepared.Inputs {
			return env.Record{}, errors.New("环境未就绪：候选固定提交的环境材料与当前依据不同")
		}
	}
	rows, e := p.store.Environments(ctx, p.config.Identity())
	if e != nil {
		return env.Record{}, e
	}
	for _, r := range rows {
		if r.Kind == "adaptation" && r.Profile == id {
			if r.Status() != "ready" || r.Prepared.Inputs != prepared.Inputs {
				return r, errors.New("环境未就绪：请完成当前项目和配置的适配")
			}
			return r, nil
		}
	}
	return env.Record{}, errors.New("环境未就绪：尚未适配当前远端配置")
}
