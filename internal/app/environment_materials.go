package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation"
	model "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
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
	var previous json.RawMessage
	if p.store != nil {
		rows, err := p.store.Environments(ctx, p.config.Identity())
		if err != nil {
			return nil, err
		}
		for _, record := range rows {
			if record.Kind != "adaptation" || !record.Settled() || len(record.Rounds) == 0 {
				continue
			}
			actual, err := model.Key(record.Execution)
			if err == nil && actual == key {
				previous = env.PreviousFacts(record, record.Rounds[len(record.Rounds)-1])
				break
			}
		}
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
		Previous json.RawMessage `json:"previous"`
	}{1, p.config.Identity(), key, execution, struct {
		Provider   string `json:"provider"`
		Repository string `json:"repository"`
		Base       string `json:"base_branch"`
	}{pub.Provider, pub.Repository, pub.BaseBranch}, previous})
}

func (p *environmentPort) materials(ctx context.Context, key string) (any, error) {
	if _, err := p.targetProfile(key); err != nil {
		return nil, err
	}
	body, err := p.generationContext(ctx, key)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"environment_key": key, "document_path": model.DocumentPath(key), "context_path": model.GenerationContextPath(key), "expected_context": string(body), "context_digest": model.Digest(body), "generation_ready": false}
	commit, err := p.observedSource(ctx)
	if err != nil {
		out["generation_diagnostic"] = err.Error()
		return out, nil
	}
	out["source_commit"] = commit
	if document, err := p.read(ctx, commit, model.DocumentPath(key)); err == nil {
		out["document"] = string(document)
		if manifest, err := model.ParseManifest(document); err == nil && manifest.EnvironmentKey == key {
			out["script_path"] = manifest.Script
			if script, err := p.read(ctx, commit, manifest.Script); err == nil {
				out["script"] = string(script)
			}
		}
	}
	for _, item := range []struct {
		path     string
		expected func() ([]byte, error)
	}{{model.GenerationRulesPath, prompts.EnvironmentGenerationRules}, {model.ExecutionRulesPath, prompts.EnvironmentExecutionRules}} {
		expected, err := item.expected()
		if err != nil {
			return nil, err
		}
		actual, err := p.read(ctx, commit, item.path)
		if err != nil || !prompts.SameRules(actual, expected) {
			out["generation_diagnostic"] = fmt.Sprintf("生成输入待维护：%s 缺失或不同源，请按正常 Git 流程维护项目规则", item.path)
			return out, nil
		}
	}
	for _, path := range []string{"SteerlineFlow/testing.md", "SteerlineFlow/testing/test-list.json"} {
		if _, err := p.read(ctx, commit, path); err != nil {
			out["generation_diagnostic"] = "生成输入待维护：" + path
			return out, nil
		}
	}
	actual, err := p.read(ctx, commit, model.GenerationContextPath(key))
	if err != nil || !bytes.Equal(actual, body) {
		out["generation_diagnostic"] = "生成输入待维护：导出本环境上下文，按正常 Git 流程提交后刷新。"
		return out, nil
	}
	out["generation_ready"] = true
	return out, nil
}
