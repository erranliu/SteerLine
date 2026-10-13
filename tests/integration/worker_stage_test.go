package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	terminal "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func TestWorkerStageLocalRoundTrip(t *testing.T) { testWorkerStageRoundTrip(t, "local") }

func TestWorkerStageLocalPublishReport(t *testing.T) {
	t.Setenv("STEERLINE_STAGE_PUBLISH_PLAIN", "1")
	testWorkerStageRun(t, "local")
}

func testWorkerStageRoundTrip(t *testing.T, host string) { testWorkerStageRun(t, host) }
func testWorkerStageRun(t *testing.T, host string) {
	gate := os.Getenv("STEERLINE_STAGE_NATIVE_GATE") == "1"
	steps := []string{executionmodel.StepImplement, executionmodel.StepReport, executionmodel.StepPublish}
	if os.Getenv("STEERLINE_STAGE_PUBLISH_PLAIN") == "1" {
		steps = append(steps, executionmodel.StepPublishReport)
	}
	turns := len(steps)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	f := newLifecycleProjectBase(t, "local", false)
	provider := filepath.Join(filepath.Dir(f.configPath), "stage-provider.json")
	t.Setenv("STEERLINE_STAGE_PROVIDER", provider)
	raw, err := os.ReadFile(f.configPath)
	require(t, err)
	var config map[string]any
	require(t, json.Unmarshal(raw, &config))
	config["pr"] = map[string]any{"kind": "command", "base_url": "https://github.com", "repository": "owner/repo", "query": executionbackend.Command{Executable: os.Args[0], Args: []string{"--fake", "staged-pr"}}}
	raw, err = json.Marshal(config)
	require(t, err)
	require(t, os.WriteFile(f.configPath, raw, 0600))
	f.config, err = steerlineapp.LoadConfig(f.configPath)
	require(t, err)
	if host != "local" {
		environment := ""
		if host == "orca-environment" {
			environment = "paired-fixture"
		}
		require(t, os.MkdirAll(f.nativeStateDir, 0700))
		require(t, AtomicWrite(filepath.Join(f.nativeStateDir, "transport.json"), mustJSON(terminalFixture{NativeGate: gate, Repository: f.repository, Environment: environment, Agent: "codex", Incarnation: "original", CommandMode: os.Getenv("STEERLINE_STAGE_AGENT_LAUNCH") == "1", CollapsedInput: os.Getenv("STEERLINE_STAGE_COLLAPSED_INPUT") == "1"})))
		if gate {
			require(t, AtomicWrite(filepath.Join(f.nativeStateDir, "gate-phase"), []byte("working")))
		}
		f.config.Host, f.config.Agent = "orca", "codex"
		setNativeImplementationProfiles(t, &f.config, codexworker.ProfileConfig{}, &terminal.Profile{Executable: os.Args[0], Args: []string{"--fake", "terminal-orca", f.nativeStateDir}, RepositorySelector: "id:fixture", Environment: environment, WaitTimeout: "5s"})
		f.config.ExecutionDirectory, err = steerlineapp.NewBuiltinDirectoryWithOrcaStateFactory(f.config.ProcessHost, integrationStateFactory(f.nativeStateDir, nil))
		require(t, err)
		t.Cleanup(func() {
			if t.Failed() {
				data, _ := os.ReadFile(filepath.Join(f.nativeStateDir, "transport.json"))
				t.Logf("original terminal fixture: %s", data)
			}
		})
	}
	project, err := steerlineapp.OpenNativeTaskProject(ctx, &f.config)
	require(t, err)
	require(t, project.Close())

	createIntegrationTask(t, ctx, f, 901)
	if host == "local" && turns == 3 {
		createIntegrationTask(t, ctx, f, 902, 901)
	}
	resolver := &integrationLifecycleResolver{}
	var publishQueries atomic.Int32
	if gate {
		resolver.stateFactory = integrationStateFactory(f.nativeStateDir, func() {
			raw, _ := os.ReadFile(filepath.Join(f.nativeStateDir, "transport.json"))
			var state terminalFixture
			if json.Unmarshal(raw, &state) == nil && state.Sends == 3 {
				publishQueries.Add(1)
			}
		})
	}
	service := startIntegrationService(t, ctx, f.config, resolver)
	defer service.stop(t)
	claimIntegrationTask(t, ctx, f, 901, service.instanceID)
	if gate {
		working := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return run.WorkflowStage == executionmodel.StepPublish && publishQueries.Load() >= 1 && e.CurrentTurnID != "" && e.CurrentTurnID == e.CurrentCommandID && e.LastStoppedTurnID != e.CurrentTurnID
		})
		if len(working.runtime.WorkflowRuns[0].StageInputs) != 3 || len(working.runtime.WorkflowRuns[0].StageResults) != 2 || working.execution.TurnEndedAt != nil {
			t.Fatal("working publish accepted the old report")
		}
		// Let the old screen settle for longer than the removed screen heuristic.
		select {
		case <-time.After(2200 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		still := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return run.WorkflowStage == executionmodel.StepPublish
		})
		if len(still.runtime.WorkflowRuns[0].StageResults) != 2 || publishQueries.Load() != 1 {
			t.Fatal("stable old report or duplicate hint advanced/requeried")
		}
		require(t, AtomicWrite(filepath.Join(f.nativeStateDir, "gate-phase"), []byte("ended")))
		ended := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return run.WorkflowStatus == "reconciling" && e.TurnEndedAt != nil && publishQueries.Load() >= 2
		})
		if len(ended.runtime.WorkflowRuns[0].StageInputs) != 3 || len(ended.runtime.WorkflowRuns[0].StageResults) != 2 || ended.execution.LastTurnOutcome != "unknown" {
			t.Fatal("native end froze an old reply or declared success")
		}
		require(t, AtomicWrite(filepath.Join(f.nativeStateDir, "gate-phase"), []byte("reply")))
	}

	ready := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.WorkflowStatus == "waiting_external" && run.WorkflowStage == executionmodel.StepAwaitMerge
	}, 3*time.Minute)
	w := ready.runtime.WorkflowRuns[0]
	if len(w.StageInputs) != turns || len(w.StageResults) != turns || ready.execution.TurnNumber != uint64(turns) || len(ready.runtime.Attempts) != 1 || ready.execution.PendingCommandID != "" {
		t.Fatalf("native chain has unexpected input count: %+v", w)
	}
	for i, step := range steps {
		input, result := w.StageInputs[i], w.StageResults[i]
		inputValues := 0
		for _, operation := range ready.runtime.Operations {
			if operation.Kind == "worker_input_value" {
				var sent executionbackend.TurnInput
				if json.Unmarshal(operation.Request, &sent) == nil && sent.CommandID == input.CommandID {
					inputValues++
					if sent.Purpose != "business" || sent.StepID != step || sent.InputVersion != input.Version {
						t.Fatal("business step was confused with native purpose")
					}
				}
			}
		}
		if host != "local" && result.NativeOutcome != "unknown" {
			t.Fatal("missing native outcome was upgraded to success")
		}
		if inputValues != 1 {
			t.Fatal("actual native input does not have one frozen value")
		}
		if input.StepID != step || result.StepID != step || input.CommandID != result.CommandID || input.Version != result.InputVersion || input.AttemptID != ready.run.AttemptID || result.SessionID != ready.execution.SessionID || result.FinalSource == "" || !result.Succeeded {
			t.Fatalf("input/final provenance differs at %s: %+v / %+v", step, input, result)
		}
	}
	if w.StageInputs[1].SourceCommandID != w.StageResults[0].CommandID || w.StageInputs[1].SourceTurnID != w.StageResults[0].TurnID {
		t.Fatal("report lost source binding")
	}
	if turns == 4 && (w.StageInputs[3].SourceCommandID != w.StageResults[2].CommandID || w.StageInputs[3].SourceTurnID != w.StageResults[2].TurnID) {
		t.Fatal("publication report lost original binding")
	}
	if strings.HasPrefix(host, "orca") {
		var captured terminalFixture
		raw, err := os.ReadFile(filepath.Join(f.nativeStateDir, "transport.json"))
		require(t, err)
		require(t, json.Unmarshal(raw, &captured))
		inputs := ready.runtime.WorkflowRuns[0].StageInputs
		if len(captured.Prompts) != len(inputs) {
			t.Fatal("transport input count changed")
		}
		for i, prompt := range captured.Prompts {
			if prompt != inputs[i].Prompt {
				t.Fatalf("Orca changed worker stage %s prompt", inputs[i].StepID)
			}
		}
	}

	if host == "local" && turns == 3 {
		branch, prURL, session := ready.execution.Workspace.Branch, ready.runtime.WorkflowRuns[0].PRURL, ready.execution.SessionID
		firstHead := fixtureGitOutput(t, "--git-dir", f.bareRemote, "rev-parse", "refs/heads/"+branch)
		if ready.run.Decision == nil || ready.run.Decision.Status != "pending" {
			t.Fatal("waiting PR lacks optional feedback")
		}
		submitIntegrationAnswerOption(t, ctx, f.config, ready.run, "implement")
		updated := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return run.WorkflowStatus == "waiting_external" && run.WorkflowStage == executionmodel.StepAwaitMerge && e.TurnNumber == 6
		})
		w := updated.runtime.WorkflowRuns[0]
		secondHead := fixtureGitOutput(t, "--git-dir", f.bareRemote, "rev-parse", "refs/heads/"+branch)
		if firstHead == secondHead || w.PRURL != prURL || updated.execution.SessionID != session || len(w.StageInputs) != 6 || len(w.StageResults) != 6 || len(updated.runtime.Attempts) != 1 {
			t.Fatal("feedback did not update the original branch/PR in the original session")
		}
		for _, i := range []int{3, 5} {
			if !strings.Contains(w.StageInputs[i].Prompt, "Existing PR: "+prURL) {
				t.Fatal("implementation/publication input lost the existing PR")
			}
		}
		turns = 6
	}

	// Restart the completed input chain: neither backend may replay a stage.
	service.stop(t)
	service = startIntegrationService(t, ctx, f.config, resolver)
	defer service.stop(t)
	after := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.WorkflowStatus == "waiting_external"
	})
	if after.execution.TurnNumber != uint64(turns) || len(after.runtime.WorkflowRuns[0].StageInputs) != turns || len(after.runtime.Attempts) != 1 {
		t.Fatal("service restart automatically sent another stage")
	}

	assertNoTaskDelivery(t, after.runtime, after.run)
	if host == "local" && turns == 6 {
		branch, prURL := after.execution.Workspace.Branch, after.runtime.WorkflowRuns[0].PRURL
		secondHead := fixtureGitOutput(t, "--git-dir", f.bareRemote, "rev-parse", "refs/heads/"+branch)
		// Merge the actual updated branch in this isolated Git repository.
		fixtureGitCommand(t, "-C", f.repository, "fetch", "--quiet", f.bareRemote, branch)
		fixtureGitCommand(t, "-C", f.repository, "merge", "--no-ff", "--no-edit", secondHead)
		mergeCommit := fixtureGitOutput(t, "-C", f.repository, "rev-parse", "HEAD")
		fixtureGitCommand(t, "-C", f.repository, "push", "--quiet", f.bareRemote, "HEAD:refs/heads/main")
		raw, err := os.ReadFile(provider)
		require(t, err)
		var info executionmodel.PRInfo
		require(t, json.Unmarshal(raw, &info))
		mergedAt := time.Now().UTC()
		info.HeadRevision, info.State, info.Merged = "", "merged", true
		info.MergedAt, info.MergeCommit = &mergedAt, &mergeCommit
		require(t, AtomicWrite(provider, mustJSON(info)))
		completed := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return run.WorkflowStatus == "completed"
		})
		if len(completed.runtime.Deliveries) != 1 || completed.runtime.Deliveries[0].MergeCommit != mergeCommit || completed.runtime.Deliveries[0].PRURL != prURL {
			t.Fatal("updated PR did not produce its one actual merged delivery")
		}
		readyTasks, err := service.queries.ReadyNativeTasks(ctx)
		require(t, err)
		if len(readyTasks) != 1 || readyTasks[0].TaskID != "902" || len(completed.runtime.WorkflowRuns) != 1 {
			t.Fatal("merged delivery did not release the successor without starting it")
		}
	}
	if os.Getenv("STEERLINE_STAGE_AGENT_LAUNCH") == "1" {
		raw, err := os.ReadFile(filepath.Join(f.nativeStateDir, "transport.json"))
		require(t, err)
		var state terminalFixture
		require(t, json.Unmarshal(raw, &state))
		if state.Launches != 1 || state.Sends != turns {
			t.Fatal("startup or business input repeated", state.Launches, state.Sends)
		}
	}
}

func TestWorkerStageOrcaNativeStateGate(t *testing.T) {
	t.Setenv("STEERLINE_STAGE_NATIVE_GATE", "1")
	for _, host := range []string{"orca", "orca-environment"} {
		t.Run(host, func(t *testing.T) { testWorkerStageRun(t, host) })
	}
}

func TestWorkerStageOrcaTerminalRoundTrip(t *testing.T) { testWorkerStageRoundTrip(t, "orca") }
func TestWorkerStageOrcaEnvironmentRoundTrip(t *testing.T) {
	testWorkerStageRoundTrip(t, "orca-environment")
}
func TestWorkerStageOrcaAgentLaunchLocalRoundTrip(t *testing.T) {
	t.Setenv("STEERLINE_STAGE_AGENT_LAUNCH", "1")
	testWorkerStageRoundTrip(t, "orca")
}
func TestWorkerStageOrcaAgentLaunchEnvironmentRoundTrip(t *testing.T) {
	t.Setenv("STEERLINE_STAGE_AGENT_LAUNCH", "1")
	testWorkerStageRoundTrip(t, "orca-environment")
}

func TestWorkerStageOrcaCollapsedInputRoundTrip(t *testing.T) {
	t.Setenv("STEERLINE_STAGE_COLLAPSED_INPUT", "1")
	t.Setenv("STEERLINE_STAGE_AGENT_LAUNCH", "1")
	for _, host := range []string{"orca", "orca-environment"} {
		t.Run(host, func(t *testing.T) { testWorkerStageRoundTrip(t, host) })
	}
}
