package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

func TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	require(t, os.MkdirAll(EtcDir(), 0700))
	configPath := filepath.Join(EtcDir(), "one.json")
	require(t, os.WriteFile(configPath, projectConfigJSON(t, c), 0600))
	ctx := context.Background()
	d := NewApplicationDirectory(HomeDir())
	d.Start()
	switchCatalog(t, d, func(v contract.Catalog) bool { return len(v.Projects) == 1 && v.Projects[0].RuntimeState == "ready" })
	require(t, d.Close())
	p := d.ShutdownResults()[0]
	c, err := LoadQueryConfig(configPath)
	require(t, err)
	writer, err := OpenNativeTaskCommandStore(ctx, &c)
	require(t, err)
	snapshot, err := writer.ReadRunnerSnapshot(ctx, c.Identity())
	require(t, err)
	cp := *snapshot.Checkpoint
	process, err := httpclient.CurrentProcessIdentity()
	require(t, err)
	process.Created = "previous-os-generation"
	cp.Service.ProcessIdentity.Created = process.Created
	_, err = writer.SaveCheckpoint(ctx, cp)
	require(t, err)
	require(t, writer.Close())
	homeID, err := localservice.HomeID(HomeDir())
	require(t, err)
	id := contract.Identity{Protocol: contract.Protocol, HomeID: homeID, InstanceID: localservice.Token(), Process: process, Address: "127.0.0.1:18742", Ready: true}
	backend, _ := components(HomeDir())
	require(t, localservice.Write(backend.dir, "instance.json", id))
	shared := d.runner.result()
	shared.ResourcesSettled = false
	p.Diagnostic = "Runner service stopped; original native resource settlement remains unverified"
	original := contract.Shutdown{Version: 2, Runner: shared, Identity: id, Projects: []contract.ProjectShutdown{p}, Clean: false, Diagnostic: p.ConfigID + ": " + p.Diagnostic}
	require(t, localservice.Write(backend.dir, "shutdown.json", original))
	if err := StopAll(ctx); err == nil {
		t.Fatal("original dirty receipt unexpectedly passed ordinary stop")
	}
	paths := []string{filepath.Join(backend.dir, "shutdown.json"), filepath.Join(c.RunDir, "steerline.db"), filepath.Join(runnerstatepath.Directory(c.runnerHome()), "idle-fixture.json"), GlobalExecutionConfigPath()}
	before := map[string][]byte{}
	for _, path := range paths {
		before[path], err = os.ReadFile(path)
		require(t, err)
	}
	exe, err := os.Executable()
	require(t, err)
	result, err := VerifyApplicationStopped(ctx, exe)
	require(t, err)
	if result.OriginalHost != id || len(result.VerifierSHA256) != 64 || len(result.Projects) != 1 || !result.Runner.ResourcesSettled {
		t.Fatal(result)
	}
	for _, path := range paths {
		after, err := os.ReadFile(path)
		require(t, err)
		if !bytes.Equal(before[path], after) {
			t.Fatal("verification changed original evidence or state", path)
		}
	}
	lock, err := host.LockRepository(c.RunDir)
	require(t, err)
	if _, err = VerifyApplicationStopped(ctx, exe); err == nil {
		lock.Close()
		t.Fatal("active project lock accepted")
	}
	lock.Close()
	global := before[GlobalExecutionConfigPath()]
	require(t, os.WriteFile(GlobalExecutionConfigPath(), append(append([]byte{}, global...), '\n'), 0600))
	if _, err = VerifyApplicationStopped(ctx, exe); err == nil {
		t.Fatal("changed frozen configuration accepted")
	}
	require(t, os.WriteFile(GlobalExecutionConfigPath(), global, 0600))
	original.Projects[0].LocalClosed = false
	require(t, localservice.Write(backend.dir, "shutdown.json", original))
	if _, err = VerifyApplicationStopped(ctx, exe); err == nil {
		t.Fatal("unclosed original runtime accepted")
	}
}

func TestDeploymentCandidateVerificationKeepsOriginalBinding(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runner")
	proof := runnerclient.StoppedVerification{StateDir: dir, ResourcesSettled: true, Runs: []runnerclient.StoppedRun{{RunID: "original", InputDigest: strings.Repeat("a", 64)}}}
	c := runnerstate.Candidate{StateDir: dir, RunID: "original", InputDigest: proof.Runs[0].InputDigest}
	require(t, matchStoppedCandidates([]runnerstate.Candidate{c}, proof))
	for _, alter := range []func(*runnerstate.Candidate){
		func(c *runnerstate.Candidate) { c.RunID = "missing" },
		func(c *runnerstate.Candidate) { c.StateDir += "-other" },
		func(c *runnerstate.Candidate) { c.InputDigest = strings.Repeat("b", 64) },
	} {
		bad := c
		alter(&bad)
		if err := matchStoppedCandidates([]runnerstate.Candidate{bad}, proof); err == nil {
			t.Fatal("changed original binding accepted")
		}
	}
}
