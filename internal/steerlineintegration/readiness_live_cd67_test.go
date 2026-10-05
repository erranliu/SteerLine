package steerlineintegration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"net/url"
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
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// The original opt-in requires a registered isolated repository and pinned
// smoke contract. Every action uses public application/task/query ports.
func TestWorkerEnvironmentNativeReadinessLive(t *testing.T) {
	repo, commit, worker := os.Getenv("STEERLINE_READINESS_LIVE_REPO"), os.Getenv("STEERLINE_READINESS_LIVE_COMMIT"), os.Getenv("STEERLINE_READINESS_LIVE_CODEX")
	if repo == "" || commit == "" || worker == "" {
		t.Skip("isolated native readiness acceptance was not selected")
	}
	home := os.Getenv("STEERLINE_READINESS_LIVE_STATE_ROOT")
	resume := home != ""
	var err error
	if !resume {
		home, err = os.MkdirTemp("", "steerline-pr154-live-")
		require(t, err)
	}
	t.Setenv("STEERLINE_HOME", home)
	ctx := context.Background()
	remoteBytes, err := exec.CommandContext(ctx, "git", "-C", repo, "remote", "get-url", "origin").Output()
	require(t, err)
	remote := strings.TrimSpace(string(remoteBytes))
	var host, repository string
	if strings.HasPrefix(remote, "git@") {
		host, repository, _ = strings.Cut(strings.TrimPrefix(remote, "git@"), ":")
	} else {
		parsed, err := url.Parse(remote)
		require(t, err)
		host, repository = parsed.Host, strings.TrimPrefix(parsed.Path, "/")
	}
	repository = strings.TrimSuffix(repository, ".git")
	payload, err := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: worker, WindowsSandbox: "unelevated"})
	require(t, err)
	terminal, err := orcaworker.EncodeProfileConfig(orcaworker.ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "4m"})
	require(t, err)
	inputs := []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: payload}, {ImplementationKey: orcaworker.ProfileCodecKey, CodecVersion: 1, Payload: terminal}}
	require(t, os.MkdirAll(filepath.Dir(steerlineapp.GlobalExecutionConfigPath()), 0700))
	global := map[string]any{"version": 2, "default_execution_profile": "readiness", "execution_profiles": []map[string]any{{"id": "readiness", "name": "Registered isolated readiness", "host": "orca", "agent": "codex", "implementation_configs": inputs}}}
	require(t, os.WriteFile(steerlineapp.GlobalExecutionConfigPath(), mustJSON(global), 0600))
	project := map[string]any{"task_authority": "native", "workflow_protocol": executionmodel.Version, "repository": repo, "remote": "origin", "branch": "main", "claimant": "readiness-acceptance", "executable_types": []string{"task"}, "git": executionbackend.Command{Executable: "git"}, "command_timeout": "10s", "task_poll": "10s", "pr_poll": "30s", "heartbeat": "5s", "shutdown_grace": "10s", "execution_policy": workspacePolicyFixture(), "publication_policy": executionbackend.PublicationPolicy{Publisher: "worker"}, "pr": publication.Config{Kind: "command", BaseURL: "https://" + host, Repository: repository, Query: executionbackend.Command{Executable: "git", Args: []string{"status"}}}}
	configPath := filepath.Join(home, "readiness-project.json")
	require(t, os.WriteFile(configPath, mustJSON(project), 0600))
	c, err := steerlineapp.LoadConfig(configPath)
	require(t, err)
	authority, err := steerline.OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, authority.Close())
	store, err := steerline.OpenNativeTaskCommandStore(ctx, &c)
	require(t, err)
	queries := productquery.NewService(store, c.Identity())
	commands := appExecutionCommands(t, store, c)
	spec, plan := liveReadinessReference(t, ctx, repo, commit, "docs/spec/pr154-readiness-gate.spec.md"), liveReadinessReference(t, ctx, repo, commit, "docs/spec/pr154-readiness-gate.plan.md")
	contract := taskmodel.TaskContract{ContractID: "pr154-readiness-live", Repository: remote, Remote: "origin", Branch: "main", SpecRef: spec, PlanRef: plan, AcceptanceRefs: []taskmodel.FileReference{spec}}
	contract.Digest, err = taskmodel.ContractDigest(contract)
	require(t, err)
	priority := 2
	batch := taskmodel.TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: c.Identity(), Tasks: []taskmodel.TaskBatchTask{{LocalKey: "readiness", TaskID: 9001154, Title: "PR154 isolated readiness and business dispatch acceptance", Type: "task", Priority: &priority, Contract: contract}}, Dependencies: []taskmodel.TaskBatchEdge{}}
	validation, err := validateTaskBatchFixture(ctx, commands.Store, c, mustJSON(batch))
	require(t, err)
	if !resume && !validation.Valid {
		t.Fatalf("live contract rejected: %+v", validation)
	}
	if !resume {
		_, err = createTaskBatchFixture(ctx, commands.Store, c, mustJSON(batch), newID(), validation.AuthorityVersion, "acceptance")
		require(t, err)
		page, err := queries.ShowNativeTask(ctx, "9001154")
		require(t, err)
		paused := false
		_, err = commands.SetNativeDispatchState(ctx, execution.NativeDispatchUpdate{RequestID: newID(), ExpectedAuthorityVersion: page.AuthorityVersion, Paused: &paused, RequestedBy: "acceptance"})
		require(t, err)
	}
	require(t, store.Close())
	service := startReadinessLiveService(t, ctx, c)
	defer service.stop(t)
	if !resume {
		page, err := service.queries.ShowNativeTask(ctx, "9001154")
		require(t, err)
		store, err := steerline.OpenNativeTaskCommandStore(ctx, &c)
		require(t, err)
		profiles := c.AvailableExecutionProfiles()
		_, err = (appExecutionCommands(t, store, c)).ClaimNativeTask(ctx, c, execution.TaskClaimOptions{TaskID: "9001154", RequestID: newID(), ExpectedTaskVersion: page.Tasks[0].Version, ExpectedAuthorityVersion: page.AuthorityVersion, RequestedBy: "acceptance", Explicit: true, ExecutionProfile: &profiles[0], ServiceInstanceID: service.instanceID})
		require(t, err)
		require(t, store.Close())
	}
	settleCtx, cancel := context.WithTimeout(ctx, 7*time.Minute)
	defer cancel()
	ctx = settleCtx
	answered := map[string]bool{}
	for ctx.Err() == nil {
		view, err := service.queries.ReadProjectView(ctx, "9001154", 100)
		require(t, err)
		if view.Runtime == nil {
			time.Sleep(time.Second)
			continue
		}
		for _, run := range view.Runtime.Runs {
			if run.TaskID != "9001154" {
				continue
			}
			var e *executionmodel.ExecutionRecord
			for i := range view.Runtime.Executions {
				if view.Runtime.Executions[i].ExecutionID == run.ExecutionID {
					e = &view.Runtime.Executions[i]
					break
				}
			}
			if e == nil || e.Environment == nil {
				continue
			}
			if blocker := e.Environment.Blocker; blocker != nil {
				reason := blocker.Fact.Reason
				if reason != "resource_preparation_unverified" && reason != "preparation_dispatch_unverified" && reason != "preparation_unverified" && reason != "agent_input_unverified" {
					t.Fatalf("live readiness blocked: %+v", blocker)
				}
				if run.Decision != nil && run.Decision.Status == "pending" && !answered[run.Decision.ID] {
					nativeLiveAnswer(t, ctx, c, run, "recheck_environment", "")
					answered[run.Decision.ID] = true
				}
			}
			if e.Process == nil {
				continue
			}
			state := integratedEnvironmentState(t, e)
			if len(state.Turns) == 2 && state.Turns[1].Settled {
				if !e.Environment.Ready || !e.Environment.Preparation.Verified || e.TurnNumber != 1 || state.Turns[0].Purpose != "readiness" || state.Turns[1].Purpose != "business" || state.Turns[0].DispatchID == state.Turns[1].DispatchID || state.Turns[1].Outcome != "succeeded" || e.Process.Terminal != e.Environment.Preparation.Probe.Terminal || e.Process.Incarnation != e.Environment.Preparation.Probe.Incarnation {
					t.Fatal("live readiness/business lost its original session or dispatch boundaries")
				}
				report := map[string]any{"captured_at": time.Now().UTC(), "workflow_run_id": run.RunID, "execution_id": e.ExecutionID, "profile": findReadinessLiveProfile(t, view.Runtime, run.RunID), "environment": e.Environment, "turns": state.Turns, "process": e.Process, "state_root": home, "scope": "real readiness probe and settlement, exactly one business Dispatch, original terminal/incarnation, business command and settlement; no PR delivery"}
				if path := os.Getenv("STEERLINE_READINESS_LIVE_REPORT"); path != "" {
					raw, err := json.MarshalIndent(report, "", "  ")
					require(t, err)
					require(t, os.WriteFile(path, raw, 0600))
				}
				t.Logf("real readiness and business settled: %s / %s; isolated state retained at %s", state.Turns[0].DispatchID, state.Turns[1].DispatchID, home)
				// Clean up only the two exact settled terminals owned by this isolated
				// fixture, using checked native identity/close receipts.
				service.stop(t)
				config := c.ApplyFixedExecutionProfile(findReadinessLiveProfile(t, view.Runtime, run.RunID))
				client := orcaworker.New(orcaworker.Config{Repository: config.Repository, CommandTimeout: config.CommandTimeout, Target: e.Target, Output: func(callCtx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
					return (execution.Runner{Dir: config.Repository, Timeout: timeout}).Output(callCtx, executionbackend.Command{Executable: "orca"}, args...)
				}})
				closed, err := client.CloseTerminalChecked(ctx, e.Process.Terminal, e.Process.RuntimeID)
				require(t, err)
				if closed.Disposition != "confirmed" {
					t.Fatal("original agent cleanup was not confirmed")
				}
				closed, err = client.CloseTerminalChecked(ctx, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
				require(t, err)
				if closed.Disposition != "confirmed" {
					t.Fatal("original coordinator cleanup was not confirmed")
				}
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("real readiness acceptance did not settle; preserve original isolated resources: %s", home)
}

func liveReadinessReference(t *testing.T, ctx context.Context, repo, commit, path string) taskmodel.FileReference {
	t.Helper()
	raw, err := exec.CommandContext(ctx, "git", "-C", repo, "show", commit+":"+path).Output()
	require(t, err)
	sum := sha256.Sum256(raw)
	return taskmodel.FileReference{Path: path, Commit: commit, SHA256: hex.EncodeToString(sum[:])}
}

func findReadinessLiveProfile(t *testing.T, runtime *steerline.Runtime, runID string) executionmodel.FixedExecutionProfile {
	t.Helper()
	var profile executionmodel.FixedExecutionProfile
	require(t, json.Unmarshal(integrationFixedProfileJSON(t, runtime, runID), &profile))
	return profile
}

func startReadinessLiveService(t *testing.T, ctx context.Context, c execution.Config) *runningIntegrationService {
	t.Helper()
	service, err := steerlineapp.NewService(c)
	require(t, err)
	reader, err := steerline.OpenNativeTaskReader(ctx, &c)
	require(t, err)
	runCtx, cancel := context.WithCancel(ctx)
	result := &runningIntegrationService{service: service, config: c, reader: reader, queries: productquery.NewService(reader, c.Identity()), cancel: cancel, done: make(chan error, 1)}
	t.Cleanup(func() { result.stop(t) })
	go func() { result.done <- steerline.RunService(runCtx, service, service.Config, "manual") }()
	waitFor(t, 30*time.Second, func() bool {
		view, err := result.queries.ReadProjectView(ctx, "9001154", 100)
		if err == nil && view.Runtime != nil && view.Runtime.Service.Ready {
			result.instanceID = view.Runtime.Service.InstanceID
			return true
		}
		return false
	})
	return result
}
