// Package orcaworker handles Orca worker protocol operations and evidence
// without owning workflow state or service persistence.
package orcaworker

import (
	"errors"
	"strings"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

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

// WrapAgentCommand applies only the terminal shell boundary to a fixed argv.
func WrapAgentCommand(command executionapi.LaunchCommand, shell string) (string, error) {
	if shell != "powershell.exe" && shell != "pwsh.exe" {
		return "", errors.New("fixed Orca agent launch requires a PowerShell terminal")
	}
	parts := []string{"&", powershellLiteral(command.Executable)}
	for _, arg := range command.Args {
		parts = append(parts, powershellLiteral(arg))
	}
	return strings.Join(parts, " "), nil
}
