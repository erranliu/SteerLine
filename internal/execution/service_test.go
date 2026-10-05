package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// fakeDB is the isolated PR-provider state used by native workflow tests.
type fakeDB struct {
	PRHeadRevision string
	PRState        string
	PRMerged       bool
	PRMergedAt     *time.Time
	PRMergeCommit  string
}

func TestProjectRuntimeOmitsSettledDiscardedRuns(t *testing.T) {
	state := reconstructiontypes.Runtime{Tasks: []taskmodel.TaskRecord{{TaskID: "14", Status: "discarded"}, {TaskID: "17", Status: "in_progress"}, {TaskID: "18", Status: "discarded"}},
		WorkflowRuns: []executionmodel.WorkflowRunRecord{{TaskID: "14", WorkflowRunID: "old", Phase: "abandoned"}, {TaskID: "17", WorkflowRunID: "active", Phase: "executing"}, {TaskID: "18", WorkflowRunID: "pending-stop", Phase: "blocked"}},
		Executions:   []executionmodel.ExecutionRecord{{WorkflowRunID: "active", ExecutionID: "active-execution", Status: "active"}, {WorkflowRunID: "pending-stop", ExecutionID: "stopping-execution", Status: "blocked"}},
		SlotOccupied: true, CurrentTask: ptr("14")}
	require(t, projectRuntime(&state))
	if len(state.Runs) != 2 || state.Runs[0].RunID != "active" || state.Runs[1].RunID != "pending-stop" || !state.SlotOccupied || state.CurrentTask != nil {
		t.Fatalf("settled discarded run or reservation remained: %+v", state)
	}
}

type fixture struct {
	c      Config
	db     string
	commit string
	server *httptest.Server
	mu     sync.Mutex
}

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && (os.Args[1] == "mc-provider" || os.Args[1] == "mc-worker") {
		os.Exit(mcSubprocess(os.Args[1:]))
	}
	if os.Getenv("STEERLINE_LIFECYCLE_EXIT_EARLY") == "1" {
		os.Exit(23)
	}
	if os.Getenv("STEERLINE_TEST_HOST_GH_FILE") != "" && len(os.Args) > 1 && (os.Args[1] == "api" || os.Args[1] == "pr") {
		os.Exit(fakeHostGH(os.Args[1:]))
	}
	for i, arg := range os.Args {
		if arg == "--fake" {
			os.Exit(fakeMain(os.Args[i+1:]))
		}
	}
	home, err := os.MkdirTemp("", "steerline-core-fixture-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("STEERLINE_HOME", home); err != nil {
		panic(err)
	}
	code := m.Run()
	if relative, err := filepath.Rel(os.TempDir(), home); err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		_ = os.RemoveAll(home)
	}
	if testArtifactsDir != "" {
		_ = os.RemoveAll(testArtifactsDir)
	}
	os.Exit(code)
}

var (
	testArtifactsOnce sync.Once
	testArtifactsDir  string
	testArtifactsErr  error
)

func sharedTestArtifactsDir() (string, error) {
	testArtifactsOnce.Do(func() {
		testArtifactsDir, testArtifactsErr = os.MkdirTemp("", "steerline-test-artifacts-")
	})
	return testArtifactsDir, testArtifactsErr
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	config, commit := nativeTaskFixture(t)
	remoteBytes, err := exec.Command("git", "-C", config.Repository, "remote", "get-url", "origin").Output()
	if err != nil {
		t.Fatal(err)
	}
	remotePath := strings.TrimSpace(string(remoteBytes))
	if output, err := exec.Command("git", "-C", config.Repository, "push", remotePath, "main").CombinedOutput(); err != nil {
		t.Fatalf("push native fixture base: %v: %s", err, output)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{c: config, db: filepath.Join(t.TempDir(), "native-pr.json")}
	f.commit = commit
	writeFake(f.db, fakeDB{PRHeadRevision: commit})
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if delay := os.Getenv("STEERLINE_FAKE_PR_DELAY"); delay != "" {
			if duration, err := time.ParseDuration(delay); err == nil && duration > 0 {
				time.Sleep(duration)
			}
		}
		if r.URL.Path == "/api/v1/repos/owner/repo" {
			_ = json.NewEncoder(w).Encode(map[string]string{"full_name": "owner/repo"})
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/repos/owner/repo/pulls/") {
			http.NotFound(w, r)
			return
		}
		number := strings.TrimPrefix(r.URL.Path, "/api/v1/repos/owner/repo/pulls/")
		db := readFake(f.db)
		if r.Method == http.MethodPatch {
			db.PRState, db.PRMerged = "closed_unmerged", false
			db.PRMergedAt, db.PRMergeCommit = nil, ""
			writeFake(f.db, db)
		}
		state := db.PRState
		if state == "" {
			state = "open"
		}
		payload := map[string]any{
			"number": number, "html_url": f.server.URL + "/owner/repo/pulls/" + number,
			"state":  map[bool]string{true: "closed", false: state}[r.Method == http.MethodPatch],
			"merged": db.PRMerged, "merged_at": db.PRMergedAt, "merge_commit_sha": db.PRMergeCommit,
			"base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "owner/repo"}},
			"head": map[string]string{"sha": db.PRHeadRevision},
		}
		if r.Method != http.MethodGet && r.Method != http.MethodPatch {
			http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(f.server.Close)

	f.c.Remote, f.c.Branch = "origin", "main"
	t.Setenv("STEERLINE_FAKE_GIT_FETCH_REMOTE", remotePath)
	f.c.Git = executionbackend.Command{Executable: executable, Args: []string{"--fake", "git-proxy", git}}
	if output, err := exec.Command("git", "-C", f.c.Repository, "remote", "set-url", "origin", f.server.URL+"/owner/repo.git").CombinedOutput(); err != nil {
		t.Fatalf("set native fixture remote: %v: %s", err, output)
	}
	f.c.Claimant = "native-fixture"
	f.c.Heartbeat, f.c.TaskPoll, f.c.PRPoll, f.c.CommandTimeout, f.c.ShutdownGrace = "100ms", "100ms", "100ms", "5s", "1s"
	f.c.PR = publication.Config{Kind: "gitea", BaseURL: f.server.URL, Repository: "owner/repo"}
	setNeutralProfile(t, &f.c, neutralSettings{Executable: executable})
	f.c.WorkflowProtocol = executionmodel.Version
	if err = f.c.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) service(t *testing.T) *Service {
	t.Helper()
	store, err := openFixtureProject(context.Background(), &f.c)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	service, err := newTestService(f.c)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestIdleServiceDoesNotPublishOnNoOpPolls(t *testing.T) {
	f := newFixture(t)
	f.c.Heartbeat, f.c.TaskPoll, f.c.PRPoll = "30s", "500ms", "500ms"
	s := f.service(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runTestService(ctx, s, "manual") }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("stop idle service: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("idle service did not stop")
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		state, err := loadFixtureRuntime(f.c)
		if err == nil && state != nil && state.Service.Ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("idle service did not become ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Let the final startup dispatch checkpoint settle before measuring idle time.
	time.Sleep(200 * time.Millisecond)
	before, err := loadFixtureRuntime(f.c)
	require(t, err)
	started := time.Now()
	time.Sleep(2200 * time.Millisecond)
	after, err := loadFixtureRuntime(f.c)
	require(t, err)
	elapsed := time.Since(started)
	allowed := uint64(2) // No-op task, decision, PR and control polls do not publish runtime revisions.
	if delta := after.Revision - before.Revision; delta > allowed {
		t.Fatalf("idle polls published %d full runtime revisions in %s", delta, elapsed)
	}
}

func (f *fixture) pr(number, state string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	db := readFake(f.db)
	db.PRState = state
	db.PRMerged = state == "merged"
	if db.PRMerged {
		db.PRMergedAt = ptr(now())
		db.PRMergeCommit = strings.Repeat("a", 40)
	}
	writeFake(f.db, db)
	_ = number
}

func readFake(path string) fakeDB {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var db fakeDB
	if err = json.Unmarshal(data, &db); err != nil {
		panic(err)
	}
	return db
}

func writeFake(path string, db fakeDB) {
	data, err := json.Marshal(db)
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		panic(err)
	}
}

func arg(args []string, key string) string {
	for i, value := range args {
		if value == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func fakeMain(args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "publication-git":
		if len(args) > 3 && args[2] == "remote" && args[3] == "get-url" {
			fmt.Println("https://github.com/owner/repo.git")
			return 0
		}
		cmd := exec.Command(args[1], args[2:]...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				return e.ExitCode()
			}
			return 1
		}
		return 0
	case "git-proxy":
		gitArgs := append([]string(nil), args[2:]...)
		if len(gitArgs) > 0 && gitArgs[0] == "fetch" {
			if remote := os.Getenv("STEERLINE_FAKE_GIT_FETCH_REMOTE"); remote != "" {
				for i := 1; i < len(gitArgs); i++ {
					if gitArgs[i] == "origin" {
						gitArgs[i] = remote
						break
					}
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
	case "publish-control":
		if len(args) != 3 {
			return 2
		}
		request, err := readControlRequest(args[2])
		if err == nil {
			err = publishControlRequest(args[1], request)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	case "pr-query":
		db := readFake(args[1])
		state := db.PRState
		if state == "" {
			state = "open"
		}
		info := executionmodel.PRInfo{URL: args[2], Repository: args[3], BaseBranch: "main", HeadRevision: db.PRHeadRevision, State: state, Merged: db.PRMerged, MergedAt: db.PRMergedAt}
		if db.PRMergeCommit != "" {
			info.MergeCommit = ptr(db.PRMergeCommit)
		}
		_ = json.NewEncoder(os.Stdout).Encode(info)
		return 0
	case "pr-close":
		db := readFake(args[1])
		db.PRState, db.PRMerged = "closed_unmerged", false
		db.PRMergedAt, db.PRMergeCommit = nil, ""
		writeFake(args[1], db)
		info := executionmodel.PRInfo{URL: args[2], Repository: args[3], BaseBranch: "main", HeadRevision: db.PRHeadRevision, State: "closed_unmerged"}
		_ = json.NewEncoder(os.Stdout).Encode(info)
		return 0
	case "sleep":
		time.Sleep(time.Hour)
		return 0
	case "spawn-owned-child":
		if len(args) != 2 {
			return 2
		}
		child := exec.Command(os.Args[0], "--fake", "sleep")
		if err := child.Start(); err != nil {
			return 1
		}
		if err := os.WriteFile(args[1], []byte(fmt.Sprint(child.Process.Pid)), 0600); err != nil {
			_ = child.Process.Kill()
			return 1
		}
		time.Sleep(time.Hour)
		return 0
	}
	return 2
}

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := append([]string{"-C", dir}, args...)
	output, err := exec.Command("git", command...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func seedFakeCapabilities(t *testing.T, c Config) {
	t.Helper()
	if err := os.MkdirAll(c.RunDir, 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte("fake platform fixture: " + t.Name())
	sum := sha256.Sum256(data)
	path := filepath.Join(c.RunDir, "execution", "v1", "evidence", hex.EncodeToString(sum[:])+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := fixtureAtomicWrite(path, data); err != nil {
		t.Fatal(err)
	}
	evidence := path + "#sha256=" + hex.EncodeToString(sum[:])
	backend, protocol := BackendFor(c).Identity()
	version := "fixture-version-1"
	report := capabilityReport(backend, protocol, TargetFor(c), AgentFor(c), version)
	for i := range report.Capabilities {
		name := report.Capabilities[i].Name
		if name == "output_turns" || name == "output_final_answer" {
			continue
		}
		if name == "resource_settlement" {
			report.Capabilities[i].Status = executionbackend.CapabilityUnsupported
			continue
		}
		report.Capabilities[i].Status = executionbackend.CapabilityVerified
		report.Capabilities[i].Evidence = evidence
	}
	if err := (executionRecords{root: c.RunDir, resources: fixtureResources{}}).saveCapabilityReport(report); err != nil {
		t.Fatal(err)
	}
}
