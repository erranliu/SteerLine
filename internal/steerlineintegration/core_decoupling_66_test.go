package steerlineintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
	taskapp "github.com/erranliu/SteerLine/internal/task"
)

func TestCD66FixedProfileRecovery(t *testing.T) {
	t.Run("original_app_server_two_phase_turns", testIntegratedAppServerTwoTurns)
	t.Run("changed_config_keeps_native_original_identity_and_manual_gate", func(t *testing.T) { testIntegratedNativeSessionGate(t, true) })
}

func changedIntegrationConfig(t *testing.T, fixture integrationFixture) steerline.Config {
	t.Helper()
	raw, err := os.ReadFile(steerlineapp.GlobalExecutionConfigPath())
	require(t, err)
	var global struct {
		Version  int                          `json:"version"`
		Default  string                       `json:"default_execution_profile"`
		Profiles []steerline.ExecutionProfile `json:"execution_profiles"`
	}
	require(t, json.Unmarshal(raw, &global))
	for i := range global.Profiles {
		global.Profiles[i].Name = "Changed current selection"
		for j := range global.Profiles[i].ImplementationConfigs {
			input := &global.Profiles[i].ImplementationConfigs[j]
			if input.ImplementationKey == codexworker.ProfileCodecKey {
				settings, err := codexworker.DecodeProfileConfig(input.Payload)
				require(t, err)
				settings.Model = "changed-current-model"
				input.Payload, err = codexworker.EncodeProfileConfig(settings)
				require(t, err)
			}
		}
	}
	raw, err = json.Marshal(global)
	require(t, err)
	require(t, os.WriteFile(steerlineapp.GlobalExecutionConfigPath(), raw, 0600))
	config, err := steerlineapp.LoadConfig(fixture.configPath)
	require(t, err)
	require(t, config.ResolveIdentity(context.Background()))
	if config.ExecutionDirectory == fixture.config.ExecutionDirectory || config.LoadedConfigDigest() == fixture.config.LoadedConfigDigest() {
		t.Fatal("changed config reused the old catalog instance or accepted digest")
	}
	return config
}

func integrationFixedProfileJSON(t *testing.T, state *steerline.Runtime, id string) []byte {
	t.Helper()
	for _, workflow := range state.WorkflowRuns {
		if workflow.WorkflowRunID == id {
			raw, err := json.Marshal(workflow.ExecutionProfile)
			require(t, err)
			return raw
		}
	}
	t.Fatal("original workflow profile was not queryable")
	return nil
}

func TestCD66ExplicitCatalogAndEntrypoints(t *testing.T) {
	t.Run("missing_catalog_claim_rejects_before_target_or_write", func(t *testing.T) {
		fixture := newLifecycleProject(t, "local")
		ctx := context.Background()
		createIntegrationTask(t, ctx, fixture, 906)
		store, err := sqlite.OpenSQLiteStore(fixture.config.RunDir + string(os.PathSeparator) + "steerline.db")
		require(t, err)
		defer store.Close()
		_, before, err := store.DatabaseVersion(ctx)
		require(t, err)
		config := fixture.config
		config.ExecutionDirectory = nil
		config.Git.Executable = "must-not-be-called"
		_, err = (steerline.NewCommandService(store, config.Identity(), taskapp.Service{Reader: store}, taskapp.Service{Reader: store})).ClaimNativeTask(ctx, config, steerline.TaskClaimOptions{TaskID: "906", RequestID: strings.Repeat("a", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 1})
		if err == nil || !strings.Contains(err.Error(), "directory") || strings.Contains(err.Error(), "must-not-be-called") {
			t.Fatalf("missing directory reached a target or claim: %v", err)
		}
		_, after, err := store.DatabaseVersion(ctx)
		require(t, err)
		if before != after {
			t.Fatal("rejected claim changed the database")
		}
	})
	t.Run("offline_persisted_query_and_console_survive_missing_support", func(t *testing.T) {
		fixture := newLifecycleProject(t, "local")
		ctx := context.Background()
		createIntegrationTask(t, ctx, fixture, 907)
		raw, err := os.ReadFile(steerlineapp.GlobalExecutionConfigPath())
		require(t, err)
		raw = bytes.ReplaceAll(raw, []byte(codexworker.ProfileCodecKey), []byte("unavailable.implementation"))
		require(t, os.WriteFile(steerlineapp.GlobalExecutionConfigPath(), raw, 0600))
		if _, err := steerlineapp.LoadConfig(fixture.configPath); err == nil {
			t.Fatal("mutation loader accepted an unsupported codec")
		}
		config, err := steerlineapp.LoadQueryConfig(fixture.configPath)
		require(t, err)
		reader, err := steerline.OpenNativeTaskReader(ctx, &config)
		require(t, err)
		defer reader.Close()
		_, before, err := reader.DatabaseVersion(ctx)
		require(t, err)
		view, err := steerline.NewQueryService(reader, config.Identity()).ReadProjectView(ctx, "907", 500)
		require(t, err)
		if view.SelectedTask == nil {
			t.Fatal("offline query lost the persisted selected task")
		}
		refs, err := steerlineapp.LoadConsoleProjects(fixture.configPath)
		require(t, err)
		if len(refs) != 1 || refs[0].Error != "" || refs[0].Presentation.Profiles[0].UnavailableReason == "" {
			t.Fatalf("console failed to show unknown support: %+v", refs)
		}
		if _, err := steerline.OpenNativeTaskCommandStore(ctx, &config); err == nil {
			t.Fatal("unknown support admitted a writable command store")
		}
		_, after, err := reader.DatabaseVersion(ctx)
		require(t, err)
		if before != after {
			t.Fatal("offline reads or rejection changed database revision")
		}
	})
	t.Run("capability_evidence_keeps_original_target_and_versions", TestNativeEvidenceCertificationRequiresFixedTargetAndVersion)
	t.Run("terminal_evidence_keeps_agent_and_resource_proofs", TestOrcaCertificationRequiresAgentAndTerminalEvidence)
}

var _ executionbackend.LifecycleResolver = (*integrationLifecycleResolver)(nil)
