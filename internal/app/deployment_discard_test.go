package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestDeploymentDiscardRuns(t *testing.T) {
	t.Run("offline-flow", testDiscardOfflineFlow)
	for _, schema := range []string{"current", "schema44", "schema45", "schema51"} {
		t.Run("task-graph-"+schema, func(t *testing.T) {
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
			switch schema {
			case "schema44":
				makeDiscardSchema44(t, c)
			case "schema45":
				makeDiscardSchema45(t, c)
			case "schema51":
				execDiscardFixtureSQL(t, c, `UPDATE schema_migrations SET version=51`)
			}
			beforeFile, err := os.ReadFile(filepath.Join(c.RunDir, "steerline.db"))
			require(t, err)
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
			if graph.DatabaseID == old.DatabaseID || graph.Authority.DispatchMode != "manual" || !reflect.DeepEqual(graph.Edges, old.Edges) {
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
			afterFile, err := os.ReadFile(filepath.Join(c.RunDir, "steerline.db"))
			require(t, err)
			if !bytes.Equal(beforeFile, afterFile) {
				t.Fatal("discard preparation modified original database")
			}
			if schema != "current" {
				if normal, err := OpenNativeTaskReader(ctx, &c); err == nil {
					normal.Close()
					t.Fatal("normal reader admitted", schema)
				}
				if writer, err := sqlite.OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db")); err == nil {
					writer.Close()
					t.Fatal("normal writer admitted", schema)
				}
			}
			original, err := openDiscardTaskReader(ctx, &c)
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
	}
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
	for _, fault := range []string{"original", "schema51", "unstarted", "unstarted-new-runner", "unstarted-new-runner-pending-resume", "unstarted-live-owner", "unstarted-unclosed", "unstarted-project", "unstarted-repository", "unstarted-runner", "unstarted-build-mismatch", "unstarted-active-checkpoint", "unstarted-current-owner", "unstarted-partial", "unstarted-locked", "schema46", "schema44", "schema44-missing-shutdown", "schema44-wrong-host", "schema44-invalid-definition", "schema44-unbound-roadmap", "unsupported-schema", "future-schema", "saved-profile", "changed-during-preparation", "changed-runner", "missing-shutdown", "interrupted", "interrupted-wrong-host", "interrupted-wrong-repository", "interrupted-wrong-runner", "interrupted-locked", "corrupt-shutdown", "stale-shutdown",
		"environment-missing", "environment-preparing", "environment-rejected", "environment-accepted", "environment-foreign", "environment-outside-inventory", "environment-unavailable"} {
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
	unstarted := strings.HasPrefix(fault, "unstarted")
	if unstarted {
		process.Created = "failed-start-generation"
	}
	interrupted := strings.HasPrefix(fault, "interrupted") || fault == "missing-shutdown" || fault == "schema44-missing-shutdown"
	if strings.HasPrefix(fault, "interrupted") {
		cp.Service.Status, cp.Service.Ready = "ready", true
	}
	switch fault {
	case "interrupted-wrong-host", "schema44-wrong-host":
		cp.Service.ProcessIdentity.Created = "foreign-host"
	case "interrupted-wrong-repository":
		cp.Repository.Branch = "foreign"
	case "interrupted-wrong-runner":
		cp.Service.RunnerBuild.ResourceDigest = "foreign"
	case "unstarted-live-owner":
		live, err := httpclient.CurrentProcessIdentity()
		require(t, err)
		owner := hostcontract.ProcessIdentity{Kind: live.Kind, PID: live.PID, Created: live.Created, Executable: live.Executable}
		cp.Service.ProcessIdentity = &owner
	case "unstarted-current-owner":
		cp.Service.ProcessIdentity.Created = process.Created
	case "unstarted-repository":
		cp.Repository.Branch = "foreign"
	case "unstarted-runner":
		cp.Service.RunnerBuild.StateDir = t.TempDir()
	case "unstarted-build-mismatch":
		cp.Service.RunnerBuild.Version = "foreign"
	case "unstarted-active-checkpoint":
		cp.Service.Status, cp.Service.Ready = "ready", true
	case "unstarted-new-runner", "unstarted-new-runner-pending-resume":
		cp.Service.RunnerBuild.SHA256 = strings.Repeat("a", 64)
		cp.Service.RunnerBuild.Version = "previous-runner"
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
	if unstarted {
		original.Clean, original.Diagnostic = false, "startup recovery: unsupported SQLite schema 51"
		original.Runner = &contract.RunnerShutdown{Attempted: true, StateDir: cp.Service.RunnerBuild.StateDir, Diagnostic: "original Runner shutdown remains unverified"}
		for i := range original.Projects {
			original.Projects[i].InstanceID = ""
		}
		switch fault {
		case "unstarted-unclosed":
			original.Projects[0].LocalClosed = false
		case "unstarted-project":
			original.Projects[0].ProjectID = "foreign"
		case "unstarted-partial":
			original.Projects[0].InstanceID = "different-project-instance"
		}
	}
	if !interrupted {
		if fault == "stale-shutdown" {
			original.Identity.InstanceID = localservice.Token()
		}
		require(t, localservice.Write(backend.dir, "shutdown.json", original))
		if fault == "corrupt-shutdown" {
			require(t, os.WriteFile(filepath.Join(backend.dir, "shutdown.json"), []byte(`{`), 0600))
		}
	}
	if fault == "interrupted-locked" || fault == "unstarted-locked" {
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
	if strings.HasPrefix(fault, "schema44") {
		makeDiscardSchema44(t, c)
	} else if fault == "schema46" {
		execDiscardFixtureSQL(t, c, `DROP TABLE refactoring_actions; UPDATE schema_migrations SET version=46`)
	} else if fault == "schema51" || unstarted {
		// Schema 52 changed maintenance payload semantics, not SQL definitions.
		execDiscardFixtureSQL(t, c, `UPDATE schema_migrations SET version=51`)
	}
	switch fault {
	case "schema44-invalid-definition":
		execDiscardFixtureSQL(t, c, `ALTER TABLE tasks ADD COLUMN unexpected TEXT`)
	case "unsupported-schema":
		execDiscardFixtureSQL(t, c, `DELETE FROM schema_migrations`)
	case "future-schema":
		execDiscardFixtureSQL(t, c, `UPDATE schema_migrations SET version=?`, sqlite.SchemaVersion+1)
	case "schema44-unbound-roadmap":
		// A schema 44 document batch bound its sole execution on the parent.
		r := roadmap.Record{Purpose: "documents", ID: strings.Repeat("a", 32), Project: c.Identity(), Revision: 1,
			Runner: *cp.Service.RunnerBuild, Request: runnercontract.Request{RequestID: "original"},
			Submission: "started", RunID: "missing-run", InputDigest: "original-input"}
		execDiscardFixtureSQL(t, c, `INSERT INTO roadmaps(project_identity,roadmap_id,revision,payload) VALUES(?,?,?,?)`,
			r.Project, r.ID, r.Revision, mustJSON(roadmap.StoreValue(r)))
	}
	if strings.HasPrefix(fault, "environment-") {
		seedDiscardUnknownEnvironment(t, c, *cp.Service.RunnerBuild, fault)
	}
	if strings.HasPrefix(fault, "unstarted-new-runner") {
		run := &runnercontract.Run{RunID: "previous-run", Revision: 1, InputDigest: "previous-input", Status: "completed", StopStatus: "stopped", Closed: true,
			ResumeRequired: fault == "unstarted-new-runner-pending-resume"}
		require(t, os.WriteFile(filepath.Join(cp.Service.RunnerBuild.StateDir, "caller-fixture.json"), mustJSON(map[string]any{"runs": map[string]any{run.RunID: run}}), 0600))
		require(t, os.WriteFile(filepath.Join(cp.Service.RunnerBuild.StateDir, "idle-run-fixture.json"), mustJSON(runnerclient.Inspection{Run: run}), 0600))
	}
	originalDB, err := os.ReadFile(filepath.Join(c.RunDir, "steerline.db"))
	require(t, err)
	result, err := prepareApplicationDiscard(ctx, exe, strings.Repeat("c", 32), func() error {
		if fault == "changed-during-preparation" {
			require(t, os.WriteFile(GlobalExecutionConfigPath(), append(saved, '\n'), 0600))
		}
		return nil
	})
	blocked := fault == "changed-during-preparation" || fault == "changed-runner" || strings.HasPrefix(fault, "interrupted-") ||
		fault == "corrupt-shutdown" || fault == "stale-shutdown" || fault == "schema44-wrong-host" ||
		fault == "schema44-invalid-definition" || fault == "schema44-unbound-roadmap" || fault == "unsupported-schema" || fault == "future-schema" ||
		fault == "environment-foreign" || fault == "environment-outside-inventory" || fault == "environment-unavailable"
	blocked = blocked || unstarted && fault != "unstarted" && fault != "unstarted-new-runner"
	if blocked {
		if err == nil {
			t.Fatal("discard accepted changed ownership or concurrent configuration", fault)
		}
		if fault == "schema44-unbound-roadmap" && !strings.Contains(err.Error(), "original generation resource settlement unverified") {
			t.Fatal("discard did not check the original document batch binding", err)
		}
		if strings.HasPrefix(fault, "environment-") && !strings.Contains(err.Error(), "activity "+strings.Repeat("a", 32)+" round 1") {
			t.Fatal("discard did not identify the unverified environment round", err)
		}
		if fault == "unstarted-new-runner-pending-resume" && !strings.Contains(err.Error(), "drained") {
			t.Fatal("changed Runner did not verify original pending controls", err)
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
	if unstarted {
		if result.Discard.UnstartedHost == nil || *result.Discard.UnstartedHost != id || result.Stopped == nil || len(result.Stopped.Projects) != 1 ||
			result.Stopped.Projects[0].CheckpointProcess == nil || result.Stopped.Projects[0].CheckpointProcess.Created != cp.Service.ProcessIdentity.Created || result.Stopped.Projects[0].InstanceID != cp.Service.InstanceID {
			t.Fatal("failed startup lost its original checkpoint owner", result)
		}
		var savedShutdown contract.Shutdown
		require(t, localservice.ReadPrivateJSON(filepath.Join(result.Discard.Archive, "original-shutdown.json"), &savedShutdown, 1<<20))
		if !reflect.DeepEqual(savedShutdown, original) {
			t.Fatal("discard rewrote failed host shutdown", savedShutdown)
		}
	}
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
	archivedDB, err := os.ReadFile(filepath.Join(result.Discard.Archive, "project-state", filepath.Base(c.RunDir), "steerline.db"))
	require(t, err)
	if !bytes.Equal(archivedDB, originalDB) {
		t.Fatal("discard changed the archived database")
	}
	fresh, err := OpenNativeTaskReader(ctx, &c)
	require(t, err)
	defer fresh.Close()
	graph, err := fresh.TaskGraphSnapshot(ctx, c.Identity())
	require(t, err)
	if graph.Authority.DispatchMode != "manual" || len(graph.Tasks) != 0 {
		t.Fatal(graph)
	}
	// The normal upgrade verifier now sees a genuinely fresh namespace.
	_, err = VerifyRunnerUpgrade(ctx, HomeDir(), exe, taskPayloadDigest(body))
	require(t, err)
}

func seedDiscardUnknownEnvironment(t *testing.T, c Config, pin runnercontract.BuildPin, scenario string) {
	t.Helper()
	const requestID = "original-environment-submit"
	record := envmodel.Record{FormatVersion: 1, ID: strings.Repeat("a", 32), Project: c.Identity(), Revision: 1,
		Kind: "adaptation", Build: pin, Rounds: []envmodel.Round{{Number: 1, Submission: "unknown", Request: runnercontract.Request{RequestID: requestID}}}}
	execDiscardFixtureSQL(t, c, `INSERT INTO project_environments(project_identity,activity_id,revision,payload) VALUES(?,?,?,?)`,
		record.Project, record.ID, record.Revision, mustJSON(record))

	command := &runnercontract.Command{Command: "submit", RequestID: requestID}
	var receipt any = runnerclient.Inspection{Command: command}
	inspections := map[string]any{}
	switch scenario {
	case "environment-missing":
		receipt = map[string]string{"fixture_error": "request_not_found"}
	case "environment-unavailable":
		receipt = map[string]string{"fixture_error": "service_unavailable"}
	case "environment-preparing":
		command.ReceiptStatus, command.ActionStatus = "pending", "preparing"
	case "environment-rejected":
		command.ReceiptStatus, command.ActionStatus = "rejected", "rejected"
		command.Error = &runnercontract.CommandError{Code: "submit_rejected", Message: "fixture rejection"}
	default:
		run := &runnercontract.Run{RunID: "original-environment-run", Revision: 1, InputDigest: "original-input",
			Status: "completed", StopStatus: "stopped", Correlation: map[string]string{
				"steerline_project": record.Project, "environment_activity": record.ID, "environment_round": "1",
			}}
		if scenario == "environment-foreign" {
			run.Correlation["environment_round"] = "2"
		}
		command.ReceiptStatus, command.ActionStatus, command.RunID = "accepted", "completed", &run.RunID
		receipt = runnerclient.Inspection{Command: command, Run: run}
		if scenario != "environment-outside-inventory" {
			inspections[run.RunID] = runnerclient.Inspection{Run: run}
			inventory := map[string]any{"runs": map[string]any{run.RunID: run}}
			require(t, os.WriteFile(filepath.Join(pin.StateDir, "caller-fixture.json"), mustJSON(inventory), 0600))
		}
	}
	inspections[requestID] = receipt
	require(t, os.WriteFile(filepath.Join(pin.StateDir, "idle-run-fixture.json"), []byte(`{}`), 0600))
	require(t, os.WriteFile(filepath.Join(pin.StateDir, "idle-inspections-fixture.json"), mustJSON(inspections), 0600))
}

// Schema 45 added three observation tables and their indexes.
func makeDiscardSchema44(t *testing.T, c Config) {
	t.Helper()
	makeDiscardSchema45(t, c)
	execDiscardFixtureSQL(t, c, `DROP TABLE reading_fields;
		DROP TABLE reading_versions;
		DROP TABLE message_observations;
		UPDATE schema_migrations SET version=44`)
}

func makeDiscardSchema45(t *testing.T, c Config) {
	t.Helper()
	execDiscardFixtureSQL(t, c, `DROP TABLE refactoring_actions;
		ALTER TABLE project_task_authorities RENAME TO current_authorities;
		CREATE TABLE project_task_authorities (project_identity TEXT PRIMARY KEY, authority TEXT NOT NULL CHECK(authority='native'), authority_version INTEGER NOT NULL CHECK(authority_version>0), graph_revision INTEGER NOT NULL CHECK(graph_revision>=0), repository TEXT NOT NULL, remote TEXT NOT NULL, branch TEXT NOT NULL, dispatch_mode TEXT NOT NULL CHECK(dispatch_mode IN ('manual','auto')), dispatch_paused INTEGER NOT NULL CHECK(dispatch_paused IN (0,1)), created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
		INSERT INTO project_task_authorities SELECT project_identity,authority,authority_version,graph_revision,repository,remote,branch,dispatch_mode,1,created_at,updated_at FROM current_authorities;
		DROP TABLE current_authorities;
		UPDATE schema_migrations SET version=45`)
}

func execDiscardFixtureSQL(t *testing.T, c Config, stmt string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(c.RunDir, "steerline.db"))
	require(t, err)
	_, err = db.Exec(stmt, args...)
	require(t, errors.Join(err, db.Close()))
}

func testDiscardStopProtocol(t *testing.T) {
	for _, fault := range []string{"valid", "persist", "online", "input", "unknown", "pending-stop", "pending-resume", "start", "stop-failed", "receipt", "wait-canceled", "unsettled", "inventory-changed"} {
		t.Run(fault, func(t *testing.T) {
			persisted, started, serviceStopped := false, false, false
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var pendingRequest string
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
					if fault == "pending-stop" {
						run.StopStatus = "pending"
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
					if fault == "wait-canceled" {
						if pendingRequest != "" {
							cancel()
							return runnerclient.Inspection{}, errors.New("Runner query response is unavailable")
						}
						pendingRequest = id
						run := &runnerclient.Run{RunID: "run", StopStatus: "pending", Operations: []runnercontract.Operation{{ID: "original-operation", Action: "prepare_workspace", Status: "outcome_unknown", Diagnostic: "original worktree is missing"}}}
						return runnerclient.Inspection{Run: run, Command: &runnerclient.Command{Command: "stop", RequestID: id, ActionStatus: "pending"}}, nil
					}
					action := "completed"
					if fault == "stop-failed" {
						action = "failed"
					}
					if fault == "receipt" {
						id = "foreign"
					}
					return runnerclient.Inspection{Run: &runnerclient.Run{RunID: "run", StopStatus: "stopped"}, Command: &runnerclient.Command{Command: "stop", RequestID: id, ActionStatus: action}}, nil
				},
				stopService: func(ctx context.Context, instance, id string) error {
					if !persisted || instance != "original" || id != "sl-service-stop-original" || journal.ServiceStop == nil {
						t.Fatal("service shutdown not durable")
					}
					require(t, ctx.Err())
					serviceStopped = true
					return nil
				},
				verify: func(context.Context) (runnerclient.StoppedVerification, error) {
					if fault == "wait-canceled" {
						t.Fatal("cancelled wait continued toward discard")
					}
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
			err := discardRuns(ctx, ops, journal, func() error {
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
			if (fault == "persist" || fault == "online" || fault == "input" || fault == "unknown" || fault == "pending-stop" || fault == "pending-resume") && (started || persisted) {
				t.Fatal("unverified stop started a worker")
			}
			if fault == "wait-canceled" && (!serviceStopped || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "Run run stop request "+pendingRequest) || !strings.Contains(err.Error(), "original prepare_workspace operation original-operation: original worktree is missing")) {
				t.Fatal("cancellation lost the original stop evidence or left its writer running", err)
			}
		})
	}
}
