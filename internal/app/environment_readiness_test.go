package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	nativefixture "github.com/erranliu/SteerLine/internal/testkit/protocol"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type readinessTasks struct{ project string }

func TestEnvironmentConfigurationQueryDoesNotTouchProject(t *testing.T) {
	p, c, store := environmentFixture(t)
	require(t, store.Close())
	p.store = nil
	p.config.Repository = filepath.Join(t.TempDir(), "absent-project")
	p.config.Git.Executable = "missing-git"
	c.runtimeEnvironment = p
	port := &runtimeProjectPort{config: c}
	value, err := port.environmentRead(context.Background(), "/environment/options", url.Values{"configuration": {"true"}})
	require(t, err)
	options := value.(environmentOptions)
	wantPrompt, err := prompts.EnvironmentRepair(env.DocumentPath())
	require(t, err)
	if options.RepairPrompt != wantPrompt || wantPrompt == "" {
		t.Fatal("configuration lost the canonical repair prompt", options.RepairPrompt)
	}
	if len(options.Profiles) != len(c.RunnerExecution.Profiles) || len(options.Environments) == 0 {
		t.Fatal(options)
	}
	for _, item := range options.Environments {
		if item.Status != "not_ready" || item.Stage != "unverified" || item.ActivityID != "" || item.CurrentRevision != 0 {
			t.Fatal("configuration claimed project qualification", item)
		}
	}
	for _, query := range []url.Values{{"configuration": {"false"}}, {"configuration": {"true", "true"}}, {"configuration": {"true"}, "extra": {"true"}}} {
		if _, err := port.environmentRead(context.Background(), "/environment/options", query); err == nil {
			t.Fatal("accepted invalid query", query)
		}
	}
}

func TestEnvironmentUnqualifiedQueryDoesNotReadDependencyBasis(t *testing.T) {
	p, c, _ := environmentFixture(t)
	profile, err := c.RunnerExecution.Profile("remote", "")
	require(t, err)
	key, err := env.Key(profile.Execution, nativefixture.ExecutionIdentity(profile.Execution))
	require(t, err)
	path := filepath.Join(c.Repository, filepath.FromSlash(env.DocumentPath()))
	document := environmentFixtureDocument("ps1", []string{"missing-dependency.md"})
	require(t, os.WriteFile(path, []byte(document), 0600))
	commitEnvironmentMaterials(t, c)
	value, err := p.readinessOptions(context.Background())
	require(t, err)
	for _, item := range value.(environmentOptions).Environments {
		if item.Key == key && (item.Stage != "awaiting_user" || item.Status == "ready") {
			t.Fatal(item)
		}
	}
	if _, err := p.prepare(context.Background(), "remote"); err == nil || !strings.Contains(err.Error(), "missing-dependency.md") {
		t.Fatal("start did not validate the full dependency basis", err)
	}
}

func TestProjectEnvironmentConfigurationSurvivesProjectFailures(t *testing.T) {
	for _, failure := range []string{"materials", "source", "storage"} {
		t.Run(failure, func(t *testing.T) {
			p, c, store := environmentFixture(t)
			switch failure {
			case "materials":
				fixtureGit(t, c.Repository, "rm", "SteerlineFlow/environment.md")
				commitEnvironmentMaterials(t, c)
			case "source":
				p.config.Branch = "missing-source"
			case "storage":
				require(t, store.Close())
			}
			value, err := p.readinessOptions(context.Background())
			require(t, err)
			raw, _ := json.Marshal(value)
			var options struct {
				Environments []environmentView
				Profiles     []environmentProfileView
			}
			require(t, json.Unmarshal(raw, &options))
			if len(options.Profiles) != len(c.RunnerExecution.Profiles) || len(options.Environments) == 0 {
				t.Fatal("project failure hid configured targets", string(raw))
			}
			for _, item := range options.Environments {
				if item.Adapter == "" || item.Target.Kind == "" || item.Status == "ready" || item.Reason == "" {
					t.Fatal("missing target or false qualification", item)
				}
				if failure != "materials" && item.Stage != "unverified" {
					t.Fatal("unavailable state was presented as a detection result", item)
				}
				if failure == "materials" && !strings.Contains(item.Reason, "项目尚未接入检测") {
					t.Fatal(item.Reason)
				}
			}
		})
	}
}

func TestEnvironmentOptionsWaitForStartBaselineCheck(t *testing.T) {
	checkFailure := errors.New("environment basis check failed")
	for _, tc := range []struct {
		name, stage, reason string
		checkErr            error
	}{
		{name: "unchanged", stage: "ready"},
		{name: "changed", stage: "stale", reason: "检测依赖内容变化：go.mod"},
		{name: "failed", stage: "unverified", reason: checkFailure.Error(), checkErr: checkFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, c, store := environmentFixture(t)
			existing := seedDetectedEnvironment(t, p, "remote")
			existingKey := mustEnvironmentKey(t, existing.Execution)
			selected := c.RunnerExecution.Profiles[0]
			selectedKey := mustEnvironmentKey(t, selected.Execution)
			maintenance := newProjectMaintenance(c, store)
			p.config.runtimeMaintenance = maintenance
			maintenance.environmentObservation = p.observeBasis
			_, err := maintenance.check(t.Context())
			require(t, err)
			port := &runtimeProjectPort{config: c}
			assertStage := func(value any, stage, reason string) {
				t.Helper()
				for _, item := range value.(environmentOptions).Environments {
					if item.Key == existingKey {
						if item.Stage != stage || (item.Status == "ready") != (stage == "ready") || item.ActivityID != existing.ID || !strings.Contains(item.Reason, reason) {
							t.Fatalf("existing environment: got %+v, want stage %q and reason %q", item, stage, reason)
						}
						return
					}
				}
				t.Fatal("existing environment missing from options")
			}
			before, err := port.environmentRead(t.Context(), "/environment/options", nil)
			require(t, err)
			assertStage(before, "ready", "")
			if tc.stage == "stale" {
				require(t, os.WriteFile(filepath.Join(c.Repository, "go.mod"), []byte("module changed.invalid\n"), 0600))
				commitEnvironmentMaterials(t, c)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			entered, release := make(chan struct{}), make(chan struct{})
			maintenance.environmentObservation = func(ctx context.Context, commit string) error {
				close(entered)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-release:
				}
				if tc.checkErr != nil {
					return tc.checkErr
				}
				return p.observeBasis(ctx, commit)
			}
			started := make(chan error, 1)
			go func() {
				_, err := p.service.Start(ctx, env.ID(), json.RawMessage(`{}`), env.Start{
					EnvironmentKey: selectedKey, Profile: selected.ID, TimeoutSeconds: 60,
				})
				started <- err
				close(started)
			}()
			defer func() { cancel(); <-started }()
			select {
			case <-entered:
			case err := <-started:
				t.Fatalf("start finished before checking baseline: %v", err)
			case <-ctx.Done():
				t.Fatal("start did not reach the baseline check")
			}

			type optionsResult struct {
				value any
				err   error
			}
			options := make(chan optionsResult, 1)
			go func() {
				value, err := port.environmentRead(ctx, "/environment/options", nil)
				options <- optionsResult{value, err}
				close(options)
			}()
			defer func() { cancel(); <-options }()

			// An options request that expires during the check must yield no snapshot.
			readCtx, cancelRead := context.WithTimeout(ctx, 250*time.Millisecond)
			during, err := port.environmentRead(readCtx, "/environment/options", nil)
			cancelRead()
			if during != nil || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("options returned before the baseline check finished: value=%+v err=%v", during, err)
			}
			select {
			case result := <-options:
				t.Fatalf("options completed while the baseline check was paused: %+v", result)
			default:
			}
			close(release)
			if err := <-started; !errors.Is(err, tc.checkErr) {
				t.Fatalf("unexpected detection start result: %v", err)
			}
			result := <-options
			require(t, result.err)
			assertStage(result.value, tc.stage, tc.reason)
			current, err := store.CurrentEnvironment(ctx, c.Identity(), existingKey)
			require(t, err)
			if current.Invalidated != (tc.stage == "stale") {
				t.Fatalf("unexpected persisted qualification: %+v", current)
			}
		})
	}
}

func (r readinessTasks) TaskSnapshot(context.Context, string) (taskmodel.Snapshot, error) {
	task := taskmodel.TaskRecord{ProjectIdentity: r.project, TaskID: "1", Version: 1, Status: "open", Type: "task", Title: "admission probe"}
	return taskmodel.Snapshot{Authority: taskmodel.GraphAuthority{ProjectIdentity: r.project, AuthorityVersion: 1, GraphRevision: 1}, Tasks: []taskmodel.TaskRecord{task}, ByID: map[string]taskmodel.TaskRecord{"1": task}}, nil
}
func (r readinessTasks) TaskContract(context.Context, string, taskmodel.TaskRecord) (taskmodel.TaskContract, error) {
	return taskmodel.TaskContract{}, nil
}
func TestProjectEnvironmentReadinessRefreshRejectsBeforeMaterialPreparation(t *testing.T) {
	p, c, store := environmentFixture(t)
	ctx := context.Background()
	fixtureGit(t, c.Repository, "push", "origin", "HEAD:refs/heads/environment-validation")
	prepared, err := p.prepare(ctx, "remote")
	require(t, err)
	record := env.Record{FormatVersion: 1, ID: env.ID(), Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: "remote", Execution: prepared.Input.Execution, Prepared: prepared, State: "ready", CreatedAt: time.Now().UTC(), Requests: []env.Request{{ID: env.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []env.Round{{Number: 1, Settled: true}}}
	require(t, store.StartEnvironment(ctx, record, 0))
	maintenance := newProjectMaintenance(c, store)
	c.runtimeMaintenance = maintenance
	p.config.runtimeMaintenance = maintenance
	_, err = maintenance.check(ctx)
	require(t, err)
	service := runnerTaskService(c, store, runnerclient.Client{StateDir: filepath.Join(t.TempDir(), "runner")}, maintenance.prepare, maintenance)
	service.Tasks.Reader = readinessTasks{c.Identity()}
	// This owner test isolates preparation ordering; execution-wait capability
	// validation is covered by the existing published-catalog tests.
	selected := service.Profiles["remote"]
	selected.UnavailableReason = ""
	service.Profiles["remote"] = selected
	preparedCalls := 0
	service.PrepareExecution = func(context.Context, taskmodel.FixedWork, bool) (taskmodel.FixedWork, string, error) {
		preparedCalls++
		return taskmodel.FixedWork{}, "", errors.New("must not prepare")
	}
	require(t, service.CheckExecution(ctx, "remote", false))
	require(t, os.WriteFile(filepath.Join(c.Repository, "go.mod"), []byte("module changed.invalid/fixture\n"), 0600))
	fixtureGit(t, c.Repository, "add", "go.mod")
	fixtureGit(t, c.Repository, "commit", "-qm", "Change preparation input")
	fixtureGit(t, c.Repository, "push", "origin", "main")
	_, err = service.Start(ctx, taskrunnerStartForReadiness())
	var blocked *taskmodel.Error
	if !errors.As(err, &blocked) || blocked.Code != "environment_not_ready" || preparedCalls != 0 {
		t.Fatalf("late admission: err=%v prepare=%d", err, preparedCalls)
	}
	if maintenance.current().Commit == prepared.Commit {
		t.Fatal("actual Start did not refresh original maintenance observation")
	}
	// The cheap next tick now sees the updated observation; no Start or candidate.
	require(t, service.Dispatch(ctx))
	candidates, err := store.RunnerCandidates(ctx, c.Identity(), "")
	require(t, err)
	if len(candidates) != 0 || preparedCalls != 0 {
		t.Fatal("unprepared tick created work")
	}
}

func taskrunnerStartForReadiness() taskrunner.Start {
	return taskrunner.Start{RequestID: env.ID(), TaskID: "1", TaskVersion: 1, GraphRevision: 1, ProfileID: "remote", Explicit: true}
}

type invalidationReadFailure struct{ taskStorage }

func (invalidationReadFailure) LookupCommand(context.Context, string, string) (ports.CommandRecord, bool, error) {
	return ports.CommandRecord{}, false, &ports.StorageError{Code: ports.StorageIO, Cause: errors.New("unavailable")}
}
func TestProjectEnvironmentReadinessInvalidationUnknownPreservesAppliedReceipt(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	require(t, err)
	defer store.Close()
	id := env.ID()
	out := httpcontract.CommandResponse{RequestID: id, Status: "applied"}
	result, _ := json.Marshal(out)
	command := ports.CommandRecord{ProjectIdentity: "project", CommandID: id, Kind: "project_environment", TargetID: "project", IdempotencyKey: id, RequestedBy: "user", Payload: json.RawMessage(`{"action":"environment_invalidate"}`), Status: "applied", Result: result, CreatedAt: time.Now().UTC()}
	_, _, err = store.EnqueueCommand(ctx, command)
	require(t, err)
	runtime := runtimeProjectPort{store: store}
	unknown := &ports.StorageError{Code: ports.StorageCommitFailed, Cause: errors.New("lost commit result")}
	response := httpcontract.CommandResponse{Status: "rejected"}
	if !runtime.reconcileInvalidationFailure(ctx, command, &response, unknown) || response.Status != "applied" {
		t.Fatal(response)
	}
	runtime.store = invalidationReadFailure{store}
	response.Status = "rejected"
	if !runtime.reconcileInvalidationFailure(ctx, command, &response, unknown) || response.Status != "outcome_unknown" {
		t.Fatal(response)
	}
	saved, _, err := store.LookupCommand(ctx, "project", id)
	require(t, err)
	if saved.Status != "applied" {
		t.Fatal("unknown read overwrote applied receipt")
	}
}

func TestProjectEnvironmentReadinessLocalAndModelGrouping(t *testing.T) {
	p, c, store := environmentFixture(t)
	ctx := context.Background()
	fixtureGit(t, c.Repository, "push", "origin", "HEAD:refs/heads/environment-validation")
	local := c.RunnerExecution.Profiles[0]
	local.Workflow = runnercontract.Workflow{ID: "development", Version: 2}
	local.ID = "local"
	local.Name = "Local"
	local.Execution.Settings = json.RawMessage(`{"executable":"native-fixture","model":"one"}`)
	other := local
	other.ID = "local-high"
	other.Name = "Local high"
	other.Execution.Settings = json.RawMessage(`{"executable":"native-fixture","model":"two","reasoning_effort":"high"}`)
	p.config.RunnerExecution.Profiles = []RunnerProfile{local, other}
	for _, profile := range p.config.RunnerExecution.Profiles {
		_, e := p.config.describeProfile(ctx, profile)
		require(t, e)
	}
	prepared, err := p.prepare(ctx, "local")
	require(t, err)
	record := env.Record{FormatVersion: 1, ID: env.ID(), Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: "local", Execution: prepared.Input.Execution, Prepared: prepared, State: "ready", CreatedAt: time.Now().UTC(), Requests: []env.Request{{ID: env.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []env.Round{{Number: 1, Settled: true}}}
	require(t, store.StartEnvironment(ctx, record, 0))
	_, _, err = p.admission(ctx, "local-high", "", false)
	require(t, err)
	value, err := p.readinessOptions(ctx)
	require(t, err)
	raw, _ := json.Marshal(value)
	var options struct {
		Environments []environmentView
		Profiles     []environmentProfileView
	}
	require(t, json.Unmarshal(raw, &options))
	if len(options.Environments) != 1 || len(options.Profiles) != 2 || options.Profiles[1].Status != "ready" {
		t.Fatal(string(raw))
	}
	if options.Profiles[0].Model != "one" || options.Profiles[1].Model != "two" || options.Profiles[1].ReasoningEffort != "high" || options.Environments[0].Profiles[1].Model != "two" {
		t.Fatalf("operation model facts were lost while grouping: %s", raw)
	}
	view := options.Environments[0]
	if view.Target != local.Execution.Target || view.Adapter != local.Execution.Adapter || !view.ActivitySettled || view.DocumentPath == "" {
		t.Fatalf("missing original display facts: %s", raw)
	}
	if strings.Contains(string(raw), `"settings"`) || strings.Contains(string(raw), `"executable"`) {
		t.Fatalf("execution settings exposed: %s", raw)
	}
	saved, err := store.Environment(ctx, c.Identity(), record.ID)
	require(t, err)
	if string(saved.Execution.Settings) != string(local.Execution.Settings) {
		t.Fatal("preparation projection changed original execution")
	}
}

func TestProjectEnvironmentReadinessLocalTargetOccupancyAcrossContexts(t *testing.T) {
	_, c, store := environmentFixture(t)
	ctx := context.Background()
	profile := c.RunnerExecution.Profiles[0]
	first := profile.Execution
	other := first
	other.Settings = json.RawMessage(`{"executable":"other-entry"}`)
	key1, err := env.Key(first, nativefixture.ExecutionIdentity(first))
	require(t, err)
	key2, err := env.Key(other, nativefixture.ExecutionIdentity(other))
	require(t, err)
	if key1 == key2 || executionconfig.Target(first) != executionconfig.Target(other) || executionconfig.Target(first) == "" {
		t.Fatal("qualification and occupancy keys conflated")
	}
	id := env.ID()
	require(t, store.StartEnvironment(ctx, env.Record{FormatVersion: 1, ID: id, Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: profile.ID, Execution: first, Prepared: env.Prepared{Identity: nativefixture.ExecutionIdentity(first), DerivedExecution: first, Input: runnercontract.RunInput{Execution: first}}, State: "running", Requests: []env.Request{{ID: id, Original: json.RawMessage(`{}`)}}, Rounds: []env.Round{{Number: 1}}}, 0))
	admitted := false
	if err := c.runtimeEnvironmentAdmission.adapt(ctx, executionconfig.Target(other), "other", func() error { admitted = true; return nil }); err == nil || admitted {
		t.Fatal("different local context bypassed original target occupancy", err)
	}
}
