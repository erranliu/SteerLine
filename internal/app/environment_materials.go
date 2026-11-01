package app

import (
	"context"
	"fmt"
	model "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

func (p *environmentPort) targetProfile(key string) (RunnerProfile, error) {
	for _, profile := range p.config.RunnerExecution.Profiles {
		if profile.Workflow.ID != "development" && profile.Workflow.ID != "development-audited" {
			continue
		}
		k, e := model.Key(profile.Execution)
		if e == nil && k == key {
			return profile, nil
		}
	}
	return RunnerProfile{}, fmt.Errorf("所选环境不存在于当前开发方案")
}
func (p *environmentPort) materials(ctx context.Context, key string) (any, error) {
	profile, e := p.targetProfile(key)
	if e != nil {
		return nil, e
	}
	out := map[string]any{"environment_key": key, "document_path": model.DocumentPath(key)}
	commit, e := p.observedSource(ctx)
	if e != nil {
		out["diagnostic"] = e.Error()
		return out, nil
	}
	out["source_commit"] = commit
	document, e := p.read(ctx, commit, model.DocumentPath(key))
	if e != nil {
		out["diagnostic"] = e.Error()
		return out, nil
	}
	out["document"] = string(document)
	material, e := p.materialAt(ctx, profile.ID, commit)
	if e != nil {
		out["diagnostic"] = e.Error()
		return out, nil
	}
	out["script_path"] = material.Manifest.Script
	out["inputs"] = material.Inputs
	return out, nil
}
