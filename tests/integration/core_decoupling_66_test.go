package integration

import (
	"bytes"
	"context"
	"encoding/json"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"os"
	"strings"
	"testing"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func changedIntegrationConfig(t *testing.T, fixture integrationFixture) steerlineapp.Config {
	t.Helper()
	raw, err := os.ReadFile(steerlineapp.GlobalExecutionConfigPath())
	require(t, err)
	var global struct {
		Version  int                          `json:"version"`
		Default  string                       `json:"default_execution_profile"`
		Profiles []execution.ExecutionProfile `json:"execution_profiles"`
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

func integrationFixedProfileJSON(t *testing.T, state *reconstructiontypes.Runtime, id string) []byte {
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
		_, err = steerlineapp.NewCommandService(store, config)
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
		reader, err := steerlineapp.OpenNativeTaskReader(ctx, &config)
		require(t, err)
		defer reader.Close()
		_, before, err := reader.DatabaseVersion(ctx)
		require(t, err)
		view, err := productquery.NewService(reader, config.Identity()).ReadProjectView(ctx, "907", 500)
		require(t, err)
		if view.SelectedTask == nil {
			t.Fatal("offline query lost the persisted selected task")
		}
		refs, options, err := steerlineapp.PrepareConsole(ctx, fixture.configPath, "")
		require(t, err)
		if len(refs) != 1 || refs[0].Error != "" || len(refs[0].Presentation.Profiles) != 0 {
			t.Fatalf("offline console fabricated backend profiles: %+v", refs)
		}
		consoleClient := options.Clients[refs[0].Key]
		defer consoleClient.Close()
		offline := consoleClient.ReadProject(ctx, "907", productapi.ProjectSnapshot{})
		if offline.Connection.State != productapi.ConnectionOffline || offline.Connection.CanControl || offline.SelectedNative != nil || offline.NativeTasks != nil {
			t.Fatal("cold HTTP console read backend data while offline")
		}
		if _, err := steerlineapp.OpenNativeTaskCommandStore(ctx, &config); err == nil {
			t.Fatal("unknown support admitted a writable command store")
		}
		_, after, err := reader.DatabaseVersion(ctx)
		require(t, err)
		if before != after {
			t.Fatal("offline reads or rejection changed database revision")
		}
	})
	t.Run("service_entries_reject_missing_catalog_before_target", func(t *testing.T) {
		state := t.TempDir() + "/absent"
		config := steerlineapp.Config{TaskAuthority: "native", WorkflowProtocol: executionmodel.Version, Repository: "must-not-be-read", RunDir: state, Git: executionbackend.Command{Executable: "must-not-be-called"}}
		if _, err := steerlineapp.StartConfiguredService(context.Background(), config, "unused.json", steerline.StartServiceOptions{}); err == nil || !strings.Contains(err.Error(), "directory") {
			t.Fatalf("start reached target without directory: %v", err)
		}
		if err := steerlineapp.RunConfiguredService(context.Background(), config, "manual", "", "off"); err == nil || !strings.Contains(err.Error(), "directory") {
			t.Fatalf("run reached target without directory: %v", err)
		}
		if _, err := os.Stat(state); !os.IsNotExist(err) {
			t.Fatal("rejected startup created state")
		}
	})
}

var _ executionbackend.LifecycleResolver = (*integrationLifecycleResolver)(nil)
