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

const executionImplementationVersion = "builtin-execution-v1"

// Registration keys identify implementations; concrete targets remain separate.
// All service callbacks run on the center writer and retain its existing commits.
type executionRegistrationKey struct {
	Backend  executionapi.BackendKind
	Agent    string
	Protocol executionapi.ExecutionProtocol
}
type executionRegistration struct {
	VerifyWorker      func(*Service, context.Context, *Run, *ExecutionRecord) error
	OrcaOperations    func(Config) orcaExecutionOperations
	RelayResult       func(*Service, context.Context, *Run, *ExecutionRecord, *OrcaNativeTurn) (bool, error)
	ClearRelay        func(*ExecutionRecord, *Run, *OrcaNativeTurn)
	WorkspaceRequest  func(*Run) (workerWorkspacePreparationInput, error)
	PrepareWorkspace  func(*Service, context.Context, *Run, string) (bool, error)
	WaitFacts         func(*Run, *ExecutionRecord) workerWaitFacts
	SettleRequests    func(*Service, *Run, *WorkflowRunRecord, *ExecutionRecord, workerTurnEvidence) (bool, error)
	Cleanup           func(*Service, context.Context, *Run) error
	Key               executionRegistrationKey
	Preflight         func(context.Context, Config) error
	Host              string
	Prepare           func(*Service, context.Context, TaskRecord, *Run) error
	Start             func(*Service, context.Context, *Run, string, string, string) error
	Resume            func(*Service, context.Context, *TaskRecord, *Run) (bool, error)
	PendingIdentity   func(string) (string, string)
	TurnInstruction   func(string, string) string
	DecodeEvent       func(WorkerEvent, *executionapi.SessionContext, bool) (workerFlowEventFact, error)
	ReadTurn          func(workerFlowContext) (workerTurnEvidence, error)
	PrepareFeedback   func(*Service, context.Context, *Run, *ExecutionRecord, DecisionAnswer, string) (bool, error)
	RecoverFeedback   func(*Service, context.Context, *Run, *ExecutionRecord, DecisionAnswer, string) (bool, error)
	DeliverFeedback   func(*Service, context.Context, *Run, *WorkflowRunRecord, *ExecutionRecord, DecisionAnswer, string) error
	Backend           func(Config) executionapi.ExecutionBackend
	Target            func(Config) executionapi.ExecutionTarget
	ProbeRequirements func(context.Context, Config) (executionapi.CapabilityReport, error)
	QueryTool         func(context.Context, Config, string) executionapi.RequirementEvidence
}

func buildExecutionRegistry(entries []executionRegistration) (map[executionRegistrationKey]*executionRegistration, error) {
	result := make(map[executionRegistrationKey]*executionRegistration, len(entries))
	routes := map[string]bool{}
	for _, entry := range entries {
		if entry.Key.Backend == "" || entry.Key.Agent == "" || entry.Key.Protocol == "" || entry.Host == "" || result[entry.Key] != nil {
			return nil, fmt.Errorf("duplicate or incomplete execution registration: %+v", entry.Key)
		}
		if entry.Preflight == nil || entry.Prepare == nil || entry.Start == nil || entry.Resume == nil || entry.PendingIdentity == nil || entry.Backend == nil || entry.Target == nil || entry.ProbeRequirements == nil || entry.QueryTool == nil || entry.DeliverFeedback == nil || entry.DecodeEvent == nil || entry.ReadTurn == nil {
			return nil, fmt.Errorf("incomplete execution operations: %+v", entry.Key)
		}
		if entry.Key.Backend == executionapi.BackendOrca && (entry.OrcaOperations == nil || entry.VerifyWorker == nil) {
			return nil, fmt.Errorf("incomplete Orca operations: %+v", entry.Key)
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
			Key: executionRegistrationKey{executionapi.BackendLocal, "codex", executionapi.ProtocolLocalCodexAppServer}, Host: "local", Preflight: preflightCodexWorkerFlow, WorkspaceRequest: workerWorkspaceRequest, PrepareWorkspace: prepareLocalWorkerWorkspace, WaitFacts: codexWorkerWaitFacts, SettleRequests: settleCodexRequests,
			Prepare: (*Service).prepareCodexWorker, Start: (*Service).startCodexWorkerTurn, Resume: (*Service).resumeCodexWorker,
			PendingIdentity: codexworker.PendingIdentity, DecodeEvent: decodeCodexWorkerFlowEvent, ReadTurn: readCodexWorkerTurnEvidence,
			RecoverFeedback: recoverCodexFeedback, DeliverFeedback: deliverCodexFeedback,
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
			Key: executionRegistrationKey{executionapi.BackendOrca, "codex", executionapi.ProtocolOrcaTerminalV2}, Host: "orca", OrcaOperations: nativeOrcaOperations, VerifyWorker: (*Service).verifyLocalNativeOrcaWorker, RelayResult: reconcileNativeResultRelay, ClearRelay: clearNativeResultRelay, Preflight: preflightNativeWorkerFlow, Cleanup: (*Service).finishNativeOrcaCleanup,
			Prepare: (*Service).prepareNativeOrcaWorkerFlow, Start: (*Service).startNativeOrcaWorkerTurn,
			Resume: func(s *Service, ctx context.Context, task *TaskRecord, run *Run) (bool, error) {
				_, err := s.resumeNativeOrcaWorker(ctx, task, run)
				return true, err
			},
			PendingIdentity: orcaworker.PendingIdentity,
			TurnInstruction: func(run, turn string) string {
				return "\nOrca Run, Task and Dispatch IDs are separate from the fixed SteerLine workflow_run_id.\n" + nativeOrcaRelayInstruction(run, turn)
			},
			DecodeEvent: rejectDispatchEvent, ReadTurn: rejectDispatchTurn,
			PrepareFeedback: prepareOrcaFeedback, RecoverFeedback: recoverOrcaFeedback, DeliverFeedback: (*Service).applyNativeOrcaDecision,
			Backend: func(c Config) executionapi.ExecutionBackend { return orcaworker.NewBackend(nativeOrcaConfig(c)) },
			Target: func(c Config) executionapi.ExecutionTarget {
				return executionapi.ExecutionTarget{ID: "orca:" + digestValue(struct {
					Executable string
					Args       []string
				}{c.Orca.Executable, c.Orca.Args})[:24]}
			},
			ProbeRequirements: func(ctx context.Context, c Config) (executionapi.CapabilityReport, error) {
				return orcaworker.NewBackend(nativeOrcaConfig(c)).Probe(ctx, targetFor(c), agentFor(c))
			}, QueryTool: queryNativeRequirementTool,
		},
		{
			Key: executionRegistrationKey{executionapi.BackendOrca, "codex", executionapi.ProtocolOrcaSupervised}, Host: "orca_supervised", OrcaOperations: remoteOrcaOperations, VerifyWorker: (*Service).verifyRemoteOrcaWorker, Preflight: preflightRemoteWorkerFlow, WorkspaceRequest: remoteWorkerWorkspaceRequest, PrepareWorkspace: prepareRegisteredRemoteWorkspace, Cleanup: (*Service).finishRemoteOrcaCleanup,
			Prepare: (*Service).prepareSupervisedOrcaWorker, Start: (*Service).startSupervisedOrcaWorkerTurn, Resume: (*Service).resumeSupervisedOrcaWorker,
			PendingIdentity: remote.PendingIdentity, DecodeEvent: rejectDispatchEvent, ReadTurn: rejectDispatchTurn,
			DeliverFeedback: (*Service).applyNativeOrcaDecision,
			Backend:         func(c Config) executionapi.ExecutionBackend { return remote.NewBackend(remoteWorkerConfig(c)) },
			Target: func(c Config) executionapi.ExecutionTarget {
				if c.Orca.Supervised == nil {
					return executionapi.ExecutionTarget{}
				}
				return executionapi.ExecutionTarget{ID: "orca-supervised:" + c.Orca.Supervised.ConnectionID, ConnectionID: c.Orca.Supervised.ConnectionID, ExternalRuntime: c.Orca.Supervised.EnvironmentID}
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
func rejectDispatchEvent(WorkerEvent, *executionapi.SessionContext, bool) (workerFlowEventFact, error) {
	return workerFlowEventFact{}, unsupported("Orca event without fixed Dispatch receipt")
}
func rejectDispatchTurn(workerFlowContext) (workerTurnEvidence, error) {
	return workerTurnEvidence{}, unsupported("Orca settlement without fixed Dispatch receipt")
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
