package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Native subprocess substitutes: no real task writes, model calls, or PR mutations.
type fakeDB struct {
	Tasks           []Task
	Dependencies    map[string]string
	Claims          map[string]int
	Starts          map[string]int
	FailClose       int
	Behavior        string
	PRBase          string
	PRHeadRevision  string
	CompletionCalls int
	FailCompletion  int
	CompletionMode  string
}

func TestMain(m *testing.M) {
	if os.Getenv("STEERLINE_LIFECYCLE_EXIT_EARLY") == "1" {
		os.Exit(23)
	}
	for i, a := range os.Args {
		if a == "--fake" {
			os.Exit(fakeMain(os.Args[i+1:]))
		}
	}
	os.Exit(m.Run())
}
func readFake(path string) fakeDB {
	b, e := os.ReadFile(path)
	if e != nil {
		panic(e)
	}
	var db fakeDB
	if e = json.Unmarshal(b, &db); e != nil {
		panic(e)
	}
	return db
}

func fakeMinimalStep(path string, db fakeDB, argv []string) int {
	if len(argv) != 1 {
		return 20
	}
	var input MinimalStepInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 21
	}
	worktree, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 22
	}
	git := func(args ...string) (string, error) {
		cmd := exec.Command(argv[0], args...)
		cmd.Dir = worktree
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %v: %w: %s", args, err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	if input.StepID == "implement" {
		if err = os.WriteFile(filepath.Join(worktree, "minimal-step-test.txt"), []byte(input.TaskID+" "+input.AttemptID+"\n"), 0600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 23
		}
		if _, err = git("add", "--all"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 24
		}
		cmd := exec.Command(argv[0], "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "minimal test implementation")
		cmd.Dir = worktree
		if out, e := cmd.CombinedOutput(); e != nil {
			fmt.Fprintf(os.Stderr, "git commit: %v: %s", e, out)
			return 25
		}
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 26
	}
	branch, err := git("branch", "--show-current")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 27
	}
	dirty, err := git("status", "--porcelain")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 28
	}
	base := input.Workspace.Base
	workspace := WorkspaceEvidence{Repository: input.ProjectIdentity, Worktree: canonical(worktree), Branch: branch, Base: base, Head: head, Dirty: dirty != ""}
	if db.Behavior == "minimal-decision" && input.StepID == "implement" && input.Decision == nil {
		result := StepGateResult{StepID: input.StepID, ResultID: newID(), Workspace: workspace, PrerequisiteIDs: append([]string(nil), input.PrerequisiteResultIDs...), Decision: &DecisionRequest{Question: "Which implementation should be used?", Options: []DecisionOption{{ID: "keep", Label: "Keep the current design"}}}}
		envelope := MinimalStepResultEnvelope{SchemaVersion: 1, ProjectIdentity: input.ProjectIdentity, TaskID: input.TaskID, WorkflowRunID: input.WorkflowRunID, ExecutionID: input.ExecutionID, AttemptID: input.AttemptID, Result: result}
		if err = json.NewEncoder(os.Stdout).Encode(envelope); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 29
		}
		return 0
	}
	revision := input.CandidateRevision
	if input.StepID == "implement" {
		revision = head
	}
	result := StepGateResult{StepID: input.StepID, Succeeded: true, InputRevision: input.InputRevision, Revision: revision, Workspace: workspace, ResultID: newID(), PrerequisiteIDs: append([]string(nil), input.PrerequisiteResultIDs...)}
	if input.Decision != nil {
		result.AppliedDecisionID = input.Decision.Answer.DecisionID
		result.AppliedDecisionAttemptID = input.Decision.AttemptID
		result.AppliedDecisionExecutionID = input.Decision.ExecutionID
		result.AppliedAnswerSHA256 = input.Decision.AnswerSHA256
	}
	switch input.StepID {
	case "verify":
		for _, name := range input.ProjectSteps.Verify.RequiredChecks {
			result.Checks = append(result.Checks, CheckEvidence{Name: name, Status: "passed", Revision: revision})
		}
		if db.Behavior == "minimal-no-checks" {
			result.Checks = nil
		}
	case "review":
		result.ReviewVerdict = "passed"
	case "publish_pr":
		result.PRURL = db.PRBase + "/owner/repo/pulls/9"
		result.PublishedHead = revision
	}
	if input.StepID == "implement" {
		db.PRHeadRevision = head
		writeFake(path, db)
	}
	envelope := MinimalStepResultEnvelope{SchemaVersion: 1, ProjectIdentity: input.ProjectIdentity, TaskID: input.TaskID, WorkflowRunID: input.WorkflowRunID, ExecutionID: input.ExecutionID, AttemptID: input.AttemptID, Result: result}
	if err = json.NewEncoder(os.Stdout).Encode(envelope); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 29
	}
	return 0
}

func writeFake(path string, db fakeDB) {
	b, _ := json.Marshal(db)
	if e := AtomicWrite(path, b); e != nil {
		panic(e)
	}
}
func arg(args []string, key string) string {
	for i, a := range args {
		if a == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
func fakeMain(args []string) int {
	mode := args[0]
	if mode == "git-proxy" {
		gitArgs := append([]string(nil), args[2:]...)
		if len(gitArgs) > 0 && gitArgs[0] == "fetch" {
			for i := range gitArgs {
				if gitArgs[i] == "web" {
					gitArgs[i] = "origin"
					break
				}
			}
		}
		cmd := exec.Command(args[1], gitArgs...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode()
			}
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	if mode == "sleep" {
		time.Sleep(time.Hour)
		return 0
	}
	if mode == "orca" {
		return fakeOrca(args[1], args[2:])
	}
	if mode == "lock" {
		l, e := LockRepository(args[1])
		if e != nil {
			return 3
		}
		l.Close()
		return 0
	}
	if mode == "reader" {
		end := time.Now().Add(1200 * time.Millisecond)
		count := 0
		for time.Now().Before(end) {
			b, e := os.ReadFile(args[1])
			if e == nil {
				var v Runtime
				if json.Unmarshal(b, &v) != nil {
					return 4
				}
				count++
			} // Windows may briefly reject open during atomic replacement; retry.
		}
		if count == 0 {
			return 6
		}
		fmt.Println(count)
		return 0
	}
	if mode == "publish-control" {
		b, e := os.ReadFile(args[2])
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 7
		}
		var req ControlRequest
		if e = json.Unmarshal(b, &req); e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 7
		}
		if e = publishControlRequest(args[1], req); e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 8
		}
		return 0
	}
	if mode == "codex" && len(args) >= 3 && args[2] == "--version" {
		version := os.Getenv("STEERLINE_FAKE_CODEX_VERSION")
		if version == "" {
			version = "fake-codex/1"
		}
		fmt.Println(version)
		return 0
	}
	path := args[1]
	argv := args[2:]
	if mode == "codex" && len(argv) > 0 && argv[0] == "app-server" {
		fmt.Fprintln(os.Stderr, "fake Codex does not support public history")
		return 1
	}
	if arg(argv, "--help") != "" || strings.Contains(strings.Join(argv, " "), "--help") {
		fmt.Println("--unassigned --limit --claim --actor --assignee --all --reason --json --output-schema --output-last-message --sandbox")
		return 0
	}
	db := readFake(path)
	if mode == "minimal-step" {
		return fakeMinimalStep(path, db, argv)
	}
	if mode == "pr-query" {
		info := PRInfo{URL: argv[0], Repository: argv[1], BaseBranch: "main", HeadRevision: db.PRHeadRevision, State: "open"}
		_ = json.NewEncoder(os.Stdout).Encode(info)
		return 0
	}
	if mode == "completion" {
		var request CompletionRequest
		if e := json.NewDecoder(os.Stdin).Decode(&request); e != nil {
			panic(e)
		}
		cwd, _ := os.Getwd()
		if request.PR.State != "merged" || request.Claimant != os.Getenv("BEADS_ACTOR") || canonical(request.Beads.Directory) != canonical(os.Getenv("BEADS_DIR")) || canonical(cwd) != canonical(request.WorktreePath) || len(argv) != 1 || canonical(argv[0]) != canonical(cwd) {
			fmt.Fprintln(os.Stderr, "invalid completion dispatch")
			return 9
		}
		db.CompletionCalls++
		if db.CompletionMode != "no-close" {
			for i := range db.Tasks {
				if db.Tasks[i].ID == request.TaskID {
					db.Tasks[i].Status = "closed"
				}
			}
		}
		receipt := CompletionReceipt{SchemaVersion: 1, TaskID: request.TaskID, RunID: request.RunID, PRURL: request.PR.URL, Status: "completed"}
		if db.FailCompletion > 0 {
			db.FailCompletion--
			writeFake(path, db)
			receipt.Status = "failed"
			receipt.Error = ptr("BV refresh unavailable after task close")
			receipt.Details = json.RawMessage(`{"changed":true}`)
			json.NewEncoder(os.Stdout).Encode(receipt)
			fmt.Fprintln(os.Stderr, *receipt.Error)
			return 2
		}
		writeFake(path, db)
		switch db.CompletionMode {
		case "wrong-run":
			receipt.RunID = "wrong"
		case "wrong-task":
			receipt.TaskID = "other"
		case "wrong-pr":
			receipt.PRURL = "http://wrong/pulls/1"
		case "malformed":
			fmt.Print("not JSON")
			return 0
		}
		json.NewEncoder(os.Stdout).Encode(receipt)
		return 0
	}
	if mode == "codex" {
		b, _ := io.ReadAll(os.Stdin)
		get := func(key string) string {
			matches := regexp.MustCompile(`"` + key + `": "([^"]+)"`).FindSubmatch(b)
			if len(matches) != 2 {
				panic("missing prompt context " + key)
			}
			return string(matches[1])
		}
		id := get("task_id")
		runID := get("run_id")
		cwd, _ := os.Getwd()
		if os.Getenv("BEADS_ACTOR") != get("claimant") || canonical(os.Getenv("BEADS_DIR")) != canonical(filepath.Join(filepath.Dir(path), "shared beads")) {
			fmt.Fprintln(os.Stderr, "incorrect shared Beads environment")
			return 8
		}
		db.Starts[id]++
		writeFake(path, db)
		fmt.Println(`{"type":"thread.started","thread_id":"test-session"}`)
		if db.Behavior == "conversation-stream" {
			trigger := path + ".conversation-trigger"
			deadline := time.Now().Add(90 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(trigger); err == nil {
					break
				}
				time.Sleep(25 * time.Millisecond)
			}
			if _, err := os.Stat(trigger); err != nil {
				fmt.Fprintln(os.Stderr, "conversation trigger was not delivered")
				return 7
			}
			fmt.Println(`{"type":"turn.started"}`)
			message := map[string]any{"type": "item.completed", "item": map[string]any{"id": "stream-message", "type": "agent_message", "phase": "commentary", "text": strings.Repeat("窗口仍在采集🙂", 1000)}}
			if err := json.NewEncoder(os.Stdout).Encode(message); err != nil {
				return 8
			}
			fmt.Println(`{"type":"turn.completed"}`)
			time.Sleep(time.Hour)
		}
		if db.Behavior == "sleep" {
			exe, _ := os.Executable()
			child := exec.Command(exe, "--fake", "sleep")
			if e := child.Start(); e != nil {
				panic(e)
			}
			os.WriteFile(path+".child", []byte(strconv.Itoa(child.Process.Pid)), 0600)
			time.Sleep(time.Hour)
		}
		if db.Behavior == "exit" {
			return 7
		}
		if db.Behavior == "missing" {
			return 0
		}
		if db.Behavior == "malformed" {
			os.WriteFile(arg(argv, "--output-last-message"), []byte("not json"), 0600)
			return 0
		}
		n := strings.TrimPrefix(id, "t-")
		result := Result{TaskID: id, RunID: runID, Outcome: "pr_submitted", PRURL: ptr(db.PRBase + "/owner/repo/pulls/" + n), WorktreePath: ptr(cwd), BaseCommit: ptr(get("base_commit")), Summary: ptr("done")}
		if db.Behavior == "wrong-worktree" {
			result.WorktreePath = ptr(filepath.Dir(path))
		}
		if db.Behavior == "wrong-commit" {
			result.BaseCommit = ptr(strings.Repeat("f", 40))
		}
		if db.Behavior == "wrong-id" {
			result.TaskID = "other"
		}
		if db.Behavior == "wrong-url" {
			result.PRURL = ptr("https://wrong.example/owner/repo/pulls/1")
		}
		out, _ := json.Marshal(result)
		if e := os.WriteFile(arg(argv, "--output-last-message"), out, 0600); e != nil {
			panic(e)
		}
		fmt.Println(`{"type":"turn.completed"}`)
		return 0
	}
	if db.Behavior == "bd-unavailable" {
		fmt.Fprintln(os.Stderr, "unavailable")
		return 2
	}
	var out []Task = []Task{}
	find := func(id string) *Task {
		for i := range db.Tasks {
			if db.Tasks[i].ID == id {
				return &db.Tasks[i]
			}
		}
		return nil
	}
	switch argv[0] {
	case "context":
		if db.Behavior == "slow-context" {
			os.WriteFile(path+".context-started", []byte("ready"), 0600)
			time.Sleep(time.Hour)
		}
		dir := filepath.Join(filepath.Dir(path), "shared beads")
		project := "fixture-project"
		cwd, _ := os.Getwd()
		if db.Behavior == "wrong-database" && canonical(cwd) != canonical(filepath.Join(filepath.Dir(path), "repo")) {
			project = "other-project"
		}
		json.NewEncoder(os.Stdout).Encode(BeadsContext{Directory: dir, ProjectID: project, Database: "fixture-db", Backend: "dolt"})
		return 0
	case "ready":
		if db.Behavior == "slow-ready-control" {
			time.Sleep(time.Second)
		}
		for _, task := range db.Tasks {
			if task.Status != "open" || task.Assignee != "" {
				continue
			}
			dep := db.Dependencies[task.ID]
			if dep != "" && find(dep).Status != "closed" {
				continue
			}
			out = append(out, task)
		}
	case "list":
		for _, task := range db.Tasks {
			if task.Assignee == arg(argv, "--assignee") {
				out = append(out, task)
			}
		}
	case "show":
		if task := find(arg(argv, "--id")); task != nil {
			out = append(out, *task)
		}
	case "update":
		task := find(argv[1])
		if db.Behavior == "claim-conflict" {
			task.Assignee = "another-agent"
			task.Status = "in_progress"
			writeFake(path, db)
			return 0
		}
		task.Assignee = arg(argv, "--actor")
		task.Status = "in_progress"
		db.Claims[task.ID]++
		out = append(out, *task)
		writeFake(path, db)
		if db.Behavior == "path-conflict" {
			v, e := LoadRuntime(filepath.Join(filepath.Dir(path), "runtime", "runtime.json"))
			if e != nil {
				panic(e)
			}
			intent := v.Runs[len(v.Runs)-1].WorkspaceIntent
			if e = os.MkdirAll(intent.Path, 0700); e != nil {
				panic(e)
			}
			os.WriteFile(filepath.Join(intent.Path, "keep.txt"), []byte("unrelated existing contents"), 0600)
		}
	case "close":
		if db.FailClose > 0 {
			db.FailClose--
			writeFake(path, db)
			return 2
		}
		task := find(argv[1])
		task.Status = "closed"
		out = append(out, *task)
		writeFake(path, db)
	default:
		return 2
	}
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
	return 0
}

type fixture struct {
	c              Config
	db             string
	mu             sync.Mutex
	states         map[string]string
	prHeadRevision string
	server         *httptest.Server
}

func newFixture(t *testing.T, tasks ...Task) *fixture {
	t.Helper()
	base := filepath.Join(t.TempDir(), "中文 path with spaces")
	repo := filepath.Join(base, "repo")
	runs := filepath.Join(base, "runtime")
	os.MkdirAll(repo, 0700)
	os.MkdirAll(runs, 0700)
	os.MkdirAll(filepath.Join(base, "shared beads"), 0700)
	f := &fixture{db: filepath.Join(base, "fake.json"), states: map[string]string{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method != "GET" {
			t.Errorf("unexpected mutation %s", r.Method)
		}
		if r.URL.Path == "/api/v1/repos/owner/repo" {
			fmt.Fprint(w, `{"full_name":"owner/repo"}`)
			return
		}
		n := filepath.Base(r.URL.Path)
		state := f.states[n]
		if state == "failure" {
			w.WriteHeader(503)
			return
		}
		if state == "" {
			state = "open"
		}
		branch := "main"
		if state == "wrong-base" {
			branch = "other"
			state = "merged"
		}
		merged := state == "merged"
		rawState := state
		if merged || state == "closed_unmerged" {
			rawState = "closed"
		}
		payload := map[string]any{"number": mustInt(n), "html_url": f.server.URL + "/owner/repo/pulls/" + n, "state": rawState, "merged": merged, "merged_at": nil, "merge_commit_sha": nil, "base": map[string]any{"ref": branch, "repo": map[string]string{"full_name": "owner/repo"}}, "head": map[string]string{"sha": f.prHeadRevision}}
		if merged {
			payload["merged_at"] = now()
			payload["merge_commit_sha"] = strings.Repeat("b", 40)
		}
		json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(f.server.Close)
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	git, e := exec.LookPath("git")
	if e != nil {
		t.Fatal(e)
	}
	remote := filepath.Join(base, "remote.git")
	for _, args := range [][]string{{"init", "--quiet", "-b", "main", repo}, {"init", "--quiet", "--bare", remote}, {"-C", repo, "config", "core.hooksPath", filepath.Join(base, "no-hooks")}, {"-C", repo, "remote", "add", "origin", remote}, {"-C", repo, "remote", "add", "web", f.server.URL + "/owner/repo.git"}} {
		if out, e := exec.Command(git, args...).CombinedOutput(); e != nil {
			t.Fatalf("git: %s %v", out, e)
		}
	}
	for _, name := range []string{"start", "finish"} {
		os.WriteFile(filepath.Join(repo, name+".md"), []byte("<!-- steerline:"+name+"-v1 -->\nFixture skill"), 0600)
	}
	for _, args := range [][]string{{"-C", repo, "add", "start.md", "finish.md"}, {"-C", repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "Fixture baseline"}, {"-C", repo, "push", "--quiet", "origin", "main"}} {
		if out, e := exec.Command(git, args...).CombinedOutput(); e != nil {
			t.Fatalf("git fixture: %v %s", e, out)
		}
	}
	f.c = Config{Repository: repo, Remote: "origin", Branch: "main", RunDir: runs, RuntimeFile: "runtime.json", Claimant: "test-claimant", Git: Command{Executable: git}, BD: Command{Executable: exe, Args: []string{"--fake", "bd", f.db}}, Codex: Command{Executable: exe, Args: []string{"--fake", "codex", f.db}}, StartSkill: Skill{Path: "start.md", Invocation: "Read start.md"}, FinishSkill: Skill{Path: "finish.md", Invocation: "Read finish.md"}, ExecutableTypes: []string{"task", "bug", "feature", "chore"}, TaskPoll: "100ms", PRPoll: "100ms", Heartbeat: "20ms", CommandTimeout: "5s", ShutdownGrace: "80ms", Sandbox: "workspace-write", PR: PRConfig{Kind: "gitea", BaseURL: f.server.URL, Repository: "owner/repo"}, identity: canonical(filepath.Join(repo, ".git"))}
	configBytes, _ := json.Marshal(f.c)
	configDigest := sha256.Sum256(configBytes)
	f.c.configDigest = hex.EncodeToString(configDigest[:])
	if tasks == nil {
		tasks = []Task{}
	}
	writeFake(f.db, fakeDB{Tasks: tasks, Dependencies: map[string]string{}, Claims: map[string]int{}, Starts: map[string]int{}, PRBase: f.server.URL})
	seedFakeCapabilities(t, f.c)
	return f
}

func seedFakeCapabilities(t *testing.T, c Config) {
	t.Helper()
	if err := os.MkdirAll(c.RunDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.RunDir, "fake-native-evidence.json")
	data := []byte("fake platform fixture: " + t.Name())
	if err := AtomicWrite(path, data); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	evidence := path + "#sha256=" + hex.EncodeToString(sum[:])
	backend, protocol := backendFor(c, BeadsContext{}).Identity()
	version := "fake-codex/1"
	if backend == BackendOrca {
		version = "fake-orca/1"
	}
	report := capabilityReport(backend, protocol, targetFor(c), agentFor(c), version)
	for i := range report.Capabilities {
		name := report.Capabilities[i].Name
		if name == "output_turns" || name == "output_final_answer" {
			continue
		}
		if backend == BackendLocal && name == "reply_continue" || backend == BackendOrca && name == "retry" || name == "resource_settlement" {
			report.Capabilities[i].Status = CapabilityUnsupported
			continue
		}
		report.Capabilities[i].Status = CapabilityVerified
		report.Capabilities[i].Evidence = evidence
	}
	if err := (executionRecords{root: c.RunDir}).saveCapabilityReport(report); err != nil {
		t.Fatal(err)
	}
}
func mustInt(s string) int { n, _ := strconv.Atoi(s); return n }
func task(id string) Task {
	return Task{ID: id, Title: "中文 task & $(no shell)", Status: "open", Type: "task"}
}
func (f *fixture) pr(n, state string) { f.mu.Lock(); defer f.mu.Unlock(); f.states[n] = state }
func (f *fixture) service(t *testing.T) *Service {
	t.Helper()
	seedFakeCapabilities(t, f.c)
	s, e := NewService(f.c)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func finishWorker(t *testing.T, s *Service) {
	t.Helper()
	if s.worker == nil {
		t.Fatal("worker not launched")
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-s.worker.Events:
			if ok {
				if e := s.event(ev); e != nil {
					t.Fatal(e)
				}
			}
		case e := <-s.worker.Done:
			if e = s.finished(e); e != nil {
				t.Fatal(e)
			}
			return
		case <-timer.C:
			s.worker.Kill()
			t.Fatal("worker timeout")
		}
	}
}
func require(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}

func TestLifecycleDependenciesAndRestart(t *testing.T) {
	f := newFixture(t, task("t-1"), task("t-2"), task("t-3"))
	db := readFake(f.db)
	db.Dependencies["t-3"] = "t-1"
	writeFake(f.db, db)
	ctx := context.Background()
	s := f.service(t)
	require(t, s.dispatch(ctx))
	finishWorker(t, s)
	require(t, s.pollPRs(ctx))
	if s.State.Runs[0].FinishTaskStatus != "waiting_pr_merge" || s.State.SlotOccupied {
		t.Fatal("open PR must release execution slot and remain unfinished")
	}
	require(t, s.dispatch(ctx))
	finishWorker(t, s)
	require(t, s.dispatch(ctx))
	if len(s.State.Runs) != 2 {
		t.Fatal("dependent launched early")
	}
	s = f.service(t)
	require(t, s.dispatch(ctx))
	if s.worker != nil {
		t.Fatal("restart relaunched waiting task")
	}
	f.pr("1", "merged")
	db = readFake(f.db)
	db.FailClose = 1
	writeFake(f.db, db)
	if s.pollPRs(ctx) == nil {
		t.Fatal("expected failed Beads close")
	}
	if s.State.Runs[0].FinishTaskStatus != "waiting_pr_merge" {
		t.Fatal("close failure became complete")
	}
	s = f.service(t)
	require(t, s.pollPRs(ctx))
	if s.State.Runs[0].FinishTaskStatus != "completed" {
		t.Fatal("close retry failed")
	}
	require(t, s.dispatch(ctx))
	finishWorker(t, s)
	prompt, e := os.ReadFile(filepath.Join(f.c.RunDir, "runs", s.State.Runs[2].RunID, "prompt.txt"))
	require(t, e)
	if !strings.Contains(string(prompt), "scheduler has already fetched") || !strings.Contains(string(prompt), `"branch": "main"`) {
		t.Fatal("missing fresh base contract")
	}
	db = readFake(f.db)
	for _, id := range []string{"t-1", "t-2", "t-3"} {
		if db.Claims[id] != 1 || db.Starts[id] != 1 {
			t.Fatalf("%s executed more than once: %+v", id, db)
		}
	}
}
func TestBadHandoffsNeverClose(t *testing.T) {
	for _, behavior := range []string{"missing", "malformed", "wrong-id", "wrong-url", "wrong-worktree", "wrong-commit", "exit"} {
		t.Run(behavior, func(t *testing.T) {
			f := newFixture(t, task("t-1"))
			db := readFake(f.db)
			db.Behavior = behavior
			writeFake(f.db, db)
			s := f.service(t)
			require(t, s.dispatch(context.Background()))
			finishWorker(t, s)
			require(t, s.pollPRs(context.Background()))
			if s.State.Runs[0].FinishTaskStatus != "blocked" || readFake(f.db).Tasks[0].Status == "closed" {
				t.Fatal("invalid result closed a task")
			}
		})
	}
}
func TestPRFailuresNeverClose(t *testing.T) {
	for _, state := range []string{"failure", "closed_unmerged", "wrong-base"} {
		t.Run(state, func(t *testing.T) {
			f := newFixture(t, task("t-1"))
			s := f.service(t)
			require(t, s.dispatch(context.Background()))
			finishWorker(t, s)
			f.pr("1", state)
			require(t, s.pollPRs(context.Background()))
			if readFake(f.db).Tasks[0].Status == "closed" {
				t.Fatal("unsafe PR close")
			}
			if state == "failure" {
				if s.State.Runs[0].PR.State != "unknown" {
					t.Fatal("failed query not unknown")
				}
				f.pr("1", "merged")
				require(t, s.pollPRs(context.Background()))
				if s.State.Runs[0].FinishTaskStatus != "completed" {
					t.Fatal("query failure did not retry")
				}
			}
		})
	}
}
func TestRecoverOrphanAndRunningWithoutRestart(t *testing.T) {
	f := newFixture(t, task("t-1"))
	db := readFake(f.db)
	db.Tasks[0].Assignee = f.c.Claimant
	db.Tasks[0].Status = "in_progress"
	writeFake(f.db, db)
	s := f.service(t)
	if s.dispatch(context.Background()) == nil {
		t.Fatal("orphan must pause dispatch")
	}
	r := s.State.Runs[0]
	if r.ExecutionStatus != "unknown" || len(readFake(f.db).Starts) != 0 {
		t.Fatal("orphan relaunched")
	}
	r.ExecutionStatus = "running"
	r.Process = &ProcessIdentity{Kind: "os", PID: os.Getpid(), Created: "wrong identity", Executable: "wrong"}
	require(t, s.save())
	s = f.service(t)
	if s.State.Runs[0].ExecutionStatus != "unknown" || s.State.Runs[0].FinishTaskStatus != "blocked" || !s.State.SlotOccupied {
		t.Fatal("orphan with no fixed binding was not blocked")
	}
}
func TestClaimConflictAndGrouping(t *testing.T) {
	f := newFixture(t, task("t-1"))
	db := readFake(f.db)
	db.Behavior = "claim-conflict"
	writeFake(f.db, db)
	s := f.service(t)
	require(t, s.dispatch(context.Background()))
	if s.worker != nil || len(s.State.Runs) != 0 {
		t.Fatal("lost claim started worker")
	}
	snapshot, err := s.domainStore.Query(context.Background(), f.c.identity)
	require(t, err)
	if len(snapshot.WorkflowRuns) != 0 || len(snapshot.Executions) != 0 || len(snapshot.Attempts) != 0 || len(snapshot.Tasks) != 0 {
		t.Fatalf("rejected claim left a ghost workflow: %+v", snapshot)
	}
	aggregate := task("group")
	aggregate.Metadata = map[string]any{"split.role": "aggregate", "split.registered": "1"}
	if executable(aggregate, f.c.ExecutableTypes) {
		t.Fatal("aggregate executable")
	}
}
func TestBeadsUnavailableStopsDispatch(t *testing.T) {
	f := newFixture(t, task("t-1"))
	db := readFake(f.db)
	db.Behavior = "bd-unavailable"
	writeFake(f.db, db)
	s := f.service(t)
	if s.dispatch(context.Background()) == nil || s.worker != nil {
		t.Fatal("dispatched without Beads")
	}
}
func TestPreflightReadsSkillsAndNativeCommands(t *testing.T) {
	f := newFixture(t)
	f.c.Remote = "web"
	require(t, f.c.Preflight(context.Background(), true))
	require(t, os.WriteFile(filepath.Join(f.c.Repository, "finish.md"), []byte("bd close; incompatible legacy skill"), 0600))
	if e := f.c.Preflight(context.Background(), true); e == nil || !strings.Contains(e.Error(), "skill contract conflict") {
		t.Fatalf("expected skill contract conflict: %v", e)
	}
}
func TestRepositoryLockIndependentOfRuntime(t *testing.T) {
	f := newFixture(t)
	l, e := LockRepository(f.c.identity)
	require(t, e)
	defer l.Close()
	exe, _ := os.Executable()
	e = exec.Command(exe, "--fake", "lock", f.c.identity).Run()
	if e == nil {
		t.Fatal("second process acquired same repository lock")
	}
	l.Close()
	l, e = LockRepository(f.c.identity)
	require(t, e)
	l.Close()
}
func TestHeartbeatAndConcurrentReader(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	defer cancel()
	exe, _ := os.Executable()
	out, e := exec.Command(exe, "--fake", "reader", filepath.Join(f.c.RunDir, f.c.RuntimeFile)).CombinedOutput()
	require(t, e)
	if len(out) == 0 {
		t.Fatal("reader parsed no snapshots")
	}
	first, e := LoadRuntime(filepath.Join(f.c.RunDir, f.c.RuntimeFile))
	require(t, e)
	deadline := time.Now().Add(time.Second)
	for {
		second, e := LoadRuntime(filepath.Join(f.c.RunDir, f.c.RuntimeFile))
		require(t, e)
		if second != nil && second.HeartbeatAt.After(first.HeartbeatAt) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("idle heartbeat stalled")
		}
		time.Sleep(20 * time.Millisecond)
	}
	require(t, RequestStop(f.c))
	select {
	case e := <-done:
		require(t, e)
	case <-time.After(10 * time.Second):
		t.Fatal("stop timed out")
	}
	last, e := LoadRuntime(filepath.Join(f.c.RunDir, f.c.RuntimeFile))
	require(t, e)
	if last.Service.Status != "stopped" {
		t.Fatal("stop status missing")
	}
}
func TestProjectionFailureDoesNotRollbackSQLiteState(t *testing.T) {
	f := newFixture(t, task("t-1"))
	s := f.service(t)
	require(t, s.save())
	before, _, e := s.domainStore.LoadProjection(context.Background(), f.c.identity)
	require(t, e)
	if len(before) == 0 {
		t.Fatal("initial database projection missing")
	}
	s.store.Path = filepath.Join(f.c.RunDir, "missing-directory", "runtime.json")
	s.State.Service.Status = "maintenance"
	if e = s.save(); e != nil {
		t.Fatalf("projection failure rolled back committed database state: %v", e)
	}
	if s.store.Err() != nil || s.store.ProjectionErr() == nil {
		t.Fatal("projection failure was not reported separately from database persistence")
	}
	after, revision, e := s.domainStore.LoadProjection(context.Background(), f.c.identity)
	require(t, e)
	if revision <= s.State.Revision-1 || len(after) == 0 {
		t.Fatal("SQLite projection did not commit before export failed")
	}
	var projected Runtime
	require(t, json.Unmarshal(after, &projected))
	if projected.Service.Status != "maintenance" {
		t.Fatalf("committed state missing from database: %+v", projected.Service)
	}
}

func TestUnverifiedBackendCannotClaimOrStart(t *testing.T) {
	f := newFixture(t, task("t-1"))
	records := executionRecords{root: f.c.RunDir}
	key := capabilityReport(BackendLocal, ProtocolLocalProcess, targetFor(f.c), agentFor(f.c), "fake-codex/1")
	if err := os.Remove(records.capabilityPath(key)); err != nil {
		t.Fatal(err)
	}
	s, err := NewService(f.c)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.dispatch(context.Background()); err == nil {
		t.Fatal("unverified start capability was accepted")
	}
	if len(s.State.Runs) != 0 || len(readFake(f.db).Claims) != 0 || len(readFake(f.db).Starts) != 0 {
		t.Fatal("unverified backend caused task side effects")
	}
	report, err := records.loadCapabilityReport(key)
	if err != nil || capabilityStatus(report, "start") != CapabilityNotTested {
		t.Fatalf("missing not_tested report: %#v, %v", report, err)
	}
	seedFakeCapabilities(t, f.c)
	t.Setenv("STEERLINE_FAKE_CODEX_VERSION", "fake-codex/2")
	if err = s.dispatch(context.Background()); err == nil {
		t.Fatal("changed backend version reused prior verification")
	}
	if len(readFake(f.db).Claims) != 0 {
		t.Fatal("version change claimed a task")
	}
}

func TestChangedCapabilityEvidenceBlocksClaim(t *testing.T) {
	f := newFixture(t, task("t-1"))
	s := f.service(t)
	if err := os.WriteFile(filepath.Join(f.c.RunDir, "fake-native-evidence.json"), []byte("changed after verification"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(context.Background()); err == nil {
		t.Fatal("tampered capability evidence was accepted")
	}
	if len(readFake(f.db).Claims) != 0 || len(s.State.Runs) != 0 {
		t.Fatal("tampered evidence caused a task side effect")
	}
}
func TestShutdownKillsProcessTree(t *testing.T) {
	f := newFixture(t, task("t-1"))
	db := readFake(f.db)
	db.Behavior = "sleep"
	writeFake(f.db, db)
	s := f.service(t)
	require(t, s.dispatch(context.Background()))
	var childPID int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, e := os.ReadFile(f.db + ".child")
		if e == nil {
			childPID = mustInt(string(b))
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		s.worker.Kill()
		t.Fatal("child did not start")
	}
	s.shutdown()
	if s.worker != nil || s.State.Runs[0].ExecutionStatus != "failed" {
		t.Fatal("shutdown failed")
	}
	exe, _ := os.Executable()
	check := &ProcessIdentity{PID: childPID, Created: "", Executable: exe}
	if result := inspectProcess(check); result == "PID belongs to a different process" {
		t.Fatal("descendant still exists after job termination")
	}
}

func TestLocalDetachObservationKeepsWorkerAlive(t *testing.T) {
	f := newFixture(t, task("t-1"))
	db := readFake(f.db)
	db.Behavior = "sleep"
	writeFake(f.db, db)
	s := f.service(t)
	require(t, s.dispatch(context.Background()))
	w := s.worker
	if w == nil {
		t.Fatal("worker did not start")
	}
	defer w.Kill()
	backend := localExecutionBackend{config: f.c}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require(t, backend.DetachObservation(ctx, w))
	require(t, backend.DetachObservation(ctx, w))
	for {
		select {
		case _, ok := <-w.Events:
			if !ok {
				goto observerClosed
			}
		case <-ctx.Done():
			t.Fatal("local observer did not close after detach")
		}
	}
observerClosed:
	identity := w.proc.Identity()
	if got := inspectProcess(&identity); got != "matching process still exists" {
		t.Fatalf("detach changed worker liveness: %s", got)
	}
	s.shutdown()
	if s.State.SlotOccupied || s.worker != nil {
		t.Fatal("confirmed shutdown retained local execution")
	}
}

func TestLateAttemptEventOnlyEntersItsHistory(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	runID, currentAttempt := newID(), newID()
	r := &Run{TaskID: "t-history", RunID: runID, AttemptID: currentAttempt, Claimant: f.c.Claimant, Host: "local", BeadsStatus: "in_progress", ExecutionStatus: "running", FinishTaskStatus: "pending"}
	s.State.Runs = []*Run{r}
	r.AttemptID, r.ExecutionStatus, r.FinishTaskStatus = runID, "failed", "blocked"
	require(t, s.save())
	r.AttemptID, r.ExecutionStatus, r.FinishTaskStatus = currentAttempt, "running", "pending"
	r.DomainAttemptNumber++
	require(t, s.save())
	s.State.CurrentTask = ptr(r.TaskID)
	s.State.SlotOccupied = true
	s.active = r
	if err := os.MkdirAll(filepath.Join(f.c.RunDir, "runs", runID), 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.event(WorkerEvent{RunID: runID, AttemptID: runID, Session: "old-session", Summary: "old fact", Activity: ActivityEvent{Source: "codex", Kind: "note", Title: "late-old-attempt"}}); err != nil {
		t.Fatal(err)
	}
	if r.SessionID != nil || r.LastEvent != nil || r.ExecutionStatus != "running" || r.FinishTaskStatus != "pending" || r.BeadsStatus != "in_progress" || !s.State.SlotOccupied || deref(s.State.CurrentTask) != r.TaskID {
		t.Fatalf("late fact changed current execution: %+v", r)
	}
	history, err := os.ReadFile(filepath.Join(f.c.RunDir, "runs", runID, "activity.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(history), "late-old-attempt") {
		t.Fatal("late fact was not retained with its original attempt")
	}
	if err = s.event(WorkerEvent{RunID: newID(), AttemptID: newID(), Summary: "foreign", Activity: ActivityEvent{Kind: "note", Title: "foreign"}}); err != nil {
		t.Fatal(err)
	}
	if r.LastEvent != nil || !s.State.SlotOccupied {
		t.Fatal("foreign fact changed the active run or slot")
	}
}

func TestManualRecoveryDoesNotLaunchWorker(t *testing.T) {
	f, s, r := failedStartup(t)
	result := Result{TaskID: r.TaskID, RunID: r.RunID, Outcome: "pr_submitted", PRURL: ptr(f.server.URL + "/owner/repo/pulls/1"), WorktreePath: r.WorktreePath, BaseCommit: r.BaseCommit, Summary: ptr("recovered result")}
	resultBytes, err := json.Marshal(result)
	require(t, err)
	head, err := gitRunner(f.c, *r.WorktreePath).Output(context.Background(), f.c.Git, "rev-parse", "HEAD")
	require(t, err)
	fake := readFake(f.db)
	fake.PRHeadRevision = strings.TrimSpace(string(head))
	writeFake(f.db, fake)
	f.mu.Lock()
	f.prHeadRevision = fake.PRHeadRevision
	f.mu.Unlock()
	sourceResultPath := *r.ResultPath
	require(t, os.WriteFile(sourceResultPath, resultBytes, 0600))
	r.ExecutionStatus = "unknown"
	r.FinishTaskStatus = "blocked"
	require(t, s.save())
	require(t, s.markStoppedCore(r.RunID, "test-operator", "I verified the original worker has stopped"))
	if s.State.Runs[0].ExecutionStatus != "failed" {
		t.Fatal("manual stop not recorded")
	}
	resultDigest := sha256.Sum256(resultBytes)
	require(t, os.WriteFile(sourceResultPath, []byte(`{"task_id":"replacement","run_id":"replacement","outcome":"blocked","pr_url":null,"worktree_path":null,"base_commit":null,"summary":"replacement","error":null}`), 0600))
	require(t, s.acceptResultBytes(context.Background(), r.RunID, newID(), "test-operator", "I verified the original worker has stopped", resultBytes, hex.EncodeToString(resultDigest[:])))
	if s.State.Runs[0].FinishTaskStatus != "waiting_pr_merge" || readFake(f.db).Starts["t-1"] != 1 {
		t.Fatal("manual acceptance started another worker")
	}
	acceptedPath := *s.State.Runs[0].ResultPath
	acceptedBytes, err := os.ReadFile(acceptedPath)
	require(t, err)
	if string(acceptedBytes) != string(resultBytes) || acceptedPath == sourceResultPath || s.State.Service.Status == "stopped" {
		t.Fatal("accept-result did not preserve the frozen bytes or stopped the online service")
	}
	f.pr("1", "merged")
	require(t, s.pollPRs(context.Background()))
	if s.State.Runs[0].FinishTaskStatus != "completed" {
		t.Fatal("manual recovery failed to finish")
	}
}
