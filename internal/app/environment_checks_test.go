package app

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/environmentcheck"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentCheckProjectInput(t *testing.T) {
	cfg, sha := nativeTaskFixture(t)
	fixtureGit(t, cfg.Repository, "update-ref", "refs/remotes/origin/main", sha)
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "checks.db"))
	require(t, err)
	defer store.Close()
	port := newEnvironmentCheckPort(cfg, store)
	execution := cfg.RunnerExecution.Profiles[0].Execution
	input, err := port.service.ResolveInput(context.Background(), execution)
	require(t, err)
	if input.Repository.BaseRevision != sha || input.Repository.BaseBranch != "main" || input.Repository.Remote != "origin" || input.Repository.URL != "https://github.com/owner/repo.git" || input.Publication == nil || input.Publication.Repository != cfg.PR.Repository || input.Publication.Provider != cfg.PR.Kind || input.Correlation["project"] != cfg.Identity() {
		t.Fatalf("project input lost: %+v", input)
	}
	view, err := projectEnvironmentCheck(environmentcheck.Record{Input: input, ID: "check", Project: cfg.Identity()})
	require(t, err)
	var projected runnerclient.CheckInput
	require(t, json.Unmarshal(view.Input, &projected))
	if projected.Publication == nil || projected.Publication.Repository != cfg.PR.Repository {
		t.Fatal("HTTP projection omitted publication")
	}
	cfg.Branch = "missing"
	missing := newEnvironmentCheckPort(cfg, store)
	if _, err = missing.service.ResolveInput(context.Background(), execution); err == nil {
		t.Fatal("missing source base silently guessed")
	}
}

func TestEnvironmentCheckReusesProjectRepositoryGuards(t *testing.T) {
	cfg, sha := nativeTaskFixture(t)
	fixtureGit(t, cfg.Repository, "update-ref", "refs/remotes/origin/main", sha)
	execution := cfg.RunnerExecution.Profiles[0].Execution
	wrong := cfg
	wrong.PR.Repository = "other/repo"
	if _, err := newEnvironmentCheckPort(wrong, nil).service.ResolveInput(context.Background(), execution); err == nil {
		t.Fatal("check bypassed configured publication repository binding")
	}
	fixtureGit(t, cfg.Repository, "remote", "set-url", "origin", "https://fixture-user:fixture-secret@github.com/owner/repo.git")
	input, err := newEnvironmentCheckPort(cfg, nil).service.ResolveInput(context.Background(), execution)
	if err == nil || strings.Contains(err.Error(), "fixture-secret") || input.Repository.URL != "" {
		t.Fatal("credential-bearing remote entered public check input")
	}
}
