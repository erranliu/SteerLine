package execution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

type stageTestPort struct {
	executionbackend.LifecyclePort
	workspace executionbackend.WorkspaceInspection
	err       error
}

type unsupportedStageResolver struct {
	port executionbackend.LifecyclePort
}

func (r unsupportedStageResolver) ResolveLifecycle(executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return r.port, nil
}

func TestWorkerStageUnsupportedBeforeNativeProbe(t *testing.T) {
	s, _, port := workerStageFixture(t)
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
	return p.workspace, p.err
}
func (p *stageTestPort) PolicyCapabilities() executionbackend.LifecyclePolicyCapabilities {
	return executionbackend.LifecyclePolicyCapabilities{}
}

func workerStageFixture(t *testing.T) (*Service, *reconstructiontypes.RunView, *stageTestPort) {
	t.Helper()
	s, run := neutralAuditService(t)
	w, e := s.workflowRecords(run)
	e.SessionID, e.SessionGeneration = "original-session", "original-generation"
	e.OwnerServiceInstanceID = s.State.Service.InstanceID
	e.Workspace.Branch = e.WorkspaceIntent.Branch
	e.Workspace.Worktree = e.WorkspaceIntent.Path
	e.PolicyResources = neutralPolicyReceipt(w.ExecutionProfile.Digest, e.Workspace.Worktree, e)
	w.StepID, w.InputVersion = executionmodel.StepImplement, 1
	command := newID()
	w.StageInputs = []executionmodel.StageInput{{CommandID: command, AttemptID: run.AttemptID, StepID: w.StepID, Version: 1, Prompt: "implement only"}}
	e.CurrentCommandID, e.CurrentTurnID, e.Status = command, "native-turn-1", "active"
	e.TurnNumber = 1
	w.Status = "running"
	require(t, s.commitWorker(run))
	return s, run, &stageTestPort{workspace: executionbackend.WorkspaceInspection{Repository: s.Config.Repository, Base: e.WorkspaceIntent.BaseCommit, Worktree: e.WorkspaceIntent.Path, Branch: e.Workspace.Branch, Head: strings.Repeat("a", 40), ContentDigest: strings.Repeat("d", 64)}}
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
	fixed, err := s.workerFlowContext(run)
	require(t, err)
	return s.acceptStageTurn(context.Background(), run, port, fixed, workerStageEvidence(s, run, body, success))
}

func TestWorkerStageTransitions(t *testing.T) {
	s, run, port := workerStageFixture(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"html_url": server.URL + "/org/repo/pulls/1", "number": 1, "state": "open", "merged": false, "base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "org/repo"}}, "head": map[string]any{"ref": port.workspace.Branch, "sha": port.workspace.Head, "repo": map[string]string{"full_name": "org/repo"}}})
	}))
	defer server.Close()
	initial := workerStageEvidence(s, run, "ordinary final; no JSON", true)
	require(t, acceptTestStage(t, s, run, port, initial.LastMessage, true))
	w := s.workflowRecord(run)
	if w.StepID != executionmodel.StepReport || len(w.StageResults) != 1 || w.StageInputs[1].SourceCommandID != initial.CommandID || w.StageInputs[1].SourceTurnID != initial.TurnID {
		t.Fatal("source execution did not bind its report")
	}
	if !strings.Contains(w.StageInputs[1].ReportToolPolicy, "fact_comparison_only") {
		t.Fatal("missing honest native permission guarantee")
	}
	if !strings.Contains(w.StageInputs[1].Prompt, `"original_goal":"implement only"`) || !strings.Contains(w.StageInputs[1].Prompt, `"goal_truncated":false`) {
		t.Fatal("report lost the frozen original stage goal or its evidence boundary")
	}
	startQueuedStage(t, s, run)
	require(t, acceptTestStage(t, s, run, port, `{"outcome":"done","outputs":{"candidate_sha":"`+port.workspace.Head+`"},"summary":"implemented"}`, true))
	if s.workflowRecord(run).StepID != executionmodel.StepPublish || s.workflowRecord(run).CandidateSHA != port.workspace.Head {
		t.Fatal("report did not freeze publication candidate")
	}
	startQueuedStage(t, s, run)
	fixed, err := s.workerFlowContext(run)
	require(t, err)
	fixed.config.PR.Kind, fixed.config.PR.BaseURL, fixed.config.PR.Repository = "gitea", server.URL, "org/repo"
	url := server.URL + "/org/repo/pulls/1"
	require(t, s.acceptStageTurn(context.Background(), run, port, fixed, workerStageEvidence(s, run, `{"outcome":"done","outputs":{"pr_url":"`+url+`"},"summary":"published"}`, true)))
	w = s.workflowRecord(run)
	if w.Status != "waiting_external" || w.StepID != executionmodel.StepAwaitMerge || len(w.StageInputs) != 3 || len(w.StageResults) != 3 || s.executionRecord(run).PendingCommandID != "" {
		t.Fatal("publish added a report or failed to wait externally")
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
			fixed, err := s.workerFlowContext(run)
			require(t, err)
			evidence := workerStageEvidence(s, run, `{"outcome":"done","outputs":{"candidate_sha":"`+port.workspace.Head+`"},"summary":"implemented"}`, true)
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
	for _, scenario := range []string{"source_failed", "report_invalid", "report_write", "unknown", "needs_input", "publish_invalid", "publish_partial"} {
		t.Run(scenario, func(t *testing.T) {
			s, run, port := workerStageFixture(t)
			require(t, acceptTestStage(t, s, run, port, "implementation facts", scenario != "source_failed"))
			startQueuedStage(t, s, run)
			body := `{"outcome":"done","outputs":{"candidate_sha":"` + port.workspace.Head + `"},"summary":"reported"}`
			switch scenario {
			case "report_invalid":
				body = "not JSON"
			case "report_write":
				port.workspace.ContentDigest = strings.Repeat("e", 64)
			case "needs_input":
				body = `{"outcome":"needs_input","summary":"decision needed","question":"Which original target should be completed?"}`
			case "unknown":
				body = `{"outcome":"blocked","summary":"unknown","blocker":{"category":"unknown_external","action":"original command","error":"response lost"}}`
			}
			require(t, acceptTestStage(t, s, run, port, body, true))
			w := s.workflowRecord(run)
			switch scenario {
			case "source_failed":
				if w.Status != "blocked" || len(w.StageResults) != 2 || w.StageResults[0].Succeeded {
					t.Fatal("report overwrote source failure")
				}
			case "report_write":
				if w.Status != "blocked" || w.StepID != executionmodel.StepReport || w.ReportRetry != 0 {
					t.Fatal("report write escaped artifact gate")
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
					body = `{"outcome":"blocked","outputs":{"candidate_sha":"` + port.workspace.Head + `","branch":"` + port.workspace.Branch + `"},"summary":"push succeeded; PR failed","blocker":{"category":"failure","action":"create PR","error":"rejected","known":"branch pushed"}}`
				}
				require(t, acceptTestStage(t, s, run, port, body, true))
				if s.workflowRecord(run).Status != "blocked" || s.workflowRecord(run).StepID != executionmodel.StepPublish || len(s.workflowRecord(run).StageInputs) != 3 {
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
	for _, body := range []string{`{"outcome":"blocked","outcome":"done","summary":"x"}`, `{"outcome":"done","summary":"x","status":"completed"}`, "```json\n{}\n```", `{} {}`, `{"outcome":"done","summary":"x","outputs":{"candidate_sha":"fake"}}`} {
		if _, err := decodeStageReport(body, executionmodel.StepReport); err == nil {
			t.Fatalf("accepted invalid result %q", body)
		}
	}
}
