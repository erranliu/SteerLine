package execution

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestWorkerStageUnverifiedImplementationEntersReport(t *testing.T) {
	s, run, port := workerStageFixture(t)
	fixed, err := s.workerFlowContext(run)
	require(t, err)
	evidence := workerStageEvidence(s, run, "unverified projection with plausible source labels", true)
	evidence.FinalVerified = false
	evidence.FinalSource, evidence.FinalItemID, evidence.LastMessage = "", "", ""
	evidence.FinalDiagnostic = "implementation reply unavailable"
	require(t, s.acceptStageTurn(context.Background(), run, port, fixed, evidence))
	w := s.workflowRecord(run)
	if w.StepID != executionmodel.StepReport || w.Status != "queued" || len(w.StageResults) != 1 || len(w.StageInputs) != 2 {
		t.Fatal("stopped implementation without full reply did not enter report")
	}
	startQueuedStage(t, s, run)
	require(t, acceptTestStage(t, s, run, port, `{"outcome":"done","outputs":{},"summary":"reported from supplied facts"}`, true))
	if w.StepID != executionmodel.StepPublish || w.StageResults[0].FinalVerified {
		t.Fatal("verified report did not advance, or rewrote original missing reply")
	}
}

func TestWorkerStageTransientFactsReconcileOriginalTurn(t *testing.T) {
	for _, problem := range []string{"final"} {
		t.Run(problem, func(t *testing.T) {
			s, run, port := workerStageFixture(t)
			s.lifecycleResolver = unsupportedStageResolver{port: port}
			require(t, acceptTestStage(t, s, run, port, "verified execution", true))
			startQueuedStage(t, s, run)
			fixed, err := s.workerFlowContext(run)
			require(t, err)
			evidence := workerStageEvidence(s, run, `{"outcome":"done","outputs":{},"summary":"reported"}`, true)
			evidence.FinalVerified = false
			require(t, s.acceptStageTurn(context.Background(), run, port, fixed, evidence))
			w := s.workflowRecord(run)
			if w.Status != "reconciling" || len(w.StageResults) != 1 || len(w.StageInputs) != 2 || w.ReportRetry != 0 {
				t.Fatal("unavailable facts sealed a result or sent another report")
			}
			port.err = nil
			evidence.FinalVerified = true
			port.evidence = &evidence
			require(t, s.reconcileStageFacts(context.Background(), run))
			w = s.workflowRecord(run)
			if w.StepID != executionmodel.StepPublish || w.Status != "queued" || len(w.StageResults) != 2 || len(w.StageInputs) != 3 || port.observed != 1 || s.executionRecord(run).TurnNumber != 2 {
				t.Fatal("original stopped result did not reconcile without a new model turn")
			}
		})
	}
}

func TestWorkerStagePublicationReadRetrySettlesOpenPR(t *testing.T) {
	testWorkerStagePublicationReadRetry(t, false)
}

func TestWorkerStagePublicationReportReadRetrySettlesOpenPR(t *testing.T) {
	testWorkerStagePublicationReadRetry(t, true)
}

func testWorkerStagePublicationReadRetry(t *testing.T, report bool) {
	queryFile := filepath.Join(t.TempDir(), "query.json")
	t.Setenv("STEERLINE_STAGE_QUERY_FILE", queryFile)
	const url = "https://github.com/owner/repo/pull/84"
	s, run, port := workerStageFixture(t, func(c *Config) {
		c.PR.Kind, c.PR.Repository = "command", "owner/repo"
		c.PR.Query = executionbackend.Command{Executable: os.Args[0], Args: []string{"-test.run=^TestWorkerStageProviderCommandFixture$"}}
	})
	s.lifecycleResolver = unsupportedStageResolver{port: port}
	require(t, acceptTestStage(t, s, run, port, "verified execution", true))
	startQueuedStage(t, s, run)
	require(t, acceptTestStage(t, s, run, port, `{"outcome":"done","outputs":{},"summary":"reported"}`, true))
	startQueuedStage(t, s, run)
	turns := 3
	if report {
		require(t, acceptTestStage(t, s, run, port, "Published PR: "+url, true))
		startQueuedStage(t, s, run)
		turns = 4
	}
	require(t, acceptTestStage(t, s, run, port, `{"outcome":"done","outputs":{"pr_url":"`+url+`"},"summary":"published"}`, true))
	if s.workflowRecord(run).Status != "reconciling" {
		t.Fatal("provider failure was ignored")
	}
	proof, err := json.Marshal(executionmodel.PRInfo{URL: url, Repository: "owner/repo", BaseBranch: "main", HeadRepository: "owner/repo", HeadBranch: port.workspace.Branch, HeadRevision: port.workspace.Head, State: "open"})
	require(t, err)
	require(t, os.WriteFile(queryFile, proof, 0600))
	require(t, s.reconcileStageFacts(context.Background(), run))
	w := s.workflowRecord(run)
	if w.StepID != executionmodel.StepAwaitMerge || w.Status != "waiting_external" || w.PRURL == "" || len(w.StageInputs) != turns || len(w.StageResults) != turns || s.executionRecord(run).TurnNumber != uint64(turns) {
		t.Fatalf("original successful publication remained stuck or was reexecuted: step=%s status=%s diagnostic=%s query=%v", w.StepID, w.Status, w.StageDiagnostic, w.PRQueryError)
	}
}

func TestWorkerStageProviderCommandFixture(t *testing.T) {
	path := os.Getenv("STEERLINE_STAGE_QUERY_FILE")
	if path == "" {
		return
	}
	body, err := os.ReadFile(path)
	if err != nil {
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(body)
	os.Exit(0)
}

func TestWorkerStageFeedbackUsesExactFrozenNativePrompt(t *testing.T) {
	s, run, port := workerStageFixture(t)
	e := s.executionRecord(run)
	if output, err := exec.Command("git", "-C", s.Config.Repository, "worktree", "add", "-b", e.Workspace.Branch, e.Workspace.Worktree, e.Workspace.Base).CombinedOutput(); err != nil {
		t.Fatalf("prepare owned fixture worktree: %v %s", err, output)
	}
	s.lifecycleResolver = unsupportedStageResolver{port: port}
	require(t, acceptTestStage(t, s, run, port, "verified execution", true))
	startQueuedStage(t, s, run)
	require(t, acceptTestStage(t, s, run, port, `{"outcome":"done","outputs":{},"summary":"reported"}`, true))
	command := newID()
	err := s.startStageFeedback(context.Background(), run, executionmodel.DecisionAnswer{OptionID: "continue"}, command, executionmodel.DecisionRequest{Question: "continue publication"})
	if err == nil {
		t.Fatal("fixture planner unexpectedly started a native turn")
	}
	w, e := s.workflowRecords(run)
	input := stageInput(w, command)
	operation := s.operationByKey("worker-input-value:" + e.ExecutionID + ":" + command)
	if input == nil || operation == nil {
		t.Fatalf("frozen feedback input was not saved: %v", err)
	}
	var native executionbackend.TurnInput
	require(t, json.Unmarshal(operation.Request, &native))
	if native.Prompt != input.Prompt || strings.Contains(input.Prompt, "Fixed publication target:") || strings.Contains(input.Prompt, "Fixed workspace:") {
		t.Fatal("native feedback prompt changed after freezing or duplicated fixed instructions")
	}
}
