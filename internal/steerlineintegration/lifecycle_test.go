package steerlineintegration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type integrationProcessHost struct{}

func (integrationProcessHost) StartManaged(*exec.Cmd) (executionapi.StopHandle, error) {
	return nil, executionapi.UnsupportedAdapterOperation("StartManaged", "preparation is outside this plan-only integration assertion")
}
func (integrationProcessHost) InspectProcess(*executionapi.ProcessIdentity) string { return "unknown" }
func (integrationProcessHost) NewProcessGeneration() string                        { return "probe-generation" }
func (integrationProcessHost) WriteExecutionFile(string, []byte) error {
	return executionapi.UnsupportedAdapterOperation("WriteExecutionFile", "no process preparation requested")
}
func (integrationProcessHost) OutputCommand(context.Context, executionapi.LaunchCommand) ([]byte, error) {
	return nil, executionapi.UnsupportedAdapterOperation("OutputCommand", "no capability probe requested")
}

type integrationResolver struct{ calls int }

func (r *integrationResolver) ResolveLifecycle(setup executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
	r.calls++
	return codexworker.NewExecutionLifecycle(setup)
}

func TestCD64OriginalSessionReconciliation(t *testing.T) {
	t.Run("remote_protocol_original_assertions", testCD64RemoteProtocolAssertions)
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("STEERLINE_HOME", home)
	repository := filepath.Join(t.TempDir(), "repo")
	for _, args := range [][]string{{"init", "--quiet", "-b", "main", repository}, {"-C", repository, "config", "user.name", "Fixture"}, {"-C", repository, "config", "user.email", "fixture@example.invalid"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-C", repository, "add", "."}, {"-C", repository, "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture"}, {"-C", repository, "remote", "add", "origin", "https://github.com/owner/repo.git"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	policy := executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}
	profileConfig := codexworker.ProfileConfig{Executable: os.Args[0]}
	if runtime.GOOS == "windows" {
		profileConfig.WindowsSandbox = "elevated"
	}
	payload, err := codexworker.EncodeProfileConfig(profileConfig)
	if err != nil {
		t.Fatal(err)
	}
	globalData, err := json.Marshal(map[string]any{
		"version": 2, "default_execution_profile": "local",
		"execution_profiles": []map[string]any{{"id": "local", "name": "Local", "host": "local", "agent": "codex",
			"implementation_configs": []executionapi.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: payload}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(steerline.GlobalExecutionConfigPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(steerline.GlobalExecutionConfigPath(), globalData, 0600); err != nil {
		t.Fatal(err)
	}
	projectData, err := json.Marshal(map[string]any{
		"repository": repository, "remote": "origin", "branch": "main", "task_authority": "native", "workflow_protocol": "worker-flow-v1",
		"execution_policy": policy, "publication_policy": executionapi.PublicationPolicy{Publisher: executionapi.PublisherHostService},
		"pr": map[string]any{"kind": "command", "base_url": "https://github.com", "repository": "owner/repo",
			"query": steerline.Command{Executable: "git", Args: []string{"status"}}, "close": steerline.Command{Executable: "git", Args: []string{"status"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "project.json")
	if err = os.WriteFile(configPath, projectData, 0600); err != nil {
		t.Fatal(err)
	}
	config, err := steerline.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	config.Claimant = "integration"
	store, err := steerline.OpenNativeTaskProject(ctx, &config)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	resolver := &integrationResolver{}
	service, err := steerline.NewServiceWithLifecycleResolver(config, resolver)
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	finished := make(chan error, 1)
	go func() { finished <- service.RunWithDispatch(runCtx, "manual") }()
	deadline := time.After(5 * time.Second)
	for !service.State.Service.Ready {
		select {
		case err := <-finished:
			cancel()
			_ = service.Close()
			t.Fatalf("injected service exited before ready: %v", err)
		case <-deadline:
			cancel()
			_ = service.Close()
			t.Fatal("injected service did not become ready")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	if err := <-finished; err != nil {
		_ = service.Close()
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 0 {
		t.Fatalf("empty service resolved lifecycle %d times without a persisted execution", resolver.calls)
	}
	profile := config.AvailableExecutionProfiles()[0]
	target := executionapi.ExecutionTarget{ID: "local:" + runtime.GOOS + ":integration"}
	ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalCodexAppServer, Target: target, Agent: executionapi.AgentSpec{Name: "codex"}, WorkspacePath: repository}
	session := executionapi.SessionContext{ProjectID: config.Identity(), TaskID: "901", WorkflowRunID: "00000000000000000000000000000001", ExecutionID: "00000000000000000000000000000002", AttemptID: "00000000000000000000000000000003", Agent: ref.Agent, Target: target}
	snapshot := executionapi.LifecycleSnapshot{Ref: ref, Session: session, ConfigurationDigest: profile.Digest(), ResourceGeneration: "resource-generation"}
	setup := executionapi.LifecycleSetup{Snapshot: snapshot, ProfileData: config.ImplementationData, ProfileID: profile.ID, AcceptedConfigVersion: config.LoadedConfigDigest(), Policy: config.ExecutionPolicy, ProcessHost: integrationProcessHost{}, RepositoryPath: repository, RunDirectory: config.RunDir, CommandTimeout: 10 * time.Second, AttemptNumber: 1}
	port, err := resolver.ResolveLifecycle(setup)
	if err != nil {
		t.Fatal(err)
	}
	turn := executionapi.TurnInput{CommandID: "00000000000000000000000000000004", Prompt: "original saved prompt"}
	snapshot.PendingTurn = &turn
	candidate, err := port.Next(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Action != executionapi.LifecyclePrepareSession || candidate.Input.Turn == nil || candidate.Input.Turn.CommandID != turn.CommandID || candidate.Input.Turn.Prompt != turn.Prompt {
		t.Fatalf("injected implementation changed or skipped the original session preparation: %+v", candidate)
	}
}
