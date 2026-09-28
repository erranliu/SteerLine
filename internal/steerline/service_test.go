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
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDB is the isolated PR-provider state used by native workflow tests.
type fakeDB struct {
	PRHeadRevision string
	PRState        string
	PRMerged       bool
	PRMergedAt     *time.Time
	PRMergeCommit  string
}

type fixture struct {
	c      Config
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
	code := m.Run()
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
	f.c.Git = Command{Executable: executable, Args: []string{"--fake", "git-proxy", git}}
	if output, err := exec.Command("git", "-C", f.c.Repository, "remote", "set-url", "origin", f.server.URL+"/owner/repo.git").CombinedOutput(); err != nil {
		t.Fatalf("set native fixture remote: %v: %s", err, output)
	}
	f.c.RuntimeFile = "runtime.json"
	f.c.Claimant = "native-fixture"
	f.c.Heartbeat, f.c.TaskPoll, f.c.PRPoll, f.c.CommandTimeout, f.c.ShutdownGrace = "100ms", "100ms", "100ms", "5s", "1s"
	f.c.PR = PRConfig{Kind: "gitea", BaseURL: f.server.URL, Repository: "owner/repo"}
	f.c.Codex = Command{Executable: executable, Args: []string{"--fake", "codex"}}
	f.c.WorkflowDefinition = &WorkflowDefinition{ID: MinimalWorkflowVersion, Version: MinimalWorkflowVersion, ReviewRequired: false, MaxRepairRounds: 1}
	step := func(name string) StepContract {
		return StepContract{Provider: "command", Entry: "builtin:" + name, Version: "fixture-v1", InputSchema: NativeTaskInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "local", Timeout: 5 * time.Second, Command: []string{git, "status", "--porcelain"}}
	}
	f.c.ProjectSteps = &ProjectSteps{Implement: step("implement"), Verify: step("verify"), PublishPR: step("publish_pr")}
	f.c.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	if err = f.c.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) service(t *testing.T) *Service {
	t.Helper()
	store, err := OpenNativeTaskProject(context.Background(), &f.c)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(f.c)
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
	go func() { done <- s.RunWithDispatch(ctx, "manual") }()
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
		state, err := LoadDatabaseRuntime(f.c)
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
	before, err := LoadDatabaseRuntime(f.c)
	require(t, err)
	started := time.Now()
	time.Sleep(2200 * time.Millisecond)
	after, err := LoadDatabaseRuntime(f.c)
	require(t, err)
	elapsed := time.Since(started)
	allowed := uint64(2) // No-op task, decision, PR and control polls do not publish runtime revisions.
	if delta := after.Revision - before.Revision; delta > allowed {
		t.Fatalf("idle polls published %d full runtime revisions in %s", delta, elapsed)
	}
}

func controlReviewService(t *testing.T) *Service {
	t.Helper()
	service := newFixture(t).service(t)
	service.Config.configDigest = "lifecycle-review-config"
	service.State.Service.Mode = "scheduler"
	service.State.Service.Status = "running"
	service.State.Service.Ready = true
	identity, err := currentProcessIdentity()
	if err != nil {
		t.Fatal(err)
	}
	startupID := newID()
	if _, _, err := createStartupAttempt(service.Config, startupID, true); err != nil {
		t.Fatal(err)
	}
	record := initialStartupRecord(service.Config, startupID)
	record.Status = "started"
	record.InstanceID = service.State.Service.InstanceID
	record.ProcessIdentity = identity
	if err := writeStartupRecord(service.Config, record); err != nil {
		t.Fatal(err)
	}
	service.State.Service.StartupID = &startupID
	service.State.Service.PID = identity.PID
	service.State.Service.ProcessIdentity = identity
	service.State.Service.ConfigDigest = service.Config.configDigest
	if err := service.save(); err != nil {
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
	case "orca":
		return fakeOrca(args[1], args[2:])
	case "orca-supervised":
		return fakeSupervisedOrca(args[1:])
	case "native-step":
		return fakeNativeMinimalStep()
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
		info := PRInfo{URL: args[2], Repository: args[3], BaseBranch: "main", HeadRevision: db.PRHeadRevision, State: state, Merged: db.PRMerged, MergedAt: db.PRMergedAt}
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
		info := PRInfo{URL: args[2], Repository: args[3], BaseBranch: "main", HeadRevision: db.PRHeadRevision, State: "closed_unmerged"}
		_ = json.NewEncoder(os.Stdout).Encode(info)
		return 0
	case "codex":
		if len(args) > 1 && args[1] == "--version" {
			fmt.Println("fake-codex/1")
			return 0
		}
	case "sleep":
		time.Sleep(time.Hour)
		return 0
	}
	return 2
}

func fakeNativeMinimalStep() int {
	if _, ok := os.LookupEnv("BEADS_DIR"); ok {
		fmt.Fprintln(os.Stderr, "native worker inherited BEADS_DIR")
		return 31
	}
	if _, ok := os.LookupEnv("BEADS_ACTOR"); ok {
		fmt.Fprintln(os.Stderr, "native worker inherited BEADS_ACTOR")
		return 32
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 33
	}
	var input NativeTaskWorkerContext
	if err = json.Unmarshal(raw, &input); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 33
	}
	if input.SchemaVersion != NativeTaskInputSchema || input.ProjectID == "" || input.TaskID == "" || input.Contract.Digest != input.ContractDigest || input.CurrentStep == "" || input.StepInput.TaskID != input.TaskID {
		fmt.Fprintln(os.Stderr, "native-task/v1 context is incomplete")
		return 34
	}
	if os.Getenv("STEERLINE_FAKE_NATIVE_STEP_FAIL") == input.CurrentStep {
		marker := os.Getenv("STEERLINE_FAKE_NATIVE_STEP_FAIL_MARKER")
		if marker == "" {
			return 38
		}
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			if err = os.WriteFile(marker, []byte(input.AttemptID), 0600); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 39
			}
			return 38
		}
	}
	result := StepGateResult{StepID: input.CurrentStep, Succeeded: true, InputRevision: input.StepInput.InputRevision, Workspace: input.StepInput.Workspace, ResultID: newID(), PrerequisiteIDs: append([]string(nil), input.StepInput.PrerequisiteResultIDs...)}
	if os.Getenv("STEERLINE_FAKE_NATIVE_STEP_DECISION") == input.CurrentStep && input.StepInput.Decision == nil {
		result.Decision = &DecisionRequest{Question: "Choose the implementation path", Options: []DecisionOption{{ID: "keep", Label: "Keep current plan"}}}
	}
	if decision := input.StepInput.Decision; decision != nil {
		result.AppliedDecisionID = decision.Answer.DecisionID
		result.AppliedDecisionAttemptID = decision.AttemptID
		result.AppliedDecisionExecutionID = decision.ExecutionID
		result.AppliedAnswerSHA256 = decision.AnswerSHA256
	}
	switch input.CurrentStep {
	case "implement":
		result.Revision = input.StepInput.Workspace.Head
	case "verify":
		result.Revision = input.StepInput.CandidateRevision
		for _, name := range input.StepInput.ProjectSteps.Verify.RequiredChecks {
			result.Checks = append(result.Checks, CheckEvidence{Name: name, Status: "passed", Revision: result.Revision})
		}
	case "publish_pr":
		result.Revision = input.StepInput.CandidateRevision
		result.PublishedHead = result.Revision
		result.PRURL = "https://forge.invalid/org/repo/pulls/" + input.TaskID
	default:
		fmt.Fprintln(os.Stderr, "unsupported native test step", input.CurrentStep)
		return 35
	}
	envelope := MinimalStepResultEnvelope{SchemaVersion: 1, ProjectIdentity: input.ProjectID, TaskID: input.TaskID, WorkflowRunID: input.WorkflowRunID, ExecutionID: input.ExecutionID, AttemptID: input.AttemptID, Result: result}
	if err := json.NewEncoder(os.Stdout).Encode(envelope); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 36
	}
	return 0
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
	if err := AtomicWrite(path, data); err != nil {
		t.Fatal(err)
	}
	evidence := path + "#sha256=" + hex.EncodeToString(sum[:])
	backend, protocol := backendFor(c).Identity()
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
		if name == "resource_settlement" {
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
