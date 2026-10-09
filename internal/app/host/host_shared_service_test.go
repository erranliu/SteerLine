package host

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/testkit/fixtures"

	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	execution "github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

type fakeDB = fixtures.PRState

type fixture struct {
	c      execution.Config
	db     string
	commit string
	server *httptest.Server
	mu     sync.Mutex
}

func TestMain(m *testing.M) {
	if os.Getenv("STEERLINE_LIFECYCLE_EXIT_EARLY") == "1" {
		os.Exit(23)
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
	f.c.CommandTimeout = "5s"
	f.c.PR = executionmodel.PRProviderConfig{Kind: "gitea", BaseURL: f.server.URL, Repository: "owner/repo"}
	setNeutralProfile(t, &f.c, neutralSettings{Executable: executable})
	f.c.WorkflowProtocol = executionmodel.Version
	return f
}

func (f *fixture) service(t *testing.T) *execution.Service {
	t.Helper()
	store, err := openHostFixtureNativeProject(context.Background(), &f.c)
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

var readFake = fixtures.ReadPRState

var writeFake = fixtures.WritePRState

func arg(args []string, key string) string {
	for i, value := range args {
		if value == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func fakeMain(args []string) int {
	return fixtures.FakeMain(args, func(dir, path string) error {
		request, err := readControlRequest(path)
		if err != nil {
			return err
		}
		return publishControlRequest(dir, request)
	})
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
