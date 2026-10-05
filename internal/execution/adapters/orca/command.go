// Package orcaworker handles Orca worker protocol operations and evidence
// without owning workflow state or service persistence.
package orcaworker

import (
	"errors"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func FixedAgentTitle(executionID string) string { return "SteerLine agent " + executionID }

func powershellLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

// WrapAgentCommand applies only the terminal shell boundary to a fixed argv.
func WrapAgentCommand(command executionbackend.LaunchCommand, shell string) (string, error) {
	if shell != "powershell.exe" && shell != "pwsh.exe" {
		return "", errors.New("fixed Orca agent launch requires a PowerShell terminal")
	}
	parts := []string{"&", powershellLiteral(command.Executable)}
	for _, arg := range command.Args {
		parts = append(parts, powershellLiteral(arg))
	}
	return strings.Join(parts, " "), nil
}
