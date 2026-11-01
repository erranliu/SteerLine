package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/prompts"
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
	key, _ := envmodel.Key(c.RunnerExecution.Profiles[0].Execution)
	files := map[string]string{"SteerlineFlow/environment-project.json": `{"version":1,"requirements":["SteerlineFlow/testing.md"],"testing_catalogue":"SteerlineFlow/testing/test-list.json"}`, envmodel.DocumentPath(key): environmentFixtureDocument(key, "ps1", "environment-validation", []string{"go.mod"}), envmodel.ScriptPath(key, "ps1"): "param($Phase)\n# fixture only\n", "go.mod": "module example.invalid/fixture\n", "SteerlineFlow/testing.md": "# Rules\nL2 + PR\n", "SteerlineFlow/testing/test-list.json": `{"tests":[{"id":"one","name":"One","level":"L2","environment":"controlled","command":"go test ./...","check":"passes"}]}`}
	executionRules, e := prompts.EnvironmentExecutionRules()
	require(t, e)
	generationRules, e := prompts.EnvironmentGenerationRules()
	require(t, e)
	files[envmodel.ExecutionRulesPath] = string(executionRules)
	files[envmodel.GenerationRulesPath] = string(generationRules)
	for _, profile := range c.RunnerExecution.Profiles {
		k, _ := envmodel.Key(profile.Execution)
		files[envmodel.DocumentPath(k)] = environmentFixtureDocument(k, "ps1", "environment-validation", []string{"go.mod"})
		files[envmodel.ScriptPath(k, "ps1")] = "param($Phase)\n# fixture only\n"

	}
	guide, e := os.ReadFile("../../SteerlineProjectGuideDoc/environment-adaptation.md")
	require(t, e)
	files["SteerlineProjectGuideDoc/environment-adaptation.md"] = string(guide)
	for name, body := range files {
		path := filepath.Join(c.Repository, name)
		require(t, os.MkdirAll(filepath.Dir(path), 0700))
		require(t, os.WriteFile(path, []byte(body), 0600))
	}
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "environment materials")
	fixtureGit(t, c.Repository, "push", "origin", "main")
	caps := promotionCatalog(t).Capabilities
	readsFile := false
	caps.ResultFile = &runnerclient.ResultCapability{ProgressCapability: runnerclient.ProgressCapability{Contract: runnercontract.ResultContract, Command: "result-file --run-id R --json", SubmitProperty: "result_file", EnvironmentVariable: "STEERLINE_RESULT_FILE", MaxBytes: 65536, Encoding: "base64", Digest: "sha256"}, Collection: "end_only", QueryReadsFile: &readsFile}
	caps.ResultFileSchema, e = os.ReadFile("../runnerclient/testdata/result-file.schema.json")
	require(t, e)
	c.runtimeCatalog, e = runnerclient.CompileCatalog(caps)
	require(t, e)

	reader, e := OpenNativeTaskProject(ctx, &c)
	require(t, e)
	require(t, reader.Close())
	store, e := OpenNativeTaskCommandStore(ctx, &c)
	require(t, e)
	t.Cleanup(func() { store.Close() })
	c.runtimeEnvironmentAdmission = &environmentAdmission{}
	c.runtimeEnvironmentAdmission.register(c)
	port := newEnvironmentPort(c, store, runnerclient.Client{StateDir: filepath.Join(t.TempDir(), "runner")}, nil)
	c.runtimeEnvironment = port
	return port, c, store
}
func TestProjectEnvironmentGenerationPinnedMaterialsAndSeparateUseCase(t *testing.T) {
	p, c, _ := environmentFixture(t)
	key, _ := envmodel.Key(c.RunnerExecution.Profiles[0].Execution)
	input, e := p.generationInput(context.Background(), envmodel.Generate{TargetKey: key, Profile: "fixture"})
	require(t, e)
	if input.Workflow.ID != "generate" || len(input.Generate.Materials) != 0 || input.Generate.Repository == nil || !strings.Contains(input.Generate.Instruction, envmodel.GenerationRulesPath) || !strings.Contains(input.Generate.Instruction, key) || strings.Contains(input.Generate.Instruction, "environment-context.json") {
		t.Fatal("generation did not reference pinned actual files")
	}
	source := appRepositoryManifest(t, *input.Generate.Repository)
	if err := runnerclient.VerifyRepositoryFiles(source, *input.Generate.Repository, nil); err != nil {
		t.Fatal(err)
	}

}

func TestCallerPromptConvergenceEnvironmentMaterialPreparation(t *testing.T) {
	p, c, store := environmentFixture(t)
	key, _ := envmodel.Key(c.RunnerExecution.Profiles[0].Execution)
	ctx := context.Background()
	report := &envmodel.Report{Message: "original diagnosis", Checks: []envmodel.Check{{ID: "passed", Status: "passed", Message: strings.Repeat("complete successful detail;", 300)}, {ID: "failed", Status: "failed", Message: "完整失败原因"}}, L2: []envmodel.Item{{ID: "one", Status: "incomplete", Message: "not executed"}}, PR: envmodel.PR{OutcomeUnknown: true}}
	r := envmodel.Record{ID: envmodel.ID(), Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: "fixture", Execution: c.RunnerExecution.Profiles[0].Execution, State: "failed", Diagnostic: "original failure", Requests: []envmodel.Request{{ID: envmodel.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []envmodel.Round{{Number: 1, Report: report, Settled: true}}}
	require(t, store.SaveEnvironment(ctx, r, 0))
	in, err := p.generationInput(ctx, envmodel.Generate{TargetKey: key, Profile: "fixture"})
	require(t, err)
	if in.Generate.Repository == nil || len(in.Generate.Materials) != 0 || strings.Contains(in.Generate.Instruction, report.Checks[0].Message) || strings.Contains(in.Generate.Instruction, "environment-context.json") {
		t.Fatal("generation depended on a committed context/history")
	}
	commit := func() {
		fixtureGit(t, c.Repository, "add", ".")
		fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "changed protocol")
		fixtureGit(t, c.Repository, "push", "origin", "main")
	}
	require(t, os.WriteFile(filepath.Join(c.Repository, envmodel.ExecutionRulesPath), []byte("changed report contract"), 0600))
	commit()
	if _, err = p.generationInput(ctx, envmodel.Generate{TargetKey: key, Profile: "fixture"}); err == nil || !strings.Contains(err.Error(), "不同源") {
		t.Fatal("changed fixed rules accepted", err)
	}
}
func TestProjectEnvironmentAdmissionFixedBaseAndFiniteFreshness(t *testing.T) {
	p, c, store := environmentFixture(t)
	ctx := context.Background()
	if _, e := p.prepare(ctx, "remote"); e == nil || !strings.Contains(e.Error(), "预先准备") {
		t.Fatal("missing base auto-created", e)
	}
	fixtureGit(t, c.Repository, "push", "origin", "HEAD:refs/heads/environment-validation")
	prepared, e := p.prepare(ctx, "remote")
	require(t, e)
	r := envmodel.Record{ID: envmodel.ID(), Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: "remote", Execution: prepared.Input.Execution, Prepared: prepared, State: "ready", CreatedAt: time.Now().UTC(), Requests: []envmodel.Request{{ID: envmodel.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []envmodel.Round{{Number: 1, Settled: true}}}
	require(t, store.StartEnvironment(ctx, r, 0))
	if _, _, e = p.admission(ctx, "remote", "", false); e != nil {
		t.Fatal(e)
	}
	// Source moves beyond the old validation base, but registered environment
	// inputs are unchanged: this must not invalidate task admission.
	require(t, os.WriteFile(filepath.Join(c.Repository, "business.txt"), []byte("change"), 0600))
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "ordinary change")
	fixtureGit(t, c.Repository, "push", "origin", "main")
	if _, _, e = p.admission(ctx, "remote", "", false); e != nil {
		t.Fatal("ordinary commit invalidated ready", e)
	}
	if _, e = p.prepare(ctx, "remote"); e == nil {
		t.Fatal("new round allowed base without pinned source")
	}
	require(t, os.WriteFile(filepath.Join(c.Repository, "go.mod"), []byte("module example.invalid/new\n"), 0600))
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "dependency change")
	fixtureGit(t, c.Repository, "push", "origin", "main")
	if _, _, e = p.admission(ctx, "remote", "", false); e == nil {
		t.Fatal("registered dependency change ignored")
	}
	changedCommit := fixtureGit(t, c.Repository, "rev-parse", "HEAD")
	require(t, os.WriteFile(filepath.Join(c.Repository, "go.mod"), []byte("module example.invalid/fixture\n"), 0600))
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "restore registered environment inputs")
	fixtureGit(t, c.Repository, "push", "origin", "main")
	if _, _, e = p.admission(ctx, "remote", "", false); e != nil {
		t.Fatal("restored unchanged environment basis should remain ready", e)
	}
	if _, _, e = p.admission(ctx, "remote", changedCommit, false); e == nil || !strings.Contains(e.Error(), "候选固定提交") {
		t.Fatal("candidate with other frozen environment inputs admitted", e)
	}
}
func TestProjectEnvironmentAdmissionReservationAndConcurrentStart(t *testing.T) {
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
			err := c.runtimeEnvironmentAdmission.adapt(ctx, envmodel.Target(prepared.Input.Execution), id, func() error {
				return store.SaveEnvironment(ctx, envmodel.Record{ID: id, Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: "remote", Execution: prepared.Input.Execution, State: "running", CreatedAt: time.Now().UTC(), Requests: []envmodel.Request{{ID: id, Original: json.RawMessage(`{}`)}}, Rounds: []envmodel.Round{{Number: 1, Submission: "pending"}}}, 0)
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
	e = rebuilt.check(ctx, envmodel.Target(prepared.Input.Execution), "", false)
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
	require(t, store.SaveEnvironment(ctx, envmodel.Record{ID: id, Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: "remote", Execution: profile.Execution, State: "running", Requests: []envmodel.Request{{ID: id, Original: json.RawMessage(`{}`)}}, Rounds: []envmodel.Round{{Number: 1, Submission: "unknown"}}}, 0))
	accepted := false
	a := p.config.runtimeEnvironmentAdmission
	if e = a.adapt(ctx, envmodel.Target(profile.Execution), other.Identity()+"\n"+id, func() error { accepted = true; return nil }); e == nil || accepted {
		t.Fatal("other project's original unknown activity did not block shared target")
	}
	profile.Execution.Target.Environment = "different-public-target"
	require(t, a.adapt(ctx, envmodel.Target(profile.Execution), other.Identity()+"\n"+id, func() error { accepted = true; return nil }))
	if !accepted {
		t.Fatal("unrelated public target was blocked")
	}
}

func environmentFixtureDocument(key, extension, base string, deps []string) string {
	interpreter := "powershell.exe"
	if extension == "sh" {
		interpreter = "bash"
	}
	body, _ := json.Marshal(envmodel.Manifest{Version: 3, RequiredChecks: []string{}, Gaps: []string{}, EnvironmentKey: key, Script: envmodel.ScriptPath(key, extension), Interpreter: interpreter, ValidationBase: base, Dependencies: deps})
	return "# Environment\n```steerline-environment\n" + string(body) + "\n```\n"
}
