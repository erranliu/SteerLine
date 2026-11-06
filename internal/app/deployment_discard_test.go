package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestDeploymentDiscardRuns(t *testing.T) {
	t.Run("offline-flow", testDiscardOfflineFlow)
	t.Run("task-graph", func(t *testing.T) {
		c, _ := nativeTaskFixture(t)
		prepareStoppedRunRepository(t, c)
		ctx := context.Background()
		initial, err := OpenNativeTaskProject(ctx, &c)
		require(t, err)
		require(t, initial.Close())
		store, err := OpenNativeTaskCommandStore(ctx, &c)
		require(t, err)
		commit := fixtureGit(t, c.Repository, "rev-parse", "HEAD")
		ref := func(path string) taskmodel.FileReference {
			body, err := os.ReadFile(filepath.Join(c.Repository, filepath.FromSlash(path)))
			require(t, err)
			return taskmodel.FileReference{Path: path, Commit: commit, SHA256: taskPayloadDigest(body)}
		}
		spec, plan := ref("SteerlineFlow/spec/task.spec.md"), ref("SteerlineFlow/plans/task.plan.md")
		batch := taskmodel.TaskBatchInput{SchemaVersion: taskmodel.BatchSchemaVersion, ProjectID: c.Identity()}
		for i, id := range []int64{7, 42} {
			contract := taskmodel.TaskContract{ContractID: "contract-" + []string{"7", "42"}[i], Repository: "https://github.com/owner/repo.git", Remote: "origin", Branch: "main",
				SpecRef: spec, PlanRef: plan, CommonRefs: []taskmodel.FileReference{}, AcceptanceRefs: []taskmodel.FileReference{spec}}
			contract.Digest, err = taskmodel.ContractDigest(contract)
			require(t, err)
			priority := i * 4
			batch.Tasks = append(batch.Tasks, taskmodel.TaskBatchTask{LocalKey: contract.ContractID, TaskID: id, Title: "Frozen task " + contract.ContractID, Type: "task", Priority: &priority, GoalIDs: []string{"goal"}, Contract: contract})
		}
		batch.Dependencies = []taskmodel.TaskBatchEdge{{Predecessor: taskmodel.TaskBatchReference{LocalKey: "contract-7"}, Successor: taskmodel.TaskBatchReference{LocalKey: "contract-42"}}}
		_, err = NewTaskCommands(store, c).CreateTaskBatch(ctx, mustJSON(batch), strings.Repeat("7", 32), 1, "fixture")
		require(t, err)
		old, err := store.TaskGraphSnapshot(ctx, c.Identity())
		require(t, err)
		_, err = store.CommitRunnerDiscard(ctx, storageports.RunnerDiscard{
			Project: c.Identity(), RequestID: strings.Repeat("d", 32),
			Intent:           taskmodel.DiscardIntent{TaskID: "7", ExpectedVersion: old.ByID["7"].Version, Reason: "original discarded task", RequestedBy: "original operator"},
			AuthorityVersion: old.Authority.AuthorityVersion, GraphRevision: old.Authority.GraphRevision,
			CandidateRevisions: map[string]uint64{}, PRs: map[string]storageports.RunnerPRObservation{}, VerificationStarted: time.Now().UTC(),
		})
		require(t, err)
		require(t, store.Close())
		dir := filepath.Join(t.TempDir(), "prepared-project")
		counts, err := prepareDiscardedTasks(ctx, c, dir)
		require(t, err)
		if counts.Tasks != 2 || counts.Dependencies != 1 {
			t.Fatal(counts)
		}
		fresh, err := sqlite.OpenSQLiteReader(filepath.Join(dir, "steerline.db"))
		require(t, err)
		defer fresh.Close()
		graph, err := fresh.TaskGraphSnapshot(ctx, c.Identity())
		require(t, err)
		if graph.DatabaseID == old.DatabaseID || !graph.Authority.DispatchPaused || graph.Authority.DispatchMode != "manual" || !reflect.DeepEqual(graph.Edges, old.Edges) {
			t.Fatal("task graph or dispatch differs", graph)
		}
		for _, expected := range batch.Tasks {
			task := graph.ByID[expected.LocalKey[len("contract-"):]]
			if task.Status != "open" || task.Claimant != "" || task.Title != expected.Title || task.Priority != *expected.Priority || !reflect.DeepEqual(task.GoalIDs, expected.GoalIDs) {
				t.Fatal(task)
			}
			contract, err := fresh.TaskContract(ctx, c.Identity(), task)
			require(t, err)
			if !reflect.DeepEqual(contract, expected.Contract) {
				t.Fatal("contract changed", contract)
			}
		}
		snapshot, err := fresh.ReadRunnerSnapshot(ctx, c.Identity())
		require(t, err)
		if len(snapshot.Candidates) != 0 || snapshot.Checkpoint != nil {
			t.Fatal("copied execution state", snapshot)
		}
		original, err := OpenNativeTaskReader(ctx, &c)
		require(t, err)
		defer original.Close()
		before, err := original.TaskGraphSnapshot(ctx, c.Identity())
		require(t, err)
		if before.ByID["7"].Status != "discarded" || before.DatabaseID != old.DatabaseID {
			t.Fatal("preparation changed original graph")
		}
		bad := c
		bad.Branch = "foreign"
		if _, err := prepareDiscardedTasks(ctx, bad, filepath.Join(t.TempDir(), "bad")); err == nil {
			t.Fatal("foreign authority accepted")
		}
	})
	t.Run("stop-order-and-unknowns", testDiscardStopProtocol)
	t.Run("commit-and-boundaries", func(t *testing.T) {
		home := t.TempDir()
		archive := filepath.Join(home, "maintenance", "discard-runs-fixed")
		oldState, prepared := filepath.Join(home, "state"), filepath.Join(archive, "prepared-state")
		for _, dir := range []string{oldState, prepared} {
			require(t, os.MkdirAll(dir, 0700))
		}
		require(t, os.WriteFile(filepath.Join(oldState, "original"), []byte("original evidence"), 0600))
		require(t, os.WriteFile(filepath.Join(prepared, "fresh"), []byte("new graph"), 0600))
		journal := &DeploymentDiscard{Archive: archive, Resources: &runnerclient.StoppedVerification{ResourcesSettled: true}, Moves: []discardMove{{Source: oldState, Target: filepath.Join(archive, "project-state")}, {Source: prepared, Target: oldState}}}
		require(t, checkDeploymentDiscard(home))
		foreign := *journal
		foreign.Moves = []discardMove{{Source: oldState, Target: filepath.Join(t.TempDir(), "foreign")}}
		if err := commitDiscard(home, &foreign); err == nil {
			t.Fatal("foreign archive target accepted")
		}
		unsettled := *journal
		unsettled.Resources = &runnerclient.StoppedVerification{ResourcesSettled: false}
		if err := commitDiscard(home, &unsettled); err == nil {
			t.Fatal("unsettled resource proof accepted")
		}
		require(t, commitDiscard(home, journal))
		require(t, checkDeploymentDiscard(home))
		for _, path := range []string{filepath.Join(home, "state", "fresh"), filepath.Join(archive, "project-state", "original"), filepath.Join(archive, "discard.json")} {
			if _, err := os.Stat(path); err != nil {
				t.Fatal(err)
			}
		}
		if journal.Phase != "complete" {
			t.Fatal(journal)
		}
		require(t, localservice.WritePrivateFile(home, discardMarker, map[string]string{"phase": "committing"}))
		if err := checkDeploymentDiscard(home); err == nil {
			t.Fatal("interrupted swap allowed startup")
		}
		t.Setenv("STEERLINE_HOME", home)
		if _, err := StartApplication(context.Background(), true); err == nil {
			t.Fatal("start bypassed incomplete swap")
		}
		if err := ServeApplication(context.Background()); err == nil {
			t.Fatal("serve bypassed incomplete swap")
		}
		if _, err := VerifyRunnerUpgrade(context.Background(), home, "missing", "missing"); err == nil || !strings.Contains(err.Error(), "interrupted") {
			t.Fatal("upgrade bypassed incomplete swap", err)
		}
	})
}

func testDiscardOfflineFlow(t *testing.T) {
	for _, fault := range []string{"original", "saved-profile", "changed-during-preparation", "changed-runner", "missing-shutdown", "interrupted", "interrupted-wrong-host", "interrupted-wrong-repository", "interrupted-wrong-runner", "interrupted-locked", "corrupt-shutdown", "stale-shutdown"} {
		t.Run(fault, func(t *testing.T) { testDiscardOfflineConfiguration(t, fault) })
	}
}

func testDiscardOfflineConfiguration(t *testing.T, fault string) {
	c, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	exe, err := os.Executable()
	require(t, err)
	body, err := os.ReadFile(exe)
	require(t, err)
	installed := filepath.Join(HomeDir(), "bin", "steerrunner.exe")
	require(t, os.MkdirAll(filepath.Dir(installed), 0700))
	require(t, os.WriteFile(installed, body, 0700))
	global := RunnerExecutionConfig{}
	raw, err := os.ReadFile(GlobalExecutionConfigPath())
	require(t, err)
	require(t, json.Unmarshal(raw, &global))
	global.Runner.Executable = installed
	require(t, os.WriteFile(GlobalExecutionConfigPath(), mustJSON(global), 0600))
	require(t, os.MkdirAll(EtcDir(), 0700))
	configPath := filepath.Join(EtcDir(), "one.json")
	require(t, os.WriteFile(configPath, projectConfigJSON(t, c), 0600))
	ctx := context.Background()
	d := NewApplicationDirectory(HomeDir())
	d.Start()
	switchCatalog(t, d, func(cat contract.Catalog) bool { return len(cat.Projects) == 1 && cat.Projects[0].Online })
	require(t, d.Close())
	c, err = LoadQueryConfig(configPath)
	require(t, err)
	store, err := OpenNativeTaskCommandStore(ctx, &c)
	require(t, err)
	snapshot, err := store.ReadRunnerSnapshot(ctx, c.Identity())
	require(t, err)
	cp := *snapshot.Checkpoint
	process, err := httpclient.CurrentProcessIdentity()
	require(t, err)
	process.Created = "previous-os-generation"
	cp.Service.ProcessIdentity.Created = process.Created
	interrupted := strings.HasPrefix(fault, "interrupted") || fault == "missing-shutdown"
	if strings.HasPrefix(fault, "interrupted") {
		cp.Service.Status, cp.Service.Ready = "ready", true
	}
	switch fault {
	case "interrupted-wrong-host":
		cp.Service.ProcessIdentity.Created = "foreign-host"
	case "interrupted-wrong-repository":
		cp.Repository.Branch = "foreign"
	case "interrupted-wrong-runner":
		cp.Service.RunnerBuild.ResourceDigest = "foreign"
	}
	_, err = store.SaveCheckpoint(ctx, cp)
	require(t, err)
	require(t, store.Close())
	homeID, err := localservice.HomeID(HomeDir())
	require(t, err)
	id := contract.Identity{Protocol: contract.Protocol, HomeID: homeID, InstanceID: localservice.Token(), Process: process, Address: "127.0.0.1:18742", Ready: true}
	backend, _ := components(HomeDir())
	require(t, localservice.Write(backend.dir, "instance.json", id))
	original := contract.Shutdown{Version: 2, Runner: d.runner.result(), Identity: id, Projects: d.ShutdownResults(), Clean: true}
	if !interrupted {
		if fault == "stale-shutdown" {
			original.Identity.InstanceID = localservice.Token()
		}
		require(t, localservice.Write(backend.dir, "shutdown.json", original))
		if fault == "corrupt-shutdown" {
			require(t, os.WriteFile(filepath.Join(backend.dir, "shutdown.json"), []byte(`{`), 0600))
		}
	}
	if fault == "interrupted-locked" {
		lock, err := host.LockRepository(c.RunDir)
		require(t, err)
		defer lock.Close()
	}
	global.Profiles[0].Name = "Saved profile for the next host"
	if fault != "original" {
		if fault == "changed-runner" {
			global.Runner.Version = "another-build"
		}
		require(t, os.WriteFile(GlobalExecutionConfigPath(), mustJSON(global), 0600))
	}
	saved, err := os.ReadFile(GlobalExecutionConfigPath())
	require(t, err)
	result, err := prepareApplicationDiscard(ctx, exe, strings.Repeat("c", 32), func() error {
		if fault == "changed-during-preparation" {
			require(t, os.WriteFile(GlobalExecutionConfigPath(), append(saved, '\n'), 0600))
		}
		return nil
	})
	blocked := fault == "changed-during-preparation" || fault == "changed-runner" || strings.HasPrefix(fault, "interrupted-") || fault == "corrupt-shutdown" || fault == "stale-shutdown"
	if blocked {
		if err == nil {
			t.Fatal("discard accepted changed ownership or concurrent configuration", fault)
		}
		if _, err := os.Stat(filepath.Join(c.RunDir, "steerline.db")); err != nil {
			t.Fatal("blocked discard moved original state", err)
		}
		if _, err := os.Stat(filepath.Join(backend.dir, "instance.json")); err != nil {
			t.Fatal("blocked discard moved original host", err)
		}
		return
	}
	require(t, err)
	if interrupted {
		if result.Discard.InterruptedHost == nil || *result.Discard.InterruptedHost != id || result.Stopped == nil || len(result.Stopped.Projects) != 1 || result.Stopped.Projects[0].InstanceID != cp.Service.InstanceID {
			t.Fatal("missing interrupted host or checkpoint evidence", result)
		}
		if _, err := os.Stat(filepath.Join(result.Discard.Archive, "original-shutdown.json")); !os.IsNotExist(err) {
			t.Fatal("interrupted discard fabricated a shutdown receipt", err)
		}
	}
	after, err := os.ReadFile(GlobalExecutionConfigPath())
	require(t, err)
	if string(after) != string(saved) {
		t.Fatal("discard overwrote saved execution profiles")
	}
	if result.Outcome != "prepared" || result.Discard == nil || result.Discard.Phase != "complete" || result.Facts.NativeResources != "verified" || len(result.Discard.Projects) != 1 {
		t.Fatal(result)
	}
	if _, err := os.Stat(filepath.Join(result.Discard.Archive, "project-state", filepath.Base(c.RunDir), "steerline.db")); err != nil {
		t.Fatal(err)
	}
	fresh, err := OpenNativeTaskReader(ctx, &c)
	require(t, err)
	defer fresh.Close()
	graph, err := fresh.TaskGraphSnapshot(ctx, c.Identity())
	require(t, err)
	if !graph.Authority.DispatchPaused || len(graph.Tasks) != 0 {
		t.Fatal(graph)
	}
	// The normal upgrade verifier now sees a genuinely fresh namespace.
	_, err = VerifyRunnerUpgrade(ctx, HomeDir(), exe, taskPayloadDigest(body))
	require(t, err)
}

func testDiscardStopProtocol(t *testing.T) {
	for _, fault := range []string{"valid", "persist", "online", "input", "unknown", "pending-resume", "start", "stop-failed", "receipt", "unsettled", "inventory-changed"} {
		t.Run(fault, func(t *testing.T) {
			persisted, started, serviceStopped := false, false, false
			journal := &DeploymentDiscard{Stops: []runnercontract.PendingStop{}}
			proof := runnerclient.StoppedVerification{StateDir: "original", ResourcesSettled: true, Runs: []runnerclient.StoppedRun{{RunID: "run", InputDigest: "fixed", ResourcesSettled: true}}}
			ops := discardRunOperations{
				inventory: func(context.Context) (runnerclient.StoppedVerification, error) { return proof, nil },
				inspectCold: func(context.Context, string) (runnerclient.Inspection, bool, error) {
					run := &runnerclient.Run{RunID: "run", InputDigest: "fixed", Revision: 7, Controls: runnerclient.Controls{CanStop: true}}
					if fault == "input" {
						run.InputDigest = "changed"
					}
					if fault == "unknown" {
						run.Controls.CanStop = false
					}
					if fault == "pending-resume" {
						run.StopStatus, run.ResumeRequired, run.Controls.CanStop = "stopped", true, false
					}
					return runnerclient.Inspection{Run: run}, fault != "online", nil
				},
				start: func(_ context.Context, stops []runnercontract.PendingStop) (runnerclient.ServiceInfo, error) {
					if !persisted || len(stops) != 1 || stops[0].RunID != "run" || len(stops[0].Request.RequestID) != 32 || string(stops[0].Request.Input) != `{"expected_revision":7}` {
						t.Fatal("stop not durable before startup", stops)
					}
					started = true
					if fault == "start" {
						return runnerclient.ServiceInfo{}, errors.New("startup outcome unknown")
					}
					return runnerclient.ServiceInfo{InstanceID: "original"}, nil
				},
				inspectStop: func(_ context.Context, id string) (runnerclient.Inspection, error) {
					action := "completed"
					if fault == "stop-failed" {
						action = "failed"
					}
					if fault == "receipt" {
						id = "foreign"
					}
					return runnerclient.Inspection{Run: &runnerclient.Run{RunID: "run", StopStatus: "stopped"}, Command: &runnerclient.Command{Command: "stop", RequestID: id, ActionStatus: action}}, nil
				},
				stopService: func(_ context.Context, instance, id string) error {
					if !persisted || instance != "original" || id != "sl-service-stop-original" || journal.ServiceStop == nil {
						t.Fatal("service shutdown not durable")
					}
					serviceStopped = true
					return nil
				},
				verify: func(context.Context) (runnerclient.StoppedVerification, error) {
					if !serviceStopped {
						t.Fatal("verified resources before original service exit")
					}
					if fault == "unsettled" {
						return proof, errors.New("native stop unknown")
					}
					value := proof
					value.Runs = append([]runnerclient.StoppedRun(nil), proof.Runs...)
					if fault == "inventory-changed" {
						value.Runs[0].InputDigest = "changed"
					}
					return value, nil
				},
			}
			err := discardRuns(context.Background(), ops, journal, func() error {
				if fault == "persist" {
					return errors.New("disk full")
				}
				persisted = true
				return nil
			})
			if (err == nil) != (fault == "valid") {
				t.Fatal("wrong discard outcome", err)
			}
			if fault == "valid" && (journal.Phase != "settled" || journal.Resources == nil) {
				t.Fatal(journal)
			}
			if (fault == "persist" || fault == "online" || fault == "input" || fault == "unknown" || fault == "pending-resume") && started {
				t.Fatal("unverified stop started a worker")
			}
		})
	}
}
