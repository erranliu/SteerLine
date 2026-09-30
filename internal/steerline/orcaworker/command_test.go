package orcaworker

import (
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func TestWrapAgentCommandPreservesIndependentArguments(t *testing.T) {
	command := executionapi.LaunchCommand{Executable: "C:/agent's bin/codex.exe", Args: []string{"--cd", "C:/worker's checkout", "$env:SECRET; $(invoke)", ""}}
	want := "& 'C:/agent''s bin/codex.exe' '--cd' 'C:/worker''s checkout' '$env:SECRET; $(invoke)' ''"
	for _, shell := range []string{"powershell.exe", "pwsh.exe"} {
		if wrapped, err := WrapAgentCommand(command, shell); err != nil || wrapped != want {
			t.Fatalf("argv changed at shell boundary: %q / %v", wrapped, err)
		}
	}
}
