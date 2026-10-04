package steerlineintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const liveNativeQuestion = "Call steerline_ask with one question (id choice, header Choice, question Should I reply A or B?, options A and B, allow_free_text false). Wait for the answer. Then reply with the chosen letter only."

type appServerHostExitReceipt struct {
	Config     execution.Config                 `json:"config"`
	ConfigPath string                           `json:"config_path"`
	Home       string                           `json:"home"`
	RunID      string                           `json:"run_id"`
	Process    executionbackend.ProcessIdentity `json:"process"`
	ThreadID   string                           `json:"thread_id"`
	TurnID     string                           `json:"turn_id"`
	CallID     string                           `json:"call_id"`
	Workspace  string                           `json:"workspace"`
}

func newNativeLiveRecoveryProject(t *testing.T) integrationFixture {
	t.Helper()
	f := newLifecycleProjectBase(t, "local", false)
	var global map[string]json.RawMessage
	raw, err := os.ReadFile(steerlineapp.GlobalExecutionConfigPath())
	require(t, err)
	require(t, json.Unmarshal(raw, &global))
	payload, err := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "low", WindowsSandbox: "elevated"})
	require(t, err)
	global["execution_profiles"] = mustJSON([]map[string]any{{"id": "fixture", "name": "Native recovery", "host": "local", "agent": "codex", "implementation_configs": []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: payload}}}})
	require(t, os.WriteFile(steerlineapp.GlobalExecutionConfigPath(), mustJSON(global), 0600))
	projectRaw, err := os.ReadFile(f.configPath)
	require(t, err)
	var project map[string]json.RawMessage
	require(t, json.Unmarshal(projectRaw, &project))
	project["command_timeout"] = mustJSON("45s")
	project["execution_policy"] = mustJSON(workspacePolicyFixture())
	require(t, os.WriteFile(f.configPath, mustJSON(project), 0600))
	f.config, err = steerlineapp.LoadConfig(f.configPath)
	require(t, err)
	// Bind authority only after the original native policy/model is final.
	store, err := steerline.OpenNativeTaskProject(context.Background(), &f.config)
	require(t, err)
	require(t, store.Close())
	return f
}

func createNativeLiveTask(t *testing.T, ctx context.Context, f *integrationFixture, id int64, input string) {
	t.Helper()
	specPath := fmt.Sprintf("docs/spec/native-live-%d.spec.md", id)
	require(t, os.WriteFile(filepath.Join(f.repository, specPath), []byte("# Native acceptance\n\n## Acceptance\n\n"+input+"\n"), 0600))
	fixtureGitCommand(t, "-C", f.repository, "add", specPath)
	fixtureGitCommand(t, "-C", f.repository, "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "native acceptance contract")
	f.commit = fixtureGitOutput(t, "-C", f.repository, "rev-parse", "HEAD")
	fixtureGitCommand(t, "-C", f.repository, "push", "--quiet", f.bareRemote, "HEAD:refs/heads/main")
	contract := taskmodel.TaskContract{ContractID: fmt.Sprintf("native-live-%d", id), Repository: f.remoteURL, Remote: "origin", Branch: "main", SpecRef: integrationReference(t, f.repository, f.commit, specPath), PlanRef: integrationReference(t, f.repository, f.commit, "docs/spec/integration.plan.md"), CommonRefs: []taskmodel.FileReference{integrationReference(t, f.repository, f.commit, "docs/spec/native-task-constraints.md")}}
	contract.AcceptanceRefs = []taskmodel.FileReference{contract.SpecRef}
	var err error
	contract.Digest, err = taskmodel.ContractDigest(contract)
	require(t, err)
	store, err := steerline.OpenNativeTaskCommandStore(ctx, &f.config)
	require(t, err)
	defer store.Close()
	queries := steerline.NewQueryService(store, f.config.Identity())
	authority, err := queries.ReadyNativeTaskSnapshot(ctx)
	require(t, err)
	commands := appExecutionCommands(t, store, f.config)
	batch := taskmodel.TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: f.config.Identity(), Tasks: []taskmodel.TaskBatchTask{{LocalKey: "fixture", TaskID: id, Title: "Native recovery acceptance", Type: "task", Contract: contract}}, Dependencies: []taskmodel.TaskBatchEdge{}}
	_, err = createTaskBatchFixture(ctx, commands.Store, f.config, mustJSON(batch), newID(), authority.AuthorityVersion, "operator")
	require(t, err)
	page, err := queries.ShowNativeTask(ctx, fmt.Sprint(id))
	require(t, err)
	paused := false
	_, err = commands.SetNativeDispatchState(ctx, execution.NativeDispatchUpdate{RequestID: newID(), ExpectedAuthorityVersion: page.AuthorityVersion, Paused: &paused, RequestedBy: "operator"})
	require(t, err)
}

func nativeLiveAnswer(t *testing.T, ctx context.Context, c execution.Config, run *steerline.Run, option, note string) {
	t.Helper()
	_, _, err := steerline.SubmitDecisionAnswer(ctx, c, steerline.AnswerOptions{RunID: run.RunID, DecisionID: run.Decision.ID, ExpectedVersion: run.Decision.Version, Option: option, Note: note, By: "operator"})
	require(t, err)
}

func nativeLiveProcessState(identity *executionbackend.ProcessIdentity) string {
	return steerlineapp.NewProcessHost(time.Second).InspectProcess(identity)
}

func nativeLiveEnv(base []string, changes map[string]string) []string {
	out := make([]string, 0, len(base)+len(changes))
	for _, item := range base {
		key, _, _ := strings.Cut(item, "=")
		if _, ok := changes[key]; !ok {
			out = append(out, item)
		}
	}
	for key, value := range changes {
		out = append(out, key+"="+value)
	}
	return out
}

func nativeHostExitHelper(t *testing.T, ctx context.Context, root string) appServerHostExitReceipt {
	t.Helper()
	executable, err := os.Executable()
	require(t, err)
	command := exec.CommandContext(ctx, executable, "-test.run=^TestAppServerHostExitHelper$", "-test.timeout=8m")
	command.Env = nativeLiveEnv(os.Environ(), map[string]string{"STEERLINE_APP_SERVER_HOST_EXIT_HELPER": "1", "STEERLINE_APP_SERVER_HOST_EXIT_ROOT": root, "TEMP": root, "TMP": root})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("isolated native question helper: %v: %s", err, output)
	}
	raw, err := os.ReadFile(filepath.Join(root, "host-exit-receipt.json"))
	require(t, err)
	var receipt appServerHostExitReceipt
	require(t, json.Unmarshal(raw, &receipt))
	if receipt.Home == "" || receipt.ConfigPath == "" || receipt.RunID == "" || receipt.ThreadID == "" || receipt.TurnID == "" || receipt.CallID == "" {
		t.Fatal("native question receipt is incomplete")
	}
	return receipt
}

// Original opt-in and pinned model remain. This producer uses public task,
// service and query ports; process exit deliberately bypasses test cleanup.
func TestAppServerHostExitHelper(t *testing.T) {
	root := os.Getenv("STEERLINE_APP_SERVER_HOST_EXIT_ROOT")
	if os.Getenv("STEERLINE_APP_SERVER_HOST_EXIT_HELPER") != "1" || root == "" {
		t.Skip("host-exit helper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	f := newNativeLiveRecoveryProject(t)
	createNativeLiveTask(t, ctx, &f, 901, liveNativeQuestion)
	service := startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
	claimIntegrationTask(t, ctx, f, 901, service.instanceID)
	question := waitIntegrationRun(t, ctx, service, f.config, func(run *steerline.Run, e *executionmodel.ExecutionRecord) bool {
		return run.TaskID == "901" && run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute != nil
	})
	if question.execution.Process == nil {
		t.Fatal("native app-server has no durable process identity")
	}
	receipt := appServerHostExitReceipt{Config: f.config, ConfigPath: f.configPath, Home: os.Getenv("STEERLINE_HOME"), RunID: question.run.RunID, Process: *question.execution.Process, ThreadID: question.execution.SessionID, TurnID: question.execution.CurrentTurnID, CallID: question.run.Decision.Request.QuestionRoute.QuestionID, Workspace: question.execution.Workspace.Worktree}
	require(t, f.config.ProcessHost.WriteExecutionFile(filepath.Join(root, "host-exit-receipt.json"), mustJSON(receipt)))
	os.Exit(0)
}

func TestLocalCodexManualRecoveryLive(t *testing.T) {
	if os.Getenv("STEERLINE_WORKER_RECOVERY_LIVE") != "1" {
		t.Skip("real local Codex recovery is opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	receipt := nativeHostExitHelper(t, ctx, t.TempDir())
	waitFor(t, 5*time.Second, func() bool { return nativeLiveProcessState(&receipt.Process) == "process absent" })
	t.Setenv("STEERLINE_HOME", receipt.Home)
	config, err := steerlineapp.LoadConfig(receipt.ConfigPath)
	require(t, err)
	service := startIntegrationService(t, ctx, config, config.ExecutionDirectory)
	defer service.stop(t)
	recovered := waitIntegrationRun(t, ctx, service, config, func(run *steerline.Run, e *executionmodel.ExecutionRecord) bool {
		return run.RunID == receipt.RunID && e.RecoveryRequired && e.RecoveryState == "interrupted" && run.Decision != nil && run.Decision.Status == "pending"
	})
	oldAttempt := recovered.run.AttemptID
	if recovered.run.Decision.Request.QuestionRoute != nil || recovered.execution.SessionID != receipt.ThreadID || recovered.execution.Workspace.Worktree != receipt.Workspace || !strings.Contains(recovered.run.Decision.Request.Context, "Should I reply A or B?") || len(recovered.runtime.Attempts) != 1 {
		t.Fatal("reopen lost original question or created a generation before manual input")
	}
	if recovered.execution.Process != nil && nativeLiveProcessState(recovered.execution.Process) != "process absent" {
		t.Fatal("reopen started a worker before manual confirmation")
	}
	note := "The old question asked A or B. Choose A. Inspect the current worktree first, then reply with A only."
	nativeLiveAnswer(t, ctx, config, recovered.run, "continue", note)
	resumed := waitIntegrationRun(t, ctx, service, config, func(run *steerline.Run, e *executionmodel.ExecutionRecord) bool {
		return run.RunID == receipt.RunID && run.AttemptID != oldAttempt && e.Process != nil && e.CurrentCommandID != ""
	})
	if *resumed.execution.Process == receipt.Process || len(resumed.runtime.Attempts) != 2 || resumed.execution.SessionID != receipt.ThreadID || resumed.execution.Workspace.Worktree != receipt.Workspace {
		t.Fatal("manual successor replaced the session/workspace or lost its distinct Attempt/process")
	}
	settled := waitIntegrationRun(t, ctx, service, config, func(run *steerline.Run, e *executionmodel.ExecutionRecord) bool {
		return run.RunID == receipt.RunID && e.LastStoppedTurnID != "" && e.LastStoppedTurnID != receipt.TurnID && e.TurnNumber >= 2
	})
	t.Logf("manual execution recovery: old_pid=%d new_pid=%d session=%s old_turn=%s new_turn=%s old_attempt=%s new_attempt=%s", receipt.Process.PID, settled.execution.Process.PID, settled.execution.SessionID, receipt.TurnID, settled.execution.LastStoppedTurnID, oldAttempt, settled.run.AttemptID)
}

// The historical detached-process gate is retained, including its original
// failure assertions. Current managed workers receive no detached fallback.
func TestAppServerHostExitRecoveryLive(t *testing.T) {
	if os.Getenv("STEERLINE_APP_SERVER_SERVICE_LIVE_LEGACY") != "1" {
		t.Skip("historical detached-process acceptance")
	}
	helperCtx, cancelHelper := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancelHelper()
	receipt := nativeHostExitHelper(t, helperCtx, t.TempDir())
	if nativeLiveProcessState(&receipt.Process) != "matching process still exists" {
		t.Fatal("app-server did not survive its service process")
	}
	// Preserve the original separate helper (8m) and recovery (4m) budgets.
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	t.Setenv("STEERLINE_HOME", receipt.Home)
	config, err := steerlineapp.LoadConfig(receipt.ConfigPath)
	require(t, err)
	service := startIntegrationService(t, ctx, config, config.ExecutionDirectory)
	defer service.stop(t)
	recovered := waitIntegrationRun(t, ctx, service, config, func(run *steerline.Run, e *executionmodel.ExecutionRecord) bool {
		return run.RunID == receipt.RunID && run.Decision != nil && run.Decision.Request.QuestionRoute != nil
	})
	if recovered.run.Decision.Request.QuestionRoute.QuestionID != receipt.CallID || recovered.execution.SessionID != receipt.ThreadID || recovered.execution.CurrentTurnID != receipt.TurnID || recovered.execution.Workspace.Worktree != receipt.Workspace || recovered.execution.Process == nil || *recovered.execution.Process != receipt.Process {
		t.Fatal("new host did not recover the original question, process, thread, turn and worktree")
	}
	nativeLiveAnswer(t, ctx, config, recovered.run, "A", "")
	settled := waitIntegrationRun(t, ctx, service, config, func(run *steerline.Run, e *executionmodel.ExecutionRecord) bool {
		return run.RunID == receipt.RunID && e.LastStoppedTurnID == receipt.TurnID && run.Decision != nil && run.Decision.Request.QuestionRoute == nil
	})
	t.Logf("host-exit native recovery: process=%d thread=%s turn=%s call=%s", settled.execution.Process.PID, settled.execution.SessionID, settled.execution.CurrentTurnID, receipt.CallID)
}

func TestAppServerServiceParallelRecoveryLive(t *testing.T) {
	if os.Getenv("STEERLINE_APP_SERVER_SERVICE_LIVE_LEGACY") != "1" {
		t.Skip("historical detached-process acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	f := newNativeLiveRecoveryProject(t)
	createNativeLiveTask(t, ctx, &f, 901, liveNativeQuestion)
	createNativeLiveTask(t, ctx, &f, 902, "Reply READY only. Do not call steerline_ask.")
	first := startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
	claimIntegrationTask(t, ctx, f, 901, first.instanceID)
	question := waitIntegrationRun(t, ctx, first, f.config, func(run *steerline.Run, e *executionmodel.ExecutionRecord) bool {
		return run.TaskID == "901" && run.Decision != nil && run.Decision.Request.QuestionRoute != nil
	})
	if question.execution.Process == nil {
		t.Fatal("first native task has no original process identity")
	}
	original := *question.execution.Process
	firstTurn, firstThread, firstWorkspace := question.execution.CurrentTurnID, question.execution.SessionID, question.execution.Workspace.Worktree
	if firstTurn == "" || firstThread == "" {
		t.Fatal("first task lost its process, thread or turn")
	}
	claimIntegrationTask(t, ctx, f, 902, first.instanceID)
	second := waitIntegrationRun(t, ctx, first, f.config, func(run *steerline.Run, e *executionmodel.ExecutionRecord) bool {
		return run.TaskID == "902" && e.LastStoppedTurnID != "" && run.Decision != nil && run.Decision.Request.QuestionRoute == nil
	})
	if second.execution.Process == nil || second.execution.Process.PID == original.PID || second.execution.Workspace.Worktree == firstWorkspace || second.execution.SessionID == firstThread || second.execution.CurrentTurnID == "" {
		t.Fatal("second task lost its independent native process, workspace, thread or turn")
	}
	stillWaiting := waitIntegrationRun(t, ctx, first, f.config, func(run *steerline.Run, e *executionmodel.ExecutionRecord) bool {
		return run.TaskID == "901" && run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute != nil
	})
	callID := stillWaiting.run.Decision.Request.QuestionRoute.QuestionID
	first.stop(t)
	restarted := startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
	defer restarted.stop(t)
	recovered := waitIntegrationRun(t, ctx, restarted, f.config, func(run *steerline.Run, e *executionmodel.ExecutionRecord) bool {
		return run.TaskID == "901" && run.Decision != nil && run.Decision.Request.QuestionRoute != nil
	})
	if recovered.run.Decision.Request.QuestionRoute.QuestionID != callID || recovered.execution.SessionID != firstThread || recovered.execution.CurrentTurnID != firstTurn || recovered.execution.Process == nil || *recovered.execution.Process != original {
		t.Fatal("restart failed to reattach the original native question/process/thread/turn")
	}
	nativeLiveAnswer(t, ctx, f.config, recovered.run, "A", "")
	settled := waitIntegrationRun(t, ctx, restarted, f.config, func(run *steerline.Run, e *executionmodel.ExecutionRecord) bool {
		return run.TaskID == "901" && e.LastStoppedTurnID == firstTurn && run.Decision != nil && run.Decision.Request.QuestionRoute == nil
	})
	nativeLiveAnswer(t, ctx, f.config, settled.run, "continue", "Feedback: reply ACK only.")
	feedback := waitIntegrationRun(t, ctx, restarted, f.config, func(run *steerline.Run, e *executionmodel.ExecutionRecord) bool {
		return run.TaskID == "901" && e.TurnNumber == 2 && e.CurrentTurnID != firstTurn && e.CurrentTurnID == e.LastStoppedTurnID
	})
	if feedback.execution.SessionID != firstThread || feedback.execution.Workspace.Worktree != firstWorkspace {
		t.Fatal("post-turn feedback replaced the original thread/worktree")
	}
}
