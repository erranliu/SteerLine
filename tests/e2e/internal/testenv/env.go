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

	"github.com/google/uuid"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testprocess"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
)

type Run struct{ Root, Binary, Helper, Output, Scratch, AuthHome, Level string }

type Scenario struct {
	Run              Run
	ID, Root, Output string
	Context          context.Context
	cancel           context.CancelFunc
	t                *testing.T
	projects         []*Project
	processes        []*testprocess.Process
	files            []*os.File
	resources        []map[string]any
	usageVerified    bool
}

type Project struct {
	Scenario                                                              *Scenario
	Name, Root, Home, Repo, Remote, RemoteURL, Config, Database, Provider string
	Driver                                                                driver.Client
	Service                                                               *testprocess.Process
	GitServer                                                             *testprocess.Process
	serviceGeneration                                                     int
	Tasks                                                                 map[string]Task
	Chain, OrcaExecutable, OrcaRepoID, OrcaSetupID                        string
	RealPR                                                                bool
	BaseBranch, DefaultBranch                                             string
	RealPRProof                                                           *executionmodel.PRInfo
}

type Task struct {
	ID       string
	Contract taskmodel.TaskContract
}

type Snapshot struct {
	Page    contracts.NativeTaskPage `json:"page"`
	Domain  contracts.DomainSnapshot `json:"domain"`
	Helper  controlled.State         `json:"helper"`
	Runtime *contracts.Runtime       `json:"runtime,omitempty"`
}

func NewScenario(t *testing.T, id string) *Scenario {
	t.Helper()
	var run Run
	if err := json.Unmarshal([]byte(os.Getenv("STEERLINE_E2E_RUN")), &run); err != nil {
		t.Fatalf("use go run ./cmd/test-feedback -level=L2/L3: %v", err)
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
	if strings.Contains(id, "local-codex") || strings.Contains(id, "orca-codex") {
		timeout = 10 * time.Minute
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

func (s *Scenario) register(kind, path string, pid int) {
	s.resources = append(s.resources, map[string]any{"kind": kind, "path": path, "pid": pid, "created_at": time.Now().UTC()})
	s.Must(controlled.Write(filepath.Join(s.Output, "resources.json"), s.resources))
}

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

func (s *Scenario) NewProject(name string) *Project {
	s.t.Helper()
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		s.t.Fatal("invalid project name")
	}
	for _, p := range s.projects {
		if p.Name == name {
			s.t.Fatal("duplicate project")
		}
	}
	root := filepath.Join(s.Root, name)
	p := &Project{Scenario: s, Name: name, Root: root, Home: filepath.Join(root, "home"), Repo: filepath.Join(root, "repo"), Remote: filepath.Join(root, "remotes", "e2e", "project.git"), Config: filepath.Join(root, "project.json"), Database: filepath.Join(root, "home", "state", "steerline.db"), Provider: filepath.Join(root, "provider.json"), Tasks: map[string]Task{}}
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
	cmd.Env = driver.Environment(os.Environ(), p.Home, p.Provider)
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
	s.Must(os.MkdirAll(filepath.Join(p.Repo, "docs", "spec"), 0700))
	for _, name := range []string{"task.spec.md", "task.plan.md"} {
		raw, err := os.ReadFile(filepath.Join(s.Run.Root, "tests", "e2e", "testdata", "tasks", name))
		s.Must(err)
		// Git may check out templates as CRLF; fixture contracts use stable LF bytes.
		raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
		s.Must(os.WriteFile(filepath.Join(p.Repo, "docs", "spec", name), raw, 0600))
	}
	p.git(p.Repo, "add", ".")
	p.git(p.Repo, "commit", "--quiet", "-m", "E2E project and contracts")
	p.git(p.Repo, "push", "--quiet", "origin", "main")
	p.git(p.Repo, "remote", "set-url", "origin", p.RemoteURL)
	s.Must(controlled.Write(p.Provider, controlled.State{GitURL: "http://" + address + "/e2e/project.git"}))
	query := map[string]any{"executable": s.Run.Helper, "args": []string{"provider", "--e2e-state", p.Provider}}
	policy := map[string]any{"filesystem": map[string]any{"read_scope": "host", "write_scope": "workspace", "git_metadata_write": true}, "network": "allow", "escalation": "deny"}
	if s.Run.Level == "L3" {
		policy = fullAccessPolicy()
	}
	s.Must(controlled.Write(p.Config, map[string]any{
		"repository": p.Repo, "remote": "origin", "branch": "main", "task_authority": "native", "workflow_protocol": "worker-flow-v1", "worktree_root": filepath.Join(p.Home, "worktrees"), "claimant": "e2e", "git": map[string]any{"executable": s.Run.Helper, "args": []string{"git"}},
		"publication_policy": map[string]any{"publisher": "worker"}, "execution_policy": policy,
		"pr": map[string]any{"kind": "command", "base_url": "https://github.com", "repository": "e2e/project", "query": query, "close": query}, "task_poll": "100ms", "pr_poll": "100ms", "heartbeat": "100ms", "command_timeout": "30s", "shutdown_grace": "5s",
	}))
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), map[string]any{"version": 2, "execution_profiles": []any{map[string]any{"id": "controlled", "name": "Controlled E2E", "host": "local", "agent": "codex", "implementation_configs": []any{map[string]any{"implementation_key": "codex.worker-profile", "codec_version": 1, "payload": map[string]any{"executable": s.Run.Helper}}}}}, "default_execution_profile": "controlled"}))
	commandLogs := filepath.Join(s.Output, name+"-commands")
	s.Must(os.MkdirAll(commandLogs, 0700))
	p.Driver = driver.Client{Binary: s.Run.Binary, Config: p.Config, Home: p.Home, StatePath: p.Provider, LogDir: commandLogs}
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

func (p *Project) git(dir string, args ...string) string {
	p.Scenario.t.Helper()
	ctx, cancel := context.WithTimeout(p.Scenario.Context, 30*time.Second)
	defer cancel()
	out, err := controlled.Git(ctx, dir, args...)
	p.Scenario.Must(err)
	return out
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
	s := p.Scenario
	s.t.Helper()
	head := p.git(p.Repo, "rev-parse", "HEAD")
	ref := func(name string) taskmodel.FileReference {
		raw, err := os.ReadFile(filepath.Join(p.Repo, "docs", "spec", name))
		s.Must(err)
		sum := sha256.Sum256(raw)
		return taskmodel.FileReference{Path: "docs/spec/" + name, Commit: head, SHA256: hex.EncodeToString(sum[:])}
	}
	spec, plan := ref("task.spec.md"), ref("task.plan.md")
	// task validate is the product's project initialization and validation path.
	batch := taskmodel.TaskBatchInput{SchemaVersion: taskmodel.BatchSchemaVersion}
	for i, key := range []string{"a", "b"} {
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
	raw, err = p.Driver.Run(s.Context, "task", "create", "-config", p.Config, "-file", input, "-request-id", strings.ReplaceAll(uuid.NewString(), "-", ""), "-expected-authority-version", strconv.FormatUint(validation.AuthorityVersion, 10), "-by", "e2e")
	s.Must(err)
	var receipt taskmodel.TaskBatchReceipt
	s.Must(json.Unmarshal(raw, &receipt))
	if receipt.Status != "accepted" {
		s.t.Fatalf("unexpected create receipt: %+v", receipt)
	}
	for key, t := range p.Tasks {
		id, ok := receipt.TaskIDs[key]
		if !ok {
			s.t.Fatal("task receipt missing identity")
		}
		t.ID = id
		p.Tasks[key] = t
	}
}

func (p *Project) StartService() {
	s := p.Scenario
	s.t.Helper()
	p.serviceGeneration++
	// Context cancellation is handled by the owning Scenario's cleanup, not exec's root-only Kill.
	cmd := exec.Command(s.Run.Binary, "run", "-config", p.Config, "-dispatch", "manual", "-console", "off")
	cmd.Env = driver.Environment(os.Environ(), p.Home, p.Provider)
	p.Service = s.start(cmd, fmt.Sprintf("%s-service-%d", p.Name, p.serviceGeneration))
	s.Wait("service readiness", func() (bool, error) {
		select {
		case <-p.Service.Done:
			s.t.Fatal("service exited; see log")
		default:
		}
		page, err := p.Driver.Tasks(s.Context)
		if err != nil {
			return false, err
		}
		reader, err := openView(s.Context, p.Database, page.ProjectID)
		if err != nil {
			return false, err
		}
		return reader.Runtime != nil && reader.Runtime.Service.Ready, nil
	})
}

func (p *Project) StopService() {
	p.Scenario.t.Helper()
	if p.Service == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := p.Driver.Run(ctx, "stop-service", "-config", p.Config, "-by", "e2e")
	p.Scenario.Must(err)
	p.Scenario.Must(p.Service.Wait(ctx))
	p.Service = nil
}

func (p *Project) RestartService() { p.StopService(); p.StartService() }

func (p *Project) Claim(key string) {
	p.Scenario.t.Helper()
	p.Scenario.Must(p.Driver.Control(p.Scenario.Context, p.Scenario.Run.Helper, "resume_dispatch", ""))
	p.Scenario.Must(p.Driver.Control(p.Scenario.Context, p.Scenario.Run.Helper, "start_task", p.Tasks[key].ID))
}

func (p *Project) Snapshot() (Snapshot, error) {
	var snap Snapshot
	page, err := p.Driver.Tasks(p.Scenario.Context)
	if err != nil {
		return snap, err
	}
	view, err := openView(p.Scenario.Context, p.Database, page.ProjectID)
	if err != nil {
		return snap, err
	}
	if view.TaskError != nil || view.DomainError != nil {
		return snap, errors.Join(view.TaskError, view.DomainError)
	}
	if view.Tasks == nil || view.Domain == nil {
		return snap, errors.New("public project snapshot lacks tasks or domain")
	}
	// The product reader samples tasks and execution facts in one read transaction.
	snap.Page, snap.Domain = *view.Tasks, *view.Domain
	snap.Runtime = view.Runtime
	snap.Helper, err = controlled.Read(p.Provider)
	if err == nil && p.RealPR {
		snap.Helper.PR, err = p.githubPR(snap)
	}
	return snap, err
}

func (p *Project) WaitPhase(phase string) Snapshot {
	var result Snapshot
	lastPhase := ""
	rechecked := false
	p.Scenario.Wait("workflow "+phase, func() (bool, error) {
		snap, err := p.Snapshot()
		result = snap
		if err != nil {
			return false, err
		}
		if p.Chain != "" && len(snap.Domain.WorkflowRuns) == 1 {
			current := snap.Domain.WorkflowRuns[0].Phase
			if current != lastPhase {
				p.Record("latest-phase", snap)
				lastPhase = current
			}
			if current == "waiting_decision" && len(snap.Domain.Executions) == 1 {
				e := snap.Domain.Executions[0]
				if e.Environment == nil || e.Environment.Blocker == nil {
					message := "pending decision"
					pending := false
					if e.Error != nil {
						message = *e.Error
					}
					if snap.Runtime != nil {
						for _, run := range snap.Runtime.Runs {
							if run.RunID == e.WorkflowRunID && run.Decision != nil {
								message = run.Decision.Request.Context
								pending = true
							}
						}
					}
					if pending {
						p.Scenario.t.Fatalf("native worker stopped before delivery: %s; see latest-phase snapshot", message)
					}
				}
				if e.Environment != nil && e.Environment.Blocker != nil {
					reason := e.Environment.Blocker.Fact.Reason
					if (reason == "agent_input_unverified" || reason == "resource_preparation_unverified" || reason == "preparation_dispatch_unverified" || reason == "business_dispatch_unverified") && !rechecked {
						rechecked = true
						_, err := p.Driver.Run(p.Scenario.Context, "answer", "-config", p.Config, "-run-id", e.WorkflowRunID, "-option", "recheck_environment", "-by", "e2e")
						return false, err
					}
					if reason != "agent_input_unverified" {
						p.Scenario.t.Fatalf("native environment blocked: %s; see latest-phase snapshot", reason)
					}
				}
			}
		}
		return len(snap.Domain.WorkflowRuns) == 1 && snap.Domain.WorkflowRuns[0].Phase == phase && len(snap.Domain.Executions) == 1 && snap.Domain.Executions[0].Status != "active", nil
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
	defer s.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var failures []string
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
		if p.Chain != "" {
			if err := p.captureNativeDiagnostics(); err != nil {
				failures = append(failures, "native diagnostics: "+err.Error())
			}
			snap, err := p.Snapshot()
			if err == nil {
				u, usageErr := p.NativeUsage(snap, "cleanup")
				_ = controlled.Write(filepath.Join(s.Output, p.Name+"-cleanup-usage-result.json"), map[string]any{"usage": u, "error": fmt.Sprint(usageErr)})
			}
		}
	}
	for i := len(s.processes) - 1; i >= 0; i-- {
		if err := s.processes[i].Stop(ctx); err != nil {
			failures = append(failures, err.Error())
		}
	}
	for _, f := range s.files {
		if err := f.Close(); err != nil {
			failures = append(failures, err.Error())
		}
	}
	for _, p := range s.projects {
		if p.OrcaExecutable != "" {
			if err := p.cleanupOrca(); err != nil {
				failures = append(failures, err.Error())
			}
		}
	}
	for _, p := range s.projects {
		if p.RealPR {
			if err := p.cleanupGitHub(); err != nil {
				failures = append(failures, err.Error())
			}
		}
	}
	if err := RemoveOwned(s.Run.Scratch, s.Root); err != nil {
		failures = append(failures, err.Error())
	}
	if len(failures) > 0 {
		s.t.Errorf("E2E cleanup: %s", strings.Join(failures, "; "))
	}
	worker := "controlled"
	provider := "controlled"
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
	evidence := map[string]any{"id": s.ID, "passed": !s.t.Failed() && s.Context.Err() == nil, "context_error": fmt.Sprint(s.Context.Err()), "cleanup_complete": len(failures) == 0, "cleanup_errors": failures, "resources": s.resources, "components": map[string]string{"worker": worker, "provider": provider}, "usage_verified": s.usageVerified, "real_pr": realPR, "finished_at": time.Now().UTC()}
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
