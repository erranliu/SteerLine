package steerline

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

const executionImplementationVersion = "builtin-execution-v2"

// This entrypoint only assembles the existing launcher process/file ports.
// The default application assembly moves out of the center in #66.
func RunCodexSessionLauncher(ctx context.Context, requestPath string) error {
	host := localManagedProcessHost{}
	return codexworker.RunSessionLauncher(ctx, requestPath, host.StartManaged, host.WriteExecutionFile)
}

// Registration keys identify implementations; concrete targets remain separate.
// All service callbacks run on the center writer and retain its existing commits.
type executionRegistrationKey struct {
	Backend  executionapi.BackendKind
	Agent    string
	Protocol executionapi.ExecutionProtocol
}
type executionRegistration struct {
	Key                  executionRegistrationKey
	QuestionCodecKey     string
	QuestionCodecVersion int
	Host                 string
	Backend              func(Config) executionapi.ExecutionBackend
	Target               func(Config) executionapi.ExecutionTarget
	ProbeRequirements    func(context.Context, Config) (executionapi.CapabilityReport, error)
	QueryTool            func(context.Context, Config, string) executionapi.RequirementEvidence
	Lifecycle            func(executionapi.LifecycleSetup) (executionapi.LifecyclePort, error)
}

func buildExecutionRegistry(entries []executionRegistration) (map[executionRegistrationKey]*executionRegistration, error) {
	result := make(map[executionRegistrationKey]*executionRegistration, len(entries))
	routes := map[string]bool{}
	for _, entry := range entries {
		if entry.Key.Backend == "" || entry.Key.Agent == "" || entry.Key.Protocol == "" || entry.Host == "" || result[entry.Key] != nil {
			return nil, fmt.Errorf("duplicate or incomplete execution registration: %+v", entry.Key)
		}
		if entry.Lifecycle == nil || entry.QuestionCodecKey == "" || entry.QuestionCodecVersion <= 0 || entry.Backend == nil || entry.Target == nil || entry.ProbeRequirements == nil || entry.QueryTool == nil {
			return nil, fmt.Errorf("incomplete execution operations: %+v", entry.Key)
		}
		route := entry.Host + ":" + entry.Key.Agent
		if routes[route] {
			return nil, fmt.Errorf("ambiguous execution combination %s", route)
		}
		routes[route] = true
		copy := entry
		result[entry.Key] = &copy
	}
	return result, nil
}

var builtinExecutionRegistry map[executionRegistrationKey]*executionRegistration

func init() {
	entries := []executionRegistration{
		{
			Key: executionRegistrationKey{executionapi.BackendLocal, "codex", executionapi.ProtocolLocalCodexAppServer}, Host: "local", Lifecycle: codexworker.NewExecutionLifecycle,
			QuestionCodecKey: codexworker.QuestionRouteImplementationKey, QuestionCodecVersion: 1,
			Backend: func(c Config) executionapi.ExecutionBackend { return localExecutionBackend{config: c} },
			Target: func(Config) executionapi.ExecutionTarget {
				name, _ := os.Hostname()
				return executionapi.ExecutionTarget{ID: "local:" + runtime.GOOS + ":" + name}
			},
			ProbeRequirements: func(ctx context.Context, c Config) (executionapi.CapabilityReport, error) {
				v, err := policyNativeVersion(ctx, c)
				return capabilityReport(executionapi.BackendLocal, executionapi.ProtocolLocalCodexAppServer, targetFor(c), agentFor(c), v), err
			},
			QueryTool: queryHostRequirementTool,
		},
		{
			Key: executionRegistrationKey{executionapi.BackendOrca, "codex", executionapi.ProtocolOrcaTerminalV2}, Host: "orca", Lifecycle: orcaworker.NewExecutionLifecycle,
			QuestionCodecKey: orcaworker.QuestionRouteImplementationKey, QuestionCodecVersion: 1,
			Backend: func(c Config) executionapi.ExecutionBackend { return orcaworker.NewBackend(nativeOrcaConfig(c)) },
			Target: func(c Config) executionapi.ExecutionTarget {
				settings := orcaProfileSettings(c)
				return executionapi.ExecutionTarget{ID: "orca:" + digestValue(struct {
					Executable string
					Args       []string
				}{settings.Executable, settings.Args})[:24]}
			},
			ProbeRequirements: func(ctx context.Context, c Config) (executionapi.CapabilityReport, error) {
				return orcaworker.NewBackend(nativeOrcaConfig(c)).Probe(ctx, targetFor(c), agentFor(c))
			}, QueryTool: queryNativeRequirementTool,
		},
		{
			Key: executionRegistrationKey{executionapi.BackendOrca, "codex", executionapi.ProtocolOrcaSupervised}, Host: "orca_supervised", Lifecycle: remote.NewExecutionLifecycle,
			QuestionCodecKey: orcaworker.QuestionRouteImplementationKey, QuestionCodecVersion: 1,
			Backend: func(c Config) executionapi.ExecutionBackend { return remote.NewBackend(remoteWorkerConfig(c)) },
			Target: func(c Config) executionapi.ExecutionTarget {
				supervised := remoteProfileSettings(c)
				if supervised.ConnectionID == "" {
					return executionapi.ExecutionTarget{}
				}
				return executionapi.ExecutionTarget{ID: "orca-supervised:" + supervised.ConnectionID, ConnectionID: supervised.ConnectionID, ExternalRuntime: supervised.EnvironmentID}
			},
			ProbeRequirements: func(ctx context.Context, c Config) (executionapi.CapabilityReport, error) {
				return remote.NewBackend(remoteWorkerConfig(c)).Probe(ctx, targetFor(c), agentFor(c))
			}, QueryTool: queryRemoteRequirementTool,
		},
	}
	registry, err := buildExecutionRegistry(entries)
	if err != nil {
		panic(err)
	}
	builtinExecutionRegistry = registry
}

type rejectedExecutionBackend struct{ err error }

func (b rejectedExecutionBackend) Identity() (executionapi.BackendKind, executionapi.ExecutionProtocol) {
	return "", ""
}
func (b rejectedExecutionBackend) Probe(context.Context, executionapi.ExecutionTarget, executionapi.AgentSpec) (executionapi.CapabilityReport, error) {
	return executionapi.CapabilityReport{}, b.err
}
func (b rejectedExecutionBackend) Observe(context.Context, executionapi.ExecutionRef) (executionapi.Observation, error) {
	return executionapi.Observation{}, b.err
}
func (b rejectedExecutionBackend) InspectWorkspace(context.Context, executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
	return executionapi.WorkspaceInspection{}, b.err
}

func registrationFor(c Config) (*executionRegistration, error) {
	for key, entry := range builtinExecutionRegistry {
		if entry.Host == c.hostName() && key.Agent == c.agentName() {
			return entry, nil
		}
	}
	return nil, &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: fmt.Sprintf("unregistered execution combination host=%s agent=%s", c.hostName(), c.agentName())}
}

// Resolve the registered implementation against the original persisted identity.
func workerFlowRegistrationFor(config Config, run *Run, execution *ExecutionRecord) (*executionRegistration, error) {
	if run == nil || execution == nil {
		return nil, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "worker flow has no fixed run or execution"}
	}
	entry, err := registrationFor(config)
	if err != nil {
		return nil, err
	}
	if run.Host != entry.Host || run.Agent != entry.Key.Agent || execution.Agent != run.Agent || run.Backend != string(entry.Key.Backend) || execution.Backend != run.Backend || run.Protocol != string(entry.Key.Protocol) || execution.Protocol != run.Protocol || run.Target != execution.Target || !sameConfiguredExecutionTarget(run.Target, entry.Target(config)) {
		return nil, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "fixed worker target differs from its persisted execution"}
	}
	return entry, nil
}
func sameConfiguredExecutionTarget(actual, configured executionapi.ExecutionTarget) bool {
	return actual.ID != "" && actual.ID == configured.ID && actual.ConnectionID == configured.ConnectionID && actual.ExternalRuntime == configured.ExternalRuntime
}
func (s *Service) workerFlowRegistration(run *Run) (*executionRegistration, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return nil, err
	}
	if fixed.workflow.WorkflowVersion != WorkerFlowVersion {
		return nil, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "unsupported fixed workflow version"}
	}
	return workerFlowRegistrationFor(fixed.config, run, fixed.execution)
}
