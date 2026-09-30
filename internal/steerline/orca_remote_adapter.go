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
	cfg := remote.Config{Repository: c.Repository, ProjectID: c.identity, Agent: c.agentName(), Model: c.Model, ReasoningEffort: c.ReasoningEffort, CommandTimeout: c.CommandTimeout, Call: nativeOrcaClient(c).Raw, Origin: func(ctx context.Context) ([]byte, error) {
		return (Runner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}).Output(ctx, c.Git, "remote", "get-url", c.Remote)
	}}
	if sc := c.Orca.Supervised; sc != nil {
		cfg.Settings = remote.Settings{ConnectionID: sc.ConnectionID, EnvironmentID: sc.EnvironmentID, RepositorySelector: sc.RepositorySelector, CoordinatorTerminal: sc.CoordinatorTerminal, ExpectedCodexVersion: sc.ExpectedCodexVersion, PollInterval: sc.PollInterval}
	}
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
	if err := validateSupervisedOrcaConfig(&c); err != nil {
		return supervisedPreflight{}, err
	}
	v, e := remote.Preflight(ctx, remoteWorkerConfig(c))
	return v, remoteWorkerError(e)
}

const maxSupervisedMessageBytes = remote.MaxMessageBytes
