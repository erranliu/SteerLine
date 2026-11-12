package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	nativefixture "github.com/erranliu/SteerLine/internal/testkit/protocol"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func environmentFixture(t *testing.T) (*environmentPort, Config, *sqlite.SQLiteStore) {
	t.Helper()
	c, _ := nativeTaskFixture(t)
	c.PR.Query.Args = []string{"api"}
	ctx := context.Background()
	origin := filepath.Join(t.TempDir(), "origin.git")
	fixtureGit(t, c.Repository, "clone", "--bare", c.Repository, origin)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url."+filepath.ToSlash(origin)+".insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://github.com/owner/repo.git")
	c.RunnerExecution.Profiles = append(c.RunnerExecution.Profiles, RunnerProfile{ID: "remote", Name: "Remote", Execution: runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "orca_environment", Environment: "remote-a"}, Settings: json.RawMessage(`{"agent_launch":{"executable":"codex","model":"selected"},"access_mode":"full_access"}`)}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}})
	files := map[string]string{"go.mod": "module example.invalid/fixture\n"}
	files[envmodel.DocumentPath()] = environmentFixtureDocument("ps1", []string{"go.mod"})
	files[envmodel.ScriptPath("ps1")] = "# fixture only\n"
	for name, body := range files {
		path := filepath.Join(c.Repository, name)
		require(t, os.MkdirAll(filepath.Dir(path), 0700))
		require(t, os.WriteFile(path, []byte(body), 0600))
	}
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "environment materials")
	fixtureGit(t, c.Repository, "push", "origin", "main")
	var e error
	caps := promotionCatalog(t).Capabilities
	c.runtimeCatalog, e = runnerclient.CompileCatalog(caps)
	require(t, e)

	reader, e := OpenNativeTaskProject(ctx, &c)
	require(t, e)
	require(t, reader.Close())
	store, e := OpenNativeTaskCommandStore(ctx, &c)
	require(t, e)
	t.Cleanup(func() { store.Close() })
	c.runtimeNative = &runnerclient.ConfigurationSnapshot{Query: nativefixture.ExecutionConfigurationFixture}
	for _, profile := range c.RunnerExecution.Profiles {
		_, _ = c.describeProfile(ctx, profile)
	}
	c.runtimeEnvironmentAdmission = &environmentAdmission{}
	c.runtimeEnvironmentAdmission.register(c)
	port := newEnvironmentPort(c, store, runnerclient.Client{StateDir: filepath.Join(t.TempDir(), "runner")}, nil)
	c.runtimeEnvironment = port
	return port, c, store
}
func TestProjectEnvironmentAdmissionDeclaredBasisAndPersistentRevocation(t *testing.T) {
	p, c, store := environmentFixture(t)
	ctx := context.Background()
	prepared, e := p.prepare(ctx, "remote")
	require(t, e)
	r := envmodel.Record{FormatVersion: 1, ID: envmodel.ID(), Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: "remote", Execution: prepared.Input.Execution, Prepared: prepared, State: "ready", CreatedAt: time.Now().UTC(), Requests: []envmodel.Request{{ID: envmodel.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []envmodel.Round{{Number: 1, Settled: true}}}
	require(t, store.StartEnvironment(ctx, r, 0))
	if _, _, e = p.admission(ctx, "remote", "", false); e != nil {
		t.Fatal(e)
	}
	require(t, os.WriteFile(filepath.Join(c.Repository, "business.txt"), []byte("unrelated"), 0600))
	commitEnvironmentMaterials(t, c)
	if _, _, e = p.admission(ctx, "remote", "", true); e != nil {
		t.Fatal("unrelated commit invalidated", e)
	}
	require(t, os.WriteFile(filepath.Join(c.Repository, "go.mod"), []byte("module changed.invalid\n"), 0600))
	commitEnvironmentMaterials(t, c)
	if _, _, e = p.admission(ctx, "remote", "", true); e == nil {
		t.Fatal("changed dependency admitted")
	}
	current, e := store.CurrentEnvironment(ctx, c.Identity(), mustEnvironmentKey(t, prepared.Input.Execution))
	require(t, e)
	if !current.Invalidated || current.Revision != 2 {
		t.Fatal(current)
	}
	require(t, os.WriteFile(filepath.Join(c.Repository, "go.mod"), []byte("module example.invalid/fixture\n"), 0600))
	commitEnvironmentMaterials(t, c)
	if _, _, e = p.admission(ctx, "remote", "", true); e == nil {
		t.Fatal("rollback revived revoked qualification")
	}
}
func mustEnvironmentKey(t *testing.T, e runnercontract.ExecutionInput) string {
	t.Helper()
	key, err := envmodel.Key(e, nativefixture.ExecutionIdentity(e))
	require(t, err)
	return key
}

func TestProjectEnvironmentAdmissionReservationAndConcurrentStart(t *testing.T) {
	t.Run("running task allows recheck", testEnvironmentRecheckWithRunningTask)
	p, c, store := environmentFixture(t)
	ctx := context.Background()
	fixtureGit(t, c.Repository, "push", "origin", "HEAD:refs/heads/environment-validation")
	prepared, e := p.prepare(ctx, "remote")
	require(t, e)
	service := runnerTaskService(c, store, runnerclient.Client{})
	input, _ := json.Marshal(prepared.Input)
	candidate := runnerbridge.Candidate{ProfileID: "remote", Submit: runnerclient.Request{Input: input}}
	if e = service.ReserveExecution(ctx, taskmodel.FixedWork{}, candidate, false); e == nil || !strings.Contains(e.Error(), "环境未就绪") {
		t.Fatal("unadapted remote reached reservation", e)
	}
	var denial *taskmodel.Error
	if !errors.As(e, &denial) || denial.Code != "environment_not_ready" {
		t.Fatal("environment denial must produce a terminal control receipt", e)
	}
	// Two starts contend on the same public target; exactly one durable record.
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := envmodel.ID()
			err := c.runtimeEnvironmentAdmission.adapt(ctx, executionconfig.Target(prepared.Input.Execution), id, func() error {
				return store.SaveEnvironment(ctx, envmodel.Record{FormatVersion: 1, ID: id, Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: "remote", Execution: prepared.Input.Execution, Prepared: prepared, State: "running", CreatedAt: time.Now().UTC(), Requests: []envmodel.Request{{ID: id, Original: json.RawMessage(`{}`)}}, Rounds: []envmodel.Round{{Number: 1, Submission: "pending"}}}, 0)
			})
			if err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if success != 1 {
		t.Fatal("overlapping starts", success)
	}
	rebuilt := &environmentAdmission{}
	rebuilt.register(c)
	rebuilt.mu.Lock()
	e = rebuilt.check(ctx, executionconfig.Target(prepared.Input.Execution), "")
	rebuilt.mu.Unlock()
	if e == nil {
		t.Fatal("restart lost durable occupancy")
	}
}

func TestProjectEnvironmentAdmissionAcrossProjectsUsesPublicTarget(t *testing.T) {
	p, c, store := environmentFixture(t)
	ctx := context.Background()
	profile, e := c.RunnerExecution.Profile("remote", "")
	require(t, e)
	other := c
	other.identity = ""
	other.Repository = projectRepo(t)
	fixtureGit(t, other.Repository, "remote", "add", "origin", "https://github.com/owner/repo.git")
	require(t, other.ResolveIdentity(ctx))
	other.RunDir = filepath.Join(StateRoot(), RepositoryStateKey(other.Identity()))
	reader, e := OpenNativeTaskProject(ctx, &other)
	require(t, e)
	require(t, reader.Close())
	p.config.runtimeEnvironmentAdmission.register(other)
	id := envmodel.ID()
	require(t, store.SaveEnvironment(ctx, envmodel.Record{FormatVersion: 1, ID: id, Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: "remote", Execution: profile.Execution, Prepared: envmodel.Prepared{Identity: nativefixture.ExecutionIdentity(profile.Execution), DerivedExecution: profile.Execution, Input: runnercontract.RunInput{Execution: profile.Execution}}, State: "running", Requests: []envmodel.Request{{ID: id, Original: json.RawMessage(`{}`)}}, Rounds: []envmodel.Round{{Number: 1, Submission: "unknown"}}}, 0))
	accepted := false
	a := p.config.runtimeEnvironmentAdmission
	if e = a.adapt(ctx, executionconfig.Target(profile.Execution), other.Identity()+"\n"+id, func() error { accepted = true; return nil }); e == nil || accepted {
		t.Fatal("other project's original unknown activity did not block shared target")
	}
	profile.Execution.Target.Environment = "different-public-target"
	require(t, a.adapt(ctx, executionconfig.Target(profile.Execution), other.Identity()+"\n"+id, func() error { accepted = true; return nil }))
	if !accepted {
		t.Fatal("unrelated public target was blocked")
	}
}

func testEnvironmentRecheckWithRunningTask(t *testing.T) {
	p, c, store := environmentFixture(t)
	ctx := context.Background()
	const specPath = "SteerlineFlow/spec/task.spec.md"
	const planPath = "SteerlineFlow/plans/task.plan.md"
	const spec = "# Task Spec\n\n## Acceptance\n\nTask completes.\n"
	const plan = "# Task Plan\n"
	for path, contents := range map[string]string{specPath: spec, planPath: plan} {
		file := filepath.Join(c.Repository, path)
		require(t, os.MkdirAll(filepath.Dir(file), 0700))
		require(t, os.WriteFile(file, []byte(contents), 0600))
	}
	commit := commitEnvironmentMaterials(t, c)
	specRef := taskmodel.FileReference{Path: specPath, Commit: commit, SHA256: taskPayloadDigest([]byte(spec))}
	contract := taskmodel.TaskContract{
		ContractID: "recheck-task", Repository: "https://github.com/owner/repo.git", Remote: "origin", Branch: "main",
		SpecRef: specRef, PlanRef: taskmodel.FileReference{Path: planPath, Commit: commit, SHA256: taskPayloadDigest([]byte(plan))},
		CommonRefs: []taskmodel.FileReference{}, AcceptanceRefs: []taskmodel.FileReference{specRef},
	}
	var err error
	contract.Digest, err = taskmodel.ContractDigest(contract)
	require(t, err)
	batch := taskmodel.TaskBatchInput{SchemaVersion: taskmodel.BatchSchemaVersion, ProjectID: c.Identity(),
		Tasks:        []taskmodel.TaskBatchTask{{LocalKey: "task", TaskID: 1, Type: "task", Title: "Running task", Contract: contract}},
		Dependencies: []taskmodel.TaskBatchEdge{},
	}
	_, err = NewTaskCommands(store, c).CreateTaskBatch(ctx, mustJSON(batch), envmodel.ID(), 1, "fixture")
	require(t, err)
	graph, err := store.TaskGraphSnapshot(ctx, c.Identity())
	require(t, err)
	previous := seedDetectedEnvironment(t, p, "remote")
	task := graph.ByID["1"]
	input := previous.Prepared.Input
	input.Title = task.Title
	input.Documents = runnercontract.Documents{Spec: runnercontract.Document{Path: specPath}, Plan: runnercontract.Document{Path: planPath}}
	input.Publication.BaseBranch = contract.Branch
	candidate := runnerbridge.Candidate{
		Project: c.Identity(), TaskID: task.TaskID, ID: envmodel.ID(), Revision: 1,
		StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "fixture", Explicit: true,
		ProfileID: "remote", ContractDigest: contract.Digest, BaseRevision: commit, Participation: "active", SubmitState: "pending",
		Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: envmodel.ID(), Input: mustJSON(input)},
	}
	work := taskmodel.FixedWork{Record: task, Contract: contract, Authority: graph.TaskGraph().Authority}
	require(t, runnerTaskService(c, store, runnerclient.Client{}).ReserveExecution(ctx, work, candidate, false))
	candidate, err = store.Candidate(ctx, c.Identity(), candidate.ID)
	require(t, err)
	candidate.Revision++
	candidate.RunID, candidate.SubmitState = envmodel.ID(), "accepted"
	candidate.Projection = &runnercontract.Run{RunID: candidate.RunID, Revision: 1, Status: "running"}
	require(t, store.SaveCandidate(ctx, candidate, 1, nil))

	key := mustEnvironmentKey(t, previous.Execution)
	start := envmodel.Start{EnvironmentKey: key, Profile: "remote", ExpectedCurrentRevision: 1, TimeoutSeconds: 60}
	recheck, err := p.service.Start(ctx, envmodel.ID(), mustJSON(start), start)
	require(t, err)
	start.ExpectedCurrentRevision++
	if _, err := p.service.Start(ctx, envmodel.ID(), mustJSON(start), start); err == nil || !strings.Contains(err.Error(), "仍有未核实的环境适配") {
		t.Fatalf("a second detection on the target was not rejected for occupancy: %v", err)
	}
	current, err := store.CurrentEnvironment(ctx, c.Identity(), key)
	require(t, err)
	if current.ActivityID != recheck.ID || current.Revision != start.ExpectedCurrentRevision {
		t.Fatal("the second request replaced the active detection", current)
	}
	saved, err := store.Candidate(ctx, c.Identity(), candidate.ID)
	require(t, err)
	if saved.Revision != candidate.Revision || saved.RunID != candidate.RunID || saved.StopRequested || saved.Projection == nil || saved.Projection.Status != "running" {
		t.Fatal("recheck changed the original task execution", saved)
	}
}

func environmentFixtureDocument(extension string, deps []string) string {
	interpreter := "powershell.exe"
	if extension == "sh" {
		interpreter = "bash"
	}
	body, _ := json.Marshal(envmodel.Manifest{Version: 6, Requirements: map[string]string{}, Script: envmodel.ScriptPath(extension), Interpreter: interpreter, Dependencies: deps})
	return "# Environment\n```steerline-environment\n" + string(body) + "\n```\n"
}
