package codexworker

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestManagedOrcaLaunchFreezesPermissionsAndRejectsArgs(t *testing.T) {
	base := ManagedCommandOptions{Host: "orca", Agent: "codex", Executable: "codex.exe", Sandbox: "workspace-write", WindowsSandbox: "unelevated"}
	for _, model := range []string{"", "chosen-model"} {
		options := base
		options.Model = model
		if model != "" {
			options.ReasoningEffort = "high"
		}
		command, err := ManagedCommand(options)
		if err != nil || !strings.Contains(strings.Join(command.Args, " "), "never") || !strings.Contains(strings.Join(command.Args, " "), "windows.sandbox=unelevated") {
			t.Fatalf("permissions not frozen: %v / %v", command, err)
		}
		if model == "" && strings.Contains(strings.Join(command.Args, " "), "--model") {
			t.Fatal("default model was overwritten")
		}
	}
	for _, args := range [][]string{{"-a", "on-request"}, {"--ask-for-approval=on-request"}, {"-sworkspace-write"}, {"-c", "windows.sandbox=unelevated"}, {"-capproval_policy=never"}, {"--config=model='x'"}, {"--config", "sandbox_workspace_write.writable_roots=['C:/']"}, {"--profile=x"}, {"-p", "x"}, {"--add-dir=C:/"}, {"-C", "C:/"}, {"exec"}, {"--dangerously-bypass-approvals-and-sandbox"}, {"--approve-for-me"}, {"--remote", "ws://localhost"}, {"--worktree"}} {
		options := base
		options.Args = args
		if _, err := ManagedCommand(options); err == nil {
			t.Fatalf("conflicting args accepted: %v", args)
		}
	}
	for _, mode := range []string{"", "automatic", "danger-full-access"} {
		options := base
		options.WindowsSandbox = mode
		if _, err := ManagedCommand(options); err == nil {
			t.Fatalf("invalid windows sandbox accepted: %q", mode)
		}
	}
}

func TestManagedCommandIndependentEffortAndOrderedPermissionArgs(t *testing.T) {
	options := ManagedCommandOptions{Host: "orca", Agent: "codex", Executable: "codex.exe", Args: []string{"--no-alt-screen", "--search"}, ReasoningEffort: "high", Sandbox: "workspace-write", WindowsSandbox: "elevated"}
	command, err := ManagedCommand(options)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--no-alt-screen", "--search", "--config", "model_reasoning_effort=high", "--strict-config", "--sandbox", "workspace-write", "--ask-for-approval", "never", "--config", "windows.sandbox=elevated", "--config", "sandbox_workspace_write.writable_roots=[]", "--config", "sandbox_workspace_write.exclude_tmpdir_env_var=true", "--config", "sandbox_workspace_write.exclude_slash_tmp=true"}
	if runtime.GOOS == "windows" {
		want = append(want, "--config", "sandbox_workspace_write.network_access=true")
	}
	if command.Executable != options.Executable || !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("fixed command drifted: %+v", command)
	}
	options.Args[0] = "changed"
	if command.Args[0] != "--no-alt-screen" {
		t.Fatal("fixed argv aliases mutable configuration")
	}
}
