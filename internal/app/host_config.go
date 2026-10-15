package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	executionprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/pathguard"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type applicationControlValidation struct{}

func (applicationControlValidation) ValidAction(action string) bool {
	return taskrunner.ValidControlAction(action)
}
func (applicationControlValidation) DecodePayload(request productapi.ControlRequest) (productapi.ControlPayload, error) {
	return taskrunner.DecodeControlPayload(request)
}
func (applicationControlValidation) ValidateRequest(request productapi.ControlRequest, instance, identity string, revision uint64, at time.Time) (string, error) {
	return taskrunner.ValidateControlRequest(request, instance, identity, revision, at)
}
func (applicationControlValidation) RequestDigest(request productapi.ControlRequest) string {
	return taskrunner.RequestDigest(request)
}
func (applicationControlValidation) TerminalStatus(status string) bool {
	return taskrunner.TerminalControlStatus(status)
}

// hostRuntimeConfig is the only conversion from the app's broad configured
// service to the host's process, control, metadata and runtime-read inputs.
func hostRuntimeConfig(ctx context.Context, config Config) (host.Config, error) {
	if err := config.RequireNativeTaskAuthority(); err != nil {
		return host.Config{}, err
	}
	if err := config.ValidateTaskCommandConfiguration(); err != nil {
		return host.Config{}, err
	}
	if err := config.ResolveIdentity(ctx); err != nil {
		return host.Config{}, err
	}
	if config.ProcessHost == nil {
		return host.Config{}, errors.New("managed execution process host is required")
	}
	runtimeInfo := CurrentRuntimeIdentity(config.ConfigPath)
	return host.Config{
		Repository: config.Repository, RepositoryIdentity: config.Identity(), Remote: config.Remote, Branch: config.Branch,
		RunDir: config.RunDir, RuntimeExecutable: runtimeInfo.Executable, RuntimeHome: runtimeInfo.Home,
		RuntimeConfig: runtimeInfo.Config, SourceCommit: runtimeInfo.SourceCommit, SourceDirty: runtimeInfo.SourceDirty,
		Claimant: config.Claimant, ConfigDigest: config.LoadedConfigDigest(),
		TaskAuthority: config.TaskAuthority, WorkflowProtocol: config.WorkflowProtocol,
		TaskPoll: config.TaskPoll, PRPoll: config.PRPoll, Heartbeat: config.Heartbeat, CommandTimeout: config.CommandTimeout, ShutdownGrace: config.ShutdownGrace,
		ProcessFacts:                executionprocess.NewServiceFacts(),
		WorktreeMetadataDirectories: []string{orcaworker.WorktreeTrashDirectory},
		RuntimeReader: func(ctx context.Context, input host.Config) (*reconstruction.Runtime, error) {
			resolved := config
			if err := syncExecutionHostFacts(&resolved, input); err != nil {
				return nil, err
			}
			return LoadDatabaseRuntime(resolved)
		},
		ValidateNativeStore: func(ctx context.Context, input host.Config) error {
			resolved := config
			if err := syncExecutionHostFacts(&resolved, input); err != nil {
				return err
			}
			return validateNativeStore(ctx, resolved)
		},
		EnsureRunDirOutsideRepo: func(ctx context.Context, input host.Config) error {
			resolved := config
			if err := syncExecutionHostFacts(&resolved, input); err != nil {
				return err
			}
			return ensureRunDirOutsideRepository(ctx, resolved)
		},
		ControlValidation: applicationControlValidation{},
	}, nil
}

// BuildHostConfig extracts the narrow process-host inputs from a fully loaded
// application config for callers that need to invoke a host operation directly.
func BuildHostConfig(ctx context.Context, config Config) (host.Config, error) {
	return hostRuntimeConfig(ctx, config)
}

func connectHostService(ctx context.Context, config Config) host.ServiceConnection {
	hostConfig, err := hostRuntimeConfig(ctx, config)
	if err != nil {
		return host.ServiceConnection{State: host.ConnectionIdentityError, Diagnostic: err.Error(), CanStart: false, Capabilities: []string{}}
	}
	return host.ConnectService(ctx, hostConfig)
}

func connectHostServiceFromSnapshot(ctx context.Context, config Config, observation host.ObservationSnapshot) host.ServiceConnection {
	hostConfig, err := hostRuntimeConfig(ctx, config)
	if err != nil {
		return host.ServiceConnection{State: host.ConnectionIdentityError, Diagnostic: err.Error(), CanStart: false, Capabilities: []string{}}
	}
	return host.ConnectServiceFromSnapshot(ctx, hostConfig, observation)
}

func observeHostRuntime(ctx context.Context, config Config, runtimeState *reconstruction.Runtime, readErr error, sampledAt time.Time) host.ObservationSnapshot {
	hostConfig, err := hostRuntimeConfig(ctx, config)
	if err != nil {
		return host.ObservationSnapshot{State: host.SnapshotReadFailed, SampledAt: sampledAt.UTC(), Error: err.Error()}
	}
	return host.ObserveRuntimeSnapshot(hostConfig, runtimeState, readErr, sampledAt)
}

func ensureRunDirOutsideRepository(ctx context.Context, config Config) error {
	if config.RunDir == "" || config.Repository == "" {
		return errors.New("repository and run_dir are required")
	}
	runDir, err := resolveProspectiveApplicationPath(config.RunDir)
	if err != nil {
		return err
	}
	repository, err := pathguard.ResolveProspective(config.Repository)
	if err != nil {
		return err
	}
	if applicationPathInside(repository, runDir) {
		return errors.New("native state directory must be outside the repository")
	}
	runner := execution.Runner{Dir: config.Repository, Timeout: positiveDuration(config.CommandTimeout, 30*time.Second), ProcessHost: config.ProcessHost}
	worktrees, err := runner.Output(ctx, config.Git, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return errors.New("cannot verify native state against linked Git worktrees")
	}
	for _, record := range strings.Split(string(worktrees), "\x00") {
		if strings.HasPrefix(record, "worktree ") && applicationPathInside(strings.TrimPrefix(record, "worktree "), runDir) {
			return errors.New("native state directory must be outside every linked repository worktree")
		}
	}
	return nil
}

func resolveProspectiveApplicationPath(input string) (string, error) {
	return pathguard.ResolveProspective(input)
}

func applicationPathInside(root, path string) bool {
	return pathguard.Within(root, path)
}
