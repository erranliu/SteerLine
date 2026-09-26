package steerline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func nativeTaskFixture(t *testing.T) (Config, string) {
	t.Helper()
	t.Setenv("STEERLINE_HOME", t.TempDir())
	repo := filepath.Join(t.TempDir(), "repo")
	remote := filepath.Join(t.TempDir(), "origin.git")
	for _, args := range [][]string{{"init", "--quiet", "-b", "main", repo}, {"init", "--quiet", "--bare", remote}, {"-C", repo, "remote", "add", "origin", remote}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	for name, content := range map[string]string{
		"docs/spec/task-core.spec.md":          "# Task Spec\n\n## Acceptance\n\nPass.\n",
		"docs/spec/task-core.plan.md":          "# Task Plan\n",
		"docs/spec/native-task-constraints.md": "# Shared constraints\n",
	} {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"-C", repo, "add", "."}, {"-C", repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	commitBytes, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(commitBytes))
	c := Config{TaskAuthority: "native", Repository: repo, Remote: "origin", Branch: "main", Git: Command{Executable: "git"}, CommandTimeout: "10s"}
	key, err := repoStateKey(c)
	if err != nil {
		t.Fatal(err)
	}
	c.RunDir = filepath.Join(SteerlineStateRoot(), key)
	return c, commit
}

func nativeRef(t *testing.T, c Config, commit, name string) FileReference {
	t.Helper()
	blob, err := exec.Command("git", "-C", c.Repository, "cat-file", "blob", commit+":"+name).Output()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(blob)
	return FileReference{Path: name, Commit: commit, SHA256: hex.EncodeToString(sum[:])}
}

func nativeContract(t *testing.T, c Config, commit, id string) TaskContract {
	t.Helper()
	spec := nativeRef(t, c, commit, "docs/spec/task-core.spec.md")
	plan := nativeRef(t, c, commit, "docs/spec/task-core.plan.md")
	common := nativeRef(t, c, commit, "docs/spec/native-task-constraints.md")
	target, err := configuredRepositoryTarget(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	v := TaskContract{ContractID: id, Repository: target, Remote: c.Remote, Branch: c.Branch, SpecRef: spec, PlanRef: plan, CommonRefs: []FileReference{common}, AcceptanceRefs: []FileReference{spec}}
	v.Digest, err = TaskContractDigest(v)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNativePreflightDoesNotRequireBeadsAndRejectsLegacyStepEntrypoints(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	c, _ := nativeTaskFixture(t)
	if out, err := exec.Command("git", "-C", c.Repository, "remote", "set-url", "origin", "https://example.com/owner/repo.git").CombinedOutput(); err != nil {
		t.Fatalf("set fixture remote: %v: %s", err, out)
	}
	steps := map[string]string{
		"implement":  "package main\nfunc main() {}\n",
		"verify":     "package main\nfunc main() {}\n",
		"publish_pr": "package main\nfunc main() {}\n",
	}
	for name, content := range steps {
		path := filepath.Join(c.Repository, "steps", name+".go")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "-C", c.Repository, "add", "steps").CombinedOutput(); err != nil {
		t.Fatalf("stage fixture steps: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "native project steps").CombinedOutput(); err != nil {
		t.Fatalf("commit fixture steps: %v: %s", err, out)
	}
	c.BD = Command{Executable: "bd-and-bv-are-not-installed"}
	c.PR = PRConfig{Kind: "command", BaseURL: "https://example.com", Repository: "owner/repo", Query: Command{Executable: "git"}, Close: Command{Executable: "git"}}
	c.WorkflowDefinition = &WorkflowDefinition{ID: MinimalWorkflowVersion, Version: MinimalWorkflowVersion, ReviewRequired: false, MaxRepairRounds: 1}
	contract := func(name string) StepContract {
		return StepContract{Provider: "command", Entry: "steps/" + name + ".go", Version: "v1", InputSchema: NativeTaskInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "local", Timeout: time.Minute, Command: []string{"git"}}
	}
	c.ProjectSteps = &ProjectSteps{
		Implement: contract("implement"),
		Verify:    contract("verify"),
		PublishPR: contract("publish_pr"),
	}
	c.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	if err := c.Preflight(context.Background(), true); err != nil {
		t.Fatalf("native preflight required Beads or rejected valid project steps: %v", err)
	}
	legacy := []byte("#!/bin/sh\nbd ready --json\n")
	if err := os.WriteFile(filepath.Join(c.Repository, "steps", "implement.go"), legacy, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", c.Repository, "add", "steps/implement.go").CombinedOutput(); err != nil {
		t.Fatalf("stage legacy fixture step: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "legacy project step").CombinedOutput(); err != nil {
		t.Fatalf("commit legacy fixture step: %v: %s", err, out)
	}
	if err := c.Preflight(context.Background(), true); err == nil || !strings.Contains(err.Error(), "legacy Beads") {
		t.Fatalf("native preflight accepted a Beads-dependent project step: %v", err)
	}
}

func TestNativeOrcaProjectStepsUseFixedTaskContextAndCenterDelivery(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("fake Orca target step launches use Windows PowerShell")
	}
	t.Setenv("STEERLINE_HOME", t.TempDir())
	c, _ := nativeTaskFixture(t)
	localRemote, err := exec.Command("git", "-C", c.Repository, "remote", "get-url", "origin").Output()
	if err != nil {
		t.Fatal(err)
	}
	remotePath := strings.TrimSpace(string(localRemote))
	for name, content := range map[string]string{"implement": "package steps\n", "verify": "package steps\n", "publish_pr": "package steps\n"} {
		path := filepath.Join(c.Repository, "steps", name+".go")
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"-C", c.Repository, "add", "steps"}, {"-C", c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "native Orca project steps"}} {
		if out, runErr := exec.Command("git", args...).CombinedOutput(); runErr != nil {
			t.Fatalf("git %v: %v: %s", args, runErr, out)
		}
	}
	if out, runErr := exec.Command("git", "-C", c.Repository, "push", remotePath, "main").CombinedOutput(); runErr != nil {
		t.Fatalf("push native Orca base: %v: %s", runErr, out)
	}
	if out, runErr := exec.Command("git", "-C", c.Repository, "remote", "set-url", "origin", "https://forge.invalid/org/repo.git").CombinedOutput(); runErr != nil {
		t.Fatalf("configure native Orca PR remote: %v: %s", runErr, out)
	}
	commitBytes, err := exec.Command("git", "-C", c.Repository, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(commitBytes))
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEERLINE_FAKE_GIT_FETCH_REMOTE", remotePath)
	c.Git = Command{Executable: executable, Args: []string{"--fake", "git-proxy", gitExecutable}}
	c.BD = Command{Executable: "bd-and-bv-are-not-installed"}
	c.Host, c.Agent = "orca", "codex"
	c.Orca = OrcaConfig{Executable: executable, Args: []string{"--fake", "orca", filepath.Join(t.TempDir(), "orca-state")}, Shell: "cmd.exe", WaitTimeout: "10s"}
	c.PR = PRConfig{Kind: "command", BaseURL: "https://forge.invalid", Repository: "org/repo", Query: Command{Executable: executable, Args: []string{"--fake", "pr-query", filepath.Join(t.TempDir(), "native-orca-pr.json"), "{pr_url}", "{repository}"}}, Close: Command{Executable: executable, Args: []string{"--fake", "pr-close", filepath.Join(t.TempDir(), "native-orca-pr.json"), "{pr_url}", "{repository}"}}}
	prPath := c.PR.Query.Args[2]
	c.PR.Query.Args[2], c.PR.Close.Args[2] = prPath, prPath
	if err = c.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.Claimant = "native-orca-test"
	c.configDigest = strings.Repeat("e", 64)
	c.ExecutableTypes = []string{"task", "bug", "feature", "chore"}
	c.CommandTimeout, c.ShutdownGrace = "10s", "1s"
	c.WorkflowDefinition = &WorkflowDefinition{ID: MinimalWorkflowVersion, Version: MinimalWorkflowVersion, ReviewRequired: false, MaxRepairRounds: 1}
	step := func(name string) StepContract {
		return StepContract{Provider: "command", Entry: "steps/" + name + ".go", Version: "v1", InputSchema: NativeTaskInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "target", Timeout: time.Minute, Command: []string{executable, "--fake", "native-step"}}
	}
	c.ProjectSteps = &ProjectSteps{Implement: step("implement"), Verify: step("verify"), PublishPR: step("publish_pr")}
	c.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	if _, err = c.validateNativeWorkflow(); err != nil {
		t.Fatalf("native Orca workflow did not accept target-side project steps: %v", err)
	}
	supervised := c
	supervised.Host = "orca_supervised"
	supervised.Orca.Shell = "powershell"
	if _, err = supervised.validateNativeWorkflow(); err != nil {
		t.Fatalf("native supervised workflow did not accept target-side project steps: %v", err)
	}
	withEnvironment := *supervised.ProjectSteps
	withEnvironment.Implement.Environment = map[string]string{"NATIVE_STEP_MODE": "test"}
	supervised.ProjectSteps = &withEnvironment
	if _, err = supervised.validateNativeWorkflow(); err == nil || !strings.Contains(err.Error(), "cannot pin environment values") {
		t.Fatalf("native supervised workflow accepted unverified remote environment injection: %v", err)
	}
	localSite := *c.ProjectSteps
	localSite.Implement.ExecutionSite = "local"
	invalid := c
	invalid.ProjectSteps = &localSite
	if _, err = invalid.validateNativeWorkflow(); err == nil || !strings.Contains(err.Error(), "execution_site=target") {
		t.Fatalf("native Orca accepted a local-only project step: %v", err)
	}
	writeFake(prPath, fakeDB{PRHeadRevision: commit})
	reader, err := OpenNativeTaskProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	lock, err := LockRepository(c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if lock != nil {
			lock.Close()
		}
	}()
	store, err := OpenNativeTaskCommandStore(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if store != nil {
			_ = store.Close()
		}
	}()
	commands := CommandService{Store: store, Project: c.Identity()}
	contract := nativeContract(t, c, commit, "contract-orca-201")
	contractB := nativeContract(t, c, commit, "contract-orca-202")
	batchBytes, err := json.Marshal(TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: c.Identity(), Tasks: []TaskBatchTask{{LocalKey: "orca", TaskID: 201, Title: "Native Orca A", Type: "task", Contract: contract}, {LocalKey: "dependent", TaskID: 202, Title: "Native Orca B", Type: "task", Contract: contractB}}, Dependencies: []TaskBatchEdge{{Predecessor: TaskBatchReference{LocalKey: "orca"}, Successor: TaskBatchReference{LocalKey: "dependent"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = commands.CreateTaskBatch(context.Background(), c, batchBytes, strings.Repeat("1", 32), 1, "planner"); err != nil {
		t.Fatal(err)
	}
	authority, err := store.nativeAuthority(context.Background(), c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	auto, unpaused := "auto", false
	if _, err = commands.SetNativeDispatchState(context.Background(), NativeDispatchUpdate{RequestID: strings.Repeat("2", 32), ExpectedAuthorityVersion: authority.AuthorityVersion, Mode: &auto, Paused: &unpaused, RequestedBy: "operator"}); err != nil {
		t.Fatal(err)
	}
	proof := []byte("fake Orca native terminal protocol acceptance")
	proofHash := sha256.Sum256(proof)
	proofPath := filepath.Join(c.RunDir, "execution", "v1", "evidence", hex.EncodeToString(proofHash[:])+".json")
	if err = os.MkdirAll(filepath.Dir(proofPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err = AtomicWrite(proofPath, proof); err != nil {
		t.Fatal(err)
	}
	report, err := (orcaTerminalBackend{config: c}).Probe(context.Background(), targetFor(c), agentFor(c))
	if err != nil {
		t.Fatal(err)
	}
	for i := range report.Capabilities {
		switch report.Capabilities[i].Name {
		case "start", "observe", "stop", "workspace_inspection", "output_reading":
			report.Capabilities[i].Status = CapabilityVerified
			report.Capabilities[i].Evidence = proofPath + "#sha256=" + hex.EncodeToString(proofHash[:])
		}
	}
	if err = (executionRecords{root: c.RunDir}).saveCapabilityReport(report); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = nil
	lock.Close()
	lock = nil
	s, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			_ = s.Close()
		}
	}()
	if err = s.dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	claimed := s.find("201")
	claimedTask := nativeTaskRecord(s.State.Tasks, "201")
	claimedWorkflow, claimedExecution := s.minimalRecords(claimed)
	if claimed == nil || claimedTask == nil || claimedWorkflow == nil || claimedExecution == nil {
		t.Fatal("native claim did not retain its fixed task and workflow context")
	}
	remoteConfig := s.Config
	remoteConfig.Host = "orca_supervised"
	remoteCenter := &Service{Config: remoteConfig, native: true, domainStore: s.domainStore, State: s.State}
	remoteRun := cloneRun(*claimed)
	remoteRun.WorktreePath, remoteRun.WorktreeBranch = nil, nil
	remoteRun.Target.WorktreeID = ""
	remoteExecution := *claimedExecution
	remoteExecution.Target.WorktreeID = ""
	remoteExecution.Workspace.Base = commit
	remotePrepare := StepAttemptRecord{AttemptID: "native-remote-prepare", ExecutionID: remoteExecution.ExecutionID, StepID: "prepare", InputRevision: commit}
	remoteTask := nativeTaskModel(*claimedTask)
	remoteInput := buildMinimalStepInput(c.Identity(), remoteTask, &remoteRun, claimedWorkflow, &remoteExecution, &remotePrepare)
	remoteContext, contextErr := remoteCenter.minimalWorkerInput(context.Background(), remoteTask, &remoteRun, claimedWorkflow, &remoteExecution, &remotePrepare, remoteInput)
	if contextErr != nil {
		t.Fatalf("native supervised prepare rejected the fixed task context before Orca returned a worktree: %v", contextErr)
	}
	if contextValue, ok := remoteContext.(NativeTaskWorkerContext); !ok || contextValue.TaskID != "201" || contextValue.Contract.ContractID != contract.ContractID || contextValue.BaseCommit != commit || contextValue.CurrentStep != "prepare" {
		t.Fatalf("native supervised prepare lost its fixed task contract: %+v", remoteContext)
	}
	for range 3 {
		if s.worker == nil {
			t.Fatal("native Orca project step did not start")
		}
		worker := s.worker
		if err = s.finished(<-worker.Done); err != nil {
			t.Fatal(err)
		}
		if s.find("201").FinishTaskStatus == "waiting_pr_merge" {
			break
		}
		if err = s.resumeMinimalSteps(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if run := s.find("201"); run == nil || run.FinishTaskStatus != "waiting_pr_merge" || run.PR.URL == nil {
		t.Fatalf("native Orca did not reach the center merge gate: %+v", run)
	} else {
		attempt := s.minimalAttempt(run, run.AttemptID)
		binding, bindErr := (executionRecords{root: c.RunDir}).loadBinding(run.RunID, attempt.AttemptID)
		if bindErr != nil || binding.Backend != BackendOrca || binding.Protocol != ProtocolOrcaTerminalV2 || binding.External.Terminal == "" {
			t.Fatalf("native Orca terminal identity was not bound to the fixed step: %+v %v", binding, bindErr)
		}
	}
	db := readFake(prPath)
	db.PRState, db.PRMerged, db.PRMergedAt, db.PRMergeCommit = "merged", true, ptr(time.Now().UTC()), commit
	writeFake(prPath, db)
	if err = s.pollPRs(context.Background()); err != nil {
		t.Fatal(err)
	}
	page, err := (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "201")
	if err != nil || len(page.Tasks) != 1 || page.Tasks[0].Status != "closed" || page.Tasks[0].Delivery == nil {
		t.Fatalf("native Orca merge did not produce center TaskDelivery: %+v %v", page, err)
	}
	blocked, err := (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "202")
	if err != nil || len(blocked.Tasks) != 1 || !blocked.Tasks[0].DependencyReady {
		t.Fatalf("native Orca delivery did not release the dependent task: %+v %v", blocked, err)
	}
	t.Setenv("STEERLINE_FAKE_ORCA_TERMINAL_CREATE_RESPONSE_LOST", "1")
	if err = s.dispatch(context.Background()); err == nil {
		t.Fatal("lost Orca native step-start receipt was treated as a rejected start")
	}
	runB := s.find("202")
	if runB == nil || runB.ExecutionStatus != "unknown" || !s.State.SlotOccupied || runB.Process != nil {
		t.Fatalf("lost Orca step-start receipt did not preserve the unknown fixed attempt: %+v", runB)
	}
	unknownAttempt := s.minimalAttempt(runB, runB.AttemptID)
	if unknownAttempt == nil || unknownAttempt.Status != "outcome_unknown" {
		t.Fatalf("lost Orca step-start receipt did not retain the exact attempt state: %+v", unknownAttempt)
	}
	projection, _, projectionErr := s.domainStore.LoadProjection(context.Background(), c.Identity())
	if projectionErr != nil {
		t.Fatal(projectionErr)
	}
	var savedRuntime Runtime
	if err = json.Unmarshal(projection, &savedRuntime); err != nil {
		t.Fatal(err)
	}
	for _, savedRun := range savedRuntime.Runs {
		if savedRun.TaskID == "202" && savedRun.AttemptID == "" {
			var step string
			for _, savedExecution := range savedRuntime.Executions {
				if savedExecution.ExecutionID == savedRun.ExecutionID {
					step = savedExecution.CurrentStep
				}
			}
			t.Fatalf("persisted unknown native run lost its attempt ID before restart: run=%+v current_step=%q attempts=%+v", savedRun, step, savedRuntime.Attempts)
		}
	}
	stateEntries, err := os.ReadDir(c.Orca.Args[2])
	if err != nil {
		t.Fatal(err)
	}
	terminalCount := len(stateEntries)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = nil
	s, err = NewService(c)
	if err != nil {
		t.Fatalf("native Orca service did not recover the unknown step-start state: %v", err)
	}
	if s.worker != nil || !s.State.SlotOccupied || s.find("202") == nil || s.find("202").ExecutionStatus != "unknown" {
		t.Fatalf("native Orca restart changed the unknown worker identity or released its slot: %+v", s.State.Runs)
	}
	if err = s.dispatch(context.Background()); err != nil || s.worker != nil {
		t.Fatalf("native Orca restart attempted a replacement worker: worker=%v err=%v", s.worker, err)
	}
	stateEntries, err = os.ReadDir(c.Orca.Args[2])
	if err != nil || len(stateEntries) != terminalCount {
		t.Fatalf("native Orca restart created a duplicate terminal: before=%d after=%d err=%v", terminalCount, len(stateEntries), err)
	}
}

func TestNativeLocalProjectStepsDeliverTaskAndReleaseSuccessor(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	t.Setenv("BEADS_DIR", filepath.Join(t.TempDir(), "old-beads"))
	t.Setenv("BEADS_ACTOR", "legacy-agent")
	c, _ := nativeTaskFixture(t)
	remoteURL, err := exec.Command("git", "-C", c.Repository, "remote", "get-url", "origin").Output()
	if err != nil {
		t.Fatal(err)
	}
	remotePath := strings.TrimSpace(string(remoteURL))
	for _, args := range [][]string{
		{"-C", c.Repository, "remote", "set-url", "origin", "https://forge.invalid/org/repo.git"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	for name, content := range map[string]string{"implement": "package steps\n", "verify": "package steps\n", "publish_pr": "package steps\n"} {
		path := filepath.Join(c.Repository, "steps", name+".go")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "-C", c.Repository, "add", "steps").CombinedOutput(); err != nil {
		t.Fatalf("stage project steps: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "native workflow steps").CombinedOutput(); err != nil {
		t.Fatalf("commit project steps: %v: %s", err, out)
	}
	pushOut, pushErr := exec.Command("git", "-C", c.Repository, "push", remotePath, "main").CombinedOutput()
	if pushErr != nil {
		t.Fatalf("push fixture base: %v: %s", pushErr, pushOut)
	}
	commitBytes, err := exec.Command("git", "-C", c.Repository, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(commitBytes))
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEERLINE_FAKE_GIT_FETCH_REMOTE", remotePath)
	c.Git = Command{Executable: executable, Args: []string{"--fake", "git-proxy", gitExecutable}}
	c.Codex = Command{Executable: executable, Args: []string{"--fake", "codex", "unused"}}
	fakePRPath := filepath.Join(t.TempDir(), "native-pr.json")
	c.PR = PRConfig{Kind: "command", BaseURL: "https://forge.invalid", Repository: "org/repo", Query: Command{Executable: executable, Args: []string{"--fake", "pr-query", fakePRPath, "{pr_url}", "{repository}"}}, Close: Command{Executable: executable, Args: []string{"--fake", "pr-close", fakePRPath, "{pr_url}", "{repository}"}}}
	if err := c.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.Claimant = "native-test"
	c.configDigest = strings.Repeat("d", 64)
	c.ExecutableTypes = []string{"task", "bug", "feature", "chore"}
	c.Host, c.Agent = "local", "codex"
	c.BD = Command{Executable: "bd-and-bv-are-not-installed"}
	c.CommandTimeout, c.ShutdownGrace = "10s", "1s"
	c.WorkflowDefinition = &WorkflowDefinition{ID: MinimalWorkflowVersion, Version: MinimalWorkflowVersion, ReviewRequired: false, MaxRepairRounds: 1}
	step := func(name string) StepContract {
		return StepContract{Provider: "command", Entry: "steps/" + name + ".go", Version: "v1", InputSchema: NativeTaskInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "local", Timeout: 20 * time.Second, Command: []string{executable, "--fake", "native-step"}}
	}
	c.ProjectSteps = &ProjectSteps{Implement: step("implement"), Verify: step("verify"), PublishPR: step("publish_pr")}
	c.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	writeFake(fakePRPath, fakeDB{PRHeadRevision: commit})
	reader, err := OpenNativeTaskProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	lock, err := LockRepository(c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenNativeTaskCommandStore(context.Background(), &c)
	if err != nil {
		lock.Close()
		t.Fatal(err)
	}
	contractA := nativeContract(t, c, commit, "contract-101")
	contractB := nativeContract(t, c, commit, "contract-102")
	contractC := nativeContract(t, c, commit, "contract-103")
	batchBytes, err := json.Marshal(TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: c.Identity(), Tasks: []TaskBatchTask{{LocalKey: "a", TaskID: 101, Title: "Deliver A", Type: "task", Contract: contractA}, {LocalKey: "b", TaskID: 102, Title: "Wait for A", Type: "task", Contract: contractB}, {LocalKey: "c", TaskID: 103, Title: "Active discard probe", Type: "task", Contract: contractC}}, Dependencies: []TaskBatchEdge{{Predecessor: TaskBatchReference{LocalKey: "a"}, Successor: TaskBatchReference{LocalKey: "b"}}}})
	if err != nil {
		t.Fatal(err)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
	if _, err = commands.CreateTaskBatch(context.Background(), c, batchBytes, strings.Repeat("a", 32), 1, "planner"); err != nil {
		store.Close()
		lock.Close()
		t.Fatal(err)
	}
	authority, err := store.nativeAuthority(context.Background(), c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	auto, unpaused, by := "auto", false, "operator"
	update := NativeDispatchUpdate{RequestID: strings.Repeat("b", 32), ExpectedAuthorityVersion: authority.AuthorityVersion, Mode: &auto, Paused: &unpaused, RequestedBy: by}
	updated, updateErr := commands.SetNativeDispatchState(context.Background(), update)
	if updateErr != nil {
		store.Close()
		lock.Close()
		t.Fatal(updateErr)
	}
	replayed, replayErr := commands.SetNativeDispatchState(context.Background(), update)
	if replayErr != nil || replayed.AuthorityVersion != updated.AuthorityVersion || replayed.DispatchMode != "auto" || replayed.DispatchPaused {
		store.Close()
		lock.Close()
		t.Fatalf("native dispatch control did not replay its center receipt: %+v %v", replayed, replayErr)
	}
	conflict := update
	manual := "manual"
	conflict.Mode = &manual
	if _, conflictErr := commands.SetNativeDispatchState(context.Background(), conflict); conflictErr == nil || !strings.Contains(conflictErr.Error(), "idempotency_conflict") {
		store.Close()
		lock.Close()
		t.Fatalf("native dispatch request ID was reused with different content: %v", conflictErr)
	}
	proof := []byte("isolated native local worker capability evidence")
	proofHash := sha256.Sum256(proof)
	proofPath := filepath.Join(c.RunDir, "execution", "v1", "evidence", hex.EncodeToString(proofHash[:])+".json")
	if err = os.MkdirAll(filepath.Dir(proofPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err = AtomicWrite(proofPath, proof); err != nil {
		t.Fatal(err)
	}
	backend := backendFor(c, BeadsContext{})
	report, err := backend.Probe(context.Background(), targetFor(c), agentFor(c))
	if err != nil {
		t.Fatal(err)
	}
	for i := range report.Capabilities {
		switch report.Capabilities[i].Name {
		case "start", "observe", "stop", "workspace_inspection", "output_reading":
			report.Capabilities[i].Status = CapabilityVerified
			report.Capabilities[i].Evidence = proofPath + "#sha256=" + hex.EncodeToString(proofHash[:])
		}
	}
	if err = (executionRecords{root: c.RunDir}).saveCapabilityReport(report); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	lock.Close()
	s, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			_ = s.Close()
		}
	}()
	retryMarker := filepath.Join(t.TempDir(), "fail-first-native-implement")
	t.Setenv("STEERLINE_FAKE_NATIVE_STEP_FAIL", "implement")
	t.Setenv("STEERLINE_FAKE_NATIVE_STEP_FAIL_MARKER", retryMarker)
	if err = s.dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.worker == nil {
		t.Fatal("native implement attempt did not start")
	}
	failedWorker := s.worker
	failedRunID := s.find("101").RunID
	failedExecutionID := s.find("101").ExecutionID
	if finishErr := s.finished(<-failedWorker.Done); finishErr == nil {
		t.Fatal("injected implement failure did not settle as failed")
	}
	failedAttemptID := s.find("101").AttemptID
	if s.find("101").ExecutionStatus != "failed" || s.find("101").FinishTaskStatus != "blocked" || !strings.Contains(deref(s.find("101").Error), "exit status 38") {
		t.Fatalf("failed native step did not settle with its execution blocked: %+v", s.find("101"))
	}
	authority, err = s.domainStore.nativeAuthority(context.Background(), c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	manual = "manual"
	if _, err = (CommandService{Store: s.domainStore, Project: c.Identity()}).SetNativeDispatchState(context.Background(), NativeDispatchUpdate{RequestID: newID(), ExpectedAuthorityVersion: authority.AuthorityVersion, Mode: &manual, RequestedBy: "operator"}); err != nil {
		t.Fatal(err)
	}
	if err = s.refreshNativeSnapshot(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	s.State.Service.Status, s.State.Service.Ready = "running", true
	failedRun := s.find("101")
	serviceCommand := CommandRecord{CommandID: newID(), Kind: "retry_run", TargetID: failedRunID, ExpectedVersion: failedRun.ControlRevision, RequestedBy: "operator", Payload: mustJSON(ServiceCommandPayload{ServiceInstanceID: s.State.Service.InstanceID, ExpectedRevision: failedRun.ControlRevision, RunAttemptID: failedAttemptID, Body: mustJSON(RecoveryCommandBody{ConfirmedProcessStopped: true})})}
	status, result := s.applyServiceCommand(context.Background(), serviceCommand)
	if status != "accepted" {
		t.Fatalf("online native retry was not admitted: %s %s", status, result)
	}
	if s.worker == nil || s.find("101").AttemptID == failedAttemptID {
		t.Fatal("native retry did not create a new step attempt on the same workflow")
	}
	if receipt, applied, proofErr := s.nativeRetryProof(context.Background(), s.find("101"), failedAttemptID, "operator"); proofErr != nil || !applied || receipt.AttemptID != s.find("101").AttemptID {
		t.Fatalf("online native retry has no durable center proof: %+v %t %v", receipt, applied, proofErr)
	}
	recoveryCommands := CommandService{Store: s.domainStore, Project: c.Identity()}
	serviceCommand.CommandID, serviceCommand.IdempotencyKey, serviceCommand.Status, serviceCommand.CreatedAt = newID(), "", "pending", now()
	serviceCommand.IdempotencyKey = serviceCommand.CommandID
	queued, _, queueErr := recoveryCommands.Submit(context.Background(), serviceCommand)
	if queueErr != nil {
		t.Fatal(queueErr)
	}
	if err = recoveryCommands.SetStatus(context.Background(), queued, "in_flight", nil); err != nil {
		t.Fatal(err)
	}
	queued.Status = "in_flight"
	if err = s.recoverInFlightServiceCommand(context.Background(), recoveryCommands, queued); err != nil {
		t.Fatal(err)
	}
	reconciled, err := s.domainStore.Command(context.Background(), c.Identity(), queued.CommandID)
	if err != nil || reconciled.Status != "accepted" {
		t.Fatalf("in-flight online native retry was not reconciled from its center attempt: %+v %v", reconciled, err)
	}
	retryTask := nativeTaskRecord(s.State.Tasks, "101")
	retryWorkflow, _ := s.minimalRecords(s.find("101"))
	replay, replayErr := (CommandService{Store: s.domainStore, Project: c.Identity()}).RetryNativeStep(context.Background(), NativeStepRetryOptions{RequestID: nativeRetryRequestID("101", failedRunID, failedExecutionID, failedAttemptID), TaskID: "101", ExpectedTaskVersion: retryTask.Version, WorkflowRunID: failedRunID, ExecutionID: failedExecutionID, FailedAttemptID: failedAttemptID, RequestedBy: "operator"})
	if replayErr != nil || retryWorkflow == nil || replay.AttemptID != s.find("101").AttemptID || replay.PreviousAttempt != failedAttemptID {
		t.Fatalf("center retry command did not replay its fixed receipt: %+v %v", replay, replayErr)
	}
	for range 3 {
		if s.worker == nil {
			t.Fatal("native project step did not start a worker")
		}
		worker := s.worker
		finishErr := <-worker.Done
		if err = s.finished(finishErr); err != nil {
			t.Fatal(err)
		}
		if s.find("101").FinishTaskStatus == "waiting_pr_merge" {
			break
		}
		if err = s.resumeMinimalSteps(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	detail, err := (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "101")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Tasks[0].Status != "in_progress" || len(detail.Tasks[0].WorkflowRuns) != 1 || detail.Tasks[0].WorkflowRuns[0].Phase != "waiting_pr_merge" {
		t.Fatalf("native steps did not reach the human merge gate: %+v", detail.Tasks[0])
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = nil
	s, err = NewService(c)
	if err != nil {
		t.Fatalf("native service restart did not recover the center workflow: %v", err)
	}
	if s.worker != nil || s.find("101") == nil || s.find("101").FinishTaskStatus != "waiting_pr_merge" {
		t.Fatalf("native service restart changed worker or PR-wait identity: %+v", s.State.Runs)
	}
	blocked, err := (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "102")
	if err != nil || blocked.Tasks[0].DependencyReady || len(blocked.Tasks[0].UnmetPredecessors) != 1 {
		t.Fatalf("successor was not blocked before A delivery: %+v %v", blocked, err)
	}
	db := readFake(fakePRPath)
	db.PRState, db.PRMerged, db.PRMergedAt, db.PRMergeCommit = "merged", true, ptr(time.Now().UTC()), commit
	writeFake(fakePRPath, db)
	if err = s.pollPRs(context.Background()); err != nil {
		snapshot, _ := s.domainStore.Query(context.Background(), c.Identity())
		t.Fatalf("native delivery failed: %v; workflows=%+v executions=%+v attempts=%+v", err, snapshot.WorkflowRuns, snapshot.Executions, snapshot.Attempts)
	}
	delivered, err := (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "101")
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Tasks[0].Status != "closed" || delivered.Tasks[0].Delivery == nil || delivered.Tasks[0].Delivery.MergeCommit != commit {
		t.Fatalf("native merge verification did not commit TaskDelivery: %+v", delivered.Tasks[0])
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = nil
	s, err = NewService(c)
	if err != nil {
		t.Fatalf("native service restart lost completed execution identity: %v", err)
	}
	if s.find("101") == nil || s.find("101").AttemptID == "" || s.find("101").FinishTaskStatus != "completed" {
		t.Fatalf("completed native workflow did not retain its latest attempt identity: %+v", s.State.Runs)
	}
	ready, err := (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "102")
	if err != nil || !ready.Tasks[0].DependencyReady {
		t.Fatalf("successor did not become ready after center delivery: %+v %v", ready, err)
	}
	db = readFake(fakePRPath)
	db.PRState, db.PRMerged, db.PRMergedAt, db.PRMergeCommit = "open", false, nil, ""
	writeFake(fakePRPath, db)
	authority, err = s.domainStore.nativeAuthority(context.Background(), c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	auto = "auto"
	if _, err = (CommandService{Store: s.domainStore, Project: c.Identity()}).SetNativeDispatchState(context.Background(), NativeDispatchUpdate{RequestID: newID(), ExpectedAuthorityVersion: authority.AuthorityVersion, Mode: &auto, RequestedBy: "operator"}); err != nil {
		t.Fatal(err)
	}
	if err = s.refreshNativeSnapshot(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err = s.dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if s.worker == nil {
			t.Fatal("ready dependent task did not start its native worker")
		}
		worker := s.worker
		if err = s.finished(<-worker.Done); err != nil {
			t.Fatal(err)
		}
		if s.find("102").FinishTaskStatus == "waiting_pr_merge" {
			break
		}
		if err = s.resumeMinimalSteps(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	runB := s.find("102")
	if runB == nil || runB.FinishTaskStatus != "waiting_pr_merge" || runB.PR.URL == nil {
		t.Fatalf("dependent native task did not reach its fixed PR wait: %+v", runB)
	}
	bDetail, err := (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "102")
	if err != nil {
		t.Fatal(err)
	}
	stopLock, err := LockRepository(c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	commandStore, err := OpenNativeTaskCommandStore(context.Background(), &c)
	if err != nil {
		stopLock.Close()
		t.Fatal(err)
	}
	discardedPR, err := (CommandService{Store: commandStore, Project: c.Identity()}).DiscardNativeTask(context.Background(), c, "102", strings.Repeat("c", 32), bDetail.Tasks[0].Version, "operator discarded published work", "operator")
	closeErr := commandStore.Close()
	stopLock.Close()
	if err != nil || closeErr != nil || discardedPR.Status != "discarded" || !hasString(discardedPR.CleanupPending, "pull_request_close:"+*runB.PR.URL) {
		t.Fatalf("published native discard did not retain its exact PR cleanup identity: %+v err=%v close=%v", discardedPR, err, closeErr)
	}
	bDetail, err = (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "102")
	if err != nil || len(bDetail.Tasks) != 1 {
		t.Fatalf("discarded native task could not be read for cleanup validation: %+v %v", bDetail, err)
	}
	discardedVersion := bDetail.Tasks[0].Version
	workspaceItem := ""
	for _, pending := range discardedPR.CleanupPending {
		if strings.HasPrefix(pending, "workspace_cleanup:") {
			workspaceItem = pending
			break
		}
	}
	if workspaceItem == "" {
		t.Fatalf("discarded native task has no fixed workspace cleanup identity: %+v", discardedPR)
	}
	if _, badReceiptErr := (CommandService{Store: s.domainStore, Project: c.Identity()}).RecordNativeCleanupReceipt(context.Background(), "102", strings.Repeat("9", 32), discardedVersion, workspaceItem, "operator", json.RawMessage(`{}`)); !isNativeErrorCode(badReceiptErr, "contract_mismatch") {
		t.Fatalf("unverified workspace cleanup receipt was accepted: %v", badReceiptErr)
	}
	cleanupWorkflow, cleanupExecution := s.minimalRecords(runB)
	if cleanupWorkflow == nil || cleanupExecution == nil {
		t.Fatal("discarded workspace lost its fixed execution identity")
	}
	forged := NativeWorkspaceCleanupReceipt{WorkflowRunID: cleanupWorkflow.WorkflowRunID, ExecutionID: cleanupExecution.ExecutionID, Repository: cleanupExecution.Target.ID, Worktree: strings.TrimPrefix(workspaceItem, "workspace_cleanup:"), Branch: cleanupExecution.Target.Branch, BaseCommit: cleanupExecution.Target.BaseRevision, WorktreeGone: true, BranchGone: true, ObservedAt: now()}
	forgedBytes, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	if _, badReceiptErr := (CommandService{Store: s.domainStore, Project: c.Identity()}).RecordNativeCleanupReceipt(context.Background(), "102", strings.Repeat("8", 32), discardedVersion, workspaceItem, "operator", forgedBytes); !isNativeErrorCode(badReceiptErr, "outcome_unknown") {
		t.Fatalf("workspace cleanup was settled while its fixed worktree still existed: %v", badReceiptErr)
	}
	bDetail, err = (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "102")
	if err != nil || bDetail.Tasks[0].Version != discardedVersion || !hasString(bDetail.Tasks[0].CleanupPending, workspaceItem) {
		t.Fatalf("rejected workspace receipt changed the discarded task: %+v %v", bDetail, err)
	}
	if _, err = s.domainStore.db.Exec(`CREATE TRIGGER fail_native_cleanup_receipt BEFORE UPDATE OF version,payload ON tasks WHEN OLD.status='discarded' AND json_array_length(json_extract(OLD.payload,'$.cleanup_pending')) > 0 BEGIN SELECT RAISE(ABORT,'injected native cleanup receipt failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err = s.reconcile(context.Background()); err == nil || !strings.Contains(err.Error(), "injected native cleanup receipt failure") {
		t.Fatalf("native cleanup receipt failure was not injected: %v", err)
	}
	bDetail, err = (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "102")
	if err != nil || !hasString(bDetail.Tasks[0].CleanupPending, "pull_request_close:"+*runB.PR.URL) || !hasString(bDetail.Tasks[0].CleanupPending, "workspace_cleanup:"+deref(runB.WorktreePath)) {
		t.Fatalf("failed cleanup receipt erased the pending cleanup identities: %+v %v", bDetail.Tasks[0], err)
	}
	db = readFake(fakePRPath)
	if db.PRState != "closed_unmerged" || db.PRMerged {
		t.Fatalf("cleanup receipt failure lost the external closed-PR fact: %+v", db)
	}
	if _, statErr := os.Lstat(*runB.WorktreePath); !os.IsNotExist(statErr) {
		t.Fatalf("independent workspace cleanup did not run after PR receipt failure: %v", statErr)
	}
	if _, err = s.domainStore.db.Exec(`DROP TRIGGER fail_native_cleanup_receipt`); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = nil
	db = readFake(fakePRPath)
	db.PRState = "unknown"
	writeFake(fakePRPath, db)
	s, err = NewService(c)
	if err != nil {
		t.Fatalf("native service did not recover a failed cleanup receipt: %v", err)
	}
	if err = s.reconcile(context.Background()); err != nil {
		t.Fatalf("workspace cleanup could not proceed while PR state was unknown: %v", err)
	}
	bDetail, err = (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "102")
	if err != nil || !hasString(bDetail.Tasks[0].CleanupPending, "pull_request_close:"+*runB.PR.URL) || hasString(bDetail.Tasks[0].CleanupPending, workspaceItem) {
		t.Fatalf("PR uncertainty did not leave only its own cleanup pending: %+v %v", bDetail, err)
	}
	if _, statErr := os.Lstat(*runB.WorktreePath); !os.IsNotExist(statErr) {
		t.Fatalf("independent workspace cleanup retained the discarded worktree: %v", statErr)
	}
	db = readFake(fakePRPath)
	db.PRState = "closed_unmerged"
	writeFake(fakePRPath, db)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if err = s.reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		bDetail, err = (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "102")
		if err != nil {
			t.Fatal(err)
		}
		if len(bDetail.Tasks[0].CleanupPending) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if bDetail.Tasks[0].Status != "discarded" || bDetail.Tasks[0].Delivery != nil || len(bDetail.Tasks[0].CleanupPending) != 0 || runB.WorktreePath == nil {
		t.Fatalf("native PR/workspace cleanup did not settle independently: %+v", bDetail.Tasks[0])
	}
	if _, statErr := os.Lstat(*runB.WorktreePath); !os.IsNotExist(statErr) {
		t.Fatalf("discarded PR task worktree was not removed: %v", statErr)
	}
	db = readFake(fakePRPath)
	if db.PRState != "closed_unmerged" || db.PRMerged {
		t.Fatalf("native PR close command did not persist the platform's unmerged close: %+v", db)
	}
	t.Setenv("STEERLINE_FAKE_NATIVE_STEP_WAIT", "30s")
	if err = s.dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopWorker := s.worker
	runC := s.find("103")
	if stopWorker == nil || runC == nil {
		t.Fatal("disconnected native task did not start its worker")
	}
	cDetail, err := (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "103")
	if err != nil {
		t.Fatal(err)
	}
	stopLock, err = LockRepository(c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	commandStore, err = OpenNativeTaskCommandStore(context.Background(), &c)
	if err != nil {
		stopLock.Close()
		t.Fatal(err)
	}
	discarded, err := (CommandService{Store: commandStore, Project: c.Identity()}).DiscardNativeTask(context.Background(), c, "103", strings.Repeat("d", 32), cDetail.Tasks[0].Version, "operator discarded active work", "operator")
	closeErr = commandStore.Close()
	stopLock.Close()
	if err != nil || closeErr != nil || discarded.Status != "discarded" || !hasString(discarded.CleanupPending, "execution_stop_manual:"+runC.ExecutionID) {
		t.Fatalf("active native discard did not preserve its stop identity: %+v err=%v close=%v", discarded, err, closeErr)
	}
	deadline = time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if err = s.reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		cDetail, err = (QueryService{Store: s.domainStore, Project: c.Identity()}).ShowNativeTask(context.Background(), "103")
		if err != nil {
			t.Fatal(err)
		}
		if !hasString(cDetail.Tasks[0].CleanupPending, "execution_stop_manual:"+runC.ExecutionID) && !hasString(cDetail.Tasks[0].CleanupPending, "workspace_cleanup:"+deref(runC.WorktreePath)) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if hasString(cDetail.Tasks[0].CleanupPending, "execution_stop_manual:"+runC.ExecutionID) || cDetail.Tasks[0].Status != "discarded" || cDetail.Tasks[0].Delivery != nil || hasString(cDetail.Tasks[0].CleanupPending, "workspace_cleanup:"+deref(runC.WorktreePath)) {
		t.Fatalf("confirmed local stop/workspace cleanup changed discard identity or remained pending: %+v", cDetail.Tasks[0])
	}
	if _, statErr := os.Lstat(deref(runC.WorktreePath)); !os.IsNotExist(statErr) {
		t.Fatalf("discarded native worktree was not removed after its exact stop receipt: %v", statErr)
	}
	stoppedSnapshot, err := s.domainStore.Query(context.Background(), c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	var foundStoppedExecution, foundStoppedAttempt bool
	for _, execution := range stoppedSnapshot.Executions {
		if execution.ExecutionID == runC.ExecutionID {
			foundStoppedExecution = execution.Status == "stopped"
		}
	}
	for _, attempt := range stoppedSnapshot.Attempts {
		if attempt.AttemptID == runC.AttemptID {
			foundStoppedAttempt = attempt.Status == "stopped"
		}
	}
	stopOperation, err := (executionRecords{root: c.RunDir}).loadOperation(runC.RunID, runC.AttemptID, "stop-"+runC.AttemptID)
	if err != nil || stopOperation.Status != OperationConfirmed || !foundStoppedExecution || !foundStoppedAttempt {
		t.Fatalf("local discard stop lacked center and backend receipts: op=%+v execution_stopped=%t attempt_stopped=%t err=%v", stopOperation, foundStoppedExecution, foundStoppedAttempt, err)
	}
	select {
	case <-stopWorker.Done:
	default:
		t.Fatal("confirmed native stop did not settle the original worker observer")
	}
}

func TestNativeProjectInitializesOnceAndRejectsMismatchedBinding(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	db, err := OpenNativeTaskProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenSQLiteReader(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	authority, err := reader.nativeAuthority(context.Background(), c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	if authority.Authority != "native" || authority.AuthorityVersion != 1 || authority.GraphRevision != 0 || authority.DispatchMode != "manual" || !authority.DispatchPaused {
		t.Fatalf("bad initial native authority: %+v", authority)
	}
	repeat, openErr := OpenNativeTaskProject(context.Background(), &c)
	if openErr != nil {
		t.Fatalf("repeat open: %v", openErr)
	}
	_ = repeat.Close()
	writable, openErr := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if openErr != nil {
		t.Fatal(openErr)
	}
	next := authority
	next.AuthorityVersion = 2
	next.UpdatedAt = authority.UpdatedAt.Add(time.Minute)
	if openErr = writable.Transact(context.Background(), func(tx DomainTx) error { return tx.UpdateProjectTaskAuthorityCAS(next, 1) }); openErr != nil {
		t.Fatal(openErr)
	}
	if openErr = writable.Transact(context.Background(), func(tx DomainTx) error { return tx.UpdateProjectTaskAuthorityCAS(next, 1) }); openErr == nil || !strings.Contains(openErr.Error(), "stale_version") {
		t.Fatalf("stale project authority CAS was accepted: %v", openErr)
	}
	_ = writable.Close()
	changed := c
	changed.Branch = "release"
	if _, err = OpenNativeTaskProject(context.Background(), &changed); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("branch drift did not fail closed: %v", err)
	}
	beadsConfig := c
	beadsConfig.TaskAuthority = "beads"
	if err = preflightExistingState(beadsConfig.RunDir, beadsConfig.RuntimeFile); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("Beads config opened native task authority: %v", err)
	}
}

func TestNativePreflightAllowsItsEmptyRunLockFile(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	store, err := OpenNativeTaskProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(c.RunDir, "steerline.lock")
	if err = os.WriteFile(lockPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err = preflightNativeStatePath(c); err != nil {
		t.Fatalf("native preflight rejected its own empty OS lock file: %v", err)
	}
	if err = os.WriteFile(lockPath, []byte("unknown"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = preflightNativeStatePath(c); err == nil || !strings.Contains(err.Error(), "native service lock path is not an empty regular file") {
		t.Fatalf("native preflight accepted a nonempty lock path: %v", err)
	}
}

func TestNativeTaskQuerySnapshotPagingPersistenceAndIsolation(t *testing.T) {
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	reader, err := OpenNativeTaskProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	project := c.Identity()
	contract2 := nativeContract(t, c, commit, "contract-2")
	contract10 := nativeContract(t, c, commit, "contract-10")
	foreignContract := nativeContract(t, c, commit, "foreign-contract")
	foreignContract.Repository = "https://example.invalid/Other/Repo.git"
	foreignContract.Digest, err = TaskContractDigest(foreignContract)
	if err != nil {
		t.Fatal(err)
	}
	batchDigest := strings.Repeat("a", 64)
	created := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	staleBatch := TaskBatchRecord{BatchID: "stale-batch", ProjectIdentity: project, SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("f", 64), CreatedAt: created, CommandID: "stale-command", TaskIDs: map[string]string{"stale": "99"}}
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.PutTaskBatch(staleBatch, 2, 0) }); err == nil || !strings.Contains(err.Error(), "stale_version") {
		t.Fatalf("stale project authority created a batch: %v", err)
	}
	var staleCount int
	if err = store.db.QueryRow(`SELECT COUNT(*) FROM task_batches WHERE project_identity=? AND batch_id=?`, project, "stale-batch").Scan(&staleCount); err != nil || staleCount != 0 {
		t.Fatalf("stale batch transaction left partial state: count=%d err=%v", staleCount, err)
	}
	err = store.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutTaskBatch(TaskBatchRecord{BatchID: "batch-1", ProjectIdentity: project, SchemaVersion: "task-batch/v1", PayloadDigest: batchDigest, CreatedAt: created, CommandID: "command-1", TaskIDs: map[string]string{"two": "2", "ten": "10"}}, 1, 0); err != nil {
			return err
		}
		for _, entry := range []struct {
			id       string
			contract TaskContract
			priority int
		}{{"2", contract2, 1}, {"10", contract10, 2}} {
			record := TaskRecord{ProjectIdentity: project, TaskID: entry.id, BatchID: "batch-1", Title: "task " + entry.id, Type: "task", Priority: entry.priority, ContractID: entry.contract.ContractID, Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
			if err := tx.PutTask(record); err != nil {
				return err
			}
			if err := tx.PutTaskContract(project, entry.id, entry.contract); err != nil {
				return err
			}
		}
		foreignProject := "other-project"
		otherAuthority := ProjectTaskAuthority{ProjectIdentity: foreignProject, Authority: "native", AuthorityVersion: 1, Repository: foreignContract.Repository, Remote: "origin", Branch: "main", DispatchMode: "manual", DispatchPaused: true, CreatedAt: created, UpdatedAt: created}
		if err := tx.PutProjectTaskAuthority(otherAuthority); err != nil {
			return err
		}
		if err := tx.PutTaskBatch(TaskBatchRecord{BatchID: "other-batch", ProjectIdentity: foreignProject, SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("e", 64), CreatedAt: created, CommandID: "other-command", TaskIDs: map[string]string{"other": "2"}}, 1, 0); err != nil {
			return err
		}
		foreign := TaskRecord{ProjectIdentity: foreignProject, TaskID: "2", BatchID: "other-batch", Title: "foreign", Type: "task", ContractID: "foreign-contract", Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
		if err := tx.PutTask(foreign); err != nil {
			return err
		}
		if err := tx.PutTaskContract(foreignProject, "2", foreignContract); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	replayedBatch := TaskBatchRecord{BatchID: "batch-1", ProjectIdentity: project, SchemaVersion: "task-batch/v1", PayloadDigest: batchDigest, CreatedAt: created, CommandID: "command-1", TaskIDs: map[string]string{"two": "2", "ten": "10"}}
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.PutTaskBatch(replayedBatch, 1, 0) }); err != nil {
		t.Fatalf("identical batch retry was not idempotent: %v", err)
	}
	query := QueryService{Store: store, Project: project}
	first, err := query.ListNativeTasks(ctx, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Protocol != "native-tasks/v1" || first.ProjectID != project || first.AuthorityVersion != 1 || first.GraphRevision != 1 || first.DatabaseRevision == 0 || first.SampledAt.IsZero() || len(first.Tasks) != 1 || first.Tasks[0].TaskID != "2" || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("bad first page: %+v", first)
	}
	if first.Capabilities.DependencyReadiness != "provided" || first.Capabilities.DependencyBlockers != "provided" {
		t.Fatalf("DAG query capability was not provided: %+v", first.Capabilities)
	}
	second, err := query.ListNativeTasks(ctx, first.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Tasks) != 1 || second.Tasks[0].TaskID != "10" || second.ProjectID != project {
		t.Fatalf("numeric ordering or project isolation failed: %+v", second)
	}
	show, err := query.ShowNativeTask(ctx, "2")
	if err != nil {
		t.Fatal(err)
	}
	if len(show.Tasks) != 1 || show.Tasks[0].Contract.ContractID != "contract-2" || show.Tasks[0].Status != "open" {
		t.Fatalf("bad task detail: %+v", show)
	}
	duplicate := TaskRecord{ProjectIdentity: project, TaskID: "2", BatchID: "batch-1", Title: "replacement", Type: "task", ContractID: "contract-2", Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.PutTask(duplicate) }); err == nil {
		t.Fatal("duplicate numeric task ID was accepted")
	}
	// Any committed center write changes the database revision and invalidates a continuation cursor.
	if err = store.Transact(ctx, func(tx DomainTx) error {
		_, err := tx.AdvanceTaskGraphRevision(project, first.AuthorityVersion, first.GraphRevision)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = query.ListNativeTasks(ctx, first.NextCursor, 1); err == nil || !strings.Contains(err.Error(), "cursor_invalidated") {
		t.Fatalf("stale page cursor was accepted: %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenSQLiteReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	restarted, err := (QueryService{Store: store, Project: project}).ShowNativeTask(ctx, "10")
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.Tasks) != 1 || restarted.Tasks[0].Title != "task 10" {
		t.Fatalf("task projection did not survive restart: %+v", restarted)
	}
}

func TestNativeTaskCASGuardsAndDeliveryProjection(t *testing.T) {
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	r, err := OpenNativeTaskProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	project := c.Identity()
	contract := nativeContract(t, c, commit, "contract-1")
	created := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	task := TaskRecord{ProjectIdentity: project, TaskID: "7", BatchID: "batch-7", Title: "task", Type: "task", Priority: 2, ContractID: contract.ContractID, Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
	err = store.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutTaskBatch(TaskBatchRecord{BatchID: "batch-7", ProjectIdentity: project, SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("b", 64), CreatedAt: created, CommandID: "cmd-7", TaskIDs: map[string]string{"seven": "7"}}, 1, 0); err != nil {
			return err
		}
		if err := tx.PutTask(task); err != nil {
			return err
		}
		return tx.PutTaskContract(project, "7", contract)
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed := task
	claimed.Status = "in_progress"
	claimed.Claimant = "agent"
	claimed.Version = 2
	claimed.UpdatedAt = created.Add(time.Minute)
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.UpdateNativeTaskCAS(claimed, 1) }); err != nil {
		t.Fatal(err)
	}
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.UpdateNativeTaskCAS(claimed, 1) }); err == nil || !strings.Contains(err.Error(), "stale_version") {
		t.Fatalf("stale task update was accepted: %v", err)
	}
	closed := claimed
	closed.Status = "closed"
	closed.Version = 3
	closed.UpdatedAt = created.Add(2 * time.Minute)
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.UpdateNativeTaskCAS(closed, 2) }); err == nil {
		t.Fatal("unverified delivery closed a task")
	}
	run := WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "wf-7", TaskID: "7", WorkflowVersion: MinimalWorkflowVersion, Phase: "waiting_pr_merge", SelectedResult: "exec-7", CreatedAt: created, UpdatedAt: created}
	execution := ExecutionRecord{ProjectIdentity: project, ExecutionID: "exec-7", WorkflowRunID: "wf-7", Status: "succeeded", CurrentStep: "waiting_pr_merge", CreatedAt: created, UpdatedAt: created}
	err = store.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutWorkflowRun(run); err != nil {
			return err
		}
		return tx.PutExecution(execution)
	})
	if err != nil {
		t.Fatal(err)
	}
	delivery := TaskDeliveryRecord{ProjectIdentity: project, TaskID: "7", SelectedWorkflowRunID: "wf-7", PRInstance: "https://forge.invalid", PRRepository: "org/repo", PRNumber: 4, PRURL: "https://forge.invalid/org/repo/pulls/4", MergeCommit: strings.Repeat("c", 40), MergedAt: created.Add(3 * time.Minute), RecordedAt: created.Add(4 * time.Minute), Evidence: json.RawMessage(`{"merged":true}`)}
	delivery.Evidence, err = json.Marshal(TaskDeliveryEvidence{PRInstance: delivery.PRInstance, URL: delivery.PRURL, Repository: delivery.PRRepository, PRNumber: delivery.PRNumber, BaseBranch: "main", State: "merged", Merged: true, MergedAt: delivery.MergedAt, MergeCommit: delivery.MergeCommit})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutTaskDelivery(delivery); err != nil {
			return err
		}
		return tx.UpdateNativeTaskCAS(closed, 2)
	}); err != nil {
		t.Fatal(err)
	}
	page, err := (QueryService{Store: store, Project: project}).ShowNativeTask(ctx, "7")
	if err != nil {
		t.Fatal(err)
	}
	if page.Tasks[0].Status != "closed" || page.Tasks[0].Delivery == nil || page.Tasks[0].Delivery.MergeCommit != delivery.MergeCommit || len(page.Tasks[0].WorkflowRuns) != 1 {
		t.Fatalf("delivery/run association did not project: %+v", page.Tasks[0])
	}
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.PutTaskDelivery(delivery) }); err != nil {
		t.Fatalf("same verified delivery was not idempotent after restart boundary: %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenSQLiteReader(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	recovered, err := (QueryService{Store: restarted, Project: project}).ShowNativeTask(ctx, "7")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Tasks[0].Delivery == nil || recovered.Tasks[0].WorkflowRuns[0].SelectedResult != "exec-7" || recovered.Tasks[0].WorkflowRuns[0].Phase != "completed" || recovered.Tasks[0].Status != "closed" || recovered.GraphRevision != 2 {
		t.Fatalf("task/run/delivery projection did not survive restart: %+v", recovered.Tasks[0])
	}
}

func TestNativeDiscardCASKeepsAuditAndNeverReusesID(t *testing.T) {
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	reader, err := OpenNativeTaskProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	project := c.Identity()
	contract := nativeContract(t, c, commit, "contract-4")
	created := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	task := TaskRecord{ProjectIdentity: project, TaskID: "4", BatchID: "batch-4", Title: "discard me", Type: "task", Priority: 2, ContractID: contract.ContractID, Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
	if err = store.Transact(ctx, func(tx DomainTx) error {
		if err := tx.PutTaskBatch(TaskBatchRecord{BatchID: "batch-4", ProjectIdentity: project, SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("4", 64), CreatedAt: created, CommandID: "cmd-4", TaskIDs: map[string]string{"four": "4"}}, 1, 0); err != nil {
			return err
		}
		if err := tx.PutTask(task); err != nil {
			return err
		}
		return tx.PutTaskContract(project, "4", contract)
	}); err != nil {
		t.Fatal(err)
	}
	discarded := task
	discarded.Status = "discarded"
	discarded.Version = 2
	discarded.DiscardedAt = &created
	discarded.DiscardReason = "superseded by explicit follow-up"
	discarded.DiscardedBy = "fixture"
	discarded.DiscardRequestID = strings.Repeat("a", 32)
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.UpdateNativeTaskCAS(discarded, 1) }); err != nil {
		t.Fatal(err)
	}
	reopened := discarded
	reopened.Status = "open"
	reopened.Version = 3
	reopened.DiscardedAt = nil
	reopened.DiscardReason = ""
	reopened.DiscardedBy = ""
	reopened.DiscardRequestID = ""
	reopened.CleanupPending = nil
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.UpdateNativeTaskCAS(reopened, 2) }); err == nil || !strings.Contains(err.Error(), "stale_version") {
		t.Fatalf("discarded task was reopened: %v", err)
	}
	replacement := task
	replacement.Title = "new identity"
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.PutTask(replacement) }); err == nil {
		t.Fatal("discarded task ID was reused")
	}
	page, err := (QueryService{Store: store, Project: project}).ShowNativeTask(ctx, "4")
	if err != nil {
		t.Fatal(err)
	}
	if page.Tasks[0].Status != "discarded" || page.Tasks[0].DiscardedAt == nil || page.Tasks[0].DiscardReason != discarded.DiscardReason || page.Tasks[0].Delivery != nil || page.GraphRevision != 2 {
		t.Fatalf("discard audit was not retained: %+v", page.Tasks[0])
	}
}

func TestTaskContractPinsCommittedBlobsAndRejectsInvalidReferences(t *testing.T) {
	ctx := context.Background()
	c, commit := nativeTaskFixture(t)
	contract := nativeContract(t, c, commit, "contract")
	if err := VerifyTaskContractFiles(ctx, c, contract); err != nil {
		t.Fatal(err)
	}
	// Checkout newline conversion does not affect the pinned Git blob bytes.
	specPath := filepath.Join(c.Repository, filepath.FromSlash(contract.SpecRef.Path))
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(specPath, []byte(strings.ReplaceAll(string(raw), "\n", "\r\n")), 0600); err != nil {
		t.Fatal(err)
	}
	if err = VerifyTaskContractFiles(ctx, c, contract); err != nil {
		t.Fatalf("CRLF checkout changed committed-blob verification: %v", err)
	}
	for _, bad := range []string{"../outside.spec.md", "/absolute.spec.md", "C:/outside.spec.md", "docs\\spec\\bad.spec.md"} {
		v := contract
		v.SpecRef.Path = bad
		v.Digest, _ = TaskContractDigest(v)
		if err = ValidateTaskContract(v); err == nil {
			t.Fatalf("invalid contract path accepted: %q", bad)
		}
	}
	missing := contract
	missing.SpecRef.Path = "docs/spec/missing-task.spec.md"
	missing.PlanRef.Path = "docs/spec/missing-task.plan.md"
	missing.Digest, _ = TaskContractDigest(missing)
	if err = VerifyTaskContractFiles(ctx, c, missing); err == nil || !strings.Contains(err.Error(), "contract_mismatch") {
		t.Fatalf("missing committed contract files were accepted: %v", err)
	}
	v := contract
	v.CommonRefs = []FileReference{contract.SpecRef}
	v.Digest, _ = TaskContractDigest(v)
	if err = ValidateTaskContract(v); err == nil {
		t.Fatal("task-specific spec was accepted as a common reference")
	}
	v = contract
	v.PlanRef = FileReference{}
	v.Digest, _ = TaskContractDigest(v)
	if err = ValidateTaskContract(v); err == nil {
		t.Fatal("missing plan was accepted")
	}
	v = contract
	v.AcceptanceRefs = []FileReference{contract.PlanRef}
	v.Digest, _ = TaskContractDigest(v)
	if err = VerifyTaskContractFiles(ctx, c, v); err == nil || !strings.Contains(err.Error(), "acceptance heading") {
		t.Fatalf("acceptance reference without its section was accepted: %v", err)
	}
	if _, err = ValidateTaskContractJSON([]byte(`{"contract_id":"one","contract_id":"two"}`)); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
		t.Fatalf("duplicate JSON key was accepted: %v", err)
	}
	// The digest is over canonical JSON and therefore independent of object key order.
	canonicalA, err := CanonicalJSON([]byte(`{"b":2,"a":{"d":4,"c":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	canonicalB, err := CanonicalJSON([]byte(` { "a" : {"c":3,"d":4},"b":2 } `))
	if err != nil {
		t.Fatal(err)
	}
	if string(canonicalA) != string(canonicalB) {
		t.Fatalf("canonical JSON differs: %s / %s", canonicalA, canonicalB)
	}
}

func TestTaskContractRejectsCommittedSymlinkReferences(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	planPath := filepath.Join(c.Repository, "docs", "spec", "linked.plan.md")
	if err := os.WriteFile(planPath, []byte("# Linked Plan\n"), 0600); err != nil {
		t.Fatal(err)
	}
	hash := exec.Command("git", "-C", c.Repository, "hash-object", "-w", "--stdin")
	hash.Stdin = strings.NewReader("task-core.spec.md")
	blob, err := hash.Output()
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", c.Repository, "update-index", "--add", "--cacheinfo", "120000", strings.TrimSpace(string(blob)), "docs/spec/linked.spec.md").CombinedOutput(); err != nil {
		t.Fatalf("stage symlink blob: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", c.Repository, "add", "--", "docs/spec/linked.plan.md").CombinedOutput(); err != nil {
		t.Fatalf("stage linked plan: %v: %s", err, out)
	}
	args := []string{"-C", c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "add symlink reference"}
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("commit symlink fixture: %v: %s", err, out)
	}
	head, err := exec.Command("git", "-C", c.Repository, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	contract := nativeContract(t, c, strings.TrimSpace(string(head)), "contract-link")
	contract.SpecRef = nativeRef(t, c, contract.SpecRef.Commit, "docs/spec/linked.spec.md")
	contract.PlanRef = nativeRef(t, c, contract.PlanRef.Commit, "docs/spec/linked.plan.md")
	contract.AcceptanceRefs = []FileReference{contract.SpecRef}
	contract.Digest, err = TaskContractDigest(contract)
	if err != nil {
		t.Fatal(err)
	}
	if err = VerifyTaskContractFiles(ctx, c, contract); err == nil || !strings.Contains(err.Error(), "regular committed files") {
		t.Fatalf("committed symlink contract path was accepted: %v", err)
	}
}

func TestNativeInitializationRejectsBeadsStateAndCustomDatabaseFork(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	if err := os.Mkdir(filepath.Join(c.Repository, ".beads"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenNativeTaskProject(ctx, &c); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("Beads state was adopted: %v", err)
	}
	if err := os.Remove(filepath.Join(c.Repository, ".beads")); err != nil {
		t.Fatal(err)
	}
	custom := c
	custom.RunDir = filepath.Join(t.TempDir(), "custom")
	if err := validateNativeStateLocation(custom); err == nil {
		t.Fatal("custom state directory could fork a second task authority")
	}
}

func TestNativeInitializationRefusesExistingBeadsTaskDatabase(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	if err := os.MkdirAll(c.RunDir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	old := TaskRecord{ProjectIdentity: "legacy-project", TaskID: "legacy:one", Source: "beads", Status: "open", Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err = store.Transact(ctx, func(tx DomainTx) error { return tx.PutTask(old) }); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenNativeTaskProject(ctx, &c); err == nil || !strings.Contains(err.Error(), "authority_mismatch") {
		t.Fatalf("native config adopted a database without matching authority: %v", err)
	}
	reader, err := OpenSQLiteReader(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var count int
	if err = reader.db.QueryRow(`SELECT COUNT(*) FROM project_task_authorities`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed native init wrote project authority: count=%d", count)
	}
}

func TestNativeDatabaseInitializesCurrentSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steerline.db")
	// A fresh database contains the center model and native record constraints.
	db, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err = db.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != domainSchemaVersion {
		t.Fatalf("schema version=%d", version)
	}
	for _, table := range []string{"project_task_authorities", "task_contracts", "task_batches", "task_dependencies", "task_deliveries", "database_state"} {
		var count int
		if err = db.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("missing table %s: count=%d err=%v", table, count, err)
		}
	}
	var revision uint64
	if err = db.db.QueryRow(`SELECT revision FROM database_state WHERE singleton=1`).Scan(&revision); err != nil || revision != 0 {
		t.Fatalf("unexpected initial DB revision %d: %v", revision, err)
	}
}

func TestSchemaOneCenterDatabaseIsRejectedWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "center-v1.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := raw.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range domainExecutionSchema {
		if _, err = tx.Exec(stmt); err != nil {
			t.Fatalf("create center schema: %v: %s", err, stmt)
		}
	}
	created := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	old := TaskRecord{ProjectIdentity: "repo:previous-center", TaskID: "legacy:stable", Title: "center task", Source: "beads", Status: "in_progress", Version: 2, CreatedAt: created, UpdatedAt: created}
	payload, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO tasks(project_identity,task_id,status,version,payload) VALUES(?,?,?,?,?)`, old.ProjectIdentity, old.TaskID, old.Status, old.Version, payload); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(1,?)`, created.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if store, err := OpenSQLiteStore(path); err == nil {
		store.Close()
		t.Fatal("previous database was automatically upgraded")
	} else {
		requireNativeAuditCode(t, err, "unsupported_schema")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected database was modified: %v", err)
	}
}

func TestNativeCursorErrorHasStableCode(t *testing.T) {
	var native *NativeTaskError
	if !errors.As(nativeErr("cursor_invalidated", "expired"), &native) || native.Code != "cursor_invalidated" {
		t.Fatalf("native error code missing: %+v", native)
	}
}
