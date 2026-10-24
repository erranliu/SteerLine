// Package testenv owns isolated projects, tasks, processes and evidence for E2E.
package testenv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
	testfeedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/google/uuid"
)

type Run struct{ Root, Binary, Helper, Runner, Output, Scratch, AuthHome, Level, SourceCommit, SourceDirty, ExecutionModel, AuditModel string }
type Scenario struct {
	LiveProof            *testfeedback.LiveDeliveryProof
	TestingProof         *testfeedback.TestManagementProof
	externalCleanupError string
	bodyFinishedAt       time.Time
	bodyContextErr       error
	Run                  Run
	ID, Root, Output     string
	Context              context.Context
	cancel               context.CancelFunc
	t                    *testing.T
	projects             []*Project
	processes            []*testprocess.Process
	files                []*os.File
	resources            []map[string]any
}

type Project struct {
	AuditObservations                                                                 map[string]string
	RemoteCleanupFailed                                                               bool
	RunnerCleanupFailed                                                               bool
	Scenario                                                                          *Scenario
	Name, Root, Home, Repo, Remote, RemoteURL, Config, Database, Provider, Executable string
	ProjectID, DatabaseID                                                             string
	Driver                                                                            driver.Client
	Service                                                                           *testprocess.Process
	GitServer                                                                         *testprocess.Process
	serviceGeneration                                                                 int
	Tasks                                                                             map[string]Task
	Chain                                                                             string
	RealPR                                                                            bool
	BaseBranch, DefaultBranch                                                         string
	RealPRProof                                                                       *executionmodel.PRInfo
}

type Task struct {
	ID       string
	Contract taskmodel.TaskContract
}

// TaskFactsSnapshot contains caller-owned task and delivery facts only.
type TaskFactsSnapshot struct {
	ProjectIdentity string
	Tasks           []taskmodel.TaskRecord
	Deliveries      []taskmodel.TaskDeliveryRecord
}

type Snapshot struct {
	Runner  storageports.RunnerSnapshot `json:"runner"`
	Page    storageports.NativeTaskPage `json:"page"`
	Domain  TaskFactsSnapshot           `json:"domain"`
	Helper  controlled.State            `json:"helper"`
	Runtime *reconstruction.Runtime     `json:"runtime,omitempty"`
}

func NewScenario(t *testing.T, id string) *Scenario {
	t.Helper()
	var run Run
	if err := json.Unmarshal([]byte(os.Getenv("STEERLINE_E2E_RUN")), &run); err != nil {
		t.Fatalf("use go run ./cmd/test-feedback -level=L2/L3/L4: %v", err)
	}
	return newScenario(t, id, run)
}

func newScenario(t *testing.T, id string, run Run) *Scenario {
	t.Helper()
	if id == "" || filepath.Base(id) != id || id == "." || id == ".." {
		t.Fatal("invalid scenario ID")
	}
	for _, p := range []string{run.Root, run.Binary, run.Helper, run.Output, run.Scratch} {
		if !filepath.IsAbs(p) {
			t.Fatal("E2E requires absolute run paths; use test-feedback")
		}
	}
	timeout := 5 * time.Minute
	if id == "configuration.execution-profiles" || id == "runtime.project-maintenance" || id == "testing.management-local-codex" || id == "testing.project-progress-local-codex" || id == "recovery.waiting-merge-restart" || id == "smoke.local-codex" || id == "workflow.local-codex-audited" {
		timeout = 10 * time.Minute
	}
	if id == "workflow.local-codex-audited" {
		timeout = 20 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	s := &Scenario{Run: run, ID: id, Root: filepath.Join(run.Scratch, id), Output: filepath.Join(run.Output, "scenarios", id), Context: ctx, cancel: cancel, t: t}
	t.Cleanup(s.cleanup)
	for _, dir := range []string{s.Root, s.Output} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func (s *Scenario) Must(err error) {
	s.t.Helper()
	if err != nil {
		s.t.Fatal(err)
	}
}

// FinishBody freezes the business deadline before independent service/resource cleanup.
// Cancelling this context also prevents later business I/O after this boundary.
func (s *Scenario) FinishBody() error {
	if !s.bodyFinishedAt.IsZero() {
		return errors.New("scenario business phase already finished")
	}
	s.bodyFinishedAt = time.Now().UTC()
	s.bodyContextErr = s.Context.Err()
	if deadline, ok := s.Context.Deadline(); ok && !s.bodyFinishedAt.Before(deadline) && s.bodyContextErr == nil {
		s.bodyContextErr = context.DeadlineExceeded
	}
	s.cancel()
	return s.bodyContextErr
}

func (s *Scenario) register(kind, path string, pid int) {
	s.resources = append(s.resources, map[string]any{"kind": kind, "path": path, "pid": pid, "created_at": time.Now().UTC()})
	s.Must(controlled.Write(filepath.Join(s.Output, "resources.json"), s.resources))
}

func (s *Scenario) RecordResource(kind, path string, pid int) { s.register(kind, path, pid) }

func (s *Scenario) start(cmd *exec.Cmd, name string) *testprocess.Process {
	s.t.Helper()
	f, err := os.Create(filepath.Join(s.Output, name+".log"))
	s.Must(err)
	s.files = append(s.files, f)
	cmd.Stdout, cmd.Stderr = f, f
	p, err := testprocess.Start(cmd)
	s.Must(err)
	s.processes = append(s.processes, p)
	s.register("process", cmd.Path, p.Cmd.Process.Pid)
	return p
}

// RunCommand owns a short helper/process command in the scenario job and keeps
// its output in the scenario evidence directory, including expected failures.
func (s *Scenario) RunCommand(cmd *exec.Cmd, name string) ([]byte, error) {
	return s.RunCommandFor(cmd, name, 4*time.Minute)
}

func (s *Scenario) RunCommandFor(cmd *exec.Cmd, name string, timeout time.Duration) ([]byte, error) {
	s.t.Helper()
	process := s.start(cmd, name)
	ctx, cancel := context.WithTimeout(s.Context, timeout)
	defer cancel()
	err := process.Wait(ctx)
	if ctx.Err() != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		err = errors.Join(err, process.Stop(stopCtx))
		stopCancel()
	}
	output, readErr := os.ReadFile(filepath.Join(s.Output, name+".log"))
	if readErr != nil {
		return output, errors.Join(err, readErr)
	}
	return output, err
}

func (s *Scenario) NewProject(name string) *Project {
	return s.NewProjectAt(name, "", s.Run.Binary)
}

// NewProjectAt creates an isolated target clone/home at a caller-selected
// relative path and binds every product process to the requested executable.
func (s *Scenario) NewProjectAt(name, relativeRoot, executable string) *Project {
	s.t.Helper()
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		s.t.Fatal("invalid project name")
	}
	if relativeRoot != "" {
		clean := filepath.Clean(relativeRoot)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			s.t.Fatal("project path must stay inside scenario scratch")
		}
		relativeRoot = clean
	}
	if executable == "" {
		executable = s.Run.Binary
	}
	for _, p := range s.projects {
		if p.Name == name {
			s.t.Fatal("duplicate project")
		}
	}
	root := filepath.Join(s.Root, relativeRoot, name)
	p := &Project{Scenario: s, Name: name, Root: root, Home: filepath.Join(root, "home"), Repo: filepath.Join(root, "repo"), Remote: filepath.Join(root, "remotes", "e2e", "project.git"), Config: filepath.Join(root, "home", "etc", "SteerLine.json"), Database: filepath.Join(root, "home", "state", "steerline.db"), Provider: filepath.Join(root, "provider.json"), Executable: executable, Tasks: map[string]Task{}}
	p.BaseBranch = "main"
	s.projects = append(s.projects, p)
	s.register("project", root, 0)
	for _, dir := range []string{root, filepath.Dir(p.Remote), filepath.Join(p.Home, "temp"), filepath.Join(p.Home, "codex"), filepath.Join(p.Home, "bin"), filepath.Join(p.Home, "etc")} {
		s.Must(os.MkdirAll(dir, 0700))
	}
	s.Must(os.CopyFS(p.Repo, os.DirFS(filepath.Join(s.Run.Root, "tests", "e2e", "testdata", "projects", "minimal"))))
	p.git(p.Repo, "init", "--quiet", "-b", "main")
	for _, kv := range [][2]string{{"user.name", "E2E"}, {"user.email", "e2e@example.invalid"}, {"commit.gpgsign", "false"}} {
		p.git(p.Repo, "config", kv[0], kv[1])
	}
	p.git(p.Repo, "config", "core.autocrlf", "false")
	p.git(p.Repo, "init", "--quiet", "--bare", p.Remote)
	// A real loopback HTTP Git backend preserves remote URL identity validation.
	addressPath := filepath.Join(root, "git-address.json")
	p.git(p.Repo, "--git-dir", p.Remote, "config", "http.receivepack", "true")
	cmd := exec.Command(s.Run.Helper, "git-server", "--root", filepath.Join(root, "remotes"), "--address-file", addressPath, "--e2e-state", p.Provider)
	cmd.Env = p.Driver.Environment(os.Environ())
	server := s.start(cmd, name+"-git-server")
	p.GitServer = server
	address := ""
	s.Wait("Git server readiness", func() (bool, error) {
		select {
		case <-server.Done:
			s.t.Fatal("Git server exited; see log")
		default:
		}
		raw, err := os.ReadFile(addressPath)
		if err != nil {
			return false, err
		}
		var receipt map[string]string
		if err = json.Unmarshal(raw, &receipt); err != nil {
			return false, err
		}
		address = receipt["address"]
		return address != "", nil
	})
	p.RemoteURL = "https://github.com/e2e/project.git"
	p.git(p.Repo, "remote", "add", "origin", "http://"+address+"/e2e/project.git")
	for _, name := range []string{"task.spec.md", "task.plan.md", "task.tests.json"} {
		raw, err := os.ReadFile(filepath.Join(s.Run.Root, "tests", "e2e", "testdata", "tasks", name))
		s.Must(err)
		// Git may check out templates as CRLF; fixture contracts use stable LF bytes.
		raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
		directory := "plans"
		if name == "task.spec.md" {
			directory = "spec"
		}
		path := filepath.Join(p.Repo, "SteerlineFlow", directory, name)
		s.Must(os.MkdirAll(filepath.Dir(path), 0700))
		s.Must(os.WriteFile(path, raw, 0600))
	}
	p.git(p.Repo, "add", ".")
	p.git(p.Repo, "commit", "--quiet", "-m", "E2E project and contracts")
	p.git(p.Repo, "push", "--quiet", "origin", "main")
	p.git(p.Repo, "remote", "set-url", "origin", p.RemoteURL)
	s.Must(controlled.Write(p.Provider, controlled.State{LocalRepository: p.Repo, GitURL: "http://" + address + "/e2e/project.git"}))
	query := map[string]any{"executable": s.Run.Helper, "args": []string{"provider", "--e2e-state", p.Provider}}
	s.Must(controlled.Write(p.Config, map[string]any{
		"repository": p.Repo, "remote": "origin", "branch": "main", "task_authority": "native", "workflow_protocol": "worker-flow-v2", "claimant": "e2e", "git": map[string]any{"executable": s.Run.Helper, "args": []string{"git"}},
		"publication_policy": map[string]any{"publisher": "worker"},
		"pr":                 map[string]any{"kind": "command", "base_url": "https://github.com", "repository": "e2e/project", "query": query}, "task_poll": "100ms", "pr_poll": "100ms", "heartbeat": "100ms", "command_timeout": "30s", "shutdown_grace": "5s",
	}))
	selection, err := driver.RunnerSelection(s.Run.Helper)
	s.Must(err)
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), map[string]any{"version": 3, "runner": selection, "execution_profiles": []any{map[string]any{"id": "controlled", "name": "Controlled E2E", "execution": map[string]any{"adapter": "codex-app-server", "target": map[string]string{"kind": "local"}, "settings": map[string]any{"executable": s.Run.Helper}}, "workflow": map[string]any{"id": "development", "version": 1}}}, "default_execution_profile": "controlled"}))
	commandLogs := filepath.Join(s.Output, name+"-commands")
	s.Must(os.MkdirAll(commandLogs, 0700))
	p.Driver = driver.Client{Binary: p.Executable, Config: p.Config, Home: p.Home, StatePath: p.Provider, LogDir: commandLogs}
	rawHome, err := p.Driver.Run(s.Context, "home", "-config", p.Config)
	s.Must(err)
	p.Database = ""
	for _, line := range strings.Split(string(rawHome), "\n") {
		if strings.HasPrefix(line, "run_dir ") {
			p.Database = filepath.Join(strings.TrimSpace(strings.TrimPrefix(line, "run_dir")), "steerline.db")
		}
	}
	if !filepath.IsAbs(p.Database) {
		s.t.Fatalf("home did not expose project state: %s", rawHome)
	}
	return p
}

// NewHomeForProject binds the same Git clone to another independent home.
// It is used to verify that the repository's existing cross-home lock remains
// the authority when no per-home service record exists yet.
func (s *Scenario) NewHomeForProject(source *Project, name, executable string) *Project {
	s.t.Helper()
	if source == nil || name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		s.t.Fatal("invalid same-repository home")
	}
	if executable == "" {
		executable = s.Run.Binary
	}
	root := filepath.Join(s.Root, "same-repository", name)
	p := &Project{Scenario: s, Name: name, Root: root, Home: filepath.Join(root, "home"), Repo: source.Repo,
		Remote: source.Remote, RemoteURL: source.RemoteURL, Config: filepath.Join(root, "home", "etc", "SteerLine.json"),
		Database: filepath.Join(root, "home", "state", "steerline.db"), Provider: filepath.Join(root, "provider.json"),
		Executable: executable, Tasks: map[string]Task{}}
	s.register("project-same-repository", root, 0)
	for _, dir := range []string{root, filepath.Join(p.Home, "temp"), filepath.Join(p.Home, "codex"), filepath.Join(p.Home, "bin"), filepath.Join(p.Home, "etc")} {
		s.Must(os.MkdirAll(dir, 0700))
	}
	configRaw, err := os.ReadFile(source.Config)
	s.Must(err)
	var config map[string]any
	s.Must(json.Unmarshal(configRaw, &config))
	delete(config, "run_dir")

	if pr, ok := config["pr"].(map[string]any); ok {
		if query, ok := pr["query"].(map[string]any); ok {
			if args, ok := query["args"].([]any); ok && len(args) >= 3 {
				args[2] = p.Provider
			}
		}
	}
	s.Must(controlled.Write(p.Config, config))
	executionConfig, err := os.ReadFile(filepath.Join(source.Home, "execution.json"))
	s.Must(err)
	s.Must(os.WriteFile(filepath.Join(p.Home, "execution.json"), executionConfig, 0600))
	provider, err := controlled.Read(source.Provider)
	s.Must(err)
	s.Must(controlled.Write(p.Provider, provider))
	commandLogs := filepath.Join(s.Output, name+"-commands")
	s.Must(os.MkdirAll(commandLogs, 0700))
	p.Driver = driver.Client{Binary: p.Executable, Config: p.Config, Home: p.Home, StatePath: p.Provider, LogDir: commandLogs}
	rawHome, err := p.Driver.Run(s.Context, "home", "-config", p.Config)
	s.Must(err)
	for _, line := range strings.Split(string(rawHome), "\n") {
		if strings.HasPrefix(line, "run_dir ") {
			p.Database = filepath.Join(strings.TrimSpace(strings.TrimPrefix(line, "run_dir")), "steerline.db")
		}
	}
	if !filepath.IsAbs(p.Database) {
		s.t.Fatalf("same-repository home did not expose project state: %s", rawHome)
	}
	return p
}

func (p *Project) git(dir string, args ...string) string {
	p.Scenario.t.Helper()
	ctx, cancel := context.WithTimeout(p.Scenario.Context, 30*time.Second)
	defer cancel()
	if p.Chain != "" {
		command := exec.CommandContext(ctx, "git", append([]string{"-c", "core.longpaths=true", "-c", "core.autocrlf=false", "-c", "user.name=E2E", "-c", "user.email=e2e@example.invalid", "-c", "commit.gpgsign=false", "-C", dir}, args...)...)
		command.Env = p.Driver.Environment(os.Environ())
		out, err := command.CombinedOutput()
		if err != nil {
			p.Scenario.t.Fatalf("owned native fixture Git: %v: %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	out, err := controlled.Git(ctx, dir, args...)
	p.Scenario.Must(err)
	return out
}

func (p *Project) Command(executable string, args ...string) *exec.Cmd {
	cmd := exec.Command(executable, args...)
	cmd.Env = p.Driver.Environment(os.Environ())
	cmd.Dir = p.Driver.WorkingDirectory
	return cmd
}

func (s *Scenario) Wait(label string, condition func() (bool, error)) {
	s.t.Helper()
	var last error
	timer := time.NewTicker(100 * time.Millisecond)
	defer timer.Stop()
	for {
		ready, err := condition()
		if err != nil {
			last = err
		}
		if ready && err == nil {
			return
		}
		select {
		case <-s.Context.Done():
			s.t.Fatalf("%s: %v (last error: %v)", label, s.Context.Err(), last)
		case <-timer.C:
		}
	}
}

func (p *Project) CreateTasks() {
	p.createTasks([]string{"a", "b"})
}

func (p *Project) createTasks(keys []string) {
	s := p.Scenario
	s.t.Helper()
	head := p.git(p.Repo, "rev-parse", "HEAD")
	ref := func(path string) taskmodel.FileReference {
		raw, err := os.ReadFile(filepath.Join(p.Repo, filepath.FromSlash(path)))
		s.Must(err)
		sum := sha256.Sum256(raw)
		return taskmodel.FileReference{Path: path, Commit: head, SHA256: hex.EncodeToString(sum[:])}
	}
	spec, plan := ref("SteerlineFlow/spec/task.spec.md"), ref("SteerlineFlow/plans/task.plan.md")
	p.StartService()
	// The shared host owns initialization before CLI task validation.
	batch := taskmodel.TaskBatchInput{SchemaVersion: taskmodel.BatchSchemaVersion}
	for i, key := range keys {
		contract := taskmodel.TaskContract{ContractID: "e2e-" + key, Repository: p.RemoteURL, Remote: "origin", Branch: p.BaseBranch, SpecRef: spec, PlanRef: plan, CommonRefs: []taskmodel.FileReference{}, AcceptanceRefs: []taskmodel.FileReference{spec}}
		digest, err := taskmodel.ContractDigest(contract)
		s.Must(err)
		contract.Digest = digest
		batch.Tasks = append(batch.Tasks, taskmodel.TaskBatchTask{LocalKey: key, TaskID: int64(i + 1), Title: "E2E " + key, Type: "task", Contract: contract})
		p.Tasks[key] = Task{ID: strconv.Itoa(i + 1), Contract: contract}
	}
	batch.Dependencies = []taskmodel.TaskBatchEdge{{Predecessor: taskmodel.TaskBatchReference{LocalKey: "a"}, Successor: taskmodel.TaskBatchReference{LocalKey: "b"}}}
	// Resolve project identity through the documented common Git directory contract.
	common := p.git(p.Repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	batch.ProjectID = strings.ToLower(filepath.Clean(filepath.FromSlash(common)))
	input := filepath.Join(p.Root, "tasks.json")
	s.Must(controlled.Write(input, batch))
	raw, err := p.Driver.Run(s.Context, "task", "validate", "-config", p.Config, "-file", input)
	s.Must(err)
	var validation taskmodel.TaskBatchValidation
	s.Must(json.Unmarshal(raw, &validation))
	if !validation.Valid {
		s.t.Fatalf("invalid task batch: %+v", validation)
	}
	if validation.ProjectID != batch.ProjectID || validation.DatabaseID == "" {
		s.t.Fatal("task validation did not bind the owned project/database identity")
	}
	raw, err = p.Driver.Run(s.Context, "task", "create", "-config", p.Config, "-file", input, "-request-id", strings.ReplaceAll(uuid.NewString(), "-", ""), "-expected-authority-version", strconv.FormatUint(validation.AuthorityVersion, 10), "-by", "e2e")
	s.Must(err)
	var receipt taskmodel.TaskBatchReceipt
	s.Must(json.Unmarshal(raw, &receipt))
	if receipt.Status != "accepted" {
		s.t.Fatalf("unexpected create receipt: %+v", receipt)
	}
	p.ProjectID, p.DatabaseID = validation.ProjectID, validation.DatabaseID
	p.adoptFixtureMaterials()
	for key, t := range p.Tasks {
		id, ok := receipt.TaskIDs[key]
		if !ok {
			s.t.Fatal("task receipt missing identity")
		}
		t.ID = id
		p.Tasks[key] = t
	}
}

// StartHost starts the home owner without requiring any project to succeed.
func (p *Project) StartHost() {
	s := p.Scenario
	s.t.Helper()
	if p.Service != nil {
		return
	}
	p.serviceGeneration++
	cmd := exec.Command(p.Executable, "serve")
	cmd.Env = p.Driver.Environment(os.Environ())
	p.Service = s.start(cmd, fmt.Sprintf("%s-host-%d", p.Name, p.serviceGeneration))
}
func (p *Project) StartService() {
	s := p.Scenario
	s.t.Helper()
	p.StartHost()
	s.Wait("project runtime readiness", func() (bool, error) {
		select {
		case <-p.Service.Done:
			s.t.Fatal("host exited; see log")
		default:
		}
		status, err := p.Driver.HostStatus(s.Context)
		if err != nil {
			return false, err
		}
		for _, project := range status.Projects {
			if project.ConfigID != filepath.Base(p.Config) {
				continue
			}
			if project.RuntimeState == "failed" {
				s.t.Fatal(project.Diagnostic)
			}
			return project.Online && status.Backend.Identity.Process.PID == p.Service.Cmd.Process.Pid, nil
		}
		return false, nil
	})
}

func (p *Project) StopService() {
	p.Scenario.t.Helper()
	if p.Service == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 130*time.Second)
	defer cancel()
	_, err := p.Driver.Run(ctx, "stop")
	p.Scenario.Must(err)
	p.Scenario.Must(p.Service.Wait(ctx))
	p.Service = nil
}

// Restart only the caller host. The public Runner peer remains independent;
// graceful application shutdown also reports resource settlement and is tested separately.
func (p *Project) RestartService() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p.Scenario.Must(p.Service.Stop(ctx))
	p.Service = nil
	p.StartService()
}

func (p *Project) Claim(key string) {
	p.ClaimExpectedStatus(key, "applied")
}

func (p *Project) ClaimExpectedStatus(key, status string) {
	p.Scenario.t.Helper()
	p.Scenario.Must(p.Driver.Control(p.Scenario.Context, p.Scenario.Run.Helper, "resume_dispatch", ""))
	p.Scenario.Must(p.Driver.ControlExpectedStatus(p.Scenario.Context, p.Scenario.Run.Helper, "start_task", p.Tasks[key].ID, status))
}

func (p *Project) Snapshot() (Snapshot, error) {
	return p.SnapshotContext(p.Scenario.Context)
}

func snapshotFromApplicationRead(read storageports.ApplicationProjectSnapshot) (Snapshot, error) {
	var snap Snapshot
	view := read.ProjectView
	if view.TaskError != nil {
		return snap, view.TaskError
	}
	if view.Tasks == nil || view.Ready == nil {
		return snap, errors.New("combined project snapshot lacks task page or ready values")
	}
	if read.RunnerError != nil {
		return snap, read.RunnerError
	}
	state := read.RunnerState
	project := view.Tasks.ProjectID
	graph := state.Graph
	if project == "" || graph.Authority.ProjectIdentity != project ||
		graph.DatabaseID != view.Tasks.DatabaseID || graph.DatabaseRevision != view.Tasks.DatabaseRevision ||
		graph.Authority.AuthorityVersion != view.Tasks.AuthorityVersion || graph.Authority.GraphRevision != view.Tasks.GraphRevision ||
		view.Ready.ProjectID != project || view.Ready.DatabaseID != view.Tasks.DatabaseID || view.Ready.DatabaseRevision != view.Tasks.DatabaseRevision ||
		view.Ready.AuthorityVersion != view.Tasks.AuthorityVersion || view.Ready.GraphRevision != view.Tasks.GraphRevision ||
		!view.Tasks.SampledAt.Equal(view.SampledAt) || !view.Ready.SampledAt.Equal(view.SampledAt) {
		return snap, errors.New("combined project snapshot contains different database revisions")
	}
	snap.Page = *view.Tasks
	snap.Runner = state
	// Task facts come from the same caller-owned application read transaction.
	snap.Domain = TaskFactsSnapshot{ProjectIdentity: project, Tasks: graph.Tasks}
	for _, delivery := range graph.Deliveries {
		snap.Domain.Deliveries = append(snap.Domain.Deliveries, delivery)
	}
	if state.Checkpoint != nil {
		runtime := taskrunner.Project(state, *state.Checkpoint)
		snap.Runtime = &runtime
	}
	return snap, nil
}

func (p *Project) SnapshotContext(ctx context.Context) (Snapshot, error) {
	var snap Snapshot
	if p.ProjectID == "" || p.DatabaseID == "" {
		return snap, errors.New("project snapshot lacks its validated project/database identity")
	}
	read, err := driver.ReadApplicationProjectSnapshot(ctx, p.Database, p.ProjectID, "", 100)
	if err != nil {
		return snap, err
	}
	snap, err = snapshotFromApplicationRead(read)
	if err != nil {
		return snap, err
	}
	if snap.Page.ProjectID != p.ProjectID || snap.Page.DatabaseID != p.DatabaseID {
		return snap, errors.New("project snapshot differs from its validated project/database identity")
	}
	snap.Helper, err = controlled.Read(p.Provider)
	if err == nil && p.RealPR {
		snap.Helper.PR, err = p.githubPRContext(ctx, snap)
	}
	return snap, err
}

func (p *Project) WaitPhase(phase string) Snapshot {
	var result Snapshot
	p.Scenario.Wait("Runner phase "+phase, func() (bool, error) {
		snap, err := p.Snapshot()
		result = snap
		if err != nil {
			return false, err
		}
		if len(snap.Runner.Candidates) != 1 {
			return false, nil
		}
		c := snap.Runner.Candidates[0]
		if c.SubmitState == "rejected" {
			p.Scenario.t.Fatal("original Runner submission rejected: ", c.Diagnostic)
		}
		if c.Projection == nil {
			return false, nil
		}
		if p.Scenario.ID == "workflow.local-codex-audited" {
			p.observeAuditedProduct(c, false)
			if c.Projection.Status == "blocked" || c.Projection.Status == "failed" {
				p.Record("blocked", snap)
				p.Scenario.t.Fatal("audited Run blocked: ", c.Projection.Diagnostic)
			}
		}
		if phase == "completed" {
			return c.Participation == "completed" && len(snap.Runner.Graph.Deliveries) == 1, nil
		}
		wanted := phase
		if phase == "waiting_pr_merge" || phase == "waiting_external" {
			wanted = "pr_ready"
		}
		if phase == "waiting_decision" {
			return c.Projection.Question != nil, nil
		}
		if c.Projection.ResumeRequired && wanted == "pr_ready" && c.Projection.Status != wanted {
			p.Scenario.t.Fatal("Runner needs explicit recovery: ", c.Projection.Diagnostic)
		}
		return c.Projection.Status == wanted, nil
	})
	return result
}

func (p *Project) Merge() {
	if p.RealPR {
		p.Scenario.Must(p.mergeGitHub())
		return
	}
	p.Scenario.Must(controlled.Merge(p.Scenario.Context, p.Provider, p.Repo))
}

func (p *Project) Record(label string, snap Snapshot) {
	p.Scenario.Must(controlled.Write(filepath.Join(p.Scenario.Output, p.Name+"-"+label+".json"), snap))

}

func (s *Scenario) cleanup() {
	if s.bodyFinishedAt.IsZero() {
		_ = s.FinishBody()
	}
	bodyContextErr := s.bodyContextErr
	defer s.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	var failures []string
	if s.externalCleanupError != "" {
		failures = append(failures, s.externalCleanupError)
	}
	// Capture public state before shutting down, even after an assertion timeout.
	for _, p := range s.projects {
		if p.Driver.Binary == "" {
			continue
		}
		page, err := p.Driver.Tasks(ctx)
		if err == nil {
			domain, readErr := driver.ReadWorkflow(ctx, p.Database, page.ProjectID)
			if readErr == nil {
				err = controlled.Write(filepath.Join(s.Output, p.Name+"-final.json"), map[string]any{"page": page, "domain": domain})
			}
		}
		if err != nil {
			_ = controlled.Write(filepath.Join(s.Output, p.Name+"-diagnostic.json"), map[string]string{"error": err.Error()})
		}

	}
	for i := len(s.processes) - 1; i >= 0; i-- {
		if err := s.processes[i].Stop(ctx); err != nil {
			failures = append(failures, err.Error())
		}
	}
	cleanedHomes := map[string]bool{}
	for _, p := range s.projects {
		if p.Driver.Binary == "" || cleanedHomes[p.Home] {
			continue
		}
		cleanedHomes[p.Home] = true
		if _, err := os.Stat(p.Database); os.IsNotExist(err) {
			if _, err := os.Stat(runnerstatepath.Directory(p.Home)); os.IsNotExist(err) {
				continue
			}
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		project := p.ProjectID
		if project == "" {
			project = strings.ToLower(filepath.Clean(filepath.Join(p.Repo, ".git")))
		}
		_, readErr := driver.ReadWorkflow(cleanupCtx, p.Database, project)
		if readErr == nil {
			readErr = p.Driver.CleanupOwnedRunner(cleanupCtx, s.Run.Helper)
		}
		cleanupCancel()
		if readErr != nil {
			p.RunnerCleanupFailed = true
			failures = append(failures, "original Runner cleanup: "+readErr.Error())
		}
		if readErr == nil {
			if err := runnerstatepath.RemoveStopped(p.Home); err != nil {
				p.RunnerCleanupFailed = true
				failures = append(failures, "Runner directory cleanup: "+err.Error())
			}
		}
		_ = controlled.Write(filepath.Join(s.Output, p.Name+"-runner-cleanup.json"), map[string]any{"confirmed": readErr == nil, "error": fmt.Sprint(readErr)})
	}
	for _, f := range s.files {
		if err := f.Close(); err != nil {
			failures = append(failures, err.Error())
		}
	}
	for _, p := range s.projects {
		if p.RealPR && !p.RemoteCleanupFailed {
			if err := p.cleanupGitHub(); err != nil {
				failures = append(failures, err.Error())
			}
		}
	}
	preserveScratch := false
	for _, p := range s.projects {
		preserveScratch = preserveScratch || p.RemoteCleanupFailed || p.RunnerCleanupFailed
	}
	if !preserveScratch {
		if err := RemoveOwned(s.Run.Scratch, s.Root); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		s.t.Errorf("E2E cleanup: %s", strings.Join(failures, "; "))
	}
	worker := "controlled"
	provider := "controlled"
	if s.ID == "testing.management-local-codex" {
		worker = "local-codex"
		provider = "github/" + githubRepository + " (owned base only)"
	}
	var realPR *executionmodel.PRInfo
	for _, p := range s.projects {
		if p.Chain != "" {
			worker = p.Chain
		}
		if p.RealPR {
			provider = "github/" + githubRepository
			realPR = p.RealPRProof
		}
	}
	evidence := map[string]any{"id": s.ID, "passed": !s.t.Failed() && bodyContextErr == nil && len(failures) == 0, "context_error": fmt.Sprint(bodyContextErr), "body_finished_at": s.bodyFinishedAt, "cleanup_complete": len(failures) == 0, "cleanup_errors": failures, "resources": s.resources, "components": map[string]string{"worker": worker, "provider": provider}, "real_pr": realPR, "live_delivery": s.LiveProof, "test_management": s.TestingProof, "finished_at": time.Now().UTC()}
	if err := controlled.Write(filepath.Join(s.Output, "scenario.json"), evidence); err != nil {
		s.t.Errorf("write scenario evidence: %v", err)
	}
}

func RemoveOwned(root, path string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("refuse deletion outside run scratch")
	}
	return os.RemoveAll(path)
}
