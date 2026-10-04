package codexworker

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

type ProbeConfig struct {
	Executable string
	Args       []string
	Repository string
	RunDir     string
}

type ProbePort struct {
	Output       func(context.Context, string, []string) ([]byte, error)
	CheckHistory func(context.Context) error
	StartManaged func(*exec.Cmd) (func(), error)
}

type ProbeError struct {
	Diagnostic string
	Cause      error
}

func (e *ProbeError) Error() string { return e.Diagnostic }
func (e *ProbeError) Unwrap() error { return e.Cause }

func ProbeCapabilities(ctx context.Context, config ProbeConfig, port ProbePort) error {
	nativeExecutable, err := NativeExecutable(config.Executable)
	if err != nil {
		return &ProbeError{Diagnostic: "local Codex app-server needs a directly bound native process", Cause: err}
	}
	configuredVersion, configuredErr := port.Output(ctx, config.Executable, append(append([]string(nil), config.Args...), "--version"))
	nativeVersion, nativeErr := port.Output(ctx, nativeExecutable, append(append([]string(nil), config.Args...), "--version"))
	if configuredErr != nil || nativeErr != nil || strings.TrimSpace(string(configuredVersion)) == "" || strings.TrimSpace(string(configuredVersion)) != strings.TrimSpace(string(nativeVersion)) {
		return &ProbeError{Diagnostic: "configured Codex and native app-server versions differ or cannot be verified", Cause: errors.Join(configuredErr, nativeErr)}
	}
	if err := VerifyPolicyVersion(string(nativeVersion)); err != nil {
		return err
	}
	out, err := port.Output(ctx, nativeExecutable, append(append([]string(nil), config.Args...), "app-server", "--help"))
	if err != nil || !strings.Contains(string(out), "--listen") {
		return &ProbeError{Diagnostic: "configured Codex executable does not expose app-server WebSocket transport", Cause: err}
	}
	if err := port.CheckHistory(ctx); err != nil {
		return &ProbeError{Diagnostic: "configured Codex session history cannot be read for exact turn reconciliation: " + err.Error(), Cause: err}
	}
	// Preflight must work before runtime state exists and leave it untouched.
	probeFile, err := os.CreateTemp("", "worker-process-probe-*.log")
	if err != nil {
		return err
	}
	defer os.Remove(probeFile.Name())
	defer probeFile.Close()
	process := exec.CommandContext(ctx, nativeExecutable, append(append([]string(nil), config.Args...), "--version")...)
	process.Dir, process.Stdout, process.Stderr = config.Repository, probeFile, probeFile
	closeOwned, err := port.StartManaged(process)
	if err != nil {
		return &ProbeError{Diagnostic: "local host cannot start an owned Codex worker process", Cause: err}
	}
	defer closeOwned()
	if err := process.Wait(); err != nil {
		return &ProbeError{Diagnostic: "owned Codex capability probe failed", Cause: err}
	}
	return nil
}

func ProbeManagedCLI(ctx context.Context, command contracts.Command, output func(context.Context, string, []string) ([]byte, error)) error {
	version, err := output(ctx, command.Executable, []string{"--version"})
	if err != nil {
		return err
	}
	if err := VerifyPolicyVersion(string(version)); err != nil {
		return err
	}
	help, err := output(ctx, command.Executable, []string{"--help"})
	if err != nil {
		return err
	}
	for _, flag := range []string{"--strict-config", "--sandbox", "--ask-for-approval", "never", "--config"} {
		if !strings.Contains(string(help), flag) {
			return fmt.Errorf("Codex CLI does not support managed launch parameter %s", flag)
		}
	}
	return nil
}
