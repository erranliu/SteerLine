package app

import (
	"context"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// supervisedExecutionRegistration captures only this catalog instance; callbacks run after binding.
func supervisedExecutionRegistration(processHost executionbackend.ManagedProcessHost, directory func() *execution.ExecutionDirectory) (execution.ImplementationProfileCodec, execution.ExecutionRegistration) {
	key := execution.ExecutionRegistrationKey{Backend: executionbackend.BackendOrca, Agent: "*", Protocol: executionbackend.ProtocolOrcaSupervised}
	codec := execution.ImplementationProfileCodec{Key: remote.ProfileCodecKey, Version: remote.ProfileCodecVersion, Normalize: func(payload []byte, _, _ string) ([]byte, []string, error) {
		return remote.NormalizeProfilePayload(payload)
	}}
	return codec, execution.ExecutionRegistration{
		ServiceStopBehavior: remote.ServiceStopBehavior, Key: key, ProfileKey: "orca/orca_supervised", WorkspaceBranchPrefix: "worker", ProfileCodecKeys: []string{remote.ProfileCodecKey},
		QuestionCodecKey: orcaworker.QuestionRouteImplementationKey, QuestionCodecVersion: 1, Host: "orca_supervised",
		Backend: func(c execution.Config) executionbackend.ExecutionBackend {
			return remote.NewBackend(remoteWorkerConfig(c, processHost, directory()))
		},
		Target: func(c execution.Config) executionbackend.ExecutionTarget {
			return supervisedTarget(c, key, directory())
		},

		Lifecycle:                remote.NewExecutionLifecycle,
		ValidateProfileSelection: validateSupervisedProfileSelection,
		ValidateConfig: func(ctx context.Context, c execution.Config) error {
			return validateSupervisedConfig(ctx, c, directory())
		},
		Preflight: func(ctx context.Context, c execution.Config) error {
			return preflightSupervisedConfig(ctx, c, processHost, directory())
		},
		PrepareClaim: func(ctx context.Context, c execution.Config) (execution.ClaimPreparation, error) {
			return prepareSupervisedClaim(ctx, c, directory(), processHost)
		},
		ReadinessRequired:         func(execution.Config) bool { return false },
		BuildPolicyResource:       buildRemotePolicyResource,
		VerifyPolicyResource:      verifyRemotePolicyResource,
		LinkedWorktreeWriteArgs:   func(context.Context, execution.Config, string) ([]string, error) { return nil, nil },
		ValidTurnID:               executionbackend.StableExecutionID,
		ValidateRef:               validateCatalogRef,
		CleanupCapabilities:       []string{"start", "observe", "workspace_inspection", "output_reading", "message_inbox", "resource_settlement"},
		ValidatePublicationPolicy: validateWorkerPublicationPolicy,
	}
}

func supervisedTarget(c execution.Config, _ execution.ExecutionRegistrationKey, directory *execution.ExecutionDirectory) executionbackend.ExecutionTarget {
	settings, err := remoteSettings(c, directory)
	if err != nil {
		return executionbackend.ExecutionTarget{}
	}
	return (remote.Config{Settings: remote.Settings{Target: settings.Target, ConnectionID: settings.ConnectionID, EnvironmentID: settings.EnvironmentID}}).Target()
}

func remoteSettings(c execution.Config, directories ...*execution.ExecutionDirectory) (remote.ProfileConfig, error) {
	payload, err := c.DecodeImplementationConfig(remote.ProfileCodecKey, directories...)
	if err != nil {
		return remote.ProfileConfig{}, err
	}
	return remote.DecodeProfileConfig(payload)
}

func validateSupervisedProfileSelection(agent string, payloads map[string][]byte) error {
	return remote.ValidateProfileSelection(agent, payloads[remote.ProfileCodecKey])
}

func validateSupervisedConfig(_ context.Context, c execution.Config, directory *execution.ExecutionDirectory) error {
	_, err := remoteSettings(c, directory)
	return err
}

func preflightSupervisedConfig(ctx context.Context, c execution.Config, processHost executionbackend.ManagedProcessHost, directory *execution.ExecutionDirectory) error {
	_, err := remote.Preflight(ctx, remoteWorkerConfig(c, processHost, directory))
	if err != nil {
		var remoteErr *remote.Error
		if errors.As(err, &remoteErr) {
			return &executionbackend.BackendError{Code: executionbackend.BackendErrorCode(remoteErr.Code), Diagnostic: remoteErr.Diagnostic, Cause: remoteErr.Cause}
		}
	}
	return err
}

func prepareSupervisedClaim(ctx context.Context, c execution.Config, directory *execution.ExecutionDirectory, processHost executionbackend.ManagedProcessHost) (execution.ClaimPreparation, error) {
	// Orca launches the agent on the paired host using that host's native
	// settings. A center-local Codex binary cannot attest the remote launch.
	// The mandatory capability preflight follows PrepareClaim in the claim
	// application; do not repeat all remote calls in the same control request.
	return execution.ClaimPreparation{}, nil
}

func remoteWorkerConfig(c execution.Config, processHost executionbackend.ManagedProcessHost, directory *execution.ExecutionDirectory) remote.Config {
	settings, _ := remoteSettings(c, directory)
	call := func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
		all := append(append([]string(nil), settings.Args...), args...)
		return (execution.Runner{Dir: c.Repository, ProcessHost: processHost, Timeout: timeout}).Output(ctx, executionbackend.Command{Executable: settings.Executable}, all...)
	}
	return remote.Config{Repository: c.Repository, ProjectID: c.Identity(), Agent: c.Agent, Model: settings.Model, ReasoningEffort: settings.ReasoningEffort, CoordinatorShell: settings.Shell, CommandTimeout: c.CommandTimeout, Call: call,
		Settings: remote.Settings{Target: settings.Target, ConnectionID: settings.ConnectionID, EnvironmentID: settings.EnvironmentID, RepositorySelector: settings.RepositorySelector, CoordinatorTerminal: settings.CoordinatorTerminal, PollInterval: settings.PollInterval},
		Origin: func(ctx context.Context) ([]byte, error) {
			return (execution.Runner{Dir: c.Repository, ProcessHost: processHost, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}).Output(ctx, c.Git, "remote", "get-url", c.Remote)
		}}
}

func buildRemotePolicyResource(_ context.Context, r execution.PolicyResourceBuildRequest) (execution.PolicyResourceBuildResult, error) {
	envelope, err := remote.BuildNativeResource(remote.PolicyResourceIdentity{ProjectID: r.Resource.ProjectID, ExecutionID: r.Resource.ExecutionID, AttemptID: r.Resource.AttemptID, Generation: r.Resource.Generation, ConfigDigest: r.Resource.ConfigDigest}, r.ProfileID, r.AcceptedConfigVersion, r.Workspace, r.Agent, r.ProfileData)
	return execution.PolicyResourceBuildResult{Envelope: envelope, WriteRoots: []string{r.Workspace}}, err
}
func verifyRemotePolicyResource(envelope executionbackend.ImplementationDataEnvelope, r execution.PolicyResourceIdentity) error {
	_, err := remote.DecodeNativeResource(envelope, remote.PolicyResourceIdentity{ProjectID: r.ProjectID, ExecutionID: r.ExecutionID, AttemptID: r.AttemptID, Generation: r.Generation, ConfigDigest: r.ConfigDigest})
	return err
}
