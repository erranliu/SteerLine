package steerline

import (
	"context"
	"errors"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

type orcaTerminal = orcaworker.Terminal
type orcaTerminalExitCause = orcaworker.TerminalExitCause

func positiveDuration(s string, def time.Duration) time.Duration {
	if d, e := time.ParseDuration(s); e == nil && d > 0 {
		return d
	}
	return def
}
func nativeOrcaClient(c Config) orcaworker.Client { return orcaworker.New(nativeOrcaConfig(c)) }
func nativeOrcaConfig(c Config) orcaworker.Config {
	orca := orcaProfileSettings(c)
	codex := codexProfileSettings(c)
	cfg := orcaworker.Config{Repository: c.Repository, CommandTimeout: c.CommandTimeout, Target: targetFor(c), Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
		return (Runner{Dir: c.Repository, Timeout: timeout}).Output(ctx, Command{Executable: orca.Executable, Args: orca.Args}, args...)
	}, InspectWorkspace: func(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
		return inspectBoundWorkspace(ctx, c, ref)
	}}
	if c.needsCodexExecutable() {
		cfg.AgentVersion = func(ctx context.Context) (string, error) {
			out, e := (Runner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}).Output(ctx, Command{Executable: codex.Executable, Args: codex.Args}, "--version")
			return string(out), e
		}
	}
	return cfg
}
func validateOrcaRepositoryRegistration(ctx context.Context, c Config) error {
	_, err := orcaworker.FindRepo(ctx, nativeOrcaClient(c).Call, c.Repository)
	if errors.Is(err, orcaworker.ErrRepoUnavailable) {
		return executionapi.UnsupportedAdapterOperation("Preflight", "native Orca project repository is not registered on the local Orca host; register the fixed repository before claim")
	}
	return err
}
