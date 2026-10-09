package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	testprotocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
)

func TestWorkerStageLocalRoundTrip(t *testing.T) { testWorkerStageRoundTrip(t, "local") }
func TestWorkerStageOrcaRoundTrip(t *testing.T)  { testWorkerStageRoundTrip(t, "orca") }

func TestWorkerStageOrcaUnknownReport(t *testing.T) { testWorkerStageRun(t, "orca", true) }

func testWorkerStageRoundTrip(t *testing.T, host string) { testWorkerStageRun(t, host, false) }
func testWorkerStageRun(t *testing.T, host string, unknown bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	f := newLifecycleProjectBase(t, host, false)
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
	project, err := steerlineapp.OpenNativeTaskProject(ctx, &f.config)
	require(t, err)
	require(t, project.Close())
	if host == "orca" {
		state := testprotocol.NativeOrcaState{EnvironmentMode: true, FirstBusinessDispatch: 2, StageSourceLost: true, StageProvider: provider, StageRemote: f.bareRemote, StageTranscript: filepath.Join(f.nativeStateDir, "stage-transcript.jsonl")}
		if unknown {
			state.LoseDispatchAt = 3
		}
		require(t, testprotocol.WriteNativeOrcaState(f.nativeStateDir, state))
	}
	createIntegrationTask(t, ctx, f, 901)
	resolver := &integrationLifecycleResolver{}
	service := startIntegrationService(t, ctx, f.config, resolver)
	defer service.stop(t)
	claimIntegrationTask(t, ctx, f, 901, service.instanceID)
	if unknown {
		stopped := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return run.StepID == executionmodel.StepPublish && run.BusinessStatus == "waiting_input" && e.CurrentTurnID == e.LastStoppedTurnID
		})
		w := stopped.runtime.WorkflowRuns[0]
		if stopped.execution.TurnNumber != 2 || len(w.StageResults) != 2 || len(w.StageInputs) != 3 || w.StageInputs[2].StepID != executionmodel.StepPublish {
			t.Fatal("unknown report skipped facts or automatically published")
		}
		service.stop(t)
		service = startIntegrationService(t, ctx, f.config, resolver)
		defer service.stop(t)
		recovered := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return e.RecoveryRequired && run.StepID == executionmodel.StepPublish
		})
		native, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		if native.Dispatches != 3 || recovered.execution.TurnNumber != 2 || len(recovered.runtime.Attempts) != 1 {
			t.Fatal("reopen blindly reinjected an unknown or queued input")
		}
		return
	}
	ready := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.BusinessStatus == "waiting_external" && run.StepID == executionmodel.StepAwaitMerge
	})
	w := ready.runtime.WorkflowRuns[0]
	if len(w.StageInputs) != 3 || len(w.StageResults) != 3 || ready.execution.TurnNumber != 3 || len(ready.runtime.Attempts) != 1 || ready.execution.PendingCommandID != "" {
		t.Fatalf("native chain is not exactly three original inputs: %+v", w)
	}
	for i, step := range []string{executionmodel.StepImplement, executionmodel.StepReport, executionmodel.StepPublish} {
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
	// Restart the completed input chain: neither backend may replay a stage.
	service.stop(t)
	service = startIntegrationService(t, ctx, f.config, resolver)
	defer service.stop(t)
	after := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.BusinessStatus == "waiting_external"
	})
	if after.execution.TurnNumber != 3 || len(after.runtime.WorkflowRuns[0].StageInputs) != 3 || len(after.runtime.Attempts) != 1 {
		t.Fatal("service restart automatically sent another stage")
	}
	if host == "orca" {
		native, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		if native.Dispatches != 4 || len(native.StageFinals) != 3 {
			t.Fatal("unknown report Dispatch was blindly reinjected")
		}
	}
	assertNoTaskDelivery(t, after.runtime, after.run)
}
