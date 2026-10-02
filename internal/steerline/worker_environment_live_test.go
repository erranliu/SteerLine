package steerline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

// Explicit opt-in: a registered isolated test repository and a pinned smoke
// contract are required. Failure preserves resources and durable evidence.
func TestWorkerEnvironmentNativeReadinessLive(t *testing.T) {
	repo, commit, codex := os.Getenv("STEERLINE_READINESS_LIVE_REPO"), os.Getenv("STEERLINE_READINESS_LIVE_COMMIT"), os.Getenv("STEERLINE_READINESS_LIVE_CODEX")
	if repo == "" || commit == "" || codex == "" {
		t.Skip("isolated native readiness acceptance was not selected")
	}
	stateRoot := os.Getenv("STEERLINE_READINESS_LIVE_STATE_ROOT")
	resume := stateRoot != ""
	var err error
	if !resume {
		stateRoot, err = os.MkdirTemp("", "steerline-pr154-live-")
		require(t, err)
	}
	t.Setenv("STEERLINE_HOME", stateRoot)
	ctx := context.Background()
	c := Config{TaskAuthority: "native", WorkflowProtocol: WorkerFlowVersion, Repository: repo, Remote: "origin", Branch: "main", RunDir: filepath.Join(stateRoot, "state"), WorktreeRoot: filepath.Join(stateRoot, "worktrees"), Host: "orca", Agent: "codex", Claimant: "readiness-acceptance", ExecutableTypes: []string{"task"}, Git: Command{Executable: "git"}, CommandTimeout: "10s", TaskPoll: "10s", PRPoll: "30s", Heartbeat: "5s", ShutdownGrace: "10s"}
	setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: codex, WindowsSandbox: "unelevated"}, &orcaworker.ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "4m"})
	key, err := repoStateKey(c)
	require(t, err)
	c.RunDir = filepath.Join(SteerlineStateRoot(), key)
	c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	require(t, c.ResolveIdentity(ctx))
	require(t, requireWorkerFlowCapabilities(ctx, c))
	project, err := OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, project.Close())
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	require(t, err)
	defer store.Close()
	target, err := configuredRepositoryTarget(ctx, c)
	require(t, err)
	spec, plan := nativeRef(t, c, commit, "docs/spec/pr154-readiness-gate.spec.md"), nativeRef(t, c, commit, "docs/spec/pr154-readiness-gate.plan.md")
	contract := TaskContract{ContractID: "pr154-readiness-live", Repository: target, Remote: "origin", Branch: "main", SpecRef: spec, PlanRef: plan, AcceptanceRefs: []FileReference{spec}}
	contract.Digest, err = TaskContractDigest(contract)
	require(t, err)
	priority := 2
	input, err := json.Marshal(TaskBatchInput{SchemaVersion: taskBatchSchemaVersion, ProjectID: c.Identity(), Tasks: []TaskBatchTask{{LocalKey: "readiness", TaskID: 9001154, Title: "PR154 isolated readiness and business dispatch acceptance", Type: "task", Priority: &priority, Contract: contract}}, Dependencies: []TaskBatchEdge{}})
	require(t, err)
	commands := CommandService{Store: store, Project: c.Identity()}
	validation, err := commands.ValidateTaskBatch(ctx, c, input)
	require(t, err)
	if !resume && !validation.Valid {
		t.Fatalf("live contract rejected: %+v", validation)
	}
	if !resume {
		_, err = commands.CreateTaskBatch(ctx, c, input, newID(), validation.AuthorityVersion, "acceptance")
		require(t, err)
		authority, err := store.nativeAuthority(ctx, c.Identity())
		require(t, err)
		authority.AuthorityVersion++
		authority.DispatchPaused = false
		require(t, store.Transact(ctx, func(tx DomainTx) error {
			return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
		}))
		_, err = commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "9001154", RequestID: newID(), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "acceptance", Explicit: true})
		require(t, err)
	}
	s, err := NewService(c)
	require(t, err)
	defer s.Close()
	run := s.find("9001154")
	t.Logf("isolated state retained at %s; workflow=%s execution=%s", stateRoot, run.RunID, run.ExecutionID)
	if !resume {
		require(t, s.prepareWorkerFlow(ctx, *nativeTaskRecord(s.domain.Tasks, run.TaskID), run))
	} else {
		require(t, s.resumeConfiguredWork(ctx))
	}
	deadline := time.Now().Add(7 * time.Minute)
	for time.Now().Before(deadline) {
		e := s.executionRecord(run)
		if e.Environment.Blocker != nil {
			reason := e.Environment.Blocker.Fact.Reason
			if reason != "resource_preparation_unverified" && reason != "preparation_dispatch_unverified" && reason != "preparation_unverified" && reason != "agent_input_unverified" {
				t.Fatalf("live readiness blocked: %+v", e.Environment.Blocker)
			}
			// This test requests a metadata/operation recheck, never authorizes or
			// manipulates an OS menu, replaces a session, or changes its profile.
			s.State.Service.Ready = true
			require(t, s.publishCheckpoint())
			d := run.Decision
			a := DecisionAnswer{DecisionID: d.ID, RunID: run.RunID, OptionID: "recheck_environment", AnsweredBy: "acceptance", ExpectedDecisionVersion: d.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: s.State.Service.InstanceID, SourceAttemptID: run.AttemptID, ExpectedRecoveryVersion: run.RecoveryVersion}
			_, _, err = commands.AnswerDecisionWithID(ctx, newID(), d.ID, d.Version, decisionAnswerKey(a), a.AnsweredBy, mustJSON(a))
			require(t, err)
			require(t, s.processCommandsAndDecisions(ctx))
		} else if turn := nativeOrcaCurrentTurn(e); turn != nil && turn.DispatchID != "" {
			require(t, driveTestLifecycleWorker(s, run))
		} else {
			require(t, s.advanceOrcaPreparation(ctx, run))
		}
		e = s.executionRecord(run)
		state := testOrcaState(t, e)
		if state != nil && len(state.Turns) == 2 && state.Turns[1].Settled {
			if !e.Environment.Ready || !e.Environment.Preparation.Verified || e.TurnNumber != 1 || state.Turns[0].Purpose != "readiness" || state.Turns[1].Purpose != "business" || state.Turns[0].DispatchID == state.Turns[1].DispatchID || state.Turns[1].Outcome != "succeeded" {
				t.Fatalf("live session continuity failed: %+v", e)
			}
			if e.Process.Terminal != e.Environment.Preparation.Probe.Terminal || e.Process.Incarnation != e.Environment.Preparation.Probe.Incarnation {
				t.Fatal("business replaced readiness session")
			}
			report := map[string]any{"captured_at": now(), "workflow_run_id": run.RunID, "execution_id": e.ExecutionID, "profile": s.workflowRecord(run).ExecutionProfile, "environment": e.Environment, "turns": state.Turns, "process": e.Process, "state_root": stateRoot, "scope": "real readiness probe and settlement, exactly one business Dispatch, original terminal/incarnation, business command and settlement; no PR delivery"}
			data, _ := json.MarshalIndent(report, "", "  ")
			if path := os.Getenv("STEERLINE_READINESS_LIVE_REPORT"); path != "" {
				require(t, os.WriteFile(path, data, 0600))
			}
			t.Logf("real readiness and business settled: %s / %s", state.Turns[0].DispatchID, state.Turns[1].DispatchID)
			// Exact settled resources were created by this isolated center.
			lifecycle, _, err := s.lifecycleForRun(run)
			require(t, err)
			require(t, s.cleanupLifecycleResources(ctx, run, lifecycle))
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("real readiness acceptance did not settle; preserve state and original resources: %+v", s.executionRecord(run))
}
