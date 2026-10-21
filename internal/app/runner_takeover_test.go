package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

func TestSharedRunnerCompatibleBuildUsesPublicPreflight(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	ctx := context.Background()
	cli, _, err := c.RunnerExecution.Runner.Open(ctx, runnerstatepath.Directory(c.runnerHome()), 5*time.Second)
	require(t, err)
	old := cli.BuildPin()
	old.SHA256 = strings.Repeat("b", 64)
	old.Version = "original"
	old.ResourceDigest = strings.Repeat("c", 64)
	require(t, bindApplicationRunnerBuild(c.runnerHome(), old))
	require(t, runnerstatepath.Prepare(c.runnerHome()))
	proof := runnercontract.TakeoverReport{Contract: "steerrunner-takeover/v1", StateDir: cli.StateDir, CandidateSHA256: cli.SHA256, Compatible: true, Items: []runnercontract.TakeoverItem{}}
	require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-takeover-fixture.json"), mustJSON(proof), 0600))
	var owner applicationRunner
	require(t, owner.start(ctx, c.RunnerExecution, c.runnerHome(), 5*time.Second))
	defer owner.shutdown(ctx)
	if owner.client.Takeover == nil || owner.client.SHA256 != cli.SHA256 || owner.info.ExecutableSHA256 != cli.SHA256 {
		t.Fatal("compatible candidate was not bound to current service")
	}
	var current runnercontract.BuildPin
	raw, err := os.ReadFile(filepath.Join(c.runnerHome(), "application-api", "runner-build.json"))
	require(t, err)
	require(t, json.Unmarshal(raw, &current))
	if !runnerbinding.SameBuild(current, cli.BuildPin()) {
		t.Fatal("current service pin was not updated")
	}
}

func TestTakeoverKeepsFrozenCandidatesAndRequiresOriginalRequestDigest(t *testing.T) {
	home := t.TempDir()
	old := runnercontract.BuildPin{Executable: filepath.Join(home, "old.exe"), StateDir: runnerstatepath.Directory(home), SHA256: strings.Repeat("a", 64), Version: "original", ResourceDigest: strings.Repeat("b", 64)}
	current := old
	current.SHA256 = strings.Repeat("c", 64)
	current.Version = "candidate"
	proof := runnercontract.TakeoverReport{Contract: "steerrunner-takeover/v1", StateDir: current.StateDir, CandidateSHA256: current.SHA256, Compatible: true, Items: []runnercontract.TakeoverItem{{ID: "original-run", Kind: "run", Revision: 7, InputDigest: "original-input", Status: "compatible"}}}
	candidate := runnerstate.Candidate{ID: "candidate", StateDir: old.StateDir, RunnerBuild: old, RunID: "original-run", InputDigest: "original-input", PRURL: "https://example.invalid/pr/1", StopRequested: true}
	before := mustJSON(candidate)
	c := Config{executionPath: filepath.Join(home, "execution.json")}
	c.RunnerExecution.Runner = runnerclient.Selection{Executable: current.Executable, Version: current.Version, SHA256: current.SHA256}
	cp := &hostcontract.ServiceCheckpoint{Service: hostcontract.ServiceState{RunnerBuild: &current}}
	require(t, matchRestartBindings(c, cp, []runnerstate.Candidate{candidate}, nil, current, &proof))
	if !bytes.Equal(before, mustJSON(candidate)) {
		t.Fatal("takeover rewrote original build or PR/stop facts")
	}
	proof.Items[0].InputDigest = "foreign"
	if matchRestartBindings(c, cp, []runnerstate.Candidate{candidate}, nil, current, &proof) == nil {
		t.Fatal("foreign input accepted")
	}
	unknown := candidate
	unknown.RunID = ""
	unknown.InputDigest = ""
	unknown.Submit = runnercontract.Request{Protocol: runnerclient.Protocol, RequestID: "lost-submit", Input: json.RawMessage(`{"title":"original","n":1234567890123456789}`)}
	proof.Items[0].RequestID = unknown.Submit.RequestID
	proof.Items[0].RequestDigest = runnercontract.RequestDigest("submit", "", unknown.Submit)
	if _, ok := runnerbinding.Candidate(proof, unknown); !ok {
		t.Fatal("lost response could not correlate original accepted request")
	}
	unknown.Submit.Input = json.RawMessage(`{"title":"replacement","n":1234567890123456789}`)
	if _, ok := runnerbinding.Candidate(proof, unknown); ok {
		t.Fatal("changed envelope correlated by ID alone")
	}
}

func TestStartupCancellationOutboxIsDurableBeforeRunnerStart(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	prepareStoppedRunRepository(t, c)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	require(t, os.MkdirAll(EtcDir(), 0700))
	require(t, os.WriteFile(filepath.Join(EtcDir(), "one.json"), projectConfigJSON(t, c), 0600))
	d, r := startPresetDirectory(t)
	candidate := seedStoppedPublicRun(t, r, c)
	require(t, d.Close())
	ctx := context.Background()
	store, err := OpenNativeTaskCommandStore(ctx, &c)
	require(t, err)
	candidate, err = store.Candidate(ctx, c.Identity(), candidate.ID)
	require(t, err)
	expected := candidate.Revision
	candidate.Revision++
	candidate.StopRequested = true
	candidate.InputsRevoked = false
	candidate.Stop = nil
	require(t, store.SaveCandidate(ctx, candidate, expected, nil))
	require(t, store.Close())
	cli := r.runner.client
	cli.InstanceID = ""
	proof := runnercontract.TakeoverReport{Contract: "steerrunner-takeover/v1", StateDir: cli.StateDir, CandidateSHA256: cli.SHA256, Compatible: true, Items: []runnercontract.TakeoverItem{{ID: candidate.RunID, Kind: "run", Revision: 7, InputDigest: candidate.InputDigest, Status: "compatible"}}}
	cli.Takeover = &proof
	stops, err := prepareProjectRunnerStops(ctx, &c, &cli)
	require(t, err)
	if len(stops) != 1 || stops[0].RunID != candidate.RunID || string(stops[0].Request.Input) != `{"expected_revision":7}` {
		t.Fatal("startup stop did not bind original Run/revision", stops)
	}
	again, err := prepareProjectRunnerStops(ctx, &c, &cli)
	require(t, err)
	if !bytes.Equal(mustJSON(stops), mustJSON(again)) {
		t.Fatal("restart replaced original stop envelope")
	}
	reader, err := OpenNativeTaskReader(ctx, &c)
	require(t, err)
	defer reader.Close()
	saved, err := reader.Candidate(ctx, c.Identity(), candidate.ID)
	require(t, err)
	if saved.Stop == nil || saved.Stop.State != "unknown" || saved.Stop.Request.RequestID != stops[0].Request.RequestID || saved.RunnerBuild != candidate.RunnerBuild || saved.RunID != candidate.RunID || saved.Submit.RequestID != candidate.Submit.RequestID {
		t.Fatal("startup outbox was not durable or rewrote frozen facts", saved)
	}
	info, err := cli.ServiceStatus(ctx)
	require(t, err)
	if info.Status != "not_running" {
		t.Fatal("outbox preparation started Runner")
	}
}
