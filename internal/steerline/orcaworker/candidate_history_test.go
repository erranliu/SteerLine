package orcaworker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/processhost"
)

func TestCandidateSessionLaunchKeepsOriginalHistoryIntentAndPolicy(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	policy := execution.ExecutionPolicy{Filesystem: execution.FilesystemPolicy{ReadScope: "host", WriteScope: "host"}, Network: "allow", Escalation: "deny"}
	snapshot := execution.LifecycleSnapshot{Session: execution.SessionContext{ProjectID: "project", TaskID: "1", WorkflowRunID: stableIDValue(1), ExecutionID: stableIDValue(2), AttemptID: stableIDValue(3)}, Ref: execution.ExecutionRef{Backend: execution.BackendOrca, Protocol: execution.ProtocolOrcaTerminalV2, Target: execution.ExecutionTarget{ID: "native-target", WorktreeID: "native-worktree"}}, ResourceGeneration: "generation", ConfigurationDigest: "config"}
	identity := codexworker.PolicyResourceIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration, ConfigDigest: snapshot.ConfigurationDigest}
	settings := execution.LaunchCommand{Executable: "native-codex", WorkingDirectory: workspace, Args: []string{"--model", "gpt-6-luna", "--config", "sandbox_mode=danger-full-access", "--config", "approval_policy=never"}}
	resource, err := codexworker.NewPolicyResourceEnvelope(identity, codexworker.PolicyResourcePayload{NativeVersion: codexworker.PolicyCodexVersion, Contract: codexworker.FullAccessPolicyContract, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.PolicyResources = &resource
	l := ExecutionLifecycle{setup: execution.LifecycleSetup{RunDirectory: root, Policy: policy, ProcessHost: processhost.Host{}}, codex: codexworker.ProfileConfig{Model: "gpt-6-luna"}}
	command, err := l.agentLaunchCommand(snapshot, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(command.Args) != 3 || command.Args[0] != "codex-session-launch" || command.WorkingDirectory != workspace {
		t.Fatalf("unbound launcher: %+v", command)
	}
	raw, err := os.ReadFile(command.Args[2])
	if err != nil {
		t.Fatal(err)
	}
	var launch codexworker.SessionLaunch
	if err := json.Unmarshal(raw, &launch); err != nil {
		t.Fatal(err)
	}
	if launch.Binding.ExecutionID != snapshot.Session.ExecutionID || launch.Binding.AttemptID != snapshot.Session.AttemptID || launch.Binding.ExternalRef != "native-worktree" || launch.Binding.TargetID != "native-target" || launch.Binding.Command.Executable != "native-codex" || launch.Binding.SessionID != "" || launch.Prompt != "" || launch.Sandbox != "danger-full-access" || len(launch.Args) != len(settings.Args) {
		t.Fatalf("history launch lost native identity/policy: %+v", launch)
	}
	if _, err := l.agentLaunchCommand(snapshot, workspace); err != nil {
		t.Fatal("identical replay:", err)
	}
	snapshot.Session.TaskID = "replacement"
	if _, err := l.agentLaunchCommand(snapshot, workspace); err == nil {
		t.Fatal("changed original launch intent accepted")
	}
	if _, err := os.Stat(launch.BindingPath); !os.IsNotExist(err) {
		t.Fatal("test constructed native session evidence without launcher")
	}
}
