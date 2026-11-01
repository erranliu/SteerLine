package app

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	model "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

func TestEnvironmentGenerationTargetIndependentAndBootstrap(t *testing.T) {
	p, c, _ := environmentFixture(t)
	ctx := context.Background()
	key, _ := model.Key(c.RunnerExecution.Profiles[1].Execution)
	for _, path := range []string{model.DocumentPath(key), model.ScriptPath(key, "ps1")} {
		require(t, os.Remove(filepath.Join(c.Repository, path)))
	}
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "first generation has no outputs")
	fixtureGit(t, c.Repository, "push", "origin", "main")
	prepared, err := p.prepareGeneration(ctx, model.Generate{TargetKey: key, Profile: "fixture"})
	require(t, err)
	if prepared.Input.Execution.Target.Kind != "local" || prepared.Target.Target.Kind != "orca_environment" {
		t.Fatal("generation location replaced target")
	}
	if !strings.Contains(string(prepared.Context), key) || strings.Contains(string(prepared.Context), prepared.Input.Generate.Repository.Commit) {
		t.Fatal("target lost or self-referencing commit")
	}
	for _, path := range prepared.Input.Generate.Repository.Paths {
		if path == model.DocumentPath(key) || path == model.ScriptPath(key, "ps1") {
			t.Fatal("first generation depends on missing output")
		}
	}
	if _, err := p.prepareGeneration(ctx, model.Generate{TargetKey: strings.Repeat("f", 64), Profile: "fixture"}); err == nil {
		t.Fatal("unknown target accepted")
	}
}

func TestEnvironmentContextExportDoesNotExposeUnknownSettings(t *testing.T) {
	p, _, _ := environmentFixture(t)
	var settings map[string]any
	require(t, json.Unmarshal(p.config.RunnerExecution.Profiles[0].Execution.Settings, &settings))
	settings["api_key"] = "fixture-secret"
	settings["args"] = []string{"--token=fixture-secret"}
	settings["transport"] = map[string]string{"secret": "fixture-secret"}
	raw, _ := json.Marshal(settings)
	p.config.RunnerExecution.Profiles[0].Execution.Settings = raw
	key, _ := model.Key(p.config.RunnerExecution.Profiles[0].Execution)
	body, err := p.generationContext(context.Background(), key)
	require(t, err)
	for _, s := range []string{"fixture-secret", "api_key", `"transport":`, `"args":`, `"settings":`} {
		if strings.Contains(string(body), s) {
			t.Fatal("private setting exported", s)
		}
	}
	if !strings.Contains(string(body), "native-fixture") {
		t.Fatal("public executable omitted")
	}
	value, err := p.materials(context.Background(), key)
	require(t, err)
	if value.(map[string]any)["target_facts"] == nil || value.(map[string]any)["expected_context"] != nil {
		t.Fatal("missing output prevented context export")
	}
}

func TestEnvironmentHistoryFiltersBeforeTwentyAndKeepsRemovedTarget(t *testing.T) {
	p, c, store := environmentFixture(t)
	ctx := context.Background()
	key, _ := model.Key(c.RunnerExecution.Profiles[0].Execution)
	want := ""
	for i := 0; i < 26; i++ {
		profile := c.RunnerExecution.Profiles[1]
		if i == 0 {
			profile = c.RunnerExecution.Profiles[0]
		}
		r := model.Record{ID: model.ID(), Project: c.Identity(), Revision: 1, Kind: "adaptation", Execution: profile.Execution, State: "failed", CreatedAt: time.Now().UTC(), Requests: []model.Request{{ID: model.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []model.Round{}}
		require(t, store.SaveEnvironment(ctx, r, 0))
		if i == 0 {
			want = r.ID
		}
	}
	p.config.RunnerExecution.Profiles = nil
	port := runtimeProjectPort{config: p.config, store: store}
	port.config.runtimeEnvironment = p
	value, err := port.environmentRead(ctx, "/environment", url.Values{"environment_key": {key}})
	require(t, err)
	body, _ := json.Marshal(value)
	if !strings.Contains(string(body), want) || strings.Count(string(body), `"id":`) != 1 {
		t.Fatal("selected history lost before filtering", string(body))
	}
	if _, err := port.environmentRead(ctx, "/environment", url.Values{"environment_key": {key, key}}); err == nil {
		t.Fatal("duplicate filter accepted")
	}
}
