package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	model "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func (p *environmentPort) targetProfile(key string) (RunnerProfile, error) {
	if model.ValidKey(key) {
		for _, profile := range p.config.RunnerExecution.Profiles {
			if profile.Workflow.ID != "development" && profile.Workflow.ID != "development-audited" {
				continue
			}
			if actual, err := model.Key(profile.Execution); err == nil && actual == key {
				return profile, nil
			}
		}
	}
	return RunnerProfile{}, errors.New("所选目标环境不存在于当前开发方案")
}

// Deliberately no raw settings, transport, args, environment values or model.
type environmentPublicExecution struct {
	Adapter    string                         `json:"adapter"`
	Target     runnercontract.ExecutionTarget `json:"target"`
	Agent      string                         `json:"agent,omitempty"`
	Executable string                         `json:"executable,omitempty"`
	AccessMode string                         `json:"access_mode,omitempty"`
	Platform   string                         `json:"platform,omitempty"`
}

func (p *environmentPort) generationContext(ctx context.Context, key string) ([]byte, error) {
	profile, err := p.targetProfile(key)
	if err != nil {
		return nil, err
	}
	var settings struct {
		Executable  string `json:"executable"`
		Agent       string `json:"agent"`
		AccessMode  string `json:"access_mode"`
		AgentLaunch struct {
			Executable string `json:"executable"`
		} `json:"agent_launch"`
	}
	if err := json.Unmarshal(profile.Execution.Settings, &settings); err != nil {
		return nil, err
	}
	execution := environmentPublicExecution{Adapter: profile.Execution.Adapter, Target: profile.Execution.Target, Agent: settings.Agent, Executable: settings.Executable, AccessMode: settings.AccessMode}
	if profile.Execution.Adapter == "orca-terminal" {
		execution.Executable = settings.AgentLaunch.Executable
	}
	if profile.Execution.Target.Kind == "local" {
		execution.Platform = runtime.GOOS
	}
	pub := p.config.runnerPublication()
	return json.Marshal(struct {
		Version     int                        `json:"version"`
		Project     string                     `json:"project_identity"`
		Key         string                     `json:"environment_key"`
		Execution   environmentPublicExecution `json:"execution"`
		Publication struct {
			Provider   string `json:"provider"`
			Repository string `json:"repository"`
			Base       string `json:"base_branch"`
		} `json:"publication"`
	}{1, p.config.Identity(), key, execution, struct {
		Provider   string `json:"provider"`
		Repository string `json:"repository"`
		Base       string `json:"base_branch"`
	}{pub.Provider, pub.Repository, pub.BaseBranch}})
}

func (p *environmentPort) projectEntryAt(ctx context.Context, commit string) (pm.ProjectEntry, error) {
	return pm.ReadProjectEntry(func(path string) ([]byte, error) { return p.read(ctx, commit, path) })
}

func (p *environmentPort) generationFiles(ctx context.Context, commit, key string) ([]string, bool, error) {
	entry, err := p.projectEntryAt(ctx, commit)
	if err != nil {
		return nil, false, fmt.Errorf("项目材料待适配：%s: %w", pm.ProjectEntryPath, err)
	}
	paths := append([]string{model.GenerationRulesPath, model.ExecutionRulesPath}, entry.Paths()...)
	for _, item := range []struct {
		path string
		body func() ([]byte, error)
	}{{model.GenerationRulesPath, prompts.EnvironmentGenerationRules}, {model.ExecutionRulesPath, prompts.EnvironmentExecutionRules}} {
		expected, e := item.body()
		if e != nil {
			return nil, false, e
		}
		actual, e := p.read(ctx, commit, item.path)
		if e != nil || !prompts.SameRules(actual, expected) {
			return nil, false, fmt.Errorf("材料准备不足：协议缺失或不同源：%s", item.path)
		}
	}
	survey := false
	document, e := p.read(ctx, commit, model.DocumentPath(key))
	if e != nil {
		if !errors.Is(e, os.ErrNotExist) {
			return nil, false, e
		}
		survey = true
	} else {
		paths = append(paths, model.DocumentPath(key))
		manifest, e := model.ParseManifest(document)
		if e != nil || manifest.EnvironmentKey != key {
			survey = true
		} else {
			for _, path := range append([]string{manifest.Script}, manifest.Dependencies...) {
				if _, e := p.read(ctx, commit, path); e != nil {
					if !errors.Is(e, os.ErrNotExist) {
						return nil, false, e
					}
					survey = true
					continue
				}
				paths = append(paths, path)
			}
		}
	}
	if survey {
		// The existing whole-tree snapshot supplies actual source for adaptation;
		// missing outputs/obsolete references are not required snapshot files.
		paths = append(paths, "SteerlineProjectGuideDoc/environment-adaptation.md")
	}
	seen := map[string]bool{}
	selected := []string{}
	total := 0
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		body, e := p.read(ctx, commit, path)
		if e != nil {
			return nil, false, fmt.Errorf("生成依据 %s 不可读：%w", path, e)
		}
		total += len(body)
		if total > 1<<20 {
			return nil, false, errors.New("环境生成材料超限")
		}
		selected = append(selected, path)
	}
	if survey {
		tree, err := p.git().Output(ctx, p.config.Git, "ls-tree", "-r", "-l", "-z", commit)
		if err != nil {
			return nil, false, err
		}
		total := int64(0)
		for _, item := range bytes.Split(tree, []byte{0}) {
			if len(item) == 0 {
				continue
			}
			parts := bytes.SplitN(item, []byte{9}, 2)
			if len(parts) != 2 {
				return nil, false, errors.New("invalid generation source tree")
			}
			fields := strings.Fields(string(parts[0]))
			if len(fields) != 4 {
				return nil, false, errors.New("invalid generation source metadata")
			}
			if fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
				continue
			}
			size, err := strconv.ParseInt(fields[3], 10, 64)
			if err != nil || size < 0 {
				return nil, false, errors.New("invalid generation source size")
			}
			if size > (1<<20)-total {
				return nil, false, errors.New("环境调查源码材料超限")
			}
			total += size
		}
		return nil, true, nil
	}
	return selected, false, nil
}

func (p *environmentPort) materials(ctx context.Context, key string) (any, error) {
	if _, err := p.targetProfile(key); err != nil {
		return nil, err
	}
	facts, err := p.generationContext(ctx, key)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"environment_key": key, "document_path": model.DocumentPath(key), "target_facts": json.RawMessage(facts), "generation_ready": false}
	commit, err := p.observedSource(ctx)
	if err != nil {
		out["generation_diagnostic"] = err.Error()
		return out, nil
	}
	out["source_commit"] = commit
	if document, e := p.read(ctx, commit, model.DocumentPath(key)); e == nil {
		out["document"] = string(document)
		if manifest, e := model.ParseManifest(document); e == nil && manifest.EnvironmentKey == key {
			out["gaps"] = manifest.Gaps
			if e := manifest.Ready(); e != nil {
				out["verification_diagnostic"] = e.Error()
			}
			out["script_path"] = manifest.Script
			if script, e := p.read(ctx, commit, manifest.Script); e == nil {
				out["script"] = string(script)
			}
		}
	}
	_, survey, e := p.generationFiles(ctx, commit, key)
	if e != nil {
		out["generation_diagnostic"] = e.Error()
		return out, nil
	}
	out["generation_ready"] = true
	if survey {
		out["generation_mode"] = "adapt"
	} else {
		out["generation_mode"] = "update"
	}
	return out, nil
}
