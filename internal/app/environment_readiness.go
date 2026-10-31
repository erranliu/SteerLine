package app

import (
	"context"
	"encoding/json"
	"errors"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type environmentProfileView struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
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

func (p *environmentPort) readinessOptions(ctx context.Context) (any, error) {
	profiles := []environmentProfileView{}
	environments := []environmentView{}
	generators := []map[string]string{}
	groups := map[string]int{}
	rows, err := p.store.Environments(ctx, p.config.Identity())
	if err != nil {
		return nil, err
	}
	commit, sourceErr := p.observedSource(ctx)
	for _, profile := range p.config.RunnerExecution.Profiles {
		if _, err := runnerclient.GenerationExecution(profile.Execution); err == nil {
			generators = append(generators, map[string]string{"id": profile.ID, "name": profile.Name})
		}
		if profile.Workflow.ID != "development" && profile.Workflow.ID != "development-audited" {
			continue
		}
		key, keyErr := env.Key(profile.Execution)
		group := key
		if keyErr != nil {
			group = "unsupported:" + profile.ID
		}
		if index, ok := groups[group]; ok {
			first := environments[index]
			v := environmentProfileView{ID: profile.ID, Name: profile.Name, Key: key, Status: first.Status, Stage: first.Stage, Reason: first.Reason, CurrentRevision: first.CurrentRevision, ActivityID: first.ActivityID}
			profiles = append(profiles, v)
			environments[index].Profiles = append(environments[index].Profiles, v)
			continue
		}
		current, record, readErr := p.current(ctx, profile.ID)
		materialErr := errors.Join(keyErr, readErr, sourceErr)
		basis := ""
		if materialErr == nil {
			prepared, err := p.materialAt(ctx, profile.ID, commit, false)
			materialErr = err
			basis = prepared.Inputs
		}
		status, stage, reason := env.View(current, record, basis, materialErr)
		if readErr != nil || keyErr != nil {
			status, stage, reason = "not_ready", "blocked", materialErr.Error()
		}
		v := environmentProfileView{ID: profile.ID, Name: profile.Name, Key: key, Status: status, Stage: stage, Reason: reason, CurrentRevision: current.Revision, ActivityID: current.ActivityID}
		name := profile.Execution.Target.Environment
		if name == "" {
			name = "本机 · " + profile.Execution.Adapter
		}
		item := environmentView{Key: key, Name: name, Profiles: []environmentProfileView{v}, Status: status, Stage: stage, Reason: reason, CurrentRevision: current.Revision, ActivityID: current.ActivityID, SourceCommit: commit, DocumentPath: env.DocumentPath}
		item.Target = profile.Execution.Target
		item.Adapter = profile.Execution.Adapter
		item.ActivitySettled = record.ID == "" || record.Settled()
		var display struct {
			Agent string `json:"agent"`
		}
		if profile.Execution.Adapter == "orca-terminal" {
			_ = json.Unmarshal(profile.Execution.Settings, &display)
		}
		item.Agent = display.Agent
		if sourceErr == nil {
			if document, err := p.read(ctx, commit, env.DocumentPath); err == nil {
				item.Document = string(document)
				if manifest, err := env.ParseManifest(document); err == nil {
					item.ScriptPath = manifest.Script
				}
			}
		}
		for _, old := range rows {
			if old.Status() != "ready" {
				continue
			}
			oldKey, err := env.Key(old.Execution)
			if err == nil && oldKey == key {
				item.LastPassedActivity = old.ID
				break
			}
		}
		profiles = append(profiles, v)
		groups[group] = len(environments)
		environments = append(environments, item)
	}
	return map[string]any{"profiles": profiles, "environments": environments, "generators": generators, "default_profile_id": p.config.RunnerExecution.DefaultProfile}, nil
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
