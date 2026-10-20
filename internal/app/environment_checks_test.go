package app

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/environmentcheck"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/testkit/checkfixture"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestEnvironmentCheckAfterTakeoverUsesCurrentTransportKeepsOriginalSelection(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	ctx := context.Background()
	cli, _, err := c.RunnerExecution.Runner.Open(ctx, runnerstatepath.Directory(c.runnerHome()), 5*time.Second)
	require(t, err)
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "checks.db"))
	require(t, err)
	defer store.Close()
	f := checkfixture.Load()
	original := runnerclient.Selection{Executable: filepath.Join(t.TempDir(), "removed-original.exe"), Version: "original", SHA256: strings.Repeat("b", 64)}
	row := runnerstate.EnvironmentCheckRecord{StateDir: cli.StateDir, Project: c.Identity(), ID: strings.Repeat("1", 32), Revision: 1, Selection: runnerstate.EnvironmentRunnerPin(original), ResourceDigest: f.ResourceDigest, Input: f.Input, Capabilities: runnercontract.CheckCapabilities(f.Capabilities), CheckID: f.Inspection.Check.CheckID, Report: f.Inspection.Check, CreatedAt: time.Now().UTC(), Operations: []runnerstate.EnvironmentCheckOperation{{RequestID: strings.Repeat("1", 32), Status: "applied"}}}
	require(t, store.SaveCheck(ctx, row, 0))
	proof := runnerclient.TakeoverReport{Contract: "steerrunner-takeover/v1", StateDir: cli.StateDir, CandidateSHA256: cli.SHA256, Compatible: true, Items: []runnerclient.TakeoverItem{{ID: row.CheckID, Kind: "check", Revision: row.Report.Revision, InputDigest: row.Report.InputDigest, Status: "compatible"}}}
	require(t, runnerstatepath.Prepare(c.runnerHome()))
	require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-takeover-fixture.json"), mustJSON(proof), 0600))
	port := newEnvironmentCheckPort(c, store)
	transport, err := port.service.Open(ctx, original)
	require(t, err)
	current, ok := transport.(runnerclient.Client)
	if !ok || current.SHA256 != cli.SHA256 || current.Executable != cli.Executable {
		t.Fatal("saved check still requires removed original image")
	}
	rows, err := store.Checks(ctx, c.Identity())
	require(t, err)
	if len(rows) != 1 || rows[0].Selection != row.Selection || rows[0].ResourceDigest != row.ResourceDigest {
		t.Fatal("check facts rewritten")
	}
	proof.Items[0].InputDigest = "foreign"
	require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-takeover-fixture.json"), mustJSON(proof), 0600))
	if _, err := port.service.Open(ctx, original); err == nil {
		t.Fatal("foreign check used current transport")
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
