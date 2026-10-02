package steerlineapp

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

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

const executionDirectoryVersion = "builtin-execution-v2"

var requirementToolPatterns = map[string]*regexp.Regexp{
	"git":    regexp.MustCompile(`^git version ([0-9][a-zA-Z0-9.+_-]*)$`),
	"go":     regexp.MustCompile(`^go version go([0-9][a-zA-Z0-9.+_-]*) [a-zA-Z0-9]+/[a-zA-Z0-9]+$`),
	"python": regexp.MustCompile(`^Python ([0-9][a-zA-Z0-9.+_-]*)$`),
}

// NewBuiltinDirectory constructs this application's fixed implementation
// catalog. Every returned directory is independent and immutable.
func NewBuiltinDirectory(processHost executionapi.ManagedProcessHost) (*steerline.ExecutionDirectory, error) {
	if processHost == nil {
		return nil, errors.New("managed process host is required")
	}
	codecs := []steerline.ImplementationProfileCodec{
		{Key: codexworker.ProfileCodecKey, Version: 1, Normalize: func(payload []byte, policy executionapi.ExecutionPolicy, host, agent string) ([]byte, []string, error) {
			return codexworker.NormalizeProfilePayload(payload, policy, runtime.GOOS, host, agent)
		}},
		{Key: orcaworker.ProfileCodecKey, Version: 1, Normalize: func(payload []byte, _ executionapi.ExecutionPolicy, host, _ string) ([]byte, []string, error) {
			return orcaworker.NormalizeProfilePayload(payload, host)
		}},
		{Key: remote.ProfileCodecKey, Version: 1, Normalize: func(payload []byte, _ executionapi.ExecutionPolicy, _, _ string) ([]byte, []string, error) {
			return remote.NormalizeProfilePayload(payload)
		}},
	}
	localKey := steerline.ExecutionRegistrationKey{Backend: executionapi.BackendLocal, Agent: "codex", Protocol: executionapi.ProtocolLocalCodexAppServer}
	terminalKey := steerline.ExecutionRegistrationKey{Backend: executionapi.BackendOrca, Agent: "codex", Protocol: executionapi.ProtocolOrcaTerminalV2}
	supervisedKey := steerline.ExecutionRegistrationKey{Backend: executionapi.BackendOrca, Agent: "codex", Protocol: executionapi.ProtocolOrcaSupervised}
	entries := []steerline.ExecutionRegistration{
		{
			Key: localKey, ProfileKey: "local/local_codex_app_server_v1/codex", WorkspaceBranchPrefix: "codex", LocalWorkspace: true, ProfileCodecKeys: []string{codexworker.ProfileCodecKey},
			QuestionCodecKey: codexworker.QuestionRouteImplementationKey, QuestionCodecVersion: 1, Host: "local",
			Backend: func(c steerline.Config) executionapi.ExecutionBackend {
				return localExecutionBackend{config: c, target: localTarget(), host: processHost}
			},
			Target: func(steerline.Config) executionapi.ExecutionTarget { return localTarget() },
			ProbeRequirements: func(ctx context.Context, c steerline.Config) (executionapi.CapabilityReport, error) {
				version, err := policyToolVersion(ctx, c)
				return capabilityReport(localKey.Backend, localKey.Protocol, localTarget(), agentFor(c), version), err
			},
			QueryTool: queryHostRequirementTool,
			Lifecycle: func(setup executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
				setup.ProcessHost = processHost
				return codexworker.NewExecutionLifecycle(setup)
			},
			ValidateConfig:            validateLocalConfig,
			Preflight:                 preflightLocalConfig,
			PrepareClaim:              prepareLocalClaim,
			BuildPolicyResource:       buildPolicyResource,
			VerifyPolicyResource:      verifyPolicyResource,
			LinkedWorktreeWriteArgs:   linkedWorktreeWriteArgs,
			ValidTurnID:               codexworker.ValidNativeID,
			ValidateRef:               validateCatalogRef,
			CleanupCapabilities:       []string{"start", "observe", "stop", "workspace_inspection", "output_reading"},
			ValidatePublicationPolicy: validateLocalPublicationPolicy,
		},
		{
			Key: terminalKey, ProfileKey: "orca/orca_terminal_v2/codex", WorkspaceBranchPrefix: "codex", LocalWorkspace: true, ProfileCodecKeys: []string{codexworker.ProfileCodecKey, orcaworker.ProfileCodecKey},
			QuestionCodecKey: orcaworker.QuestionRouteImplementationKey, QuestionCodecVersion: 1, Host: "orca",
			Backend: func(c steerline.Config) executionapi.ExecutionBackend {
				return orcaworker.NewBackend(nativeTerminalConfig(c, terminalTarget(c, terminalKey)))
			},
			Target: func(c steerline.Config) executionapi.ExecutionTarget { return terminalTarget(c, terminalKey) },
			ProbeRequirements: func(ctx context.Context, c steerline.Config) (executionapi.CapabilityReport, error) {
				return orcaworker.NewBackend(nativeTerminalConfig(c, terminalTarget(c, terminalKey))).Probe(ctx, terminalTarget(c, terminalKey), agentFor(c))
			},
			QueryTool:                 queryTerminalRequirementTool,
			Lifecycle:                 orcaworker.NewExecutionLifecycle,
			ValidateProfileSelection:  validateTerminalProfileSelection,
			ValidateConfig:            validateTerminalConfig,
			Preflight:                 preflightTerminalConfig,
			PrepareClaim:              prepareTerminalClaim,
			BuildPolicyResource:       buildPolicyResource,
			VerifyPolicyResource:      verifyPolicyResource,
			LinkedWorktreeWriteArgs:   linkedWorktreeWriteArgs,
			ValidTurnID:               stableTurnID,
			ValidateRef:               validateCatalogRef,
			CleanupCapabilities:       []string{"start", "observe", "stop", "workspace_inspection", "output_reading"},
			ValidatePublicationPolicy: validateWorkerPublicationPolicy,
		},
		{
			Key: supervisedKey, ProfileKey: "orca/orca_supervised/codex", WorkspaceBranchPrefix: "codex", ProfileCodecKeys: []string{codexworker.ProfileCodecKey, orcaworker.ProfileCodecKey, remote.ProfileCodecKey},
			QuestionCodecKey: orcaworker.QuestionRouteImplementationKey, QuestionCodecVersion: 1, Host: "orca_supervised",
			Backend: func(c steerline.Config) executionapi.ExecutionBackend {
				return remote.NewBackend(remoteWorkerConfig(c))
			},
			Target: func(c steerline.Config) executionapi.ExecutionTarget { return supervisedTarget(c, supervisedKey) },
			ProbeRequirements: func(ctx context.Context, c steerline.Config) (executionapi.CapabilityReport, error) {
				return remote.NewBackend(remoteWorkerConfig(c)).Probe(ctx, supervisedTarget(c, supervisedKey), agentFor(c))
			},
			QueryTool:                 queryRemoteRequirementTool,
			Lifecycle:                 remote.NewExecutionLifecycle,
			ValidateProfileSelection:  validateSupervisedProfileSelection,
			ValidateConfig:            validateSupervisedConfig,
			Preflight:                 preflightSupervisedConfig,
			PrepareClaim:              prepareSupervisedClaim,
			BuildPolicyResource:       buildPolicyResource,
			VerifyPolicyResource:      verifyPolicyResource,
			LinkedWorktreeWriteArgs:   linkedWorktreeWriteArgs,
			ValidTurnID:               stableTurnID,
			ValidateRef:               validateCatalogRef,
			CleanupCapabilities:       []string{"start", "observe", "workspace_inspection", "output_reading", "message_inbox", "resource_settlement"},
			ValidatePublicationPolicy: validateWorkerPublicationPolicy,
		},
	}
	return steerline.NewExecutionDirectory(executionDirectoryVersion, codecs, entries)
}

func agentFor(c steerline.Config) executionapi.AgentSpec {
	return executionapi.AgentSpec{Name: c.Agent}
}

func stableTurnID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, digit := range value {
		if !(digit >= '0' && digit <= '9' || digit >= 'a' && digit <= 'f') {
			return false
		}
	}
	return true
}

func localTarget() executionapi.ExecutionTarget {
	hostname, _ := os.Hostname()
	return executionapi.ExecutionTarget{ID: "local:" + runtime.GOOS + ":" + hostname}
}

func terminalTarget(c steerline.Config, key steerline.ExecutionRegistrationKey) executionapi.ExecutionTarget {
	settings, err := terminalSettings(c)
	if err != nil {
		return executionapi.ExecutionTarget{}
	}
	return executionapi.ExecutionTarget{ID: string(key.Backend) + ":" + shortDigest(struct {
		Executable string
		Args       []string
	}{settings.Executable, settings.Args})[:24]}
}

func supervisedTarget(c steerline.Config, _ steerline.ExecutionRegistrationKey) executionapi.ExecutionTarget {
	settings, err := remoteSettings(c)
	if err != nil || settings.ConnectionID == "" {
		return executionapi.ExecutionTarget{}
	}
	return executionapi.ExecutionTarget{ID: "orca-supervised:" + settings.ConnectionID, ConnectionID: settings.ConnectionID, ExternalRuntime: settings.EnvironmentID}
}

func codexSettings(c steerline.Config) (codexworker.ProfileConfig, error) {
	payload, err := c.DecodeImplementationConfig(codexworker.ProfileCodecKey)
	if err != nil {
		return codexworker.ProfileConfig{}, err
	}
	return codexworker.DecodeProfileConfig(payload)
}

func terminalSettings(c steerline.Config) (orcaworker.ProfileConfig, error) {
	payload, err := c.DecodeImplementationConfig(orcaworker.ProfileCodecKey)
	if err != nil {
		return orcaworker.ProfileConfig{}, err
	}
	return orcaworker.DecodeProfileConfig(payload)
}

func remoteSettings(c steerline.Config) (remote.ProfileConfig, error) {
	payload, err := c.DecodeImplementationConfig(remote.ProfileCodecKey)
	if err != nil {
		return remote.ProfileConfig{}, err
	}
	return remote.DecodeProfileConfig(payload)
}

func validateTerminalProfileSelection(policy executionapi.ExecutionPolicy, agent string, payloads map[string][]byte) error {
	return orcaworker.ValidateProfileSelection(policy, agent, payloads[codexworker.ProfileCodecKey], payloads[orcaworker.ProfileCodecKey])
}

func validateSupervisedProfileSelection(policy executionapi.ExecutionPolicy, agent string, payloads map[string][]byte) error {
	return remote.ValidateProfileSelection(policy, agent, payloads[codexworker.ProfileCodecKey], payloads[orcaworker.ProfileCodecKey], payloads[remote.ProfileCodecKey])
}

func validateLocalConfig(_ context.Context, c steerline.Config) error {
	settings, err := codexSettings(c)
	if err != nil {
		return err
	}
	permissions, err := codexworker.CompilePolicy(c.ExecutionPolicy, runtime.GOOS, c.Host, c.Agent, settings.WindowsSandbox, settings.Profile, settings.Args)
	if err != nil {
		return err
	}
	return (codexworker.Config{Policy: c.ExecutionPolicy, Args: settings.Args, Sandbox: permissions.Sandbox, WindowsSandbox: permissions.WindowsSandbox, ReasoningEffort: settings.ReasoningEffort}).Validate(runtime.GOOS)
}

func validateTerminalConfig(_ context.Context, c steerline.Config) error {
	settings, err := codexSettings(c)
	if err != nil {
		return err
	}
	if settings.Profile != "" || c.ExecutionPolicy.Filesystem.WriteScope != "workspace" {
		return &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "terminal host requires workspace-write and an empty agent profile"}
	}
	return nil
}

func validateSupervisedConfig(_ context.Context, c steerline.Config) error {
	_, err := remoteSettings(c)
	return err
}

func validateWorkerPublicationPolicy(policy executionapi.PublicationPolicy, provider publication.Config) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	if err := provider.ValidateRemote(strings.TrimSuffix(provider.BaseURL, "/") + "/" + provider.Repository + ".git"); err != nil {
		return errors.New("publication provider requires a valid kind/base_url/repository")
	}
	queryAvailable := provider.Kind == "gitea" || provider.Kind == "command" && provider.Query.Executable != ""
	if policy.Publisher == executionapi.PublisherWorker && queryAvailable {
		return nil
	}
	return executionapi.UnsupportedAdapterOperation("PublicationPolicy", "publication_policy is unsupported by the selected implementation/provider")
}

func validateLocalPublicationPolicy(policy executionapi.PublicationPolicy, provider publication.Config) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	if err := provider.ValidateRemote(strings.TrimSuffix(provider.BaseURL, "/") + "/" + provider.Repository + ".git"); err != nil {
		return errors.New("publication provider requires a valid kind/base_url/repository")
	}
	if policy.Publisher == executionapi.PublisherHostService && provider.Kind == "command" && strings.TrimSuffix(provider.BaseURL, "/") == "https://github.com" && provider.Query.Executable != "" {
		return nil
	}
	return executionapi.UnsupportedAdapterOperation("PublicationPolicy", "publication_policy is unsupported by the selected implementation/provider")
}

func preflightLocalConfig(ctx context.Context, c steerline.Config) error {
	settings, err := codexSettings(c)
	if err != nil {
		return err
	}
	executable, err := exec.LookPath(settings.Executable)
	if err != nil {
		return err
	}
	command := steerline.Command{Executable: executable, Args: append([]string(nil), settings.Args...)}
	runner := steerline.Runner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}
	return codexworker.ProbeCapabilities(ctx, codexworker.ProbeConfig{Executable: executable, Args: settings.Args, Repository: c.Repository, RunDir: c.RunDir}, codexworker.ProbePort{
		Output: func(ctx context.Context, executable string, args []string) ([]byte, error) {
			return runner.Output(ctx, steerline.Command{Executable: executable, Args: args})
		},
		HistoryAvailable: func(ctx context.Context) bool {
			historyCommand := codexworker.ReaderCommand(command, settings.Profile)
			return codexworker.HistoryAvailable(ctx, historyCommand, c.Repository, c.ProcessHost.StartManaged)
		},
		StartManaged: func(command *exec.Cmd) (func(), error) {
			owned, err := c.ProcessHost.StartManaged(command)
			if err != nil {
				return nil, err
			}
			return owned.Close, nil
		},
	})
}

func preflightTerminalConfig(ctx context.Context, c steerline.Config) error {
	settings, err := terminalSettings(c)
	if err != nil {
		return err
	}
	if _, err := (steerline.Runner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}).Output(ctx, steerline.Command{Executable: settings.Executable, Args: settings.Args}, "status", "--json"); err != nil {
		return fmt.Errorf("terminal host is not reachable: %w", err)
	}
	client := orcaworker.New(nativeTerminalConfig(c, terminalTarget(c, steerline.ExecutionRegistrationKey{Backend: executionapi.BackendOrca})))
	_, err = orcaworker.FindRepo(ctx, client.Call, c.Repository)
	if errors.Is(err, orcaworker.ErrRepoUnavailable) {
		return executionapi.UnsupportedAdapterOperation("Preflight", "the fixed repository is not registered on the selected terminal host")
	}
	return err
}

func preflightSupervisedConfig(ctx context.Context, c steerline.Config) error {
	_, err := remote.Preflight(ctx, remoteWorkerConfig(c))
	if err != nil {
		var remoteErr *remote.Error
		if errors.As(err, &remoteErr) {
			return &executionapi.BackendError{Code: executionapi.BackendErrorCode(remoteErr.Code), Diagnostic: remoteErr.Diagnostic, Cause: remoteErr.Cause}
		}
	}
	return err
}

func prepareLocalClaim(ctx context.Context, c steerline.Config) (steerline.ClaimPreparation, error) {
	_, err := policyToolVersion(ctx, c)
	return steerline.ClaimPreparation{}, err
}

func prepareTerminalClaim(ctx context.Context, c steerline.Config) (steerline.ClaimPreparation, error) {
	if _, err := policyToolVersion(ctx, c); err != nil {
		return steerline.ClaimPreparation{}, err
	}
	settings, err := codexSettings(c)
	if err != nil {
		return steerline.ClaimPreparation{}, err
	}
	if runtime.GOOS != "windows" || settings.WindowsSandbox == "" {
		return steerline.ClaimPreparation{}, nil
	}
	if _, err := managedTerminalCommand(c, "", nil); err != nil {
		return steerline.ClaimPreparation{}, err
	}
	timeout, err := time.ParseDuration(mustTerminalSettings(c).WaitTimeout)
	if err != nil || timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return steerline.ClaimPreparation{RequiresReadiness: true, WaitTimeout: timeout}, nil
}

func prepareSupervisedClaim(ctx context.Context, c steerline.Config) (steerline.ClaimPreparation, error) {
	_, err := policyToolVersion(ctx, c)
	return steerline.ClaimPreparation{}, err
}

func mustTerminalSettings(c steerline.Config) orcaworker.ProfileConfig {
	settings, _ := terminalSettings(c)
	return settings
}

func policyToolVersion(ctx context.Context, c steerline.Config) (string, error) {
	settings, err := codexSettings(c)
	if err != nil {
		return "", err
	}
	command := steerline.Command{Executable: settings.Executable}
	if err := resolveNativeCommand(&command); err != nil {
		return "", err
	}
	out, err := (steerline.Runner{Dir: c.Repository, Timeout: min(positiveDuration(c.CommandTimeout, 10*time.Second), 10*time.Second)}).Output(ctx, command, "--version")
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(out))
	return version, codexworker.VerifyPolicyVersion(version)
}

func managedTerminalCommand(c steerline.Config, path string, roots []string) (string, error) {
	worker, err := codexSettings(c)
	if err != nil {
		return "", err
	}
	terminal, err := terminalSettings(c)
	if err != nil {
		return "", err
	}
	permissions, err := codexworker.CompilePolicy(c.ExecutionPolicy, runtime.GOOS, c.Host, c.Agent, worker.WindowsSandbox, worker.Profile, worker.Args)
	if err != nil {
		return "", err
	}
	if path != "" {
		if err := codexworker.ValidateManagedArgs(worker.Args); err != nil {
			return "", err
		}
		native, err := codexworker.NativeExecutable(worker.Executable)
		if err != nil {
			return "", err
		}
		worker.Executable = native
	}
	command, err := codexworker.ManagedCommand(codexworker.ManagedCommandOptions{Policy: c.ExecutionPolicy,
		WorkingDirectory: path, WriteRoots: roots, Host: c.Host, Agent: c.Agent, Model: worker.Model,
		ReasoningEffort: worker.ReasoningEffort, Executable: worker.Executable, Args: worker.Args,
		Sandbox: permissions.Sandbox, WindowsSandbox: permissions.WindowsSandbox})
	if err != nil {
		return "", err
	}
	return orcaworker.WrapAgentCommand(command, terminal.Shell)
}

func nativeTerminalConfig(c steerline.Config, target executionapi.ExecutionTarget) orcaworker.Config {
	terminal, _ := terminalSettings(c)
	worker, _ := codexSettings(c)
	config := orcaworker.Config{Repository: c.Repository, CommandTimeout: c.CommandTimeout, Target: target,
		Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
			return (steerline.Runner{Dir: c.Repository, Timeout: timeout}).Output(ctx, steerline.Command{Executable: terminal.Executable, Args: terminal.Args}, args...)
		}, InspectWorkspace: func(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
			return inspectBoundWorkspace(ctx, c, ref, c.ProcessHost)
		}}
	if runtime.GOOS == "windows" && worker.WindowsSandbox != "" || worker.Model != "" || worker.ReasoningEffort != "" || worker.Profile != "" {
		config.AgentVersion = func(ctx context.Context) (string, error) {
			out, err := (steerline.Runner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}).Output(ctx, steerline.Command{Executable: worker.Executable, Args: worker.Args}, "--version")
			return string(out), err
		}
	}
	return config
}

func remoteWorkerConfig(c steerline.Config) remote.Config {
	worker, _ := codexSettings(c)
	settings, _ := remoteSettings(c)
	client := orcaworker.New(nativeTerminalConfig(c, executionapi.ExecutionTarget{}))
	config := remote.Config{Repository: c.Repository, ProjectID: c.Identity(), Agent: c.Agent, Model: worker.Model,
		ReasoningEffort: worker.ReasoningEffort, CommandTimeout: c.CommandTimeout, Call: client.Raw,
		Origin: func(ctx context.Context) ([]byte, error) {
			return (steerline.Runner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}).Output(ctx, c.Git, "remote", "get-url", c.Remote)
		}}
	config.Settings = remote.Settings{ConnectionID: settings.ConnectionID, EnvironmentID: settings.EnvironmentID,
		RepositorySelector: settings.RepositorySelector, CoordinatorTerminal: settings.CoordinatorTerminal,
		ExpectedCodexVersion: settings.ExpectedCodexVersion, PollInterval: settings.PollInterval}
	return config
}

func buildPolicyResource(ctx context.Context, request steerline.PolicyResourceBuildRequest) (steerline.PolicyResourceBuildResult, error) {
	identity := codexworker.PolicyResourceIdentity{ProjectID: request.Resource.ProjectID, ExecutionID: request.Resource.ExecutionID,
		AttemptID: request.Resource.AttemptID, Generation: request.Resource.Generation, ConfigDigest: request.Resource.ConfigDigest}
	bound, err := codexworker.BuildPolicyResourceEnvelope(ctx, codexworker.PolicyResourceBuildRequest{
		ProfileData: request.ProfileData, ProfileIdentity: codexworker.ProfileConfigIdentity{ProjectID: request.ProjectID,
			ProfileID: request.ProfileID, AcceptedConfigVersion: request.AcceptedConfigVersion},
		ResourceIdentity: identity, Policy: request.Policy, Host: request.Host, Agent: request.Agent,
		GOOS: runtime.GOOS, Workspace: request.Workspace, LinkedWriteArgs: request.LinkedWorktreeArgs,
	}, func(ctx context.Context, executable string) (string, error) {
		timeout := request.CommandTimeout
		if timeout <= 0 || timeout > 10*time.Second {
			timeout = 10 * time.Second
		}
		out, err := (steerline.Runner{Dir: request.RepositoryPath, Timeout: timeout}).Output(ctx, steerline.Command{Executable: executable}, "--version")
		return strings.TrimSpace(string(out)), err
	})
	if err != nil {
		return steerline.PolicyResourceBuildResult{}, err
	}
	return steerline.PolicyResourceBuildResult{Envelope: bound.Envelope, WriteRoots: append([]string(nil), bound.WriteRoots...)}, nil
}

func verifyPolicyResource(envelope executionapi.ImplementationDataEnvelope, identity steerline.PolicyResourceIdentity) error {
	_, err := codexworker.DecodePolicyResourceEnvelope(envelope, codexworker.PolicyResourceIdentity{ProjectID: identity.ProjectID,
		ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID, Generation: identity.Generation, ConfigDigest: identity.ConfigDigest})
	return err
}

func linkedWorktreeWriteArgs(ctx context.Context, c steerline.Config, worktreePath string) ([]string, error) {
	if c.ExecutionPolicy.Filesystem.WriteScope != "workspace" || !c.ExecutionPolicy.Filesystem.GitMetadataWrite {
		return nil, nil
	}
	if c.Host != "local" && c.Host != "orca" {
		return nil, executionapi.UnsupportedAdapterOperation("PrepareWorkspace", "this implementation does not expose local linked-worktree metadata")
	}
	if c.Identity() == "" {
		return nil, errors.New("repository identity must be resolved before preparing write roots")
	}
	resources, err := workspace.ResolveLinkedResourcesAt(ctx, workspaceGit(c, c.ProcessHost), workspace.Repository{Path: c.Repository, CommonDir: c.Identity()}, worktreePath)
	if err != nil {
		return nil, err
	}
	return codexworker.LinkedWorktreeWriteArgs(resources.Objects, resources.Refs, resources.Logs, resources.Admin)
}

func queryHostRequirementTool(ctx context.Context, c steerline.Config, id string) executionapi.RequirementEvidence {
	target := targetForConfig(c)
	evidence := executionapi.RequirementEvidence{ID: id, Site: "target", Target: target, Status: executionapi.RequirementUnknown, Source: "target:inherited-path/version-query", CheckedAt: time.Now().UTC()}
	command := steerline.Command{Executable: id}
	args := []string{"--version"}
	if id == "git" {
		command = c.Git
		evidence.Source = "target:frozen-git/version-query"
	}
	if id == "go" {
		args = []string{"version"}
	}
	parser := requirementToolPatterns[id]
	if parser == nil {
		return evidence
	}
	var environment []string
	if id != "git" {
		environment = steerline.WorkerProcessEnvironment(c, &steerline.Run{}, os.Environ())
	}
	executable, err := requirementExecutable(command.Executable, environment)
	if err != nil {
		evidence.Status = executionapi.RequirementMissing
		return evidence
	}
	command.Executable = executable
	runner := steerline.Runner{Dir: c.Repository, Timeout: min(positiveDuration(c.CommandTimeout, 10*time.Second), 10*time.Second), Environment: environment}
	if id == "go" {
		runner.Env = map[string]string{"GOTOOLCHAIN": "local"}
		evidence.Source = "target:inherited-path/local-go-toolchain-version-query"
	}
	out, err := runner.Output(ctx, command, args...)
	if err != nil {
		return evidence
	}
	match := parser.FindStringSubmatch(strings.TrimSpace(string(out)))
	if len(match) == 2 {
		evidence.Status, evidence.Version = executionapi.RequirementPresent, match[1]
	}
	return evidence
}

func queryTerminalRequirementTool(ctx context.Context, c steerline.Config, id string) executionapi.RequirementEvidence {
	if id == "git" {
		return queryHostRequirementTool(ctx, c, id)
	}
	return executionapi.RequirementEvidence{ID: id, Site: "target", Target: targetForConfig(c), Status: executionapi.RequirementUnknown,
		Source: "terminal:path-query-unavailable", CheckedAt: time.Now().UTC()}
}

func queryRemoteRequirementTool(_ context.Context, c steerline.Config, id string) executionapi.RequirementEvidence {
	return executionapi.RequirementEvidence{ID: id, Site: "target", Target: targetForConfig(c), Status: executionapi.RequirementUnknown,
		Source: "remote:fixed-read-only-tool-query-unavailable", CheckedAt: time.Now().UTC()}
}

func targetForConfig(c steerline.Config) executionapi.ExecutionTarget {
	entry, err := c.ExecutionDirectory.Resolve(c)
	if err != nil {
		return executionapi.ExecutionTarget{}
	}
	return entry.Target(c)
}

func requirementExecutable(name string, environment []string) (string, error) {
	if environment == nil || strings.ContainsAny(name, `/\`) {
		return exec.LookPath(name)
	}
	path := ""
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		if key == "PATH" || runtime.GOOS == "windows" && strings.EqualFold(key, "PATH") {
			path = value
			break
		}
	}
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		name += ".exe"
	}
	for _, dir := range filepath.SplitList(path) {
		if !filepath.IsAbs(dir) {
			continue
		}
		candidate, err := exec.LookPath(filepath.Join(dir, name))
		if err == nil {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func capabilityReport(backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, target executionapi.ExecutionTarget, agent executionapi.AgentSpec, version string) executionapi.CapabilityReport {
	capabilities := []executionapi.Capability{{Name: "start", Status: executionapi.CapabilityNotTested}, {Name: "observe", Status: executionapi.CapabilityNotTested}, {Name: "stop", Status: executionapi.CapabilityNotTested}, {Name: "detach_observation", Status: executionapi.CapabilityNotTested}, {Name: "operation_reconciliation", Status: executionapi.CapabilityNotTested}, {Name: "workspace_inspection", Status: executionapi.CapabilityNotTested}, {Name: "output_reading", Status: executionapi.CapabilityNotTested}, {Name: "output_turns", Status: executionapi.CapabilityNotTested}, {Name: "output_final_answer", Status: executionapi.CapabilityNotTested}, {Name: "resource_settlement", Status: executionapi.CapabilityUnsupported}}
	return executionapi.CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: time.Now().UTC(), Capabilities: capabilities}
}

type localExecutionBackend struct {
	config steerline.Config
	target executionapi.ExecutionTarget
	host   executionapi.ManagedProcessHost
}

func (backend localExecutionBackend) Identity() (executionapi.BackendKind, executionapi.ExecutionProtocol) {
	return executionapi.BackendLocal, executionapi.ProtocolLocalCodexAppServer
}

func (backend localExecutionBackend) Probe(ctx context.Context, target executionapi.ExecutionTarget, agent executionapi.AgentSpec) (executionapi.CapabilityReport, error) {
	if agent.Name != "codex" || target != backend.target {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "fixed execution target or agent changed"}
	}
	settings, err := codexSettings(backend.config)
	if err != nil {
		return executionapi.CapabilityReport{}, err
	}
	out, err := (steerline.Runner{Dir: backend.config.Repository, Timeout: positiveDuration(backend.config.CommandTimeout, 30*time.Second)}).Output(ctx, steerline.Command{Executable: settings.Executable, Args: settings.Args}, "--version")
	if err != nil {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "configured execution version probe failed", Cause: err}
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendProtocolInvalid, Diagnostic: "configured execution version probe returned no version"}
	}
	return capabilityReport(executionapi.BackendLocal, executionapi.ProtocolLocalCodexAppServer, target, agent, version), nil
}

func (backend localExecutionBackend) Observe(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.Observation, error) {
	if err := backend.validateRef(ref); err != nil {
		return executionapi.Observation{}, err
	}
	if err := ctx.Err(); err != nil {
		return executionapi.Observation{}, err
	}
	label := backend.host.InspectProcess(&ref.External)
	liveness := "unverifiable"
	if label == "matching process still exists" {
		liveness = "live"
	} else if label == "process absent" {
		liveness = "exited"
	} else if strings.Contains(label, "different process") {
		return executionapi.Observation{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: label}
	}
	return executionapi.Observation{Liveness: liveness, Connection: "connected", ObservedAt: time.Now().UTC(), Source: "process-probe", Diagnostic: label}, nil
}

func (backend localExecutionBackend) InspectWorkspace(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
	if err := backend.validateRef(ref); err != nil {
		return executionapi.WorkspaceInspection{}, err
	}
	return inspectBoundWorkspace(ctx, backend.config, ref, backend.host)
}

func (backend localExecutionBackend) StopExecution(ctx context.Context, ref executionapi.ExecutionRef, _ executionapi.Operation, handle executionapi.StopHandle) (executionapi.StopReceipt, error) {
	if err := backend.validateRef(ref); err != nil {
		return executionapi.StopReceipt{}, err
	}
	if handle == nil || handle.Identity() != ref.External {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "stop handle does not match the fixed process identity"}
	}
	if err := ctx.Err(); err != nil {
		return executionapi.StopReceipt{}, err
	}
	observation, err := backend.Observe(ctx, ref)
	if err != nil {
		return executionapi.StopReceipt{}, err
	}
	if observation.Liveness == "exited" {
		return executionapi.StopReceipt{Disposition: "confirmed", Diagnostic: observation.Diagnostic}, nil
	}
	if observation.Liveness != "live" {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "process liveness cannot be verified"}
	}
	handle.Kill()
	return executionapi.StopReceipt{Disposition: "accepted"}, nil
}

func (backend localExecutionBackend) validateRef(ref executionapi.ExecutionRef) error {
	if ref.Backend != executionapi.BackendLocal || ref.Protocol != executionapi.ProtocolLocalCodexAppServer || ref.Target.ID != backend.target.ID || ref.Target.ConnectionID != backend.target.ConnectionID || ref.Target.ExternalRuntime != backend.target.ExternalRuntime || ref.External.Kind != "os" || ref.External.PID <= 0 || ref.External.Created == "" || ref.External.Executable == "" {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "fixed process target or identity changed"}
	}
	return nil
}

func remoteWorkerError(err error) error {
	var remoteErr *remote.Error
	if errors.As(err, &remoteErr) {
		return &executionapi.BackendError{Code: executionapi.BackendErrorCode(remoteErr.Code), Diagnostic: remoteErr.Diagnostic, Cause: remoteErr.Cause}
	}
	return err
}

func inspectBoundWorkspace(ctx context.Context, c steerline.Config, ref executionapi.ExecutionRef, processHost executionapi.ManagedProcessHost) (executionapi.WorkspaceInspection, error) {
	if ref.Incomplete || ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || !regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`).MatchString(ref.BaseCommit) {
		return executionapi.WorkspaceInspection{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "workspace ref is incomplete"}
	}
	if c.Identity() == "" {
		if err := c.ResolveIdentity(ctx); err != nil {
			return executionapi.WorkspaceInspection{}, err
		}
	}
	inspection, err := workspace.Inspect(ctx, workspaceGit(c, processHost), workspace.Repository{Path: c.Repository, CommonDir: c.Identity()}, &executionapi.WorkspaceIntent{Path: ref.WorkspacePath, Branch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit})
	if err != nil {
		return executionapi.WorkspaceInspection{}, err
	}
	return executionapi.WorkspaceInspection{Repository: inspection.Repository, Worktree: inspection.Worktree, Branch: inspection.Branch, Base: inspection.Base, Head: inspection.Head, Dirty: inspection.Dirty, CheckedAt: inspection.CheckedAt}, nil
}

func workspaceGit(c steerline.Config, processHost executionapi.ManagedProcessHost) workspace.Git {
	return workspace.Git{Command: executionapi.LaunchCommand{Executable: c.Git.Executable, Args: append([]string(nil), c.Git.Args...)}, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second), Start: processHost.StartManaged}
}

func targetFor(c steerline.Config, key steerline.ExecutionRegistrationKey) executionapi.ExecutionTarget {
	entry, err := c.ExecutionDirectory.ResolveIdentity(key)
	if err != nil {
		return executionapi.ExecutionTarget{}
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

func resolveNativeCommand(command *steerline.Command) error {
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

func validateCatalogRef(ref executionapi.ExecutionRef) error {
	switch ref.Protocol {
	case executionapi.ProtocolLocalCodexAppServer, executionapi.ProtocolLocalProcess:
		if ref.Backend != executionapi.BackendLocal || ref.External.Kind != "os" || ref.External.PID <= 0 || ref.External.Created == "" || ref.External.Executable == "" {
			return errors.New("local resource lacks its fixed process identity")
		}
	case executionapi.ProtocolOrcaTerminalV2:
		if ref.Backend != executionapi.BackendOrca || ref.External.Kind != "orca" || ref.External.Terminal == "" || ref.External.RuntimeID == "" || ref.External.Incarnation == "" {
			return errors.New("terminal resource lacks its fixed runtime/terminal/incarnation")
		}
	case executionapi.ProtocolOrcaSupervised:
		if ref.Backend != executionapi.BackendOrca || ref.External.Kind != "orca_supervised" || ref.External.RuntimeID == "" || ref.External.OrchestrationRun == "" || ref.External.Task == "" || ref.External.Dispatch == "" || ref.External.ExecutionHost == "" || ref.External.WorktreeID == "" {
			return errors.New("supervised resource lacks its fixed run/task/dispatch/host/worktree")
		}
	default:
		return errors.New("unsupported resource protocol")
	}
	return nil
}
