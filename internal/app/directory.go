package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
)

const executionDirectoryVersion = "builtin-execution-v3"

// NewBuiltinDirectory constructs this application's fixed implementation
// catalog. Every returned directory is independent and immutable.
func NewBuiltinDirectory(processHost executionbackend.ManagedProcessHost) (*execution.ExecutionDirectory, error) {
	if processHost == nil {
		return nil, errors.New("managed process host is required")
	}
	var directory *execution.ExecutionDirectory
	codecs := []execution.ImplementationProfileCodec{
		{Key: codexworker.ProfileCodecKey, Version: codexworker.ProfileCodecVersion, Normalize: func(payload []byte, host, agent string) ([]byte, []string, error) {
			return codexworker.NormalizeProfilePayload(payload, runtime.GOOS, host, agent)
		}},
		{Key: orcaworker.ProfileCodecKey, Version: 1, Normalize: func(payload []byte, host, _ string) ([]byte, []string, error) {
			return orcaworker.NormalizeProfilePayload(payload, host)
		}},
		{Key: remote.ProfileCodecKey, Version: remote.ProfileCodecVersion, Normalize: func(payload []byte, _, _ string) ([]byte, []string, error) {
			return remote.NormalizeProfilePayload(payload)
		}},
	}
	localKey := execution.ExecutionRegistrationKey{Backend: executionbackend.BackendLocal, Agent: "codex", Protocol: executionbackend.ProtocolLocalCodexAppServer}
	terminalKey := execution.ExecutionRegistrationKey{Backend: executionbackend.BackendOrca, Agent: "codex", Protocol: executionbackend.ProtocolOrcaTerminalV2}
	supervisedKey := execution.ExecutionRegistrationKey{Backend: executionbackend.BackendOrca, Agent: "*", Protocol: executionbackend.ProtocolOrcaSupervised}
	entries := []execution.ExecutionRegistration{
		{
			Key: localKey, ProfileKey: "local/local_codex_app_server_v1/codex", WorkspaceBranchPrefix: "codex", LocalWorkspace: true, ProfileCodecKeys: []string{codexworker.ProfileCodecKey},
			QuestionCodecKey: codexworker.QuestionRouteImplementationKey, QuestionCodecVersion: 1, Host: "local",
			Backend: func(c execution.Config) executionbackend.ExecutionBackend {
				return localExecutionBackend{config: c, target: localTarget(), host: processHost, directory: directory}
			},
			Target: func(execution.Config) executionbackend.ExecutionTarget { return localTarget() },

			Lifecycle: func(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
				setup.ProcessHost = processHost
				return codexworker.NewExecutionLifecycle(setup)
			},
			ValidateConfig: func(ctx context.Context, c execution.Config) error { return validateLocalConfig(ctx, c, directory) },
			Preflight: func(ctx context.Context, c execution.Config) error {
				return preflightLocalConfig(ctx, c, directory, processHost)
			},
			PrepareClaim: func(ctx context.Context, c execution.Config) (execution.ClaimPreparation, error) {
				return prepareLocalClaim(ctx, c, directory, processHost)
			},
			ReadinessRequired:    func(execution.Config) bool { return false },
			BuildPolicyResource:  buildPolicyResource,
			VerifyPolicyResource: verifyPolicyResource,
			LinkedWorktreeWriteArgs: func(ctx context.Context, c execution.Config, path string) ([]string, error) {
				return linkedWorktreeWriteArgs(ctx, c, path, processHost)
			},
			ValidTurnID:               codexworker.ValidNativeID,
			ValidateRef:               validateCatalogRef,
			CleanupCapabilities:       []string{"start", "observe", "stop", "workspace_inspection", "output_reading"},
			ValidatePublicationPolicy: validateLocalPublicationPolicy,
		},
		{
			Key: terminalKey, ProfileKey: "orca/orca_terminal_v2/codex", WorkspaceBranchPrefix: "codex", LocalWorkspace: true, ProfileCodecKeys: []string{codexworker.ProfileCodecKey, orcaworker.ProfileCodecKey},
			QuestionCodecKey: orcaworker.QuestionRouteImplementationKey, QuestionCodecVersion: 1, Host: "orca",
			Backend: func(c execution.Config) executionbackend.ExecutionBackend {
				return orcaworker.NewBackend(nativeTerminalConfig(c, terminalTarget(c, terminalKey, directory), processHost, directory))
			},
			Target: func(c execution.Config) executionbackend.ExecutionTarget {
				return terminalTarget(c, terminalKey, directory)
			},

			Lifecycle:                orcaworker.NewExecutionLifecycle,
			ValidateProfileSelection: validateTerminalProfileSelection,
			ValidateConfig:           func(ctx context.Context, c execution.Config) error { return validateTerminalConfig(ctx, c, directory) },
			Preflight: func(ctx context.Context, c execution.Config) error {
				return preflightTerminalConfig(ctx, c, directory, processHost)
			},
			PrepareClaim: func(ctx context.Context, c execution.Config) (execution.ClaimPreparation, error) {
				return prepareTerminalClaim(ctx, c, directory, processHost)
			},
			ReadinessRequired:    func(c execution.Config) bool { return terminalReadinessRequired(c, directory) },
			BuildPolicyResource:  buildPolicyResource,
			VerifyPolicyResource: verifyPolicyResource,
			LinkedWorktreeWriteArgs: func(ctx context.Context, c execution.Config, path string) ([]string, error) {
				return linkedWorktreeWriteArgs(ctx, c, path, processHost)
			},
			ValidTurnID:               executionbackend.StableExecutionID,
			ValidateRef:               validateCatalogRef,
			CleanupCapabilities:       []string{"start", "observe", "stop", "workspace_inspection", "output_reading"},
			ValidatePublicationPolicy: validateWorkerPublicationPolicy,
		},
		{
			Key: supervisedKey, ProfileKey: "orca/orca_supervised", WorkspaceBranchPrefix: "worker", ProfileCodecKeys: []string{remote.ProfileCodecKey},
			QuestionCodecKey: orcaworker.QuestionRouteImplementationKey, QuestionCodecVersion: 1, Host: "orca_supervised",
			Backend: func(c execution.Config) executionbackend.ExecutionBackend {
				return remote.NewBackend(remoteWorkerConfig(c, processHost, directory))
			},
			Target: func(c execution.Config) executionbackend.ExecutionTarget {
				return supervisedTarget(c, supervisedKey, directory)
			},

			Lifecycle:                remote.NewExecutionLifecycle,
			ValidateProfileSelection: validateSupervisedProfileSelection,
			ValidateConfig: func(ctx context.Context, c execution.Config) error {
				return validateSupervisedConfig(ctx, c, directory)
			},
			Preflight: func(ctx context.Context, c execution.Config) error {
				return preflightSupervisedConfig(ctx, c, processHost, directory)
			},
			PrepareClaim: func(ctx context.Context, c execution.Config) (execution.ClaimPreparation, error) {
				return prepareSupervisedClaim(ctx, c, directory, processHost)
			},
			ReadinessRequired:         func(execution.Config) bool { return false },
			BuildPolicyResource:       buildRemotePolicyResource,
			VerifyPolicyResource:      verifyRemotePolicyResource,
			LinkedWorktreeWriteArgs:   func(context.Context, execution.Config, string) ([]string, error) { return nil, nil },
			ValidTurnID:               executionbackend.StableExecutionID,
			ValidateRef:               validateCatalogRef,
			CleanupCapabilities:       []string{"start", "observe", "workspace_inspection", "output_reading", "message_inbox", "resource_settlement"},
			ValidatePublicationPolicy: validateWorkerPublicationPolicy,
		},
	}
	var err error
	directory, err = execution.NewExecutionDirectory(executionDirectoryVersion, codecs, entries)
	return directory, err
}

func agentFor(c execution.Config) executionbackend.AgentSpec {
	return executionbackend.AgentSpec{Name: c.Agent}
}

func localTarget() executionbackend.ExecutionTarget {
	hostname, _ := os.Hostname()
	return executionbackend.ExecutionTarget{ID: "local:" + runtime.GOOS + ":" + hostname}
}

func terminalTarget(c execution.Config, _ execution.ExecutionRegistrationKey, directory *execution.ExecutionDirectory) executionbackend.ExecutionTarget {
	settings, err := terminalSettings(c, directory)
	if err != nil {
		return executionbackend.ExecutionTarget{}
	}
	return executionbackend.ExecutionTarget{ID: orcaworker.NativeTargetID(settings)}
}

func supervisedTarget(c execution.Config, _ execution.ExecutionRegistrationKey, directory *execution.ExecutionDirectory) executionbackend.ExecutionTarget {
	settings, err := remoteSettings(c, directory)
	if err != nil || settings.ConnectionID == "" {
		return executionbackend.ExecutionTarget{}
	}
	return executionbackend.ExecutionTarget{ID: "orca-supervised:" + settings.ConnectionID, ConnectionID: settings.ConnectionID, ExternalRuntime: settings.EnvironmentID}
}

func codexSettings(c execution.Config, directories ...*execution.ExecutionDirectory) (codexworker.ProfileConfig, error) {
	payload, err := c.DecodeImplementationConfig(codexworker.ProfileCodecKey, directories...)
	if err != nil {
		return codexworker.ProfileConfig{}, err
	}
	return codexworker.DecodeProfileConfig(payload)
}

func terminalSettings(c execution.Config, directories ...*execution.ExecutionDirectory) (orcaworker.ProfileConfig, error) {
	payload, err := c.DecodeImplementationConfig(orcaworker.ProfileCodecKey, directories...)
	if err != nil {
		return orcaworker.ProfileConfig{}, err
	}
	return orcaworker.DecodeProfileConfig(payload)
}

func remoteSettings(c execution.Config, directories ...*execution.ExecutionDirectory) (remote.ProfileConfig, error) {
	payload, err := c.DecodeImplementationConfig(remote.ProfileCodecKey, directories...)
	if err != nil {
		return remote.ProfileConfig{}, err
	}
	return remote.DecodeProfileConfig(payload)
}

func validateTerminalProfileSelection(agent string, payloads map[string][]byte) error {
	return orcaworker.ValidateProfileSelection(agent, payloads[codexworker.ProfileCodecKey], payloads[orcaworker.ProfileCodecKey])
}

func validateSupervisedProfileSelection(agent string, payloads map[string][]byte) error {
	return remote.ValidateProfileSelection(agent, payloads[remote.ProfileCodecKey])
}

func validateLocalConfig(_ context.Context, c execution.Config, directory *execution.ExecutionDirectory) error {
	settings, err := codexSettings(c, directory)
	if err != nil {
		return err
	}
	permissions, err := codexworker.NativePermissions(runtime.GOOS, c.Host, c.Agent)
	if err != nil {
		return err
	}
	return (codexworker.Config{Args: nil, Sandbox: permissions.Sandbox, WindowsSandbox: permissions.WindowsSandbox, ReasoningEffort: settings.ReasoningEffort}).Validate(runtime.GOOS)
}

func validateTerminalConfig(_ context.Context, c execution.Config, directories ...*execution.ExecutionDirectory) error {
	var directory *execution.ExecutionDirectory
	if len(directories) > 0 {
		directory = directories[0]
	}
	_, err := codexSettings(c, directory)
	if err != nil {
		return err
	}
	return nil
}

func validateSupervisedConfig(_ context.Context, c execution.Config, directory *execution.ExecutionDirectory) error {
	_, err := remoteSettings(c, directory)
	return err
}

func validateWorkerPublicationPolicy(policy executionbackend.PublicationPolicy, provider publication.Config) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	if err := provider.ValidateRemote(strings.TrimSuffix(provider.BaseURL, "/") + "/" + provider.Repository + ".git"); err != nil {
		return errors.New("publication provider requires a valid kind/base_url/repository")
	}
	queryAvailable := provider.Kind == "gitea" || provider.Kind == "command" && provider.Query.Executable != ""
	if policy.Publisher == executionbackend.PublisherWorker && queryAvailable {
		return nil
	}
	return executionbackend.UnsupportedAdapterOperation("PublicationPolicy", "publication_policy is unsupported by the selected implementation/provider")
}

func validateLocalPublicationPolicy(policy executionbackend.PublicationPolicy, provider publication.Config) error {
	return validateWorkerPublicationPolicy(policy, provider)
}

func preflightLocalConfig(ctx context.Context, c execution.Config, directory *execution.ExecutionDirectory, processHost executionbackend.ManagedProcessHost) error {
	settings, err := codexSettings(c, directory)
	if err != nil {
		return err
	}
	executable, err := exec.LookPath(settings.Executable)
	if err != nil {
		return err
	}
	command := executionbackend.Command{Executable: executable, Args: []string(nil)}
	runner := execution.Runner{Dir: c.Repository, ProcessHost: processHost, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}
	return codexworker.ProbeCapabilities(ctx, codexworker.ProbeConfig{Executable: executable, Args: nil, Repository: c.Repository, RunDir: c.RunDir}, codexworker.ProbePort{
		Output: func(ctx context.Context, executable string, args []string) ([]byte, error) {
			return runner.Output(ctx, executionbackend.Command{Executable: executable, Args: args})
		},
		CheckHistory: func(ctx context.Context) error {
			historyCommand := codexworker.ReaderCommand(command, "")
			return codexworker.CheckHistory(ctx, historyCommand, c.Repository, processHost.StartManaged)
		},
		StartManaged: func(command *exec.Cmd) (func(), error) {
			owned, err := processHost.StartManaged(command)
			if err != nil {
				return nil, err
			}
			return owned.Close, nil
		},
	})
}

func preflightTerminalConfig(ctx context.Context, c execution.Config, directory *execution.ExecutionDirectory, processHost executionbackend.ManagedProcessHost) error {
	if _, err := prepareTerminalClaim(ctx, c, directory, processHost); err != nil {
		return err
	}
	settings, err := terminalSettings(c, directory)
	if err != nil {
		return err
	}
	if _, err := (execution.Runner{Dir: c.Repository, ProcessHost: processHost, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}).Output(ctx, executionbackend.Command{Executable: settings.Executable, Args: settings.Args}, "status", "--json"); err != nil {
		return fmt.Errorf("terminal host is not reachable: %w", err)
	}
	client := orcaworker.New(nativeTerminalConfig(c, terminalTarget(c, execution.ExecutionRegistrationKey{Backend: executionbackend.BackendOrca}, directory), processHost, directory))
	_, err = orcaworker.FindRepo(ctx, client.Call, c.Repository)
	if errors.Is(err, orcaworker.ErrRepoUnavailable) {
		return executionbackend.UnsupportedAdapterOperation("Preflight", "the fixed repository is not registered on the selected terminal host")
	}
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

func prepareLocalClaim(ctx context.Context, c execution.Config, directory *execution.ExecutionDirectory, processHost executionbackend.ManagedProcessHost) (execution.ClaimPreparation, error) {
	_, err := policyToolVersion(ctx, c, directory, processHost)
	return execution.ClaimPreparation{}, err
}

func prepareTerminalClaim(ctx context.Context, c execution.Config, directory *execution.ExecutionDirectory, processHost executionbackend.ManagedProcessHost) (execution.ClaimPreparation, error) {
	if _, err := policyToolVersion(ctx, c, directory, processHost); err != nil {
		return execution.ClaimPreparation{}, err
	}
	if !terminalReadinessRequired(c, directory) {
		return execution.ClaimPreparation{}, nil
	}
	if _, err := managedTerminalCommand(c, "", nil, directory); err != nil {
		return execution.ClaimPreparation{}, err
	}
	timeout, err := time.ParseDuration(mustTerminalSettings(c, directory).WaitTimeout)
	if err != nil || timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return execution.ClaimPreparation{RequiresReadiness: true, WaitTimeout: timeout}, nil
}

// A pure fixed-profile requirement shared by claim and persisted-state
// preflight. Reopening must reject missing readiness evidence before writing.
func terminalReadinessRequired(c execution.Config, directories ...*execution.ExecutionDirectory) bool {
	var directory *execution.ExecutionDirectory
	if len(directories) > 0 {
		directory = directories[0]
	}
	_, err := codexSettings(c, directory)
	return err == nil && runtime.GOOS == "windows" && c.Agent == "codex"
}

func prepareSupervisedClaim(ctx context.Context, c execution.Config, directory *execution.ExecutionDirectory, processHost executionbackend.ManagedProcessHost) (execution.ClaimPreparation, error) {
	// Orca launches the agent on the paired host using that host's native
	// settings. A center-local Codex binary cannot attest the remote launch.
	// The mandatory capability preflight follows PrepareClaim in the claim
	// application; do not repeat all remote calls in the same control request.
	return execution.ClaimPreparation{}, nil
}

func mustTerminalSettings(c execution.Config, directory *execution.ExecutionDirectory) orcaworker.ProfileConfig {
	settings, _ := terminalSettings(c, directory)
	return settings
}

func policyToolVersion(ctx context.Context, c execution.Config, directory *execution.ExecutionDirectory, processHost executionbackend.ManagedProcessHost) (string, error) {
	settings, err := codexSettings(c, directory)
	if err != nil {
		return "", err
	}
	command := executionbackend.Command{Executable: settings.Executable}
	if err := resolveNativeCommand(&command); err != nil {
		return "", err
	}
	out, err := (execution.Runner{Dir: c.Repository, ProcessHost: processHost, Timeout: min(positiveDuration(c.CommandTimeout, 10*time.Second), 10*time.Second)}).Output(ctx, command, "--version")
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(out))
	return version, codexworker.VerifyPolicyVersion(version)
}

func managedTerminalCommand(c execution.Config, path string, roots []string, directories ...*execution.ExecutionDirectory) (string, error) {
	var directory *execution.ExecutionDirectory
	if len(directories) > 0 {
		directory = directories[0]
	}
	worker, err := codexSettings(c, directory)
	if err != nil {
		return "", err
	}
	terminal, err := terminalSettings(c, directory)
	if err != nil {
		return "", err
	}
	permissions, err := codexworker.NativePermissions(runtime.GOOS, c.Host, c.Agent)
	if err != nil {
		return "", err
	}
	if path != "" {
		native, err := codexworker.NativeExecutable(worker.Executable)
		if err != nil {
			return "", err
		}
		worker.Executable = native
	}
	command, err := codexworker.ManagedCommand(codexworker.ManagedCommandOptions{
		WorkingDirectory: path, WriteRoots: roots, Host: c.Host, Agent: c.Agent, Model: worker.Model,
		ReasoningEffort: worker.ReasoningEffort, Executable: worker.Executable, Args: nil,
		Sandbox: permissions.Sandbox, WindowsSandbox: permissions.WindowsSandbox})
	if err != nil {
		return "", err
	}
	return orcaworker.WrapAgentCommand(command, terminal.Shell)
}

func nativeTerminalConfig(c execution.Config, target executionbackend.ExecutionTarget, processHost executionbackend.ManagedProcessHost, directory *execution.ExecutionDirectory) orcaworker.Config {
	terminal, _ := terminalSettings(c, directory)
	worker, _ := codexSettings(c, directory)
	config := orcaworker.Config{Repository: c.Repository, CommandTimeout: c.CommandTimeout, Target: target,
		Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
			return (execution.Runner{Dir: c.Repository, ProcessHost: processHost, Timeout: timeout}).Output(ctx, executionbackend.Command{Executable: terminal.Executable, Args: terminal.Args}, args...)
		}, InspectWorkspace: func(ctx context.Context, ref executionbackend.ExecutionRef) (executionbackend.WorkspaceInspection, error) {
			return inspectBoundWorkspace(ctx, c, ref, processHost)
		}}
	if runtime.GOOS == "windows" || worker.Model != "" || worker.ReasoningEffort != "" {
		config.AgentVersion = func(ctx context.Context) (string, error) {
			out, err := (execution.Runner{Dir: c.Repository, ProcessHost: processHost, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}).Output(ctx, executionbackend.Command{Executable: worker.Executable, Args: nil}, "--version")
			return string(out), err
		}
	}
	return config
}

func remoteWorkerConfig(c execution.Config, processHost executionbackend.ManagedProcessHost, directory *execution.ExecutionDirectory) remote.Config {
	settings, _ := remoteSettings(c, directory)
	call := func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
		all := append(append([]string(nil), settings.Args...), args...)
		return (execution.Runner{Dir: c.Repository, ProcessHost: processHost, Timeout: timeout}).Output(ctx, executionbackend.Command{Executable: settings.Executable}, all...)
	}
	return remote.Config{Repository: c.Repository, ProjectID: c.Identity(), Agent: c.Agent, Model: settings.Model, ReasoningEffort: settings.ReasoningEffort, CoordinatorShell: settings.Shell, CommandTimeout: c.CommandTimeout, Call: call,
		Settings: remote.Settings{ConnectionID: settings.ConnectionID, EnvironmentID: settings.EnvironmentID, RepositorySelector: settings.RepositorySelector, CoordinatorTerminal: settings.CoordinatorTerminal, PollInterval: settings.PollInterval},
		Origin: func(ctx context.Context) ([]byte, error) {
			return (execution.Runner{Dir: c.Repository, ProcessHost: processHost, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}).Output(ctx, c.Git, "remote", "get-url", c.Remote)
		}}
}

func buildPolicyResource(ctx context.Context, request execution.PolicyResourceBuildRequest) (execution.PolicyResourceBuildResult, error) {
	identity := codexworker.PolicyResourceIdentity{ProjectID: request.Resource.ProjectID, ExecutionID: request.Resource.ExecutionID,
		AttemptID: request.Resource.AttemptID, Generation: request.Resource.Generation, ConfigDigest: request.Resource.ConfigDigest}
	bound, err := codexworker.BuildPolicyResourceEnvelope(ctx, codexworker.PolicyResourceBuildRequest{
		ProfileData: request.ProfileData, ProfileIdentity: codexworker.ProfileConfigIdentity{ProjectID: request.ProjectID,
			ProfileID: request.ProfileID, AcceptedConfigVersion: request.AcceptedConfigVersion},
		ResourceIdentity: identity, Host: request.Host, Agent: request.Agent,
		GOOS: runtime.GOOS, Workspace: request.Workspace, LinkedWriteArgs: request.LinkedWorktreeArgs,
	}, func(ctx context.Context, executable string) (string, error) {
		timeout := request.CommandTimeout
		if timeout <= 0 || timeout > 10*time.Second {
			timeout = 10 * time.Second
		}
		out, err := (execution.Runner{Dir: request.RepositoryPath, ProcessHost: request.ProcessHost, Timeout: timeout}).Output(ctx, executionbackend.Command{Executable: executable}, "--version")
		return strings.TrimSpace(string(out)), err
	})
	if err != nil {
		return execution.PolicyResourceBuildResult{}, err
	}
	return execution.PolicyResourceBuildResult{Envelope: bound.Envelope, WriteRoots: append([]string(nil), bound.WriteRoots...)}, nil
}

func verifyPolicyResource(envelope executionbackend.ImplementationDataEnvelope, identity execution.PolicyResourceIdentity) error {
	_, err := codexworker.DecodePolicyResourceEnvelope(envelope, codexworker.PolicyResourceIdentity{ProjectID: identity.ProjectID,
		ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID, Generation: identity.Generation, ConfigDigest: identity.ConfigDigest})
	return err
}

func linkedWorktreeWriteArgs(ctx context.Context, c execution.Config, worktreePath string, processHost executionbackend.ManagedProcessHost) ([]string, error) {
	if c.Host != "local" && c.Host != "orca" {
		return nil, executionbackend.UnsupportedAdapterOperation("PrepareWorkspace", "this implementation does not expose local linked-worktree metadata")
	}
	if c.Identity() == "" {
		return nil, errors.New("repository identity must be resolved before preparing write roots")
	}
	resources, err := workspace.ResolveLinkedResourcesAt(ctx, workspaceGit(c, processHost), workspace.Repository{Path: c.Repository, CommonDir: c.Identity()}, worktreePath)
	if err != nil {
		return nil, err
	}
	return codexworker.LinkedWorktreeWriteArgs(resources.Objects, resources.Refs, resources.Logs, resources.Admin)
}

func targetForConfig(c execution.Config, directory *execution.ExecutionDirectory) executionbackend.ExecutionTarget {
	entry, err := directory.Resolve(c)
	if err != nil {
		return executionbackend.ExecutionTarget{}
	}
	return entry.Target(c)
}

func capabilityReport(backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec, version string) executionbackend.CapabilityReport {
	capabilities := []executionbackend.Capability{{Name: "start", Status: executionbackend.CapabilityNotTested}, {Name: "observe", Status: executionbackend.CapabilityNotTested}, {Name: "stop", Status: executionbackend.CapabilityNotTested}, {Name: "detach_observation", Status: executionbackend.CapabilityNotTested}, {Name: "operation_reconciliation", Status: executionbackend.CapabilityNotTested}, {Name: "workspace_inspection", Status: executionbackend.CapabilityNotTested}, {Name: "output_reading", Status: executionbackend.CapabilityNotTested}, {Name: "output_turns", Status: executionbackend.CapabilityNotTested}, {Name: "output_final_answer", Status: executionbackend.CapabilityNotTested}, {Name: "resource_settlement", Status: executionbackend.CapabilityUnsupported}}
	return executionbackend.CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: time.Now().UTC(), Capabilities: capabilities}
}

type localExecutionBackend struct {
	config    execution.Config
	target    executionbackend.ExecutionTarget
	host      executionbackend.ManagedProcessHost
	directory *execution.ExecutionDirectory
}

func (backend localExecutionBackend) Identity() (executionbackend.BackendKind, executionbackend.ExecutionProtocol) {
	return executionbackend.BackendLocal, executionbackend.ProtocolLocalCodexAppServer
}

func (backend localExecutionBackend) Probe(ctx context.Context, target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec) (executionbackend.CapabilityReport, error) {
	if agent.Name != "codex" || target != backend.target {
		return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "fixed execution target or agent changed"}
	}
	settings, err := codexSettings(backend.config, backend.directory)
	if err != nil {
		return executionbackend.CapabilityReport{}, err
	}
	out, err := (execution.Runner{Dir: backend.config.Repository, ProcessHost: backend.host, Timeout: positiveDuration(backend.config.CommandTimeout, 30*time.Second)}).Output(ctx, executionbackend.Command{Executable: settings.Executable, Args: nil}, "--version")
	if err != nil {
		return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendUnreachable, Diagnostic: "configured execution version probe failed", Cause: err}
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendProtocolInvalid, Diagnostic: "configured execution version probe returned no version"}
	}
	return capabilityReport(executionbackend.BackendLocal, executionbackend.ProtocolLocalCodexAppServer, target, agent, version), nil
}

func (backend localExecutionBackend) Observe(ctx context.Context, ref executionbackend.ExecutionRef) (executionbackend.Observation, error) {
	if err := backend.validateRef(ref); err != nil {
		return executionbackend.Observation{}, err
	}
	if err := ctx.Err(); err != nil {
		return executionbackend.Observation{}, err
	}
	label := backend.host.InspectProcess(&ref.External)
	liveness := "unverifiable"
	if label == "matching process still exists" {
		liveness = "live"
	} else if label == "process absent" {
		liveness = "exited"
	} else if strings.Contains(label, "different process") {
		return executionbackend.Observation{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: label}
	}
	return executionbackend.Observation{Liveness: liveness, Connection: "connected", ObservedAt: time.Now().UTC(), Source: "process-probe", Diagnostic: label}, nil
}

func (backend localExecutionBackend) InspectWorkspace(ctx context.Context, ref executionbackend.ExecutionRef) (executionbackend.WorkspaceInspection, error) {
	if err := backend.validateRef(ref); err != nil {
		return executionbackend.WorkspaceInspection{}, err
	}
	return inspectBoundWorkspace(ctx, backend.config, ref, backend.host)
}

func (backend localExecutionBackend) StopExecution(ctx context.Context, ref executionbackend.ExecutionRef, _ executionbackend.Operation, handle executionbackend.StopHandle) (executionbackend.StopReceipt, error) {
	if err := backend.validateRef(ref); err != nil {
		return executionbackend.StopReceipt{}, err
	}
	if handle == nil || handle.Identity() != ref.External {
		return executionbackend.StopReceipt{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "stop handle does not match the fixed process identity"}
	}
	if err := ctx.Err(); err != nil {
		return executionbackend.StopReceipt{}, err
	}
	observation, err := backend.Observe(ctx, ref)
	if err != nil {
		return executionbackend.StopReceipt{}, err
	}
	if observation.Liveness == "exited" {
		return executionbackend.StopReceipt{Disposition: "confirmed", Diagnostic: observation.Diagnostic}, nil
	}
	if observation.Liveness != "live" {
		return executionbackend.StopReceipt{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "process liveness cannot be verified"}
	}
	handle.Kill()
	return executionbackend.StopReceipt{Disposition: "accepted"}, nil
}

func (backend localExecutionBackend) validateRef(ref executionbackend.ExecutionRef) error {
	if ref.Backend != executionbackend.BackendLocal || ref.Protocol != executionbackend.ProtocolLocalCodexAppServer || ref.Target.ID != backend.target.ID || ref.Target.ConnectionID != backend.target.ConnectionID || ref.Target.ExternalRuntime != backend.target.ExternalRuntime || ref.External.Kind != "os" || ref.External.PID <= 0 || ref.External.Created == "" || ref.External.Executable == "" {
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "fixed process target or identity changed"}
	}
	return nil
}

func inspectBoundWorkspace(ctx context.Context, c execution.Config, ref executionbackend.ExecutionRef, processHost executionbackend.ManagedProcessHost) (executionbackend.WorkspaceInspection, error) {
	if ref.Incomplete || ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || !regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`).MatchString(ref.BaseCommit) {
		return executionbackend.WorkspaceInspection{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "workspace ref is incomplete"}
	}
	if c.Identity() == "" {
		return executionbackend.WorkspaceInspection{}, errors.New("app-resolved repository identity is required for workspace inspection")
	}
	inspection, err := workspace.Inspect(ctx, workspaceGit(c, processHost), workspace.Repository{Path: c.Repository, CommonDir: c.Identity()}, &executionbackend.WorkspaceIntent{Path: ref.WorkspacePath, Branch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit})
	if err != nil {
		return executionbackend.WorkspaceInspection{}, err
	}
	return executionbackend.WorkspaceInspection{Repository: inspection.Repository, Worktree: inspection.Worktree, Branch: inspection.Branch, Base: inspection.Base, Head: inspection.Head, Dirty: inspection.Dirty, CheckedAt: inspection.CheckedAt}, nil
}

func workspaceGit(c execution.Config, processHost executionbackend.ManagedProcessHost) workspace.Git {
	return workspace.Git{Command: executionbackend.LaunchCommand{Executable: c.Git.Executable, Args: append([]string(nil), c.Git.Args...)}, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second), Start: processHost.StartManaged}
}

func targetFor(c execution.Config, key execution.ExecutionRegistrationKey, directory *execution.ExecutionDirectory) executionbackend.ExecutionTarget {
	entry, err := directory.ResolveIdentity(key)
	if err != nil {
		return executionbackend.ExecutionTarget{}
	}
	return entry.Target(c)
}

func shortDigest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func positiveDuration(value string, fallback time.Duration) time.Duration {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func resolveNativeCommand(command *executionbackend.Command) error {
	path, err := exec.LookPath(command.Executable)
	if err != nil {
		return err
	}
	extension := strings.ToLower(filepath.Ext(path))
	if extension == ".cmd" || extension == ".bat" || extension == ".ps1" {
		return fmt.Errorf("use a native executable, not a shell shim: %s", path)
	}
	command.Executable = path
	return nil
}

func validateCatalogRef(ref executionbackend.ExecutionRef) error {
	switch ref.Protocol {
	case executionbackend.ProtocolLocalCodexAppServer, executionbackend.ProtocolLocalProcess:
		if ref.Backend != executionbackend.BackendLocal || ref.External.Kind != "os" || ref.External.PID <= 0 || ref.External.Created == "" || ref.External.Executable == "" {
			return errors.New("local resource lacks its fixed process identity")
		}
	case executionbackend.ProtocolOrcaTerminalV2:
		if ref.Backend != executionbackend.BackendOrca || ref.External.Kind != "orca" || ref.External.Terminal == "" || ref.External.RuntimeID == "" || ref.External.Incarnation == "" {
			return errors.New("terminal resource lacks its fixed runtime/terminal/incarnation")
		}
	case executionbackend.ProtocolOrcaSupervised:
		if ref.Backend != executionbackend.BackendOrca || ref.External.Kind != "orca_supervised" || ref.External.RuntimeID == "" || ref.External.OrchestrationRun == "" || ref.External.Task == "" || ref.External.Dispatch == "" || ref.External.ExecutionHost == "" || ref.External.WorktreeID == "" {
			return errors.New("supervised resource lacks its fixed run/task/dispatch/host/worktree")
		}
	default:
		return errors.New("unsupported resource protocol")
	}
	return nil
}
