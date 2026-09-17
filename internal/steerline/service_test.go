package steerline

import (
	"context"
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
	Tasks        []Task
	Dependencies map[string]string
	Claims       map[string]int
	Starts       map[string]int
	FailClose    int
	Behavior     string
	PRBase       string
}

func TestMain(m *testing.M) {
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
	if mode == "sleep" {
		time.Sleep(time.Hour)
		return 0
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
	path := args[1]
	argv := args[2:]
	if arg(argv, "--help") != "" || strings.Contains(strings.Join(argv, " "), "--help") {
		fmt.Println("--unassigned --limit --claim --actor --assignee --all --reason --json --output-schema --output-last-message --sandbox")
		return 0
	}
	db := readFake(path)
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
		db.Starts[id]++
		writeFake(path, db)
		fmt.Println(`{"type":"thread.started","thread_id":"test-session"}`)
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
		result := Result{TaskID: id, RunID: runID, Outcome: "pr_submitted", PRURL: ptr(db.PRBase + "/owner/repo/pulls/" + n), WorktreePath: ptr(filepath.Dir(path)), BaseCommit: ptr(strings.Repeat("a", 40)), Summary: ptr("done")}
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
	case "ready":
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
	c      Config
	db     string
	mu     sync.Mutex
	states map[string]string
	server *httptest.Server
}

func newFixture(t *testing.T, tasks ...Task) *fixture {
	t.Helper()
	base := filepath.Join(t.TempDir(), "中文 path with spaces")
	repo := filepath.Join(base, "repo")
	runs := filepath.Join(base, "runtime")
	os.MkdirAll(repo, 0700)
	os.MkdirAll(runs, 0700)
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
		payload := map[string]any{"number": mustInt(n), "html_url": f.server.URL + "/owner/repo/pulls/" + n, "state": rawState, "merged": merged, "merged_at": nil, "merge_commit_sha": nil, "base": map[string]any{"ref": branch, "repo": map[string]string{"full_name": "owner/repo"}}}
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
	for _, args := range [][]string{{"init", "--quiet", repo}, {"-C", repo, "remote", "add", "origin", f.server.URL + "/owner/repo.git"}} {
		if out, e := exec.Command(git, args...).CombinedOutput(); e != nil {
			t.Fatalf("git: %s %v", out, e)
		}
	}
	for _, name := range []string{"start", "finish"} {
		os.WriteFile(filepath.Join(repo, name+".md"), []byte("<!-- steerline:"+name+"-v1 -->\nFixture skill"), 0600)
	}
	f.c = Config{Repository: repo, Remote: "origin", Branch: "main", RunDir: runs, RuntimeFile: "runtime.json", Claimant: "test-claimant", Git: Command{Executable: git}, BD: Command{Executable: exe, Args: []string{"--fake", "bd", f.db}}, Codex: Command{Executable: exe, Args: []string{"--fake", "codex", f.db}}, StartSkill: Skill{Path: "start.md", Invocation: "Read start.md"}, FinishSkill: Skill{Path: "finish.md", Invocation: "Read finish.md"}, ExecutableTypes: []string{"task", "bug", "feature", "chore"}, TaskPoll: "100ms", PRPoll: "100ms", Heartbeat: "20ms", CommandTimeout: "5s", ShutdownGrace: "80ms", Sandbox: "workspace-write", PR: PRConfig{Kind: "gitea", BaseURL: f.server.URL, Repository: "owner/repo"}, identity: canonical(filepath.Join(repo, ".git"))}
	if tasks == nil {
		tasks = []Task{}
	}
	writeFake(f.db, fakeDB{Tasks: tasks, Dependencies: map[string]string{}, Claims: map[string]int{}, Starts: map[string]int{}, PRBase: f.server.URL})
	return f
}
func mustInt(s string) int { n, _ := strconv.Atoi(s); return n }
func task(id string) Task {
	return Task{ID: id, Title: "中文 task & $(no shell)", Status: "open", Type: "task"}
}
func (f *fixture) pr(n, state string) { f.mu.Lock(); defer f.mu.Unlock(); f.states[n] = state }
func (f *fixture) service(t *testing.T) *Service {
	t.Helper()
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
	if !strings.Contains(string(prompt), "fetch the latest") || !strings.Contains(string(prompt), `"branch": "main"`) {
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
	for _, behavior := range []string{"missing", "malformed", "wrong-id", "wrong-url", "exit"} {
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
	r.Process = &ProcessIdentity{PID: os.Getpid(), Created: "wrong identity", Executable: "wrong"}
	require(t, s.save())
	s = f.service(t)
	if s.State.Runs[0].ExecutionStatus != "unknown" || !strings.Contains(*s.State.Runs[0].Error, "different process") {
		t.Fatal("PID identity not checked")
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
	time.Sleep(60 * time.Millisecond)
	second, e := LoadRuntime(filepath.Join(f.c.RunDir, f.c.RuntimeFile))
	require(t, e)
	if !second.HeartbeatAt.After(first.HeartbeatAt) {
		t.Fatal("idle heartbeat stalled")
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
func TestPersistenceFailureStopsNewWork(t *testing.T) {
	f := newFixture(t, task("t-1"))
	s := f.service(t)
	require(t, s.save())
	original, e := os.ReadFile(s.store.Path)
	require(t, e)
	s.store.Path = filepath.Join(f.c.RunDir, "missing-directory", "runtime.json")
	if s.dispatch(context.Background()) == nil {
		t.Fatal("expected persistence failure")
	}
	db := readFake(f.db)
	if len(db.Claims) != 0 || len(db.Starts) != 0 {
		t.Fatal("side effects after persistence failure")
	}
	current, e := os.ReadFile(filepath.Join(f.c.RunDir, f.c.RuntimeFile))
	require(t, e)
	if string(current) != string(original) {
		t.Fatal("previous snapshot destroyed")
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

func TestManualRecoveryDoesNotLaunchWorker(t *testing.T) {
	f := newFixture(t, task("t-1"))
	s := f.service(t)
	require(t, s.dispatch(context.Background()))
	finishWorker(t, s)
	r := s.State.Runs[0]
	r.ExecutionStatus = "unknown"
	r.FinishTaskStatus = "blocked"
	require(t, s.save())
	s = f.service(t)
	require(t, s.MarkStopped(r.RunID))
	if s.State.Runs[0].ExecutionStatus != "failed" {
		t.Fatal("manual stop not recorded")
	}
	require(t, s.AcceptResult(context.Background(), r.RunID, *r.ResultPath))
	if s.State.Runs[0].FinishTaskStatus != "waiting_pr_merge" || readFake(f.db).Starts["t-1"] != 1 {
		t.Fatal("manual acceptance started another worker")
	}
	f.pr("1", "merged")
	require(t, s.pollPRs(context.Background()))
	if s.State.Runs[0].FinishTaskStatus != "completed" {
		t.Fatal("manual recovery failed to finish")
	}
}
