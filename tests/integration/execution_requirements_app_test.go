package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestAppRequirementsClaimFreezesEvaluatedProfile(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		name := "default_profile"
		if explicit {
			name = "explicit_selected_profile"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			fixture := newLifecycleProject(t, "local")

			tokenEnv, tokenValue := "STEERLINE_APP_REQUIREMENTS_CLAIM_TOKEN", "non-secret-test-token-value"
			t.Setenv(tokenEnv, tokenValue)
			projectBytes, err := os.ReadFile(fixture.configPath)
			require(t, err)
			var project map[string]any
			require(t, json.Unmarshal(projectBytes, &project))
			project["execution_requirements"] = executionbackend.ExecutionRequirements{
				Tools:       []executionbackend.ToolRequirement{{ID: "git"}},
				Credentials: []executionbackend.CredentialRequirement{{ID: "pr.token_env", Site: "service"}},
			}
			provider := fixture.config.PR
			provider.TokenEnv = tokenEnv
			project["pr"] = provider
			projectBytes, err = json.Marshal(project)
			require(t, err)
			require(t, os.WriteFile(fixture.configPath, projectBytes, 0600))

			config, err := steerlineapp.LoadConfig(fixture.configPath)
			require(t, err)
			require(t, config.ResolveIdentity(ctx))
			acceptedVersion := config.LoadedConfigDigest()
			evaluation, err := config.EvaluateRequirements(ctx)
			require(t, err)
			if len(evaluation.Tools) != 1 || evaluation.Tools[0].ID != "git" || evaluation.Tools[0].Status != executionbackend.RequirementPresent ||
				len(evaluation.Credentials) != 1 || evaluation.Credentials[0].ID != "pr.token_env" || evaluation.Credentials[0].Status != executionbackend.RequirementPresent {
				t.Fatalf("actual loaded app did not observe required Git/credential as present: %+v", evaluation)
			}

			fixture.config = config
			taskID := int64(901)
			createIntegrationTask(t, ctx, fixture, taskID)
			store, err := steerlineapp.OpenNativeTaskCommandStore(ctx, &config)
			require(t, err)
			defer func() { require(t, store.Close()) }()
			commands, err := steerlineapp.NewCommandService(store, config)
			require(t, err)
			graph, err := store.TaskGraphSnapshot(ctx, config.Identity())
			require(t, err)
			task := graph.ByID["901"]
			if task.TaskID == "" {
				t.Fatal("actual app task was not created before claim")
			}

			options := execution.TaskClaimOptions{
				TaskID: "901", RequestID: newID(), ExpectedTaskVersion: task.Version,
				ExpectedAuthorityVersion: graph.Authority.AuthorityVersion, ExpectedGraphRevision: graph.Authority.GraphRevision,
				RequestedBy: "operator", Explicit: true,
			}
			selected := config.AvailableExecutionProfiles()[0].Freeze(config.ExecutionDirectory, config.Identity())
			if execution.VerifyFixedExecutionProfile(selected, config.ExecutionDirectory) {
				t.Fatal("strict frozen-profile verification accepted a nonempty requirement profile before evaluation")
			}
			if explicit {
				options.ExecutionProfile = &selected
			}
			selectionDigest := selected.Digest
			receipt, err := commands.ClaimNativeTask(ctx, config.ExecutionConfig(), options)
			require(t, err)
			if receipt.Status != "claim_reserved" || receipt.WorkflowRunID == "" {
				t.Fatalf("actual requirements claim did not reserve one workflow: %+v", receipt)
			}

			snapshot, err := store.Query(ctx, config.Identity())
			require(t, err)
			var frozen *executionmodel.FixedExecutionProfile
			var storedExecution *executionmodel.ExecutionRecord
			for i := range snapshot.WorkflowRuns {
				if snapshot.WorkflowRuns[i].WorkflowRunID == receipt.WorkflowRunID {
					frozen = snapshot.WorkflowRuns[i].ExecutionProfile
					break
				}
			}
			for i := range snapshot.Executions {
				if snapshot.Executions[i].WorkflowRunID == receipt.WorkflowRunID {
					storedExecution = &snapshot.Executions[i]
					break
				}
			}
			if frozen == nil || storedExecution == nil {
				t.Fatal("claim snapshot omitted its frozen profile or execution")
			}
			if frozen.ID != selected.ID || frozen.AcceptedConfigVersion != acceptedVersion || frozen.PolicySource == "" ||
				frozen.RequirementsEvaluation == nil || len(frozen.RequirementsEvaluation.Tools) != 1 ||
				frozen.RequirementsEvaluation.Tools[0].ID != "git" || frozen.RequirementsEvaluation.Tools[0].Status != executionbackend.RequirementPresent ||
				len(frozen.RequirementsEvaluation.Credentials) != 1 || frozen.RequirementsEvaluation.Credentials[0].ID != "pr.token_env" || frozen.RequirementsEvaluation.Credentials[0].Status != executionbackend.RequirementPresent ||
				frozen.RequirementsEvidenceDigest == "" || frozen.Digest == selectionDigest || storedExecution.ConfigurationDigest != frozen.Digest {
				t.Fatalf("claim did not freeze the evaluated profile while preserving its accepted selection: profile=%+v execution=%+v selection_digest=%s", frozen, storedExecution, selectionDigest)
			}
			if !execution.VerifyFixedExecutionProfile(*frozen, config.ExecutionDirectory) {
				t.Fatalf("persisted evaluated profile failed strict frozen-profile verification: %+v", frozen)
			}
			profileJSON, err := json.Marshal(frozen)
			require(t, err)
			if bytes.Contains(profileJSON, []byte(tokenValue)) {
				t.Fatal("stored profile evidence contains the environment credential value")
			}
			profileJSONDigest := sha256.Sum256(profileJSON)
			t.Logf("evaluated claim profile explicit=%t selected_digest=%s frozen_digest=%s evidence_digest=%s accepted_version=%s profile_json_sha256=%s tool_status=%s", explicit, selectionDigest, frozen.Digest, frozen.RequirementsEvidenceDigest, frozen.AcceptedConfigVersion, hex.EncodeToString(profileJSONDigest[:]), frozen.RequirementsEvaluation.Tools[0].Status)

			replayed, err := commands.ClaimNativeTask(ctx, config.ExecutionConfig(), options)
			require(t, err)
			if replayed != receipt {
				t.Fatalf("same original request and pre-evaluation selection digest did not replay its receipt: first=%+v replay=%+v", receipt, replayed)
			}
		})
	}
}
