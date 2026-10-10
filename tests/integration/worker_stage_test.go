package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
		require(t, AtomicWrite(filepath.Join(f.nativeStateDir, "transport.json"), mustJSON(terminalFixture{Repository: f.repository, Environment: environment, Agent: "opaque-worker", Incarnation: "original"})))
		f.config.Host, f.config.Agent = "orca", "opaque-worker"
		setNativeImplementationProfiles(t, &f.config, codexworker.ProfileConfig{}, &terminal.Profile{Executable: os.Args[0], Args: []string{"--fake", "terminal-orca", f.nativeStateDir}, RepositorySelector: "id:fixture", Environment: environment, WaitTimeout: "5s"})
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
	resolver := &integrationLifecycleResolver{}
	service := startIntegrationService(t, ctx, f.config, resolver)
	defer service.stop(t)
	claimIntegrationTask(t, ctx, f, 901, service.instanceID)

	ready := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.BusinessStatus == "waiting_external" && run.StepID == executionmodel.StepAwaitMerge
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
		if inputValues != 1 {
			t.Fatal("actual native input does not have one frozen value")
		}
		if input.StepID != step || result.StepID != step || input.CommandID != result.CommandID || input.Version != result.InputVersion || input.AttemptID != ready.run.AttemptID || result.SessionID != ready.execution.SessionID || result.FinalSource == "" || !result.Succeeded {
			t.Fatalf("input/final provenance differs at %s: %+v / %+v", step, input, result)
		}
	}
	if w.StageInputs[1].SourceCommandID != w.StageResults[0].CommandID || w.StageInputs[1].SourceTurnID != w.StageResults[0].TurnID || w.StageResults[0].Snapshot.Workspace.Head != w.StageResults[1].Snapshot.Workspace.Head || w.StageResults[0].Snapshot.Workspace.ContentDigest != w.StageResults[1].Snapshot.Workspace.ContentDigest {
		t.Fatal("report was not bound or wrote business artifacts")
	}
	if w.CandidateSHA != fixtureGitOutput(t, "--git-dir", f.bareRemote, "rev-parse", "refs/heads/"+ready.execution.Workspace.Branch) {
		t.Fatal("published head differs from frozen candidate")
	}
	if turns == 4 && (w.StageInputs[3].SourceCommandID != w.StageResults[2].CommandID || w.StageInputs[3].SourceTurnID != w.StageResults[2].TurnID || w.StageInputs[3].CandidateSHA != w.CandidateSHA || w.StageResults[2].Snapshot.Workspace.ContentDigest != w.StageResults[3].Snapshot.Workspace.ContentDigest) {
		t.Fatal("publication report lost original binding or changed workspace")
	}
	// Restart the completed input chain: neither backend may replay a stage.
	service.stop(t)
	service = startIntegrationService(t, ctx, f.config, resolver)
	defer service.stop(t)
	after := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.BusinessStatus == "waiting_external"
	})
	if after.execution.TurnNumber != uint64(turns) || len(after.runtime.WorkflowRuns[0].StageInputs) != turns || len(after.runtime.Attempts) != 1 {
		t.Fatal("service restart automatically sent another stage")
	}

	assertNoTaskDelivery(t, after.runtime, after.run)
}

func TestWorkerStageOrcaTerminalRoundTrip(t *testing.T) { testWorkerStageRoundTrip(t, "orca") }
func TestWorkerStageOrcaEnvironmentRoundTrip(t *testing.T) {
	testWorkerStageRoundTrip(t, "orca-environment")
}
