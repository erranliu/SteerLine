package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type readinessTasks struct{ project string }

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
	record := env.Record{ID: env.ID(), Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: "remote", Execution: prepared.Input.Execution, Prepared: prepared, State: "ready", CreatedAt: time.Now().UTC(), Requests: []env.Request{{ID: env.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []env.Round{{Number: 1, Settled: true}}}
	require(t, store.StartEnvironment(ctx, record, 0))
	maintenance := newProjectMaintenance(c, store)
	c.runtimeMaintenance = maintenance
	p.config.runtimeMaintenance = maintenance
	_, err = maintenance.check(ctx)
	require(t, err)
	service := runnerTaskService(c, store, runnerclient.Client{StateDir: filepath.Join(t.TempDir(), "runner")})
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
	local.Execution.Settings = json.RawMessage(`{"executable":"codex","model":"one"}`)
	other := local
	other.ID = "local-high"
	other.Name = "Local high"
	other.Execution.Settings = json.RawMessage(`{"executable":"codex","model":"two","reasoning_effort":"high"}`)
	p.config.RunnerExecution.Profiles = []RunnerProfile{local, other}
	prepared, err := p.prepare(ctx, "local")
	require(t, err)
	record := env.Record{ID: env.ID(), Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: "local", Execution: prepared.Input.Execution, Prepared: prepared, State: "ready", CreatedAt: time.Now().UTC(), Requests: []env.Request{{ID: env.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []env.Round{{Number: 1, Settled: true}}}
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
	key1, err := env.Key(first)
	require(t, err)
	key2, err := env.Key(other)
	require(t, err)
	if key1 == key2 || env.Target(first) != env.Target(other) || env.Target(first) == "" {
		t.Fatal("qualification and occupancy keys conflated")
	}
	id := env.ID()
	require(t, store.StartEnvironment(ctx, env.Record{ID: id, Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: profile.ID, Execution: first, State: "running", Requests: []env.Request{{ID: id, Original: json.RawMessage(`{}`)}}, Rounds: []env.Round{{Number: 1}}}, 0))
	admitted := false
	if err := c.runtimeEnvironmentAdmission.adapt(ctx, env.Target(other), "other", func() error { admitted = true; return nil }); err == nil || admitted {
		t.Fatal("different local context bypassed original target occupancy", err)
	}
}
