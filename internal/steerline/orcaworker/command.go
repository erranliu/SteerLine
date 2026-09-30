// Package orcaworker handles Orca worker protocol operations and evidence
// without owning workflow state or service persistence.
package orcaworker

import (
	"errors"
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
	Shell           string
}

func FixedAgentCommand(options FixedAgentOptions) (string, error) {
	if options.Host != "orca" || options.Agent != "codex" || options.Model == "" || options.ReasoningEffort == "" || options.Executable == "" || options.Sandbox != "workspace-write" {
		return "", errors.New("fixed Orca agent launch requires Codex, model, effort, executable and workspace-write")
	}
	if options.Shell != "powershell.exe" && options.Shell != "pwsh.exe" {
		return "", errors.New("fixed Orca agent launch requires a PowerShell terminal")
	}
	args := append([]string(nil), options.Args...)
	args = append(args, "--model", options.Model, "--config", "model_reasoning_effort="+options.ReasoningEffort, "--sandbox", options.Sandbox)
	if runtime.GOOS == "windows" {
		args = append(args, "--config", "sandbox_workspace_write.network_access=true")
	}
	parts := []string{"&", powershellLiteral(options.Executable)}
	for _, arg := range args {
		parts = append(parts, powershellLiteral(arg))
	}
	return strings.Join(parts, " "), nil
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
