package steerline

import (
	"context"
	"os"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestCD66ExplicitCatalogAndEntrypoints(t *testing.T) {
	t.Run("resource_candidates", TestCD66CandidateResourceIdentityAndCommitRollback)
	t.Run("missing_readiness_rejects_before_reopen_write", func(t *testing.T) {
		service, run := neutralAuditService(t)
		entry, err := registrationFor(service.Config)
		require(t, err)
		entry.ReadinessRequired = func(Config) bool { return true }
		config := service.Config
		config.ExecutionDirectory = neutralDirectoryWithReplacement(t, config.ExecutionDirectory, entry)
		if service.executionRecord(run).Environment != nil {
			t.Fatal("fixture must lack required preparation evidence")
		}
		_, before, err := service.domainStore.DatabaseVersion(context.Background())
		require(t, err)
		if err := preflightWorkerFlowState(context.Background(), config); err == nil || !strings.Contains(err.Error(), "preparation intent") {
			t.Fatalf("missing original readiness was accepted: %v", err)
		}
		_, after, err := service.domainStore.DatabaseVersion(context.Background())
		require(t, err)
		if before != after {
			t.Fatal("failed recovery preflight wrote persistent state")
		}
	})
	t.Run("service_entries_reject_missing_catalog_before_target", func(t *testing.T) {
		state := t.TempDir() + "/absent"
		config := Config{Persistence: sqlite.Factory{}, TaskAuthority: "native", WorkflowProtocol: executionmodel.Version, Repository: "must-not-be-read", RunDir: state, Git: executionbackend.Command{Executable: "must-not-be-called"}}
		if _, err := StartService(context.Background(), config, "unused.json", StartServiceOptions{}); err == nil || !strings.Contains(err.Error(), "directory") {
			t.Fatalf("start reached target without directory: %v", err)
		}
		if err := RunConfiguredService(context.Background(), config, "manual", "", "off", func(Config) (*Service, error) { t.Fatal("rejected startup called constructor"); return nil, nil }); err == nil || !strings.Contains(err.Error(), "directory") {
			t.Fatalf("run reached target without directory: %v", err)
		}
		if _, err := os.Stat(state); !os.IsNotExist(err) {
			t.Fatal("rejected startup created state")
		}
	})
}

func TestCD66CandidateResourceIdentityAndCommitRollback(t *testing.T) {
	t.Run("missing_workspace_root_has_no_core_default", func(t *testing.T) {
		c := Config{Persistence: sqlite.Factory{}, Repository: "must-not-be-read", identity: "project", RunDir: t.TempDir()}
		if err := c.prepareWorktreeRoot(); err == nil || !strings.Contains(err.Error(), "explicit worktree_root") {
			t.Fatalf("missing app path acquired a core default: %v", err)
		}
		if c.WorktreeRoot != "" {
			t.Fatal("rejected path construction changed configuration")
		}
	})
	t.Run("resource_builder_cannot_change_original_owner", func(t *testing.T) {
		config := workerContextConfigFixture()
		entry, err := registrationFor(config)
		require(t, err)
		entry.BuildPolicyResource = func(_ context.Context, request PolicyResourceBuildRequest) (PolicyResourceBuildResult, error) {
			request.Resource.ProjectID = "foreign"
			result, err := neutralBuildPolicyResource(context.Background(), request)
			return PolicyResourceBuildResult{Envelope: result.Envelope, WriteRoots: result.WriteRoots}, err
		}
		config.ExecutionDirectory = neutralDirectoryWithReplacement(t, config.ExecutionDirectory, entry)
		record := executionmodel.ExecutionRecord{ProjectIdentity: "project", ExecutionID: newID(), CurrentAttemptID: newID(), Generation: newID()}
		if err := bindPolicyResources(context.Background(), config, &record, "workspace", nil); err == nil {
			t.Fatal("foreign resource owner was accepted")
		}
		if record.PolicyResources != nil {
			t.Fatal("rejected resource became a center binding")
		}
	})
	t.Run("invalid_binding_restores_private_candidates", func(t *testing.T) {
		config := workerContextConfigFixture()
		service := Service{Config: config, committed: DomainSnapshot{Tasks: []taskmodel.TaskRecord{{TaskID: "task", Status: "open"}}}, domain: DomainSnapshot{Tasks: []taskmodel.TaskRecord{{TaskID: "task", Status: "closed"}}}, pendingOperations: map[string]string{"pending": "run"}}
		ref := executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendLocal, Protocol: neutralProtocol, Agent: executionbackend.AgentSpec{Name: "fixture"}, Target: targetFor(config), RunID: newID(), AttemptID: newID(), WorkspacePath: "workspace", WorkspaceBranch: "fixture/task", BaseCommit: strings.Repeat("a", 40), External: executionbackend.ProcessIdentity{Kind: "os", Created: "original", Executable: "never-run"}}
		if err := service.commitWorkerAttached(&Run{RunID: "run"}, workerCommitAttachment{Binding: &ref}, "pending"); err == nil {
			t.Fatal("incomplete resource identity was committed")
		}
		if service.domain.Tasks[0].Status != "open" || service.pendingOperations["pending"] != "" {
			t.Fatal("validation failure leaked a private candidate or operation")
		}
	})
}
