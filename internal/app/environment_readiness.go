package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/developerconfig"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type environmentProfileView struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Key             string `json:"environment_key"`
	Status          string `json:"status"`
	Stage           string `json:"stage"`
	Reason          string `json:"reason"`
	CurrentRevision uint64 `json:"current_revision"`
	ActivityID      string `json:"activity_id,omitempty"`
}
type environmentView struct {
	Target             runnercontract.ExecutionTarget `json:"target"`
	Adapter            string                         `json:"adapter"`
	Agent              string                         `json:"agent"`
	ActivitySettled    bool                           `json:"activity_settled"`
	ScriptPath         string                         `json:"script_path,omitempty"`
	Key                string                         `json:"environment_key"`
	Name               string                         `json:"name"`
	Profiles           []environmentProfileView       `json:"profiles"`
	Status             string                         `json:"status"`
	Stage              string                         `json:"stage"`
	Reason             string                         `json:"reason"`
	CurrentRevision    uint64                         `json:"current_revision"`
	ActivityID         string                         `json:"activity_id,omitempty"`
	SourceCommit       string                         `json:"source_commit,omitempty"`
	DocumentPath       string                         `json:"document_path"`
	Document           string                         `json:"document,omitempty"`
	LastPassedActivity string                         `json:"last_passed_activity_id,omitempty"`
}

type environmentOptions struct {
	ReadinessRequired bool `json:"environment_readiness_required"`

	Profiles       []environmentProfileView `json:"profiles"`
	Environments   []environmentView        `json:"environments"`
	DefaultProfile string                   `json:"default_profile_id"`
}

func (p *environmentPort) configurationOptions() environmentOptions {
	// Configuration facts exist independently of project storage and materials.
	environments := []environmentView{}
	groups := map[string]int{}
	for _, profile := range p.config.RunnerExecution.Profiles {
		if profile.Workflow.ID != "development" && profile.Workflow.ID != "development-audited" {
			continue
		}
		key, keyErr := env.Key(profile.Execution)
		group := key
		if keyErr != nil {
			group = "unsupported:" + profile.ID
		}
		model, effort := environmentProfileModel(profile.Execution)
		view := environmentProfileView{ID: profile.ID, Name: profile.Name, Model: model, ReasoningEffort: effort, Key: key, Status: "not_ready", Stage: "unverified", Reason: "项目运行环境状态待核实"}
		if index, ok := groups[group]; ok {
			environments[index].Profiles = append(environments[index].Profiles, view)
			continue
		}
		name := profile.Execution.Target.Environment
		if name == "" {
			name = "本机 · " + profile.Execution.Adapter
		}
		var display struct {
			Agent string `json:"agent"`
		}
		if profile.Execution.Adapter == "orca-terminal" {
			_ = json.Unmarshal(profile.Execution.Settings, &display)
		}
		groups[group] = len(environments)
		environments = append(environments, environmentView{Key: key, Name: name, Target: profile.Execution.Target, Adapter: profile.Execution.Adapter, Agent: display.Agent, Profiles: []environmentProfileView{view}, DocumentPath: env.DocumentPath(key), Status: view.Status, Stage: view.Stage, Reason: view.Reason})
	}
	return p.optionsFor(environments)
}

func (p *environmentPort) readinessOptions(ctx context.Context) (any, error) {
	ctx = materialReads(ctx)
	environments := p.configurationOptions().Environments

	rows, historyErr := p.store.Environments(ctx, p.config.Identity())
	commit, sourceErr := p.observedSource(ctx)
	for i := range environments {
		item := &environments[i]
		id := item.Profiles[0].ID
		current, record, readErr := p.current(ctx, id)
		materialErr := errors.Join(historyErr, readErr, sourceErr)
		basis := ""
		if materialErr == nil && !current.Invalidated {
			if current.ActivityID == "" {
				// No qualification exists to compare. Validate the invocation only;
				// start still validates every declared dependency before executing.
				_, _, _, materialErr = p.invocationAt(ctx, item.Key, commit)
			} else if record.Status() == "ready" {
				prepared, err := p.materialAt(ctx, id, commit)
				materialErr, basis = err, prepared.Inputs
			}
		}
		item.Status, item.Stage, item.Reason = env.View(current, record, basis, materialErr)
		if historyErr != nil || readErr != nil || item.Key == "" {
			item.Status, item.Stage = "not_ready", "blocked"
			item.Reason = "项目运行环境状态无法核验"
			if materialErr != nil {
				item.Reason += "：" + materialErr.Error()
			}
		}
		item.CurrentRevision, item.ActivityID = current.Revision, current.ActivityID
		item.SourceCommit = commit
		item.ActivitySettled = record.ID == "" || record.Settled()
		for _, old := range rows {
			oldKey, err := env.Key(old.Execution)
			if old.Status() == "ready" && err == nil && oldKey == item.Key {
				item.LastPassedActivity = old.ID
				break
			}
		}
		for j := range item.Profiles {
			v := &item.Profiles[j]
			v.Status, v.Stage, v.Reason = item.Status, item.Stage, item.Reason
			v.CurrentRevision, v.ActivityID = item.CurrentRevision, item.ActivityID
		}
	}
	return p.optionsFor(environments), nil
}

func (p *environmentPort) optionsFor(environments []environmentView) environmentOptions {
	byProfile := map[string]environmentProfileView{}
	for _, item := range environments {
		for _, profile := range item.Profiles {
			byProfile[profile.ID] = profile
		}
	}
	profiles := []environmentProfileView{}
	for _, profile := range p.config.RunnerExecution.Profiles {
		if view, ok := byProfile[profile.ID]; ok {
			profiles = append(profiles, view)
		}
	}
	policy := developerconfig.Resolve(p.config.RunnerExecution.Developer)
	return environmentOptions{Profiles: profiles, Environments: environments, DefaultProfile: p.config.RunnerExecution.DefaultProfile, ReadinessRequired: !policy.SkipEnvironmentReadiness()}
}

func environmentProfileModel(execution runnercontract.ExecutionInput) (string, string) {
	var settings struct {
		Model       string `json:"model"`
		Effort      string `json:"reasoning_effort"`
		AgentLaunch struct {
			Model  string `json:"model"`
			Effort string `json:"reasoning_effort"`
		} `json:"agent_launch"`
	}
	_ = json.Unmarshal(execution.Settings, &settings)
	if execution.Adapter == "orca-terminal" {
		return settings.AgentLaunch.Model, settings.AgentLaunch.Effort
	}
	return settings.Model, settings.Effort
}

func (p *environmentPort) supportsEnvironment(key string) bool {
	for _, profile := range p.config.RunnerExecution.Profiles {
		if profile.Workflow.ID != "development" && profile.Workflow.ID != "development-audited" {
			continue
		}
		actual, err := env.Key(profile.Execution)
		if err == nil && actual == key {
			return true
		}
	}
	return false
}
