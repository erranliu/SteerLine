package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	env "github.com/erranliu/SteerLine/internal/environmentadaptation"
	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"strings"
	"time"
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
	s.WithTarget = func(ctx context.Context, target, owner, commit string, create func() error) error {
		selectCurrent := func() error {
			if maintenance := p.config.runtimeMaintenance; maintenance != nil {
				return maintenance.reserveBaseline(commit, create)
			}
			return create()
		}
		// Match candidate admission's target -> baseline -> storage lock order.
		if admission := p.config.runtimeEnvironmentAdmission; admission != nil {
			return admission.adapt(ctx, target, owner, selectCurrent)
		}
		return selectCurrent()
	}
	return p
}
func (p *environmentPort) git() commandRunner {
	return commandRunner{Dir: p.config.Repository, Timeout: positiveDuration(p.config.CommandTimeout, 30*time.Second), ProcessHost: p.config.ProcessHost}
}
func (p *environmentPort) source(ctx context.Context) (string, error) {
	if m := p.config.runtimeMaintenance; m != nil {
		o, e := m.check(ctx)
		return o.Commit, e
	}
	c := p.config
	g := p.git()
	if _, e := g.Output(ctx, c.Git, "fetch", "--no-tags", "--no-write-fetch-head", "--", c.Remote, "+refs/heads/"+c.Branch+":refs/remotes/"+c.Remote+"/"+c.Branch); e != nil {
		return "", e
	}
	b, e := g.Output(ctx, c.Git, "rev-parse", "--verify", "refs/remotes/"+c.Remote+"/"+c.Branch+"^{commit}")
	if e != nil {
		return "", e
	}
	commit := strings.TrimSpace(string(b))
	return commit, p.observeBasis(ctx, commit)
}
func (p *environmentPort) read(ctx context.Context, commit, path string) ([]byte, error) {
	r := taskDocuments{repository: p.config.Repository, git: p.config.Git, timeout: positiveDuration(p.config.CommandTimeout, 30*time.Second), processHost: p.config.ProcessHost}
	return r.ReadCommittedRegularFile(ctx, taskmodel.FileReference{Path: path, Commit: commit, SHA256: strings.Repeat("0", 64)})
}
func (p *environmentPort) prepare(ctx context.Context, id string) (envmodel.Prepared, error) {
	if p.config.runtimeCatalog == nil {
		return envmodel.Prepared{}, errors.New("Runner能力目录不可用")
	}
	profile, e := p.config.RunnerExecution.Profile(id, "")
	if e != nil {
		return envmodel.Prepared{}, e
	}
	if e = p.config.runtimeCatalog.ValidateEnvironmentExecution(profile.Execution); e != nil {
		return envmodel.Prepared{}, e
	}
	return p.materialAt(ctx, id, "")
}
func (p *environmentPort) materialAt(ctx context.Context, id, commit string) (envmodel.Prepared, error) {
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
	if commit == "" {
		commit, e = p.source(ctx)
		if e != nil {
			return out, e
		}
	}
	document, e := p.read(ctx, commit, envmodel.DocumentPath(key))
	if e != nil {
		return out, e
	}
	manifest, e := envmodel.ParseManifest(document)
	if e != nil {
		return out, &invalidEnvironmentBasis{e}
	}
	if manifest.EnvironmentKey != key {
		return out, &invalidEnvironmentBasis{errors.New("环境接口与所选目标不符")}
	}
	script, e := p.read(ctx, commit, manifest.Script)
	if e != nil {
		return out, e
	}
	files := map[string]string{envmodel.DocumentPath(key): envmodel.Digest(document), manifest.Script: envmodel.Digest(script)}
	total := len(document) + len(script)
	for _, path := range manifest.Dependencies {
		b, e := p.read(ctx, commit, path)
		if e != nil {
			return out, fmt.Errorf("环境依据 %s: %w", path, e)
		}
		total += len(b)
		files[path] = envmodel.Digest(b)
	}
	if total > 1<<20 {
		return out, &invalidEnvironmentBasis{errors.New("环境调用材料超限")}
	}
	execution, e := envmodel.Context(profile.Execution)
	if e != nil {
		return out, e
	}
	call := struct {
		Project string
		Key     string
		Context envmodel.CallContext
	}{c.Identity(), key, envmodel.CallContext{Adapter: profile.Execution.Adapter, Target: profile.Execution.Target}}
	basis, e := json.Marshal(struct {
		Files     map[string]string
		Execution runnercontract.ExecutionInput
		Call      any
		Version   int
	}{files, execution, call, envmodel.ReportVersion})
	if e != nil {
		return out, e
	}
	input := runnercontract.RunInput{Execution: profile.Execution, Workflow: runnercontract.Workflow{ID: "execute", Version: 1}, Repository: runnercontract.Repository{LocalPath: c.Repository, Remote: c.Remote, BaseBranch: c.Branch, BaseRevision: commit}, Documents: runnercontract.Documents{Spec: runnercontract.Document{Path: envmodel.DocumentPath(key)}, Plan: runnercontract.Document{Path: manifest.Script}}}
	input.Repository.URL, e = c.TaskRepositoryTarget(ctx)
	if e != nil {
		return out, e
	}
	out = envmodel.Prepared{Commit: commit, Inputs: envmodel.Digest(basis), ScriptDigest: envmodel.Digest(script), Manifest: manifest, Input: input}
	return out, nil
}

// observedSource is query-only: fetch belongs to explicit prepare/start or the
// existing project maintenance observation, never the environment page poll.
func (p *environmentPort) observedSource(ctx context.Context) (string, error) {
	if m := p.config.runtimeMaintenance; m != nil {
		m.mu.Lock()
		observation := m.observation
		pending := m.checking != nil
		m.mu.Unlock()
		if pending {
			return "", errors.New("主分支环境依据观察处理中")
		}
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
	prepared, err := p.materialAt(ctx, id, commit)
	if err != nil {
		return envmodel.Admission{}, record, err
	}
	if candidateCommit != "" && candidateCommit != commit {
		candidate, err := p.materialAt(ctx, id, candidateCommit)
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
