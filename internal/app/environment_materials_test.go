package app

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	model "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

func TestEnvironmentMaterialQueryDoesNotExposeExecutionSecrets(t *testing.T) {
	p, c, _ := environmentFixture(t)
	key, _ := model.Key(c.RunnerExecution.Profiles[0].Execution)
	value, e := p.materials(context.Background(), key)
	require(t, e)
	raw, _ := json.Marshal(value)
	if strings.Contains(string(raw), "settings") || strings.Contains(string(raw), "target_facts") || strings.Contains(string(raw), "generation_ready") {
		t.Fatal("query exposed retired generation or execution settings", string(raw))
	}
	before := fixtureGit(t, c.Repository, "rev-parse", "HEAD")
	_, e = p.materials(context.Background(), key)
	require(t, e)
	after := fixtureGit(t, c.Repository, "rev-parse", "HEAD")
	if before != after {
		t.Fatal("query wrote project")
	}
}

func TestEnvironmentHistoryFiltersBeforeTwentyAndKeepsRemovedTarget(t *testing.T) {
	p, c, store := environmentFixture(t)
	ctx := context.Background()
	key, _ := model.Key(c.RunnerExecution.Profiles[0].Execution)
	want := ""
	wantCreatedAt := time.Time{}
	for i := 0; i < 26; i++ {
		profile := c.RunnerExecution.Profiles[1]
		if i == 0 {
			profile = c.RunnerExecution.Profiles[0]
		}
		r := model.Record{ID: model.ID(), Project: c.Identity(), Revision: 1, Kind: "adaptation", Execution: profile.Execution, State: "failed", CreatedAt: time.Now().UTC(), Requests: []model.Request{{ID: model.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []model.Round{}}
		require(t, store.SaveEnvironment(ctx, r, 0))
		if i == 0 {
			want = r.ID
			wantCreatedAt = r.CreatedAt
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
	var history struct {
		Records []struct {
			CreatedAt time.Time `json:"created_at"`
		} `json:"records"`
	}
	require(t, json.Unmarshal(body, &history))
	if len(history.Records) != 1 || !history.Records[0].CreatedAt.Equal(wantCreatedAt) {
		t.Fatal("history lost original detection time", string(body))
	}
	if _, err := port.environmentRead(ctx, "/environment", url.Values{"environment_key": {key, key}}); err == nil {
		t.Fatal("duplicate filter accepted")
	}
}
