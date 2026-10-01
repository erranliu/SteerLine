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

// Opt in on the actual local Windows host. The helper creates only an isolated
// fixture and exits while Codex waits for a native question. Its owned Job
// closes with the helper; this test checks the manual successor generation.
func TestLocalCodexManualRecoveryLive(t *testing.T) {
	if os.Getenv("STEERLINE_WORKER_RECOVERY_LIVE") != "1" {
		t.Skip("real local Codex recovery is opt-in")
	}
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	helper := exec.CommandContext(ctx, exe, "-test.run=^TestAppServerHostExitHelper$", "-test.timeout=8m")
	helper.Env = overrideEnv(os.Environ(), map[string]string{
		"STEERLINE_APP_SERVER_HOST_EXIT_HELPER": "1", "STEERLINE_APP_SERVER_HOST_EXIT_ROOT": root,
		"TEMP": root, "TMP": root,
	})
	if output, err := helper.CombinedOutput(); err != nil {
		t.Fatalf("isolated Codex helper failed before recording its question: %v: %s", err, output)
	}
	var receipt appServerHostExitReceipt
	data, err := os.ReadFile(filepath.Join(root, "host-exit-receipt.json"))
	if err != nil || json.Unmarshal(data, &receipt) != nil || receipt.Home == "" || receipt.ThreadID == "" || receipt.TurnID == "" || receipt.CallID == "" {
		t.Fatalf("native question receipt is incomplete: %v", err)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && inspectProcess(&receipt.Process) != "process absent"; {
		time.Sleep(30 * time.Millisecond)
	}
	if inspectProcess(&receipt.Process) != "process absent" {
		t.Fatal("owned Codex app-server survived its service Job close")
	}
	t.Setenv("STEERLINE_HOME", receipt.Home)
	service, err := NewService(receipt.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		service.shutdown()
		for _, execution := range service.domain.Executions {
			if execution.Process == nil {
				continue
			}
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && inspectProcess(execution.Process) != "process absent"; {
				time.Sleep(30 * time.Millisecond)
			}
		}
		_ = service.Close()
	}()
	run := service.find("8")
	if run == nil || run.RunID != receipt.RunID {
		t.Fatal("reopened service lost the original synthetic task")
	}
	oldAttempt := run.AttemptID
	service.markServiceRecoveryGate()
	if err := service.commitWorker(run); err != nil {
		t.Fatal(err)
	}
	if err := service.resumeConfiguredWork(ctx); err != nil {
		t.Fatal(err)
	}
	_, execution := service.workflowRecords(run)
	if execution == nil || execution.RecoveryState != "interrupted" || !execution.RecoveryRequired ||
		run.Decision == nil || run.Decision.Status != "pending" || run.Decision.Request.QuestionRoute != nil ||
		service.workers[run.RunID] != nil || run.AttemptID != oldAttempt || codexSessionID(execution) != receipt.ThreadID ||
		execution.Workspace.Worktree != receipt.Workspace || !strings.Contains(run.Decision.Request.Context, "Should I reply A or B?") {
		t.Fatalf("reopen did not wait for manual recovery: state=%q worker=%t decision=%+v", execution.RecoveryState, service.workers[run.RunID] != nil, run.Decision)
	}
	if len(service.domain.Attempts) != 1 {
		t.Fatal("reopen created a worker generation without user input")
	}
	service.State.Service.Ready, service.State.Service.Status = true, "running"
	if err := service.commitWorker(run); err != nil {
		t.Fatal(err)
	}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: "continue", Note: "The old question asked A or B. Choose A. Inspect the current worktree first, then reply with A only.", AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, SourceAttemptID: run.AttemptID, ExpectedRecoveryVersion: run.RecoveryVersion, ServiceInstanceID: service.State.Service.InstanceID}
	commands := CommandService{Store: service.domainStore, Project: service.Config.identity}
	command, _, err := commands.AnswerDecisionWithID(ctx, newID(), answer.DecisionID, answer.ExpectedDecisionVersion, decisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := commands.AcceptAnswer(ctx, command, answer)
	if err != nil {
		t.Fatal(err)
	}
	*service.decisionRecord(run) = record
	require(t, service.refreshNativeSnapshot(context.Background()))
	execution = service.executionRecord(run)
	service.refreshRun(run)
	if err := service.commitWorker(run); err != nil {
		t.Fatal(err)
	}
	if err := service.applyWorkerFlowDecision(ctx, run); err != nil {
		t.Fatal(err)
	}
	worker := service.workers[run.RunID]
	_, execution = service.workflowRecords(run)
	if worker == nil || execution.Process == nil || *execution.Process == receipt.Process || run.AttemptID == oldAttempt ||
		len(service.domain.Attempts) != 2 || codexSessionID(execution) != receipt.ThreadID ||
		execution.Workspace.Worktree != receipt.Workspace || execution.PendingCommandID == "" && execution.CurrentCommandID == "" {
		t.Fatalf("manual continuation did not bind a new process and Attempt: worker=%t attempt=%s state=%q", worker != nil, run.AttemptID, execution.RecoveryState)
	}
	for {
		select {
		case <-ctx.Done():
			t.Fatal("manual Codex turn did not finish")
		case event, ok := <-worker.Events:
			if ok {
				if err := service.eventForRun(run, event); err != nil {
					t.Fatal(err)
				}
				continue
			}
			workerErr := <-worker.Done
			delete(service.workers, run.RunID)
			if err := service.finishedForRun(run, worker, workerErr); err != nil {
				t.Fatal(err)
			}
			_, execution = service.workflowRecords(run)
			if execution.LastStoppedTurnID == "" || execution.LastStoppedTurnID == receipt.TurnID || execution.TurnNumber < 2 {
				t.Fatal("manual recovery did not complete a distinct native turn")
			}
			t.Logf("manual Codex recovery: old_pid=%d new_pid=%d thread=%s old_turn=%s new_turn=%s old_attempt=%s new_attempt=%s", receipt.Process.PID, execution.Process.PID, codexSessionID(execution), receipt.TurnID, execution.LastStoppedTurnID, oldAttempt, run.AttemptID)
			return
		}
	}
}
