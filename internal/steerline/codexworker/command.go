package codexworker

import (
	"errors"
	"fmt"
	"runtime"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
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
	Policy           executionbackend.ExecutionPolicy
	Sandbox          string
	WindowsSandbox   string
}

func ManagedCommand(options ManagedCommandOptions) (executionbackend.LaunchCommand, error) {
	if options.Host != "orca" || options.Agent != "codex" || options.Executable == "" {
		return executionbackend.LaunchCommand{}, errors.New("managed Orca launch requires Codex and an executable")
	}
	permissions, err := CompilePolicy(options.Policy, runtime.GOOS, options.Host, options.Agent, options.WindowsSandbox, "", options.Args)
	if err != nil {
		return executionbackend.LaunchCommand{}, err
	}
	if options.Sandbox != permissions.Sandbox || (permissions.Sandbox == "danger-full-access" && len(options.WriteRoots) != 0) {
		return executionbackend.LaunchCommand{}, errors.New("managed Orca settings differ from the explicit execution_policy")
	}
	options.Sandbox, options.WindowsSandbox = permissions.Sandbox, permissions.WindowsSandbox
	args := append([]string(nil), options.Args...)
	if err := ValidateManagedArgs(args); err != nil {
		return executionbackend.LaunchCommand{}, err
	}
	if options.Model != "" {
		args = append(args, "--model", options.Model)
	}
	if options.ReasoningEffort != "" {
		args = append(args, "--config", "model_reasoning_effort="+options.ReasoningEffort)
	}
	args = append(args, "--strict-config")
	args = append(args, permissions.Args()...)
	for _, root := range options.WriteRoots {
		args = append(args, "--add-dir", root)
	}
	if options.WorkingDirectory != "" {
		args = append(args, "--cd", options.WorkingDirectory)
	}
	return executionbackend.LaunchCommand{Executable: options.Executable, Args: args, WorkingDirectory: options.WorkingDirectory}, nil
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
