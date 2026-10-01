package steerline

import (
	"context"
	"errors"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
)

type supervisedPreflight = remote.PreflightResult
type supervisedStartReceipt = remote.StartReceipt

func remoteWorkerConfig(c Config) remote.Config {
	codex := codexProfileSettings(c)
	supervised := remoteProfileSettings(c)
	cfg := remote.Config{Repository: c.Repository, ProjectID: c.identity, Agent: c.agentName(), Model: codex.Model, ReasoningEffort: codex.ReasoningEffort, CommandTimeout: c.CommandTimeout, Call: nativeOrcaClient(c).Raw, Origin: func(ctx context.Context) ([]byte, error) {
		return (Runner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}).Output(ctx, c.Git, "remote", "get-url", c.Remote)
	}}
	cfg.Settings = remote.Settings{ConnectionID: supervised.ConnectionID, EnvironmentID: supervised.EnvironmentID, RepositorySelector: supervised.RepositorySelector, CoordinatorTerminal: supervised.CoordinatorTerminal, ExpectedCodexVersion: supervised.ExpectedCodexVersion, PollInterval: supervised.PollInterval}
	return cfg
}
func remoteWorkerClient(c Config) remote.Client { return remote.New(remoteWorkerConfig(c)) }
func remoteWorkerError(err error) error {
	var e *remote.Error
	if errors.As(err, &e) {
		return &executionapi.BackendError{Code: executionapi.BackendErrorCode(e.Code), Diagnostic: e.Diagnostic, Cause: e.Cause}
	}
	return err
}
func remoteWorkerRef(ref executionapi.ExecutionRef) remote.Ref {
	return remote.FromExecutionRef(ref)
}
func probeSupervisedOrcaTarget(ctx context.Context, c Config) (supervisedPreflight, error) {
	if err := validateImplementationProfileData(c); err != nil {
		return supervisedPreflight{}, err
	}
	v, e := remote.Preflight(ctx, remoteWorkerConfig(c))
	return v, remoteWorkerError(e)
}

const maxSupervisedMessageBytes = remote.MaxMessageBytes
