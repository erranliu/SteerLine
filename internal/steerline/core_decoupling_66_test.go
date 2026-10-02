package steerline

import (
	"context"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func TestCD66CandidateResourceIdentityAndCommitRollback(t *testing.T) {
	t.Run("missing_workspace_root_has_no_core_default", func(t *testing.T) {
		c := Config{Repository: "must-not-be-read", identity: "project", RunDir: t.TempDir()}
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
			envelope, err := codexworker.NewPolicyResourceEnvelope(codexworker.PolicyResourceIdentity{ProjectID: "foreign", ExecutionID: request.Resource.ExecutionID, AttemptID: request.Resource.AttemptID, Generation: request.Resource.Generation, ConfigDigest: request.Resource.ConfigDigest}, codexworker.PolicyResourcePayload{NativeVersion: "codex-cli 0.159.2", Contract: codexworker.PolicyContract, Settings: executionapi.LaunchCommand{Executable: "never-run", WorkingDirectory: request.Workspace}})
			return PolicyResourceBuildResult{Envelope: envelope, WriteRoots: []string{request.Workspace}}, err
		}
		config.ExecutionDirectory = testDirectoryWithReplacement(t, config.ExecutionDirectory, entry)
		record := ExecutionRecord{ProjectIdentity: "project", ExecutionID: newID(), CurrentAttemptID: newID(), Generation: newID()}
		if err := bindPolicyResources(context.Background(), config, &record, "workspace", nil); err == nil {
			t.Fatal("foreign resource owner was accepted")
		}
		if record.PolicyResources != nil {
			t.Fatal("rejected resource became a center binding")
		}
	})
	t.Run("invalid_binding_restores_private_candidates", func(t *testing.T) {
		config := workerContextConfigFixture()
		service := Service{Config: config, committed: DomainSnapshot{Tasks: []TaskRecord{{TaskID: "task", Status: "open"}}}, domain: DomainSnapshot{Tasks: []TaskRecord{{TaskID: "task", Status: "closed"}}}, pendingOperations: map[string]string{"pending": "run"}}
		ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalCodexAppServer, Agent: executionapi.AgentSpec{Name: "codex"}, Target: targetFor(config), RunID: newID(), AttemptID: newID(), WorkspacePath: "workspace", WorkspaceBranch: "fixture/task", BaseCommit: strings.Repeat("a", 40), External: executionapi.ProcessIdentity{Kind: "os", Created: "original", Executable: "never-run"}}
		if err := service.commitWorkerAttached(&Run{RunID: "run"}, workerCommitAttachment{Binding: &ref}, "pending"); err == nil {
			t.Fatal("incomplete resource identity was committed")
		}
		if service.domain.Tasks[0].Status != "open" || service.pendingOperations["pending"] != "" {
			t.Fatal("validation failure leaked a private candidate or operation")
		}
	})
}
