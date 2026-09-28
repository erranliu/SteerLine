package steerline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExistingAcceptanceClosesOnlySettledOriginalTask(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	for path, content := range map[string]string{
		"docs/spec/pr-05-intervention.spec.md": "# Spec\n\n## Acceptance\n\n| ID | Gate |\n| --- | --- |\n| H-A1 | yes |\n| H-A2 | yes |\n",
		"docs/validation/acceptance.md":        "# Evidence\n\n| ID | Result |\n| --- | --- |\n| H-A1 | passed |\n| H-A2 | passed |\n",
	} {
		full := filepath.Join(c.Repository, filepath.FromSlash(path))
		require(t, os.MkdirAll(filepath.Dir(full), 0700))
		require(t, os.WriteFile(full, []byte(content), 0600))
	}
	for _, args := range [][]string{{"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "acceptance fixture"}} {
		out, err := exec.Command("git", append([]string{"-C", c.Repository}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	output, err := exec.Command("git", "-C", c.Repository, "rev-parse", "HEAD").Output()
	require(t, err)
	commit := strings.TrimSpace(string(output))
	remoteURL := "https://forge.invalid/org/repo.git"
	output, err = exec.Command("git", "-C", c.Repository, "remote", "set-url", "origin", remoteURL).CombinedOutput()
	if err != nil {
		t.Fatalf("remote: %v: %s", err, output)
	}
	c.PR = PRConfig{Kind: "command", BaseURL: "https://forge.invalid", Repository: "org/repo"}
	reader, err := OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, reader.Close())
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	require(t, err)
	defer store.Close()
	contract := nativeContract(t, c, commit, "steerline-roadmap-3-49ac56e45454")
	contract.SpecRef = nativeRef(t, c, commit, "docs/spec/pr-05-intervention.spec.md")
	contract.AcceptanceRefs = []FileReference{contract.SpecRef}
	contract.Digest, err = TaskContractDigest(contract)
	require(t, err)
	tasks := batchTasks(3, 5)
	tasks[0].Contract = contract
	payload := taskBatchFixture(t, c, commit, tasks, []TaskBatchEdge{{Predecessor: TaskBatchReference{LocalKey: "k3"}, Successor: TaskBatchReference{LocalKey: "k5"}}})
	commands := CommandService{Store: store, Project: c.Identity()}
	_, err = commands.CreateTaskBatch(ctx, c, payload, newID(), 1, "planner")
	require(t, err)
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "3")
	require(t, err)
	task := page.Tasks[0].TaskRecord
	task.Status, task.Claimant, task.Version, task.UpdatedAt = "in_progress", "operator", 2, now()
	runID, executionID, attemptID := newID(), newID(), newID()
	workflow := WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: runID, TaskID: "3", WorkflowVersion: MinimalWorkflowVersion, Phase: "blocked", CreatedAt: now(), UpdatedAt: now()}
	execution := ExecutionRecord{ProjectIdentity: c.Identity(), WorkflowRunID: runID, ExecutionID: executionID, Status: "failed", CurrentStep: "implement", CreatedAt: now(), UpdatedAt: now()}
	attempt := StepAttemptRecord{ProjectIdentity: c.Identity(), ExecutionID: executionID, AttemptID: attemptID, StepID: "implement", AttemptNumber: 1, Status: "failed", CreatedAt: now(), UpdatedAt: now()}
	run := &Run{TaskID: "3", DomainTaskID: "3", RunID: runID, ExecutionID: executionID, AttemptID: attemptID, WorkflowVersion: MinimalWorkflowVersion, ExecutionStatus: "failed", FinishTaskStatus: "blocked"}
	state := Runtime{SchemaVersion: 5, Revision: 1, Repository: Repository{Identity: c.Identity()}, Runs: []*Run{run}, WorkflowRuns: []WorkflowRunRecord{workflow}}
	require(t, store.Transact(ctx, func(d DomainTx) error {
		tx := d.(*sqliteTx)
		if e := tx.UpdateNativeTaskCAS(task, 1); e != nil {
			return e
		}
		if e := tx.PutWorkflowRun(workflow); e != nil {
			return e
		}
		if e := tx.PutExecution(execution); e != nil {
			return e
		}
		if e := tx.PutStepAttempt(attempt); e != nil {
			return e
		}
		body, e := json.Marshal(state)
		if e != nil {
			return e
		}
		return tx.PutProjection(c.Identity(), 1, body)
	}))
	url := "https://forge.invalid/org/repo/pulls/39"
	mergedAt := task.CreatedAt.Add(-time.Hour)
	verifier := &fixedTaskDeliveryVerifier{info: PRInfo{URL: url, Repository: "org/repo", BaseBranch: "main", HeadRevision: commit, State: "merged", Merged: true, MergedAt: &mergedAt, MergeCommit: &commit}}
	input := ExistingAcceptanceInput{Version: existingAcceptanceVersion, TaskID: "3", WorkflowRunID: runID, ExpectedTaskVersion: 2, RequestID: newID(), RequestedBy: "operator", PrimaryPRURL: url, MergedPRs: []ExistingMergedPRInput{{URL: url, HeadRevision: commit}}, EvidenceRef: nativeRef(t, c, commit, "docs/validation/acceptance.md")}
	unknown := attempt
	unknown.Status = "outcome_unknown"
	require(t, store.Transact(ctx, func(d DomainTx) error { return d.PutStepAttempt(unknown) }))
	if _, err := commands.CompleteExistingAcceptance(ctx, c, input, verifier); err == nil || !strings.Contains(err.Error(), "outcome_unknown") {
		t.Fatalf("unknown attempt passed: %v", err)
	}
	page, err = (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "5")
	require(t, err)
	if page.Tasks[0].DependencyReady {
		t.Fatal("unknown attempt released successor")
	}
	require(t, store.Transact(ctx, func(d DomainTx) error { return d.PutStepAttempt(attempt) }))
	receipt, err := commands.CompleteExistingAcceptance(ctx, c, input, verifier)
	require(t, err)
	if receipt.Status != "closed" || receipt.Delivery.PRURL != url {
		t.Fatalf("bad acceptance receipt: %+v", receipt)
	}
	replayed, err := commands.CompleteExistingAcceptance(ctx, c, input, verifier)
	require(t, err)
	if replayed.TaskVersion != receipt.TaskVersion || replayed.GraphRevision != receipt.GraphRevision {
		t.Fatalf("replay changed closeout: %+v", replayed)
	}
	page, err = (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "5")
	require(t, err)
	if !page.Tasks[0].DependencyReady {
		t.Fatal("verified acceptance did not release successor")
	}
	snapshot, err := store.Query(ctx, c.Identity())
	require(t, err)
	for _, recorded := range snapshot.WorkflowRuns {
		if recorded.WorkflowRunID == runID && recorded.Phase != "blocked" {
			t.Fatalf("historical run altered: %+v", recorded)
		}
	}
}

func TestExistingAcceptanceUnknownRequiresExactStoppedRemoteIdentity(t *testing.T) {
	ctx := context.Background()
	executable, err := os.Executable()
	require(t, err)
	c := Config{Repository: t.TempDir(), CommandTimeout: "5s", Orca: OrcaConfig{Executable: executable, Args: []string{"--fake", "orca-supervised"}, Supervised: &SupervisedOrcaConfig{EnvironmentID: "env-home"}}}
	steps := ProjectSteps{}
	profile := ExecutionProfile{ID: "remote-test", Name: "Remote test", Host: "orca_supervised", Agent: "codex", Orca: c.Orca, ProjectSteps: &steps}
	workflow := WorkflowRunRecord{ExecutionProfile: ptr(profile.Freeze()), ProjectSteps: steps}
	identity := ProcessIdentity{Kind: "orca_supervised", Terminal: "terminal-acceptance", RuntimeID: "fake-remote-runtime", Incarnation: "inc-acceptance", CoordinatorRuntimeID: "fake-center-runtime", OrchestrationRun: "run-acceptance", Task: "task-acceptance", Dispatch: "dispatch-acceptance", ExecutionHost: "env-home", WorktreeID: "repo-test::worktree"}
	execution := ExecutionRecord{Process: &identity}
	t.Setenv("STEERLINE_FAKE_ACCEPTANCE_DISPATCH", "abandoned")
	t.Setenv("STEERLINE_FAKE_ACCEPTANCE_TERMINAL", "wrong-incarnation")
	if err := verifyExistingStoppedUnknown(ctx, c, workflow, execution); err == nil || !strings.Contains(err.Error(), "outcome_unknown") {
		t.Fatalf("replacement terminal passed: %v", err)
	}
	t.Setenv("STEERLINE_FAKE_ACCEPTANCE_TERMINAL", "inc-acceptance")
	t.Setenv("STEERLINE_FAKE_ACCEPTANCE_DISPATCH", "running")
	if err := verifyExistingStoppedUnknown(ctx, c, workflow, execution); err == nil || !strings.Contains(err.Error(), "outcome_unknown") {
		t.Fatalf("active Dispatch passed: %v", err)
	}
	t.Setenv("STEERLINE_FAKE_ACCEPTANCE_DISPATCH", "abandoned")
	require(t, verifyExistingStoppedUnknown(ctx, c, workflow, execution))
}

func TestExistingAcceptanceUnknownStateRejectsPublicationAndReplacement(t *testing.T) {
	profile := ExecutionProfile{ID: "remote-test", Host: "orca_supervised", ProjectSteps: &ProjectSteps{}}
	process := &ProcessIdentity{Kind: "orca_supervised", Terminal: "terminal-acceptance", RuntimeID: "remote-runtime", Incarnation: "incarnation", Dispatch: "dispatch-acceptance", ExecutionHost: "env-home"}
	workflow := WorkflowRunRecord{TaskID: "4", WorkflowRunID: "run-acceptance", WorkflowVersion: MinimalWorkflowVersion, Phase: "blocked", ExecutionProfile: ptr(profile.Freeze())}
	execution := ExecutionRecord{ExecutionID: "execution-acceptance", WorkflowRunID: workflow.WorkflowRunID, Status: "blocked", CurrentStep: "verify", Process: process}
	attempt := StepAttemptRecord{AttemptID: "attempt-acceptance", ExecutionID: execution.ExecutionID, StepID: "verify", Status: "outcome_unknown", ExternalBinding: ExecutionTarget{Dispatch: process.Dispatch}}
	run := &Run{TaskID: "4", RunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, AttemptID: attempt.AttemptID, ExecutionStatus: "unknown", FinishTaskStatus: "blocked", Process: process}
	snapshot := DomainSnapshot{WorkflowRuns: []WorkflowRunRecord{workflow}, Executions: []ExecutionRecord{execution}, Attempts: []StepAttemptRecord{attempt}}
	state := Runtime{Runs: []*Run{run}, CurrentTask: ptr("4"), SlotOccupied: true}
	input := ExistingAcceptanceInput{TaskID: "4", WorkflowRunID: workflow.WorkflowRunID, StoppedUnknown: &ExistingStoppedUnknown{AttemptID: attempt.AttemptID}}
	_, _, _, _, err := existingAcceptanceUnknownState(snapshot, state, input)
	require(t, err)
	state.CurrentTask = nil // A persisted multi-task scheduler may leave this selector empty.
	_, _, _, _, err = existingAcceptanceUnknownState(snapshot, state, input)
	require(t, err)
	snapshot.Attempts = append(snapshot.Attempts, StepAttemptRecord{AttemptID: "publish", ExecutionID: execution.ExecutionID, StepID: "publish_pr", Status: "outcome_unknown"})
	if _, _, _, _, err := existingAcceptanceUnknownState(snapshot, state, input); err == nil || !strings.Contains(err.Error(), "merge_state_unknown") {
		t.Fatalf("publish attempt passed: %v", err)
	}
	snapshot.Attempts = snapshot.Attempts[:1]
	wrong := *process
	wrong.Incarnation = "replacement"
	state.Runs[0].Process = &wrong
	if _, _, _, _, err := existingAcceptanceUnknownState(snapshot, state, input); err == nil || !strings.Contains(err.Error(), "outcome_unknown") {
		t.Fatalf("replacement process passed: %v", err)
	}
}

func TestExistingAcceptanceReconciliationKeepsOtherWorkerReservation(t *testing.T) {
	stopped := &Run{TaskID: "4", RunID: "run-4", ExecutionStatus: "failed", FinishTaskStatus: "blocked"}
	otherProcess := &ProcessIdentity{Kind: "orca_supervised", Dispatch: "dispatch-14"}
	other := &Run{TaskID: "14", RunID: "run-14", ExecutionStatus: "running", FinishTaskStatus: "pending", Process: otherProcess}
	reservation := &Service{State: Runtime{Runs: []*Run{stopped, other}, CurrentTask: ptr("4"), SlotOccupied: true}}
	reservation.recalculateCurrentReservation(stopped.RunID)
	if !reservation.State.SlotOccupied || reservation.State.CurrentTask == nil || *reservation.State.CurrentTask != "14" || reservation.State.Runs[1].Process != otherProcess {
		t.Fatalf("settling #4 disturbed the other worker: %+v", reservation.State)
	}
}
