package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestDeploymentRecoveryReadiness(t *testing.T) {
	for _, state := range []string{"starting", "offline", "failed", "ready"} {
		t.Run(state, func(t *testing.T) {
			t.Setenv("STEERLINE_HOME", t.TempDir())
			backend, web := components(HomeDir())
			endpoint, err := localservice.Prepare(HomeDir(), backend.dir, backend.protocol, "127.0.0.1:0")
			require(t, err)
			var polls atomic.Int32
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !endpoint.Authorized(r) {
					w.WriteHeader(403)
					return
				}
				if r.URL.Path == backend.identity {
					localservice.JSON(w, 200, endpoint.Identity)
					return
				}
				if r.URL.Path != "/v2/application/projects" {
					w.WriteHeader(404)
					return
				}
				n := polls.Add(1)
				project := contract.Project{RuntimeState: state, Online: state == "ready"}
				if state == "offline" {
					project.RuntimeState = "ready"
				}
				if state == "starting" && n >= 3 {
					project.RuntimeState, project.Online = "ready", true
				}
				localservice.JSON(w, 200, contract.Catalog{Projects: []contract.Project{project}})
			})}
			go server.Serve(endpoint.Listener)
			t.Cleanup(func() { server.Close(); endpoint.Close() })
			startupEndpoint(t, HomeDir(), web, endpoint)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err = waitDeploymentRecoveryReady(ctx)
			if state == "failed" || state == "offline" {
				if err == nil {
					t.Fatal("recovery accepted an unavailable project", state)
				}
				return
			}
			require(t, err)
			if state == "starting" && polls.Load() < 3 {
				t.Fatal("recovery stopped before projects became ready")
			}
		})
	}
}

func TestDeploymentInterruptedRecovery(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	exe, err := os.Executable()
	require(t, err)
	image, err := os.ReadFile(exe)
	require(t, err)
	require(t, os.MkdirAll(BinDir(), 0700))
	backend, web := components(HomeDir())
	runner := filepath.Join(BinDir(), "steerrunner.exe")
	require(t, os.WriteFile(runner, image, 0700))
	require(t, os.WriteFile(backend.exe, []byte("original application"), 0700))
	require(t, os.WriteFile(web.exe, []byte("original Web"), 0700))
	global := c.RunnerExecution
	global.Runner.Executable = runner
	require(t, os.WriteFile(GlobalExecutionConfigPath(), mustJSON(global), 0600))
	require(t, os.MkdirAll(EtcDir(), 0700))
	configPath := filepath.Join(EtcDir(), "one.json")
	require(t, os.WriteFile(configPath, projectConfigJSON(t, c), 0600))
	c, err = LoadConfig(configPath)
	require(t, err)
	require(t, c.ResolveIdentity(context.Background()))
	for _, name := range []string{"spec.md", "plan.md"} {
		require(t, os.WriteFile(filepath.Join(c.Repository, name), []byte("# Frozen task\n\n## Acceptance\n\nFixture.\n"), 0600))
	}
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "frozen task")
	d, runtime := startPresetDirectory(t)
	candidate := seedStoppedPublicRun(t, runtime, c)
	require(t, d.Close())
	ctx := context.Background()
	store, err := OpenNativeTaskCommandStore(ctx, &c)
	require(t, err)
	defer store.Close()
	snapshot, err := store.ReadRunnerSnapshot(ctx, c.Identity())
	require(t, err)
	var checkpoint hostcontract.ServiceCheckpoint
	require(t, json.Unmarshal(mustJSON(snapshot.Checkpoint), &checkpoint))
	process, err := httpclient.CurrentProcessIdentity()
	require(t, err)
	process.Created, process.Executable = "interrupted-os-generation", canonicalPath(backend.exe)
	homeID, err := localservice.HomeID(HomeDir())
	require(t, err)
	id := contract.Identity{Protocol: contract.Protocol, HomeID: homeID, InstanceID: localservice.Token(), Process: process, Address: "127.0.0.1:18742", Ready: true}
	checkpoint.Service.ProcessIdentity.Created = process.Created
	checkpoint.Service.ProcessIdentity.Executable = process.Executable
	checkpoint.Service.Status, checkpoint.Service.Ready = "running", true
	require(t, localservice.Write(backend.dir, "instance.json", id))
	require(t, localservice.Write(backend.dir, "launch.json", launchRecord{Attempt: localservice.Token(), Process: &id.Process}))
	previousID := id
	previousID.InstanceID = localservice.Token()
	previous := contract.Shutdown{Version: 2, Identity: previousID, Projects: d.ShutdownResults(), Runner: d.runner.result(), Clean: false, Diagnostic: "previous native resources unverified"}
	require(t, localservice.Write(backend.dir, "shutdown.json", previous))
	files := map[string]string{}
	for _, name := range []string{"steerline.exe", "steerline-web.exe", "steerrunner.exe"} {
		data, err := os.ReadFile(filepath.Join(BinDir(), name))
		require(t, err)
		files[name] = taskPayloadDigest(data)
	}
	record := installedDeployment{Home: HomeDir(), RunnerVersion: global.Runner.Version, Files: files}
	require(t, localservice.WritePrivateFile(HomeDir(), "deployment.json", record))
	runnerDir := d.runner.client.StateDir
	infoPath := filepath.Join(runnerDir, "idle-fixture.json")
	var info runnerclient.ServiceInfo
	data, err := os.ReadFile(infoPath)
	require(t, err)
	require(t, json.Unmarshal(data, &info))
	info.LastShutdown.ResourcesSettled = false
	info.LastShutdown.Diagnostic = "original native resources remain unverified"
	require(t, os.WriteFile(infoPath, mustJSON(info), 0600))
	takeoverPath := filepath.Join(runnerDir, "idle-takeover-fixture.json")
	takeover := runnercontract.TakeoverReport{Contract: "steerrunner-takeover/v1", StateDir: runnerDir, CandidateSHA256: global.Runner.SHA256, Compatible: true, Items: []runnercontract.TakeoverItem{{ID: candidate.RunID, Kind: "run", Revision: 1, InputDigest: candidate.InputDigest, Status: "compatible"}}}
	require(t, os.WriteFile(takeoverPath, mustJSON(takeover), 0600))
	paths := []string{GlobalExecutionConfigPath(), configPath, filepath.Join(backend.dir, "instance.json"), filepath.Join(backend.dir, "launch.json"), filepath.Join(backend.dir, "shutdown.json"), filepath.Join(backend.dir, "runner-build.json"), filepath.Join(HomeDir(), "deployment.json"), infoPath, takeoverPath, web.exe}
	baseline := map[string][]byte{}
	for _, path := range paths {
		baseline[path], err = os.ReadFile(path)
		require(t, err)
	}
	restore := func() {
		for _, path := range paths {
			require(t, os.WriteFile(path, baseline[path], 0600))
		}
		_, err := store.SaveCheckpoint(ctx, checkpoint)
		require(t, err)
	}
	complete := func(context.Context) error {
		newID := id
		newID.InstanceID, newID.Process.Created = localservice.Token(), "recovered-os-generation"
		require(t, localservice.Write(backend.dir, "instance.json", newID))
		require(t, localservice.Write(backend.dir, "launch.json", launchRecord{Attempt: localservice.Token(), Process: &newID.Process}))
		var cp hostcontract.ServiceCheckpoint
		require(t, json.Unmarshal(mustJSON(checkpoint), &cp))
		cp.Service.Status, cp.Service.Ready = "stopped", false
		cp.Service.InstanceID = strings.Repeat("d", 32)
		cp.Service.ProcessIdentity.Created = newID.Process.Created
		_, err := store.SaveCheckpoint(ctx, cp)
		require(t, err)
		current := info
		stop := *info.LastShutdown
		stop.InstanceID, stop.RequestID = "recovered-runner", "recovered-original-stop"
		current.LastShutdown = &stop
		require(t, os.WriteFile(infoPath, mustJSON(current), 0600))
		shared := *d.runner.result()
		shared.InstanceID, shared.RequestID = stop.InstanceID, stop.RequestID
		shared.ResourcesSettled, shared.Diagnostic = false, stop.Diagnostic
		project := d.ShutdownResults()[0]
		project.InstanceID = cp.Service.InstanceID
		require(t, localservice.Write(backend.dir, "shutdown.json", contract.Shutdown{Version: 2, Identity: newID, Projects: []contract.ProjectShutdown{project}, Runner: &shared, Clean: false, Diagnostic: stop.Diagnostic}))
		return nil
	}
	for _, receipt := range []string{"stale", "missing"} {
		t.Run(receipt, func(t *testing.T) {
			restore()
			if receipt == "missing" {
				require(t, os.Remove(filepath.Join(backend.dir, "shutdown.json")))
			}
			before, err := store.Candidate(ctx, c.Identity(), candidate.ID)
			require(t, err)
			calls := 0
			proof, err := recoverApplicationForDeployment(ctx, runner, func(ctx context.Context) error {
				calls++
				wait, cancel := context.WithTimeout(ctx, 60*time.Millisecond)
				defer cancel()
				if release, err := localservice.Coordinate(wait, HomeDir()); err == nil {
					release()
					t.Fatal("recovery lost startup coordination")
				}
				return complete(ctx)
			})
			require(t, err)
			if calls != 1 || !proof.Restartable || proof.Recovery == nil || proof.Recovery.InterruptedHost != id || proof.OriginalHost.InstanceID == id.InstanceID || proof.RunnerService.ResourcesSettled || proof.RunnerService.RequestID != "recovered-original-stop" {
				t.Fatal("recovery bypassed original shutdown/native facts", proof)
			}
			if (proof.Recovery.PreviousShutdown == nil) != (receipt == "missing") {
				t.Fatal("lost original receipt")
			}
			after, err := store.Candidate(ctx, c.Identity(), candidate.ID)
			require(t, err)
			if !bytes.Equal(mustJSON(before), mustJSON(after)) {
				t.Fatal("recovery rewrote candidate/Run/PR/input facts")
			}
		})
	}
	for _, mode := range []string{"update", "failed-cycle", "replace"} {
		t.Run("preparation-"+mode, func(t *testing.T) {
			restore()
			calls := 0
			goal := DeploymentUpdate
			if mode == "replace" {
				goal = DeploymentReplace
			}
			result, err := prepareApplicationDeployment(ctx, runner, goal, func(ctx context.Context) error {
				calls++
				if mode == "failed-cycle" {
					return errors.New("original startup failed")
				}
				return complete(ctx)
			})
			if mode == "update" {
				require(t, err)
				if calls != 1 || result.Source != "interrupted_recovery" || result.Restart == nil || result.Restart.Recovery == nil || result.Facts.NativeResources != "unverified" || result.Facts.WriterExit != "verified" {
					t.Fatal("composed preparation lost original recovery boundary", result, calls)
				}
			} else if err == nil || result.Outcome != "blocked" || mode == "replace" && calls != 0 || mode == "failed-cycle" && calls != 1 {
				t.Fatal("preparation retried or bypassed goal", mode, result, calls, err)
			}
		})
	}
	for _, fault := range []string{"active-host", "unknown-launch", "foreign-launch", "host-lease", "web-lease", "runner-active", "matching-receipt", "conflicting-receipt", "corrupt-receipt", "missing-release", "foreign-release", "changed-exe", "wrong-pin", "changed-config", "checkpoint-host", "project-lock", "removed-project", "incompatible", "missing-run", "wrong-input", "failed-cycle", "no-new-receipt", "changed-during-cycle"} {
		t.Run(fault, func(t *testing.T) {
			restore()
			var release func()
			mutateCP := func(f func(*hostcontract.ServiceCheckpoint)) {
				var cp hostcontract.ServiceCheckpoint
				require(t, json.Unmarshal(mustJSON(checkpoint), &cp))
				f(&cp)
				_, err := store.SaveCheckpoint(ctx, cp)
				require(t, err)
			}
			switch fault {
			case "active-host":
				live, err := httpclient.CurrentProcessIdentity()
				require(t, err)
				active := id
				active.Process = live
				require(t, localservice.Write(backend.dir, "instance.json", active))
			case "unknown-launch":
				require(t, localservice.Write(backend.dir, "launch.json", launchRecord{Attempt: localservice.Token()}))
			case "foreign-launch":
				other := id.Process
				other.Created = "foreign"
				require(t, localservice.Write(backend.dir, "launch.json", launchRecord{Attempt: localservice.Token(), Process: &other}))
			case "host-lease", "web-lease", "project-lock":
				path := backend.dir
				if fault == "web-lease" {
					path = web.dir
					require(t, os.MkdirAll(path, 0700))
				}
				if fault == "project-lock" {
					path = c.RunDir
				}
				lock, err := host.LockRepository(path)
				require(t, err)
				release = lock.Close
			case "runner-active":
				active := info
				active.Status = "ready"
				require(t, os.WriteFile(infoPath, mustJSON(active), 0600))
			case "matching-receipt", "conflicting-receipt":
				current := previous
				current.Identity = id
				if fault == "conflicting-receipt" {
					current.Identity.Process.Created = "conflicting-generation"
				}
				require(t, localservice.Write(backend.dir, "shutdown.json", current))
			case "corrupt-receipt":
				require(t, os.WriteFile(filepath.Join(backend.dir, "shutdown.json"), []byte(`{`), 0600))
			case "missing-release":
				require(t, os.Remove(filepath.Join(HomeDir(), "deployment.json")))
			case "foreign-release":
				bad := record
				bad.Home += "-foreign"
				require(t, localservice.WritePrivateFile(HomeDir(), "deployment.json", bad))
			case "changed-exe":
				require(t, os.WriteFile(web.exe, []byte("unverified replacement"), 0600))
			case "wrong-pin":
				pin := d.runner.client.BuildPin()
				pin.SHA256 = strings.Repeat("f", 64)
				require(t, localservice.Write(backend.dir, "runner-build.json", pin))
			case "changed-config":
				require(t, os.WriteFile(configPath, append(append([]byte{}, baseline[configPath]...), '\n'), 0600))
			case "checkpoint-host":
				mutateCP(func(cp *hostcontract.ServiceCheckpoint) { cp.Service.ProcessIdentity.Created = "foreign" })
			case "removed-project":
				require(t, os.Remove(configPath))
			case "incompatible", "missing-run", "wrong-input":
				var bad runnercontract.TakeoverReport
				require(t, json.Unmarshal(mustJSON(takeover), &bad))
				if fault == "incompatible" {
					bad.Compatible = false
				} else if fault == "missing-run" {
					bad.Items = []runnercontract.TakeoverItem{}
				} else {
					bad.Items[0].InputDigest = "foreign"
				}
				require(t, os.WriteFile(takeoverPath, mustJSON(bad), 0600))
			}
			if release != nil {
				defer release()
			}
			calls := 0
			_, err := recoverApplicationForDeployment(ctx, runner, func(ctx context.Context) error {
				calls++
				if fault == "failed-cycle" {
					return errors.New("original startup failed")
				}
				if fault == "no-new-receipt" {
					return nil
				}
				if fault == "changed-during-cycle" {
					require(t, os.WriteFile(GlobalExecutionConfigPath(), append(append([]byte{}, baseline[GlobalExecutionConfigPath()]...), '\n'), 0600))
					return nil
				}
				t.Fatal("unverified recovery reached original startup", fault)
				return nil
			})
			if err == nil {
				t.Fatal("unverified deployment recovered", fault)
			}
			expected := 0
			if fault == "failed-cycle" || fault == "no-new-receipt" || fault == "changed-during-cycle" {
				expected = 1
			}
			if calls != expected {
				t.Fatal("recovery retried or skipped admission", calls, fault, err)
			}
		})
	}
	restore()
}
