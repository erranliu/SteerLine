package execution

import (
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestWorkerFlowKeepsSessionThroughFeedbackAndPRChanges(t *testing.T) {
	wf := executionmodel.WorkflowRunRecord{WorkflowRunID: "wf-8", TaskID: "8", WorkflowVersion: executionmodel.Version, Status: "queued", Branch: "main"}
	exec := executionmodel.ExecutionRecord{ExecutionID: "exec-8", WorkflowRunID: wf.WorkflowRunID, Status: "pending", Workspace: executionmodel.WorkspaceEvidence{Branch: "fixture/task-8"}}
	pr := func(url, state string) executionmodel.PRInfo {
		return executionmodel.PRInfo{URL: url, Repository: "o/r", BaseBranch: "main", HeadRepository: "o/r", HeadBranch: "fixture/task-8", HeadRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", State: state}
	}
	begin := func(turn string) {
		t.Helper()
		wf.Status = "queued"
		if err := BeginWorkerTurn(&wf, &exec, "session-8", turn); err != nil {
			t.Fatal(err)
		}
	}
	begin("turn-1")
	if err := BindWorkerPR(&wf, &exec, pr("pr-8", "open"), "pr-8", "o/r", false); err != nil || wf.Status != "running" {
		t.Fatalf("early PR interrupted the worker: phase=%s error=%v", wf.Status, err)
	}
	if err := NoteWorkerQuestion(&wf, &exec, "turn-1"); err != nil || wf.Status != "waiting_input" || exec.Status != "active" {
		t.Fatalf("question did not take precedence: phase=%s execution=%s error=%v", wf.Status, exec.Status, err)
	}
	if err := ContinueWorkerQuestion(&wf, &exec, "turn-1"); err != nil || wf.Status != "running" {
		t.Fatalf("same-turn reply replaced the turn: phase=%s error=%v", wf.Status, err)
	}
	if err := StopWorkerTurn(&wf, &exec, "turn-1", false); err != nil || wf.Status != "blocked" {
		t.Fatalf("stopped turn did not await PR merge: phase=%s error=%v", wf.Status, err)
	}
	begin("turn-2")
	if wf.Status != "running" || exec.TurnNumber != 2 {
		t.Fatalf("feedback changed execution identity: workflow=%+v execution=%+v", wf, exec)
	}
	if err := BindWorkerPR(&wf, &exec, pr("pr-8", "closed_unmerged"), "pr-8", "o/r", false); err != nil {
		t.Fatal(err)
	}
	if err := StopWorkerTurn(&wf, &exec, "turn-2", false); err != nil || wf.Status != "blocked" {
		t.Fatalf("closed PR did not lead to the unified decision state: phase=%s error=%v", wf.Status, err)
	}
	if err := BindWorkerPR(&wf, &exec, pr("pr-9", "open"), "pr-9", "o/r", false); err != nil || wf.Status != "blocked" {
		t.Fatalf("new PR after closure was not associated: phase=%s error=%v", wf.Status, err)
	}
	begin("turn-3")
	if err := StopWorkerTurn(&wf, &exec, "turn-3", false); err != nil || wf.Status != "blocked" {
		t.Fatal(err)
	}
	mergedAt := time.Now().UTC()
	delivery := executionmodel.TaskDeliveryRecord{TaskID: "8", SelectedWorkflowRunID: "wf-8", PRURL: "pr-9", MergeCommit: "merge-9", MergedAt: mergedAt}
	if err := reconstructiontypes.MarkWorkerDeliveryCommitted(taskmodel.TaskRecord{TaskID: "8", Status: "closed"}, &wf, delivery); err != nil || wf.Status != "completed" {
		t.Fatalf("committed delivery did not complete: phase=%s error=%v", wf.Status, err)
	}
	if err := BeginWorkerTurn(&wf, &exec, "session-8", "turn-4"); err == nil {
		t.Fatal("completed task accepted late feedback")
	}
}

func TestWorkerFlowRejectsReplacementsAndLateTurns(t *testing.T) {
	wf := executionmodel.WorkflowRunRecord{WorkflowRunID: "wf", WorkflowVersion: executionmodel.Version, Status: "queued"}
	exec := executionmodel.ExecutionRecord{ExecutionID: "exec", WorkflowRunID: "wf"}
	if err := BeginWorkerTurn(&wf, &exec, "session", "turn-1"); err != nil {
		t.Fatal(err)
	}
	if err := BeginWorkerTurn(&wf, &exec, "replacement", "turn-2"); err == nil {
		t.Fatal("accepted a replacement worker session")
	}
	if err := StopWorkerTurn(&wf, &exec, "turn-1", false); err != nil {
		t.Fatal(err)
	}
	wf.Status = "queued"
	if err := BeginWorkerTurn(&wf, &exec, "session", "turn-2"); err != nil {
		t.Fatal(err)
	}
	phase := wf.Status
	if err := NoteWorkerQuestion(&wf, &exec, "turn-1"); !errors.Is(err, executionmodel.ErrTurnStale) || wf.Status != phase {
		t.Fatalf("late question changed the newer turn: phase=%s error=%v", wf.Status, err)
	}
	if err := StopWorkerTurn(&wf, &exec, "turn-1", false); !errors.Is(err, executionmodel.ErrTurnStale) || wf.Status != phase {
		t.Fatalf("late stop changed the newer turn: phase=%s error=%v", wf.Status, err)
	}
	if err := BeginWorkerTurn(&wf, &exec, "session", "turn-3"); err == nil {
		t.Fatal("started a third turn while the second was still active")
	}
}

func TestWorkerFeedbackKeepsOptionAndAdditionalText(t *testing.T) {
	request := executionmodel.DecisionRequest{Options: []executionbackend.DecisionOption{{ID: "continue", Label: "Continue current work"}}, AllowFreeText: true}
	answer := executionmodel.DecisionAnswer{OptionID: "continue", Note: "Keep the partially committed changes and fix the compiler error."}
	message := workerFeedbackText(answer, request)
	if message != "Selected option: Continue current work\n\nAdditional instructions:\nKeep the partially committed changes and fix the compiler error." {
		t.Fatalf("feedback lost the option or note: %q", message)
	}
	if text := workerFeedbackText(executionmodel.DecisionAnswer{Note: "Pure text"}, request); text != "Pure text" {
		t.Fatalf("pure text feedback changed: %q", text)
	}
}
