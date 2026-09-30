// Package orcaworker handles Orca worker protocol operations and evidence
// without owning workflow state or service persistence.
package orcaworker

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

type FixedAgentOptions struct {
	Host            string
	Agent           string
	Model           string
	ReasoningEffort string
	Executable      string
	Args            []string
	Sandbox         string
	WindowsSandbox  string
	Shell           string
}

func FixedAgentCommand(options FixedAgentOptions) (string, error) {
	if options.Host != "orca" || options.Agent != "codex" || options.Executable == "" || options.Sandbox != "workspace-write" || (options.WindowsSandbox != "elevated" && options.WindowsSandbox != "unelevated") {
		return "", errors.New("managed Orca launch requires Codex, executable, workspace-write and explicit windows_sandbox (elevated or unelevated)")
	}
	if options.Shell != "powershell.exe" && options.Shell != "pwsh.exe" {
		return "", errors.New("fixed Orca agent launch requires a PowerShell terminal")
	}
	args := append([]string(nil), options.Args...)
	if err := ValidateManagedArgs(args); err != nil {
		return "", err
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
	parts := []string{"&", powershellLiteral(options.Executable)}
	for _, arg := range args {
		parts = append(parts, powershellLiteral(arg))
	}
	return strings.Join(parts, " "), nil
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

func FixedAgentTitle(executionID string) string { return "SteerLine agent " + executionID }

func AppendPowerShellArgs(command string, args []string) string {
	for _, arg := range args {
		command += " " + powershellLiteral(arg)
	}
	return command
}

func stableID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func powershellLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
