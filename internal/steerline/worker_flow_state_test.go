package steerline

import (
	"errors"
	"testing"
	"time"
)

func TestWorkerFlowKeepsSessionThroughFeedbackAndPRChanges(t *testing.T) {
	wf := WorkflowRunRecord{WorkflowRunID: "wf-8", TaskID: "8", WorkflowVersion: WorkerFlowVersion, Phase: "queued", Branch: "main"}
	exec := ExecutionRecord{ExecutionID: "exec-8", WorkflowRunID: wf.WorkflowRunID, Status: "pending", Workspace: WorkspaceEvidence{Branch: "codex/task-8"}}
	pr := func(url, state string) PRInfo {
		return PRInfo{URL: url, Repository: "o/r", BaseBranch: "main", HeadRepository: "o/r", HeadBranch: "codex/task-8", HeadRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", State: state}
	}
	begin := func(turn string) {
		t.Helper()
		if err := BeginWorkerTurn(&wf, &exec, "session-8", turn); err != nil {
			t.Fatal(err)
		}
	}
	begin("turn-1")
	if err := BindWorkerPR(&wf, &exec, pr("pr-8", "open"), "pr-8", "o/r", false); err != nil || wf.Phase != "executing" {
		t.Fatalf("early PR interrupted the worker: phase=%s error=%v", wf.Phase, err)
	}
	if err := NoteWorkerQuestion(&wf, &exec, "turn-1"); err != nil || wf.Phase != "waiting_decision" || exec.Status != "active" {
		t.Fatalf("question did not take precedence: phase=%s execution=%s error=%v", wf.Phase, exec.Status, err)
	}
	if err := ContinueWorkerQuestion(&wf, &exec, "turn-1"); err != nil || wf.Phase != "executing" {
		t.Fatalf("same-turn reply replaced the turn: phase=%s error=%v", wf.Phase, err)
	}
	if err := StopWorkerTurn(&wf, &exec, "turn-1", false); err != nil || wf.Phase != "waiting_pr_merge" {
		t.Fatalf("stopped turn did not await PR merge: phase=%s error=%v", wf.Phase, err)
	}
	begin("turn-2") // User asks for more work while the PR remains open.
	if wf.Phase != "executing" || exec.SessionID != "session-8" || exec.TurnNumber != 2 {
		t.Fatalf("feedback changed execution identity: workflow=%+v execution=%+v", wf, exec)
	}
	if err := BindWorkerPR(&wf, &exec, pr("pr-8", "closed_unmerged"), "pr-8", "o/r", false); err != nil {
		t.Fatal(err)
	}
	if err := StopWorkerTurn(&wf, &exec, "turn-2", false); err != nil || wf.Phase != "waiting_decision" {
		t.Fatalf("closed PR did not lead to the unified decision state: phase=%s error=%v", wf.Phase, err)
	}
	if err := BindWorkerPR(&wf, &exec, pr("pr-9", "open"), "pr-9", "o/r", false); err != nil || wf.Phase != "waiting_pr_merge" {
		t.Fatalf("new PR after closure was not associated: phase=%s error=%v", wf.Phase, err)
	}
	begin("turn-3")
	if err := StopWorkerTurn(&wf, &exec, "turn-3", false); err != nil || wf.Phase != "waiting_pr_merge" {
		t.Fatal(err)
	}
	mergedAt := time.Now().UTC()
	delivery := TaskDeliveryRecord{TaskID: "8", SelectedWorkflowRunID: "wf-8", PRURL: "pr-9", MergeCommit: "merge-9", MergedAt: mergedAt}
	if err := MarkWorkerDeliveryCommitted(TaskRecord{TaskID: "8", Status: "closed"}, &wf, delivery); err != nil || wf.Phase != "completed" {
		t.Fatalf("committed delivery did not complete: phase=%s error=%v", wf.Phase, err)
	}
	if err := BeginWorkerTurn(&wf, &exec, "session-8", "turn-4"); err == nil {
		t.Fatal("completed task accepted late feedback")
	}
}

func TestWorkerFlowRejectsReplacementsAndLateTurns(t *testing.T) {
	wf := WorkflowRunRecord{WorkflowRunID: "wf", WorkflowVersion: WorkerFlowVersion, Phase: "queued"}
	exec := ExecutionRecord{ExecutionID: "exec", WorkflowRunID: "wf"}
	if err := BeginWorkerTurn(&wf, &exec, "session", "turn-1"); err != nil {
		t.Fatal(err)
	}
	if err := BeginWorkerTurn(&wf, &exec, "replacement", "turn-2"); err == nil {
		t.Fatal("accepted a replacement worker session")
	}
	if err := StopWorkerTurn(&wf, &exec, "turn-1", false); err != nil {
		t.Fatal(err)
	}
	if err := BeginWorkerTurn(&wf, &exec, "session", "turn-2"); err != nil {
		t.Fatal(err)
	}
	phase := wf.Phase
	if err := NoteWorkerQuestion(&wf, &exec, "turn-1"); !errors.Is(err, ErrWorkerTurnStale) || wf.Phase != phase {
		t.Fatalf("late question changed the newer turn: phase=%s error=%v", wf.Phase, err)
	}
	if err := StopWorkerTurn(&wf, &exec, "turn-1", false); !errors.Is(err, ErrWorkerTurnStale) || wf.Phase != phase {
		t.Fatalf("late stop changed the newer turn: phase=%s error=%v", wf.Phase, err)
	}
	if err := BeginWorkerTurn(&wf, &exec, "session", "turn-3"); err == nil {
		t.Fatal("started a third turn while the second was still active")
	}
}

func TestWorkerFlowDeliveryGateUsesSessionAndPRWithoutStepResults(t *testing.T) {
	task := TaskRecord{TaskID: "8", ContractID: "contract-8"}
	contract := TaskContract{ContractID: "contract-8", Digest: "fixed-digest"}
	wf := WorkflowRunRecord{WorkflowRunID: "wf-8", TaskID: task.TaskID, WorkflowVersion: WorkerFlowVersion, Phase: "waiting_pr_merge", SelectedResult: "exec-8", PRURL: "https://git.example/o/r/pulls/8"}
	exec := ExecutionRecord{ExecutionID: "exec-8", WorkflowRunID: wf.WorkflowRunID, SessionID: "session-8", Workspace: WorkspaceEvidence{Branch: "codex/task-8", Dirty: true}, Status: "idle"}
	publish, err := validateNativeDeliveryGates(task, contract, wf, exec, nil)
	if err != nil || publish.PRURL != wf.PRURL {
		t.Fatalf("workflow requires an obsolete step result: publish=%+v error=%v", publish, err)
	}
	exec.SessionID = ""
	if _, err := validateNativeDeliveryGates(task, contract, wf, exec, nil); err == nil {
		t.Fatal("accepted delivery without a bound worker session")
	}
}
