package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func workspacePolicyFixture() executionapi.ExecutionPolicy {
	return executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}
}

func TestProjectPolicyRejectsMissingOrOldConfiguration(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	writeGlobalExecutionFixture(t, []ExecutionProfile{{ID: "local", Name: "Local", Host: "local", Agent: "codex", Codex: Command{Executable: "codex"}}}, "local")
	body := map[string]any{"repository": configTestRepository(t), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion}
	path := filepath.Join(t.TempDir(), "project.json")
	write := func() {
		t.Helper()
		data, err := json.Marshal(body)
		require(t, err)
		require(t, os.WriteFile(path, data, 0600))
	}
	write()
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "execution_policy") {
		t.Fatalf("missing authority accepted: %v", err)
	}
	body["execution_policy"] = workspacePolicyFixture()
	for _, field := range []string{"sandbox", "windows_sandbox"} {
		body[field] = "workspace-write"
		write()
		if _, err := LoadConfig(path); err == nil {
			t.Fatalf("old project authorization field accepted: %s", field)
		}
		delete(body, field)
	}
	write()
	if _, err := LoadConfig(path); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"sandbox", "windows_sandbox"} {
		raw, err := json.Marshal(map[string]any{"execution_profiles": []map[string]any{{"id": "local", "name": "Local", "host": "local", "agent": "codex", field: "workspace-write", "codex": map[string]string{"executable": "codex"}}}})
		require(t, err)
		require(t, os.WriteFile(GlobalExecutionConfigPath(), raw, 0600))
		if _, err := LoadConfig(path); err == nil {
			t.Fatalf("old global authorization field accepted: %s", field)
		}
	}
}

func TestProjectPolicyClaimRejectsStaticConflictsBeforeNativeQuery(t *testing.T) {
	c, _, store := openDAGFixture(t)
	commands := CommandService{Store: store, Project: c.Identity()}
	c.Codex.Executable = filepath.Join(t.TempDir(), "must-never-be-called.exe")
	for _, mutate := range []func(*Config){
		func(c *Config) { c.ExecutionPolicy = executionapi.ExecutionPolicy{} },
		func(c *Config) { c.Codex.Args = []string{"--add-dir=C:/"} },
		func(c *Config) { c.Codex.Args = []string{"-c", "approval_policy=on-request"} },
		func(c *Config) { c.ReasoningEffort = "unrecognized" },
		func(c *Config) { c.Profile = "native-profile" },
		func(c *Config) { c.Host = "orca_supervised" },
	} {
		candidate := c
		mutate(&candidate)
		_, err := commands.ClaimNativeTask(context.Background(), candidate, TaskClaimOptions{TaskID: "59"})
		if err == nil || strings.Contains(err.Error(), "must-never-be-called") {
			t.Fatalf("static policy conflict reached executable lookup: %v", err)
		}
	}
}

func TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority(t *testing.T) {
	c := Config{ExecutionPolicy: workspacePolicyFixture(), Repository: "repository", Branch: "main", Host: "local", Agent: "codex", Codex: Command{Executable: "codex"}}
	original := c.projectExecutionProfile().Freeze()
	if !verifyFixedExecutionProfile(original) {
		t.Fatalf("valid snapshot rejected: %+v", original)
	}
	c.ExecutionPolicy.Network = "deny"
	c.Codex.WindowsSandbox = "unelevated"
	restored := c.ApplyFixedExecutionProfile(original)
	if restored.ExecutionPolicy.Network != "allow" || restored.Codex.WindowsSandbox != "elevated" {
		t.Fatal("current config rewrote existing authority")
	}
	for _, mutate := range []func(*FixedExecutionProfile){
		func(p *FixedExecutionProfile) { p.Version = 1 }, func(p *FixedExecutionProfile) { p.ExecutionPolicy.Network = "deny" }, func(p *FixedExecutionProfile) { p.Sandbox = "danger-full-access" }, func(p *FixedExecutionProfile) { p.WindowsSandbox = "unelevated" }, func(p *FixedExecutionProfile) { p.AcceptedConfigVersion = "edited" }, func(p *FixedExecutionProfile) { p.PolicyDigest = "edited" },
	} {
		copy := original
		mutate(&copy)
		if verifyFixedExecutionProfile(copy) {
			t.Fatalf("damaged snapshot accepted: %+v", copy)
		}
	}
}

func TestProjectPolicyNativeReportsRetainOriginalSnapshotDigests(t *testing.T) {
	for _, name := range []string{"local-native.json", "orca-native.json"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "validation", "execution-isolation-06-policy", name))
		require(t, err)
		var report struct {
			Profile   FixedExecutionProfile `json:"profile"`
			Execution ExecutionRecord       `json:"execution"`
		}
		require(t, json.Unmarshal(raw, &report))
		if !verifyFixedExecutionProfile(report.Profile) {
			t.Fatalf("native report no longer matches final snapshot: %s", name)
		}
		require(t, verifyPolicyResources(report.Execution.PolicyResources, report.Profile.Digest, report.Execution.Workspace.Worktree))
	}
}

func TestProjectPolicyRejectsGlobalConflictsBeforeRepositoryProbe(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	writeGlobalExecutionFixture(t, []ExecutionProfile{{ID: "local", Name: "Local", Host: "local", Agent: "codex", Codex: Command{Executable: "codex", Args: []string{"--config", "sandbox_mode=danger-full-access"}}}}, "local")
	path := filepath.Join(t.TempDir(), "project.json")
	raw, err := json.Marshal(map[string]any{"execution_policy": workspacePolicyFixture(), "repository": t.TempDir(), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion, "git": map[string]string{"executable": "must-never-be-called"}})
	require(t, err)
	require(t, os.WriteFile(path, raw, 0600))
	_, err = LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported managed launch") {
		t.Fatalf("unsafe args reached repository identity probe: %v", err)
	}
}

func TestProjectPolicyMissingBoundResourcesAreNotReconstructed(t *testing.T) {
	c := Config{ExecutionPolicy: workspacePolicyFixture(), Codex: Command{Executable: "must-not-be-called"}}
	e := ExecutionRecord{SessionID: "original-native-session"}
	if err := bindPolicyResources(context.Background(), c, &e, "workspace", nil); err == nil || !strings.Contains(err.Error(), "snapshot is missing") {
		t.Fatalf("missing original resources were reconstructed: %v", err)
	}
	if e.PolicyResources != nil {
		t.Fatal("original snapshot was backfilled")
	}
}

func TestAppServerLaunchMessagePersistsPolicyResources(t *testing.T) {
	for _, phase := range []string{"thread_ready", "turn_result"} {
		t.Run(phase, func(t *testing.T) {
			s, run := appServerAuditService(t)
			workflow, execution := s.workflowRecords(run)
			path := run.WorkspaceIntent.Path
			execution.Workspace = WorkspaceEvidence{Worktree: path, Branch: run.WorkspaceIntent.Branch, Base: run.WorkspaceIntent.BaseCommit}
			commandID := newID()
			execution.PendingCommandID = commandID
			s.refreshRun(run)
			opID, err := s.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+run.ExecutionID+":"+commandID, map[string]string{"command_id": commandID})
			require(t, err)
			identity, err := currentProcessIdentity()
			require(t, err)
			partial := *s.executionRecord(run)
			partial.Process, partial.SessionID = identity, workerFlowMockSession
			partial.AppServerEndpoint, partial.AppServerGeneration = "ws://127.0.0.1:12345", "policy-generation"
			resource := contracts.PolicyResources{Version: 2, ProfileDigest: workflow.ExecutionProfile.Digest, NativeVersion: codexworker.PolicyCodexVersion, Contract: codexworker.PolicyContract, Workspace: path, WriteRoots: []string{path}, TemporaryRoots: []string{}}
			resource.Digest = policyResourceDigest(resource)
			partial.PolicyResources = &resource
			message := &appServerLaunchMessage{phase: phase, commandID: commandID, opID: opID, run: *run, execution: partial}
			if phase == "turn_result" {
				message.err = errors.New("thread receipt persistence failed")
			}
			require(t, s.handleAppServerLaunchMessage(run.RunID, message))
			snapshot, err := s.domainStore.Query(context.Background(), s.Config.identity)
			require(t, err)
			got := snapshot.Executions[0]
			if got.PolicyResources == nil || got.PolicyResources.Digest != resource.Digest || got.SessionID != partial.SessionID || got.PendingTurnID != "" || s.workers[run.RunID] != nil {
				t.Fatalf("policy resource receipt was lost or advanced a turn: %+v", got)
			}
			_, err = s.executionConfigForRun(run)
			require(t, err)
		})
	}
}

func policyResourceReceiptFixture(profileDigest, path string) *contracts.PolicyResources {
	resource := &contracts.PolicyResources{Version: 2, ProfileDigest: profileDigest, NativeVersion: codexworker.PolicyCodexVersion, Contract: codexworker.PolicyContract, Workspace: path, WriteRoots: []string{path}, TemporaryRoots: []string{}}
	resource.Digest = policyResourceDigest(*resource)
	return resource
}
