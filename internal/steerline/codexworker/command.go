package codexworker

import (
	"errors"
	"fmt"
	"runtime"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type ManagedCommandOptions struct {
	WorkingDirectory string
	WriteRoots       []string
	Host             string
	Agent            string
	Model            string
	ReasoningEffort  string
	Executable       string
	Args             []string
	Sandbox          string
	WindowsSandbox   string
}

func ManagedCommand(options ManagedCommandOptions) (executionapi.LaunchCommand, error) {
	if options.Host != "orca" || options.Agent != "codex" || options.Executable == "" || options.Sandbox != "workspace-write" || !ValidWindowsSandbox(options.WindowsSandbox) {
		return executionapi.LaunchCommand{}, errors.New("managed Orca launch requires Codex, executable, workspace-write and explicit windows_sandbox (elevated or unelevated)")
	}
	args := append([]string(nil), options.Args...)
	if err := ValidateManagedArgs(args); err != nil {
		return executionapi.LaunchCommand{}, err
	}
	if options.Model != "" {
		args = append(args, "--model", options.Model)
	}
	if options.ReasoningEffort != "" {
		args = append(args, "--config", "model_reasoning_effort="+options.ReasoningEffort)
	}
	args = append(args, "--strict-config", "--sandbox", options.Sandbox, "--ask-for-approval", "never", "--config", "windows.sandbox="+options.WindowsSandbox,
		"--config", "sandbox_workspace_write.writable_roots=[]", "--config", "sandbox_workspace_write.exclude_tmpdir_env_var=true", "--config", "sandbox_workspace_write.exclude_slash_tmp=true")
	if runtime.GOOS == "windows" {
		args = append(args, "--config", "sandbox_workspace_write.network_access=true")
	}
	for _, root := range options.WriteRoots {
		args = append(args, "--add-dir", root)
	}
	if options.WorkingDirectory != "" {
		args = append(args, "--cd", options.WorkingDirectory)
	}
	return executionapi.LaunchCommand{Executable: options.Executable, Args: args, WorkingDirectory: options.WorkingDirectory}, nil
}

// The managed launcher accepts only standalone presentation switches. This
// deliberately excludes subcommands, config/profile injection, shell roots,
// remote servers and future unknown switches before any resource is created.
func ValidateManagedArgs(args []string) error {
	for _, arg := range args {
		switch arg {
		case "--no-alt-screen", "--no-daemon", "--search":
		default:
			return fmt.Errorf("codex.args contains an unsupported managed launch argument %q", arg)
		}
	}
	return nil
}
