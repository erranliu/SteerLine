package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This test uses an isolated SQLite state directory and a mock Codex child in
// a temporary worktree of a registered local Orca test repository.
func TestWorkerFlowOrcaLocalServiceFirstFeedbackLive(t *testing.T) {
	testWorkerFlowOrcaServiceFirstFeedbackLive(t, false)
}

func TestWorkerFlowOrcaRemoteServiceFirstFeedbackLive(t *testing.T) {
	if os.Getenv("STEERLINE_ORCA_REMOTE_MOCK_EXE") == "" {
		t.Skip("remote mock Codex executable was not selected")
	}
	testWorkerFlowOrcaServiceFirstFeedbackLive(t, true)
}

func testWorkerFlowOrcaServiceFirstFeedbackLive(t *testing.T, remote bool) {
	repository := os.Getenv("STEERLINE_WORKER_FLOW_ORCA_TEST_REPO")
	if repository == "" {
		t.Skip("registered Orca test repository was not selected")
	}
	t.Setenv("STEERLINE_HOME", t.TempDir())
	ctx := context.Background()
	c := Config{TaskAuthority: "native", WorkflowProtocol: WorkerFlowVersion, Repository: repository, Remote: "origin", Branch: "main", TaskPoll: "10s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "30s", ShutdownGrace: "10s", Sandbox: "read-only", Agent: "codex", Host: "orca", Claimant: "operator", ExecutableTypes: []string{"task"}, Git: Command{Executable: "git"}, Codex: Command{Executable: os.Args[0], Args: []string{"-test.run=^TestWorkerFlowMockCodexHelper$", "--", "wf-orca-mock"}}, Orca: OrcaConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "15m"}}
	if remote {
		c.Host, c.Model, c.ReasoningEffort = "orca_supervised", "gpt-6-luna", "low"
		c.Codex.Executable = os.Getenv("STEERLINE_ORCA_REMOTE_MOCK_EXE")
		c.Orca.Supervised = &SupervisedOrcaConfig{ConnectionID: "832501c0-57ad-4432-b9a7-6d931662b05e", EnvironmentID: "e68f4edf-8a83-42c2-a9da-6d404d3aa3d8", RepositorySelector: "id:903fb315-7d5e-4246-8913-816e1d3bdf03", CoordinatorTerminal: "term_a676c7b8-6b7a-4b50-b5da-3767fdc84fa1", PollInterval: "1s", CapabilityReportVersion: 1}
	}
	key, err := repoStateKey(c)
	if err != nil {
		t.Fatal(err)
	}
	c.RunDir = filepath.Join(SteerlineStateRoot(), key)
	c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	project, err := OpenNativeTaskProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	if err := project.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	baseBytes, err := exec.Command("git", "-C", c.Repository, "rev-parse", "origin/main").Output()
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSpace(string(baseBytes))
	target, err := configuredRepositoryTarget(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	spec := nativeRef(t, c, base, "docs/spec/native-101.spec.md")
	plan := nativeRef(t, c, base, "docs/spec/native-101.plan.md")
	contract := TaskContract{ContractID: "wf-orca-smoke", Repository: target, Remote: c.Remote, Branch: c.Branch, SpecRef: spec, PlanRef: plan, AcceptanceRefs: []FileReference{spec}}
	contract.Digest, err = TaskContractDigest(contract)
	if err != nil {
		t.Fatal(err)
	}
	priority := 2
	input, err := json.Marshal(TaskBatchInput{SchemaVersion: taskBatchSchemaVersion, ProjectID: c.Identity(), Tasks: []TaskBatchTask{{LocalKey: "smoke", TaskID: 9001, Title: "Orca worker-flow session smoke", Type: "task", Priority: &priority, Contract: contract}}, Dependencies: []TaskBatchEdge{}})
	if err != nil {
		t.Fatal(err)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
	validation, err := commands.ValidateTaskBatch(ctx, c, input)
	if err != nil || !validation.Valid {
		t.Fatalf("isolated task contract is invalid: %+v %v", validation, err)
	}
	if _, err := commands.CreateTaskBatch(ctx, c, input, strings.Repeat("a", 32), validation.AuthorityVersion, "operator"); err != nil {
		t.Fatal(err)
	}
	authority, err := store.nativeAuthority(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err := store.Transact(ctx, func(tx DomainTx) error {
		return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	receipt, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "9001", RequestID: strings.Repeat("b", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.Close() }()
	run := service.find("9001")
	if run == nil || run.RunID != receipt.WorkflowRunID {
		t.Fatal("claimed Orca worker was not reconstructed")
	}
	defer func() {
		if remote {
			return // Preserve uncertain remote resources for explicit inspection.
		}
		if run.Process != nil {
			_, _ = workerOrcaCall(context.Background(), c, "terminal", "close", "--terminal", run.Process.Terminal, "--json")
		}
		if run.WorktreePath != nil && inside(c.RunDir, *run.WorktreePath) {
			_ = exec.Command("git", "-C", c.Repository, "worktree", "remove", "--force", "--", *run.WorktreePath).Run()
		}
	}()
	task := nativeTaskRecord(service.domain.Tasks, "9001")
	if task == nil {
		t.Fatal("isolated native task disappeared")
	}
	if err := service.prepareWorkerFlow(ctx, *task, run); err != nil {
		t.Fatal(err)
	}
	finish := func() {
		t.Helper()
		worker := service.workers[run.RunID]
		if worker == nil {
			t.Fatalf("original Orca worker observer is missing: finish=%q execution=%q reason=%q", run.FinishTaskStatus, run.ExecutionStatus, deref(run.Error))
		}
		for event := range worker.Events {
			if err := service.eventForRun(run, event); err != nil {
				t.Fatal(err)
			}
		}
		workerErr := <-worker.Done
		delete(service.workers, run.RunID)
		if err := service.finishedForRun(run, worker, workerErr); err != nil {
			t.Fatal(err)
		}
		if run.FinishTaskStatus != "waiting_decision" || run.Decision == nil || run.Decision.Status != "pending" {
			status := "missing"
			if run.Decision != nil {
				status = run.Decision.Status
			}
			t.Fatalf("Orca worker did not stop at unified Decision: finish=%q decision=%q error=%q", run.FinishTaskStatus, status, deref(run.Error))
		}
	}
	finish()
	firstProcess, firstSession, firstWorkspace := *run.Process, deref(run.SessionID), deref(run.WorktreePath)
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	service, err = NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	run = service.find("9001")
	if run == nil || run.RunID != receipt.WorkflowRunID || run.Process == nil || *run.Process != firstProcess || deref(run.SessionID) != firstSession || run.Decision == nil || run.Decision.Status != "pending" {
		t.Fatalf("Orca center restart lost the original waiting worker: run=%+v", run)
	}
	service.State.Service.Ready, service.State.Service.Status = true, "running"
	if err := service.publishCheckpoint(); err != nil {
		t.Fatal(err)
	}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: "continue", Note: strings.Repeat("续", 200), AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	feedbackText := workerFeedbackText(answer, run.Decision.Request)
	payload, _ := json.Marshal(answer)
	command, _, err := commands.AnswerDecisionWithID(ctx, strings.Repeat("c", 32), answer.DecisionID, answer.ExpectedDecisionVersion, decisionAnswerKey(answer), answer.AnsweredBy, payload)
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := commands.AcceptAnswer(ctx, command, answer)
	if err != nil {
		t.Fatal(err)
	}
	run.Decision.Version, run.Decision.Status, run.Decision.OptionID, run.Decision.Note, run.Decision.AnsweredBy = record.Version, record.Status, answer.OptionID, answer.Note, answer.AnsweredBy
	run.Decision.AnsweredAt = &record.UpdatedAt
	if err := service.publishCheckpoint(); err != nil {
		t.Fatal(err)
	}
	if err := service.applyWorkerFlowDecision(ctx, run); err != nil {
		t.Fatal(err)
	}
	finish()
	journal := filepath.Join(orcaTurnPath(c, run.RunID, command.CommandID), "events.jsonl")
	events, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(feedbackText))
	if !strings.Contains(string(events), "received_sha256:"+hex.EncodeToString(sum[:])) {
		found := "missing"
		if index := strings.Index(string(events), "received_sha256:"); index >= 0 {
			found = string(events)[index:min(len(events), index+500)]
		}
		t.Fatalf("Orca terminal did not deliver the complete option and Unicode note: expected=%s observed=%s", hex.EncodeToString(sum[:]), found)
	}
	_, execution := service.workflowRecords(run)
	if execution == nil || execution.SessionID != firstSession || execution.TurnNumber != 2 || run.Process == nil || *run.Process != firstProcess || canonical(deref(run.WorktreePath)) != canonical(firstWorkspace) {
		t.Fatalf("Orca feedback replaced worker identity: execution=%+v run=%+v", execution, run)
	}
}
