package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/internal/testmanagement"
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

	t.Run("same_build_restart_retains_native_unknown", func(t *testing.T) {
		original.Projects[0].LocalClosed = true
		require(t, localservice.Write(backend.dir, "shutdown.json", original))
		fixturePath := filepath.Join(runnerstatepath.Directory(c.runnerHome()), "idle-fixture.json")
		var info runnerclient.ServiceInfo
		require(t, json.Unmarshal(before[fixturePath], &info))
		info.LastShutdown.ResourcesSettled = false
		info.LastShutdown.Diagnostic = "Original Orca terminal is still active"
		require(t, os.WriteFile(fixturePath, mustJSON(info), 0600))
		for _, path := range paths {
			before[path], err = os.ReadFile(path)
			require(t, err)
		}
		proof, err := VerifyApplicationRestartable(ctx, exe)
		require(t, err)
		if !proof.Restartable || proof.RunnerService.ResourcesSettled || proof.RunnerService.Diagnostic != info.LastShutdown.Diagnostic || proof.RunnerBuild.SHA256 != proof.VerifierSHA256 || proof.OriginalHost != id {
			t.Fatal("restart fabricated native settlement or changed ownership", proof)
		}
		for _, path := range paths {
			after, err := os.ReadFile(path)
			require(t, err)
			if !bytes.Equal(before[path], after) {
				t.Fatal("restart verification changed original records", path)
			}
		}
		lock, err := host.LockRepository(c.RunDir)
		require(t, err)
		_, verifyErr := VerifyApplicationRestartable(ctx, exe)
		lock.Close()
		if verifyErr == nil {
			t.Fatal("restart accepted an active project writer")
		}
		changedExe := filepath.Join(t.TempDir(), "different-runner.exe")
		require(t, os.WriteFile(changedExe, []byte("another build"), 0600))
		if _, err := VerifyApplicationRestartable(ctx, changedExe); err == nil {
			t.Fatal("restart accepted a different Runner executable")
		}
		info.Status = "ready"
		require(t, os.WriteFile(fixturePath, mustJSON(info), 0600))
		if _, err := VerifyApplicationRestartable(ctx, exe); err == nil {
			t.Fatal("restart accepted an active Runner writer")
		}
		info.Status = "not_running"
		require(t, os.WriteFile(fixturePath, mustJSON(info), 0600))
		original.Projects[0].LocalClosed = false
		require(t, localservice.Write(backend.dir, "shutdown.json", original))
		if _, err := VerifyApplicationRestartable(ctx, exe); err == nil {
			t.Fatal("restart accepted unclosed project resources")
		}
		original.Projects[0].LocalClosed = true
		require(t, localservice.Write(backend.dir, "shutdown.json", original))
		require(t, os.WriteFile(GlobalExecutionConfigPath(), append(append([]byte{}, global...), '\n'), 0600))
		if _, err := VerifyApplicationRestartable(ctx, exe); err == nil {
			t.Fatal("restart accepted changed frozen configuration")
		}
	})

	t.Run("cross_build_generation_uses_selected_candidate_proof", func(t *testing.T) {
		require(t, os.WriteFile(GlobalExecutionConfigPath(), global, 0600))
		original.Projects[0].LocalClosed = true
		require(t, localservice.Write(backend.dir, "shutdown.json", original))
		fixturePath := filepath.Join(runnerstatepath.Directory(c.runnerHome()), "idle-fixture.json")
		var info runnerclient.ServiceInfo
		require(t, json.Unmarshal(before[fixturePath], &info))
		info.LastShutdown.ResourcesSettled = false
		info.LastShutdown.Diagnostic = "original native resources remain unverified"
		require(t, os.WriteFile(fixturePath, mustJSON(info), 0600))

		// A historical generation, the last resident Runner, and the staged
		// verifier have three different builds, as in successive deployments.
		g := testmanagement.Generation{ID: strings.Repeat("e", 32), Project: c.Identity(), Revision: 1, Runner: d.runner.client.BuildPin(), State: "accepted", RunID: "original-generation", InputDigest: "original-generation-input"}
		g.Runner.SHA256 = strings.Repeat("a", 64)
		g.Runner.Version = "historical"
		g.Request = runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "original-generation-submit", Input: json.RawMessage(`{"title":"original"}`)}
		store, err := OpenNativeTaskCommandStore(ctx, &c)
		require(t, err)
		require(t, store.SaveGeneration(ctx, g, 0))
		require(t, store.Close())
		image, err := os.ReadFile(exe)
		require(t, err)
		candidate := filepath.Join(t.TempDir(), "candidate.exe")
		require(t, os.WriteFile(candidate, append(image, []byte("candidate-build")...), 0700))
		sha := taskPayloadDigest(append(image, []byte("candidate-build")...))
		takeover := runnercontract.TakeoverReport{Contract: "steerrunner-takeover/v1", StateDir: g.Runner.StateDir, CandidateSHA256: sha, Compatible: true, Items: []runnercontract.TakeoverItem{{Kind: "run", ID: g.RunID, InputDigest: g.InputDigest, Revision: 3, Status: "compatible"}}}
		takeoverPath := filepath.Join(g.Runner.StateDir, "idle-takeover-fixture.json")
		require(t, os.WriteFile(takeoverPath, mustJSON(takeover), 0600))
		for _, path := range paths {
			before[path], err = os.ReadFile(path)
			require(t, err)
		}
		proof, err := VerifyApplicationRestartable(ctx, candidate)
		require(t, err)
		if !proof.Restartable || proof.RunnerService.ResourcesSettled || proof.RunnerService.Diagnostic != info.LastShutdown.Diagnostic || proof.CandidateBuild == nil || proof.CandidateBuild.SHA256 != sha || proof.RunnerBuild.SHA256 != original.Runner.ExecutableSHA256 {
			t.Fatal("restart confused original and selected builds or native settlement", proof)
		}
		for _, path := range paths {
			after, err := os.ReadFile(path)
			require(t, err)
			if !bytes.Equal(before[path], after) {
				t.Fatal("cross-build verification changed original facts", path)
			}
		}
		for _, fault := range []string{"input", "namespace", "candidate", "missing", "read_only"} {
			t.Run(fault, func(t *testing.T) {
				var bad runnercontract.TakeoverReport
				require(t, json.Unmarshal(mustJSON(takeover), &bad))
				switch fault {
				case "input":
					bad.Items[0].InputDigest = "foreign"
				case "namespace":
					bad.StateDir += "-foreign"
				case "candidate":
					bad.CandidateSHA256 = g.Runner.SHA256
				case "missing":
					bad.Items = []runnercontract.TakeoverItem{}
				case "read_only":
					bad.Items[0].Status = "read_only"
				}
				require(t, os.WriteFile(takeoverPath, mustJSON(bad), 0600))
				if _, err := VerifyApplicationRestartable(ctx, candidate); err == nil {
					t.Fatal("accepted invalid generation takeover", fault)
				}
			})
		}
	})
}

func TestDeploymentRestartCandidateBindingsPreserveOriginalState(t *testing.T) {
	home := t.TempDir()
	c := Config{executionPath: filepath.Join(home, "execution.json")}
	pin := runnercontract.BuildPin{Executable: filepath.Join(home, "runner.exe"), Version: "fixed", SHA256: strings.Repeat("a", 64), ResourceDigest: strings.Repeat("b", 64), StateDir: runnerstatepath.Directory(home)}
	c.RunnerExecution.Runner = runnerclient.Selection{Executable: pin.Executable, Version: pin.Version, SHA256: pin.SHA256}
	cp := &hostcontract.ServiceCheckpoint{Service: hostcontract.ServiceState{RunnerBuild: &pin}}
	for _, status := range []string{"outcome_unknown", "accepted", "rejected"} {
		candidate := runnerstate.Candidate{ID: "candidate", RunnerBuild: pin, StateDir: pin.StateDir, RunID: "original", InputDigest: strings.Repeat("c", 64), SubmitState: status, PRURL: "https://github.com/owner/repo/pull/1", Diagnostic: "Original resource remains unverified"}
		original := mustJSON(candidate)
		require(t, matchRestartBindings(c, cp, []runnerstate.Candidate{candidate}, pin))
		if !bytes.Equal(original, mustJSON(candidate)) {
			t.Fatal("restart rewrote candidate facts")
		}
		for _, alter := range []func(*runnerstate.Candidate){
			func(r *runnerstate.Candidate) { r.RunnerBuild.SHA256 = strings.Repeat("d", 64) },
			func(r *runnerstate.Candidate) { r.StateDir += "-other" },
			func(r *runnerstate.Candidate) { r.InputDigest = "" },
		} {
			bad := candidate
			alter(&bad)
			if matchRestartBindings(c, cp, []runnerstate.Candidate{bad}, pin) == nil {
				t.Fatal("restart accepted a different original binding")
			}
		}
	}
	unknown := runnerstate.Candidate{ID: "unknown-submit", RunnerBuild: pin, StateDir: pin.StateDir, SubmitState: "outcome_unknown"}
	require(t, matchRestartBindings(c, cp, []runnerstate.Candidate{unknown}, pin))

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
