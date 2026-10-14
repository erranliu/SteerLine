package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	workflow "github.com/erranliu/SteerLine/internal/execution/workflow"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

type stageTestPort struct {
	executionbackend.LifecyclePort
	workspace executionbackend.WorkspaceInspection
	err       error
	evidence  *executionbackend.TurnEvidence
	observed  int
	inspected int
}

type unsupportedStageResolver struct {
	port executionbackend.LifecyclePort
}

func (r unsupportedStageResolver) ResolveLifecycle(executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return r.port, nil
}

func TestWorkerStageUnsupportedBeforeNativeProbe(t *testing.T) {
	s, _, port := workerStageFixture(t)
	workflow := &s.domain.WorkflowRuns[0]
	workflow.FlowRef = "development/v2"
	if err := s.Config.ValidateStoredExecutions(s.domain, s.runtime.Directory); !isNativeErrorCode(err, "unsupported_schema") {
		t.Fatalf("old business flow accepted: %v", err)
	}
	workflow.FlowRef = executionmodel.DevelopmentFlow
	before := recordFingerprint(s.domain)
	// The embedded port is nil: any accidental native preflight, preparation
	// or model operation panics instead of turning this guard into a fake pass.
	err := requireWorkerFlowCapabilitiesWithResolver(context.Background(), s.Config, s.runtime.Directory, s.runtime.ProcessHost, unsupportedStageResolver{port: port})
	blocked, ok := err.(*executionbackend.BackendError)
	if !ok || blocked.Code != executionbackend.BackendUnsupported || recordFingerprint(s.domain) != before {
		t.Fatalf("unsupported final source reached native effects or changed authority: %v", err)
	}
}

func (p *stageTestPort) InspectLifecycleWorkspace(context.Context, executionbackend.LifecycleSnapshot) (executionbackend.WorkspaceInspection, error) {
	p.inspected++
	return p.workspace, p.err
}
func (p *stageTestPort) PolicyCapabilities() executionbackend.LifecyclePolicyCapabilities {
	return executionbackend.LifecyclePolicyCapabilities{ServiceStopBehavior: executionbackend.StopExecutionOnServiceStop}
}

func (p *stageTestPort) ObserveLifecycle(context.Context, executionbackend.LifecycleSnapshot) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	p.observed++
	return executionbackend.LifecycleReceipt{Disposition: executionbackend.OperationConfirmed, TurnEvidence: p.evidence}, nil, nil
}
func (p *stageTestPort) PreparationFacts(context.Context, executionbackend.LifecycleSnapshot) (executionbackend.PreparationFacts, error) {
	return executionbackend.PreparationFacts{Workspace:executionbackend.GateReady,Resource:executionbackend.GateNotApplicable,Session:executionbackend.GateUnverified}, nil
}

func workerStageFixture(t *testing.T, configure ...func(*Config)) (*Service, *reconstructiontypes.RunView, *stageTestPort) {
	t.Helper()
	var setup func(*Config)
	if len(configure) > 0 {
		setup = configure[0]
	}
	s, run := neutralAuditServiceWithConfig(t, setup)
	w, e := s.workflowRecords(run)
	e.SessionID, e.SessionGeneration = "original-session", "original-generation"
	e.OwnerServiceInstanceID = s.State.Service.InstanceID
	e.Workspace.Branch = e.WorkspaceIntent.Branch
	e.Workspace.Worktree = e.WorkspaceIntent.Path
	e.PolicyResources = neutralPolicyReceipt(w.ExecutionProfile.Digest, e.Workspace.Worktree, e)
	w.StepID, w.InputVersion = executionmodel.StepImplement, 1
	command := newID()
	w.StageInputs = []executionmodel.StageInput{{CommandID: command, AttemptID: run.AttemptID, StepID: w.StepID, Version: 1, Prompt: "implement only", TaskFacts: json.RawMessage(`{"task_id":"fixed-task","title":"bound goal"}`)}}
	e.CurrentCommandID, e.CurrentTurnID, e.Status = command, "native-turn-1", "active"
	e.TurnNumber = 1
	w.Status = "running"
	require(t, s.commitWorker(run))
	return s, run, &stageTestPort{workspace: executionbackend.WorkspaceInspection{Repository: s.Config.Repository, Base: e.WorkspaceIntent.BaseCommit, Worktree: e.WorkspaceIntent.Path, Branch: e.Workspace.Branch, Head: strings.Repeat("a", 40)}}
}

func workerStageEvidence(s *Service, run *reconstructiontypes.RunView, body string, success bool) executionbackend.TurnEvidence {
	e := s.executionRecord(run)
	return executionbackend.TurnEvidence{CommandID: e.CurrentCommandID, TurnID: e.CurrentTurnID, SessionID: e.SessionID, Started: true, Completed: true, Succeeded: success, LastMessage: body, FinalSource: "test.native.final_answer", FinalItemID: "item:" + e.CurrentTurnID, FinalVerified: true}
}

func startQueuedStage(t *testing.T, s *Service, run *reconstructiontypes.RunView) {
	t.Helper()
	w, e := s.workflowRecords(run)
	if w.Status != "queued" || e.PendingCommandID == "" {
		t.Fatal("next stage has no committed input intent")
	}
	command := e.PendingCommandID
	e.PendingTurnID = "native:" + command
	require(t, BeginWorkerTurn(w, e, e.SessionID, e.PendingTurnID))
	e.CurrentCommandID, e.PendingCommandID = command, ""
	require(t, s.commitWorker(run))
}

func acceptTestStage(t *testing.T, s *Service, run *reconstructiontypes.RunView, port *stageTestPort, body string, success bool) error {
	t.Helper()
	fixed, err := s.workerRecords(run)
	require(t, err)
	return s.acceptStageTurn(context.Background(), run, port, fixed, workerStageEvidence(s, run, body, success))
}

func TestWorkerStagePublicationMismatchShowsActualBranch(t *testing.T) {
	s, run, port := workerStageFixture(t)
	require(t, acceptTestStage(t, s, run, port, "implemented", true))
	startQueuedStage(t, s, run)
	require(t, acceptTestStage(t, s, run, port, `{"outcome":"done","summary":"implemented"}`, true))
	startQueuedStage(t, s, run)
	fixed, err := s.workerRecords(run)
	require(t, err)
	const url = "https://git.example/org/repo/pulls/1"
	actualBranch := port.workspace.Branch + "-wrong"
	info := executionmodel.PRInfo{URL: url, Repository: fixed.config.PR.Repository, BaseBranch: fixed.workflow.Branch, HeadRepository: fixed.config.PR.Repository, HeadBranch: actualBranch, HeadRevision: port.workspace.Head, State: "open"}
	require(t, s.acceptStagePublication(run, fixed, workflow.Report{Outcome: "done", Summary: "published", Outputs: workflow.Outputs{PRURL: url}}, info))
	w, e := s.workflowRecords(run)
	want := fmt.Sprintf("head_branch: expected=%q, actual=%q", port.workspace.Branch, actualBranch)
	if w.Status != "blocked" || !strings.Contains(w.StageDiagnostic, want) || e.Error == nil || !strings.Contains(*e.Error, want) {
		t.Fatalf("missing blocked branch diagnostic: status=%s diagnostic=%s error=%v", w.Status, w.StageDiagnostic, e.Error)
	}
}

func TestWorkerStageClosedPublicationKeepsPRPolling(t *testing.T) {
	s, run, port := workerStageFixture(t)
	require(t, acceptTestStage(t, s, run, port, "implemented", true))
	startQueuedStage(t, s, run)
	require(t, acceptTestStage(t, s, run, port, `{"outcome":"done","summary":"implemented"}`, true))
	startQueuedStage(t, s, run)
	fixed, err := s.workerRecords(run)
	require(t, err)
	info := executionmodel.PRInfo{URL: "https://git.example/org/repo/pulls/1", Repository: fixed.config.PR.Repository, BaseBranch: fixed.workflow.Branch, HeadRepository: fixed.config.PR.Repository, HeadBranch: port.workspace.Branch, State: "closed_unmerged"}
	require(t, s.stopVerifiedWorkerTurn(run, fixed.workflow, fixed.execution, fixed.execution.CurrentTurnID, "succeeded", false))
	require(t, s.acceptStagePublication(run, fixed, workflow.Report{Outcome: "done", Summary: "published", Outputs: workflow.Outputs{PRURL: info.URL}}, info))
	w := s.workflowRecord(run)
	if w.StepID != executionmodel.StepAwaitMerge || w.Status != "waiting_input" || w.PRURL != info.URL || run.Decision == nil || !workerContinuationPrompt(run.Decision.Request) {
		t.Fatalf("closed bound PR lost polling or usable continuation: step=%s status=%s pr=%s decision=%s diagnostic=%s", w.StepID, w.Status, w.PRURL, mustJSON(run.Decision), w.StageDiagnostic)
	}
}

func TestWorkerStageTransitions(t *testing.T) {
	s, run, port := workerStageFixture(t)
	var queries atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"html_url": server.URL + "/org/repo/pulls/1", "number": 1, "state": "open", "merged": false, "base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "org/repo"}}, "head": map[string]any{"ref": port.workspace.Branch, "sha": port.workspace.Head, "repo": map[string]string{"full_name": "org/repo"}}})
	}))
	defer server.Close()
	initial := workerStageEvidence(s, run, "ordinary final; no JSON", true)
	require(t, acceptTestStage(t, s, run, port, initial.LastMessage, true))
	w := s.workflowRecord(run)
	if w.StepID != executionmodel.StepReport || len(w.StageResults) != 1 || w.StageInputs[1].SourceCommandID != initial.CommandID || w.StageInputs[1].SourceTurnID != initial.TurnID {
		t.Fatal("source execution did not bind its report")
	}
	if !strings.Contains(w.StageInputs[1].ReportToolPolicy, "business_read_only_is_prompt_policy") {
		t.Fatal("missing honest native permission guarantee")
	}
	if strings.Contains(w.StageInputs[1].Prompt, "task_id") || strings.Contains(w.StageInputs[1].Prompt, initial.LastMessage) {
		t.Fatal("report repeats task identity or the preceding reply")
	}
	startQueuedStage(t, s, run)
	require(t, acceptTestStage(t, s, run, port, `{"outcome":"done","outputs":{},"summary":"implemented"}`, true))
	if s.workflowRecord(run).StepID != executionmodel.StepPublish {
		t.Fatal("report did not queue publication")
	}
	startQueuedStage(t, s, run)
	fixed, err := s.workerRecords(run)
	require(t, err)
	fixed.config.PR.Kind, fixed.config.PR.BaseURL, fixed.config.PR.Repository = "gitea", server.URL, "org/repo"
	url := server.URL + "/org/repo/pulls/1"
	require(t, s.acceptStageTurn(context.Background(), run, port, fixed, workerStageEvidence(s, run, `{"outcome":"done","outputs":{"pr_url":"`+url+`"},"summary":"published"}`, true)))
	w = s.workflowRecord(run)
	if w.Status != "waiting_external" || w.StepID != executionmodel.StepAwaitMerge || len(w.StageInputs) != 3 || len(w.StageResults) != 3 || s.executionRecord(run).PendingCommandID != "" {
		t.Fatal("publish added a report or failed to wait externally")
	}
	if queries.Load() != 1 || w.PRCandidateURL != url || w.PRURL != url {
		t.Fatal("publication did not query and retain the structured PR URL")
	}
	fingerprint := recordFingerprint(*w)
	require(t, s.acceptStageTurn(context.Background(), run, port, fixed, initial))
	initial.LastMessage = "conflicting late result"
	if err := s.acceptStageTurn(context.Background(), run, port, fixed, initial); err == nil {
		t.Fatal("conflicting old source accepted")
	}
	if recordFingerprint(*s.workflowRecord(run)) != fingerprint {
		t.Fatal("late result changed committed state")
	}
}

func TestWorkerStageResultBinding(t *testing.T) {
	for _, change := range []string{"command", "turn", "session", "input_version", "final_source"} {
		t.Run(change, func(t *testing.T) {
			s, run, port := workerStageFixture(t)
			require(t, acceptTestStage(t, s, run, port, "free implementation final", true))
			startQueuedStage(t, s, run)
			fixed, err := s.workerRecords(run)
			require(t, err)
			evidence := workerStageEvidence(s, run, `{"outcome":"done","outputs":{},"summary":"implemented"}`, true)
			switch change {
			case "command":
				evidence.CommandID = "old"
			case "turn":
				evidence.TurnID = "old"
			case "session":
				evidence.SessionID = "replacement"
			case "input_version":
				fixed.workflow.InputVersion++
			case "final_source":
				evidence.FinalVerified = false
			}
			err = s.acceptStageTurn(context.Background(), run, port, fixed, evidence)
			if change != "final_source" && err == nil {
				t.Fatal("stale result accepted")
			}
			if s.workflowRecord(run).StepID == executionmodel.StepPublish {
				t.Fatal("invalid source promoted publication")
			}
		})
	}
}

func TestWorkerStageReportRound(t *testing.T) {
	for _, scenario := range []string{"no_snapshot", "unavailable_inspection", "source_failed", "report_invalid", "head_advanced", "unknown", "needs_input", "publish_invalid", "publish_partial"} {
		t.Run(scenario, func(t *testing.T) {
			s, run, port := workerStageFixture(t)
			require(t, acceptTestStage(t, s, run, port, "implementation facts", scenario != "source_failed"))
			if scenario == "source_failed" {
				w := s.workflowRecord(run)
				if w.Status != "waiting_input" || len(w.StageResults) != 1 || w.StageResults[0].Succeeded || len(w.StageInputs) != 1 || run.Decision == nil {
					t.Fatal("known source failure was overwritten or started another report")
				}
				return
			}
			startQueuedStage(t, s, run)
			body := `{"outcome":"done","outputs":{},"summary":"reported"}`
			switch scenario {
			case "no_snapshot":
				body = `{"outcome":"done","summary":"reported"}`
			case "unavailable_inspection":
				port.err = errors.New("stage inspection unavailable")
			case "report_invalid":
				body = "not JSON"
			case "head_advanced":
				port.workspace.Head, port.workspace.Dirty = strings.Repeat("e", 40), true
			case "needs_input":
				body = `{"outcome":"needs_input","summary":"decision needed","question":"Which original target should be completed?"}`
			case "unknown":
				body = `{"outcome":"blocked","summary":"unknown","blocker":{"category":"unknown_external","action":"original command","error":"response lost"}}`
			}
			require(t, acceptTestStage(t, s, run, port, body, true))
			w := s.workflowRecord(run)
			switch scenario {
			case "no_snapshot", "unavailable_inspection", "head_advanced":
				if w.StepID != executionmodel.StepPublish || w.Status != "queued" || port.inspected != 0 {
					t.Fatal("report still depends on workspace observations")
				}
			case "source_failed":
				if w.Status != "blocked" || len(w.StageResults) != 2 || w.StageResults[0].Succeeded {
					t.Fatal("report overwrote source failure")
				}
			case "needs_input":
				if w.Status != "waiting_input" || w.StepID != executionmodel.StepReport || run.Decision == nil || run.Decision.Request.Question != "Which original target should be completed?" {
					t.Fatal("structured question did not bind the original report step")
				}
			case "unknown":
				if w.Status != "reconciling" || len(w.StageInputs) != 2 {
					t.Fatal("unknown report was retransmitted")
				}
			case "report_invalid":
				if w.ReportRetry != 1 || len(w.StageInputs) != 3 || w.StageInputs[1].SourceCommandID != w.StageInputs[2].SourceCommandID {
					t.Fatal("report retry reran implementation")
				}
				startQueuedStage(t, s, run)
				require(t, acceptTestStage(t, s, run, port, "invalid again", true))
				if s.workflowRecord(run).Status != "blocked" || len(s.workflowRecord(run).StageInputs) != 3 {
					t.Fatal("report retry was not bounded")
				}
			case "publish_invalid", "publish_partial":
				startQueuedStage(t, s, run)
				body = "invalid publication JSON"
				if scenario == "publish_partial" {
					body = `{"outcome":"blocked","outputs":{"branch":"` + port.workspace.Branch + `"},"summary":"push succeeded; PR failed","blocker":{"category":"failure","action":"create PR","error":"rejected","known":"branch pushed"}}`
				}
				require(t, acceptTestStage(t, s, run, port, body, true))
				expectedStep, expectedStatus, inputs := executionmodel.StepPublish, "blocked", 3
				if scenario == "publish_invalid" {
					expectedStep, expectedStatus, inputs = executionmodel.StepPublishReport, "queued", 4
				}
				if s.workflowRecord(run).Status != expectedStatus || s.workflowRecord(run).StepID != expectedStep || len(s.workflowRecord(run).StageInputs) != inputs {
					t.Fatal("publish reran side effects to repair JSON")
				}
			}
		})
	}
}

func TestWorkerStageAtomicCommitFailure(t *testing.T) {
	s, run, port := workerStageFixture(t)
	before, err := copyExecutionSnapshot(s.committed)
	require(t, err)
	_, err = testSQLite(s).db.ExecContext(context.Background(), `CREATE TRIGGER fail_stage_checkpoint BEFORE UPDATE ON projection BEGIN SELECT RAISE(ABORT,'stage checkpoint unavailable'); END`)
	require(t, err)
	if err := acceptTestStage(t, s, run, port, "completed implementation", true); err == nil {
		t.Fatal("injected transaction failure did not fail")
	}
	after, err := readServiceSnapshot(context.Background(), s.domainStore, s.Config.Identity())
	require(t, err)
	if recordFingerprint(before) != recordFingerprint(after) || len(s.workflowRecord(run).StageResults) != 0 || len(s.workflowRecord(run).StageInputs) != 1 {
		t.Fatal("result/transition/next intent committed partially")
	}
}

func TestWorkerStageReopenedSourceQueuesOnlyReport(t *testing.T) {
	s, run, port := workerStageFixture(t)
	e := s.executionRecord(run)
	e.RecoveryRequired, e.RecoveryState, e.RecoveryVersion = true, "completed", 1
	e.OwnerServiceInstanceID = "prior-service"
	e.RecoveryMarkedAt = ptr(now())
	require(t, s.commitWorker(run))
	require(t, acceptTestStage(t, s, run, port, "original implementation completed before restart", true))
	w := s.workflowRecord(run)
	if w.StepID != executionmodel.StepReport || w.Status != "waiting_input" || len(w.StageInputs) != 2 || len(w.StageResults) != 1 || w.StageInputs[1].SourceCommandID != w.StageInputs[0].CommandID || s.executionRecord(run).PendingCommandID != w.StageInputs[1].CommandID {
		t.Fatal("reopened source recreated implementation or automatically sent a report")
	}
	if run.Decision == nil || run.Decision.Status != "pending" {
		t.Fatal("reopened queued report lacks a fresh manual gate")
	}
}

func TestWorkerStageStrictResult(t *testing.T) {
	for _, body := range []string{
		`{"outcome":"blocked","Outcome":"done","summary":"x"}`,
		`{"outcome":"done","summary":"x","outputs":{"pr_url":"https://example.com/pull/1","PR_URL":"https://example.com/pull/2"}}`,
	} {
		if _, err := workflow.DecodeReport(body, executionmodel.StepPublish); err == nil {
			t.Fatalf("case-insensitive duplicate changed result %q", body)
		}
	}
	for _, body := range []string{`{"outcome":"blocked","outcome":"done","summary":"x"}`, "```json\n{}\n```", `{} {}`, `{"outcome":"done","summary":"x","outputs":{"pr_url":1}}`, `{"outcome":"done","summary":"x","outputs":{"pr_url":"https://example.com/pull/1","pr_url":"https://example.com/pull/2"}}`} {
		if _, err := workflow.DecodeReport(body, executionmodel.StepReport); err == nil {
			t.Fatalf("accepted invalid result %q", body)
		}
	}
}

func TestWorkerStagePresentationDoesNotBlockResult(t *testing.T) {
	for _, body := range []string{
		"```json\r\n{\"outcome\":\"done\",\"summary\":\"ready\"}\r\n```",
		"```\n{\"outcome\":\"done\",\"summary\":\"ready\"}\n```",
		`{"outcome":"done","summary":"ready","status":"completed","outputs":{"candidate_sha":"extra"}}`,
		`{"outcome":"done","summary":"ready","metadata":{"outcome":"note","Outcome":"another note"}}`,
		`{"outcome":" done ","summary":" ready ","question":" "}`,
	} {
		report, err := workflow.DecodeReport(body, executionmodel.StepReport)
		if err != nil || report.Outcome != "done" || report.Summary != "ready" {
			t.Fatalf("usable report rejected: %q, %v", body, err)
		}
	}
}

func TestWorkerStagePublishReport(t *testing.T) {
	for _, scenario := range []string{"success", "missing", "missing_pr_url", "invalid_report", "source_failed", "unknown", "head_advanced", "no_head", "wrong_pr", "query_unavailable", "recovery"} {
		t.Run(scenario, func(t *testing.T) {
			s, run, port := workerStageFixture(t)
			require(t, acceptTestStage(t, s, run, port, "implementation facts", true))
			startQueuedStage(t, s, run)
			require(t, acceptTestStage(t, s, run, port, `{"outcome":"done","outputs":{},"summary":"implemented"}`, true))
			startQueuedStage(t, s, run)
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "query_unavailable" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				head, branch := port.workspace.Head, port.workspace.Branch
				if scenario == "no_head" {
					head = ""
				}
				if scenario == "wrong_pr" {
					branch += "-wrong"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"html_url": server.URL + "/org/repo/pulls/1", "number": 1, "state": "open", "merged": false, "base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "org/repo"}}, "head": map[string]any{"ref": branch, "sha": head, "repo": map[string]string{"full_name": "org/repo"}}})
			}))
			defer server.Close()
			url := server.URL + "/org/repo/pulls/1"
			original := workerStageEvidence(s, run, "Published candidate; PR: "+url, scenario != "source_failed")
			if scenario == "source_failed" {
				original.Failure = "original command failed"
			}
			if scenario == "missing" {
				original.LastMessage = ""
			}
			if scenario == "missing_pr_url" {
				original.LastMessage = `{"outcome":"done","summary":"Published PR: ` + url + `"}`
			}
			if scenario == "recovery" {
				e := s.executionRecord(run)
				e.RecoveryRequired, e.RecoveryState, e.RecoveryVersion = true, "completed", 1
				e.OwnerServiceInstanceID = "prior-service"
				e.RecoveryMarkedAt = ptr(now())
			}
			fixed, err := s.workerRecords(run)
			require(t, err)
			require(t, s.acceptStageTurn(context.Background(), run, port, fixed, original))
			w := s.workflowRecord(run)
			input := w.StageInputs[3]
			if w.StepID != executionmodel.StepPublishReport || len(w.StageInputs) != 4 || len(w.StageResults) != 3 || input.SourceCommandID != original.CommandID || input.SourceTurnID != original.TurnID || !strings.HasPrefix(input.Prompt, workflow.PublishReportPrompt) || !strings.Contains(input.ReportToolPolicy, "business_read_only_is_prompt_policy") {
				t.Fatal("publication report lost its source or formatting instruction")
			}
			if scenario == "recovery" {
				if w.Status != "waiting_input" || run.Decision == nil {
					t.Fatal("recovered publication automatically dispatched report")
				}
				return
			}
			// Duplicate original results cannot queue a second report.
			require(t, s.acceptStageTurn(context.Background(), run, port, fixed, original))
			if len(s.workflowRecord(run).StageInputs) != 4 {
				t.Fatal("original result was replayed")
			}
			startQueuedStage(t, s, run)
			body := `{"outcome":"done","outputs":{"pr_url":"` + url + `"},"summary":"publication reported"}`
			if scenario == "invalid_report" {
				body = "invalid again"
			}
			if scenario == "unknown" {
				body = `{"outcome":"blocked","outputs":{"pr_url":"` + url + `"},"summary":"result unknown","blocker":{"category":"unknown_external","action":"publish","error":"response lost"}}`
			}
			if scenario == "head_advanced" {
				port.workspace.Head, port.workspace.Dirty = strings.Repeat("e", 40), true
			}
			fixed, err = s.workerRecords(run)
			require(t, err)
			fixed.config.PR.Kind, fixed.config.PR.BaseURL, fixed.config.PR.Repository = "gitea", server.URL, "org/repo"
			require(t, s.acceptStageTurn(context.Background(), run, port, fixed, workerStageEvidence(s, run, body, true)))
			w = s.workflowRecord(run)
			want := "waiting_external"
			switch scenario {
			case "invalid_report", "wrong_pr":
				want = "blocked"
			case "source_failed", "unknown", "query_unavailable":
				want = "reconciling"
			}
			if w.Status != want || len(w.StageInputs) != 4 || len(w.StageResults) != 4 || s.executionRecord(run).PendingCommandID != "" {
				t.Fatalf("report was retried or accepted incorrectly: %s", w.Status)
			}
			if want == "waiting_external" && (w.StepID != executionmodel.StepAwaitMerge || w.PRURL != url) {
				t.Fatal("report did not verify and bind PR")
			}
			if scenario == "source_failed" && (w.StageResults[2].Succeeded || w.StageResults[2].Failure != original.Failure || w.PRURL != "") {
				t.Fatal("report upgraded original publication failure")
			}
		})
	}
}
