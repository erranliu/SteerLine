package steerline

import (
	"context"
	"errors"
	"time"

	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
)

type supervisedPreflight = remote.PreflightResult
type supervisedStartReceipt = remote.StartReceipt
type supervisedWorkerObservation struct {
	Identity                                                      ProcessIdentity
	Liveness, DispatchState, AgentWaitState, WorktreePath, Branch string
	ObservedAt                                                    time.Time
}
type supervisedDelivery struct {
	ID       string
	Messages []OrcaInboxMessage
	TimedOut bool
}

func remoteWorkerConfig(c Config) remote.Config {
	cfg := remote.Config{Repository: c.Repository, ProjectID: c.identity, Agent: c.agentName(), Model: c.Model, ReasoningEffort: c.ReasoningEffort, CommandTimeout: c.CommandTimeout, Call: (orcaClient{cfg: c}).call, Origin: func(ctx context.Context) ([]byte, error) {
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
		return &BackendError{Code: BackendErrorCode(e.Code), Diagnostic: e.Diagnostic, Cause: e.Cause}
	}
	return err
}
func remoteWorkerRef(ref ExecutionRef) remote.Ref {
	return remote.Ref{Backend: string(ref.Backend), Protocol: string(ref.Protocol), RunID: ref.RunID, Target: remote.Target(ref.Target), External: remote.Identity(ref.External), WorkspacePath: ref.WorkspacePath, WorkspaceBranch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit}
}
func probeSupervisedOrcaTarget(ctx context.Context, c Config) (supervisedPreflight, error) {
	if err := validateSupervisedOrcaConfig(&c); err != nil {
		return supervisedPreflight{}, err
	}
	v, e := remote.Preflight(ctx, remoteWorkerConfig(c))
	return v, remoteWorkerError(e)
}
func (b orcaSupervisedBackend) createRunCoordinator(ctx context.Context, id, runtime string) (string, error) {
	v, e := remoteWorkerClient(b.config).CreateCoordinator(ctx, id, runtime)
	return v, remoteWorkerError(e)
}
func (b orcaSupervisedBackend) showWorker(ctx context.Context, expected supervisedStartReceipt) (supervisedWorkerObservation, error) {
	v, e := remoteWorkerClient(b.config).Show(ctx, expected)
	return supervisedWorkerObservation{Identity: ProcessIdentity(v.Identity), Liveness: v.Liveness, DispatchState: v.DispatchState, AgentWaitState: v.AgentWaitState, WorktreePath: v.WorktreePath, Branch: v.Branch, ObservedAt: v.ObservedAt}, remoteWorkerError(e)
}
func (b orcaSupervisedBackend) settledWorker(ctx context.Context, ref ExecutionRef, outcome string) (string, error) {
	v, e := remoteWorkerClient(b.config).Settlement(ctx, remoteWorkerRef(ref), outcome)
	return v, remoteWorkerError(e)
}
func (b orcaSupervisedBackend) inspectSupervisedWorktree(ctx context.Context, pair supervisedPreflight, id, path, branch, base, head string) (WorkspaceInspection, error) {
	v, e := remoteWorkerClient(b.config).InspectWorktree(ctx, pair, id, path, branch, base, head)
	return WorkspaceInspection(v), remoteWorkerError(e)
}
func (b orcaSupervisedBackend) checkDelivery(ctx context.Context, ref ExecutionRef) (supervisedDelivery, error) {
	v, e := remoteWorkerClient(b.config).Check(ctx, remoteWorkerRef(ref))
	return supervisedDelivery{ID: v.ID, Messages: v.Messages, TimedOut: v.TimedOut}, remoteWorkerError(e)
}
func (b orcaSupervisedBackend) replyQuestion(ctx context.Context, ref ExecutionRef, id, body string) (string, error) {
	v, e := remoteWorkerClient(b.config).Reply(ctx, remoteWorkerRef(ref), id, body)
	return v, remoteWorkerError(e)
}
func (b orcaSupervisedBackend) findAnsweredQuestion(ctx context.Context, ref ExecutionRef, id, body string) (string, error) {
	v, e := remoteWorkerClient(b.config).FindAnswer(ctx, remoteWorkerRef(ref), id, body)
	return v, remoteWorkerError(e)
}
func (b orcaSupervisedBackend) ReadOutput(ctx context.Context, ref ExecutionRef, cursor string, limit int) (OutputBatch, error) {
	v, e := remoteWorkerClient(b.config).ReadOutput(ctx, remoteWorkerRef(ref), cursor, limit)
	return OutputBatch{Data: v.Data, Source: v.Source, ObservedAt: v.ObservedAt, NextCursor: v.NextCursor, Gap: v.Gap, SourceChanged: v.SourceChanged}, remoteWorkerError(e)
}

const maxSupervisedMessageBytes = remote.MaxMessageBytes
