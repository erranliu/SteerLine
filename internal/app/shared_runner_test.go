package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/internal/testkit/checkfixture"
)

func TestSharedRunnerBuildPinBeforeService(t *testing.T) {
	home := t.TempDir()
	pin := runnercontract.BuildPin{Executable: filepath.Join(home, "runner.exe"), SHA256: strings.Repeat("a", 64), Version: "test", ResourceDigest: strings.Repeat("b", 64), StateDir: runnerstatepath.Directory(home)}
	require(t, bindApplicationRunnerBuild(home, pin))
	require(t, bindApplicationRunnerBuild(home, pin))
	if _, err := os.Stat(pin.StateDir); !os.IsNotExist(err) {
		t.Fatal("build gate created Runner state", err)
	}
	original, err := os.ReadFile(filepath.Join(home, "application-api", "runner-build.json"))
	require(t, err)
	changed := pin
	changed.SHA256 = strings.Repeat("c", 64)
	if bindApplicationRunnerBuild(home, changed) == nil {
		t.Fatal("changed build accepted")
	}
	after, err := os.ReadFile(filepath.Join(home, "application-api", "runner-build.json"))
	require(t, err)
	if string(original) != string(after) {
		t.Fatal("failed gate changed original pin")
	}
	other := t.TempDir()
	pin.StateDir = runnerstatepath.Directory(other)
	require(t, runnerstatepath.Prepare(other))
	t.Cleanup(func() { _ = runnerstatepath.RemoveStopped(other) })
	if bindApplicationRunnerBuild(other, pin) == nil {
		t.Fatal("existing state adopted without original pin")
	}
}

func TestSharedRunnerCheckBindingFailureIsProjectLocal(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	ctx := context.Background()
	require(t, os.MkdirAll(EtcDir(), 0700))
	require(t, os.WriteFile(filepath.Join(EtcDir(), "one.json"), projectConfigJSON(t, c), 0600))
	second := c
	second.Repository = filepath.Join(t.TempDir(), "second")
	fixtureGit(t, HomeDir(), "clone", "--quiet", "--no-hardlinks", c.Repository, second.Repository)
	fixtureGit(t, second.Repository, "remote", "set-url", "origin", "https://github.com/owner/repo.git")
	second.RunDir = ""
	require(t, os.WriteFile(filepath.Join(EtcDir(), "two.json"), projectConfigJSON(t, second), 0600))
	cli, _, err := c.RunnerExecution.Runner.Open(ctx, runnerstatepath.Directory(c.runnerHome()), 5*time.Second)
	require(t, err)
	require(t, bindApplicationRunnerBuild(c.runnerHome(), cli.BuildPin()))
	require(t, runnerstatepath.Prepare(c.runnerHome()))
	f := checkfixture.Load()
	record := runnerstate.EnvironmentCheckRecord{StateDir: cli.StateDir, Project: c.Identity(), ID: strings.Repeat("1", 32), Revision: 1, Selection: runnerstate.EnvironmentRunnerPin(c.RunnerExecution.Runner), ResourceDigest: f.ResourceDigest, Input: f.Input, Capabilities: runnercontract.CheckCapabilities(f.Capabilities), CheckID: f.Inspection.Check.CheckID, Report: f.Inspection.Check, CreatedAt: time.Now().UTC(), Operations: []runnerstate.EnvironmentCheckOperation{{RequestID: strings.Repeat("1", 32), Status: "applied"}}}
	reader, err := OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, reader.Close())
	store, err := OpenNativeTaskCommandStore(ctx, &c)
	require(t, err)
	require(t, store.SaveCheck(ctx, record, 0))
	require(t, store.Close())
	f.Inspection.Check.CheckID = "foreign-check"
	require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-check-fixture.json"), mustJSON(f.Inspection), 0600))
	d := NewApplicationDirectory(c.runnerHome())
	defer d.Close()
	d.Start()
	cat := switchCatalog(t, d, func(v contract.Catalog) bool {
		return len(v.Projects) == 2 && v.Projects[0].RuntimeState != "starting" && v.Projects[1].RuntimeState != "starting"
	})
	if cat.Projects[0].Online || !strings.Contains(cat.Projects[0].Diagnostic, "check binding") || !cat.Projects[1].Online {
		t.Fatal("unverified check opened project or stopped sibling", cat)
	}
	info, err := cli.ServiceStatus(ctx)
	require(t, err)
	if info.Status != "ready" || info.InstanceID != d.runner.info.InstanceID {
		t.Fatal("project check failure replaced/stopped shared service", info)
	}
	require(t, d.Close())
}

func TestSharedRunnerCloseDuringUnknownStart(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	marker := filepath.Join(t.TempDir(), "accepted")
	t.Setenv("STEERLINE_TEST_RUNNER_START_ACCEPTED", marker)
	require(t, os.MkdirAll(EtcDir(), 0700))
	require(t, os.WriteFile(filepath.Join(EtcDir(), "one.json"), projectConfigJSON(t, c), 0600))
	d := NewApplicationDirectory(c.runnerHome())
	d.Start()
	defer d.Close()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("service initialization did not reach public acceptance")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if d.Close() == nil || d.runner.result().Settled() || !d.runner.result().Attempted {
		t.Fatal("unknown initialization claimed no resources", d.runner.result())
	}
	cat, err := d.Catalog(context.Background())
	require(t, err)
	if len(cat.Projects) != 1 || cat.Projects[0].RuntimeState != "failed" || cat.Projects[0].Online {
		t.Fatal("late initialization reopened project", cat)
	}
	if _, err := os.Stat(filepath.Join(HomeDir(), "application-api", "runner-build.json")); err != nil {
		t.Fatal("lost original build gate", err)
	}
	// This public fixture has no native resources. Explicit teardown reads the
	// accepted original instance, never turns the owner's unknown result clean.
	cli, err := c.runnerClient(context.Background())
	require(t, err)
	info, err := cli.ServiceStatus(context.Background())
	require(t, err)
	require(t, cli.StopService(context.Background(), info.InstanceID, "fixture-final-stop"))
	require(t, cli.VerifyServiceSettlement(context.Background(), info.InstanceID, "fixture-final-stop"))
}

func TestSharedRunnerOfflineChecksKeepOriginalBinding(t *testing.T) {
	dir := t.TempDir()
	proof := runnerclient.StoppedVerification{StateDir: dir, ResourcesSettled: true, Checks: []runnerclient.StoppedCheck{{CheckID: "original", InputDigest: "digest", ResourcesSettled: true}}}
	check := runnerstate.EnvironmentCheckRecord{StateDir: dir, CheckID: "original", Report: &runnercontract.Check{InputDigest: "digest"}}
	require(t, matchStoppedChecks([]runnerstate.EnvironmentCheckRecord{check}, proof))
	for _, alter := range []func(*runnerstate.EnvironmentCheckRecord){
		func(c *runnerstate.EnvironmentCheckRecord) { c.StateDir += "-other" },
		func(c *runnerstate.EnvironmentCheckRecord) { c.CheckID = "missing" },
		func(c *runnerstate.EnvironmentCheckRecord) { c.Report = nil },
		func(c *runnerstate.EnvironmentCheckRecord) { c.CheckID = ""; c.Operations = nil },
		func(c *runnerstate.EnvironmentCheckRecord) {
			c.CheckID = ""
			c.Operations = []runnerstate.EnvironmentCheckOperation{{Status: "outcome_unknown"}}
		},
	} {
		bad := check
		alter(&bad)
		if matchStoppedChecks([]runnerstate.EnvironmentCheckRecord{bad}, proof) == nil {
			t.Fatal("unverified check accepted", bad)
		}
	}
	check.CheckID = ""
	check.Operations = []runnerstate.EnvironmentCheckOperation{{Status: "rejected"}}
	require(t, matchStoppedChecks([]runnerstate.EnvironmentCheckRecord{check}, proof))
}

func TestSharedRunnerRejectsBuildBeforeStart(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	cli, _, err := c.RunnerExecution.Runner.Open(context.Background(), runnerstatepath.Directory(c.runnerHome()), time.Second)
	require(t, err)
	pin := cli.BuildPin()
	pin.ResourceDigest = strings.Repeat("f", 64)
	require(t, bindApplicationRunnerBuild(c.runnerHome(), pin))
	var owner applicationRunner
	if owner.start(context.Background(), c.RunnerExecution, c.runnerHome(), time.Second) == nil || owner.attempted {
		t.Fatal("wrong build reached service start")
	}
	if _, err := os.Stat(cli.StateDir); !os.IsNotExist(err) {
		t.Fatal("wrong build created service state", err)
	}
}

func TestSharedRunnerCatalogFailureStillClosesAdoptedInstance(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	var original applicationRunner
	require(t, original.start(context.Background(), c.RunnerExecution, c.runnerHome(), 5*time.Second))
	defer original.shutdown(context.Background())
	require(t, os.WriteFile(filepath.Join(original.client.StateDir, "idle-capabilities-fixture.json"), []byte(`{}`), 0600))
	var adopted applicationRunner
	if adopted.start(context.Background(), c.RunnerExecution, c.runnerHome(), 5*time.Second) == nil {
		t.Fatal("invalid catalog accepted")
	}
	if adopted.info.InstanceID != original.info.InstanceID {
		t.Fatal("trusted original instance was lost")
	}
	require(t, adopted.shutdown(context.Background()))
	if !adopted.result().Settled() {
		t.Fatal("known shared instance was abandoned after catalog failure", adopted.result())
	}
}
