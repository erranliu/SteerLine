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
	_, e := p.targetProfile(key)
	if e != nil {
		return nil, e
	}
	out := map[string]any{"environment_key": key, "document_path": model.DocumentPath()}
	commit, e := p.observedSource(ctx)
	if e != nil {
		out["diagnostic"] = e.Error()
		return out, nil
	}
	out["source_commit"] = commit
	document, manifest, _, e := p.invocationAt(ctx, commit)
	if len(document) > 0 {
		out["document"] = string(document)
	}
	if e != nil {
		out["diagnostic"] = e.Error()
		return out, nil
	}
	out["script_path"] = manifest.Script
	return out, nil
}
