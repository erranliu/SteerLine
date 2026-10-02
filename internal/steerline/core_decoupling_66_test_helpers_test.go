package steerline

import (
	"context"

	"errors"

	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"

	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/publication"

	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

var testDirectoryOnce sync.Once
var sharedTestDirectory *ExecutionDirectory
var sharedTestDirectoryError error

func testImplementationDirectory(t *testing.T) *ExecutionDirectory {
	t.Helper()
	directory, err := mustTestDirectory()
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func testExecutionDirectory(t *testing.T) *ExecutionDirectory { return testImplementationDirectory(t) }

func mustTestDirectory() (*ExecutionDirectory, error) {
	testDirectoryOnce.Do(func() { sharedTestDirectory, sharedTestDirectoryError = newTestDirectory() })
	return sharedTestDirectory, sharedTestDirectoryError
}

func testImplementationCodecs() []ImplementationProfileCodec {
	return []ImplementationProfileCodec{
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
}

func newTestDirectory() (*ExecutionDirectory, error) {
	local := ExecutionRegistrationKey{Backend: executionapi.BackendLocal, Agent: "codex", Protocol: executionapi.ProtocolLocalCodexAppServer}
	terminal := ExecutionRegistrationKey{Backend: executionapi.BackendOrca, Agent: "codex", Protocol: executionapi.ProtocolOrcaTerminalV2}
	supervised := ExecutionRegistrationKey{Backend: executionapi.BackendOrca, Agent: "codex", Protocol: executionapi.ProtocolOrcaSupervised}
	entries := []ExecutionRegistration{
		{
			Key: local, ProfileKey: "local/local_codex_app_server_v1/codex", WorkspaceBranchPrefix: "codex", LocalWorkspace: true,
			ProfileCodecKeys: []string{codexworker.ProfileCodecKey}, QuestionCodecKey: codexworker.QuestionRouteImplementationKey,
			QuestionCodecVersion: 1, Host: "local",
			Backend: func(c Config) executionapi.ExecutionBackend { return localExecutionBackend{config: c} },
			Target:  func(Config) executionapi.ExecutionTarget { return testLocalTarget() },
			ProbeRequirements: func(ctx context.Context, c Config) (executionapi.CapabilityReport, error) {
				return capabilityReport(local.Backend, local.Protocol, testLocalTarget(), agentFor(c), "0.159.2"), nil
			},
			QueryTool: queryHostRequirementTool,
			Lifecycle: func(setup executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
				setup.ProcessHost = testProcessHost{}
				return codexworker.NewExecutionLifecycle(setup)
			},
			ValidateConfig:            func(context.Context, Config) error { return nil },
			Preflight:                 func(context.Context, Config) error { return nil },
			PrepareClaim:              testPrepareClaim,
			ReadinessRequired:         func(Config) bool { return false },
			BuildPolicyResource:       testBuildPolicyResource,
			VerifyPolicyResource:      testVerifyPolicyResource,
			LinkedWorktreeWriteArgs:   codexLinkedWorktreeWriteArgs,
			ValidTurnID:               codexworker.ValidNativeID,
			ValidateRef:               validateCatalogRef,
			CleanupCapabilities:       []string{"start", "observe", "stop", "workspace_inspection", "output_reading"},
			ValidatePublicationPolicy: testLocalPublicationPolicy,
		},
		{
			Key: terminal, ProfileKey: "orca/orca_terminal_v2/codex", WorkspaceBranchPrefix: "codex", LocalWorkspace: true,
			ProfileCodecKeys: []string{codexworker.ProfileCodecKey, orcaworker.ProfileCodecKey}, QuestionCodecKey: orcaworker.QuestionRouteImplementationKey,
			QuestionCodecVersion: 1, Host: "orca",
			Backend: func(c Config) executionapi.ExecutionBackend { return orcaworker.NewBackend(nativeOrcaConfig(c)) },
			Target:  func(c Config) executionapi.ExecutionTarget { return testTerminalTarget(c) },
			ProbeRequirements: func(ctx context.Context, c Config) (executionapi.CapabilityReport, error) {
				return orcaworker.NewBackend(nativeOrcaConfig(c)).Probe(ctx, testTerminalTarget(c), agentFor(c))
			},
			QueryTool: queryNativeRequirementTool,
			Lifecycle: orcaworker.NewExecutionLifecycle,
			ValidateProfileSelection: func(policy executionapi.ExecutionPolicy, agent string, payloads map[string][]byte) error {
				return orcaworker.ValidateProfileSelection(policy, agent, payloads[codexworker.ProfileCodecKey], payloads[orcaworker.ProfileCodecKey])
			},
			ValidateConfig:            func(context.Context, Config) error { return nil },
			Preflight:                 func(context.Context, Config) error { return nil },
			PrepareClaim:              testPrepareClaim,
			ReadinessRequired:         func(c Config) bool { return runtime.GOOS == "windows" && codexProfileSettings(c).WindowsSandbox != "" },
			BuildPolicyResource:       testBuildPolicyResource,
			VerifyPolicyResource:      testVerifyPolicyResource,
			LinkedWorktreeWriteArgs:   codexLinkedWorktreeWriteArgs,
			ValidTurnID:               testStableTurnID,
			ValidateRef:               validateCatalogRef,
			CleanupCapabilities:       []string{"start", "observe", "stop", "workspace_inspection", "output_reading"},
			ValidatePublicationPolicy: testWorkerPublicationPolicy,
		},
		{
			Key: supervised, ProfileKey: "orca/orca_supervised/codex", WorkspaceBranchPrefix: "codex",
			ProfileCodecKeys: []string{codexworker.ProfileCodecKey, orcaworker.ProfileCodecKey, remote.ProfileCodecKey}, QuestionCodecKey: orcaworker.QuestionRouteImplementationKey,
			QuestionCodecVersion: 1, Host: "orca_supervised",
			Backend: func(c Config) executionapi.ExecutionBackend { return remote.NewBackend(remoteWorkerConfig(c)) },
			Target:  func(c Config) executionapi.ExecutionTarget { return testSupervisedTarget(c) },
			ProbeRequirements: func(ctx context.Context, c Config) (executionapi.CapabilityReport, error) {
				return remote.NewBackend(remoteWorkerConfig(c)).Probe(ctx, testSupervisedTarget(c), agentFor(c))
			},
			QueryTool: queryRemoteRequirementTool,
			Lifecycle: remote.NewExecutionLifecycle,
			ValidateProfileSelection: func(policy executionapi.ExecutionPolicy, agent string, payloads map[string][]byte) error {
				return remote.ValidateProfileSelection(policy, agent, payloads[codexworker.ProfileCodecKey], payloads[orcaworker.ProfileCodecKey], payloads[remote.ProfileCodecKey])
			},
			ValidateConfig:            func(context.Context, Config) error { return nil },
			Preflight:                 func(context.Context, Config) error { return nil },
			PrepareClaim:              testPrepareClaim,
			ReadinessRequired:         func(Config) bool { return false },
			BuildPolicyResource:       testBuildPolicyResource,
			VerifyPolicyResource:      testVerifyPolicyResource,
			LinkedWorktreeWriteArgs:   codexLinkedWorktreeWriteArgs,
			ValidTurnID:               testStableTurnID,
			ValidateRef:               validateCatalogRef,
			CleanupCapabilities:       []string{"start", "observe", "workspace_inspection", "output_reading", "message_inbox", "resource_settlement"},
			ValidatePublicationPolicy: testWorkerPublicationPolicy,
		},
	}
	return NewExecutionDirectory("builtin-execution-v2", testImplementationCodecs(), entries)
}

func testDirectoryWithReplacement(t *testing.T, directory *ExecutionDirectory, replacement ExecutionRegistration) *ExecutionDirectory {
	t.Helper()
	entries := make([]ExecutionRegistration, 0, len(directory.entries))
	for key, entry := range directory.entries {
		if key == replacement.Key {
			entries = append(entries, replacement)
		} else {
			entries = append(entries, entry)
		}
	}
	clone, err := NewExecutionDirectory(directory.Version(), testImplementationCodecs(), entries)
	if err != nil {
		t.Fatal(err)
	}
	return clone
}

type testProcessHost struct{}

type localManagedProcessHost = testProcessHost
type orcaTerminal = orcaworker.Terminal
type orcaTerminalExitCause = orcaworker.TerminalExitCause
type supervisedStartReceipt = remote.StartReceipt
type supervisedPreflight = remote.PreflightResult

func NewCodexNormalizer() Normalizer { return codexworker.NewNormalizer() }

func orcaQuestionID(decision *DecisionState) string {
	if decision == nil || decision.Request.QuestionRoute == nil {
		return ""
	}
	return decision.Request.QuestionRoute.QuestionID
}

func probeSupervisedOrcaTarget(ctx context.Context, c Config) (remote.PreflightResult, error) {
	if err := validateImplementationProfileData(c); err != nil {
		return remote.PreflightResult{}, err
	}
	result, err := remote.Preflight(ctx, remoteWorkerConfig(c))
	return result, remoteWorkerError(err)
}

func (testProcessHost) StartManaged(command *exec.Cmd) (executionapi.StopHandle, error) {
	return startProcess(command)
}
func (testProcessHost) InspectProcess(identity *executionapi.ProcessIdentity) string {
	return inspectProcess(identity)
}
func (testProcessHost) NewProcessGeneration() string { return newID() }
func (testProcessHost) WriteExecutionFile(path string, value []byte) error {
	return AtomicWrite(path, value)
}
func (testProcessHost) OutputCommand(ctx context.Context, command executionapi.LaunchCommand) ([]byte, error) {
	return (Runner{Dir: command.WorkingDirectory, Timeout: 30 * time.Second}).Output(ctx, Command{Executable: command.Executable, Args: command.Args})
}

func testConfigWithDirectory(c Config) Config {
	if c.Host == "" {
		c.Host = "local"
	}
	if c.Agent == "" {
		c.Agent = "codex"
	}
	directory, err := mustTestDirectory()
	if err != nil {
		panic(err)
	}
	if c.ExecutionDirectory == nil {
		c.ExecutionDirectory = directory
	}
	c.ProcessHost = testProcessHost{}
	if c.WorktreeRoot == "" && c.RunDir != "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	}
	if c.StateRoot == "" {
		c.StateRoot = fixtureStateRoot()
	}
	if c.RepositoryStateKey == "" && c.Identity() != "" {
		c.RepositoryStateKey = RepositoryStateKey(c.Identity())
	}
	if entry, err := registrationFor(c); err == nil && c.WorkspaceBranchPrefix == "" {
		c.WorkspaceBranchPrefix = entry.WorkspaceBranchPrefix
	}
	return c
}

func newTestService(c Config) (*Service, error) {
	c = testConfigWithDirectory(c)
	return NewService(c, c.ExecutionDirectory, c.ExecutionDirectory, codexworker.NewConversationAdapter(), c.ProcessHost)
}

func testPrepareClaim(ctx context.Context, c Config) (ClaimPreparation, error) {
	settings := codexProfileSettings(c)
	executable, err := codexworker.NativeExecutable(settings.Executable)
	if err != nil {
		return ClaimPreparation{}, err
	}
	out, err := (Runner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 10*time.Second)}).Output(ctx, Command{Executable: executable}, "--version")
	if err != nil {
		return ClaimPreparation{}, err
	}
	if err := codexworker.VerifyPolicyVersion(strings.TrimSpace(string(out))); err != nil {
		return ClaimPreparation{}, err
	}
	if runtime.GOOS == "windows" && c.Host == "orca" && settings.WindowsSandbox != "" {
		if _, err := nativeOrcaFixedAgentCommand(c); err != nil {
			return ClaimPreparation{}, err
		}
		return ClaimPreparation{RequiresReadiness: true, WaitTimeout: positiveDuration(orcaProfileSettings(c).WaitTimeout, 2*time.Minute)}, nil
	}
	return ClaimPreparation{}, nil
}

func testVerifyFixedExecutionProfile(profile FixedExecutionProfile) bool {
	directory, err := mustTestDirectory()
	return err == nil && verifyFixedExecutionProfile(profile, directory)
}

func testLocalTarget() executionapi.ExecutionTarget {
	hostname, _ := os.Hostname()
	return executionapi.ExecutionTarget{ID: "local:" + runtime.GOOS + ":" + hostname}
}

func testTerminalTarget(c Config) executionapi.ExecutionTarget {
	settings := orcaProfileSettings(c)
	return executionapi.ExecutionTarget{ID: "orca:" + digestValue(struct {
		Executable string
		Args       []string
	}{settings.Executable, settings.Args})[:24]}
}

func testSupervisedTarget(c Config) executionapi.ExecutionTarget {
	settings := remoteProfileSettings(c)
	if settings.ConnectionID == "" {
		return executionapi.ExecutionTarget{}
	}
	return executionapi.ExecutionTarget{ID: "orca-supervised:" + settings.ConnectionID, ConnectionID: settings.ConnectionID, ExternalRuntime: settings.EnvironmentID}
}

func codexProfileSettings(c Config) codexworker.ProfileConfig {
	payload, err := c.DecodeImplementationConfig(codexworker.ProfileCodecKey)
	if err != nil {
		return codexworker.ProfileConfig{}
	}
	value, err := codexworker.DecodeProfileConfig(payload)
	if err != nil {
		return codexworker.ProfileConfig{}
	}
	return value
}

func orcaProfileSettings(c Config) orcaworker.ProfileConfig {
	payload, err := c.DecodeImplementationConfig(orcaworker.ProfileCodecKey)
	if err != nil {
		return orcaworker.ProfileConfig{}
	}
	value, err := orcaworker.DecodeProfileConfig(payload)
	if err != nil {
		return orcaworker.ProfileConfig{}
	}
	return value
}

func remoteProfileSettings(c Config) remote.ProfileConfig {
	payload, err := c.DecodeImplementationConfig(remote.ProfileCodecKey)
	if err != nil {
		return remote.ProfileConfig{}
	}
	value, err := remote.DecodeProfileConfig(payload)
	if err != nil {
		return remote.ProfileConfig{}
	}
	return value
}

func codexProfileFromInputs(inputs []executionapi.ImplementationConfigInput) codexworker.ProfileConfig {
	for _, input := range inputs {
		if input.ImplementationKey == codexworker.ProfileCodecKey {
			profile, _ := codexworker.DecodeProfileConfig(input.Payload)
			return profile
		}
	}
	return codexworker.ProfileConfig{}
}

func (c Config) needsCodexExecutable() bool {
	return c.Host == "local" || c.Host == "orca" && c.Agent == "codex"
}

func validatePublication(providerPolicy executionapi.PublicationPolicy, provider publication.Config, local bool) error {
	if err := providerPolicy.Validate(); err != nil {
		return err
	}
	if err := provider.ValidateRemote(strings.TrimSuffix(provider.BaseURL, "/") + "/" + provider.Repository + ".git"); err != nil {
		return err
	}
	query := provider.Kind == "gitea" || provider.Kind == "command" && provider.Query.Executable != ""
	if local {
		if providerPolicy.Publisher == executionapi.PublisherHostService && provider.Kind == "command" && strings.TrimSuffix(provider.BaseURL, "/") == "https://github.com" && provider.Query.Executable != "" {
			return nil
		}
	} else if providerPolicy.Publisher == executionapi.PublisherWorker && query {
		return nil
	}
	return executionapi.UnsupportedAdapterOperation("PublicationPolicy", "publication policy unsupported by test directory")
}

func testLocalPublicationPolicy(policy executionapi.PublicationPolicy, provider publication.Config) error {
	return validatePublication(policy, provider, true)
}
func testWorkerPublicationPolicy(policy executionapi.PublicationPolicy, provider publication.Config) error {
	return validatePublication(policy, provider, false)
}

func testStableTurnID(value string) bool {
	return len(value) == 32 && regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(value)
}

func testBuildPolicyResource(ctx context.Context, request PolicyResourceBuildRequest) (PolicyResourceBuildResult, error) {
	identity := codexworker.PolicyResourceIdentity{ProjectID: request.Resource.ProjectID, ExecutionID: request.Resource.ExecutionID, AttemptID: request.Resource.AttemptID, Generation: request.Resource.Generation, ConfigDigest: request.Resource.ConfigDigest}
	bound, err := codexworker.BuildPolicyResourceEnvelope(ctx, codexworker.PolicyResourceBuildRequest{
		ProfileData: request.ProfileData, ProfileIdentity: codexworker.ProfileConfigIdentity{ProjectID: request.ProjectID, ProfileID: request.ProfileID, AcceptedConfigVersion: request.AcceptedConfigVersion},
		ResourceIdentity: identity, Policy: request.Policy, Host: request.Host, Agent: request.Agent, GOOS: runtime.GOOS, Workspace: request.Workspace, LinkedWriteArgs: request.LinkedWorktreeArgs,
	}, func(ctx context.Context, executable string) (string, error) {
		out, err := (Runner{Dir: request.RepositoryPath, Timeout: positiveDuration(request.CommandTimeout.String(), 10*time.Second)}).Output(ctx, Command{Executable: executable}, "--version")
		return strings.TrimSpace(string(out)), err
	})
	if err != nil {
		return PolicyResourceBuildResult{}, err
	}
	return PolicyResourceBuildResult{Envelope: bound.Envelope, WriteRoots: bound.WriteRoots}, nil
}

func testVerifyPolicyResource(envelope executionapi.ImplementationDataEnvelope, identity PolicyResourceIdentity) error {
	_, err := codexworker.DecodePolicyResourceEnvelope(envelope, codexworker.PolicyResourceIdentity{ProjectID: identity.ProjectID, ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID, Generation: identity.Generation, ConfigDigest: identity.ConfigDigest})
	return err
}

type localExecutionBackend struct{ config Config }

func (backend localExecutionBackend) Identity() (executionapi.BackendKind, executionapi.ExecutionProtocol) {
	return executionapi.BackendLocal, executionapi.ProtocolLocalProcess
}
func (backend localExecutionBackend) Probe(ctx context.Context, target executionapi.ExecutionTarget, agent executionapi.AgentSpec) (executionapi.CapabilityReport, error) {
	if agent.Name != backend.config.agentName() || target != targetFor(backend.config) {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "fixed test execution identity differs"}
	}
	settings := codexProfileSettings(backend.config)
	out, err := (Runner{Dir: backend.config.Repository, Timeout: positiveDuration(backend.config.CommandTimeout, 30*time.Second)}).Output(ctx, Command{Executable: settings.Executable, Args: settings.Args}, "--version")
	if err != nil {
		return executionapi.CapabilityReport{}, err
	}
	return capabilityReport(executionapi.BackendLocal, executionapi.ProtocolLocalProcess, target, agent, strings.TrimSpace(string(out))), nil
}
func (backend localExecutionBackend) Observe(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.Observation, error) {
	if err := backend.validate(ref); err != nil {
		return executionapi.Observation{}, err
	}
	label := inspectProcess(&ref.External)
	liveness := "unverifiable"
	if label == "matching process still exists" {
		liveness = "live"
	} else if label == "process absent" {
		liveness = "exited"
	} else if strings.Contains(label, "different process") {
		return executionapi.Observation{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: label}
	}
	return executionapi.Observation{Liveness: liveness, Connection: "connected", ObservedAt: now(), Source: "os-process-probe", Diagnostic: label}, nil
}
func (backend localExecutionBackend) InspectWorkspace(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
	if err := backend.validate(ref); err != nil {
		return executionapi.WorkspaceInspection{}, err
	}
	if backend.config.Identity() == "" {
		if err := backend.config.ResolveIdentity(ctx); err != nil {
			return executionapi.WorkspaceInspection{}, err
		}
	}
	inspection, err := workspace.Inspect(ctx, workspaceGit(backend.config), workspace.Repository{Path: backend.config.Repository, CommonDir: backend.config.Identity()}, &executionapi.WorkspaceIntent{Path: ref.WorkspacePath, Branch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit})
	if err != nil {
		return executionapi.WorkspaceInspection{}, err
	}
	return executionapi.WorkspaceInspection{Repository: inspection.Repository, Worktree: inspection.Worktree, Branch: inspection.Branch, Base: inspection.Base, Head: inspection.Head, Dirty: inspection.Dirty, CheckedAt: inspection.CheckedAt}, nil
}
func (backend localExecutionBackend) StopExecution(ctx context.Context, ref executionapi.ExecutionRef, _ executionapi.Operation, handle executionapi.StopHandle) (executionapi.StopReceipt, error) {
	if err := backend.validate(ref); err != nil {
		return executionapi.StopReceipt{}, err
	}
	if handle == nil || handle.Identity() != ref.External {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "stop handle differs"}
	}
	obs, err := backend.Observe(ctx, ref)
	if err != nil {
		return executionapi.StopReceipt{}, err
	}
	if obs.Liveness == "exited" {
		return executionapi.StopReceipt{Disposition: "confirmed", Diagnostic: obs.Diagnostic}, nil
	}
	if obs.Liveness != "live" {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "process liveness is unknown"}
	}
	handle.Kill()
	return executionapi.StopReceipt{Disposition: "accepted"}, nil
}
func (backend localExecutionBackend) validate(ref executionapi.ExecutionRef) error {
	if ref.Backend != executionapi.BackendLocal || ref.Target != targetFor(backend.config) || ref.External.Kind != "os" || ref.External.PID <= 0 || ref.External.Created == "" || ref.External.Executable == "" {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "fixed local test identity differs"}
	}
	return nil
}

func nativeOrcaConfig(c Config) orcaworker.Config {
	terminal, worker := orcaProfileSettings(c), codexProfileSettings(c)
	return orcaworker.Config{Repository: c.Repository, CommandTimeout: c.CommandTimeout, Target: targetFor(c),
		Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
			return (Runner{Dir: c.Repository, Timeout: timeout}).Output(ctx, Command{Executable: terminal.Executable, Args: terminal.Args}, args...)
		},
		InspectWorkspace: func(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
			return inspectTestWorkspace(ctx, c, ref)
		},
		AgentVersion: func(ctx context.Context) (string, error) {
			out, err := (Runner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}).Output(ctx, Command{Executable: worker.Executable, Args: worker.Args}, "--version")
			return string(out), err
		}}
}

func inspectTestWorkspace(ctx context.Context, c Config, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
	if ref.Incomplete || ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || !commitHash.MatchString(ref.BaseCommit) {
		return executionapi.WorkspaceInspection{}, errors.New("workspace ref is incomplete")
	}
	if c.Identity() == "" {
		if err := c.ResolveIdentity(ctx); err != nil {
			return executionapi.WorkspaceInspection{}, err
		}
	}
	repository, err := workspaceRepository(c)
	if err != nil {
		return executionapi.WorkspaceInspection{}, err
	}
	inspection, err := workspace.Inspect(ctx, workspaceGit(c), repository, &executionapi.WorkspaceIntent{Path: ref.WorkspacePath, Branch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit})
	if err != nil {
		return executionapi.WorkspaceInspection{}, err
	}
	return executionapi.WorkspaceInspection{Repository: inspection.Repository, Worktree: inspection.Worktree, Branch: inspection.Branch, Base: inspection.Base, Head: inspection.Head, Dirty: inspection.Dirty, CheckedAt: inspection.CheckedAt}, nil
}

func nativeOrcaClient(c Config) orcaworker.Client { return orcaworker.New(nativeOrcaConfig(c)) }
func nativeOrcaFixedAgentTitle(executionID string) string {
	return orcaworker.FixedAgentTitle(executionID)
}
func nativeOrcaFixedAgentCommand(c Config) (string, error) { return nativeOrcaCommand(c, "", nil) }
func nativeOrcaCommand(c Config, path string, roots []string) (string, error) {
	worker, terminal := codexProfileSettings(c), orcaProfileSettings(c)
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
	command, err := codexworker.ManagedCommand(codexworker.ManagedCommandOptions{Policy: c.ExecutionPolicy, WorkingDirectory: path, WriteRoots: roots, Host: c.Host, Agent: c.Agent, Model: worker.Model, ReasoningEffort: worker.ReasoningEffort, Executable: worker.Executable, Args: worker.Args, Sandbox: permissions.Sandbox, WindowsSandbox: permissions.WindowsSandbox})
	if err != nil {
		return "", err
	}
	return orcaworker.WrapAgentCommand(command, terminal.Shell)
}

func remoteWorkerConfig(c Config) remote.Config {
	worker, settings := codexProfileSettings(c), remoteProfileSettings(c)
	terminalClient := nativeOrcaClient(c)
	config := remote.Config{Repository: c.Repository, ProjectID: c.Identity(), Agent: c.Agent, Model: worker.Model, ReasoningEffort: worker.ReasoningEffort, CommandTimeout: c.CommandTimeout, Call: terminalClient.Raw, Origin: func(ctx context.Context) ([]byte, error) {
		return (Runner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second)}).Output(ctx, c.Git, "remote", "get-url", c.Remote)
	}}
	config.Settings = remote.Settings{ConnectionID: settings.ConnectionID, EnvironmentID: settings.EnvironmentID, RepositorySelector: settings.RepositorySelector, CoordinatorTerminal: settings.CoordinatorTerminal, ExpectedCodexVersion: settings.ExpectedCodexVersion, PollInterval: settings.PollInterval}
	return config
}

func remoteWorkerClient(c Config) remote.Client { return remote.New(remoteWorkerConfig(c)) }
func remoteWorkerError(err error) error {
	var value *remote.Error
	if errors.As(err, &value) {
		return &executionapi.BackendError{Code: executionapi.BackendErrorCode(value.Code), Diagnostic: value.Diagnostic, Cause: value.Cause}
	}
	return err
}
func remoteWorkerRef(ref executionapi.ExecutionRef) remote.Ref { return remote.FromExecutionRef(ref) }

const maxSupervisedMessageBytes = remote.MaxMessageBytes

func testHomePath() string {
	if value := strings.TrimSpace(os.Getenv("STEERLINE_HOME")); value != "" {
		return filepath.Clean(value)
	}
	return filepath.Join(os.TempDir(), "steerline-test")
}

func configTestRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	out, err := exec.Command("git", "init", "--quiet", "-b", "main", repository).CombinedOutput()
	if err != nil {
		t.Fatalf("isolated config repository: %v: %s", err, out)
	}
	return repository
}

func fixtureHome() string      { return testHomePath() }
func fixtureBinDir() string    { return filepath.Join(testHomePath(), "bin") }
func fixtureEtcDir() string    { return filepath.Join(testHomePath(), "etc") }
func fixtureStateRoot() string { return filepath.Join(testHomePath(), "state") }
func fixtureGlobalExecutionConfigPath() string {
	return filepath.Join(testHomePath(), "execution.json")
}

func fixtureRepositoryStateKey(c Config) (string, error) {
	repository, err := workspace.ResolveRepository(context.Background(), workspaceGit(testConfigWithDirectory(c)), c.Repository)
	if err != nil {
		return "", err
	}
	return RepositoryStateKey(repository.CommonDir), nil
}

func loadFixtureConfig(path string) (Config, error) {
	c := Config{Remote: "origin", TaskPoll: "10s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "30s", ShutdownGrace: "10s", Git: Command{Executable: "git"}, ExecutableTypes: []string{"task", "bug", "feature", "chore"}, StallTimeout: "5m"}
	projectPath, err := filepath.Abs(path)
	if err != nil {
		return c, err
	}
	projectBytes, err := os.ReadFile(projectPath)
	if err != nil {
		return c, err
	}
	if err := decode(projectBytes, &c); err != nil {
		return c, err
	}
	if c.TaskAuthority != "native" {
		return c, errors.New("task_authority must be explicitly set to native; Beads is not supported")
	}
	if c.WorkflowProtocol != WorkerFlowVersion {
		return c, errors.New("workflow_protocol must be worker-flow-v1")
	}
	if err := c.ExecutionPolicy.Validate(); err != nil {
		return c, err
	}
	if err := c.ExecutionRequirements.Validate(); err != nil {
		return c, err
	}
	if err := publication.ValidateNonSecret(c.PR, c.Git); err != nil {
		return c, err
	}
	if c.Host != "" || c.Agent != "" || len(c.ExecutionProfiles) != 0 || c.DefaultExecutionProfile != "" {
		return c, errors.New("execution profiles belong in global execution config")
	}
	globalPath := fixtureGlobalExecutionConfigPath()
	globalBytes, err := os.ReadFile(globalPath)
	if err != nil {
		return c, err
	}
	var global struct {
		Version           int                `json:"version"`
		ExecutionProfiles []ExecutionProfile `json:"execution_profiles"`
		DefaultProfile    string             `json:"default_execution_profile"`
	}
	if err := decode(globalBytes, &global); err != nil {
		return c, err
	}
	if global.Version != 2 || len(global.ExecutionProfiles) == 0 {
		return c, errors.New("global execution config version or profiles are invalid")
	}
	directory, err := mustTestDirectory()
	if err != nil {
		return c, err
	}
	c.ExecutionDirectory, c.ProcessHost, c.StateRoot = directory, testProcessHost{}, fixtureStateRoot()
	c.ExecutionProfiles, c.DefaultExecutionProfile = global.ExecutionProfiles, global.DefaultProfile
	c.BindApplicationConfig(directory, projectPath, globalPath, loadedConfigDigest(projectBytes, globalBytes), true)
	base := filepath.Dir(projectPath)
	if c.Repository == "" || c.Branch == "" {
		return c, errors.New("repository and branch are required")
	}
	if !filepath.IsAbs(c.Repository) {
		c.Repository = filepath.Join(base, c.Repository)
	}
	key, err := fixtureRepositoryStateKey(c)
	if err != nil {
		return c, err
	}
	c.RepositoryStateKey = key
	if c.RunDir == "" {
		c.RunDir = filepath.Join(c.StateRoot, key)
	} else if !filepath.IsAbs(c.RunDir) {
		c.RunDir = filepath.Join(base, c.RunDir)
	}
	if c.WorktreeRoot == "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	} else if !filepath.IsAbs(c.WorktreeRoot) {
		c.WorktreeRoot = filepath.Join(base, c.WorktreeRoot)
	}
	if err := validateExecutionProfiles(c); err != nil {
		return c, err
	}
	initial := global.ExecutionProfiles[0]
	if global.DefaultProfile != "" {
		for _, profile := range global.ExecutionProfiles {
			if profile.ID == global.DefaultProfile {
				initial = profile
			}
		}
	}
	c = c.ApplyExecutionProfile(initial)
	if err := c.ValidateApplicationPaths(); err != nil {
		return c, err
	}
	return c, nil
}

func testStartService(ctx context.Context, configPath string, options StartServiceOptions) (ServiceStartResult, error) {
	config, err := loadFixtureConfig(configPath)
	if err != nil {
		return ServiceStartResult{}, err
	}
	if options.ApplicationHome == "" {
		options.ApplicationHome = fixtureHome()
	}
	if options.ApplicationBin == "" {
		options.ApplicationBin = fixtureBinDir()
	}
	return StartService(ctx, config, configPath, options)
}

func testRunConfiguredService(ctx context.Context, config Config, mode, startupID, console string) error {
	return RunConfiguredService(ctx, testConfigWithDirectory(config), mode, startupID, console, newTestService)
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
