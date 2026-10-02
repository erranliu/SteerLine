package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func workspacePolicyFixture() executionapi.ExecutionPolicy {
	return executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}
}

func TestFullAccessProfileFreezeSurvivesClaimValidation(t *testing.T) {
	for _, host := range []string{"local", "orca"} {
		policy := workspacePolicyFixture()
		policy.Filesystem.WriteScope, policy.Filesystem.GitMetadataWrite = "host", false
		c := Config{PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), ExecutionPolicy: policy, Repository: "repository", Branch: "main", Host: host, Agent: "codex"}
		if host == "orca" {
			c.PublicationPolicy.Publisher = "worker"
		}
		if host == "orca" {
			setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "codex"}, &orcaworker.ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "1m"})
		} else {
			setTestCodexProfile(t, &c, codexworker.ProfileConfig{Executable: "codex"})
		}
		original := c.projectExecutionProfile().Freeze(configurationProjectIdentity(c))
		if c.projectExecutionProfile().DigestFor(configurationProjectIdentity(c)) != original.Digest {
			t.Fatal("selection digest differs from its unchanged frozen envelope")
		}
		if !verifyFixedExecutionProfile(original) {
			t.Fatalf("%s full-access snapshot rejected at claim: %+v", host, original)
		}
		restored := c.ApplyFixedExecutionProfile(original)
		if restored.ExecutionPolicy.Filesystem.WriteScope != "host" || codexProfileSettings(restored).WindowsSandbox != "" {
			t.Fatal("frozen full-access authority was not restored")
		}
		original.ImplementationData[0].Payload[0] = '['
		if verifyFixedExecutionProfile(original) {
			t.Fatal("full-access snapshot accepted a changed Windows sandbox mode")
		}
	}
}

func TestProjectPolicyRejectsMissingOrOldConfiguration(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	writeGlobalExecutionFixture(t, []ExecutionProfile{testExecutionProfile(t, "local", "Local", "local", "codex", codexworker.ProfileConfig{Executable: "codex"}, nil)}, "local")
	body := map[string]any{"publication_policy": executionPublicationFixture(), "repository": configTestRepository(t), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion}
	path := filepath.Join(t.TempDir(), "project.json")
	write := func() {
		t.Helper()
		data, err := json.Marshal(body)
		require(t, err)
		require(t, os.WriteFile(path, data, 0600))
	}
	write()
	if _, err := loadFixtureConfig(path); err == nil || !strings.Contains(err.Error(), "execution_policy") {
		t.Fatalf("missing authority accepted: %v", err)
	}
	body["execution_policy"] = workspacePolicyFixture()
	body["pr"] = hostPublicationProviderFixture()
	for _, field := range []string{"sandbox", "windows_sandbox"} {
		body[field] = "workspace-write"
		write()
		if _, err := loadFixtureConfig(path); err == nil {
			t.Fatalf("old project authorization field accepted: %s", field)
		}
		delete(body, field)
	}
	write()
	if _, err := loadFixtureConfig(path); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"sandbox", "windows_sandbox"} {
		raw, err := json.Marshal(map[string]any{"execution_profiles": []map[string]any{{"id": "local", "name": "Local", "host": "local", "agent": "codex", field: "workspace-write", "codex": map[string]string{"executable": "codex"}}}})
		require(t, err)
		require(t, os.WriteFile(fixtureGlobalExecutionConfigPath(), raw, 0600))
		if _, err := loadFixtureConfig(path); err == nil {
			t.Fatalf("old global authorization field accepted: %s", field)
		}
	}
}

func TestProjectPolicyClaimRejectsStaticConflictsBeforeNativeQuery(t *testing.T) {
	c, _, store := openDAGFixture(t)
	commands := CommandService{Store: store, Project: c.Identity()}
	codex := codexProfileSettings(c)
	codex.Executable = filepath.Join(t.TempDir(), "must-never-be-called.exe")
	setTestCodexProfile(t, &c, codex)
	for _, mutate := range []func(*Config){
		func(c *Config) { c.ExecutionPolicy = executionapi.ExecutionPolicy{} },
		func(c *Config) {
			settings := codexProfileSettings(*c)
			settings.Args = []string{"--add-dir=C:/"}
			setTestCodexProfile(t, c, settings)
		},
		func(c *Config) {
			settings := codexProfileSettings(*c)
			settings.Args = []string{"-c", "approval_policy=on-request"}
			setTestCodexProfile(t, c, settings)
		},
		func(c *Config) {
			settings := codexProfileSettings(*c)
			settings.ReasoningEffort = "unrecognized"
			setTestCodexProfile(t, c, settings)
		},
		func(c *Config) {
			settings := codexProfileSettings(*c)
			settings.Profile = "native-profile"
			setTestCodexProfile(t, c, settings)
		},
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
	c := Config{PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), ExecutionPolicy: workspacePolicyFixture(), Repository: "repository", Branch: "main", Host: "local", Agent: "codex"}
	setTestCodexProfile(t, &c, codexworker.ProfileConfig{Executable: "codex"})
	original := c.projectExecutionProfile().Freeze(configurationProjectIdentity(c))
	if !testVerifyFixedExecutionProfile(original) {
		t.Fatalf("valid snapshot rejected: %+v", original)
	}
	c.ExecutionPolicy.Network = "deny"
	codex := codexProfileSettings(c)
	codex.WindowsSandbox = "unelevated"
	setTestCodexProfile(t, &c, codex)
	restored := c.ApplyFixedExecutionProfile(original)
	if restored.ExecutionPolicy.Network != "allow" || codexProfileSettings(restored).WindowsSandbox != "elevated" {
		t.Fatal("current config rewrote existing authority")
	}
	for _, mutate := range []func(*FixedExecutionProfile){
		func(p *FixedExecutionProfile) { p.Version = 1 }, func(p *FixedExecutionProfile) { p.ExecutionPolicy.Network = "deny" }, func(p *FixedExecutionProfile) { p.ImplementationData[0].Payload[0] = '[' }, func(p *FixedExecutionProfile) { p.AcceptedConfigVersion = "edited" }, func(p *FixedExecutionProfile) { p.PolicyDigest = "edited" },
	} {
		copy := original
		mutate(&copy)
		if testVerifyFixedExecutionProfile(copy) {
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
		if err := json.Unmarshal(raw, &report); err != nil {
			// Historical native-shaped receipts are no longer a supported wire format.
			if !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("historical native report failed for an unrelated reason: %v", err)
			}
			continue
		}
		if testVerifyFixedExecutionProfile(report.Profile) {
			t.Fatalf("old native report was accepted as current format: %s", name)
		}
		if verifyPolicyResources(testConfigWithDirectory(Config{}).ApplyFixedExecutionProfile(report.Profile), report.Execution.PolicyResources, report.Profile.Digest, report.Execution.Workspace.Worktree, &report.Execution) == nil {
			t.Fatalf("historical native permission receipt was accepted as current evidence: %s", name)
		}
	}
}

func TestProjectPolicyRejectsGlobalConflictsBeforeRepositoryProbe(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	writeGlobalExecutionFixture(t, []ExecutionProfile{testExecutionProfile(t, "local", "Local", "local", "codex", codexworker.ProfileConfig{Executable: "codex", Args: []string{"--config", "sandbox_mode=danger-full-access"}}, nil)}, "local")
	path := filepath.Join(t.TempDir(), "project.json")
	raw, err := json.Marshal(map[string]any{"publication_policy": executionPublicationFixture(), "pr": hostPublicationProviderFixture(), "execution_policy": workspacePolicyFixture(), "repository": t.TempDir(), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion, "git": map[string]string{"executable": "must-never-be-called"}})
	require(t, err)
	require(t, os.WriteFile(path, raw, 0600))
	_, err = loadFixtureConfig(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported managed launch") {
		t.Fatalf("unsafe args reached repository identity probe: %v", err)
	}
}

func TestProjectPolicyMissingBoundResourcesAreNotReconstructed(t *testing.T) {
	c := Config{ExecutionPolicy: workspacePolicyFixture()}
	e := ExecutionRecord{Generation: "original-generation", Process: &executionapi.ProcessIdentity{Kind: "local"}}
	if err := bindPolicyResources(context.Background(), c, &e, "workspace", nil); err == nil || !strings.Contains(err.Error(), "snapshot is missing") {
		t.Fatalf("missing original resources were reconstructed: %v", err)
	}
	if e.PolicyResources != nil {
		t.Fatal("original snapshot was backfilled")
	}
}

func TestAppServerLaunchMessagePersistsPolicyResources(t *testing.T) {
	t.Run("session_receipt", func(t *testing.T) {
		s, run := appServerAuditService(t)
		workflow, execution := s.workflowRecords(run)
		path := run.WorkspaceIntent.Path
		execution.Workspace = WorkspaceEvidence{Worktree: path, Branch: run.WorkspaceIntent.Branch, Base: run.WorkspaceIntent.BaseCommit}
		commandID := newID()
		execution.Generation = newID()
		execution.PendingCommandID = commandID
		execution.ConfigurationDigest = workflow.ExecutionProfile.Digest
		s.refreshRun(run)
		input := executionapi.TurnInput{CommandID: commandID, Prompt: "fixed task instruction"}
		key := "worker-session:" + run.ExecutionID + ":" + commandID
		opID, err := s.startWorkerOperation(run, "prepare_worker_session", key, &input)
		require(t, err)
		identity, err := currentProcessIdentity()
		require(t, err)
		partial := *s.executionRecord(run)
		partial.Process = identity
		setTestCodexBinding(t, &partial, "ws://127.0.0.1:12345", "policy-generation", workerFlowMockSession)
		resource := policyResourceReceiptFixture(workflow.ExecutionProfile.Digest, path, &partial)
		s.executionRecord(run).PolicyResources = resource
		snapshot, err := lifecycleSnapshot(run, s.workflowRecord(run), s.executionRecord(run), s.workflowAttempt(run, run.AttemptID), &input)
		require(t, err)
		operation := s.operationByKey(key)
		request := lifecycleRequest(snapshot, *operation, executionapi.LifecyclePrepareSession, executionapi.LifecycleInput{Turn: &input})
		receipt := executionapi.LifecycleReceipt{
			Operation: request.Operation, Disposition: executionapi.OperationUnknown,
			RequestID: workerFlowMockSession, Process: identity,
			Implementation: partial.ImplementationData,
			State:          executionapi.LifecycleStateFacts{SessionID: workerFlowMockSession, SessionGeneration: "policy-generation"},
		}
		message := &appServerLaunchMessage{phase: "session_prepared", request: request, receipt: receipt, err: errors.New("session receipt persistence failed")}
		require(t, s.handleAppServerLaunchMessage(run.RunID, message))
		snapshotResult, err := s.domainStore.Query(context.Background(), s.Config.identity)
		require(t, err)
		got := snapshotResult.Executions[0]
		if got.PolicyResources == nil || got.PolicyResources.Digest != resource.Digest || got.SessionID != workerFlowMockSession || got.PendingTurnID != "" || s.workers[run.RunID] != nil {
			t.Fatalf("policy resource receipt was lost or advanced a turn: %+v", got)
		}
		if snapshotOperation(&snapshotResult, opID).Status != "outcome_unknown" {
			t.Fatal("unverified session receipt changed the original operation identity")
		}
		_, err = s.executionConfigForRun(run)
		require(t, err)
	})

	t.Run("turn_result", func(t *testing.T) {
		s, run := appServerAuditService(t)
		workflow, execution := s.workflowRecords(run)
		path := run.WorkspaceIntent.Path
		execution.Workspace = WorkspaceEvidence{Worktree: path, Branch: run.WorkspaceIntent.Branch, Base: run.WorkspaceIntent.BaseCommit}
		commandID := newID()
		execution.PendingCommandID = commandID
		identity, err := currentProcessIdentity()
		require(t, err)
		execution.Process = identity
		setTestCodexBinding(t, execution, "ws://127.0.0.1:12345", "policy-generation", workerFlowMockSession)
		execution.SessionID, execution.SessionGeneration = workerFlowMockSession, "policy-generation"
		resource := policyResourceReceiptFixture(workflow.ExecutionProfile.Digest, path, execution)
		execution.PolicyResources = resource
		s.refreshRun(run)
		input := executionapi.TurnInput{CommandID: commandID, Prompt: "fixed task instruction"}
		key := "worker-turn:" + run.ExecutionID + ":" + commandID
		opID, err := s.startWorkerOperation(run, "start_worker_turn", key, &input)
		require(t, err)
		snapshot, err := lifecycleSnapshot(run, s.workflowRecord(run), execution, s.workflowAttempt(run, run.AttemptID), &input)
		require(t, err)
		operation := s.operationByKey(key)
		request := lifecycleRequest(snapshot, *operation, executionapi.LifecycleStartTurn, executionapi.LifecycleInput{Turn: &input})
		receipt := executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationUnknown, RequestID: workerFlowMockSession, Process: identity,
			Implementation: cloneImplementationData(execution.ImplementationData), State: executionapi.LifecycleStateFacts{SessionID: workerFlowMockSession, SessionGeneration: "policy-generation"}}
		message := &appServerLaunchMessage{phase: "turn_result", request: request, receipt: receipt, err: errors.New("turn receipt persistence failed")}
		require(t, s.handleAppServerLaunchMessage(run.RunID, message))
		snapshotResult, err := s.domainStore.Query(context.Background(), s.Config.identity)
		require(t, err)
		got := snapshotResult.Executions[0]
		if got.PolicyResources == nil || got.PolicyResources.Digest != resource.Digest || got.SessionID != workerFlowMockSession || got.PendingTurnID != "" || s.workers[run.RunID] != nil {
			t.Fatalf("policy resource receipt was lost or advanced a turn: %+v", got)
		}
		if snapshotOperation(&snapshotResult, opID).Status != "outcome_unknown" {
			t.Fatal("unverified turn receipt changed the original operation identity")
		}
		_, err = s.executionConfigForRun(run)
		require(t, err)
	})
}

func policyResourceReceiptFixture(profileDigest, path string, record *ExecutionRecord) *contracts.PolicyResources {
	if record.ProjectIdentity == "" {
		record.ProjectIdentity = "test-project"
	}
	if record.ExecutionID == "" {
		record.ExecutionID = "test-execution"
	}
	if record.CurrentAttemptID == "" {
		record.CurrentAttemptID = "test-attempt"
	}
	if record.Generation == "" {
		record.Generation = newID()
	}
	if record.ConfigurationDigest == "" {
		record.ConfigurationDigest = profileDigest
	}
	envelope, err := codexworker.NewPolicyResourceEnvelope(codexworker.PolicyResourceIdentity{ProjectID: record.ProjectIdentity, ExecutionID: record.ExecutionID, AttemptID: record.CurrentAttemptID, Generation: record.Generation, ConfigDigest: profileDigest}, codexworker.PolicyResourcePayload{NativeVersion: codexworker.PolicyCodexVersion, Contract: codexworker.PolicyContract, Settings: executionapi.LaunchCommand{Executable: "codex", WorkingDirectory: path}})
	if err != nil {
		panic(err)
	}
	resource := &contracts.PolicyResources{Version: 3, ProfileDigest: profileDigest, Workspace: path, WriteRoots: []string{path}, TemporaryRoots: []string{}, ImplementationData: envelope}
	resource.Digest = policyResourceDigest(*resource)
	return resource
}
