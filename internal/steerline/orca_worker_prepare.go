package steerline

import (
	"runtime"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func nativeOrcaFixedAgentTitle(executionID string) string {
	return orcaworker.FixedAgentTitle(executionID)
}

func nativeOrcaFixedAgentCommand(c Config) (string, error) {
	return nativeOrcaCommand(c, "", nil)
}

func nativeOrcaCommand(c Config, path string, roots []string) (string, error) {
	codex := codexProfileSettings(c)
	orca := orcaProfileSettings(c)
	permissions, err := codexworker.CompilePolicy(c.ExecutionPolicy, runtime.GOOS, c.hostName(), c.agentName(), codex.WindowsSandbox, codex.Profile, codex.Args)
	if err != nil {
		return "", err
	}
	if path != "" {
		if err := codexworker.ValidateManagedArgs(codex.Args); err != nil {
			return "", err
		}
		native, err := codexworker.NativeExecutable(codex.Executable)
		if err != nil {
			return "", err
		}
		codex.Executable = native
	}
	command, err := codexworker.ManagedCommand(codexworker.ManagedCommandOptions{
		Policy: c.ExecutionPolicy, WorkingDirectory: path, WriteRoots: roots, Host: c.hostName(), Agent: c.agentName(), Model: codex.Model, ReasoningEffort: codex.ReasoningEffort,
		Executable: codex.Executable, Args: codex.Args, Sandbox: permissions.Sandbox, WindowsSandbox: permissions.WindowsSandbox,
	})
	if err != nil {
		return "", err
	}
	return orcaworker.WrapAgentCommand(command, orca.Shell)
}
