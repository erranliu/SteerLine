package codexworker

import (
	"reflect"
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
		if err != nil || !strings.Contains(strings.Join(command.Args, " "), "auto_review") || !strings.Contains(strings.Join(command.Args, " "), "windows.sandbox=unelevated") {
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
	options := ManagedCommandOptions{Host: "orca", Agent: "codex", Executable: "codex.exe", Args: []string{"--no-alt-screen", "--search"}, ReasoningEffort: "high", Sandbox: "workspace-write", WindowsSandbox: "unelevated", WorkingDirectory: "C:/worker's checkout", WriteRoots: []string{"C:/repo/.git/objects"}}
	command, err := ManagedCommand(options)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--no-alt-screen", "--search", "--config", "model_reasoning_effort=high", "--no-daemon", "--strict-config"}
	permissions, err := NativePermissions("windows", "orca", "codex")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, permissions.Args()...)
	want = append(want, "--add-dir", options.WriteRoots[0], "--cd", options.WorkingDirectory)
	if command.Executable != options.Executable || command.WorkingDirectory != options.WorkingDirectory || !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("fixed command drifted: %+v", command)
	}
	options.Args[0] = "changed"
	if command.Args[0] != "--no-alt-screen" {
		t.Fatal("fixed argv aliases mutable configuration")
	}
}
