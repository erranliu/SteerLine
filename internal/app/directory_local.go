package app

import (
	"context"
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
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
)

// localExecutionRegistration captures only this catalog instance; callbacks run after binding.
func localExecutionRegistration(processHost executionbackend.ManagedProcessHost, directory func() *execution.ExecutionDirectory) (execution.ImplementationProfileCodec, execution.ExecutionRegistration) {
	key := execution.ExecutionRegistrationKey{Backend: executionbackend.BackendLocal, Agent: "codex", Protocol: executionbackend.ProtocolLocalCodexAppServer}
	codec := execution.ImplementationProfileCodec{Key: codexworker.ProfileCodecKey, Version: codexworker.ProfileCodecVersion, Normalize: func(payload []byte, host, agent string) ([]byte, []string, error) {
		return codexworker.NormalizeProfilePayload(payload, runtime.GOOS, host, agent)
	}}
	return codec, execution.ExecutionRegistration{
		ServiceStopBehavior: codexworker.ServiceStopBehavior, Key: key, ProfileKey: "local/local_codex_app_server_v1/codex", WorkspaceBranchPrefix: "codex", LocalWorkspace: true, ProfileCodecKeys: []string{codexworker.ProfileCodecKey},
		QuestionCodecKey: codexworker.QuestionRouteImplementationKey, QuestionCodecVersion: 1, Host: "local",
		Backend: func(c execution.Config) executionbackend.ExecutionBackend {
			return localExecutionBackend{config: c, target: localTarget(), host: processHost, directory: directory()}
		},
		Target: func(execution.Config) executionbackend.ExecutionTarget { return localTarget() },

		Lifecycle: func(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
			setup.ProcessHost = processHost
			return codexworker.NewExecutionLifecycle(setup)
		},
		ValidateConfig: func(ctx context.Context, c execution.Config) error { return validateLocalConfig(ctx, c, directory()) },
		Preflight: func(ctx context.Context, c execution.Config) error {
			return preflightLocalConfig(ctx, c, directory(), processHost)
		},
		PrepareClaim: func(ctx context.Context, c execution.Config) (execution.ClaimPreparation, error) {
			return prepareLocalClaim(ctx, c, directory(), processHost)
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
	}
}

func localTarget() executionbackend.ExecutionTarget {
	hostname, _ := os.Hostname()
	return executionbackend.ExecutionTarget{ID: "local:" + runtime.GOOS + ":" + hostname}
}

func codexSettings(c execution.Config, directories ...*execution.ExecutionDirectory) (codexworker.ProfileConfig, error) {
	payload, err := c.DecodeImplementationConfig(codexworker.ProfileCodecKey, directories...)
	if err != nil {
		return codexworker.ProfileConfig{}, err
	}
	return codexworker.DecodeProfileConfig(payload)
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

func prepareLocalClaim(ctx context.Context, c execution.Config, directory *execution.ExecutionDirectory, processHost executionbackend.ManagedProcessHost) (execution.ClaimPreparation, error) {
	_, err := policyToolVersion(ctx, c, directory, processHost)
	return execution.ClaimPreparation{}, err
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
	if c.Host != "local" {
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

func capabilityReport(backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec, version string) executionbackend.CapabilityReport {
	capabilities := []executionbackend.Capability{{Name: "start", Status: executionbackend.CapabilityNotTested}, {Name: "observe", Status: executionbackend.CapabilityNotTested}, {Name: "stop", Status: executionbackend.CapabilityNotTested}, {Name: "detach_observation", Status: executionbackend.CapabilityNotTested}, {Name: "operation_reconciliation", Status: executionbackend.CapabilityNotTested}, {Name: "workspace_inspection", Status: executionbackend.CapabilityNotTested}, {Name: "output_reading", Status: executionbackend.CapabilityNotTested}, {Name: "output_turns", Status: executionbackend.CapabilityNotTested}, {Name: "output_final_answer", Status: executionbackend.CapabilityNotTested}, {Name: "resource_settlement", Status: executionbackend.CapabilityUnsupported}}
	return executionbackend.CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: time.Now().UTC(), Capabilities: capabilities}
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
