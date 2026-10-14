package integration

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

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const liveNativeQuestion = "Call steerline_ask with one question (id choice, header Choice, question Should I reply A or B?, options A and B, allow_free_text false). Wait for the answer. Then reply with the chosen letter only."

type appServerHostExitReceipt struct {
	Config     steerlineapp.Config              `json:"config"`
	ConfigPath string                           `json:"config_path"`
	Home       string                           `json:"home"`
	RunID      string                           `json:"run_id"`
	Process    executionbackend.ProcessIdentity `json:"process"`
	ThreadID   string                           `json:"thread_id"`
	TurnID     string                           `json:"turn_id"`
	CallID     string                           `json:"call_id"`
	Workspace  string                           `json:"workspace"`
	CodexHome  string                           `json:"codex_home"`
}

func newNativeLiveRecoveryProject(t *testing.T) integrationFixture {
	t.Helper()
	f := newLifecycleProjectBase(t, "local", false)

	authHome := os.Getenv("CODEX_HOME")
	if authHome == "" {
		userHome, err := os.UserHomeDir()
		require(t, err)
		authHome = filepath.Join(userHome, ".codex")
	}
	auth, err := os.ReadFile(filepath.Join(authHome, "auth.json"))
	require(t, err)
	codexHome := filepath.Join(t.TempDir(), "codex")
	require(t, os.MkdirAll(codexHome, 0700))
	require(t, os.WriteFile(filepath.Join(codexHome, "auth.json"), auth, 0600))
	require(t, os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(fmt.Sprintf("[projects.%q]\ntrust_level = \"trusted\"\n", filepath.ToSlash(f.repository))), 0600))
	t.Setenv("CODEX_HOME", codexHome)
	var global map[string]json.RawMessage
	raw, err := os.ReadFile(steerlineapp.GlobalExecutionConfigPath())
	require(t, err)
	require(t, json.Unmarshal(raw, &global))
	nativeExecutable, err := codexworker.NativeExecutable("codex")
	require(t, err)
	payload, err := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: nativeExecutable, Model: "gpt-6-luna", ReasoningEffort: "low"})
	require(t, err)
	global["execution_profiles"] = mustJSON([]map[string]any{{"id": "fixture", "name": "Native recovery", "host": "local", "agent": "codex", "implementation_configs": []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 2, Payload: payload}}}})
	require(t, os.WriteFile(steerlineapp.GlobalExecutionConfigPath(), mustJSON(global), 0600))
	projectRaw, err := os.ReadFile(f.configPath)
	require(t, err)
	var project map[string]json.RawMessage
	require(t, json.Unmarshal(projectRaw, &project))
	project["command_timeout"] = mustJSON("45s")
	require(t, os.WriteFile(f.configPath, mustJSON(project), 0600))
	f.config, err = steerlineapp.LoadConfig(f.configPath)
	require(t, err)
	// Bind authority only after the original native policy/model is final.
	store, err := steerlineapp.OpenNativeTaskProject(context.Background(), &f.config)
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
	store, err := steerlineapp.OpenNativeTaskCommandStore(ctx, &f.config)
	require(t, err)
	defer store.Close()
	queries := productquery.NewService(store, f.config.Identity())
	authority, err := queries.ReadyNativeTaskSnapshot(ctx)
	require(t, err)
	commands := appExecutionCommands(t, store, f.config)
	batch := taskmodel.TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: f.config.Identity(), Tasks: []taskmodel.TaskBatchTask{{LocalKey: "fixture", TaskID: id, Title: "Native recovery acceptance", Type: "task", Contract: contract}}, Dependencies: []taskmodel.TaskBatchEdge{}}
	_, err = createTaskBatchFixture(ctx, sqliteCommandStore(commands.Store), f.config, mustJSON(batch), newID(), authority.AuthorityVersion, "operator")
	require(t, err)
	page, err := queries.ShowNativeTask(ctx, fmt.Sprint(id))
	require(t, err)
	paused := false
	_, err = commands.SetNativeDispatchState(ctx, execution.NativeDispatchUpdate{RequestID: newID(), ExpectedAuthorityVersion: page.AuthorityVersion, Paused: &paused, RequestedBy: "operator"})
	require(t, err)
}

func nativeLiveAnswer(t *testing.T, ctx context.Context, c steerlineapp.Config, run *reconstructiontypes.RunView, option, note string) {
	t.Helper()
	_, _, err := steerlineapp.SubmitDecisionAnswer(ctx, c, steerlineapp.AnswerOptions{RunID: run.RunID, DecisionID: run.Decision.ID, ExpectedVersion: run.Decision.Version, Option: option, Note: note, By: "operator"})
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
	question := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.TaskID == "901" && run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute != nil
	})
	if question.execution.Process == nil {
		t.Fatal("native app-server has no durable process identity")
	}
	receipt := appServerHostExitReceipt{Config: f.config, ConfigPath: f.configPath, Home: os.Getenv("STEERLINE_HOME"), RunID: question.run.RunID, Process: *question.execution.Process, ThreadID: question.execution.SessionID, TurnID: question.execution.CurrentTurnID, CallID: question.run.Decision.Request.QuestionRoute.QuestionID, Workspace: question.execution.Workspace.Worktree, CodexHome: os.Getenv("CODEX_HOME")}
	require(t, f.config.ProcessHost.WriteExecutionFile(filepath.Join(root, "host-exit-receipt.json"), mustJSON(receipt)))
	os.Exit(0)
}

func TestLocalCodexManualRecoveryLive(t *testing.T) {
	if os.Getenv("STEERLINE_WORKER_RECOVERY_LIVE") != "1" {
		t.Skip("real local worker recovery is opt-in")
	}
	t.Run("host_exit", testLocalCodexHostExitRecovery)
	t.Run("normal_service_stop", testLocalCodexNormalStopRecovery)
}

func testLocalCodexHostExitRecovery(t *testing.T) {
	if os.Getenv("STEERLINE_WORKER_RECOVERY_LIVE") != "1" {
		t.Skip("real local Codex recovery is opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	receipt := nativeHostExitHelper(t, ctx, t.TempDir())
	waitFor(t, 5*time.Second, func() bool { return nativeLiveProcessState(&receipt.Process) == "process absent" })
	t.Setenv("STEERLINE_HOME", receipt.Home)
	t.Setenv("CODEX_HOME", receipt.CodexHome)
	t.Setenv("STEERLINE_TEST_REMOTE_URL", fixtureGitOutput(t, "-C", receipt.Config.Repository, "remote", "get-url", "origin"))
	t.Setenv("STEERLINE_FAKE_GIT_FETCH_REMOTE", filepath.Join(filepath.Dir(receipt.Config.Repository), "origin.git"))
	config, err := steerlineapp.LoadConfig(receipt.ConfigPath)
	require(t, err)
	service := startIntegrationService(t, ctx, config, config.ExecutionDirectory)
	defer service.stop(t)
	recovered := waitIntegrationRun(t, ctx, service, config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
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
	resumed := waitIntegrationRun(t, ctx, service, config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.RunID == receipt.RunID && run.AttemptID != oldAttempt && e.Process != nil && e.CurrentCommandID != ""
	})
	if *resumed.execution.Process == receipt.Process || len(resumed.runtime.Attempts) != 2 || resumed.execution.SessionID != receipt.ThreadID || resumed.execution.Workspace.Worktree != receipt.Workspace {
		t.Fatal("manual successor replaced the session/workspace or lost its distinct Attempt/process")
	}
	settled := waitIntegrationRun(t, ctx, service, config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.RunID == receipt.RunID && e.LastStoppedTurnID != "" && e.LastStoppedTurnID != receipt.TurnID && e.TurnNumber >= 2
	})
	t.Logf("manual execution recovery: old_pid=%d new_pid=%d session=%s old_turn=%s new_turn=%s old_attempt=%s new_attempt=%s", receipt.Process.PID, settled.execution.Process.PID, settled.execution.SessionID, receipt.TurnID, settled.execution.LastStoppedTurnID, oldAttempt, settled.run.AttemptID)
}

func testLocalCodexNormalStopRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	f := newNativeLiveRecoveryProject(t)
	createNativeLiveTask(t, ctx, &f, 901, liveNativeQuestion)
	service := startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
	defer func() { service.stop(t) }()
	claimIntegrationTask(t, ctx, f, 901, service.instanceID)
	question := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute != nil
	})
	oldProcess, oldAttempt, oldSession, oldWorkspace := *question.execution.Process, question.run.AttemptID, question.execution.SessionID, question.execution.Workspace.Worktree
	if question.runtime.WorkflowRuns[0].ExecutionProfile.ServiceStopBehavior != executionbackend.StopExecutionOnServiceStop {
		t.Fatal("wrong frozen worker shutdown behavior")
	}
	service.stop(t)
	waitFor(t, 5*time.Second, func() bool { return nativeLiveProcessState(&oldProcess) == "process absent" })
	service = startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
	recovered := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return e.RecoveryRequired && e.RecoveryState == "interrupted" && run.Decision != nil && run.Decision.Status == "pending"
	})
	if recovered.run.AttemptID != oldAttempt || recovered.execution.SessionID != oldSession || recovered.execution.Workspace.Worktree != oldWorkspace || recovered.execution.TurnNumber != 1 {
		t.Fatal("normal reopen replaced the original execution before consent")
	}
	for _, op := range recovered.runtime.Operations {
		if op.Kind == "stop_worker_observation" {
			t.Fatal("service shutdown journaled an interrupt RPC")
		}
	}
	nativeLiveAnswer(t, ctx, f.config, recovered.run, "continue", "The prior question asked A or B. Choose A; inspect existing work first, then answer A only.")
	settled := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.AttemptID != oldAttempt && e.LastStoppedTurnID != "" && e.TurnNumber >= 2
	})
	if settled.execution.SessionID != oldSession || settled.execution.Workspace.Worktree != oldWorkspace || *settled.execution.Process == oldProcess || len(settled.runtime.Attempts) != 2 {
		t.Fatal("normal-stop recovery lost its original session or duplicated the replacement Attempt")
	}
	t.Logf("normal worker service stop: old_pid=%d new_pid=%d thread=%s old_attempt=%s new_attempt=%s old_turn=%s new_turn=%s", oldProcess.PID, settled.execution.Process.PID, oldSession, oldAttempt, settled.run.AttemptID, question.execution.CurrentTurnID, settled.execution.LastStoppedTurnID)
}
